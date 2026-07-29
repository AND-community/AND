package moderation

// handlers_test.go — moderation_test.go'daki (verifyCert/verifyBan/isBanned
// gibi saf yardımcı fonksiyonların birim testleri) YANINDA, mesaj-işleme
// (handleBanMsg/handleRevokeMsg) ve diske kalıcılık (saveBans/loadBans,
// saveRevocations/loadRevocations) uçtan uca akışlarını kapsar — bunlar
// önceki denetimde test kapsamı dışında kalan yollardı.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func randomPeerID(t *testing.T) peer.ID {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	lp2pPub, err := lp2pcrypto.UnmarshalEd25519PublicKey(pub)
	if err != nil {
		t.Fatalf("UnmarshalEd25519PublicKey: %v", err)
	}
	pid, err := peer.IDFromPublicKey(lp2pPub)
	if err != nil {
		t.Fatalf("IDFromPublicKey: %v", err)
	}
	return pid
}

func signedModCert(t *testing.T, founderPriv ed25519.PrivateKey, modPub ed25519.PublicKey) ModeratorCert {
	t.Helper()
	modKey := hex.EncodeToString(modPub)
	issuedAt := time.Now()
	expires := issuedAt.Add(24 * time.Hour)
	sig := ed25519.Sign(founderPriv, ModeratorCertPayload(modKey, issuedAt, expires, false))
	return ModeratorCert{ModeratorKey: modKey, IssuedAt: issuedAt, ExpiresAt: expires, Sig: hex.EncodeToString(sig)}
}

func signedBan(t *testing.T, modPriv ed25519.PrivateKey, cert ModeratorCert, target peer.ID, reason string) *BanMsg {
	t.Helper()
	return signedBanWithExpiry(t, modPriv, cert, target, reason, time.Now().Add(2*time.Hour))
}

func signedBanWithExpiry(t *testing.T, modPriv ed25519.PrivateKey, cert ModeratorCert, target peer.ID, reason string, expires time.Time) *BanMsg {
	t.Helper()
	payload := fmt.Sprintf("%s|%s|%d", target.String(), reason, expires.Unix())
	sig := ed25519.Sign(modPriv, []byte(payload))
	return &BanMsg{BannedPeer: target.String(), Reason: reason, ExpiresAt: expires, Cert: cert, Sig: hex.EncodeToString(sig)}
}

func signedPermanentBan(t *testing.T, modPriv ed25519.PrivateKey, cert ModeratorCert, target peer.ID, reason string) *BanMsg {
	t.Helper()
	payload := fmt.Sprintf("%s|%s|permanent", target.String(), reason)
	sig := ed25519.Sign(modPriv, []byte(payload))
	return &BanMsg{BannedPeer: target.String(), Reason: reason, Permanent: true, Cert: cert, Sig: hex.EncodeToString(sig)}
}

func signedRevoke(t *testing.T, founderPriv ed25519.PrivateKey, modKey string, issuedAt time.Time) *RevokeMsg {
	t.Helper()
	payload := fmt.Sprintf("revoke|%s|%d", modKey, issuedAt.Unix())
	sig := ed25519.Sign(founderPriv, []byte(payload))
	return &RevokeMsg{ModeratorKey: modKey, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}
}

func signedUnban(t *testing.T, modPriv ed25519.PrivateKey, cert ModeratorCert, target peer.ID) *UnbanMsg {
	t.Helper()
	issuedAt := time.Now()
	payload := fmt.Sprintf("unban|%s|%d", target.String(), issuedAt.Unix())
	sig := ed25519.Sign(modPriv, []byte(payload))
	return &UnbanMsg{BannedPeer: target.String(), Cert: cert, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}
}

// ─── handleBanMsg ───────────────────────────────────────────────────────────

func TestHandleBanMsg_ValidBanIsApplied(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))

	if !mod.IsBanned(target) {
		t.Fatal("geçerli, imzalı bir ban sonrası hedef banlı olmalı")
	}
}

func TestHandleBanMsg_TamperedSignatureRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	ban := signedBan(t, modPriv, cert, target, "spam")
	ban.Reason = "spam-degistirildi" // imzalanan payload'dan farklı — imza artık geçersiz

	mod.handleBanMsg(ban)

	if mod.IsBanned(target) {
		t.Fatal("değiştirilmiş (imza uyuşmayan) bir ban uygulanmamalı")
	}
}

