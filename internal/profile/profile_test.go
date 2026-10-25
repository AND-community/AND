package profile

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/storage"
)

// testEnv bundles everything a test needs: a ready Store, the identity it
// was built from, the founder keypair moderation.FounderPubKeyHex was
// pointed at (needed to mint ModeratorCert fixtures), and the Moderator's
// dataDir (needed to seed bans/mod_*.json cert files for PublishOwn's
// auto-attach path).
type testEnv struct {
	store       *Store
	identity    *stdcrypto.Identity
	founderPriv ed25519.PrivateKey
	dataDir     string
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

	founderPub, founderPriv := genKey(t)
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })

	dataDir := t.TempDir()
	mod, err := moderation.New(dataDir, peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	s, err := New(id, nil, db, mod)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &testEnv{store: s, identity: id, founderPriv: founderPriv, dataDir: dataDir}
}

func genKey(t testing.TB) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

// signedProfile builds and signs a Profile. updatedAt is stripped to UTC
// (as PublishOwn does) before signing — a raw time.Now() carries a
// monotonic clock reading that time.Time.String() renders as a trailing
// "m=+..." suffix, which a JSON marshal/unmarshal round-trip silently
// strips; sign against the same representation the wire format survives,
// or verification fails after every handleMessage(mustJSON(...)) call.
func signedProfile(priv ed25519.PrivateKey, pub ed25519.PublicKey, bio string, updatedAt time.Time) *Profile {
	p := &Profile{
		AuthorKey: hex.EncodeToString(pub),
		Bio:       bio,
		UpdatedAt: updatedAt.UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(p)))
	return p
}

// signedFullProfile is signedProfile plus Status/Tags/Link.
func signedFullProfile(priv ed25519.PrivateKey, pub ed25519.PublicKey, bio, status string, tags []string, link string, updatedAt time.Time) *Profile {
	p := &Profile{
		AuthorKey: hex.EncodeToString(pub),
		Bio:       bio,
		Status:    status,
		Tags:      tags,
		Link:      link,
		UpdatedAt: updatedAt.UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(p)))
	return p
}

// issueCert mints a moderation.ModeratorCert signed by founderPriv, mirroring
// the payload format moderation.verifyCert expects.
func issueCert(founderPriv ed25519.PrivateKey, modKey string, permanent bool, expiresAt time.Time) moderation.ModeratorCert {
	return moderation.SignModeratorCert(founderPriv, modKey, expiresAt, permanent)
}

// writeCertFile seeds dataDir/bans/mod_<short>.json the way moderation.FindModCert
// (and thus Moderator.FindOwnCert) expects to find a moderator's own cert.
func writeCertFile(t *testing.T, dataDir, ownerPubHex string, cert moderation.ModeratorCert) {
	t.Helper()
	dir := filepath.Join(dataDir, "bans")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(cert, "", "  ")
	if err != nil {
		t.Fatalf("Marshal cert: %v", err)
	}
	short := ownerPubHex
	if len(short) > 8 {
		short = short[:8]
	}
	if err := os.WriteFile(filepath.Join(dir, "mod_"+short+".json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func mustJSON(t *testing.T, p *Profile) []byte {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// ─── PublishOwn ────────────────────────────────────────────────────────────────

func TestPublishOwn_StoresLocallyWithNilTopic(t *testing.T) {
	env := newTestStore(t)

	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Bio: "merhaba dünya"}); err != nil {
		t.Fatalf("PublishOwn (nil topic): %v", err)
	}

	myKey := hex.EncodeToString(env.identity.PublicKey())
	got := env.store.Get(myKey)
	if got == nil {
		t.Fatal("PublishOwn sonrası kendi profilimiz Get ile bulunmalıydı")
	}
	if got.Bio != "merhaba dünya" {
		t.Errorf("Bio = %q, want %q", got.Bio, "merhaba dünya")
	}
	if !verifyProfile(got) {
		t.Fatal("PublishOwn'ın ürettiği profil kendi imzasıyla doğrulanmalı")
	}
}

func TestPublishOwn_AllFieldsRoundTrip(t *testing.T) {
	env := newTestStore(t)
	u := OwnUpdate{
		Bio: "bio metni", Status: "kod yazıyorum",
		Tags: []string{"Go", "Rust"}, Link: "https://example.com",
	}
	if err := env.store.PublishOwn(context.Background(), u); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}

	myKey := hex.EncodeToString(env.identity.PublicKey())
	got := env.store.Get(myKey)
	if got == nil {
		t.Fatal("profil bulunamadı")
	}
	if got.Status != u.Status || got.Link != u.Link {
		t.Errorf("Status/Link = %q/%q, want %q/%q", got.Status, got.Link, u.Status, u.Link)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "Go" || got.Tags[1] != "Rust" {
		t.Errorf("Tags = %v, want [Go Rust]", got.Tags)
	}
}

func TestPublishOwn_BioTooLongRejected(t *testing.T) {
	env := newTestStore(t)
	tooLong := make([]byte, MaxBioBytes+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Bio: string(tooLong)}); err == nil {
		t.Fatal("MaxBioBytes'ı aşan bir bio reddedilmeliydi")
	}
}

