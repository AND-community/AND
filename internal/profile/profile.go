// Package profile, kullanıcıların forumdaki kimliklerine eklenen isteğe
// bağlı özelleştirmeleri (biyografi, durum mesajı, ilgi etiketleri, iletişim
// linki) ve varsa moderatör rozetini yönetir. Profil güncellemeleri, forum
// konularıyla aynı desende (Ed25519 imzalı, GossipSub üzerinden yayılan,
// "en yeni kazanır" semantiğiyle) ağa duyurulur ve aynı forum.db SQLite
// dosyasında (bkz. forum.Forum.DB) ayrı bir tabloda saklanır.
//
// Konu/yanıt sayısı ve katılım tarihi gibi istatistikler burada TUTULMAZ —
// bunlar zaten senkronize edilmiş forum verisinden anlık hesaplanır (bkz.
// forum.Forum.AuthorStats), ayrı bir senkronizasyon mekanizması gerektirmez.
package profile

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/timewin"
	"github.com/lucian95511/and/internal/githubverify"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/storage"
)

// ProfileTopic is the GossipSub topic profile updates are published on.
const ProfileTopic = "github.com/lucian95511/and/profile/1.0.0"

// Alan boyut/say sınırları — forum'un title/body limitleriyle aynı ruhta.
// Exported so the TUI can align input CharLimits and live counters with the
// values actually enforced here, instead of duplicating magic numbers.
const (
	MaxBioBytes    = 500
	MaxStatusBytes = 100
	MaxLinkBytes   = 200
	MaxTags        = 5
)

// minRepublishInterval bounds how often a *received* (not locally-authored)
// profile update from the same author is accepted, to blunt a flooding peer
// without a dedicated rate-limiter package for a single low-frequency
// message type. A var (not const) so tests can shrink/grow it.
var minRepublishInterval = 5 * time.Second

// maxClockSkew bounds how far into the future an UpdatedAt may claim to be
// before handleMessage rejects the message outright — mirrors moderation's
// forward-bound sanity check on cert expiry (see verifyCert/maxCertDays).
// Without this, one future-dated message would permanently poison every
// peer's "newest known update" for that author, silently dropping all of
// their subsequent honestly-timestamped updates.
const maxClockSkew = 2 * time.Minute

// Sybil koruması: bir saldırgan ed25519 anahtar çifti üretmenin neredeyse
// bedava olmasından faydalanıp sınırsız sayıda "yeni yazar" profili
// yayınlayarak Store.profiles'i ve profiles SQLite tablosunu sınırsız
// büyütebilir. minRepublishInterval yalnızca AYNI yazardan gelen tekrarları
// sınırlar, farklı anahtarların sayısını değil — bu yüzden forum paketinin
// rlGlobalMaxPosts'una (bkz. forum/ratelimit.go) denk bir küresel, saatlik
// "yeni yazar" sınırı ve toplam profil sayısına sert bir tavan ekliyoruz.
// Tavan aşıldığında YENİ yazarlar reddedilir (var olanlar etkilenmez ve
// tahliye edilmez) — bu, gerçekçi kullanım için astronomik derecede geniş
// bir sınır olduğundan, meşru büyümeyi neredeyse hiç etkilemeden sürekli bir
// saldırıya karşı basit ve güvenli bir arka plan sağlar.
const newAuthorWindow = time.Hour

// maxNewAuthorsPerHour/maxStoredProfiles are vars (not consts) so tests can
// shrink them for fast, deterministic Sybil-cap coverage instead of having
// to actually mint thousands of ed25519 keys.
var (
	maxNewAuthorsPerHour = 200
	maxStoredProfiles    = 5000
)

// Role identifies a peer's highest known standing in the network.
type Role int

const (
	RoleMember Role = iota
	RoleModerator
	RoleFounder
)

// Label returns a short Turkish label for r, e.g. for TUI display.
func (r Role) Label() string {
	switch r {
	case RoleFounder:
		return "Kurucu"
	case RoleModerator:
		return "Moderatör"
	default:
		return "Üye"
	}
}