func TestHandleBanMsg_UnknownFounderRejected(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	// Sertifika GERÇEK kurucu tarafından değil, rastgele bir anahtarla imzalanıyor.
	_, fakeFounderPriv := genKey(t)
	modPub, modPriv := genKey(t)
	fakeCert := signedModCert(t, fakeFounderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, fakeCert, target, "spam"))

	if mod.IsBanned(target) {
		t.Fatal("gerçek kurucu tarafından imzalanmamış bir sertifikayla ban uygulanmamalı")
	}
}

func TestHandleBanMsg_ReplayDoesNotConsumeExtraRateLimit(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)
	ban := signedBan(t, modPriv, cert, target, "spam")

	// Aynı (aynı imzalı) mesajı çok kez "oynat" — sahte tekrar saldırısı.
	for i := 0; i < banRateLimit+10; i++ {
		mod.handleBanMsg(ban)
	}
	if !mod.IsBanned(target) {
		t.Fatal("ilk uygulamadan sonra hedef banlı kalmalı")
	}

	// Tekrarlar hız sınırını tüketmediyse, aynı moderatör GERÇEK (farklı,
	// yeni imzalı) banlar için hâlâ hakkı olmalı — ilk (tekrarlanan) ban
	// zaten kotadan 1 birim harcadığı için burada banRateLimit-1 tanesi
	// başarılı olmalı (toplamda tam banRateLimit).
	for i := 0; i < banRateLimit-1; i++ {
		other := randomPeerID(t)
		mod.handleBanMsg(signedBan(t, modPriv, cert, other, "spam"))
		if !mod.IsBanned(other) {
			t.Fatalf("tekrar (%d.) yeni ban, hız sınırı tüketilmediği için uygulanabilmeli", i)
		}
	}
}

func TestHandleBanMsg_RevokedCertRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub) // ExpiresAt = şimdi+24s

	modKey := hex.EncodeToString(modPub)
	// İptalin issued_at'ı sertifikanın kendi süresinden SONRAYA ayarlanır —
	// bkz. verifyCert: revokedAt, cert.ExpiresAt'tan sonraysa cert iptal
	// öncesi verilmiş sayılıp geçersiz kılınır (TestVerifyCert_Revoked'daki
	// aynı desen, burada handleRevokeMsg üzerinden uçtan uca doğrulanıyor).
	issuedAt := cert.ExpiresAt.Add(time.Second)
	mod.handleRevokeMsg(signedRevoke(t, founderPriv, modKey, issuedAt))

	target := randomPeerID(t)
	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))

	if mod.IsBanned(target) {
		t.Fatal("revokedAt > cert.ExpiresAt olduğunda bu sertifikayla ban uygulanmamalı")
	}
}

// ─── kurucu (sertifikasız) ban yolu ─────────────────────────────────────────

func TestApplyBan_FounderBanIsApplied(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "kötüye kullanım", time.Now().Add(time.Hour), false)
	mod.handleBanMsg(&ban)

	if !mod.IsBanned(target) {
		t.Fatal("gerçek kurucu anahtarıyla imzalanmış, sertifikasız bir ban uygulanmalıydı")
	}
	mod.mu.RLock()
	rec := mod.bans[target]
	mod.mu.RUnlock()
	if rec == nil || rec.ModeratorKey != hex.EncodeToString(founderPub) {
		t.Fatalf("kayıtta ModeratorKey kurucunun anahtarı olmalıydı, got %+v", rec)
	}
}

func TestApplyBan_PermanentFounderBanIsApplied(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "ısrarlı spam", time.Time{}, true)
	mod.handleBanMsg(&ban)

	if !mod.IsBanned(target) {
		t.Fatal("kalıcı kurucu banı uygulanmalıydı")
	}
	mod.mu.RLock()
	rec := mod.bans[target]
	mod.mu.RUnlock()
	if rec == nil || !rec.Permanent {
		t.Fatal("kayıt Permanent=true olarak işaretlenmeliydi")
	}
}

