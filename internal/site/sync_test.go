package site

// sync_test.go — gerçek iki libp2p düğümü üzerinden uçtan uca sync testleri,
// internal/forum'un TestSyncWithPeer_TransfersPost'unun aynı deseni.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"math"
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

// ─── Sürüm farkına duyarlı senkron (KnownVersions) ───────────────────────────

// newSyncPair, birbirine bağlı iki gerçek libp2p düğümü ve her biri için
// ayrı veritabanlı birer Store döner — yukarıdaki testlerin her birinde
// tekrarlanan kurulumun, aşağıdaki testler için ortak hali.
func newSyncPair(t *testing.T) (ctx context.Context, siteA, siteB *Store, nodeA, nodeB *network.Node) {
	t.Helper()
	idA, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (A): %v", err)
	}
	idB, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (B): %v", err)
	}
	nodeA, err = network.New(idA, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (A): %v", err)
	}
	t.Cleanup(func() { _ = nodeA.Close() })
	nodeB, err = network.New(idB, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New (B): %v", err)
	}
	t.Cleanup(func() { _ = nodeB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	for _, side := range []struct {
		id  *stdcrypto.Identity
		out **Store
	}{{idA, &siteA}, {idB, &siteB}} {
		db, err := storage.Open(filepath.Join(t.TempDir(), "site.db"))
		if err != nil {
			t.Fatalf("storage.Open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		s, err := New(side.id, nil, db)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		*side.out = s
	}
	return ctx, siteA, siteB, nodeA, nodeB
}

// TestSyncWithPeer_TransfersEditedPage, bir sayfanın DÜZENLENMİŞ halinin de
// senkronla geriye dönük yakalandığını doğrular: B bir sayfanın yalnızca
// ESKİ sürümünü bilirken (ör. düzenleme canlı gossip'te minPageEditInterval
// yüzünden düştü ya da B o sırada çevrimdışıydı) A ile sync, B'ye yeni
// sürümü getirmeli. KnownVersions'tan önce yanıtlayan "B bu slug'ı zaten
// biliyor" diye sayfayı hiç göndermiyordu — B eski sürümde sonsuza dek
// kalıyordu.
func TestSyncWithPeer_TransfersEditedPage(t *testing.T) {
	ctx, siteA, siteB, nodeA, nodeB := newSyncPair(t)

	pub, priv := genKey(t)
	now := time.Now()
	v1 := signedPage(priv, pub, "home", "ilk başlık", "ilk gövde", now)
	v2 := signedPage(priv, pub, "home", "düzenlenmiş başlık", "düzenlenmiş gövde", now.Add(time.Second))
	siteA.handlePage(v2, true)
	siteB.handlePage(v1, true)

	siteA.RegisterSync(nodeA.Host)
	if err := siteB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	got := siteB.Get(v1.AuthorKey, "home")
	if got == nil || got.Title != "düzenlenmiş başlık" || got.Body != "düzenlenmiş gövde" {
		t.Fatalf("sync sonrası B, sayfanın A'daki DÜZENLENMİŞ sürümüne geçmeliydi, got %+v", got)
	}
}

// TestSyncWithPeer_KeepsNewerLocalVersion, tersi yönün güvenliğini
// doğrular: B zaten DAHA YENİ bir sürüme sahipken, eski sürümü tutan A ile
// sync B'nin sürümünü geri almamalı.
func TestSyncWithPeer_KeepsNewerLocalVersion(t *testing.T) {
	ctx, siteA, siteB, nodeA, nodeB := newSyncPair(t)

	pub, priv := genKey(t)
	now := time.Now()
	v1 := signedPage(priv, pub, "home", "eski", "eski gövde", now)
	v2 := signedPage(priv, pub, "home", "yeni", "yeni gövde", now.Add(time.Second))
	siteA.handlePage(v1, true)
	siteB.handlePage(v2, true)

	siteA.RegisterSync(nodeA.Host)
	if err := siteB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if got := siteB.Get(v2.AuthorKey, "home"); got == nil || got.Body != "yeni gövde" {
		t.Fatalf("B'nin daha yeni sürümü, eski sürümü tutan bir eşle sync sonrası korunmalıydı, got %+v", got)
	}
}

func TestSyncResponseFor_SendsNewerVersionOfKnownSlug(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()
	v1 := signedPage(priv, pub, "home", "eski", "eski gövde", now)
	v2 := signedPage(priv, pub, "home", "yeni", "yeni gövde", now.Add(time.Second))
	env.store.handlePage(v2, true)

	resp := env.store.syncResponseFor(syncRequest{
		Known:         map[string][]string{v1.AuthorKey: {"home"}},
		KnownVersions: map[string]map[string]int64{v1.AuthorKey: {"home": v1.UpdatedAt.UnixNano()}},
	})
	if len(resp.Pages) != 1 || resp.Pages[0].Body != "yeni gövde" {
		t.Fatalf("istekçi slug'ın ESKİ bir sürümünü bildirdiğinde yeni sürüm gönderilmeliydi, got %+v", resp.Pages)
	}
}

func TestSyncResponseFor_SkipsKnownSlugAtSameOrNewerVersion(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", "b", time.Now())
	env.store.handlePage(p, true)

	for _, v := range []int64{p.UpdatedAt.UnixNano(), p.UpdatedAt.Add(time.Second).UnixNano()} {
		resp := env.store.syncResponseFor(syncRequest{
			Known:         map[string][]string{p.AuthorKey: {"home"}},
			KnownVersions: map[string]map[string]int64{p.AuthorKey: {"home": v}},
		})
		if len(resp.Pages) != 0 {
			t.Fatalf("istekçi aynı ya da daha yeni sürümü bildirdiğinde sayfa tekrar gönderilmemeliydi (v=%d), got %+v", v, resp.Pages)
		}
	}
}

// TestSyncResponseFor_LegacyRequestWithoutVersionsSkipsKnownSlugs, geriye
// dönük uyumluluğu doğrular: KnownVersions'ı hiç göndermeyen ESKİ bir
// istekçinin bildiği slug, eskisi gibi güncel sayılır — yanıt değişmez.
func TestSyncResponseFor_LegacyRequestWithoutVersionsSkipsKnownSlugs(t *testing.T) {
	env := newTestStore(t)
	pub, priv := genKey(t)
	p := signedPage(priv, pub, "home", "t", "b", time.Now())
	env.store.handlePage(p, true)

	resp := env.store.syncResponseFor(syncRequest{Known: map[string][]string{p.AuthorKey: {"home"}}})
	if len(resp.Pages) != 0 {
		t.Fatalf("KnownVersions'sız eski bir istekte bilinen slug atlanmalıydı, got %+v", resp.Pages)
	}
	if resp = env.store.syncResponseFor(syncRequest{}); len(resp.Pages) != 1 {
		t.Fatalf("istekçinin hiç bilmediği sayfa her durumda gönderilmeliydi, got %+v", resp.Pages)
	}
}

// TestEncodeSyncRequest_DropsVersionsOverLimit, ESKİ bir yanıtlayıcının
// maxSyncRequestBytes'ını aşacak bir istekte yalnızca KnownVersions'ın
// düşürüldüğünü, Known/KnownTombstones'un korunduğunu doğrular.
func TestEncodeSyncRequest_DropsVersionsOverLimit(t *testing.T) {
	req := syncRequest{
		Known:           map[string][]string{"ab": {"home"}},
		KnownVersions:   map[string]map[string]int64{"ab": {"home": 42}},
		KnownTombstones: []string{"cd/eski"},
	}
	full, err := encodeSyncRequest(req, maxSyncRequestBytes)
	if err != nil {
		t.Fatalf("encodeSyncRequest: %v", err)
	}
	var got syncRequest
	if err := json.Unmarshal(full, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.KnownVersions["ab"]["home"] != 42 {
		t.Fatalf("sınırın altındaki istek sürümleri taşımalıydı: %s", full)
	}

	trimmed, err := encodeSyncRequest(req, len(full)-1)
	if err != nil {
		t.Fatalf("encodeSyncRequest: %v", err)
	}
	got = syncRequest{}
	if err := json.Unmarshal(trimmed, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.KnownVersions != nil || len(got.Known["ab"]) != 1 || len(got.KnownTombstones) != 1 {
		t.Fatalf("sınırı aşan istekte yalnızca KnownVersions düşmeliydi, got %+v", got)
	}
}

// TestVersionOf_OutOfRangeTimestampFallsBackToLegacy, UnixNano'nun
// tanımsız olduğu bir UpdatedAt'in (imzalayan yazar 1678 öncesini
// seçebilir — handlePage yalnızca ÜSTTEN sınırlar) sürüm olarak hiç
// kullanılmadığını, sayfanın eski "bilinen slug = güncel" kuralıyla
// ele alındığını doğrular.
func TestVersionOf_OutOfRangeTimestampFallsBackToLegacy(t *testing.T) {
	ancient := &Page{Slug: "home", UpdatedAt: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)}
	if _, ok := versionOf(ancient); ok {
		t.Fatal("int64 nanosaniye aralığı dışındaki bir zaman sürüm olarak kullanılmamalıydı")
	}
	if newerThanKnown(ancient, map[string]int64{"home": math.MinInt64}) {
		t.Fatal("sürümü hesaplanamayan bir sayfa 'daha yeni' sayılmamalıydı")
	}

	normal := &Page{Slug: "home", UpdatedAt: time.Now()}
	if v, ok := versionOf(normal); !ok || v != normal.UpdatedAt.UnixNano() {
		t.Fatalf("olağan bir zaman UnixNano olarak dönmeliydi, got %d, %v", v, ok)
	}
}