func TestPublishOwn_StatusTooLongRejected(t *testing.T) {
	env := newTestStore(t)
	tooLong := make([]byte, MaxStatusBytes+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Status: string(tooLong)}); err == nil {
		t.Fatal("MaxStatusBytes'ı aşan bir durum mesajı reddedilmeliydi")
	}
}

func TestPublishOwn_LinkTooLongRejected(t *testing.T) {
	env := newTestStore(t)
	tooLong := make([]byte, MaxLinkBytes+1)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Link: string(tooLong)}); err == nil {
		t.Fatal("MaxLinkBytes'ı aşan bir link reddedilmeliydi")
	}
}

func TestPublishOwn_TooManyTagsRejected(t *testing.T) {
	env := newTestStore(t)
	tags := forum.Categories[:MaxTags+1]
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Tags: tags}); err == nil {
		t.Fatalf("MaxTags (%d) aşan bir etiket listesi reddedilmeliydi", MaxTags)
	}
}

func TestPublishOwn_GitHubUserRoundTrip(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{GitHubUser: "octocat"}); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())
	got := env.store.Get(myKey)
	if got == nil || got.GitHubUser != "octocat" {
		t.Fatalf("GitHubUser korunmamış, got %+v", got)
	}
}

func TestPublishOwn_InvalidGitHubUserRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{GitHubUser: "-invalid-"}); err == nil {
		t.Fatal("GitHub kullanıcı adı biçimine uymayan bir değer reddedilmeliydi")
	}
}

func TestPublishOwn_InvalidTagRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Tags: []string{"Bilinmeyen Kategori"}}); err == nil {
		t.Fatal("forum.Categories'te olmayan bir etiket reddedilmeliydi")
	}
}

func TestPublishOwn_DuplicateTagRejected(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Tags: []string{"Go", "Go"}}); err == nil {
		t.Fatal("tekrarlanan bir etiket reddedilmeliydi")
	}
}

func TestPublishOwn_AttachesOwnValidModCert(t *testing.T) {
	env := newTestStore(t)
	myKey := hex.EncodeToString(env.identity.PublicKey())
	cert := issueCert(env.founderPriv, myKey, true, time.Time{})
	writeCertFile(t, env.dataDir, myKey, cert)

	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Bio: "moderatörüm"}); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}

	got := env.store.Get(myKey)
	if got == nil || got.ModCert == nil {
		t.Fatalf("geçerli kendi sertifikamız varken ModCert iliştirilmeliydi, got %+v", got)
	}
	if !env.store.IsModerator(myKey) {
		t.Fatal("iliştirilen sertifika IsModerator=true vermeliydi")
	}
}

func TestPublishOwn_PersistsAcrossRestart(t *testing.T) {
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

	founderPub, _ := genKey(t)
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })
	mod, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	sA, err := New(id, nil, db, mod)
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	if err := sA.PublishOwn(context.Background(), OwnUpdate{Bio: "kalıcı bio", Tags: []string{"Go"}}); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}

	// "Yeniden başlatma" simülasyonu: aynı db üzerinde yeni bir Store.
	sB, err := New(id, nil, db, mod)
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	myKey := hex.EncodeToString(id.PublicKey())
	got := sB.Get(myKey)
	if got == nil || got.Bio != "kalıcı bio" {
		t.Fatalf("profil diskten yeniden yüklenmeliydi, got %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "Go" {
		t.Fatalf("Tags diskten yeniden yüklenmeliydi, got %v", got.Tags)
	}
}

