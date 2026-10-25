package moderation

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func newTestMod(t testing.TB, founderPub ed25519.PublicKey) *Moderator {
	t.Helper()
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	mod, err := New("", peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return mod
}

func signCert(founderPriv ed25519.PrivateKey, modKey string, permanent bool, expiresAt time.Time) ModeratorCert {
	return signCertIssuedAt(founderPriv, modKey, permanent, time.Now(), expiresAt)
}

// signCertIssuedAt is signCert with an explicit IssuedAt — needed by
// revocation tests, which must control whether the cert predates or
// postdates a given revoke (bkz. ModeratorCert.IssuedAt'in doc yorumu).
func signCertIssuedAt(founderPriv ed25519.PrivateKey, modKey string, permanent bool, issuedAt, expiresAt time.Time) ModeratorCert {
	sig := ed25519.Sign(founderPriv, ModeratorCertPayload(modKey, issuedAt, expiresAt, permanent))
	return ModeratorCert{
		ModeratorKey: modKey,
		Permanent:    permanent,
		IssuedAt:     issuedAt,
		ExpiresAt:    expiresAt,
		Sig:          hex.EncodeToString(sig),
	}
}

// ─── seenSet ──────────────────────────────────────────────────────────────────

func TestSeenSet_FirstTimeFalse(t *testing.T) {
	s := newSeenSet(16)
	if s.seen("abc") {
		t.Fatal("ilk kez görülen imza seen=true döndürmemeli")
	}
}

func TestSeenSet_SecondTimeTrue(t *testing.T) {
	s := newSeenSet(16)
	s.seen("abc")
	if !s.seen("abc") {
		t.Fatal("ikinci kez görülen imza seen=true döndürmeli")
	}
}

func TestSeenSet_RingBufferEviction(t *testing.T) {
	s := newSeenSet(2)
	s.seen("a")
	s.seen("b")
	s.seen("c") // "a" evict edilir
	if s.seen("a") {
		t.Fatal("evict edilmiş giriş artık 'seen' sayılmamalı")
	}
}

func TestSeenSet_Size4096(t *testing.T) {
	s := newSeenSet(seenCacheSize)
	if len(s.ring) != seenCacheSize {
		t.Fatalf("ring boyutu %d olmalı, %d", seenCacheSize, len(s.ring))
	}
}

// ─── isDisabled ───────────────────────────────────────────────────────────────

func TestIsDisabled_ZeroKey(t *testing.T) {
	mod := &Moderator{founder: make(ed25519.PublicKey, ed25519.PublicKeySize)}
	if !mod.isDisabled() {
		t.Fatal("sıfır kurucu anahtarı isDisabled=true döndürmeli")
	}
}

func TestIsDisabled_RealKey(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := &Moderator{founder: founderPub}
	if mod.isDisabled() {
		t.Fatal("gerçek kurucu anahtarı isDisabled=false döndürmeli")
	}
}

// ─── verifyCert ───────────────────────────────────────────────────────────────

func TestVerifyCert_Valid(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(24 * time.Hour)
	cert := signCert(founderPriv, modKey, false, expires)

	if !mod.verifyCert(cert) {
		t.Fatal("geçerli sertifika doğrulanmalı")
	}
}

func TestVerifyModeratorCert_MatchesInternalVerifyCert(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(24 * time.Hour)
	valid := signCert(founderPriv, modKey, false, expires)
	expired := signCert(founderPriv, modKey, false, time.Now().Add(-time.Hour))

	if !mod.VerifyModeratorCert(valid) {
		t.Fatal("exported VerifyModeratorCert, geçerli sertifikayı reddetmemeli — internal/profile buna güveniyor")
	}
	if mod.VerifyModeratorCert(expired) {
		t.Fatal("exported VerifyModeratorCert, süresi dolmuş sertifikayı kabul etmemeli")
	}
}

func TestVerifyCert_Permanent(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	cert := signCert(founderPriv, modKey, true, time.Time{})

	if !mod.verifyCert(cert) {
		t.Fatal("kalıcı sertifika doğrulanmalı")
	}
}

func TestVerifyCert_Expired(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(-1 * time.Hour)
	cert := signCert(founderPriv, modKey, false, expires)

	if mod.verifyCert(cert) {
		t.Fatal("süresi dolmuş sertifika reddedilmeli")
	}
}

func TestVerifyCert_ExceedsMaxDays(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(time.Duration(maxCertDays+1) * 24 * time.Hour)
	cert := signCert(founderPriv, modKey, false, expires)

	if mod.verifyCert(cert) {
		t.Fatal("maxCertDays aşan sertifika reddedilmeli")
	}
}

func TestVerifyCert_BadSignature(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	_, otherPriv := genKey(t)
	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(24 * time.Hour)
	cert := signCert(otherPriv, modKey, false, expires)

	if mod.verifyCert(cert) {
		t.Fatal("yanlış imzalı sertifika reddedilmeli")
	}
}

func TestVerifyCert_Revoked(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	issuedAt := time.Now().Add(-time.Minute)
	expires := time.Now().Add(24 * time.Hour) // hâlâ günlerce geçerliliği kalan, SÜRESİ DOLMAMIŞ bir sertifika
	cert := signCertIssuedAt(founderPriv, modKey, false, issuedAt, expires)

	// revokedAt, cert'in IssuedAt'inden SONRA → bu sertifika iptalin
	// kapsamında, henüz doğal olarak süresi dolmamış olsa bile hemen
	// reddedilmeli (asıl amaç budur: hâlâ aktif bir moderatörü anında
	// yetkisiz bırakabilmek).
	mod.mu.Lock()
	mod.revoked[modKey] = time.Now()
	mod.mu.Unlock()

	if mod.verifyCert(cert) {
		t.Fatal("iptal edilmiş moderatör sertifikası, süresi dolmamış olsa bile reddedilmeli")
	}
}

func TestVerifyCert_ReGrantedAfterRevokeIsValid(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)

	revokedAt := time.Now()
	mod.mu.Lock()
	mod.revoked[modKey] = revokedAt
	mod.mu.Unlock()

	// IssuedAt, revokedAt'tan SONRA → founder aynı kişiyi yeniden
	// moderatör yapmış demektir; eski iptal bunu etkilememeli.
	newCert := signCertIssuedAt(founderPriv, modKey, false, revokedAt.Add(time.Second), time.Now().Add(24*time.Hour))
	if !mod.verifyCert(newCert) {
		t.Fatal("bir iptalden SONRA yeniden verilen sertifika geçerli sayılmalı")
	}
}

