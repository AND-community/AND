package moderation

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
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

func newTestMod(t *testing.T, founderPub ed25519.PublicKey) *Moderator {
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
	var payload string
	if permanent {
		payload = fmt.Sprintf("%s|permanent", modKey)
	} else {
		payload = fmt.Sprintf("%s|%d", modKey, expiresAt.Unix())
	}
	sig := ed25519.Sign(founderPriv, []byte(payload))
	return ModeratorCert{
		ModeratorKey: modKey,
		Permanent:    permanent,
		ExpiresAt:    expiresAt,
		Sig:          hex.EncodeToString(sig),
	}
}

func signApproval(priv ed25519.PrivateKey, postID string, issuedAt time.Time, cert *ModeratorCert) ApprovalMsg {
	payload := fmt.Sprintf("approve|%s|%d", postID, issuedAt.Unix())
	sig := ed25519.Sign(priv, []byte(payload))
	return ApprovalMsg{PostID: postID, IssuedAt: issuedAt, Cert: cert, Sig: hex.EncodeToString(sig)}
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
	expires := time.Now().Add(24 * time.Hour)
	cert := signCert(founderPriv, modKey, false, expires)

	// revokedAt > cert.ExpiresAt → cert iptal öncesi verilmiş sayılır, geçersiz.
	mod.mu.Lock()
	mod.revoked[modKey] = expires.Add(time.Second)
	mod.mu.Unlock()

	if mod.verifyCert(cert) {
		t.Fatal("iptal edilmiş moderatör sertifikası reddedilmeli")
	}
}

// ─── verifyApproval (yeni freshness kontrolü) ─────────────────────────────────

func TestVerifyApproval_ValidFounder(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	msg := signApproval(founderPriv, "aabbccdd11223344", time.Now(), nil)
	if !mod.verifyApproval(&msg) {
		t.Fatal("kurucu imzalı geçerli onay doğrulanmalı")
	}
}

func TestVerifyApproval_TooOld(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	issuedAt := time.Now().Add(-25 * time.Hour)
	msg := signApproval(founderPriv, "aabbccdd11223344", issuedAt, nil)
	if mod.verifyApproval(&msg) {
		t.Fatal("24 saatten eski onay mesajı reddedilmeli")
	}
}

func TestVerifyApproval_Replay(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)
	mod.seenApprovals = newSeenSet(seenCacheSize)

	msg := signApproval(founderPriv, "aabbccdd11223344", time.Now(), nil)

	var approved int
	mod.SetOnApprove(func(string) { approved++ })

	mod.handleApproveMsg(&msg)
	mod.handleApproveMsg(&msg) // tekrar

	if approved != 1 {
		t.Fatalf("tekrar saldırısı: onay %d kez işlendi, 1 bekleniyor", approved)
	}
}

func TestVerifyApproval_ValidModerator(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	mod := newTestMod(t, founderPub)

	modPub, modPriv := genKey(t)
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(24 * time.Hour)
	cert := signCert(founderPriv, modKey, false, expires)

	msg := signApproval(modPriv, "aabbccdd11223344", time.Now(), &cert)
	if !mod.verifyApproval(&msg) {
		t.Fatal("moderatör imzalı geçerli onay doğrulanmalı")
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
	mod.bans[pid] = time.Now().Add(time.Hour)
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
	mod.bans[pid] = time.Now().Add(-time.Second)
	mod.mu.Unlock()

	if mod.IsBanned(pid) {
		t.Fatal("süresi dolmuş ban banned=false döndürmeli")
	}
}

func TestIsBanned_DisabledModerator(t *testing.T) {
	mod := &Moderator{
		founder:       make(ed25519.PublicKey, ed25519.PublicKeySize),
		bans:          make(map[peer.ID]time.Time),
		seenApprovals: newSeenSet(seenCacheSize),
	}
	pid := peer.ID("12D3KooWBanned")
	mod.bans[pid] = time.Now().Add(time.Hour)

	if mod.IsBanned(pid) {
		t.Fatal("moderasyon devre dışıyken IsBanned daima false döndürmeli")
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
	defer func() { FounderPubKeyHex = old }()

	isFounder, err := LoadFounderKey(dir, myPubHex)
	if err != nil {
		t.Fatalf("LoadFounderKey: %v", err)
	}
	if !isFounder {
		t.Fatal("ilk çalıştırmada kendi anahtarımızı kurucuya yazmalı ve isFounder=true döndürmeli")
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