// ─── Get() safety ─────────────────────────────────────────────────────────────

func TestGet_ReturnsCopyNotLivePointer(t *testing.T) {
	env := newTestStore(t)
	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Bio: "orijinal", Tags: []string{"Go"}}); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}
	myKey := hex.EncodeToString(env.identity.PublicKey())

	got := env.store.Get(myKey)
	got.Bio = "mutasyona uğratıldı"
	got.Tags[0] = "Rust"

	fresh := env.store.Get(myKey)
	if fresh.Bio != "orijinal" {
		t.Errorf("Get() sonucunu mutasyona uğratmak store'u etkilememeli, Bio = %q", fresh.Bio)
	}
	if fresh.Tags[0] != "Go" {
		t.Errorf("Get() sonucunun Tags dilimini mutasyona uğratmak store'u etkilememeli, Tags = %v", fresh.Tags)
	}
}

// ─── handleMessage ──────────────────────────────────────────────────────────────

func TestHandleMessage_ValidUpdateStored(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedProfile(priv, pub, "yeni bio", time.Now())

	env.store.handleMessage(mustJSON(t, p))

	got := env.store.Get(p.AuthorKey)
	if got == nil || got.Bio != "yeni bio" {
		t.Fatalf("geçerli imzalı bir güncelleme kabul edilmeliydi, got %+v", got)
	}
}

func TestHandleMessage_ValidFullUpdateStored(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedFullProfile(priv, pub, "bio", "durum", []string{"Go", "Rust"}, "https://example.com", time.Now())

	env.store.handleMessage(mustJSON(t, p))

	got := env.store.Get(p.AuthorKey)
	if got == nil || got.Status != "durum" || got.Link != "https://example.com" || len(got.Tags) != 2 {
		t.Fatalf("tüm alanlarıyla geçerli bir güncelleme kabul edilmeliydi, got %+v", got)
	}
}

func TestHandleMessage_TamperedBioRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedProfile(priv, pub, "orijinal bio", time.Now())
	p.Bio = "değiştirilmiş bio — imza artık geçersiz"

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("imza payload'ıyla eşleşmeyen (değiştirilmiş) bir profil kabul edilmemeliydi")
	}
}

func TestHandleMessage_TamperedTagsRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedFullProfile(priv, pub, "bio", "", []string{"Go"}, "", time.Now())
	p.Tags = []string{"Rust"} // imza "Go" üzerinden atıldı — Tags'i değiştirmek imzayı geçersiz kılmalı

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("imzalanmış Tags'ten farklı bir Tags değeri kabul edilmemeliydi")
	}
}

func TestHandleMessage_WrongAuthorKeyRejected(t *testing.T) {
	env := newTestStore(t)
	_, priv := genKey(t)     // gerçek imzalayan
	otherPub, _ := genKey(t) // iddia edilen (ama imzayı atmayan) yazar
	p := signedProfile(priv, otherPub, "sahte", time.Now())

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("AuthorKey ile imzalayan anahtar uyuşmuyorsa profil kabul edilmemeliydi")
	}
}

func TestHandleMessage_OversizedBioRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	bigBio := make([]byte, MaxBioBytes+1)
	for i := range bigBio {
		bigBio[i] = 'a'
	}
	p := signedProfile(priv, pub, string(bigBio), time.Now())

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("boyut sınırını aşan bir profil kabul edilmemeliydi")
	}
}

func TestHandleMessage_InvalidGitHubUserRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := &Profile{
		AuthorKey: hex.EncodeToString(pub), Bio: "bio",
		GitHubUser: "-invalid-", UpdatedAt: time.Now().UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(p)))

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("GitHub kullanıcı adı biçimine uymayan bir mesaj kabul edilmemeliydi")
	}
}

func TestHandleMessage_TamperedGitHubUserRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := &Profile{
		AuthorKey: hex.EncodeToString(pub), Bio: "bio",
		GitHubUser: "octocat", UpdatedAt: time.Now().UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(p)))
	p.GitHubUser = "attacker" // imza "octocat" üzerinden atıldı

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("imzalanmış GitHubUser'dan farklı bir değer kabul edilmemeliydi")
	}
}