// ─── IsBanned ─────────────────────────────────────────────────────────────────

func TestIsBanned_NotBanned(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	pid := peer.ID("12D3KooWTest")
	if mod.IsBanned(pid) {
		t.Fatal("banlı olmayan peer banned=false döndürmeli")
	}
}

func TestIsBanned_ActiveBan(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	pid := peer.ID("12D3KooWBanned")
	mod.mu.Lock()
	mod.bans[pid] = &banRecord{ExpiresAt: time.Now().Add(time.Hour)}
	mod.mu.Unlock()

	if !mod.IsBanned(pid) {
		t.Fatal("aktif banlı peer banned=true döndürmeli")
	}
}

func TestIsBanned_ExpiredBan(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	pid := peer.ID("12D3KooWExpired")
	mod.mu.Lock()
	mod.bans[pid] = &banRecord{ExpiresAt: time.Now().Add(-time.Second)}
	mod.mu.Unlock()

	if mod.IsBanned(pid) {
		t.Fatal("süresi dolmuş ban banned=false döndürmeli")
	}
}

func TestIsBanned_DisabledModerator(t *testing.T) {
	mod := &Moderator{
		founder: make(ed25519.PublicKey, ed25519.PublicKeySize),
		bans:    make(map[peer.ID]*banRecord),
	}
	pid := peer.ID("12D3KooWBanned")
	mod.bans[pid] = &banRecord{ExpiresAt: time.Now().Add(time.Hour)}

	if mod.IsBanned(pid) {
		t.Fatal("moderasyon devre dışıyken IsBanned daima false döndürmeli")
	}
}

// ─── PeerBanInfo / AuthorBanInfo ─────────────────────────────────────────────

func TestPeerBanInfo_NotBanned(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	if _, ok := mod.PeerBanInfo(peer.ID("12D3KooWNotBanned")); ok {
		t.Fatal("banlı olmayan bir peer için ok=false dönmeli")
	}
}

func TestPeerBanInfo_PermanentBan(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	pid := peer.ID("12D3KooWBanned")

	mod.mu.Lock()
	mod.bans[pid] = &banRecord{Permanent: true, ModeratorKey: "modkey", Reason: "spam"}
	mod.mu.Unlock()

	info, ok := mod.PeerBanInfo(pid)
	if !ok {
		t.Fatal("kalıcı banlı bir peer için ok=true dönmeli")
	}
	if !info.Permanent {
		t.Fatal("Permanent=true dönmeli")
	}
	if info.Reason != "spam" {
		t.Fatalf("Reason = %q, want %q", info.Reason, "spam")
	}
}

