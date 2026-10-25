package site

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/storage"
)

type testEnv struct {
	store    *Store
	identity *stdcrypto.Identity
}

func newTestStore(t testing.TB) *testEnv {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s, err := New(id, nil, db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &testEnv{store: s, identity: id}
}

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

// signedPage builds and signs a Page. updatedAt is stripped to UTC (as
// PublishPage does) before signing — bkz. profile_test.go'nun
// signedProfile'ının aynı gerekçesi: bir JSON round-trip'i monotonic clock
// okumasını siler, imza aynı temsile karşı atılmalı.
func signedPage(priv ed25519.PrivateKey, pub ed25519.PublicKey, slug, title, body string, updatedAt time.Time) *Page {
	p := &Page{
		AuthorKey: hex.EncodeToString(pub), Slug: slug, Title: title, Body: body,
		UpdatedAt: updatedAt.UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, pagePayload(p)))
	return p
}

func signedDelete(priv ed25519.PrivateKey, pub ed25519.PublicKey, slug string, deletedAt time.Time) *DeletePage {
	d := &DeletePage{AuthorKey: hex.EncodeToString(pub), Slug: slug, DeletedAt: deletedAt.UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deletePagePayload(d)))
	return d
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// ─── ValidSlug ──────────────────────────────────────────────────────────────

func TestValidSlug(t *testing.T) {
	valid := []string{"home", "a", "about-me", "proje-2026", "x1"}
	for _, s := range valid {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "-leading", "trailing-", "UPPER", "with space", "ünlü", "a/b", "-", strRepeat("a", MaxSlugBytes+1)}
	for _, s := range invalid {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}

func strRepeat(s string, n int) string {
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s[0])
	}
	return string(out)
}

// ─── PublishPage ────────────────────────────────────────────────────────────

func TestPublishPage_StoresLocallyWithNilTopic(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "Ana Sayfa", "merhaba dünya"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	got := env.store.Get(myKey, "home")
	if got == nil || got.Body != "merhaba dünya" || got.Title != "Ana Sayfa" {
		t.Fatalf("PublishPage sonrası sayfa Get ile bulunmalıydı, got %+v", got)
	}
	if !verifyPage(got) {
		t.Fatal("PublishPage'in ürettiği sayfa kendi imzasıyla doğrulanmalı")
	}
}

func TestPublishPage_MultiplePagesCoexist(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "Ana Sayfa", "gövde 1"); err != nil {
		t.Fatalf("PublishPage(home): %v", err)
	}
	if err := env.store.PublishPage(context.Background(), "projeler", "Projelerim", "gövde 2"); err != nil {
		t.Fatalf("PublishPage(projeler): %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	pages := env.store.PagesByAuthor(myKey)
	if len(pages) != 2 {
		t.Fatalf("2 sayfa bekleniyordu, %d geldi: %+v", len(pages), pages)
	}
	if pages[0].Slug != "home" || pages[1].Slug != "projeler" {
		t.Fatalf("slug'a göre sıralı bekleniyordu, got %q, %q", pages[0].Slug, pages[1].Slug)
	}
}

func TestPublishPage_ReplacesExistingSlug(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "v1", "eski gövde"); err != nil {
		t.Fatalf("PublishPage v1: %v", err)
	}
	// persist() "en yeni kazanır" ilkesiyle çalışır (bkz. doc yorumu) —
	// UpdatedAt time.Now()'dan geldiğinden, art arda iki çağrı arasında
	// saatin GERÇEKTEN ilerlediğinden emin olmak için kısa bir bekleme
	// (Windows'ta varsayılan saat çözünürlüğü ~15ms olabilir).
	time.Sleep(20 * time.Millisecond)
	if err := env.store.PublishPage(context.Background(), "home", "v2", "yeni gövde"); err != nil {
		t.Fatalf("PublishPage v2: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	got := env.store.Get(myKey, "home")
	if got == nil || got.Title != "v2" || got.Body != "yeni gövde" {
		t.Fatalf("aynı slug'a yeniden yayın, kaydı TAMAMEN değiştirmeliydi, got %+v", got)
	}
	if len(env.store.PagesByAuthor(myKey)) != 1 {
		t.Fatalf("aynı slug'a yeniden yayın yeni bir sayfa OLUŞTURMAMALIYDI")
	}
}

func TestPublishPage_InvalidSlugRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "Not Valid!", "t", "b"); err == nil {
		t.Fatal("geçersiz bir slug reddedilmeliydi")
	}
}