func TestHandleMessage_InvalidTagsRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedFullProfile(priv, pub, "bio", "", []string{"Bilinmeyen Kategori"}, "", time.Now())

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("forum.Categories'te olmayan bir etiket taşıyan mesaj kabul edilmemeliydi")
	}
}

func TestHandleMessage_FutureTimestampRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedProfile(priv, pub, "gelecekten geliyorum", time.Now().Add(time.Hour))

	env.store.handleMessage(mustJSON(t, p))

	if got := env.store.Get(p.AuthorKey); got != nil {
		t.Fatal("maxClockSkew'i aşan gelecek tarihli bir güncelleme kabul edilmemeliydi")
	}
}

func TestHandleMessage_FutureTimestampDoesNotPoisonLaterHonestUpdate(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	future := signedProfile(priv, pub, "sahte gelecek", now.Add(24*time.Hour))
	env.store.handleMessage(mustJSON(t, future))
	if got := env.store.Get(future.AuthorKey); got != nil {
		t.Fatalf("gelecek tarihli mesaj hiç kabul edilmemeliydi (bir sonraki adımın önkoşulu), got %+v", got)
	}

	honest := signedProfile(priv, pub, "gerçek güncelleme", now)
	env.store.handleMessage(mustJSON(t, honest))

	got := env.store.Get(honest.AuthorKey)
	if got == nil || got.Bio != "gerçek güncelleme" {
		t.Fatalf("reddedilen gelecek tarihli mesaj, aynı yazarın dürüst zamanlı sonraki güncellemesini engellememeliydi, got %+v", got)
	}
}

func TestHandleMessage_StaleUpdateRejected(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	newer := signedProfile(priv, pub, "yeni", now)
	env.store.handleMessage(mustJSON(t, newer))

	older := signedProfile(priv, pub, "eski (replay)", now.Add(-time.Hour))
	env.store.handleMessage(mustJSON(t, older))

	got := env.store.Get(newer.AuthorKey)
	if got == nil || got.Bio != "yeni" {
		t.Fatalf("daha eski bir güncelleme, zaten sahip olduğumuz daha yeni profili değiştirmemeliydi, got %+v", got)
	}
}

func TestHandleMessage_RapidUpdatesRateLimited(t *testing.T) {
	old := minRepublishInterval
	minRepublishInterval = time.Hour // test süresince kesin olarak tetiklensin
	t.Cleanup(func() { minRepublishInterval = old })

	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedProfile(priv, pub, "ilk", now)
	env.store.handleMessage(mustJSON(t, first))

	second := signedProfile(priv, pub, "hemen ardından", now.Add(time.Second))
	env.store.handleMessage(mustJSON(t, second))

	got := env.store.Get(first.AuthorKey)
	if got == nil || got.Bio != "ilk" {
		t.Fatalf("minRepublishInterval içinde gelen ikinci güncelleme reddedilmeliydi, got %+v", got)
	}
}

// ─── Sybil koruması ─────────────────────────────────────────────────────────────

func TestHandleMessage_NewAuthorCapRejectsBeyondLimit(t *testing.T) {
	oldLimit := maxNewAuthorsPerHour
	maxNewAuthorsPerHour = 3
	t.Cleanup(func() { maxNewAuthorsPerHour = oldLimit })

	env := newTestStore(t)
	now := time.Now()

	var accepted int
	for i := 0; i < maxNewAuthorsPerHour+2; i++ {
		pub, priv := genKey(t)
		p := signedProfile(priv, pub, fmt.Sprintf("yazar %d", i), now)
		env.store.handleMessage(mustJSON(t, p))
		if env.store.Get(p.AuthorKey) != nil {
			accepted++
		}
	}
	if accepted != maxNewAuthorsPerHour {
		t.Fatalf("saatlik yeni-yazar sınırını aşan istekler reddedilmeliydi: %d kabul edildi, want %d", accepted, maxNewAuthorsPerHour)
	}
}