func TestPeerBanInfo_TemporaryBanReturnsExpiresAt(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	pid := peer.ID("12D3KooWBanned")
	expires := time.Now().Add(2 * time.Hour)

	mod.mu.Lock()
	mod.bans[pid] = &banRecord{ExpiresAt: expires, Reason: "hakaret"}
	mod.mu.Unlock()

	info, ok := mod.PeerBanInfo(pid)
	if !ok {
		t.Fatal("aktif geçici banlı bir peer için ok=true dönmeli")
	}
	if info.Permanent {
		t.Fatal("Permanent=false olmalıydı")
	}
	if !info.ExpiresAt.Equal(expires) {
		t.Fatalf("ExpiresAt = %v, want %v", info.ExpiresAt, expires)
	}
	if info.Reason != "hakaret" {
		t.Fatalf("Reason = %q, want %q", info.Reason, "hakaret")
	}
}

func TestPeerBanInfo_ExpiredBanReturnsNotOK(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	pid := peer.ID("12D3KooWExpired")

	mod.mu.Lock()
	mod.bans[pid] = &banRecord{ExpiresAt: time.Now().Add(-time.Hour)}
	mod.mu.Unlock()

	if _, ok := mod.PeerBanInfo(pid); ok {
		t.Fatal("süresi dolmuş bir ban için ok=false dönmeli")
	}
}

func TestPeerBanInfo_DisabledModeratorReturnsNotOK(t *testing.T) {
	mod := &Moderator{
		founder: make(ed25519.PublicKey, ed25519.PublicKeySize),
		bans:    make(map[peer.ID]*banRecord),
	}
	pid := peer.ID("12D3KooWBanned")
	mod.bans[pid] = &banRecord{Permanent: true}

	if _, ok := mod.PeerBanInfo(pid); ok {
		t.Fatal("moderasyon devre dışıyken ok=false dönmeli")
	}
}

func TestAuthorBanInfo_MatchesPeerBanInfoForDerivedPeerID(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)
	pid, err := AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}

	if _, ok := mod.AuthorBanInfo(authorKey); ok {
		t.Fatal("banlanmamış bir yazar için ok=false dönmeli")
	}

	mod.mu.Lock()
	mod.bans[pid] = &banRecord{Permanent: true, Reason: "test"}
	mod.mu.Unlock()

	info, ok := mod.AuthorBanInfo(authorKey)
	if !ok || !info.Permanent || info.Reason != "test" {
		t.Fatalf("aynı ed25519 anahtardan türetilen peer.ID banlıyken doğru bilgi dönmeli, got info=%+v ok=%v", info, ok)
	}
}

// ─── AuthorKeyToPeerID / IsAuthorBanned ─────────────────────────────────────

func TestAuthorKeyToPeerID_InvalidHexReturnsError(t *testing.T) {
	if _, err := AuthorKeyToPeerID("bu-hex-degil"); err == nil {
		t.Fatal("geçersiz hex için hata dönmeliydi")
	}
}

func TestAuthorKeyToPeerID_WrongLengthReturnsError(t *testing.T) {
	if _, err := AuthorKeyToPeerID("aabbcc"); err == nil {
		t.Fatal("32 bayttan kısa bir anahtar için hata dönmeliydi")
	}
}

func TestAuthorKeyToPeerID_SameKeyProducesSamePeerID(t *testing.T) {
	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)
	pid1, err := AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}
	pid2, err := AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}
	if pid1 != pid2 {
		t.Fatal("aynı AuthorKey her zaman aynı peer.ID'yi üretmeli (deterministik olmalı)")
	}
}

func TestIsAuthorBanned_MatchesIsBannedForDerivedPeerID(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)
	pid, err := AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}

	if mod.IsAuthorBanned(authorKey) {
		t.Fatal("banlanmamış bir yazar için IsAuthorBanned false dönmeli")
	}

	mod.mu.Lock()
	mod.bans[pid] = &banRecord{ExpiresAt: time.Now().Add(time.Hour)}
	mod.mu.Unlock()

	if !mod.IsAuthorBanned(authorKey) {
		t.Fatal("aynı ed25519 anahtardan türetilen peer.ID banlıyken IsAuthorBanned true dönmeli")
	}
}

func TestIsAuthorBanned_InvalidAuthorKeyReturnsFalse(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	if mod.IsAuthorBanned("gecersiz-hex") {
		t.Fatal("geçersiz bir AuthorKey için IsAuthorBanned false dönmeli (panik değil)")
	}
}

// ─── allowBan rate limit ───────────────────────────────────────────────────────

func TestAllowBan_RateLimit(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)

	for i := 0; i < banRateLimit; i++ {
		if !mod.allowBan(modKey) {
			t.Fatalf("ban %d reddedildi, limit %d", i, banRateLimit)
		}
	}
	if mod.allowBan(modKey) {
		t.Fatal("limit aşıldıktan sonra ban izni verilmemeli")
	}
}

