// Package chatlog persists messages sent over the app's global chat
// GossipSub topic, in the same spirit as internal/forum persists posts:
// signed, stored in SQLite, and exchanged with peers via a sync protocol
// (see sync.go) so a peer who was offline — or who just joined the network
// — catches up on chat history instead of only ever seeing messages sent
// after they happened to be listening.
//
// The wire format ("n"/"t"/"a" JSON keys) intentionally matches the
// pre-existing, unsigned {name, text, at} packet the plugin API's chat
// adapter (cmd/and's chatAdapter) has always sent/received on the same
// topic — a plugin-originated message still displays live for every peer
// exactly as before, it just can't be verified or persisted/synced (no
// AuthorKey/Sig), so it stays exactly as ephemeral as chat already was.
// Only messages sent through Store.Send (and thus signed) get the
// durability this package adds.
package chatlog

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/storage"
	"github.com/lucian95511/and/internal/timewin"
)

// MessageTTL bounds how long a message stays before it's purged — chat
// history is durable but not kept forever.
const MessageTTL = 48 * time.Hour

// MaxStoredMessages caps how many messages this peer keeps at once,
// independent of MessageTTL — once exceeded, the oldest are evicted (from
// memory and SQLite alike) to make room for new ones. A var so tests can
// shrink it.
var MaxStoredMessages = 500

// MaxTextBytes bounds a single message's body — much smaller than forum's
// post body limit (8192) since these are chat lines, not long-form posts,
// and every message here rides the same 64KB-capped GossipSub topic
// (see network.NewPubSub's WithMaxMessageSize). A var (not const) so tests
// can shrink it — the TUI's chat input caps at 500 runes (≤2000 UTF-8
// bytes), so this limit is otherwise unreachable through realistic input.
var MaxTextBytes = 2000

// maxClockSkew bounds how far into the future a SentAt may claim to be
// before handleMessage/SyncWithPeer reject the message outright. Without
// this, an attacker could stamp SentAt far in the future — since
// ExpiresAt = SentAt + MessageTTL, that pushes the message's expiry equally
// far out, making it immune to cleanup's TTL purge and effectively
// permanent. Mirrors profile.maxClockSkew.
const maxClockSkew = 2 * time.Minute

// Sybil/flood resistance, mirroring internal/forum/ratelimit.go's
// allowPost/allowReply (per-author + network-wide sliding-window caps) —
// applied to both handleMessage (live GossipSub receipt) and SyncWithPeer
// (history backfill), so neither path bypasses it. Without this, an
// attacker (ed25519 keypairs are free to mint) could publish unlimited
// validly-signed messages, each permanently written to every peer's
// chat_messages table (shared with forum.db) and propagated to every new
// peer via sync. Chat is legitimately much higher-frequency than forum
// posts (real-time conversation vs. deliberate posting), so these limits
// are looser than forum's rlMaxPosts/rlGlobalMaxPosts.
const rlWindow = time.Hour

// rlSaveInterval: how often dirty rate-limit/mute/duplicate state is
// flushed to disk — mirrors internal/forum/ratelimit.go's identical
// constant/rationale (one background loop instead of a goroutine per
// accepted message).
const rlSaveInterval = 30 * time.Second

// rlMaxPerAuthor/rlGlobalMax/rlCooldown are vars (not consts) so tests can
// shrink them for fast, deterministic rate-limit coverage.
var (
	rlMaxPerAuthor = 120  // saatte yazar başına en fazla 120 mesaj (~2/dk) — canlı sohbet için bol, tek kimlikten flood'u sınırlar
	rlGlobalMax    = 2000 // saatte tüm ağdan en fazla bu kadar mesaj — sahte kimlik sayısı ne olursa olsun toplam giriş oranı sabit kalır

	// rlCooldown: art arda hızlı gönderime (patlama/burst spam) karşı ek
	// koruma — saatlik toplam sınır içinde bile biri birkaç saniyede tüm
	// kotasını tüketebilir. forum'un post/reply cooldown'larından çok daha
	// kısa: chat gerçek zamanlı bir sohbet aracı, saniyeler süren bir
	// bekleme doğal yazışmayı bozar — burada amaç yalnızca art arda
	// ölçüsüz hızlı (bot benzeri) gönderimi engellemek.
	rlCooldown = 2 * time.Second
)