func TestPublishPage_TitleTooLongRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", strRepeat("a", MaxTitleBytes+1), "b"); err == nil {
		t.Fatal("MaxTitleBytes'ı aşan bir başlık reddedilmeliydi")
	}
}

func TestPublishPage_BodyTooLongRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "t", strRepeat("a", MaxBodyBytes+1)); err == nil {
		t.Fatal("MaxBodyBytes'ı aşan bir gövde reddedilmeliydi")
	}
}

func TestPublishPage_MaxPagesPerAuthorEnforced(t *testing.T) {
	env := newTestStore(t)
	for i := 0; i < MaxPagesPerAuthor; i++ {
		slug := fmt.Sprintf("sayfa-%d", i)
		if err := env.store.PublishPage(context.Background(), slug, "t", "b"); err != nil {
			t.Fatalf("sayfa %d (kota dahilinde) reddedilmemeliydi: %v", i, err)
		}
	}
	if err := env.store.PublishPage(context.Background(), "kota-disi", "t", "b"); err == nil {
		t.Fatalf("MaxPagesPerAuthor (%d) aşan bir sayfa reddedilmeliydi", MaxPagesPerAuthor)
	}
	// Kota dolu olsa da VAROLAN bir sayfayı güncellemek serbest olmalı.
	if err := env.store.PublishPage(context.Background(), "sayfa-0", "güncel", "b"); err != nil {
		t.Fatalf("dolu kotada bile var olan bir sayfayı güncellemek serbest olmalıydı: %v", err)
	}
}