// ─── LoadFounderKey ───────────────────────────────────────────────────────────

func TestLoadFounderKey_FirstRun_BecomesFounder(t *testing.T) {
	dir := t.TempDir()
	founderPub, _ := genKey(t)
	myPubHex := hex.EncodeToString(founderPub)

	old := FounderPubKeyHex
	// Bu test yalnızca "derleme zamanında -ldflags ile kurucu anahtarı
	// gömülmedi" durumunu (bkz. defaultFounderPubKeyHex) sınıyor — ortam
	// değişkeninin (paylaşılan global) başka bir testten kirli kalmadığından
	// emin olmak için açıkça sıfırlanır.
	FounderPubKeyHex = defaultFounderPubKeyHex
	defer func() { FounderPubKeyHex = old }()

	isFounder, err := LoadFounderKey(dir, myPubHex)
	if err != nil {
		t.Fatalf("LoadFounderKey: %v", err)
	}
	if !isFounder {
		t.Fatal("ilk çalıştırmada kendi anahtarımızı kurucuya yazmalı ve isFounder=true döndürmeli")
	}
}

// Bir binary -ldflags "-X .../FounderPubKeyHex=<hex>" ile GERÇEK bir kurucu
// anahtarıyla derlenmişse, ilk çalıştıran sıradan bir kullanıcı ASLA kendi
// anahtarını kurucu ilan edemez — gömülü kurucu anahtarı otoriter kalmalı.
// Bu, LoadFounderKey'in daha önce myPubKeyHex'i koşulsuz yazan (ve böylece
// gömülü kurucu anahtarını yok sayıp her kullanıcıyı kendi kurucusu ilan
// eden) hatasının regresyon testidir.
func TestLoadFounderKey_EmbeddedFounderKey_OrdinaryUserNotFounder(t *testing.T) {
	dir := t.TempDir()
	realFounderPub, _ := genKey(t)
	realFounderHex := hex.EncodeToString(realFounderPub)

	old := FounderPubKeyHex
	FounderPubKeyHex = realFounderHex // -ldflags ile gömülmüş gibi
	defer func() { FounderPubKeyHex = old }()

	myPub, _ := genKey(t)
	myHex := hex.EncodeToString(myPub)

	isFounder, err := LoadFounderKey(dir, myHex)
	if err != nil {
		t.Fatalf("LoadFounderKey: %v", err)
	}
	if isFounder {
		t.Fatal("gömülü kurucu anahtarı kendi anahtarımızdan farklıysa isFounder=false olmalı")
	}
	if !strings.EqualFold(FounderPubKeyHex, realFounderHex) {
		t.Fatalf("gömülü kurucu anahtarı korunmalıydı, aldık: %s", FounderPubKeyHex)
	}

	data, err := os.ReadFile(filepath.Join(dir, FounderKeyFile))
	if err != nil {
		t.Fatalf("founder.key okunamadı: %v", err)
	}
	if got := strings.TrimSpace(string(data)); !strings.EqualFold(got, realFounderHex) {
		t.Fatalf("founder.key dosyasına gömülü kurucu anahtarı yazılmalıydı, aldık: %s", got)
	}
}

// Kurucunun KENDİ düğümünde ilk çalıştırma: myPubKeyHex gömülü kurucu
// anahtarıyla eşleşiyorsa isFounder=true olmalı.
func TestLoadFounderKey_EmbeddedFounderKey_FounderIsFounder(t *testing.T) {
	dir := t.TempDir()
	founderPub, _ := genKey(t)
	founderHex := hex.EncodeToString(founderPub)

	old := FounderPubKeyHex
	FounderPubKeyHex = founderHex // -ldflags ile gömülmüş gibi
	defer func() { FounderPubKeyHex = old }()

	isFounder, err := LoadFounderKey(dir, founderHex)
	if err != nil {
		t.Fatalf("LoadFounderKey: %v", err)
	}
	if !isFounder {
		t.Fatal("kendi kimliği gömülü kurucu anahtarıyla eşleşen düğüm isFounder=true olmalı")
	}
}

func TestLoadFounderKey_ExistingKey_NotFounder(t *testing.T) {
	dir := t.TempDir()
	existingPub, _ := genKey(t)
	existingHex := hex.EncodeToString(existingPub)

	old := FounderPubKeyHex
	defer func() { FounderPubKeyHex = old }()

	_, _ = LoadFounderKey(dir, existingHex)

	myPub, _ := genKey(t)
	myHex := hex.EncodeToString(myPub)

	isFounder, err := LoadFounderKey(dir, myHex)
	if err != nil {
		t.Fatalf("LoadFounderKey: %v", err)
	}
	if isFounder {
		t.Fatal("farklı anahtar isFounder=false döndürmeli")
	}
}