func TestApplyBan_FounderBanSignedByWrongKeyRejected(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	_, wrongPriv := genKey(t) // gerçek kurucu değil
	target := randomPeerID(t)

	ban := SignFounderBan(wrongPriv, target.String(), "spam", time.Now().Add(time.Hour), false)
	mod.handleBanMsg(&ban)

	if mod.IsBanned(target) {
		t.Fatal("gerçek kurucunun anahtarıyla imzalanmamış bir 'kurucu banı' uygulanmamalıydı")
	}
}

func TestApplyBan_FounderBanRespectsExpiryBounds(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Now().Add(10*time.Second), false)
	mod.handleBanMsg(&ban)

	if mod.IsBanned(target) {
		t.Fatal("minBanDuration altındaki bir kurucu banı da reddedilmeliydi (aynı süre kuralları geçerli)")
	}
}

func TestSignModeratorBan_ProducesValidBan(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	ban := SignModeratorBan(modPriv, cert, target.String(), "spam", time.Now().Add(time.Hour), false)
	mod.handleBanMsg(&ban)

	if !mod.IsBanned(target) {
		t.Fatal("SignModeratorBan'ın ürettiği ban handleBanMsg tarafından kabul edilmeliydi")
	}
}

func TestIssueBan_FounderPath_AppliesLocallyAndPublishes(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	modB, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	topicA, topicB, ctx := twoConnectedTopics(t, "test/issue-ban-founder")
	modB.Start(ctx, topicB)

	target := randomPeerID(t)
	ban := SignFounderBan(founderPriv, target.String(), "kötüye kullanım", time.Now().Add(time.Hour), false)

	if err := modA.IssueBan(ctx, topicA, ban); err != nil {
		t.Fatalf("IssueBan: %v", err)
	}
	if !modA.IsBanned(target) {
		t.Fatal("IssueBan, kendi düğümünde (modA) hemen etkili olmalıydı — GossipSub yankısını beklememeli")
	}

	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsBanned(target) {
		if time.Now().After(deadline) {
			t.Fatal("ban, IssueBan üzerinden yayınlandıktan sonra modB'de de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ─── esnek süreli / kalıcı (permanent) banlar ───────────────────────────────

func TestHandleBanMsg_PermanentBanIsApplied(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedPermanentBan(t, modPriv, cert, target, "ısrarlı spam"))

	if !mod.IsBanned(target) {
		t.Fatal("geçerli, imzalı kalıcı bir ban sonrası hedef banlı olmalı")
	}
	mod.mu.RLock()
	rec := mod.bans[target]
	mod.mu.RUnlock()
	if rec == nil || !rec.Permanent {
		t.Fatal("kayıt Permanent=true olarak işaretlenmeliydi")
	}
}

func TestHandleBanMsg_PermanentBanSurvivesCleanup(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedPermanentBan(t, modPriv, cert, target, "ısrarlı spam"))
	if !mod.IsBanned(target) {
		t.Fatal("kurulum: kalıcı ban önce uygulanmış olmalıydı")
	}

	// cleanupExpiredBans saatte bir gerçekte çalışır (bkz. cleanupLoop) —
	// burada doğrudan çağırıyoruz. Permanent bir ban, "süresi dolmuş"
	// sayılıp yanlışlıkla temizlenmemeli.
	mod.cleanupExpiredBans()

	if !mod.IsBanned(target) {
		t.Fatal("kalıcı bir ban, süre-dolum temizliğinden etkilenmemeliydi")
	}
}

func TestHandleBanMsg_ShortDurationAccepted(t *testing.T) {
	// Kullanıcının istediği somut örnek: 1 saatlik bir chat/forum yasağı.
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBanWithExpiry(t, modPriv, cert, target, "hakaret", time.Now().Add(time.Hour)))

	if !mod.IsBanned(target) {
		t.Fatal("1 saatlik geçerli bir ban uygulanmalıydı")
	}
}

func TestHandleBanMsg_TooShortDurationRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBanWithExpiry(t, modPriv, cert, target, "spam", time.Now().Add(10*time.Second)))

	if mod.IsBanned(target) {
		t.Fatal("minBanDuration'ın altındaki bir süre reddedilmeliydi")
	}
}

func TestHandleBanMsg_ExceedsMaxBanDaysRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBanWithExpiry(t, modPriv, cert, target, "spam", time.Now().Add(31*24*time.Hour)))

	if mod.IsBanned(target) {
		t.Fatal("maxBanDays'i aşan (permanent olmayan) bir ban reddedilmeliydi — bunun için --permanent kullanılmalı")
	}
}