func TestPublishPage_CanceledContextSkipsLocalPersist(t *testing.T) {
	env := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := env.store.PublishPage(ctx, "home", "t", "b"); err == nil {
		t.Fatal("iptal edilmiş bir ctx ile PublishPage hata döndürmeliydi")
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	if got := env.store.Get(myKey, "home"); got != nil {
		t.Fatalf("iptal edilmiş ctx sonrası yerel olarak da hiçbir şey kaydedilmemeliydi, got %+v", got)
	}
}

func TestPublishPage_PersistsAcrossRestart(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "forum.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sA, err := New(id, nil, db)
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	if err := sA.PublishPage(context.Background(), "home", "Ana Sayfa", "kalıcı gövde"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}

	sB, err := New(id, nil, db) // "yeniden başlatma" simülasyonu: aynı db üzerinde yeni bir Store
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	myKey := hex.EncodeToString(id.PublicKey())
	got := sB.Get(myKey, "home")
	if got == nil || got.Body != "kalıcı gövde" {
		t.Fatalf("sayfa diskten yeniden yüklenmeliydi, got %+v", got)
	}
}

// ─── DeletePage ─────────────────────────────────────────────────────────────

func TestDeletePage_RemovesLocally(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "t", "b"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	if err := env.store.DeletePage(context.Background(), "home"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	if got := env.store.Get(myKey, "home"); got != nil {
		t.Fatalf("silinen sayfa artık Get ile bulunmamalıydı, got %+v", got)
	}
}

func TestDeletePage_TombstoneBlocksRepublishFromNetwork(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "t", "b"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	if err := env.store.DeletePage(context.Background(), "home"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	// Silinmiş slug'ı ağdan yeniden canlandırmaya çalışan (eski/replay) bir
	// mesaj kabul edilmemeli.
	pub, priv := genKey(t)
	myKey := hex.EncodeToString(env.identity.PublicKey())
	_ = pub
	_ = priv
	// Kendi anahtarımızla (gerçek senaryo: eski bir gossip mesajının
	// yeniden oynatılması) imzalı, silme SONRASI bir zamanlı ama düşük
	// UpdatedAt taşıyan bir "yeniden canlandırma" denemesi.
	stalePage := &Page{AuthorKey: myKey, Slug: "home", Title: "eski", Body: "eski", UpdatedAt: time.Now().Add(-time.Hour).UTC()}
	stalePage.Sig = hex.EncodeToString(ed25519.Sign(env.identity.PrivateKey(), pagePayload(stalePage)))
	env.store.handlePage(stalePage, false)

	if got := env.store.Get(myKey, "home"); got != nil {
		t.Fatalf("tombstone'lanmış bir slug ağdan yeniden canlandırılmamalıydı, got %+v", got)
	}
}

// TestPublishPage_DeletedSlugReturnsError, silinmiş (tombstone'lanmış) bir
// slug'ı yeniden yayınlamanın AÇIK bir hata döndürdüğünü doğrular — önceden
// PublishPage nil dönüyordu ama persist() tombstone yüzünden sayfayı
// sessizce atlıyordu: TUI "yayınlandı" diyor, sayfa hiçbir yerde
// saklanmıyordu.
func TestPublishPage_DeletedSlugReturnsError(t *testing.T) {
	env := newTestStore(t)
	ctx := context.Background()
	if err := env.store.PublishPage(ctx, "home", "t", "b"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	if err := env.store.DeletePage(ctx, "home"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	if err := env.store.PublishPage(ctx, "home", "yeni", "yeni gövde"); err == nil {
		t.Fatal("silinmiş bir slug'ı yeniden yayınlamak hata döndürmeliydi")
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	if got := env.store.Get(myKey, "home"); got != nil {
		t.Fatalf("silinmiş slug yeniden canlanmamalıydı, got %+v", got)
	}
	if err := env.store.PublishPage(ctx, "home-2", "yeni", "yeni gövde"); err != nil {
		t.Fatalf("farklı bir slug ile yayın etkilenmemeliydi: %v", err)
	}
}

func TestDeletePage_NonExistentSlugErrors(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.DeletePage(context.Background(), "hicbirzaman"); err == nil {
		t.Fatal("hiç var olmayan bir sayfayı silmek hata döndürmeliydi")
	}
}

func TestDeletePage_InvalidSlugErrors(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.DeletePage(context.Background(), "Not Valid!"); err == nil {
		t.Fatal("geçersiz bir slug için hata bekleniyordu")
	}
}

// ─── handlePage (ağdan gelen) ───────────────────────────────────────────────

func TestHandlePage_ValidPageStored(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "başlık", "gövde", time.Now())

	env.store.handlePage(p, false)

	got := env.store.Get(p.AuthorKey, "home")
	if got == nil || got.Body != "gövde" {
		t.Fatalf("geçerli imzalı bir sayfa kabul edilmeliydi, got %+v", got)
	}
}

func TestHandlePage_TamperedBodyRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "başlık", "orijinal gövde", time.Now())
	p.Body = "değiştirilmiş gövde — imza artık geçersiz"

	env.store.handlePage(p, false)

	if got := env.store.Get(p.AuthorKey, "home"); got != nil {
		t.Fatal("imza payload'ıyla eşleşmeyen (değiştirilmiş) bir sayfa kabul edilmemeliydi")
	}
}

func TestHandlePage_WrongAuthorKeyRejected(t *testing.T) {
	env := newTestStore(t)
	_, priv := genKey(t)     // gerçek imzalayan
	otherPub, _ := genKey(t) // iddia edilen (ama imzayı atmayan) yazar
	p := signedPage(priv, otherPub, "home", "t", "sahte", time.Now())

	env.store.handlePage(p, false)

	if got := env.store.Get(p.AuthorKey, "home"); got != nil {
		t.Fatal("AuthorKey ile imzalayan anahtar uyuşmuyorsa sayfa kabul edilmemeliydi")
	}
}

func TestHandlePage_InvalidSlugRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "Not Valid!", "t", "b", time.Now())

	env.store.handlePage(p, false)

	if got := env.store.Get(p.AuthorKey, "Not Valid!"); got != nil {
		t.Fatal("geçersiz bir slug taşıyan sayfa kabul edilmemeliydi")
	}
}

func TestHandlePage_OversizedBodyRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", strRepeat("a", MaxBodyBytes+1), time.Now())

	env.store.handlePage(p, false)

	if got := env.store.Get(p.AuthorKey, "home"); got != nil {
		t.Fatal("boyut sınırını aşan bir sayfa kabul edilmemeliydi")
	}
}