// Profile is a peer's self-published customization: a bio, a short status
// line, up to MaxTags category tags, an optional contact link, an optional
// verified GitHub username, and optionally a moderator certificate proving
// current moderator status. Unlike Link (free text, never checked against
// anything), GitHubUser is only ever set locally after a successful
// githubverify OAuth Device Flow round-trip — the signature over it just
// proves THIS author claims it, the same trust level as every other field
// here; a peer receiving it has no way to independently re-verify the
// GitHub ownership claim without redoing the OAuth flow themselves.
// ModCert IS covered by Sig (see profilePayload) only insofar as its
// presence/ModeratorKey are signed — its own authority comes from its own
// founder signature (see moderation.ModeratorCert), so a stale or
// mismatched cert is simply ignored by IsModerator rather than
// invalidating the whole message.
type Profile struct {
	AuthorKey  string                    `json:"author_key"`
	Bio        string                    `json:"bio"`
	Status     string                    `json:"status,omitempty"`
	Tags       []string                  `json:"tags,omitempty"`
	Link       string                    `json:"link,omitempty"`
	GitHubUser string                    `json:"github_user,omitempty"`
	UpdatedAt  time.Time                 `json:"updated_at"`
	ModCert    *moderation.ModeratorCert `json:"mod_cert,omitempty"`
	Sig        string                    `json:"sig"`
}

// OwnUpdate is the full set of self-editable fields PublishOwn broadcasts.
// The wire format has no concept of a partial update — every publish
// replaces the whole Profile — so callers editing a single field (e.g. just
// Status) must pass the other fields' current values through unchanged.
type OwnUpdate struct {
	Bio        string
	Status     string
	Tags       []string
	Link       string
	GitHubUser string
}

type Store struct {
	mu       sync.RWMutex
	profiles map[string]*Profile
	lastSeen map[string]time.Time

	// newAuthorTimes: son newAuthorWindow içinde kabul edilen, o âna kadar
	// bilinmeyen yazarların kabul zamanları — Sybil koruması (bkz. paket
	// yorumu ve handleMessage).
	newAuthorTimes []time.Time

	identity *stdcrypto.Identity
	topic    *network.Topic
	db       *storage.DB
	mod      *moderation.Moderator
}

// New loads any profiles already persisted in db and returns a Store ready
// to publish/receive updates over topic. topic may be nil (tests, or a peer
// that failed to join the pubsub topic) — Run() then becomes a no-op, and
// PublishOwn stops after the local persist step (no network broadcast).
func New(id *stdcrypto.Identity, topic *network.Topic, db *storage.DB, mod *moderation.Moderator) (*Store, error) {
	s := &Store{
		profiles: make(map[string]*Profile),
		lastSeen: make(map[string]time.Time),
		identity: id,
		topic:    topic,
		db:       db,
		mod:      mod,
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("profile: load: %w", err)
	}
	return s, nil
}

func (s *Store) load() error {
	raws, err := s.db.AllProfiles()
	if err != nil {
		return err
	}
	for _, rp := range raws {
		p := &Profile{
			AuthorKey:  rp.AuthorKey,
			Bio:        rp.Bio,
			Status:     rp.Status,
			Tags:       splitTags(rp.Tags),
			Link:       rp.Link,
			GitHubUser: rp.GitHubUser,
			UpdatedAt:  rp.UpdatedAt,
			Sig:        rp.Sig,
		}
		if rp.ModCertJSON != "" {
			var cert moderation.ModeratorCert
			if json.Unmarshal([]byte(rp.ModCertJSON), &cert) == nil {
				p.ModCert = &cert
			}
		}
		s.profiles[p.AuthorKey] = p
	}
	return nil
}

// Run consumes incoming profile updates until ctx is done.
func (s *Store) Run(ctx context.Context) {
	if s.topic == nil {
		return
	}
	ch := s.topic.Messages(ctx)
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			s.handleMessage(data)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Store) handleMessage(data []byte) {
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return
	}
	if p.AuthorKey == "" ||
		len(p.Bio) > MaxBioBytes ||
		len(p.Status) > MaxStatusBytes ||
		len(p.Link) > MaxLinkBytes ||
		!validTags(p.Tags) ||
		!githubverify.ValidUsername(p.GitHubUser) {
		return
	}
	// Gelecek tarihli bir UpdatedAt, o yazar için "en yeni" olarak önbelleğe
	// alınırsa gerçek sahibinin sonraki dürüst zamanlı güncellemelerini
	// kalıcı olarak "eski" gösterip sessizce reddettirir — mesajı en baştan
	// at, hiçbir zaman referans alınmasın.
	if p.UpdatedAt.After(time.Now().Add(maxClockSkew)) {
		return
	}
	if !verifyProfile(&p) {
		return
	}

	s.mu.Lock()
	existing, known := s.profiles[p.AuthorKey]
	if known && !p.UpdatedAt.After(existing.UpdatedAt) {
		s.mu.Unlock()
		return // eski/tekrar (replay) — en yeni kazanır
	}
	if last, ok := s.lastSeen[p.AuthorKey]; ok && time.Since(last) < minRepublishInterval {
		s.mu.Unlock()
		return
	}
	if !known {
		now := time.Now()
		cutoff := now.Add(-newAuthorWindow)
		s.newAuthorTimes = keepAfter(s.newAuthorTimes, cutoff)
		if len(s.newAuthorTimes) >= maxNewAuthorsPerHour {
			s.mu.Unlock()
			return
		}
		if len(s.profiles) >= maxStoredProfiles {
			s.mu.Unlock()
			return
		}
		s.newAuthorTimes = append(s.newAuthorTimes, now)
	}
	s.lastSeen[p.AuthorKey] = time.Now()
	s.mu.Unlock()

	s.persist(&p)
}