// ─── handleRevokeMsg ────────────────────────────────────────────────────────

func TestHandleRevokeMsg_TamperedSignatureIgnored(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	modKey := hex.EncodeToString(modPub)
	cert := signedModCert(t, founderPriv, modPub)

	rev := signedRevoke(t, founderPriv, modKey, cert.ExpiresAt.Add(time.Second))
	sigBytes, _ := hex.DecodeString(rev.Sig)
	sigBytes[0] ^= 0xFF // imzayı boz
	rev.Sig = hex.EncodeToString(sigBytes)

	mod.handleRevokeMsg(rev)

	// Sahte (imzası bozuk) iptal hiç işlenmemeli — sertifika hâlâ geçerli olmalı.
	target := randomPeerID(t)
	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))
	if !mod.IsBanned(target) {
		t.Fatal("sahte (imzası bozuk) bir iptal mesajı sertifikayı geçersiz kılmamalı")
	}
}

// ─── handleUnbanMsg ─────────────────────────────────────────────────────────

func TestHandleUnbanMsg_ValidUnbanLiftsBan(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))
	if !mod.IsBanned(target) {
		t.Fatal("kurulum: ban önce uygulanmış olmalıydı")
	}

	mod.handleUnbanMsg(signedUnban(t, modPriv, cert, target))

	if mod.IsBanned(target) {
		t.Fatal("geçerli bir unban sonrası hedef artık banlı olmamalı")
	}
}

func TestHandleUnbanMsg_TamperedSignatureRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))

	u := signedUnban(t, modPriv, cert, target)
	sigBytes, _ := hex.DecodeString(u.Sig)
	sigBytes[0] ^= 0xFF
	u.Sig = hex.EncodeToString(sigBytes)
	mod.handleUnbanMsg(u)

	if !mod.IsBanned(target) {
		t.Fatal("değiştirilmiş (imza uyuşmayan) bir unban banı kaldırmamalıydı")
	}
}

func TestHandleUnbanMsg_UnknownFounderCertRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	realCert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, realCert, target, "spam"))
	if !mod.IsBanned(target) {
		t.Fatal("kurulum: ban önce uygulanmış olmalıydı")
	}

	// Aynı moderatör anahtarı, ama GERÇEK kurucu tarafından değil, rastgele
	// bir anahtarla imzalanmış bir sertifikayla unban denemesi.
	_, fakeFounderPriv := genKey(t)
	fakeCert := signedModCert(t, fakeFounderPriv, modPub)
	mod.handleUnbanMsg(signedUnban(t, modPriv, fakeCert, target))

	if !mod.IsBanned(target) {
		t.Fatal("gerçek kurucu tarafından imzalanmamış bir sertifikayla unban uygulanmamalıydı")
	}
}

func TestHandleUnbanMsg_ReplayDoesNotConsumeExtraRateLimit(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam")) // 1. kota birimi
	u := signedUnban(t, modPriv, cert, target)

	for i := 0; i < banRateLimit+10; i++ {
		mod.handleUnbanMsg(u) // ilk çağrı 2. kota birimini tüketir, kalanı deduplike edilir
	}
	if mod.IsBanned(target) {
		t.Fatal("ilk unban'dan sonra hedef banlı kalmamalı")
	}

	// Tekrarlar hız sınırını tüketmediyse, aynı moderatör GERÇEK (farklı,
	// yeni imzalı) banlar için hâlâ hakkı olmalı — bu noktaya kadar zaten
	// 2 birim (ilk ban + ilk unban) tüketildiğinden banRateLimit-2 tanesi
	// başarılı olmalı (toplamda tam banRateLimit).
	for i := 0; i < banRateLimit-2; i++ {
		other := randomPeerID(t)
		mod.handleBanMsg(signedBan(t, modPriv, cert, other, "spam"))
		if !mod.IsBanned(other) {
			t.Fatalf("tekrar (%d.) yeni ban, hız sınırı tüketilmediği için uygulanabilmeli", i)
		}
	}
}

// ─── Diske kalıcılık round-trip ─────────────────────────────────────────────