func TestHandlePage_FutureTimestampRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", "gelecekten geliyorum", time.Now().Add(time.Hour))

	env.store.handlePage(p, false)

	if got := env.store.Get(p.AuthorKey, "home"); got != nil {
		t.Fatal("maxClockSkew'i aşan gelecek tarihli bir sayfa kabul edilmemeliydi")
	}
}

func TestHandlePage_StaleUpdateRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	newer := signedPage(priv, pub, "home", "yeni", "yeni gövde", now)
	env.store.handlePage(newer, false)

	older := signedPage(priv, pub, "home", "eski", "eski (replay)", now.Add(-time.Hour))
	env.store.handlePage(older, false)

	got := env.store.Get(newer.AuthorKey, "home")
	if got == nil || got.Body != "yeni gövde" {
		t.Fatalf("daha eski bir güncelleme, zaten sahip olduğumuz daha yeni sayfayı değiştirmemeliydi, got %+v", got)
	}
}

func TestHandlePage_RapidUpdatesRateLimited(t *testing.T) {
	old := minPageEditInterval
	minPageEditInterval = time.Hour
	t.Cleanup(func() { minPageEditInterval = old })

	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedPage(priv, pub, "home", "ilk", "ilk gövde", now)
	env.store.handlePage(first, false)

	second := signedPage(priv, pub, "home", "hemen ardından", "ikinci gövde", now.Add(time.Second))
	env.store.handlePage(second, false)

	got := env.store.Get(first.AuthorKey, "home")
	if got == nil || got.Body != "ilk gövde" {
		t.Fatalf("minPageEditInterval içinde gelen ikinci güncelleme reddedilmeliydi, got %+v", got)
	}
}

func TestHandlePage_FromSyncBypassesCooldown(t *testing.T) {
	old := minPageEditInterval
	minPageEditInterval = time.Hour
	t.Cleanup(func() { minPageEditInterval = old })

	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedPage(priv, pub, "home", "ilk", "ilk gövde", now)
	env.store.handlePage(first, false)

	second := signedPage(priv, pub, "home", "sync ile geldi", "ikinci gövde", now.Add(time.Second))
	env.store.handlePage(second, true) // fromSync=true

	got := env.store.Get(first.AuthorKey, "home")
	if got == nil || got.Body != "ikinci gövde" {
		t.Fatalf("fromSync=true, minPageEditInterval'dan MUAF olmalıydı, got %+v", got)
	}
}

func TestHandlePage_PerAuthorPageCapRejectsBeyondLimit(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	var accepted int
	for i := 0; i < MaxPagesPerAuthor+2; i++ {
		slug := fmt.Sprintf("sayfa-%d", i)
		p := signedPage(priv, pub, slug, "t", "b", now.Add(time.Duration(i)*time.Second))
		env.store.handlePage(p, false)
		if env.store.Get(p.AuthorKey, slug) != nil {
			accepted++
		}
	}
	if accepted != MaxPagesPerAuthor {
		t.Fatalf("yazar başına sayfa tavanını aşan istekler reddedilmeliydi: %d kabul edildi, want %d", accepted, MaxPagesPerAuthor)
	}
}

// ─── Sybil koruması ─────────────────────────────────────────────────────────

