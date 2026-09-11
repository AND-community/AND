package site

// sync_test.go — gerçek iki libp2p düğümü üzerinden uçtan uca sync testleri,
// internal/forum'un TestSyncWithPeer_TransfersPost'unun aynı deseni.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/storage"
)

func TestSyncWithPeer_TransfersPage(t *testing.T) {
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
	defer nodeA.Close()
	nodeB, err := network.New(idB, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (B): %v", err)
	}
	defer nodeB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	dbA, err := storage.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("storage.Open (A): %v", err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := storage.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("storage.Open (B): %v", err)
	}
	t.Cleanup(func() { _ = dbB.Close() })

	siteA, err := New(idA, nil, dbA)
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	siteB, err := New(idB, nil, dbB)
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	if err := siteA.PublishPage(ctx, "home", "Senkron Başlık", "Senkron İçerik"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}

	siteA.RegisterSync(nodeA.Host)

	if err := siteB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	myKeyA := hex.EncodeToString(idA.PublicKey())
	got := siteB.Get(myKeyA, "home")
	if got == nil {
		t.Fatal("SyncWithPeer sonrası B, A'daki sayfayı almalıydı")
	}
	if got.Title != "Senkron Başlık" || got.Body != "Senkron İçerik" {
		t.Fatalf("beklenmeyen sayfa içeriği: %+v", got)
	}
}

func TestSyncWithPeer_TransfersMultiplePagesAcrossAuthors(t *testing.T) {
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
	defer nodeA.Close()
	nodeB, err := network.New(idB, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (B): %v", err)
	}
	defer nodeB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	dbA, err := storage.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("storage.Open (A): %v", err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := storage.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("storage.Open (B): %v", err)
	}
	t.Cleanup(func() { _ = dbB.Close() })

	siteA, err := New(idA, nil, dbA)
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	siteB, err := New(idB, nil, dbB)
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	if err := siteA.PublishPage(ctx, "home", "Ana Sayfa", "gövde 1"); err != nil {
		t.Fatalf("PublishPage(home): %v", err)
	}
	if err := siteA.PublishPage(ctx, "projeler", "Projelerim", "gövde 2"); err != nil {
		t.Fatalf("PublishPage(projeler): %v", err)
	}
	// B kendi (farklı yazar) sayfasını zaten biliyor olsun — sync bunu
	// TEKRAR göndermemeli/silmemeli.
	if err := siteB.PublishPage(ctx, "b-sayfasi", "B'nin sayfası", "b içeriği"); err != nil {
		t.Fatalf("PublishPage(B): %v", err)
	}

	siteA.RegisterSync(nodeA.Host)
	if err := siteB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	myKeyA := hex.EncodeToString(idA.PublicKey())
	pages := siteB.PagesByAuthor(myKeyA)
	if len(pages) != 2 {
		t.Fatalf("A'nın 2 sayfası da senkronize olmalıydı, %d geldi: %+v", len(pages), pages)
	}

	myKeyB := hex.EncodeToString(idB.PublicKey())
	if got := siteB.Get(myKeyB, "b-sayfasi"); got == nil {
		t.Fatal("B'nin kendi sayfası sync sonrası KORUNMALIYDI")
	}
}

func TestSyncWithPeer_TransfersTombstone(t *testing.T) {
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
	defer nodeA.Close()
	nodeB, err := network.New(idB, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (B): %v", err)
	}
	defer nodeB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	dbA, err := storage.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("storage.Open (A): %v", err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := storage.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("storage.Open (B): %v", err)
	}
	t.Cleanup(func() { _ = dbB.Close() })

	siteA, err := New(idA, nil, dbA)
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	siteB, err := New(idB, nil, dbB)
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}

	// B önce A'nın sayfasını (gossip'ten geldiğini simüle ederek) zaten
	// bilsin — sonra A o sayfayı SİLER; B ile sync'te bu silme B'ye de
	// ULAŞMALI, sayfa geri gelmemeli.
	if err := siteA.PublishPage(ctx, "home", "t", "b"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	myKeyA := hex.EncodeToString(idA.PublicKey())
	pageJSON := siteA.Get(myKeyA, "home")
	siteB.handlePage(pageJSON, true)
	if siteB.Get(myKeyA, "home") == nil {
		t.Fatal("ön koşul: B, silmeden ÖNCE A'nın sayfasını bilmeliydi")
	}

	if err := siteA.DeletePage(ctx, "home"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	siteA.RegisterSync(nodeA.Host)
	if err := siteB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if got := siteB.Get(myKeyA, "home"); got != nil {
		t.Fatalf("A'nın sildiği sayfa, sync sonrası B'de de silinmiş olmalıydı, got %+v", got)
	}
}

func TestSyncWithPeer_UnreachablePeerReturnsError(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New: %v", err)
	}
	defer node.Close()

	db, err := storage.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(id, nil, db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	fakePeerID, err := moderation.AuthorKeyToPeerID(hex.EncodeToString(otherPub))
	if err != nil {
		t.Fatalf("AuthorKeyToPeerID: %v", err)
	}
	if err := s.SyncWithPeer(ctx, node.Host, fakePeerID); err == nil {
		t.Fatal("hiç bağlı olmayan/ulaşılamayan bir eşle sync hata döndürmeliydi")
	}
}