// Yeni kimlik kısıtlaması: bu eş bir AuthorKey'i İLK KEZ gördüğünde (bkz.
// authorBucket.FirstSeen), newIdentityGracePeriod boyunca o yazar normal
// (rlMaxPerAuthor/rlCooldown) yerine çok daha sıkı bir hız sınırına tabi
// tutulur — mirrors internal/forum/ratelimit.go's identical mechanism/
// rationale: susturulmuş birinin yeni bir ed25519 anahtarıyla anında eski
// hızında devam etmesini engellemez, ama bunun getirisini düşürür. Vars ki
// testler küçültüp deterministik kalabilsin.
var (
	newIdentityGracePeriod        = 24 * time.Hour
	newIdentityMaxPerAuthor       = 15 // rlMaxPerAuthor (120) yerine
	newIdentityCooldownMultiplier = 4  // rlCooldown'ı bu kadar çarpar
)

// isNewIdentity reports whether b's author is still within its
// newIdentityGracePeriod — bkz. internal/forum/ratelimit.go'daki aynı
// isimli fonksiyon (zero FirstSeen = eski/kurulmuş kimlik sayılır).
func isNewIdentity(b *authorBucket, now time.Time) bool {
	return !b.FirstSeen.IsZero() && now.Sub(b.FirstSeen) < newIdentityGracePeriod
}

// Message is a single chat line. AuthorKey/Sig are empty for messages that
// arrived from an unsigned source (see package doc) — those are shown live
// but never passed to persist/sync.
type Message struct {
	ID         string    `json:"id,omitempty"`
	AuthorKey  string    `json:"author_key,omitempty"`
	AuthorName string    `json:"n"`
	Text       string    `json:"t"`
	SentAt     time.Time `json:"a"`
	Sig        string    `json:"sig,omitempty"`
	ExpiresAt  time.Time `json:"-"`
}

// Signed reports whether m carries a verifiable AuthorKey/Sig pair — see
// verify. Exported so callers (the TUI) can decide whether to render an
// "unverified" hint on messages relayed from non-signing sources.
func (m *Message) Signed() bool { return m.AuthorKey != "" && m.Sig != "" }