// persist is the single authoritative, lock-atomic "newest wins" gate for
// writing into s.profiles: it applies to EVERY caller (handleMessage's own
// received-messages, in addition to its own earlier check under a
// separately-acquired lock — see below — and PublishOwn, which had no such
// check at all before this). Without it, two concurrent local PublishOwn
// calls for the same author (e.g. cmd/and's delayed startup republish
// racing a user's own Settings edit) could persist in either order — the
// one whose persist() call happens to run last wins in memory/disk
// regardless of which one actually carries the newer UpdatedAt, silently
// discarding a more recent edit. handleMessage's existing pre-check (done
// while holding the lock, but released before calling persist) has the same
// gap for two concurrent network messages; this closes it for every caller
// uniformly instead of duplicating the check at each call site.
func (s *Store) persist(p *Profile) {
	s.mu.Lock()
	if existing, ok := s.profiles[p.AuthorKey]; ok && !p.UpdatedAt.After(existing.UpdatedAt) {
		s.mu.Unlock()
		return
	}
	s.profiles[p.AuthorKey] = p
	s.mu.Unlock()

	var modCertJSON string
	if p.ModCert != nil {
		if b, err := json.Marshal(p.ModCert); err == nil {
			modCertJSON = string(b)
		}
	}
	_ = s.db.UpsertProfile(storage.RawProfile{
		AuthorKey: p.AuthorKey, Bio: p.Bio, Status: p.Status,
		Tags: strings.Join(p.Tags, ","), Link: p.Link, GitHubUser: p.GitHubUser,
		UpdatedAt: p.UpdatedAt, ModCertJSON: modCertJSON, Sig: p.Sig,
	})
}

// PublishOwn signs and broadcasts the local identity's full profile (see
// OwnUpdate — every publish replaces the whole thing), and stores it
// locally regardless of how recently it last published (the rate limit in
// handleMessage only applies to messages received from the network). If
// the local identity holds a currently valid moderator certificate (see
// moderation.Moderator.FindOwnCert), it's attached so other peers can show
// the "Moderatör" badge without a separate lookup.
func (s *Store) PublishOwn(ctx context.Context, u OwnUpdate) error {
	if len(u.Bio) > MaxBioBytes {
		return fmt.Errorf("profile: bio en fazla %d bayt olabilir", MaxBioBytes)
	}
	if len(u.Status) > MaxStatusBytes {
		return fmt.Errorf("profile: durum mesajı en fazla %d bayt olabilir", MaxStatusBytes)
	}
	if len(u.Link) > MaxLinkBytes {
		return fmt.Errorf("profile: link en fazla %d bayt olabilir", MaxLinkBytes)
	}
	if !validTags(u.Tags) {
		return fmt.Errorf("profile: etiketler en fazla %d adet olmalı ve forum kategorilerinden seçilmeli", MaxTags)
	}
	if !githubverify.ValidUsername(u.GitHubUser) {
		return fmt.Errorf("profile: geçersiz GitHub kullanıcı adı")
	}

	p := &Profile{
		AuthorKey:  hex.EncodeToString(s.identity.PublicKey()),
		Bio:        u.Bio,
		Status:     u.Status,
		Tags:       append([]string(nil), u.Tags...),
		Link:       u.Link,
		GitHubUser: u.GitHubUser,
		UpdatedAt:  time.Now().UTC(),
	}
	if cert := s.mod.FindOwnCert(p.AuthorKey); cert != nil {
		p.ModCert = cert
	}
	sig := ed25519.Sign(s.identity.PrivateKey(), profilePayload(p))
	p.Sig = hex.EncodeToString(sig)

	// persist() itself doesn't look at ctx (it's a synchronous local
	// write, not a network call) — without this check, cancelling ctx
	// (e.g. the TUI's GitHub-link "esc" handler) would stop the
	// network Publish below but NOT the local persist, so a user who
	// backs out at exactly the wrong instant would see "iptal edildi"
	// while the value became permanent locally anyway.
	if err := ctx.Err(); err != nil {
		return err
	}
	s.persist(p)

	if s.topic == nil {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("profile: marshal: %w", err)
	}
	return s.topic.Publish(ctx, data)
}