func TestHandlePage_NewAuthorCapRejectsBeyondLimit(t *testing.T) {
	old := maxNewAuthorsPerHour
	maxNewAuthorsPerHour = 3
	t.Cleanup(func() { maxNewAuthorsPerHour = old })

	env := newTestStore(t)
	now := time.Now()

	var accepted int
	for i := 0; i < maxNewAuthorsPerHour+2; i++ {
		pub, priv := genKey(t)
		p := signedPage(priv, pub, "home", "t", fmt.Sprintf("yazar %d", i), now)
		env.store.handlePage(p, false)
		if env.store.Get(p.AuthorKey, "home") != nil {
			accepted++
		}
	}
	if accepted != maxNewAuthorsPerHour {
		t.Fatalf("saatlik yeni-yazar sınırını aşan istekler reddedilmeliydi: %d kabul edildi, want %d", accepted, maxNewAuthorsPerHour)
	}
}

func TestHandlePage_NewAuthorCapDoesNotBlockKnownAuthors(t *testing.T) {
	oldLimit := maxNewAuthorsPerHour
	maxNewAuthorsPerHour = 1
	t.Cleanup(func() { maxNewAuthorsPerHour = oldLimit })
	oldInterval := minPageEditInterval
	minPageEditInterval = 0
	t.Cleanup(func() { minPageEditInterval = oldInterval })

	env := newTestStore(t)
	now := time.Now()

	pub, priv := genKey(t)
	first := signedPage(priv, pub, "home", "ilk", "ilk", now)
	env.store.handlePage(first, false)
	if env.store.Get(first.AuthorKey, "home") == nil {
		t.Fatal("kota dahilindeki ilk yeni yazar kabul edilmeliydi")
	}

	// Kota dolu (1/1) ama AYNI (bilinen) yazardan YENİ bir sayfa (farklı
	// slug), "yeni yazar" sayılmamalı — yalnızca yeni bir slug ekliyor,
	// yeni bir yazar eklemiyor.
	second := signedPage(priv, pub, "ikinci-sayfa", "t", "b", now.Add(time.Minute))
	env.store.handlePage(second, false)
	if got := env.store.Get(second.AuthorKey, "ikinci-sayfa"); got == nil {
		t.Fatalf("dolu yeni-yazar kotası, zaten bilinen bir yazarın YENİ bir sayfasını engellememeliydi")
	}
}

func TestHandlePage_MaxStoredAuthorsCapRejectsBeyondCeiling(t *testing.T) {
	oldCap := maxStoredAuthors
	oldLimit := maxNewAuthorsPerHour
	maxStoredAuthors = 2
	maxNewAuthorsPerHour = 1000
	t.Cleanup(func() {
		maxStoredAuthors = oldCap
		maxNewAuthorsPerHour = oldLimit
	})

	env := newTestStore(t)
	now := time.Now()

	var accepted int
	for i := 0; i < maxStoredAuthors+2; i++ {
		pub, priv := genKey(t)
		p := signedPage(priv, pub, "home", "t", fmt.Sprintf("yazar %d", i), now)
		env.store.handlePage(p, false)
		if env.store.Get(p.AuthorKey, "home") != nil {
			accepted++
		}
	}
	if accepted != maxStoredAuthors {
		t.Fatalf("toplam yazar tavanını aşan istekler reddedilmeliydi: %d kabul edildi, want %d", accepted, maxStoredAuthors)
	}
}

// ─── handleDelete (ağdan gelen) ─────────────────────────────────────────────

func TestHandleDelete_ValidDeleteRemovesPage(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", "b", time.Now())
	env.store.handlePage(p, false)

	d := signedDelete(priv, pub, "home", time.Now())
	env.store.handleWireMessage(mustJSON(t, envelope{Type: "delete", Delete: d}))

	if got := env.store.Get(p.AuthorKey, "home"); got != nil {
		t.Fatalf("geçerli imzalı bir silme sonrası sayfa artık bulunmamalıydı, got %+v", got)
	}
}