func TestHandleMessage_NewAuthorCapDoesNotBlockUpdatesToKnownAuthors(t *testing.T) {
	oldLimit := maxNewAuthorsPerHour
	maxNewAuthorsPerHour = 1
	t.Cleanup(func() { maxNewAuthorsPerHour = oldLimit })
	// minRepublishInterval is keyed off wall-clock arrival time, not the
	// message's UpdatedAt — without shrinking it, the second handleMessage
	// call below (fired milliseconds after the first, in real time) would be
	// dropped by the per-author rate limit instead of exercising the Sybil
	// cap path this test targets.
	oldInterval := minRepublishInterval
	minRepublishInterval = 0
	t.Cleanup(func() { minRepublishInterval = oldInterval })

	env := newTestStore(t)
	now := time.Now()

	pub, priv := genKey(t)
	first := signedProfile(priv, pub, "ilk", now)
	env.store.handleMessage(mustJSON(t, first))
	if env.store.Get(first.AuthorKey) == nil {
		t.Fatal("kota dahilindeki ilk yeni yazar kabul edilmeliydi")
	}

	// Kota artık dolu (1/1) — ama AYNI (zaten bilinen) yazardan gelen bir
	// güncelleme "yeni yazar" sayılmamalı, bu yüzden reddedilmemeli.
	update := signedProfile(priv, pub, "güncelleme", now.Add(time.Minute))
	env.store.handleMessage(mustJSON(t, update))
	got := env.store.Get(update.AuthorKey)
	if got == nil || got.Bio != "güncelleme" {
		t.Fatalf("dolu kota, zaten bilinen bir yazarın güncellemesini engellememeliydi, got %+v", got)
	}
}

func TestHandleMessage_MaxStoredProfilesCapRejectsBeyondCeiling(t *testing.T) {
	oldCap := maxStoredProfiles
	oldLimit := maxNewAuthorsPerHour
	maxStoredProfiles = 2
	maxNewAuthorsPerHour = 1000 // bu testte saatlik oran sınırı değil, toplam tavan test ediliyor
	t.Cleanup(func() {
		maxStoredProfiles = oldCap
		maxNewAuthorsPerHour = oldLimit
	})

	env := newTestStore(t)
	now := time.Now()

	var accepted int
	for i := 0; i < maxStoredProfiles+2; i++ {
		pub, priv := genKey(t)
		p := signedProfile(priv, pub, fmt.Sprintf("yazar %d", i), now)
		env.store.handleMessage(mustJSON(t, p))
		if env.store.Get(p.AuthorKey) != nil {
			accepted++
		}
	}
	if accepted != maxStoredProfiles {
		t.Fatalf("toplam profil tavanını aşan istekler reddedilmeliydi: %d kabul edildi, want %d", accepted, maxStoredProfiles)
	}
}

// ─── IsModerator / RoleLabel / RoleOf ───────────────────────────────────────────

func TestIsModerator_ValidCertForOwnKey(t *testing.T) {
	env := newTestStore(t)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	cert := issueCert(env.founderPriv, modKey, true, time.Time{})

	env.store.mu.Lock()
	env.store.profiles[modKey] = &Profile{AuthorKey: modKey, ModCert: &cert}
	env.store.mu.Unlock()

	if !env.store.IsModerator(modKey) {
		t.Fatal("kendi anahtarına ait geçerli bir sertifika Moderatör rozetini vermeli")
	}
	if got := env.store.RoleLabel(modKey); got != "Moderatör" {
		t.Errorf("RoleLabel = %q, want %q", got, "Moderatör")
	}
	if got := env.store.RoleOf(modKey); got != RoleModerator {
		t.Errorf("RoleOf = %v, want RoleModerator", got)
	}
}

func TestIsModerator_RejectsMismatchedKey(t *testing.T) {
	env := newTestStore(t)

	// Gerçek moderatöre ait, geçerli bir sertifika (ör. bir BanMsg'den
	// "toplanmış" olabilir — bkz. moderation.BanMsg.Cert alanı).
	realModPub, _ := genKey(t)
	cert := issueCert(env.founderPriv, hex.EncodeToString(realModPub), true, time.Time{})

	// Saldırgan bu sertifikayı KENDİ profiline iliştiriyor.
	attackerPub, _ := genKey(t)
	attackerKey := hex.EncodeToString(attackerPub)
	env.store.mu.Lock()
	env.store.profiles[attackerKey] = &Profile{AuthorKey: attackerKey, ModCert: &cert}
	env.store.mu.Unlock()

	if env.store.IsModerator(attackerKey) {
		t.Fatal("başka bir yazara ait sertifikayı kopyalayan bir profil Moderatör rozeti almamalı")
	}
	if got := env.store.RoleLabel(attackerKey); got != "Üye" {
		t.Errorf("RoleLabel = %q, want %q (sahte rozet reddedilmeli)", got, "Üye")
	}
}