func TestModerator_BanPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)
	modA.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))
	if !modA.IsBanned(target) {
		t.Fatal("ban modA üzerinde uygulanmalıydı")
	}

	// "Yeniden başlatma" simülasyonu: aynı dataDir ile yeni bir Moderator.
	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	if !modB.IsBanned(target) {
		t.Fatal("ban, diskten yeniden yüklendikten sonra da geçerli olmalı")
	}
}

func TestModerator_PermanentBanPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)
	modA.handleBanMsg(signedPermanentBan(t, modPriv, cert, target, "ısrarlı spam"))
	if !modA.IsBanned(target) {
		t.Fatal("kalıcı ban modA üzerinde uygulanmalıydı")
	}

	// "Yeniden başlatma" simülasyonu: aynı dataDir ile yeni bir Moderator.
	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	if !modB.IsBanned(target) {
		t.Fatal("kalıcı ban, diskten yeniden yüklendikten sonra da geçerli olmalı")
	}
	modB.mu.RLock()
	rec := modB.bans[target]
	modB.mu.RUnlock()
	if rec == nil || !rec.Permanent {
		t.Fatal("yeniden yüklenen kayıt Permanent=true olarak korunmalıydı")
	}
}

func TestModerator_UnbanPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)
	modA.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))
	if !modA.IsBanned(target) {
		t.Fatal("kurulum: ban önce uygulanmış olmalıydı")
	}
	modA.handleUnbanMsg(signedUnban(t, modPriv, cert, target))
	if modA.IsBanned(target) {
		t.Fatal("kurulum: unban modA üzerinde uygulanmalıydı")
	}

	// "Yeniden başlatma" simülasyonu: aynı dataDir ile yeni bir Moderator.
	// Unban diske yazılmış olmalı (saveBans), yoksa restart sonrası ban
	// "diriliverir".
	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	if modB.IsBanned(target) {
		t.Fatal("kaldırılmış bir ban, diskten yeniden yüklendikten sonra geri gelmemeliydi")
	}
}

func TestModerator_RevocationPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	issuedAt := time.Now().Round(time.Second) // JSON zaman damgası çözünürlüğüyle eşleşsin
	modA.handleRevokeMsg(signedRevoke(t, founderPriv, modKey, issuedAt))

	modA.mu.RLock()
	wantRevokedAt, ok := modA.revoked[modKey]
	modA.mu.RUnlock()
	if !ok {
		t.Fatal("iptal modA üzerinde kaydedilmeliydi")
	}

	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	modB.mu.RLock()
	gotRevokedAt, ok := modB.revoked[modKey]
	modB.mu.RUnlock()
	if !ok {
		t.Fatal("iptal, diskten yeniden yüklendikten sonra da bulunmalı")
	}
	if !gotRevokedAt.Equal(wantRevokedAt) {
		t.Fatalf("iptal zaman damgası korunmadı: modA=%v modB=%v", wantRevokedAt, gotRevokedAt)
	}
}

// ─── founder-path unban (kurucunun kendi banını kaldırabilmesi) ────────────

func TestApplyUnban_FounderUnbanLiftsFounderIssuedBan(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Time{}, true)
	mod.handleBanMsg(&ban)
	if !mod.IsBanned(target) {
		t.Fatal("kurulum: kurucu banı önce uygulanmış olmalıydı")
	}

	u := SignFounderUnban(founderPriv, target.String())
	mod.handleUnbanMsg(&u)

	if mod.IsBanned(target) {
		t.Fatal("kurucunun kendi imzasıyla kaldırdığı bir ban artık etkili olmamalı")
	}
}

func TestApplyUnban_FounderUnbanCanLiftModeratorIssuedBan(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))
	if !mod.IsBanned(target) {
		t.Fatal("kurulum: moderatör banı önce uygulanmış olmalıydı")
	}

	// "Herhangi bir moderatör herhangi bir banı kaldırabilir" simetrisi
	// kurucu için de geçerli olmalı — bkz. applyUnban'ın doc yorumu.
	u := SignFounderUnban(founderPriv, target.String())
	mod.handleUnbanMsg(&u)

	if mod.IsBanned(target) {
		t.Fatal("kurucu, bir moderatörün verdiği banı da kaldırabilmeliydi")
	}
}