// authorBucket holds all per-author rate-limit/moderation state — one
// sliding-window message log plus the auto-mute escalation and duplicate-
// text tracking layered on top of it (see allow/recordViolation/
// isDuplicate). Unified into one struct (rather than four separate maps)
// so the whole per-author picture can be persisted to disk in one place —
// see saveRates/loadRates — mirroring internal/forum/ratelimit.go's
// authorBucket.
type authorBucket struct {
	MessageTimes []time.Time `json:"message_times,omitempty"`

	// FirstSeen is when this eş first saw a message from this author —
	// see isNewIdentity. Persisted so a restart doesn't make every known
	// author look "new" again.
	FirstSeen time.Time `json:"first_seen,omitempty"`

	// ViolationTimes/MutedUntil: automatic local mute escalation — see
	// recordViolation/muted.
	ViolationTimes []time.Time `json:"violation_times,omitempty"`
	MutedUntil     time.Time   `json:"muted_until,omitempty"`

	// MuteCount/LastMuteAt: how many times this author has been
	// auto-muted, and when the most recent one started — see
	// recordViolation's escalation logic.
	MuteCount  int       `json:"mute_count,omitempty"`
	LastMuteAt time.Time `json:"last_mute_at,omitempty"`

	// RecentTexts: per-normalized-text repeat counters, used to detect
	// repeat/near-duplicate spam — see isDuplicate.
	RecentTexts map[string]dupEntry `json:"recent_texts,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	messages []*Message
	byID     map[string]*Message

	// authors: per-author rate-limit/mute/duplicate state — see
	// authorBucket. globalTimes stays separate since it's network-wide,
	// not tied to one author.
	authors     map[string]*authorBucket
	globalTimes []time.Time

	dirty   bool // true → in-memory rate state is ahead of the on-disk copy
	dataDir string

	identity *stdcrypto.Identity
	topic    *network.Topic
	db       *storage.DB
	mod      atomic.Pointer[moderation.Moderator]

	newMessages chan *Message
	// historyChanged is signaled when SyncWithPeer backfills messages that
	// weren't delivered live — see that method's doc comment.
	historyChanged chan struct{}
}

// New loads any chat messages already persisted in db, plus any previously
// saved rate-limit/mute state from dataDir, and returns a Store ready to
// send/receive over topic. topic may be nil (tests) — Run() becomes a no-op
// and Send stops after the local persist step. dataDir may be empty (tests)
// — rate-limit state then stays in-memory only, exactly as before this was
// added.
func New(id *stdcrypto.Identity, topic *network.Topic, db *storage.DB, dataDir string) (*Store, error) {
	s := &Store{
		byID:           make(map[string]*Message),
		authors:        make(map[string]*authorBucket),
		identity:       id,
		topic:          topic,
		db:             db,
		dataDir:        dataDir,
		newMessages:    make(chan *Message, 256),
		historyChanged: make(chan struct{}, 1),
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("chatlog: load: %w", err)
	}
	s.loadRates()
	return s, nil
}

func (s *Store) bucket(key string) *authorBucket {
	b, ok := s.authors[key]
	if !ok {
		b = &authorBucket{FirstSeen: time.Now()}
		s.authors[key] = b
	}
	return b
}

// SetModerator bağlar (veya nil ile çözer) bir *moderation.Moderator'ı bu
// Store'a — bağlıysa handleMessage, moderatörlerin verdiği ağ-seviyesi peer
// banlarını, o kişinin AuthorKey'i üzerinden sohbet mesajlarına da uygular
// (bkz. moderation.AuthorKeyToPeerID/IsAuthorBanned'in doc yorumu: AND'de
// peer.ID ve AuthorKey aynı ed25519 anahtarından türetilir). New sonrası,
// Run'dan önce çağrılması yeterli; atomik olduğundan sonradan da güvenle
// değiştirilebilir. Ayarlanmazsa (nil kalırsa) davranış tamamen eskisiyle
// aynıdır — bu yüzden mevcut testler etkilenmez.
func (s *Store) SetModerator(mod *moderation.Moderator) {
	s.mod.Store(mod)
}

func (s *Store) authorBanned(authorKey string) bool {
	mod := s.mod.Load()
	return mod != nil && mod.IsAuthorBanned(authorKey)
}

// allow reports whether authorKey may have another message accepted right
// now, recording the attempt if so. checkCooldown should be true for live
// GossipSub receipt (handleMessage) and false for sync backfill
// (SyncWithPeer) — a batch of a prolific author's legitimately old messages,
// replayed in a tight loop during sync, would otherwise trip the cooldown
// purely because of OUR processing speed, not theirs.
func (s *Store) allow(authorKey string, checkCooldown bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rlWindow)

	s.globalTimes = keepAfter(s.globalTimes, cutoff)
	if len(s.globalTimes) >= rlGlobalMax {
		return false
	}
	b := s.bucket(authorKey)
	maxPerAuthor, cooldown := rlMaxPerAuthor, rlCooldown
	// Yeni-kimlik kısıtlaması yalnızca CANLI gossip alımında uygulanır —
	// bkz. internal/forum/ratelimit.go'daki aynı kontrolün doc yorumu.
	if checkCooldown && isNewIdentity(b, now) {
		maxPerAuthor = newIdentityMaxPerAuthor
		cooldown = rlCooldown * time.Duration(newIdentityCooldownMultiplier)
	}
	times := keepAfter(b.MessageTimes, cutoff)
	if len(times) >= maxPerAuthor {
		b.MessageTimes = times
		return false
	}
	if checkCooldown {
		if n := len(times); n > 0 && now.Sub(times[n-1]) < cooldown {
			b.MessageTimes = times
			return false
		}
	}
	b.MessageTimes = append(times, now)
	s.globalTimes = append(s.globalTimes, now)
	s.dirty = true
	return true
}

// Otomatik yerel susturma: ısrarla hız sınırını ihlal eden bir yazar,
// moderatör müdahalesi olmadan bir süreliğine görmezden gelinir — ağ
// geneli bir yasak DEĞİL, yalnızca BU eşin o yazardan gelen mesajları
// (imza doğrulaması dahil hiçbir işlem yapmadan) atlaması. Vars (const
// değil) ki testler bunları küçültüp hızlı/deterministik kalabilsin.
//
// Bilinen sınır: bu, AuthorKey'e göre izleniyor — yeni bir ed25519 anahtar
// çifti üretmek bedavadır, dolayısıyla susturulmuş biri anında yeni bir
// kimlikle "temiz" başlayabilir. Bu, bireysel-kimlik susturmasının doğal
// bir zayıflığıdır; asıl Sybil koruması rlGlobalMax'tir (kimlik sayısından
// bağımsız, ağ genelinde sabit bir toplam giriş oranı tavanı) — bkz.
// yukarıdaki yorum. Susturma, tek bir ısrarlı kimliğin gürültüsünü
// azaltmak içindir, kararlı bir Sybil savunması olarak tasarlanmamıştır.
var (
	muteViolationWindow    = 10 * time.Minute
	muteViolationThreshold = 10 // bu pencere içinde bu kadar ihlalden sonra sustur
	muteDuration           = 30 * time.Minute
)

// Kademeli susturma: aynı yazar tekrar tekrar susturma eşiğine ulaşırsa,
// her yeni susturma bir öncekinden daha uzun sürer (MuteCount'a göre
// muteDuration'ın katları, muteDurationCap'te sınırlanır) — mirrors
// internal/forum/ratelimit.go's identical mechanism. muteCountDecay kadar
// süre boyunca hiç yeni susturma tetiklenmezse sayaç sıfırlanır. Vars ki
// testler küçültüp deterministik kalabilsin.
var (
	muteDurationCap = 12 * time.Hour
	muteCountDecay  = 7 * 24 * time.Hour
)

// muted reports whether authorKey is currently under a local auto-mute.
func (s *Store) muted(authorKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.authors[authorKey]
	return ok && !b.MutedUntil.IsZero() && time.Now().Before(b.MutedUntil)
}

// recordViolation tracks a rate-limit rejection for authorKey (only called
// for live-received rejections — see handleMessage; sync backfill never
// calls this, since a prolific author's legitimately old messages
// shouldn't count against them just because sync processes many at once)
// and escalates to a temporary local mute if they've violated too many
// times too fast.
func (s *Store) recordViolation(authorKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-muteViolationWindow)
	b := s.bucket(authorKey)
	b.ViolationTimes = append(keepAfter(b.ViolationTimes, cutoff), now)
	if len(b.ViolationTimes) >= muteViolationThreshold {
		wasMuted := !b.MutedUntil.IsZero() && now.Before(b.MutedUntil)
		if !wasMuted {
			if !b.LastMuteAt.IsZero() && now.Sub(b.LastMuteAt) > muteCountDecay {
				b.MuteCount = 0
			}
			b.MuteCount++
			duration := muteDuration * time.Duration(b.MuteCount)
			if duration > muteDurationCap {
				duration = muteDurationCap
			}
			b.MutedUntil = now.Add(duration)
			b.LastMuteAt = now
		}
	}
	s.dirty = true
}

// dupEntry tracks how many times (and when last) an author has sent one
// particular normalized message text — see isDuplicate. Fields are
// exported (with lowercase JSON tags) so authorBucket.RecentTexts can be
// persisted to disk — see saveRates/loadRates.
type dupEntry struct {
	Count int       `json:"count"`
	At    time.Time `json:"at"` // en son görülme zamanı
}

// dupWindow/dupMaxRepeats bound repeat/near-duplicate spam detection: the
// same (normalized) text from the same author is allowed up to
// dupMaxRepeats times within dupWindow — a person might reasonably repeat a
// short phrase a few times ("evet", "+1", …) — before further repeats are
// rejected. The per-text counter resets once dupWindow has elapsed since
// its last occurrence. Vars so tests can shrink them.
var (
	dupWindow     = 5 * time.Minute
	dupMaxRepeats = 6
)

// normalizeForDup collapses trivial formatting differences (case, leading/
// trailing whitespace) so "Merhaba!" and "  merhaba!  " are recognized as
// the same repeated message.
func normalizeForDup(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isDuplicate reports whether text (from authorKey) has already been
// repeated more than dupMaxRepeats times within dupWindow, recording this
// occurrence either way. Only ever called for live-received messages,
// matching recordViolation's scope.
func (s *Store) isDuplicate(authorKey, text string) bool {
	norm := normalizeForDup(text)
	if norm == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	b := s.bucket(authorKey)
	byText := b.RecentTexts
	if byText == nil {
		byText = make(map[string]dupEntry)
		b.RecentTexts = byText
	} else {
		// Bu vesileyle süresi dolmuş kayıtları temizle — her benzersiz metin
		// için haritanın sonsuza dek büyümesini önler.
		for k, e := range byText {
			if now.Sub(e.At) > dupWindow {
				delete(byText, k)
			}
		}
	}

	entry, ok := byText[norm]
	s.dirty = true
	if !ok {
		byText[norm] = dupEntry{Count: 1, At: now}
		return false
	}
	entry.Count++
	entry.At = now
	byText[norm] = entry
	return entry.Count > dupMaxRepeats
}

// keepAfter drops the leading run of timestamps at or before cutoff from a
// slice assumed sorted ascending — mirrors forum/ratelimit.go's and
// profile.go's identical helper.
var keepAfter = timewin.KeepAfter

// HistoryChanged streams a signal whenever SyncWithPeer backfills messages
// that were older than anything already known — see that method's doc
// comment for why those aren't pushed through NewMessages.
func (s *Store) HistoryChanged() <-chan struct{} { return s.historyChanged }

func (s *Store) load() error {
	raws, err := s.db.AllChatMessages()
	if err != nil {
		return err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rm := range raws {
		if !rm.ExpiresAt.IsZero() && rm.ExpiresAt.Before(now) {
			continue
		}
		m := &Message{
			ID: rm.ID, AuthorKey: rm.AuthorKey, AuthorName: rm.AuthorName,
			Text: rm.Text, SentAt: rm.SentAt, Sig: rm.Sig, ExpiresAt: rm.ExpiresAt,
		}
		s.messages = append(s.messages, m)
		s.byID[m.ID] = m
	}
	return nil
}

// pruneBucket returns a fresh *authorBucket containing only b's still-live
// state as of now: sliding-window timestamps older than their own window
// dropped, an already-expired mute cleared, and stale duplicate-text
// entries removed. Shared by saveRates (pruning before writing to disk)
// and loadRates (pruning stale data read back from a possibly-old file) —
// mirrors internal/forum/ratelimit.go's identical helper.
func pruneBucket(b *authorBucket, now time.Time) *authorBucket {
	fresh := &authorBucket{
		MessageTimes:   keepAfter(b.MessageTimes, now.Add(-rlWindow)),
		FirstSeen:      b.FirstSeen,
		ViolationTimes: keepAfter(b.ViolationTimes, now.Add(-muteViolationWindow)),
		MuteCount:      b.MuteCount,
		LastMuteAt:     b.LastMuteAt,
	}
	if b.MutedUntil.After(now) {
		fresh.MutedUntil = b.MutedUntil
	}
	for text, e := range b.RecentTexts {
		if now.Sub(e.At) <= dupWindow {
			if fresh.RecentTexts == nil {
				fresh.RecentTexts = make(map[string]dupEntry)
			}
			fresh.RecentTexts[text] = e
		}
	}
	return fresh
}

// bucketEmpty reports whether b has nothing left worth keeping/persisting,
// as of now — mirrors internal/forum/ratelimit.go's identical helper
// (including keeping a live mute-escalation history alive independently of
// everything else, see that comment for why).
func bucketEmpty(b *authorBucket, now time.Time) bool {
	hasMuteHistory := b.MuteCount > 0 && !b.LastMuteAt.IsZero() && now.Sub(b.LastMuteAt) <= muteCountDecay
	return len(b.MessageTimes) == 0 && len(b.ViolationTimes) == 0 &&
		b.MutedUntil.IsZero() && len(b.RecentTexts) == 0 && !hasMuteHistory
}

// saveRates writes the current rate-limit/mute/duplicate state to disk.
// Must NOT be called while holding s.mu (takes its own snapshot under
// lock, then writes outside) — mirrors internal/forum/ratelimit.go's save.
func (s *Store) saveRates() {
	if s.dataDir == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	out := make(map[string]*authorBucket, len(s.authors))
	for k, b := range s.authors {
		fresh := pruneBucket(b, now)
		if !bucketEmpty(fresh, now) {
			out[k] = fresh
		}
	}
	s.mu.Unlock()
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.dataDir, "chat_rates.json"), data, 0o600)
}

func (s *Store) loadRates() {
	if s.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.dataDir, "chat_rates.json"))
	if err != nil {
		return
	}
	var stored map[string]*authorBucket
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	for k, b := range stored {
		fresh := pruneBucket(b, now)
		if !bucketEmpty(fresh, now) {
			s.authors[k] = fresh
		}
	}
	s.mu.Unlock()
}

// saveRatesLoop flushes dirty rate-limit state to disk at rlSaveInterval,
// and once more on shutdown — mirrors internal/forum/ratelimit.go's
// saveLoop.
func (s *Store) saveRatesLoop(ctx context.Context) {
	ticker := time.NewTicker(rlSaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			dirty := s.dirty
			s.dirty = false
			s.mu.Unlock()
			if dirty {
				s.saveRates()
			}
		case <-ctx.Done():
			s.mu.Lock()
			dirty := s.dirty
			s.dirty = false
			s.mu.Unlock()
			if dirty {
				s.saveRates()
			}
			return
		}
	}
}

// Run consumes incoming chat messages and periodically purges expired ones
// until ctx is done.
func (s *Store) Run(ctx context.Context) {
	go s.runCleanup(ctx)
	go s.saveRatesLoop(ctx)
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

func (s *Store) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Store) cleanup() {
	n, _ := s.db.DeleteExpiredChatMessages()
	if n == 0 {
		return
	}
	now := time.Now()
	s.mu.Lock()
	kept := s.messages[:0]
	for _, m := range s.messages {
		if m.ExpiresAt.IsZero() || m.ExpiresAt.After(now) {
			kept = append(kept, m)
		} else {
			delete(s.byID, m.ID)
		}
	}
	s.messages = kept
	s.mu.Unlock()
}

// handleMessage processes a raw GossipSub payload. Messages that don't
// parse, don't carry a valid signature, or are oversized/duplicate are
// dropped for persistence purposes — but an unsigned-but-otherwise-valid
// packet (the plugin adapter's shape) is still forwarded to NewMessages for
// live display, exactly matching pre-chatlog behavior for that source.
func (s *Store) handleMessage(data []byte) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil || m.Text == "" {
		return
	}
	if len(m.Text) > MaxTextBytes {
		return
	}
	if !m.Signed() {
		// Bilinmeyen/imzasız kaynak (ör. eklenti API'si) — kalıcılaştırma ya
		// da senkronizasyona dahil etme, ama canlı göstermeye devam et.
		s.publishLive(&m)
		return
	}
	if s.muted(m.AuthorKey) {
		// Israrlı ihlalci — imza doğrulaması dahil hiçbir işe girmeden atla.
		return
	}
	if s.authorBanned(m.AuthorKey) {
		// Bir moderatörün ağ-seviyesinde banladığı peer'ın AuthorKey'i —
		// bkz. SetModerator'ın doc yorumu.
		return
	}
	// ID tekrarı kontrolü, allow()/isDuplicate()'ten ÖNCE yapılıyor —
	// kasıtlı sıralama: libp2p'nin kendi gossip "seen" önbelleği zamanla
	// dolar (yaklaşık birkaç dakika), bundan sonra biri daha önce yakaladığı
	// GEÇERLİ İMZALI bir mesajı (özel anahtara ihtiyaç duymadan) aynen
	// tekrar yayınlayabilir. allow()/isDuplicate() bu kontrolden SONRA
	// gelseydi, her tekrar (replay) kurbanın hız-sınırı kotasını gerçekten
	// tüketir ve kopya-mesaj sayacını artırırdı — sonunda masum bir yazarı
	// kendi göndermediği bir "spam"den dolayı otomatik susturabilirdi.
	s.mu.RLock()
	_, dup := s.byID[m.ID]
	s.mu.RUnlock()
	if dup {
		return
	}
	if m.SentAt.After(time.Now().Add(maxClockSkew)) {
		return
	}
	if !verify(&m) {
		return
	}
	if !s.allow(m.AuthorKey, true) {
		s.recordViolation(m.AuthorKey)
		return
	}
	if s.isDuplicate(m.AuthorKey, m.Text) {
		// Not: kopya-mesaj reddi kasıtlı olarak recordViolation'ı
		// TETİKLEMİYOR — dupMaxRepeats zaten her tekrarı ayrı ayrı
		// engelliyor; bunu ayrıca bir "ihlal" sayıp otomatik susturma
		// eşiğine eklemek, kısa bir ifadeyi (ör. "lol", "+1") birkaç kez
		// tekrarlayan sıradan/masum bir kullanıcının tüm trafiğinin
		// susturulmasına yol açabilir. Gerçek flood/bot davranışı zaten
		// allow()'un hız sınırını (recordViolation'ı TETİKLEYEN tek yol)
		// aşar.
		return
	}
	if m.ExpiresAt.IsZero() {
		m.ExpiresAt = m.SentAt.Add(MessageTTL)
	}
	s.persist(&m)
	s.publishLive(&m)
}

func (s *Store) publishLive(m *Message) {
	select {
	case s.newMessages <- m:
	default:
	}
}

// persist stores m if it isn't already known, reporting whether it was
// newly added (false for a duplicate) — SyncWithPeer uses this to know
// whether to signal HistoryChanged.
func (s *Store) persist(m *Message) bool {
	s.mu.Lock()
	if _, dup := s.byID[m.ID]; dup {
		s.mu.Unlock()
		return false
	}
	s.messages = append(s.messages, m)
	s.byID[m.ID] = m

	// MaxStoredMessages tavanı aşıldıysa en eskilerini (dizinin başından,
	// varış sırasına göre) tahliye et — MessageTTL'den bağımsız, ikinci bir
	// büyüme sınırı.
	var evicted []string
	for len(s.messages) > MaxStoredMessages {
		old := s.messages[0]
		s.messages = s.messages[1:]
		delete(s.byID, old.ID)
		evicted = append(evicted, old.ID)
	}
	s.mu.Unlock()

	_ = s.db.InsertChatMessage(storage.RawChatMessage{
		ID: m.ID, AuthorKey: m.AuthorKey, AuthorName: m.AuthorName,
		Text: m.Text, SentAt: m.SentAt, ExpiresAt: m.ExpiresAt, Sig: m.Sig,
	})
	for _, id := range evicted {
		_ = s.db.DeleteChatMessage(id)
	}
	if len(evicted) > 0 {
		// Tahliye, aradan mesaj kaldırdığı için canlı akış üzerinden
		// (NewMessages, sıralı ekleme varsayar) değil HistoryChanged
		// üzerinden bildiriliyor — dinleyen taraf All()'u yeniden okuyup
		// tam (ve zaten doğru sıradaki) geçmişi baştan oluşturabilir.
		select {
		case s.historyChanged <- struct{}{}:
		default:
		}
	}
	return true
}

// Send signs, persists, and broadcasts a chat message from the local
// identity. Persistence happens regardless of whether topic is nil/publish
// fails, matching forum.CreatePost/profile.PublishOwn's "local state is
// authoritative first" pattern — and is necessary here specifically because
// network.Topic.Messages filters out the publisher's own messages, so
// Send's caller would otherwise never see their own message appear.
func (s *Store) Send(ctx context.Context, text string) (*Message, error) {
	if text == "" {
		return nil, fmt.Errorf("chatlog: mesaj boş olamaz")
	}
	if len(text) > MaxTextBytes {
		return nil, fmt.Errorf("chatlog: mesaj en fazla %d bayt olabilir", MaxTextBytes)
	}

	name := s.identity.Name()
	if name == "" {
		name = "anonim"
	}
	m := &Message{
		AuthorKey:  hex.EncodeToString(s.identity.PublicKey()),
		AuthorName: name,
		Text:       text,
		SentAt:     time.Now().UTC(),
	}
	m.ID = deriveID(m)
	m.ExpiresAt = m.SentAt.Add(MessageTTL)
	m.Sig = hex.EncodeToString(ed25519.Sign(s.identity.PrivateKey(), payload(m)))

	s.persist(m)

	if s.topic == nil {
		return m, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return m, fmt.Errorf("chatlog: marshal: %w", err)
	}
	if err := s.topic.Publish(ctx, data); err != nil {
		return m, fmt.Errorf("chatlog: publish: %w", err)
	}
	return m, nil
}

// All returns every currently-known message, oldest first.
func (s *Store) All() []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// ByAuthor returns authorKey's most recent messages (oldest first within
// that window), capped at limit — used by the TUI's profile view to show a
// peek at someone's recent chat activity.
func (s *Store) ByAuthor(authorKey string, limit int) []*Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Message
	for _, m := range s.messages {
		if m.AuthorKey == authorKey {
			out = append(out, m)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	cp := make([]*Message, len(out))
	copy(cp, out)
	return cp
}

// NewMessages streams messages as they arrive — both freshly-verified ones
// destined for persistence and unsigned live-only ones (see handleMessage).
// Send's own local message is NOT delivered here (network.Topic.Messages
// filters the publisher's own traffic, and Send doesn't loop it back
// separately) — callers should append their own sent message directly, the
// same pattern the TUI already used before chatlog existed.
func (s *Store) NewMessages() <-chan *Message { return s.newMessages }

// signablePayload mirrors Message's author-asserted fields, JSON-encoded
// rather than hand-joined with a separator — see profile.signablePayload
// for the exact rationale (a "|"-joined free-text field adjacent to
// another free-text field can let a boundary shift forge a different,
// still-validly-signed message). Chat only has one free-text field (Text),
// bounded by fixed-format hex/time fields that can't themselves contain
// "|", so the join was not actually exploitable here — but there's no
// reason to lean on that proof when the JSON form costs nothing extra.
type signablePayload struct {
	ID        string    `json:"id"`
	AuthorKey string    `json:"author_key"`
	Text      string    `json:"text"`
	SentAt    time.Time `json:"sent_at"`
}

func payload(m *Message) []byte {
	data, _ := json.Marshal(signablePayload{ID: m.ID, AuthorKey: m.AuthorKey, Text: m.Text, SentAt: m.SentAt})
	return data
}

func deriveID(m *Message) string {
	h := sha256.Sum256([]byte(m.AuthorKey + "|" + m.Text + "|" + m.SentAt.String()))
	return hex.EncodeToString(h[:16])
}

func verify(m *Message) bool {
	pub, err := hex.DecodeString(m.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(m.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), payload(m), sig)
}