func TestIsModerator_RejectsExpiredCert(t *testing.T) {
	env := newTestStore(t)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	cert := issueCert(env.founderPriv, modKey, false, time.Now().Add(-time.Hour))

	env.store.mu.Lock()
	env.store.profiles[modKey] = &Profile{AuthorKey: modKey, ModCert: &cert}
	env.store.mu.Unlock()

	if env.store.IsModerator(modKey) {
		t.Fatal("süresi dolmuş bir sertifika Moderatör rozeti vermemeli")
	}
}

func TestIsModerator_NoCert(t *testing.T) {
	env := newTestStore(t)
	pub, _ := genKey(t)
	if env.store.IsModerator(hex.EncodeToString(pub)) {
		t.Fatal("hiç profil/sertifika yokken Moderatör olmamalı")
	}
}

func TestRoleLabel_Founder(t *testing.T) {
	env := newTestStore(t)
	if got := env.store.RoleLabel(moderation.FounderPubKeyHex); got != "Kurucu" {
		t.Errorf("RoleLabel(kurucu) = %q, want %q", got, "Kurucu")
	}
	if got := env.store.RoleOf(moderation.FounderPubKeyHex); got != RoleFounder {
		t.Errorf("RoleOf(kurucu) = %v, want RoleFounder", got)
	}
	if !IsFounder(moderation.FounderPubKeyHex) {
		t.Error("IsFounder, kurucu anahtarı için true dönmeli")
	}
}

// ─── load(): bozuk veriye tolerans ──────────────────────────────────────────────

func TestLoad_ToleratesCorruptModCertJSON(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	authorKey := "aabbccddeeff"
	if err := db.UpsertProfile(storage.RawProfile{
		AuthorKey: authorKey, Bio: "bio metni korunmalı",
		ModCertJSON: "{bu geçerli bir json değil", Sig: "sig",
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	founderPub, _ := genKey(t)
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })
	mod, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	s, err := New(id, nil, db, mod)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.Get(authorKey)
	if got == nil {
		t.Fatal("bozuk mod_cert, tüm profilin yüklenmesini engellememeli")
	}
	if got.Bio != "bio metni korunmalı" {
		t.Errorf("Bio = %q, want korunmuş değer", got.Bio)
	}
	if got.ModCert != nil {
		t.Errorf("bozuk JSON'dan ModCert nil olmalı, got %+v", got.ModCert)
	}
}

// ─── profilePayload: alan sınırı belirsizliği / imza yumuşaklığı ──────────────

// TestProfilePayload_FieldBoundaryShiftInvalidatesSignature, profilePayload
// bir zamanlar ham "|" birleştirmesi kullandığında var olan bir açığın
// regresyon testidir: Bio/Status/Link serbest metindir ve "|" karakteri
// içerebilir (yalnızca bayt uzunluğu sınırlanır) — bu yüzden özel anahtara
// hiç ihtiyaç duymadan, geçerli imzalı bir mesajın Bio içindeki bir "|"
// üzerinden alan sınırını kaydırıp Bio/Status'u farklı ama aynı baytları
// üreten bir varyanta dönüştürmek mümkündü. JSON tabanlı signablePayload bu
// belirsizliği ortadan kaldırır; bu test bunu kanıtlar.
func TestProfilePayload_FieldBoundaryShiftInvalidatesSignature(t *testing.T) {
	pub, priv := genKey(t)
	now := time.Now().UTC()

	orig := &Profile{
		AuthorKey: hex.EncodeToString(pub),
		Bio:       "Blockchain dev | building AND",
		Status:    "scam link: notreal.com",
		UpdatedAt: now,
	}
	orig.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(orig)))
	if !verifyProfile(orig) {
		t.Fatal("orijinal mesaj kendi imzasıyla doğrulanmalı")
	}

	// Bio içindeki "|" karakterinden alan sınırını kaydırarak aynı toplam
	// metni farklı bir Bio/Status ikilisine böl — özel anahtar kullanılmadı.
	forged := &Profile{
		AuthorKey: orig.AuthorKey,
		Bio:       "Blockchain dev ",
		Status:    " building AND|scam link: notreal.com",
		UpdatedAt: now,
		Sig:       orig.Sig, // orijinal imza aynen kopyalandı
	}
	if verifyProfile(forged) {
		t.Fatal("alan sınırı kaydırılmış (sahte) bir Bio/Status ikilisi, orijinalin imzasıyla doğrulanmamalı")
	}
}