func TestApplyUnban_ModeratorUnbanCannotBeReplayedAsFounderUnban(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)

	mod.handleBanMsg(signedBan(t, modPriv, cert, target, "spam"))

	// Moderatörün kendi anahtarıyla imzaladığı (ModeratorUnbanPayload,
	// "unban|..." önekli) bir unban'ı, Cert boş bırakılarak "kurucu yolu"
	// gibi göstermeye çalışmak — verifyFounderUnban farklı bir önek
	// ("founder-unban|...") beklediği için reddedilmeli.
	modOnly := signedUnban(t, modPriv, cert, target)
	forged := &UnbanMsg{BannedPeer: modOnly.BannedPeer, IssuedAt: modOnly.IssuedAt, Sig: modOnly.Sig}
	mod.handleUnbanMsg(forged)

	if !mod.IsBanned(target) {
		t.Fatal("moderatör-yolu imzası, boş Cert ile kurucu-yolu olarak yeniden oynatılabilmemeliydi")
	}
}

func TestApplyUnban_FounderUnbanSignedByWrongKeyRejected(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	_, wrongPriv := genKey(t)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Time{}, true)
	mod.handleBanMsg(&ban)

	u := SignFounderUnban(wrongPriv, target.String())
	mod.handleUnbanMsg(&u)

	if !mod.IsBanned(target) {
		t.Fatal("gerçek kurucunun anahtarıyla imzalanmamış bir 'kurucu unban'ı uygulanmamalıydı")
	}
}

func TestSignFounderUnban_ProducesValidUnban(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)
	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Time{}, true)
	mod.handleBanMsg(&ban)

	u := SignFounderUnban(founderPriv, target.String())
	if !mod.applyUnban(&u) {
		t.Fatal("SignFounderUnban'ın ürettiği unban applyUnban tarafından kabul edilmeliydi")
	}
}

