package tui

// ban_test.go — profil ekranından banlama özelliğini (forum.go'daki
// resolveBanAuthority/issueBanFor/banDurationPresets) kapsar: yalnızca
// admin/moderatörün ban aksiyonunu görebildiğini ve banın gerçekten ağa
// uygulandığını uçtan uca doğrular.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

// withFounderKey geçici olarak moderation.FounderPubKeyHex'i ayarlar,
// testin sonunda eski değerine döner.
func withFounderKey(t *testing.T, founderPubHex string) {
	t.Helper()
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = founderPubHex
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })
}

// writeModCertFile, andmod grant'in ürettiğiyle aynı biçimde bir
// bans/mod_*.json dosyası yazar — resolveBanAuthority'nin moderation.
// FindModCert üzerinden bulacağı gerçekçi bir sertifika.
func writeModCertFile(t *testing.T, dataDir string, founderPriv ed25519.PrivateKey, modPub ed25519.PublicKey) {
	t.Helper()
	modKey := hex.EncodeToString(modPub)
	expires := time.Now().Add(24 * time.Hour)
	cert := moderation.SignModeratorCert(founderPriv, modKey, expires, false)

	bansDir := filepath.Join(dataDir, "bans")
	if err := os.MkdirAll(bansDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.MarshalIndent(cert, "", "  ")
	if err := os.WriteFile(filepath.Join(bansDir, "mod_test.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// ─── resolveBanAuthority: "sadece admin ve moderatör görsün" ────────────────

func TestResolveBanAuthority_CannotBanSelf(t *testing.T) {
	canBan, _, _ := resolveBanAuthority(nil, t.TempDir(), "abc", "abc")
	if canBan {
		t.Fatal("kendini banlayabilme yetkisi olmamalı")
	}
}

func TestResolveBanAuthority_CannotBanFounder(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	founderHex := hex.EncodeToString(founderPub)
	withFounderKey(t, founderHex)

	canBan, _, _ := resolveBanAuthority(nil, t.TempDir(), "some-other-key", founderHex)
	if canBan {
		t.Fatal("kurucu banlanabilir görünmemeli")
	}
}

func TestResolveBanAuthority_FounderCanBanAnyoneElse(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	founderHex := hex.EncodeToString(founderPub)
	withFounderKey(t, founderHex)

	// Kurucu yolu bir *moderation.Moderator'a ihtiyaç duymaz (ağ
	// doğrulaması gerektirmeyen tek yol) — mod=nil ile de çalışmalı.
	canBan, isFounder, cert := resolveBanAuthority(nil, t.TempDir(), founderHex, "some-member-key")
	if !canBan || !isFounder || cert != nil {
		t.Fatalf("kurucu, kurucu olmayan biri için banlayabilmeli, got canBan=%v isFounder=%v cert=%v", canBan, isFounder, cert)
	}
}

func TestResolveBanAuthority_ModeratorWithValidCertCanBan(t *testing.T) {
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

	canBan, isFounder, cert := resolveBanAuthority(mod, dataDir, hex.EncodeToString(modPub), "some-member-key")
	if !canBan || isFounder || cert == nil {
		t.Fatalf("geçerli sertifikalı moderatör banlayabilmeli, got canBan=%v isFounder=%v cert=%v", canBan, isFounder, cert)
	}
}

func TestResolveBanAuthority_RevokedCertCannotBan(t *testing.T) {
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

	mod, topic, ctx := newSoloModTopicWithDataDir(t, "test/resolve-ban-authority-revoked", dataDir)
	rev := moderation.SignRevoke(founderIdentity.PrivateKey(), modKeyHex)
	if err := mod.IssueRevoke(ctx, topic, rev); err != nil {
		t.Fatalf("IssueRevoke: %v", err)
	}

	// FindModCert kendisi iptalden habersiz olduğundan (bkz. doc yorumu),
	// dosya hâlâ orada duruyor olsa bile resolveBanAuthority artık
	// mod.VerifyModeratorCert ile ikinci bir geçiş yapıp reddetmeli.
	canBan, _, cert := resolveBanAuthority(mod, dataDir, modKeyHex, "some-member-key")
	if canBan || cert != nil {
		t.Fatal("sertifikası iptal edilmiş bir moderatör artık banlayabilir görünmemeli")
	}
}

func TestResolveBanAuthority_PlainMemberCannotBan(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderPub))

	memberPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	canBan, _, cert := resolveBanAuthority(nil, t.TempDir(), hex.EncodeToString(memberPub), "some-other-key")
	if canBan || cert != nil {
		t.Fatal("ne kurucu ne moderatör olan sıradan bir üye banlayabilir görünmemeli")
	}
}

// ─── Chat profil ekranı: "b" tuşu yalnızca yetkiliyse çalışır ───────────────

func TestAppModel_ChatBanKey_NoopWhenCannotBan(t *testing.T) {
	m := newTestModel(t)
	m.screen = screenChatProfile
	m.chatProfilKey = "someone"
	m.chatProfilAd = "Someone"
	m.chatProfilCanBan = false

	got, _ := m.handleChatProfileKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	result := got.(appModel)

	if result.banConfirming {
		t.Fatal("banlama yetkisi olmayan biri için 'b' tuşu onay ekranını açmamalıydı")
	}
}

func TestAppModel_ChatBanKey_OpensConfirmWhenCanBan(t *testing.T) {
	m := newTestModel(t)
	m.screen = screenChatProfile
	m.chatProfilKey = "someone"
	m.chatProfilAd = "Someone"
	m.chatProfilCanBan = true

	got, _ := m.handleChatProfileKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	result := got.(appModel)

	if !result.banConfirming {
		t.Fatal("banlama yetkisi olan biri için 'b' tuşu onay ekranını açmalıydı")
	}
}

func TestAppModel_ChatBanConfirm_EmptyReasonRejected(t *testing.T) {
	m := newTestModel(t)
	m.screen = screenChatProfile
	m.chatProfilKey = "someone"
	m.chatProfilCanBan = true
	m.banConfirming = true
	m.banReasonInput.SetValue("   ")

	got, _ := m.handleChatBanConfirmKey(keyMsg("enter"))
	result := got.(appModel)

	if !result.banConfirming {
		t.Fatal("boş sebeple onay ekranı kapanmamalıydı")
	}
	if !result.banNoticeErr || result.banNotice == "" {
		t.Fatal("boş sebep için bir hata bildirimi gösterilmeliydi")
	}
}

// ─── issueBanFor: hatalı girdi durumları ────────────────────────────────────

func TestIssueBanFor_NilModeratorReturnsError(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	err = issueBanFor(context.Background(), nil, nil, id, "target-key", true, nil, "sebep", banDurationPresets[0])
	if err == nil {
		t.Fatal("mod nil'ken hata dönmeliydi")
	}
}

func TestIssueBanFor_InvalidTargetKeyReturnsError(t *testing.T) {
	founderPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderPub))
	mod, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	err = issueBanFor(context.Background(), mod, &network.Topic{}, id, "gecersiz-hex-degil", true, nil, "sebep", banDurationPresets[0])
	if err == nil {
		t.Fatal("geçersiz AuthorKey için hata dönmeliydi")
	}
}

