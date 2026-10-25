// Package site, kullanıcıların forumdaki kimliklerine bağlı, ÇOK SAYFALI bir
// "kişisel site" yayınlamasını sağlar: her yazar, birbirinden bağımsız
// slug'larla (ör. "home", "projeler") adreslenen, Ed25519 imzalı, GossipSub
// üzerinden yayılan düz metin sayfalar yayınlayabilir. internal/forum'un
// post/reply desenini izler (çok satırlı, kimlik ile adreslenen, tombstone
// ile silinen içerik) — internal/profile'ın tek-satır, tüm-kaydı-değiştiren
// Upsert modelinden farklı olarak, burada yazar başına BİRDEN FAZLA bağımsız
// sayfa vardır.
//
// profile paketinin aksine (bkz. onun paket yorumundaki "ayrı bir
// senkronizasyon mekanizması gerektirmez" notu — ki bu, çevrimdışıyken
// yayınlanan bir profilin asla geriye dönük yakalanamadığı anlamına gelir),
// site sayfaları forum'un /and/sync/1.0.0'ı gibi kendi senkronizasyon
// protokolüne (bkz. sync.go) sahiptir — aksi halde bir sayfa yayınlandığında
// o an ağda olmayan bir eş, o sayfayı asla göremezdi.
//
// Sayfa gövdesi Markdown OLARAK RENDER EDİLMEZ (AND'ın TUI'sinde bir
// Markdown motoru yok) — forum post gövdeleri gibi düz metin olarak
// saklanır/gösterilir; kullanıcı isterse Markdown benzeri biçimlendirme
// yazabilir ama bu yalnızca düz metin olarak görünür.
package site

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/guvenli"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/storage"
	"github.com/lucian95511/and/internal/timewin"
	"github.com/lucian95511/and/protokol"
)

// SiteTopic is the GossipSub topic page publish/delete messages travel on.
const SiteTopic = protokol.SiteKonusu

// Alan boyut/sayı sınırları — forum'un title/body limitleriyle aynı ruhta.
// Exported so the TUI can align input CharLimits/textarea limits and live
// counters with the values actually enforced here, instead of duplicating
// magic numbers (aynı desen: profile.MaxBioBytes vb.).
const (
	MaxSlugBytes      = 40
	MaxTitleBytes     = 100
	MaxBodyBytes      = 8000
	MaxPagesPerAuthor = 20
)