func TestIssueUnban_FounderPath_AppliesLocallyAndPublishes(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	modB, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	topicA, topicB, ctx := twoConnectedTopics(t, "test/issue-unban-founder")
	modB.Start(ctx, topicB)

	target := randomPeerID(t)
	ban := SignFounderBan(founderPriv, target.String(), "kötüye kullanım", time.Time{}, true)
	if err := modA.IssueBan(ctx, topicA, ban); err != nil {
		t.Fatalf("IssueBan (kurulum): %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsBanned(target) {
		if time.Now().After(deadline) {
			t.Fatal("kurulum: ban modB'ye yayınlanmadı")
		}
		time.Sleep(20 * time.Millisecond)
	}

	u := SignFounderUnban(founderPriv, target.String())
	if err := modA.IssueUnban(ctx, topicA, u); err != nil {
		t.Fatalf("IssueUnban: %v", err)
	}
	if modA.IsBanned(target) {
		t.Fatal("IssueUnban, kendi düğümünde (modA) hemen etkili olmalıydı")
	}

	deadline = time.Now().Add(10 * time.Second)
	for modB.IsBanned(target) {
		if time.Now().After(deadline) {
			t.Fatal("unban, IssueUnban üzerinden yayınlandıktan sonra modB'de de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestIssueUnban_NotCurrentlyBannedReturnsError(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	u := SignFounderUnban(founderPriv, target.String())
	if err := mod.IssueUnban(context.Background(), nil, u); err == nil {
		t.Fatal("hiç banlı olmayan bir hedef için IssueUnban hata döndürmeliydi")
	}
}

// ─── AllBans / PeerIDToAuthorKey ────────────────────────────────────────────

func TestAllBans_ReturnsActiveExcludesExpired(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	permTarget := randomPeerID(t)
	tempTarget := randomPeerID(t)
	expiredTarget := randomPeerID(t)

	permBan := SignFounderBan(founderPriv, permTarget.String(), "kalıcı", time.Time{}, true)
	mod.handleBanMsg(&permBan)
	tempBan := SignFounderBan(founderPriv, tempTarget.String(), "geçici", time.Now().Add(time.Hour), false)
	mod.handleBanMsg(&tempBan)

	// Süresi geçmiş bir kaydı doğrudan iç haritaya ekleyip AllBans'ın onu
	// filtrelediğini doğruluyoruz (applyBan zaten geçmiş bir ExpiresAt'i
	// reddeder, bu yüzden dışarıdan bu şekilde simüle ediyoruz).
	mod.mu.Lock()
	mod.bans[expiredTarget] = &banRecord{ExpiresAt: time.Now().Add(-time.Minute), ModeratorKey: hex.EncodeToString(founderPub), Reason: "süresi geçmiş"}
	mod.mu.Unlock()

	entries := mod.AllBans()
	found := map[string]BanEntry{}
	for _, e := range entries {
		found[e.PeerID] = e
	}
	if _, ok := found[permTarget.String()]; !ok {
		t.Error("kalıcı ban AllBans içinde olmalıydı")
	}
	if _, ok := found[tempTarget.String()]; !ok {
		t.Error("geçici (süresi dolmamış) ban AllBans içinde olmalıydı")
	}
	if _, ok := found[expiredTarget.String()]; ok {
		t.Error("süresi dolmuş ban AllBans içinde OLMAMALIYDI")
	}
	if e := found[permTarget.String()]; e.AuthorKey == "" {
		t.Error("BanEntry.AuthorKey, ed25519 tabanlı bir peer.ID için doldurulmalıydı")
	}
}

func TestPeerIDToAuthorKey_RoundTripsWithAuthorKeyToPeerID(t *testing.T) {
	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)

	pid, err := AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}
	got, err := PeerIDToAuthorKey(pid)
	if err != nil {
		t.Fatalf("PeerIDToAuthorKey: %v", err)
	}
	if !strings.EqualFold(got, authorKey) {
		t.Fatalf("round-trip başarısız: want %s, got %s", authorKey, got)
	}
}

// ─── Ban geçmişi (audit log) ────────────────────────────────────────────────

func TestApplyBan_RecordsHistoryEntry(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Now().Add(time.Hour), false)
	mod.handleBanMsg(&ban)

	h := mod.History()
	if len(h) != 1 {
		t.Fatalf("bir ban sonrası tam olarak 1 geçmiş kaydı bekleniyordu, got %d", len(h))
	}
	if h[0].Action != HistoryBan || h[0].PeerID != target.String() || h[0].Reason != "spam" {
		t.Fatalf("beklenmeyen geçmiş kaydı: %+v", h[0])
	}
}

func TestApplyUnban_RecordsHistoryEntryNewestFirst(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	target := randomPeerID(t)

	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Time{}, true)
	mod.handleBanMsg(&ban)
	u := SignFounderUnban(founderPriv, target.String())
	mod.handleUnbanMsg(&u)

	h := mod.History()
	if len(h) != 2 {
		t.Fatalf("ban+unban sonrası 2 geçmiş kaydı bekleniyordu, got %d", len(h))
	}
	if h[0].Action != HistoryUnban {
		t.Fatalf("en yeni kayıt (index 0) unban olmalıydı, got %s", h[0].Action)
	}
	if h[1].Action != HistoryBan {
		t.Fatalf("ikinci kayıt (index 1) ban olmalıydı, got %s", h[1].Action)
	}
	if h[0].Reason != "spam" {
		t.Fatalf("unban kaydı, kaldırılan banın sebebini taşımalıydı, got %q", h[0].Reason)
	}
}

func TestHistory_PersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	target := randomPeerID(t)
	ban := SignFounderBan(founderPriv, target.String(), "spam", time.Now().Add(time.Hour), false)
	modA.handleBanMsg(&ban)

	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	h := modB.History()
	if len(h) != 1 || h[0].PeerID != target.String() {
		t.Fatalf("geçmiş kaydı diskten yeniden yüklenmeliydi, got %+v", h)
	}
}

// ─── Moderatör atama/kaldırma (grant/revoke) ────────────────────────────────

func TestIssueGrant_ProducesVerifiableCertAndRecordsGrant(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	targetPub, _ := genKey(t)
	targetHex := hex.EncodeToString(targetPub)

	cert, _, err := mod.IssueGrant(founderPriv, targetHex, time.Now().Add(24*time.Hour), false)
	if err != nil {
		t.Fatalf("IssueGrant: %v", err)
	}
	if !mod.VerifyModeratorCert(cert) {
		t.Fatal("IssueGrant'ın ürettiği sertifika verifyCert tarafından geçerli sayılmalıydı")
	}

	grants := mod.GrantedMods()
	if len(grants) != 1 || !strings.EqualFold(grants[0].Key, targetHex) || grants[0].Revoked {
		t.Fatalf("GrantedMods, yeni verilen sertifikayı iptal edilmemiş olarak listelemeliydi, got %+v", grants)
	}
}

