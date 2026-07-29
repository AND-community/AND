package tui

// moderation_test.go — screenModeration (Moderasyon Paneli) için testler:
// panele giriş yetkisi (canAccessModeration), pencere kaydırma (windowSlice)
// ve Banlar/Moderatörler sekmelerindeki uçtan uca aksiyonlar (issueModUnban/
// issueModGrant/issueModRevoke). bkz. moderation.go.

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/peer"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
)

// textInputWithValue builds a textinput.Model pre-filled with v — used by
// grant-form tests that need to skip straight past manual keystroke entry.
func textInputWithValue(v string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(v)
	return ti
}

// ─── canAccessModeration ────────────────────────────────────────────────────

func TestCanAccessModeration_FounderTrue(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	founderHex := hex.EncodeToString(founderPub)
	withFounderKey(t, founderHex)

	if !canAccessModeration(nil, t.TempDir(), founderHex) {
		t.Fatal("kurucu her zaman panele erişebilmeli")
	}
}

func TestCanAccessModeration_PlainUserFalse(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderPub))

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if canAccessModeration(nil, t.TempDir(), hex.EncodeToString(otherPub)) {
		t.Fatal("ne kurucu ne de sertifikalı bir moderatör olan bir kimlik panele erişememeli")
	}
}

func TestCanAccessModeration_ModeratorWithCertTrue(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderPub))

	modPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	dataDir := t.TempDir()
	writeModCertFile(t, dataDir, founderPriv, modPub)
	mod, err := moderation.New(dataDir, peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	if !canAccessModeration(mod, dataDir, hex.EncodeToString(modPub)) {
		t.Fatal("bulunabilir geçerli bir sertifikası olan bir moderatör panele erişebilmeli")
	}
}

func TestCanAccessModeration_RevokedCertFalse(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	modPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	modKeyHex := hex.EncodeToString(modPub)
	dataDir := t.TempDir()
	writeModCertFile(t, dataDir, founderIdentity.PrivateKey(), modPub)

	mod, topic, ctx := newSoloModTopicWithDataDir(t, "test/can-access-moderation-revoked", dataDir)
	rev := moderation.SignRevoke(founderIdentity.PrivateKey(), modKeyHex)
	if err := mod.IssueRevoke(ctx, topic, rev); err != nil {
		t.Fatalf("IssueRevoke: %v", err)
	}

	if canAccessModeration(mod, dataDir, modKeyHex) {
		t.Fatal("sertifikası iptal edilmiş bir moderatör panele erişebilir görünmemeli")
	}
}

// ─── windowSlice ────────────────────────────────────────────────────────────

func TestWindowSlice_FitsEntirelyWithinSize(t *testing.T) {
	start, end := windowSlice(5, 2, 12)
	if start != 0 || end != 5 {
		t.Fatalf("liste pencereden küçükse tamamı gösterilmeli, got [%d,%d)", start, end)
	}
}

func TestWindowSlice_CentersCursor(t *testing.T) {
	start, end := windowSlice(100, 50, 10)
	if end-start != 10 {
		t.Fatalf("pencere boyutu 10 olmalı, got %d", end-start)
	}
	if start > 50 || end <= 50 {
		t.Fatalf("pencere cursor'ı (50) içermeli, got [%d,%d)", start, end)
	}
}

func TestWindowSlice_ClampsAtEnd(t *testing.T) {
	start, end := windowSlice(20, 19, 10)
	if end != 20 {
		t.Fatalf("cursor listenin sonundaysa pencere de sonda bitmeli, got end=%d", end)
	}
	if end-start != 10 {
		t.Fatalf("pencere boyutu korunmalı, got %d", end-start)
	}
}

// ─── Banlar sekmesi: issueModUnban ──────────────────────────────────────────

func TestIssueModUnban_LiftsSelectedBan(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	mod, topic, ctx := newSoloModTopic(t, "test/mod-panel-unban")

	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetKeyHex := hex.EncodeToString(targetPub)
	if err := issueBanFor(ctx, mod, topic, founderIdentity, targetKeyHex, true, nil, "spam", banDurationPresets[len(banDurationPresets)-1]); err != nil {
		t.Fatalf("issueBanFor (kurulum): %v", err)
	}

	m := appModel{
		ctx:      ctx,
		identity: founderIdentity,
		mod:      mod,
		modTopic: topic,
		dataDir:  t.TempDir(),
	}
	m.refreshModerationLists()
	if len(m.modBans) != 1 {
		t.Fatalf("kurulum: tam olarak 1 aktif ban bekleniyordu, got %d", len(m.modBans))
	}
	m.modCursor = 0

	got, _ := m.issueModUnban()
	res := got.(appModel)
	if res.modNoticeErr {
		t.Fatalf("issueModUnban başarısız olmamalıydı: %s", res.modNotice)
	}
	if mod.IsAuthorBanned(targetKeyHex) {
		t.Fatal("issueModUnban sonrası hedef artık banlı olmamalı")
	}
}

