package chatlog

// moderator_bridge_test.go — SetModerator ile bağlanan bir *moderation.
// Moderator'ın, ağ-seviyesinde verdiği bir peer banının aynı kişinin sohbet
// mesajlarına (AuthorKey üzerinden) da uygulandığını uçtan uca doğrular.
// Bkz. internal/forum/moderator_bridge_test.go'daki aynı desenin açıklaması.

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
	"github.com/lucian95511/and/internal/storage"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestHandleMessage_ChatFromModeratorBannedAuthorRejected(t *testing.T) {
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
	modTopicA, err := network.JoinTopic(psA, nodeA.Host, "test/moderator-bridge-chat")
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(modTopicA.Close)
	modTopicB, err := network.JoinTopic(psB, nodeB.Host, "test/moderator-bridge-chat")
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(modTopicB.Close)
	time.Sleep(300 * time.Millisecond)

	modB.Start(ctx, modTopicB)

	authorPub, authorPriv := genKey(t)
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
	for !modB.IsBanned(targetPID) {
		if time.Now().After(deadline) {
			t.Fatal("ban, modTopicA üzerinden yayınlandıktan sonra modB'de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !modB.IsAuthorBanned(authorKey) {
		t.Fatal("peer banı, aynı ed25519 anahtardan türetilen AuthorKey için de banlı sayılmalıydı")
	}

	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(idB, nil, db, "")
	if err != nil {
		t.Fatalf("chatlog.New: %v", err)
	}
	s.SetModerator(modB)

	m := signedMessage(authorPriv, authorPub, "merhaba", time.Now())
	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("moderatör tarafından banlanmış bir peer'ın AuthorKey'inden gelen (imzası geçerli olsa bile) bir mesaj kabul edilmemeliydi, got %+v", all)
	}
}

// TestSyncWithPeer_ChatFromModeratorBannedAuthorRejected, aynı banın
// SENKRONİZASYON (SyncWithPeer) yolunda da uygulandığını doğrular — bkz.
// internal/forum/moderator_bridge_test.go'daki eşdeğer testin doc yorumu:
// handleMessage'ın authorBanned'i kontrol etmesi, ayrı bir kod yolu olan
// sync'in de aynısını yaptığını garanti etmez. Senaryo: A (dürüst ama banı
// bilmeyen bir eş) banlı yazarın mesajını zaten yerelinde tutuyor; B (banı
// uygulayan moderatörün düğümü) A'ya senkronize olmaya çalışıyor — B'nin
// mesajı kabul ETMEMESİ gerekir, aksi halde ban senkronizasyon yoluyla
// süresiz olarak atlatılabilir.
func TestSyncWithPeer_ChatFromModeratorBannedAuthorRejected(t *testing.T) {
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
	modTopicA, err := network.JoinTopic(psA, nodeA.Host, "test/moderator-bridge-sync-chat")
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(modTopicA.Close)
	modTopicB, err := network.JoinTopic(psB, nodeB.Host, "test/moderator-bridge-sync-chat")
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(modTopicB.Close)
	time.Sleep(300 * time.Millisecond)

	modB.Start(ctx, modTopicB)

	authorPub, authorPriv := genKey(t)
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

	// A, banı hiç bilmeyen/önemsemeyen dürüst bir eş — mesajı zaten
	// yerelinde tutuyor (ör. ban ağa yayılmadan önce almış).
	dbA, err := storage.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("storage.Open (A): %v", err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	storeA, err := New(idA, nil, dbA, "")
	if err != nil {
		t.Fatalf("chatlog.New (A): %v", err)
	}
	storeA.RegisterSync(nodeA.Host)

	m := signedMessage(authorPriv, authorPub, "merhaba", time.Now())
	storeA.persist(m)

	// B, banı uygulayan moderatörün düğümü — A'dan senkronize olmaya çalışıyor.
	dbB, err := storage.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("storage.Open (B): %v", err)
	}
	t.Cleanup(func() { _ = dbB.Close() })
	storeB, err := New(idB, nil, dbB, "")
	if err != nil {
		t.Fatalf("chatlog.New (B): %v", err)
	}
	storeB.SetModerator(modB)

	if err := storeB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if all := storeB.All(); len(all) != 0 {
		t.Fatalf("SyncWithPeer, moderatör tarafından banlanmış bir yazarın mesajını kabul etmemeliydi, got %+v", all)
	}
}