// reSlug: küçük harf, rakam, tire — baş/son karakter tire olamaz. URL/dosya
// yolu güvenli bir tanımlayıcı (forum kategorileriyle karışmasın diye ayrı
// bir ad alanı) ve gösterimde/loglarda kontrol karakteri sorunu yaratmaz.
var reSlug = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,` + fmt.Sprint(MaxSlugBytes-2) + `}[a-z0-9])?$`)

// ValidSlug reports whether s is an acceptable page identifier.
func ValidSlug(s string) bool { return reSlug.MatchString(s) }

// minPageEditInterval, AYNI (yazar,slug) çifti için canlı gossip'ten kabul
// edilen ardışık güncellemeler arasındaki en kısa aralıktır — profile'ın
// minRepublishInterval'ıyla aynı ruhta, dedike bir hız sınırlayıcı paketi
// olmadan tek bir düşük-frekanslı mesaj türünü hafifçe sınırlamanın basit
// yolu. Sync ile gelen (fromSync=true) toplu geçmiş bu sınırdan MUAFTIR —
// bkz. handlePage'in fromSync parametresi.
var minPageEditInterval = 5 * time.Second

// maxClockSkew, bir UpdatedAt'in ne kadar ileri tarihli olabileceğini
// sınırlar — profile.maxClockSkew ile aynı gerekçe: gelecek tarihli TEK bir
// mesaj, o (yazar,slug) için "en yeni" olarak önbelleğe alınırsa, gerçek
// sahibinin sonraki dürüst zamanlı güncellemelerini kalıcı olarak eskitip
// sessizce reddettirebilir.
const maxClockSkew = 2 * time.Minute

// Sybil koruması: profile.go'daki eşdeğerinin BİREBİR aynı gerekçesi (bkz.
// oradaki paket yorumu) — ed25519 anahtar çifti üretmek neredeyse bedava
// olduğundan, bir saldırganın sınırsız "yeni yazar" ekleyerek deposunu
// sınırsız büyütmesini önler. maxPagesPerAuthorCap (const, MaxPagesPerAuthor
// ile aynı değer) buna ek olarak BİLİNEN bir yazarın da sınırsız sayfa
// biriktirmesini engeller — profile'da karşılığı yok çünkü orada yazar
// başına zaten tek bir satır var.
const newAuthorWindow = time.Hour

var (
	maxNewAuthorsPerHour = 200
	maxStoredAuthors     = 5000
)

// Page is one page of an author's personal site.
type Page struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
	Sig       string    `json:"sig"`
}

// DeletePage is a signed request to remove one of the author's own pages —
// only the holder of AuthorKey's private key can produce a valid Sig for
// it, so nobody else can delete another author's page.
type DeletePage struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	DeletedAt time.Time `json:"deleted_at"`
	Sig       string    `json:"sig"`
}

// envelope carries either a Page publish or a DeletePage over the single
// SiteTopic — mirrors forum.envelope's Type-tagged union shape.
type envelope struct {
	Type   string      `json:"type"`
	Page   *Page       `json:"page,omitempty"`
	Delete *DeletePage `json:"delete,omitempty"`
}

// Store holds every page this peer currently knows about, keyed by author
// then slug.
type Store struct {
	mu    sync.RWMutex
	pages map[string]map[string]*Page // authorKey -> slug -> Page
	// deleted holds the actual signed tombstone (not just a bool) so it can
	// be re-offered to a peer during sync (bkz. sync.go'nun handleSyncStream'i)
	// — bir peer'a "bu silindi" demek için gerçek imzalı mesaja ihtiyaç var.
	deleted map[string]*DeletePage // authorKey+"/"+slug -> tombstone

	lastEdit       map[string]time.Time // authorKey+"/"+slug -> son kabul zamanı (bkz. minPageEditInterval)
	newAuthorTimes []time.Time          // Sybil koruması (bkz. handlePage)

	identity *stdcrypto.Identity
	topic    *network.Topic
	db       *storage.DB
}

// New loads any pages already persisted in db and returns a Store ready to
// publish/receive updates over topic. topic may be nil (tests, or a peer
// that failed to join the pubsub topic) — Run() then becomes a no-op, and
// PublishPage/DeletePage stop after the local persist step (no network
// broadcast) — same nil-topic contract as profile.New.
func New(id *stdcrypto.Identity, topic *network.Topic, db *storage.DB) (*Store, error) {
	s := &Store{
		pages:    make(map[string]map[string]*Page),
		deleted:  make(map[string]*DeletePage),
		lastEdit: make(map[string]time.Time),
		identity: id,
		topic:    topic,
		db:       db,
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("site: load: %w", err)
	}
	return s, nil
}

func (s *Store) load() error {
	raws, err := s.db.AllSitePages()
	if err != nil {
		return err
	}
	for _, rp := range raws {
		p := &Page{
			AuthorKey: rp.AuthorKey, Slug: rp.Slug, Title: rp.Title,
			Body: rp.Body, UpdatedAt: rp.UpdatedAt, Sig: rp.Sig,
		}
		if s.pages[p.AuthorKey] == nil {
			s.pages[p.AuthorKey] = make(map[string]*Page)
		}
		s.pages[p.AuthorKey][p.Slug] = p
	}

	tombstoneJSON, err := s.db.AllSiteTombstoneJSON()
	if err != nil {
		return err
	}
	for _, tj := range tombstoneJSON {
		var d DeletePage
		// profile.load'un bozuk mod_cert JSON'una toleranslı davranışıyla aynı
		// ruhta: bozuk TEK bir tombstone kaydı, geri kalan her şeyin
		// yüklenmesini engellememeli.
		if json.Unmarshal([]byte(tj), &d) == nil && d.AuthorKey != "" && d.Slug != "" {
			s.deleted[d.AuthorKey+"/"+d.Slug] = &d
		}
	}
	return nil
}

// Run consumes incoming page/delete messages until ctx is done.
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
			guvenli.Isle("site iletisi", func() { s.handleWireMessage(data) })
		case <-ctx.Done():
			return
		}
	}
}

func (s *Store) handleWireMessage(data []byte) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case "page":
		s.handlePage(env.Page, false)
	case "delete":
		s.handleDelete(env.Delete)
	}
}

// handlePage is the full validation+admission gate for a page arriving
// from the network (live gossip when fromSync=false, sync backfill when
// fromSync=true). fromSync bypasses minPageEditInterval (a burst of
// legitimately-old sync results shouldn't be throttled the way live spam
// is — mirrors forum sync.go's allowPost(..., checkCooldown=false)) but
// still goes through the Sybil/per-author-count caps.
func (s *Store) handlePage(p *Page, fromSync bool) {
	if p == nil || p.AuthorKey == "" || !ValidSlug(p.Slug) ||
		len(p.Title) > MaxTitleBytes || len(p.Body) > MaxBodyBytes {
		return
	}
	if p.UpdatedAt.After(time.Now().Add(maxClockSkew)) {
		return
	}
	if !verifyPage(p) {
		return
	}

	key := p.AuthorKey + "/" + p.Slug

	s.mu.Lock()
	if _, tombstoned := s.deleted[key]; tombstoned {
		s.mu.Unlock()
		return // tombstone'lanmış bir slug'ı yeniden canlandırma
	}
	authorPages, known := s.pages[p.AuthorKey]
	if known {
		if existing, ok := authorPages[p.Slug]; ok && !p.UpdatedAt.After(existing.UpdatedAt) {
			s.mu.Unlock()
			return // eski/tekrar (replay) — en yeni kazanır
		}
	}
	if !fromSync {
		if last, ok := s.lastEdit[key]; ok && time.Since(last) < minPageEditInterval {
			s.mu.Unlock()
			return
		}
	}
	if known {
		if _, exists := authorPages[p.Slug]; !exists && len(authorPages) >= MaxPagesPerAuthor {
			s.mu.Unlock()
			return
		}
	} else {
		now := time.Now()
		s.newAuthorTimes = keepAfter(s.newAuthorTimes, now.Add(-newAuthorWindow))
		if len(s.newAuthorTimes) >= maxNewAuthorsPerHour {
			s.mu.Unlock()
			return
		}
		if len(s.pages) >= maxStoredAuthors {
			s.mu.Unlock()
			return
		}
		s.newAuthorTimes = append(s.newAuthorTimes, now)
	}
	s.lastEdit[key] = time.Now()
	s.mu.Unlock()

	s.persist(p)
}

// persist is the single authoritative, lock-atomic "newest wins" writer —
// shared by handlePage (after its admission gate passes) AND by
// PublishPage's local write, exactly mirroring profile.Store's split
// between handleMessage's caps and persist()'s unconditional local path:
// your OWN edits must never be rejected by the Sybil/rate caps meant to
// blunt a flooding stranger.
func (s *Store) persist(p *Page) {
	key := p.AuthorKey + "/" + p.Slug
	s.mu.Lock()
	if _, tombstoned := s.deleted[key]; tombstoned {
		s.mu.Unlock()
		return
	}
	if existing, ok := s.pages[p.AuthorKey][p.Slug]; ok && !p.UpdatedAt.After(existing.UpdatedAt) {
		s.mu.Unlock()
		return
	}
	if s.pages[p.AuthorKey] == nil {
		s.pages[p.AuthorKey] = make(map[string]*Page)
	}
	s.pages[p.AuthorKey][p.Slug] = p
	s.mu.Unlock()

	_ = s.db.UpsertSitePage(storage.RawSitePage{
		AuthorKey: p.AuthorKey, Slug: p.Slug, Title: p.Title, Body: p.Body,
		UpdatedAt: p.UpdatedAt, Sig: p.Sig,
	})
}

func (s *Store) handleDelete(d *DeletePage) {
	if d == nil || d.AuthorKey == "" || !ValidSlug(d.Slug) {
		return
	}
	if !verifyDeletePage(d) {
		return
	}
	s.applyDelete(d)
}

// applyDelete is shared by handleDelete (network-received, already
// verified) and DeletePage's local write.
func (s *Store) applyDelete(d *DeletePage) {
	key := d.AuthorKey + "/" + d.Slug

	s.mu.Lock()
	if existing, ok := s.deleted[key]; ok && !d.DeletedAt.After(existing.DeletedAt) {
		s.mu.Unlock()
		return // zaten bilinen (ya da daha yeni) bir tombstone — replay
	}
	s.deleted[key] = d
	if authorPages, ok := s.pages[d.AuthorKey]; ok {
		delete(authorPages, d.Slug)
		if len(authorPages) == 0 {
			delete(s.pages, d.AuthorKey)
		}
	}
	s.mu.Unlock()

	data, err := json.Marshal(d)
	if err != nil {
		return
	}
	_ = s.db.DeleteSitePageWithTombstone(d.AuthorKey, d.Slug, string(data))
}

// PublishPage signs, persists, and broadcasts a page under the local
// identity's own author key — creating it if slug is new, replacing it
// (whole-body) if slug already exists. Mirrors profile.PublishOwn: no
// partial-update semantics, every publish replaces the whole page.
func (s *Store) PublishPage(ctx context.Context, slug, title, body string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("site: geçersiz slug (yalnızca küçük harf, rakam, tire; en fazla %d karakter)", MaxSlugBytes)
	}
	if len(title) > MaxTitleBytes {
		return fmt.Errorf("site: başlık en fazla %d bayt olabilir", MaxTitleBytes)
	}
	if len(body) > MaxBodyBytes {
		return fmt.Errorf("site: sayfa en fazla %d bayt olabilir", MaxBodyBytes)
	}

	authorKey := hex.EncodeToString(s.identity.PublicKey())

	s.mu.RLock()
	authorPages := s.pages[authorKey]
	_, editingExisting := authorPages[slug]
	pageCount := len(authorPages)
	_, tombstoned := s.deleted[authorKey+"/"+slug]
	s.mu.RUnlock()
	// Silinmiş bir slug kalıcı olarak tombstone'lanmıştır: persist() onu
	// sessizce atlar, eşler de (handlePage) reddeder. Burada açıkça hata
	// dönülmezse çağıran (TUI, VS Code) "yayınlandı" der ama sayfa hiçbir
	// yerde — kendi düğümümüzde bile — saklanmazdı.
	if tombstoned {
		return fmt.Errorf("site: %q daha önce silindi — silinen bir slug yeniden kullanılamaz, farklı bir slug seç", slug)
	}
	if !editingExisting && pageCount >= MaxPagesPerAuthor {
		return fmt.Errorf("site: en fazla %d sayfan olabilir", MaxPagesPerAuthor)
	}

	p := &Page{
		AuthorKey: authorKey, Slug: slug, Title: title, Body: body,
		UpdatedAt: time.Now().UTC(),
	}
	sig := ed25519.Sign(s.identity.PrivateKey(), pagePayload(p))
	p.Sig = hex.EncodeToString(sig)

	// persist() ctx'e bakmaz (eşzamanlı yerel bir yazma, ağ çağrısı değil) —
	// bu kontrol olmasa, ctx iptali yalnızca aşağıdaki ağ Publish'ini
	// durdurur, yerel kaydı DURDURMAZDI (bkz. profile.PublishOwn'daki aynı
	// yorumun gerekçesi).
	if err := ctx.Err(); err != nil {
		return err
	}
	s.persist(p)

	if s.topic == nil {
		return nil
	}
	data, err := json.Marshal(envelope{Type: "page", Page: p})
	if err != nil {
		return fmt.Errorf("site: marshal: %w", err)
	}
	return s.topic.Publish(ctx, data)
}

// DeletePage signs, applies, and broadcasts a tombstone for the local
// identity's own page. Deleting a slug that was never published (or that's
// already deleted) is a no-op error, not a panic/silent success.
func (s *Store) DeletePage(ctx context.Context, slug string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("site: geçersiz slug")
	}
	authorKey := hex.EncodeToString(s.identity.PublicKey())

	s.mu.RLock()
	_, exists := s.pages[authorKey][slug]
	s.mu.RUnlock()
	if !exists {
		return fmt.Errorf("site: %q adlı bir sayfan yok", slug)
	}

	d := &DeletePage{AuthorKey: authorKey, Slug: slug, DeletedAt: time.Now().UTC()}
	sig := ed25519.Sign(s.identity.PrivateKey(), deletePagePayload(d))
	d.Sig = hex.EncodeToString(sig)

	if err := ctx.Err(); err != nil {
		return err
	}
	s.applyDelete(d)

	if s.topic == nil {
		return nil
	}
	data, err := json.Marshal(envelope{Type: "delete", Delete: d})
	if err != nil {
		return fmt.Errorf("site: marshal: %w", err)
	}
	return s.topic.Publish(ctx, data)
}

// Get returns a copy of authorKey's slug page, or nil if unknown — the
// returned value is always safe to mutate (same "never leak a live
// pointer" contract as profile.Store.Get).
func (s *Store) Get(authorKey, slug string) *Page {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.pages[authorKey][slug]
	if !ok {
		return nil
	}
	cp := *p
	return &cp
}

// PagesByAuthor returns authorKey's pages sorted by slug, each a safe copy.
func (s *Store) PagesByAuthor(authorKey string) []*Page {
	s.mu.RLock()
	defer s.mu.RUnlock()
	authorPages := s.pages[authorKey]
	out := make([]*Page, 0, len(authorPages))
	for _, p := range authorPages {
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// keepAfter drops the leading run of timestamps at or before cutoff from a
// slice assumed sorted ascending — mirrors profile.go's/forum ratelimit.go's
// helper of the same name/shape (duplicated rather than shared across
// packages for something this small — same call profile.go made).
var keepAfter = timewin.KeepAfter

// pagePayload/deletePagePayload mirror profile.profilePayload's rationale
// for JSON (not "|"-joined) signable payloads verbatim: Title/Body are free
// text with only a byte-length cap, so a hand-joined string would let
// anyone shift a field boundary at a literal separator inside Body and
// still pass signature verification against the original signer's bytes.
// JSON's per-field escaping has no such ambiguous boundary.
type pageSignable struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

func pagePayload(p *Page) []byte {
	sp := pageSignable{AuthorKey: p.AuthorKey, Slug: p.Slug, Title: p.Title, Body: p.Body, UpdatedAt: p.UpdatedAt}
	data, _ := json.Marshal(sp)
	return data
}

func verifyPage(p *Page) bool {
	pub, err := hex.DecodeString(p.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(p.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), pagePayload(p), sig)
}

type deletePageSignable struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	DeletedAt time.Time `json:"deleted_at"`
}

func deletePagePayload(d *DeletePage) []byte {
	sp := deletePageSignable{AuthorKey: d.AuthorKey, Slug: d.Slug, DeletedAt: d.DeletedAt}
	data, _ := json.Marshal(sp)
	return data
}

func verifyDeletePage(d *DeletePage) bool {
	pub, err := hex.DecodeString(d.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(d.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), deletePagePayload(d), sig)
}
