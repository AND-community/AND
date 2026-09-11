package forum

// sync_test.go — handleSyncStream (sync isteğini YANITLAYAN taraf) ve
// SyncWithPeer'ın (sync isteğini GÖNDEREN tarafın yanıt işleme kodu)
// tombstone (silme) işleme yollarını, canlı gossip yolundaki (handleMessage)
// muted()/authorBanned() kontrolleriyle aynı şekilde uyguladığını doğrular.
//
// Bu iki kod yolu, aynı DeleteMsg tipini işlese de handleMessage'dan
// TAMAMEN AYRIDIR — biri diğerini otomatik olarak kapsamaz. Susturulmuş
// (ya da ağdan banlı) bir yazarın kendi-imzaladığı, aksi halde geçerli bir
// silme talebi, canlı gossip'te reddedilirken sync üzerinden sessizce
// uygulanabiliyorsa, ban/mute korumaları sync yoluyla süresiz atlatılabilir.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

// muteAuthor, authorKey'i f üzerinde YEREL olarak susturur (recordViolation'ı
// muteViolationThreshold kadar tetikleyerek) — bkz. rateLimiter.recordViolation
// ve rateLimiter.muted'in doc yorumları. Gerçek bir ban/moderasyon akışı
// gerektirmez; bu testlerin odağı olan muted() kontrolünü izole eder.
func muteAuthor(f *Forum, authorKey string) {
	for i := 0; i < muteViolationThreshold; i++ {
		f.rl.recordViolation(authorKey)
	}
}

func TestHandleSyncStream_TombstoneFromMutedAuthorRejected(t *testing.T) {
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

	// A = kurbanın (sync isteğini YANITLAYAN) düğümü — postu zaten elinde,
	// ve yazarı yerel olarak susturmuş durumda.
	forumA, err := New(idA, nil, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	t.Cleanup(func() { _ = forumA.db.Close() })
	forumA.RegisterSync(nodeA.Host)

	// B = susturulmuş yazarın kendi düğümü — postu ve KENDİ imzaladığı
	// silme (tombstone) talebini elinde tutuyor.
	forumB, err := New(idB, nil, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	t.Cleanup(func() { _ = forumB.db.Close() })

	authorPub, authorPriv := genPostKey(t)
	authorKey := hex.EncodeToString(authorPub)
	p := newSignedPost(authorPriv, authorPub, "Go", "Başlık", "İçerik")

	forumA.storePost(p)
	muteAuthor(forumA, authorKey)
	if !forumA.rl.muted(authorKey) {
		t.Fatal("test kurulumu: yazar A üzerinde susturulmuş olmalıydı")
	}

	// B, kendi postunu ekleyip GERÇEK (kalıcı, db'ye yazılmış) bir
	// tombstone üretmek için kendi imzasıyla siler — applyDelete yalnızca
	// post ZATEN biliniyorsa kalıcı bir tombstone yazar (bkz. onun doc
	// yorumu), bu yüzden önce storePost gerekiyor.
	forumB.storePost(p)
	d := &DeleteMsg{PostID: p.ID, AuthorKey: authorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(authorPriv, deleteMsgPayload(d)))
	forumB.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))
	if forumB.PostByID(p.ID) != nil {
		t.Fatal("test kurulumu: B kendi postunu silmiş olmalıydı")
	}

	// B, A'ya senkronize OLMAYA çalışıyor (SyncWithPeer'ı B çağırıyor) —
	// bu, B'nin tombstone'unu bir sync İSTEĞİ içinde A'ya gönderir; A'nın
	// handleSyncStream'i bunu işler.
	if err := forumB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if forumA.PostByID(p.ID) == nil {
		t.Fatal("A'daki post, YEREL OLARAK SUSTURULMUŞ bir yazarın sync isteği İÇİNDEKİ tombstone'u tarafından silinmemeliydi")
	}
}

func TestSyncWithPeer_TombstoneFromMutedAuthorRejected(t *testing.T) {
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

	// B = dürüst bir röle düğümü — postu ve yazarın kendi imzaladığı
	// (geçerli) silme talebini elinde tutuyor, sync isteklerini YANITLIYOR.
	forumB, err := New(idB, nil, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	t.Cleanup(func() { _ = forumB.db.Close() })
	forumB.RegisterSync(nodeB.Host)

	// A = senkronize OLAN (SyncWithPeer'ı çağıran) düğüm — postu zaten
	// elinde ve yazarı YEREL olarak susturmuş durumda.
	forumA, err := New(idA, nil, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	t.Cleanup(func() { _ = forumA.db.Close() })

	authorPub, authorPriv := genPostKey(t)
	authorKey := hex.EncodeToString(authorPub)
	p := newSignedPost(authorPriv, authorPub, "Go", "Başlık", "İçerik")

	forumA.storePost(p)
	muteAuthor(forumA, authorKey)
	if !forumA.rl.muted(authorKey) {
		t.Fatal("test kurulumu: yazar A üzerinde susturulmuş olmalıydı")
	}

	forumB.storePost(p)
	d := &DeleteMsg{PostID: p.ID, AuthorKey: authorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(authorPriv, deleteMsgPayload(d)))
	forumB.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))
	if forumB.PostByID(p.ID) != nil {
		t.Fatal("test kurulumu: B kendi postunu silmiş olmalıydı")
	}

	// A, B'ye senkronize oluyor — B'nin YANITINDAKİ tombstone'u A'nın
	// SyncWithPeer'ı işler.
	if err := forumA.SyncWithPeer(ctx, nodeA.Host, nodeB.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if forumA.PostByID(p.ID) == nil {
		t.Fatal("A'daki post, YEREL OLARAK SUSTURULMUŞ bir yazarın sync YANITINDAKİ tombstone'u tarafından silinmemeliydi")
	}
}