func TestHandleDelete_WrongAuthorRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", "b", time.Now())
	env.store.handlePage(p, false)

	_, attackerPriv := genKey(t) // farklı bir özel anahtar
	forged := &DeletePage{AuthorKey: p.AuthorKey, Slug: "home", DeletedAt: time.Now().UTC()}
	forged.Sig = hex.EncodeToString(ed25519.Sign(attackerPriv, deletePagePayload(forged)))

	env.store.handleWireMessage(mustJSON(t, envelope{Type: "delete", Delete: forged}))

	if got := env.store.Get(p.AuthorKey, "home"); got == nil {
		t.Fatal("başka bir anahtarla imzalanmış (sahte) bir silme isteği kabul edilmemeliydi")
	}
}

// ─── handleWireMessage: bozuk veriye tolerans ───────────────────────────────

func TestHandleWireMessage_MalformedJSONNoPanic(t *testing.T) {
	env := newTestStore(t)
	env.store.handleWireMessage([]byte("bu geçerli bir json değil"))
	env.store.handleWireMessage([]byte(`{"type":"page"}`)) // Page nil
	env.store.handleWireMessage([]byte(`{"type":"bilinmeyen"}`))
}

// ─── PagesByAuthor / Get: kopya döndürme ────────────────────────────────────

func TestGet_ReturnsCopyNotLivePointer(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishPage(context.Background(), "home", "orijinal", "gövde"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())

	got := env.store.Get(myKey, "home")
	got.Title = "mutasyona uğratıldı"

	fresh := env.store.Get(myKey, "home")
	if fresh.Title != "orijinal" {
		t.Errorf("Get() sonucunu mutasyona uğratmak store'u etkilememeli, Title = %q", fresh.Title)
	}
}

// ─── pagePayload: alan sınırı belirsizliği regresyonu ───────────────────────

// TestPagePayload_FieldBoundaryShiftInvalidatesSignature, profile_test.go'nun
// TestProfilePayload_FieldBoundaryShiftInvalidatesSignature'ının site
// paketindeki eşdeğeri: Title/Body serbest metindir, JSON tabanlı
// pageSignable bu ikisi arasında bir alan sınırı kaydırmasına izin
// vermemeli.
func TestPagePayload_FieldBoundaryShiftInvalidatesSignature(t *testing.T) {
	pub, priv := genKey(t)
	now := time.Now().UTC()

	orig := &Page{
		AuthorKey: hex.EncodeToString(pub), Slug: "home",
		Title: "Blockchain dev | building AND", Body: "scam link: notreal.com",
		UpdatedAt: now,
	}
	orig.Sig = hex.EncodeToString(ed25519.Sign(priv, pagePayload(orig)))
	if !verifyPage(orig) {
		t.Fatal("orijinal mesaj kendi imzasıyla doğrulanmalı")
	}

	forged := &Page{
		AuthorKey: orig.AuthorKey, Slug: "home",
		Title: "Blockchain dev ", Body: " building AND|scam link: notreal.com",
		UpdatedAt: now, Sig: orig.Sig,
	}
	if verifyPage(forged) {
		t.Fatal("alan sınırı kaydırılmış (sahte) bir Title/Body ikilisi, orijinalin imzasıyla doğrulanmamalı")
	}
}

// ─── persist(): newest-wins yarış koruması ──────────────────────────────────

func TestPersist_OlderUpdateDoesNotOverwriteNewer(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now().UTC()

	newer := signedPage(priv, pub, "home", "yeni", "yeni", now)
	older := signedPage(priv, pub, "home", "eski", "eski ama sonra persist edildi", now.Add(-time.Minute))

	env.store.persist(newer)
	env.store.persist(older)

	got := env.store.Get(newer.AuthorKey, "home")
	if got == nil || got.Body != "yeni" {
		t.Fatalf("daha eski bir UpdatedAt'e sahip persist() çağrısı, daha yeniyi ezmemeliydi, got %+v", got)
	}
}