// TestProfilePayload_ModCertStrippedInvalidatesSignature, profilePayload'ın
// (bir zamanlar) ModCert'in ne varlığını ne de ModeratorKey'ini
// imzalamadığı için bir relay peer'ın moderatör sertifikasını sessizce
// çıkarıp Sig'i hâlâ geçerli bırakabildiği bir açığın regresyon testidir.
func TestProfilePayload_ModCertStrippedInvalidatesSignature(t *testing.T) {
	env := newTestStore(t)
	myKey := hex.EncodeToString(env.identity.PublicKey())
	cert := issueCert(env.founderPriv, myKey, true, time.Time{})
	writeCertFile(t, env.dataDir, myKey, cert)

	if err := env.store.PublishOwn(context.Background(), OwnUpdate{Bio: "moderatörüm"}); err != nil {
		t.Fatalf("PublishOwn: %v", err)
	}
	signed := env.store.Get(myKey)
	if signed == nil || signed.ModCert == nil {
		t.Fatalf("ModCert iliştirilmiş olmalıydı, got %+v", signed)
	}

	stripped := *signed
	stripped.ModCert = nil
	if verifyProfile(&stripped) {
		t.Fatal("ModCert'i çıkarılmış bir mesaj, orijinalin imzasıyla doğrulanmamalı")
	}
}

// ─── persist(): newest-wins yarış koruması ──────────────────────────────────

// TestPersist_OlderUpdateDoesNotOverwriteNewer, iki eşzamanlı yerel
// PublishOwn çağrısının (ör. cmd/and'in gecikmeli başlangıç yeniden-yayını
// ile kullanıcının Ayarlar'daki kendi düzenlemesi arasında bir yarış)
// hangisinin persist() çağrısı SONRA çalışırsa o kazanmak yerine, gerçekten
// daha yeni UpdatedAt'e sahip olanın kazandığını doğrular.
func TestPersist_OlderUpdateDoesNotOverwriteNewer(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now().UTC()

	newer := signedProfile(priv, pub, "yeni", now)
	older := signedProfile(priv, pub, "eski ama sonra persist edildi", now.Add(-time.Minute))

	// persist() çağrı SIRASI kasıtlı olarak UpdatedAt sırasının tersi —
	// eski olan SONRA persist ediliyor, tıpkı gerçek dünyadaki bir
	// zamanlama yarışında olabileceği gibi.
	env.store.persist(newer)
	env.store.persist(older)

	got := env.store.Get(newer.AuthorKey)
	if got == nil || got.Bio != "yeni" {
		t.Fatalf("daha eski bir UpdatedAt'e sahip persist() çağrısı, daha yeniyi ezmemeliydi, got %+v", got)
	}
}

// ─── PublishOwn: iptal edilmiş ctx yerel kaydı da engellemeli ────────────────

func TestPublishOwn_CanceledContextSkipsLocalPersist(t *testing.T) {
	env := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := env.store.PublishOwn(ctx, OwnUpdate{Bio: "bu asla kaydedilmemeli"})
	if err == nil {
		t.Fatal("iptal edilmiş bir ctx ile PublishOwn hata döndürmeliydi")
	}

	myKey := hex.EncodeToString(env.identity.PublicKey())
	if got := env.store.Get(myKey); got != nil {
		t.Fatalf("iptal edilmiş ctx sonrası yerel olarak da hiçbir şey kaydedilmemeliydi, got %+v", got)
	}
}