// ─── issueBanFor: gerçek ağ üzerinden uçtan uca (kurucu yolu) ───────────────

func twoConnectedModTopics(t *testing.T, topicName string) (topicA, topicB *network.Topic, ctx context.Context) {
	t.Helper()
	idA, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (A): %v", err)
	}
	idB, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (B): %v", err)
	}

	nodeA, err := network.New(idA, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (A): %v", err)
	}
	t.Cleanup(func() { _ = nodeA.Close() })
	nodeB, err := network.New(idB, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (B): %v", err)
	}
	t.Cleanup(func() { _ = nodeB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	psA, err := network.NewPubSub(ctx, nodeA.Host)
	if err != nil {
		t.Fatalf("NewPubSub (A): %v", err)
	}
	psB, err := network.NewPubSub(ctx, nodeB.Host)
	if err != nil {
		t.Fatalf("NewPubSub (B): %v", err)
	}

	topicA, err = network.JoinTopic(psA, nodeA.Host, topicName)
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(topicA.Close)
	topicB, err = network.JoinTopic(psB, nodeB.Host, topicName)
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(topicB.Close)

	time.Sleep(300 * time.Millisecond) // gossipsub mesh'inin oturması için
	return topicA, topicB, ctx
}

func TestIssueBanFor_FounderPath_AppliesLocallyAndPublishes(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (founder): %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	modA, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (A): %v", err)
	}
	modB, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (B): %v", err)
	}

	topicA, topicB, ctx := twoConnectedModTopics(t, "test/tui-issue-ban-founder")
	modB.Start(ctx, topicB)

	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetKeyHex := hex.EncodeToString(targetPub)

	if err := issueBanFor(ctx, modA, topicA, founderIdentity, targetKeyHex, true, nil, "kötüye kullanım", banDurationPresets[0]); err != nil {
		t.Fatalf("issueBanFor: %v", err)
	}

	if !modA.IsAuthorBanned(targetKeyHex) {
		t.Fatal("issueBanFor, kendi düğümünde (modA) hemen etkili olmalıydı")
	}

	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsAuthorBanned(targetKeyHex) {
		if time.Now().After(deadline) {
			t.Fatal("ban, issueBanFor üzerinden yayınlandıktan sonra modB'de de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ─── profilMetni: ban durumu bölümü ──────────────────────────────────────────

func newSoloModTopic(t *testing.T, topicName string) (*moderation.Moderator, *network.Topic, context.Context) {
	t.Helper()
	return newSoloModTopicWithDataDir(t, topicName, t.TempDir())
}

// newSoloModTopicWithDataDir is newSoloModTopic with an explicit dataDir —
// needed by tests that must seed dataDir/bans/*.json (bkz. writeModCertFile)
// BEFORE constructing the *moderation.Moderator that will read it back via
// FindModCert.
func newSoloModTopicWithDataDir(t *testing.T, topicName, dataDir string) (*moderation.Moderator, *network.Topic, context.Context) {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New: %v", err)
	}
	t.Cleanup(func() { _ = node.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	ps, err := network.NewPubSub(ctx, node.Host)
	if err != nil {
		t.Fatalf("NewPubSub: %v", err)
	}
	topic, err := network.JoinTopic(ps, node.Host, topicName)
	if err != nil {
		t.Fatalf("JoinTopic: %v", err)
	}
	t.Cleanup(topic.Close)

	mod, err := moderation.New(dataDir, peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	return mod, topic, ctx
}

func TestProfilMetni_ShowsBanStatusForBannedAuthor(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (founder): %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	mod, topic, ctx := newSoloModTopic(t, "test/profilmetni-ban-status")

	targetPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (target): %v", err)
	}
	targetKeyHex := hex.EncodeToString(targetPub)

	if err := issueBanFor(ctx, mod, topic, founderIdentity, targetKeyHex, true, nil, "ısrarlı spam", banDurationPresets[len(banDurationPresets)-1]); err != nil {
		t.Fatalf("issueBanFor: %v", err)
	}

	out := profilMetni(80, nil, nil, nil, mod, targetKeyHex)
	if !strings.Contains(out, "⛔ Banlı") {
		t.Fatalf("banlı bir yazarın profilinde ban durumu gösterilmeliydi, got:\n%s", out)
	}
	if !strings.Contains(out, "Ömür boyu") {
		t.Fatalf("kalıcı ban için 'Ömür boyu' gösterilmeliydi, got:\n%s", out)
	}
	if !strings.Contains(out, "ısrarlı spam") {
		t.Fatalf("ban sebebi gösterilmeliydi, got:\n%s", out)
	}
}

func TestProfilMetni_NoBanStatusForUnbannedAuthor(t *testing.T) {
	founderIdentity, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (founder): %v", err)
	}
	withFounderKey(t, hex.EncodeToString(founderIdentity.PublicKey()))

	mod, _, _ := newSoloModTopic(t, "test/profilmetni-no-ban")

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	out := profilMetni(80, nil, nil, nil, mod, hex.EncodeToString(otherPub))
	if strings.Contains(out, "⛔ Banlı") {
		t.Fatalf("banlanmamış bir yazarın profilinde ban durumu gösterilmemeliydi, got:\n%s", out)
	}
}

func TestProfilMetni_NilModeratorDoesNotPanicOrShowBan(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	out := profilMetni(80, nil, nil, nil, nil, hex.EncodeToString(pub))
	if strings.Contains(out, "⛔ Banlı") {
		t.Fatal("mod nil'ken ban durumu gösterilmemeliydi")
	}
}