func TestIssueModUnban_WithoutAuthorityIsRejected(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderPub))

	mod, topic, ctx := newSoloModTopic(t, "test/mod-panel-unban-no-auth")

	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetKeyHex := hex.EncodeToString(targetPub)
	targetPid, err := moderation.AuthorKeyToPeerID(targetKeyHex)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}

	// Panelin "yerel kullanıcısı" ne kurucu ne de sertifikalı bir
	// moderatör — resolveBanAuthority bu hedef için canBan=false
	// dönmeli, gerçekte banlı olup olmamasından bağımsız olarak.
	plainIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	m := appModel{
		ctx: ctx, identity: plainIdentity, mod: mod, modTopic: topic, dataDir: t.TempDir(),
		modBans: []moderation.BanEntry{{PeerID: targetPid.String(), AuthorKey: targetKeyHex, Permanent: true}},
	}
	m.modCursor = 0

	got, _ := m.issueModUnban()
	res := got.(appModel)
	if !res.modNoticeErr {
		t.Fatal("yetkisiz bir kimlik unban deneyince modNoticeErr set edilmeliydi")
	}
}

// ─── Moderatörler sekmesi: issueModGrant / issueModRevoke ──────────────────

func TestIssueModGrant_CreatesVerifiableCertAndAppearsInList(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	mod, _, ctx := newSoloModTopic(t, "test/mod-panel-grant")
	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetHex := hex.EncodeToString(targetPub)

	m := appModel{ctx: ctx, identity: founderIdentity, mod: mod, modIsFounder: true}
	m.modGrantKeyInput = textInputWithValue(targetHex)
	m.modGrantDurationIdx = 0 // "1 gün"

	got, _ := m.issueModGrant()
	res := got.(appModel)
	if res.modNoticeErr {
		t.Fatalf("issueModGrant başarısız olmamalıydı: %s", res.modNotice)
	}

	grants := mod.GrantedMods()
	if len(grants) != 1 || grants[0].Key != targetHex || grants[0].Revoked {
		t.Fatalf("verilen sertifika GrantedMods'ta iptal edilmemiş olarak görünmeliydi, got %+v", grants)
	}
	// IssueGrant'ın ürettiği sertifikanın gerçekten verifyCert tarafından
	// kabul edildiği internal/moderation paketinde ayrıca doğrulanıyor
	// (TestIssueGrant_ProducesVerifiableCertAndRecordsGrant) — burada
	// yalnızca panel katmanının (issueModGrant) IssueGrant'ı doğru
	// parametrelerle çağırıp sonucu doğru yansıttığını test ediyoruz.
}

func TestIssueModGrant_InvalidKeyRejectedAtStepZero(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))
	mod, _, ctx := newSoloModTopic(t, "test/mod-panel-grant-invalid")

	m := appModel{ctx: ctx, identity: founderIdentity, mod: mod, modIsFounder: true, modGranting: true}
	m.modGrantKeyInput = textInputWithValue("not-a-valid-hex-key")

	got, _ := m.handleModGrantKey(tea.KeyMsg{Type: tea.KeyEnter})
	res := got.(appModel)
	if !res.modNoticeErr {
		t.Fatal("geçersiz hex bir hedef anahtarla ilerleme kabul edilmemeliydi")
	}
	if res.modGrantStep != 0 {
		t.Fatal("geçersiz anahtar sonrası hâlâ adım 0'da (anahtar girişi) kalınmalıydı")
	}
}

func TestIssueModRevoke_RevokesSelectedGrant(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	mod, topic, ctx := newSoloModTopic(t, "test/mod-panel-revoke")
	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetHex := hex.EncodeToString(targetPub)
	cert, _, err := mod.IssueGrant(founderIdentity.PrivateKey(), targetHex, time.Now().Add(24*time.Hour), false)
	if err != nil {
		t.Fatalf("IssueGrant (kurulum): %v", err)
	}
	if !mod.VerifyModeratorCert(cert) {
		t.Fatal("kurulum: sertifika iptalden önce geçerli olmalıydı")
	}

	m := appModel{ctx: ctx, identity: founderIdentity, mod: mod, modTopic: topic, modIsFounder: true}
	m.refreshModerationLists()
	if len(m.modGrants) != 1 {
		t.Fatalf("kurulum: tam olarak 1 verilmiş sertifika bekleniyordu, got %d", len(m.modGrants))
	}
	m.modCursor = 0

	got, _ := m.issueModRevoke()
	res := got.(appModel)
	if res.modNoticeErr {
		t.Fatalf("issueModRevoke başarısız olmamalıydı: %s", res.modNotice)
	}
	if mod.VerifyModeratorCert(cert) {
		t.Fatal("iptalden sonra sertifika artık geçerli olmamalıydı")
	}
}

// ─── Sekme sayısı ────────────────────────────────────────────────────────────

func TestModerationTabCount_FounderHasThreeTabs(t *testing.T) {
	m := appModel{modIsFounder: true}
	if got := m.moderationTabCount(); got != 3 {
		t.Fatalf("kurucu için 3 sekme bekleniyordu, got %d", got)
	}
}

func TestModerationTabCount_ModeratorHasTwoTabs(t *testing.T) {
	m := appModel{modIsFounder: false}
	if got := m.moderationTabCount(); got != 2 {
		t.Fatalf("sıradan bir moderatör için 2 sekme bekleniyordu, got %d", got)
	}
}
