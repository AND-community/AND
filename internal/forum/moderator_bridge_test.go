package forum

// moderator_bridge_test.go — SetModerator ile bağlanan bir *moderation.
// Moderator'ın, ağ-seviyesinde verdiği bir peer banının aynı kişinin forum
// içeriğine (AuthorKey üzerinden) de uygulandığını uçtan uca doğrular.
//
// AND'de bir düğümün TEK ed25519 anahtarı hem libp2p Peer ID'sini hem de
// forum/chat AuthorKey'ini üretir (bkz. moderation.AuthorKeyToPeerID'in doc
// yorumu) — bu test, banlanan peer'ın AuthorKey'i üzerinden gönderdiği bir
// postun artık kabul edilmediğini, gerçek bir moderasyon ban mesajının
// gerçek bir libp2p pubsub topic'i üzerinden yayınlanıp uygulanmasıyla
// (moderation paketinin kendi publish_test.go'sundaki desenin aynısı)
// doğrular.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestHandleMessage_PostFromModeratorBannedAuthorRejected(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (founder): %v", err)
	}
	oldFounder := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = oldFounder })

	modB, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

	// İki gerçek, birbirine bağlı libp2p düğümü — A moderasyon topic'ine
	// yayınlar, B (modB'nin dinlediği) alır. Tek düğümlü self-publish
	// gossipsub'da güvenilir olmadığından (bkz. moderation/publish_test.go)
	// bu desen kullanılıyor.
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
	modTopicA, err := network.JoinTopic(psA, nodeA.Host, "test/moderator-bridge-forum")
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(modTopicA.Close)
	modTopicB, err := network.JoinTopic(psB, nodeB.Host, "test/moderator-bridge-forum")
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(modTopicB.Close)
	time.Sleep(300 * time.Millisecond) // gossipsub mesh'inin oturması için

	modB.Start(ctx, modTopicB)

	// Banlanacak yazar: bu anahtar hem "peer" hem "forum AuthorKey" olarak
	// kullanılacak — gerçek AND kimlik modelinin aynısı.
	authorPub, authorPriv := genPostKey(t)
	authorKey := hex.EncodeToString(authorPub)
	targetPID, err := moderation.AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}

	// Moderatör sertifikası (kurucu tarafından imzalı) — andmod grant ile
	// aynı payload biçimi.
	modPub, modPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (mod): %v", err)
	}
	modKeyHex := hex.EncodeToString(modPub)
	certExpires := time.Now().Add(24 * time.Hour)
	cert := moderation.SignModeratorCert(founderPriv, modKeyHex, certExpires, false)

	// Ban mesajı (moderatör tarafından imzalı) — andmod ban ile aynı
	// payload biçimi, hedef: banlanacak yazarın peer.ID'si.
	banExpires := time.Now().Add(24 * time.Hour)
	banPayload := fmt.Sprintf("%s|%s|%d", targetPID.String(), "spam", banExpires.Unix())
	ban := moderation.BanMsg{
		BannedPeer: targetPID.String(),
		Reason:     "spam",
		ExpiresAt:  banExpires,
		Cert:       cert,
		Sig:        hex.EncodeToString(ed25519.Sign(modPriv, []byte(banPayload))),
	}

	if err := modB.PublishBan(ctx, modTopicA, ban); err != nil {
		t.Fatalf("PublishBan: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsBanned(targetPID) {
		if time.Now().After(deadline) {
			t.Fatal("ban, modTopicA üzerinden yayınlandıktan sonra modB'de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !modB.IsAuthorBanned(authorKey) {
		t.Fatal("peer banı, aynı ed25519 anahtardan türetilen AuthorKey için de banlı sayılmalıydı")
	}

	f, err := New(idB, nil, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("forum.New: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	f.SetModerator(modB)

	p := newSignedPost(authorPriv, authorPub, "Go", "Başlık", "İçerik")
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("moderatör tarafından banlanmış bir peer'ın AuthorKey'inden gelen (imzası geçerli olsa bile) bir post kabul edilmemeliydi")
	}
}

// TestSyncWithPeer_PostFromModeratorBannedAuthorRejected, aynı banın
// SENKRONİZASYON (SyncWithPeer) yolunda da uygulandığını doğrular —
// handleMessage (canlı GossipSub alımı) authorBanned'i kontrol ediyor diye
// sync'in de aynısını yaptığı varsayılamaz: bunlar ayrı kod yollarıdır ve
// biri diğerini otomatik olarak kapsamaz. Senaryo: A (dürüst ama banı
// bilmeyen/önemsemeyen bir eş) banlı yazarın postunu zaten yerelinde
// tutuyor; B (banı uygulayan moderatörün düğümü) A'ya senkronize olmaya
// çalışıyor — B'nin postu kabul ETMEMESİ gerekir, aksi halde ban
// senkronizasyon yoluyla süresiz olarak atlatılabilir.
func TestSyncWithPeer_PostFromModeratorBannedAuthorRejected(t *testing.T) {
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (founder): %v", err)
	}
	oldFounder := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = oldFounder })

	modB, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}

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
	modTopicA, err := network.JoinTopic(psA, nodeA.Host, "test/moderator-bridge-sync-forum")
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(modTopicA.Close)
	modTopicB, err := network.JoinTopic(psB, nodeB.Host, "test/moderator-bridge-sync-forum")
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(modTopicB.Close)
	time.Sleep(300 * time.Millisecond)

	modB.Start(ctx, modTopicB)

	authorPub, authorPriv := genPostKey(t)
	authorKey := hex.EncodeToString(authorPub)
	targetPID, err := moderation.AuthorKeyToPeerID(authorKey)
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}

	modPub, modPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (mod): %v", err)
	}
	modKeyHex := hex.EncodeToString(modPub)
	certExpires := time.Now().Add(24 * time.Hour)
	cert := moderation.SignModeratorCert(founderPriv, modKeyHex, certExpires, false)

	banExpires := time.Now().Add(24 * time.Hour)
	banPayload := fmt.Sprintf("%s|%s|%d", targetPID.String(), "spam", banExpires.Unix())
	ban := moderation.BanMsg{
		BannedPeer: targetPID.String(),
		Reason:     "spam",
		ExpiresAt:  banExpires,
		Cert:       cert,
		Sig:        hex.EncodeToString(ed25519.Sign(modPriv, []byte(banPayload))),
	}
	if err := modB.PublishBan(ctx, modTopicA, ban); err != nil {
		t.Fatalf("PublishBan: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !modB.IsAuthorBanned(authorKey) {
		if time.Now().After(deadline) {
			t.Fatal("ban modB'de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A, banı hiç bilmeyen/önemsemeyen dürüst bir eş — postu zaten yerelinde
	// tutuyor (ör. ban ağa yayılmadan önce almış).
	forumA, err := New(idA, nil, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("forum.New (A): %v", err)
	}
	t.Cleanup(func() { _ = forumA.db.Close() })
	forumA.RegisterSync(nodeA.Host)

	p := newSignedPost(authorPriv, authorPub, "Go", "Başlık", "İçerik")
	forumA.storePost(p)

	// B, banı uygulayan moderatörün düğümü — A'dan senkronize olmaya çalışıyor.
	forumB, err := New(idB, nil, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("forum.New (B): %v", err)
	}
	t.Cleanup(func() { _ = forumB.db.Close() })
	forumB.SetModerator(modB)

	if err := forumB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if forumB.PostByID(p.ID) != nil {
		t.Fatal("SyncWithPeer, moderatör tarafından banlanmış bir yazarın postunu kabul etmemeliydi")
	}
}