func TestIssueGrant_RejectsExpiryBeyondMaxCertDays(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	targetPub, _ := genKey(t)
	targetHex := hex.EncodeToString(targetPub)

	// IssueGrant artık kendi ürettiği sertifikayı verifyCert ile
	// self-doğruluyor (applyBan/applyUnban/applyRevoke'un hepsinin zaten
	// yaptığı gibi) — MaxCertDays'i aşan bir süre bu adımda hemen
	// reddedilir, asla "başarılı ama hiçbir yerde geçerli olmayan" bir
	// sertifika üretilmez.
	_, _, err := mod.IssueGrant(founderPriv, targetHex, time.Now().Add(365*24*time.Hour), false)
	if err == nil {
		t.Fatal("MaxCertDays'i aşan bir süre IssueGrant tarafından reddedilmeliydi")
	}
	grants := mod.GrantedMods()
	if len(grants) != 0 {
		t.Fatalf("reddedilen bir grant GrantedMods'a hiç eklenmemeliydi, got %+v", grants)
	}
}

func TestIssueGrant_WrongFounderKeyRejected(t *testing.T) {
	founderPub, _ := genKey(t)
	mod := newTestMod(t, founderPub)
	_, wrongPriv := genKey(t) // gerçek kurucu değil
	targetPub, _ := genKey(t)
	targetHex := hex.EncodeToString(targetPub)

	_, path, err := mod.IssueGrant(wrongPriv, targetHex, time.Now().Add(24*time.Hour), false)
	if err == nil {
		t.Fatal("gerçek kurucunun anahtarıyla imzalanmamış bir grant IssueGrant tarafından reddedilmeliydi")
	}
	if path != "" {
		t.Fatalf("reddedilen bir grant için hiçbir dosya yazılmamalıydı, got %q", path)
	}
	if grants := mod.GrantedMods(); len(grants) != 0 {
		t.Fatalf("reddedilen bir grant GrantedMods'a hiç eklenmemeliydi, got %+v", grants)
	}
}

func TestIssueRevoke_MarksMatchingGrantRevoked(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	mod, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	targetPub, _ := genKey(t)
	targetHex := hex.EncodeToString(targetPub)

	cert, _, err := mod.IssueGrant(founderPriv, targetHex, time.Now().Add(24*time.Hour), false)
	if err != nil {
		t.Fatalf("IssueGrant: %v", err)
	}
	if !mod.VerifyModeratorCert(cert) {
		t.Fatal("kurulum: sertifika iptalden önce geçerli olmalıydı")
	}

	rev := SignRevoke(founderPriv, targetHex)
	topic, _, ctx := twoConnectedTopics(t, "test/issue-revoke")
	if err := mod.IssueRevoke(ctx, topic, rev); err != nil {
		t.Fatalf("IssueRevoke: %v", err)
	}

	if mod.VerifyModeratorCert(cert) {
		t.Fatal("iptalden sonra sertifika artık geçerli olmamalıydı")
	}
	grants := mod.GrantedMods()
	if len(grants) != 1 || !grants[0].Revoked {
		t.Fatalf("GrantedMods, sertifikayı iptal edilmiş olarak işaretlemeliydi, got %+v", grants)
	}
}

func TestGrantedMods_PersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	founderPub, founderPriv := genKey(t)
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	targetPub, _ := genKey(t)
	targetHex := hex.EncodeToString(targetPub)
	if _, _, err := modA.IssueGrant(founderPriv, targetHex, time.Now().Add(24*time.Hour), false); err != nil {
		t.Fatalf("IssueGrant: %v", err)
	}

	modB, err := New(dir, peer.ID(""))
	if err != nil {
		t.Fatalf("New (yeniden başlatma): %v", err)
	}
	grants := modB.GrantedMods()
	if len(grants) != 1 || !strings.EqualFold(grants[0].Key, targetHex) {
		t.Fatalf("verilen sertifika kaydı diskten yeniden yüklenmeliydi, got %+v", grants)
	}
}
