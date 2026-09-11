package moderation

// publish_test.go — PublishSavedBans'in gerçek bir libp2p pubsub topic'i
// üzerinden uçtan uca (dosya yaz → yayınla → başka bir düğüm al → uygula)
// davranışını kapsar. Bu, önceden var olan kritik bir bug için regresyon
// testidir: andmod ban/andmod grant tarafından bans/ dizinine yazılan
// dosyalar, PublishSavedBans eklenmeden önce Envelope'a sarmalanmadan
// doğrudan ham baytlar olarak yayınlanıyordu — handleEnvelope ise her
// mesajın {"type":...} alanı beklediğinden, bu dosyalar ağda hiçbir zaman
// (kendi düğümü dahil) hiçbir etki yaratmıyordu.
//
// Tek düğümlü self-publish (yayıncı düğümün topic'e hiç bağlı peer'ı
// olmadan kendi mesajını geri alması) gossipsub'da güvenilir değil — bu
// yüzden forum_test.go'daki TestSyncWithPeer_TransfersPost ile aynı iki-
// düğüm deseni kullanılıyor: A yayınlar, B (A'ya bağlı, aynı topic'e abone)
// alır ve uygular.

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

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

// twoConnectedTopics, aynı isimde bir topic'e abone, birbirine bağlı iki
// gerçek libp2p düğümü kurar. topicA üzerine yayınlanan bir mesaj, kısa bir
// gecikmeyle topicB'nin Messages() kanalında görünür.
func twoConnectedTopics(t *testing.T, topicName string) (topicA, topicB *network.Topic, ctx context.Context) {
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

	// GossipSub mesh'inin (peer keşfi + grafting) oturması için kısa bir süre.
	time.Sleep(300 * time.Millisecond)

	return topicA, topicB, ctx
}

func TestPublishSavedBans_RawBanFileIsAppliedAfterPublish(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	dataDirA := t.TempDir()
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dataDirA, peer.ID(""))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	modB, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	topicA, topicB, ctx := twoConnectedTopics(t, "test/publish-saved-bans-1")
	modB.Start(ctx, topicB)
	t.Cleanup(modB.Stop)

	modPub, modPriv := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)
	target := randomPeerID(t)
	ban := signedBan(t, modPriv, cert, target, "spam")

	// andmod ban'ın gerçekte diske yazdığıyla AYNI biçim: sarmalanmamış, ham
	// BanMsg JSON'u — bkz. cmd/andmod/main.go cmdBan().
	bansDir := filepath.Join(dataDirA, "bans")
	if err := os.MkdirAll(bansDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.MarshalIndent(ban, "", "  ")
	if err := os.WriteFile(filepath.Join(bansDir, "ban_test.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	modA.PublishSavedBans(ctx, topicA, dataDirA)

	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsBanned(target) {
		if time.Now().After(deadline) {
			t.Fatal("bans/ altındaki ham BanMsg dosyası, PublishSavedBans üzerinden yayınlandıktan sonra diğer düğümde uygulanmalıydı (Envelope sarmalaması eksikse burası hiç gerçekleşmez)")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPublishSavedBans_RawRevokeFileIsAppliedAfterPublish(t *testing.T) {
	founderPub, founderPriv := genKey(t)
	dataDirA := t.TempDir()
	old := FounderPubKeyHex
	FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { FounderPubKeyHex = old })

	modA, err := New(dataDirA, peer.ID(""))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	modB, err := New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	topicA, topicB, ctx := twoConnectedTopics(t, "test/publish-saved-bans-2")
	modB.Start(ctx, topicB)
	t.Cleanup(modB.Stop)

	modPub, _ := genKey(t)
	modKey := hex.EncodeToString(modPub)
	issuedAt := time.Now().UTC()
	payload := fmt.Sprintf("revoke|%s|%d", modKey, issuedAt.Unix())
	sig := ed25519.Sign(founderPriv, []byte(payload))
	rev := RevokeMsg{ModeratorKey: modKey, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}

	bansDir := filepath.Join(dataDirA, "bans")
	if err := os.MkdirAll(bansDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.MarshalIndent(rev, "", "  ")
	if err := os.WriteFile(filepath.Join(bansDir, "revoke_test.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	modA.PublishSavedBans(ctx, topicA, dataDirA)

	deadline := time.Now().Add(10 * time.Second)
	for {
		modB.mu.RLock()
		_, revoked := modB.revoked[modKey]
		modB.mu.RUnlock()
		if revoked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bans/ altındaki ham RevokeMsg dosyası, PublishSavedBans üzerinden yayınlandıktan sonra diğer düğümde uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPublishSavedBans_SkipsModeratorCertFiles(t *testing.T) {
	_, founderPriv := genKey(t)
	dataDirA := t.TempDir()

	modA, err := New(dataDirA, peer.ID(""))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}

	topicA, topicB, ctx := twoConnectedTopics(t, "test/publish-saved-bans-3")

	modPub, _ := genKey(t)
	cert := signedModCert(t, founderPriv, modPub)

	bansDir := filepath.Join(dataDirA, "bans")
	if err := os.MkdirAll(bansDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, _ := json.MarshalIndent(cert, "", "  ")
	if err := os.WriteFile(filepath.Join(bansDir, "mod_test.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	received := make(chan []byte, 1)
	go func() {
		for data := range topicB.Messages(ctx) {
			select {
			case received <- data:
			default:
			}
		}
	}()

	modA.PublishSavedBans(ctx, topicA, dataDirA)

	select {
	case data := <-received:
		t.Fatalf("mod_*.json (moderatör sertifikası) topic'e yayınlanmamalıydı, ama şu veri geldi: %s", data)
	case <-time.After(2 * time.Second):
		// beklenen: hiçbir şey yayınlanmadı
	}
}