// Get returns a snapshot of the cached profile for authorKey, or nil if
// that author has never published one we've seen. The returned value is a
// copy — mutating it has no effect on the store and is always safe, even
// concurrently with Run()'s background updates.
func (s *Store) Get(authorKey string) *Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[authorKey]
	if !ok {
		return nil
	}
	cp := *p
	cp.Tags = append([]string(nil), p.Tags...)
	return &cp
}

// IsFounder reports whether authorKey is the network's founder key.
func IsFounder(authorKey string) bool {
	return strings.EqualFold(authorKey, moderation.FounderPubKeyHex)
}

// IsModerator reports whether authorKey's cached profile carries a
// currently-valid moderator certificate issued to that exact key. A
// ModCert whose ModeratorKey doesn't match authorKey is rejected — without
// this check, anyone could copy a moderator's cert (visible on the network
// in every BanMsg they send) onto their own profile and impersonate the
// badge.
func (s *Store) IsModerator(authorKey string) bool {
	p := s.Get(authorKey)
	if p == nil || p.ModCert == nil {
		return false
	}
	if !strings.EqualFold(p.ModCert.ModeratorKey, authorKey) {
		return false
	}
	return s.mod.VerifyModeratorCert(*p.ModCert)
}

// RoleOf reports authorKey's highest known role.
func (s *Store) RoleOf(authorKey string) Role {
	if IsFounder(authorKey) {
		return RoleFounder
	}
	if s.IsModerator(authorKey) {
		return RoleModerator
	}
	return RoleMember
}

// RoleLabel returns a short Turkish label for authorKey's highest known
// role: "Kurucu" (founder), "Moderatör" (verified cert), or "Üye" (member).
func (s *Store) RoleLabel(authorKey string) string {
	return s.RoleOf(authorKey).Label()
}

// validTags reports whether tags is a valid Tags value: at most MaxTags
// entries, each a real forum category, with no duplicates. Applied both to
// locally-authored updates (PublishOwn) and to messages received from the
// network (handleMessage) — never trust a remote Tags value just because
// its signature checks out.
func validTags(tags []string) bool {
	if len(tags) > MaxTags {
		return false
	}
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		if !forum.ValidCategory(t) || seen[t] {
			return false
		}
		seen[t] = true
	}
	return true
}

func splitTags(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, ",")
}

// keepAfter drops the leading run of timestamps at or before cutoff from a
// slice assumed sorted ascending (entries are always appended in arrival
// order) — mirrors forum/ratelimit.go's helper of the same name/shape.
var keepAfter = timewin.KeepAfter

// signablePayload mirrors Profile's self-asserted fields — everything the
// author's signature must cover — excluding Sig itself. ModCert's own bytes
// aren't included (its authority comes from its own founder signature, see
// IsModerator), but its ModeratorKey IS, so a relay can't silently strip or
// swap a moderator's cert onto/off of an otherwise-unmodified, still
// validly-signed message.
//
// JSON encoding (rather than a hand-joined "|"-separated string, which this
// replaced) matters for more than style: Bio/Status/Link are free text with
// no character restriction, only a byte-length cap, so a "|"-joined payload
// let anyone — no private key needed — take a validly-signed message and
// shift a field boundary at a literal "|" inside Bio, producing a DIFFERENT
// Bio/Status split whose recomputed payload bytes were IDENTICAL to the
// original, so the original signature still verified. JSON's per-field
// string escaping and explicit key/value structure has no such ambiguous
// boundary.
type signablePayload struct {
	AuthorKey    string    `json:"author_key"`
	Bio          string    `json:"bio"`
	Status       string    `json:"status"`
	Tags         []string  `json:"tags"`
	Link         string    `json:"link"`
	GitHubUser   string    `json:"github_user"`
	ModeratorKey string    `json:"moderator_key,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func profilePayload(p *Profile) []byte {
	sp := signablePayload{
		AuthorKey: p.AuthorKey, Bio: p.Bio, Status: p.Status,
		Tags: p.Tags, Link: p.Link, GitHubUser: p.GitHubUser, UpdatedAt: p.UpdatedAt,
	}
	if p.ModCert != nil {
		sp.ModeratorKey = p.ModCert.ModeratorKey
	}
	// signablePayload'ın tüm alanları temel tipler (string/[]string/time.Time)
	// — json.Marshal bunlar için asla hata döndürmez.
	data, _ := json.Marshal(sp)
	return data
}

func verifyProfile(p *Profile) bool {
	pub, err := hex.DecodeString(p.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(p.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), profilePayload(p), sig)
}
