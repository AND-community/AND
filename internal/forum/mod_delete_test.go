package forum

// mod_delete_test.go — ModDeleteMsg (moderatör/kurucu tarafından herhangi
// bir konunun ağ genelinde, tombstone'lu silinmesi) için testler. Bu
// mekanizma, önceden var olan DeletePost'un (yalnızca yerel, ağa hiç
// yayılmayan, senkronizasyonda geri gelen) yerini "gerçek" bir moderasyon
// aracı olarak alır — bkz. applyModDelete/IssueModDelete'in doc yorumları.

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

// withTestFounderKey generates a fresh founder keypair and points
// moderation.FounderPubKeyHex at it for the duration of the test — must be
// called BEFORE newTestForumWithModerator/moderation.New, which read the
// global at construction time.
func withTestFounderKey(t *testing.T) (founderPub ed25519.PublicKey, founderPriv ed25519.PrivateKey) {
	t.Helper()
	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (founder): %v", err)
	}
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })
	return founderPub, founderPriv
}

// newTestForumWithModerator, newTestForum'a ek olarak bağımsız bir
// *moderation.Moderator bağlar.
func newTestForumWithModerator(t *testing.T) *Forum {
	t.Helper()
	f := newTestForum(t)
	mod, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	f.SetModerator(mod)
	return f
}

// storeTestPost doğrudan f.storePost üzerinden (imza/hız sınırı kontrolü
// atlanarak) bir post ekler — bu dosyadaki testler ModDeleteMsg'i test
// ediyor, post oluşturmayı değil.
func storeTestPost(f *Forum, authorPub ed25519.PublicKey, authorPriv ed25519.PrivateKey) *Post {
	p := newSignedPost(authorPriv, authorPub, "Genel", "başlık", "gövde")
	f.storePost(p)
	return p
}

// newSoloTopic, tek bir gerçek libp2p düğümü + topic'i döner — yalnızca
// PublishX çağrılarının (gerçekte hiçbir alıcı olmasa da) hata vermeden
// çalışabilmesi için gerekli.
func newSoloTopic(t *testing.T, topicName string) (*network.Topic, context.Context) {
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
	return topic, ctx
}

// ─── applyModDelete: kurucu / moderatör yolu ────────────────────────────────

func TestApplyModDelete_FounderCanDeleteAnyPost(t *testing.T) {
	_, founderPriv := withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)
	p := storeTestPost(f, authorPub, authorPriv)

	d := SignFounderDelete(founderPriv, p.ID, "kötüye kullanım")
	if !f.applyModDelete(&d) {
		t.Fatal("kurucu silmesi kabul edilmeliydi")
	}
	if f.PostByID(p.ID) != nil {
		t.Fatal("post silindikten sonra hâlâ bulunuyor")
	}
	f.mu.RLock()
	_, tombstoned := f.deleted[p.ID]
	f.mu.RUnlock()
	if !tombstoned {
		t.Fatal("silme bir tombstone bırakmalıydı")
	}
}

func TestApplyModDelete_ModeratorCertCanDeleteAnyPost(t *testing.T) {
	_, founderPriv := withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)
	p := storeTestPost(f, authorPub, authorPriv)

	modPub, modPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (mod): %v", err)
	}
	cert := moderation.SignModeratorCert(founderPriv, hex.EncodeToString(modPub), time.Now().Add(24*time.Hour), false)

	d := SignModeratorDelete(modPriv, cert, p.ID, "spam")
	if !f.applyModDelete(&d) {
		t.Fatal("geçerli sertifikalı moderatör silmesi kabul edilmeliydi")
	}
	if f.PostByID(p.ID) != nil {
		t.Fatal("post silindikten sonra hâlâ bulunuyor")
	}
}

func TestApplyModDelete_WrongFounderKeyRejected(t *testing.T) {
	withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)
	p := storeTestPost(f, authorPub, authorPriv)

	_, wrongPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	d := SignFounderDelete(wrongPriv, p.ID, "spam")
	if f.applyModDelete(&d) {
		t.Fatal("gerçek kurucunun anahtarıyla imzalanmamış bir silme kabul edilmemeliydi")
	}
	if f.PostByID(p.ID) == nil {
		t.Fatal("reddedilen bir silmeden sonra post hâlâ var olmalıydı")
	}
}

func TestApplyModDelete_RevokedCertRejected(t *testing.T) {
	_, founderPriv := withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)
	p := storeTestPost(f, authorPub, authorPriv)

	modPub, modPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (mod): %v", err)
	}
	modKeyHex := hex.EncodeToString(modPub)
	cert := moderation.SignModeratorCert(founderPriv, modKeyHex, time.Now().Add(24*time.Hour), false)

	mod := f.mod.Load()
	if !mod.VerifyModeratorCert(cert) {
		t.Fatal("kurulum: sertifika iptalden önce geçerli olmalıydı")
	}
	topic, ctx := newSoloTopic(t, "test/forum-mod-delete-revoke")
	rev := moderation.SignRevoke(founderPriv, modKeyHex)
	if err := mod.IssueRevoke(ctx, topic, rev); err != nil {
		t.Fatalf("IssueRevoke: %v", err)
	}

	d := SignModeratorDelete(modPriv, cert, p.ID, "spam")
	if f.applyModDelete(&d) {
		t.Fatal("iptal edilmiş bir sertifikayla silme kabul edilmemeliydi")
	}
	if f.PostByID(p.ID) == nil {
		t.Fatal("reddedilen bir silmeden sonra post hâlâ var olmalıydı")
	}
}

// ─── Önleyici (pre-emptive) tombstone: asıl regresyon ───────────────────────

// TestApplyModDelete_PreemptiveTombstonePreventsLaterPostFromAppearing,
// admin panelinin eski DeletePost'unun sahip olduğu tam açığı doğrudan
// test eder: bir moderatör bir postID'yi henüz o post yerel olarak
// bilinmeden silerse (ör. ağdaki mod_delete mesajı postun kendisinden önce
// ulaşırsa), post daha SONRA gelse bile asla saklanmamalı.
func TestApplyModDelete_PreemptiveTombstonePreventsLaterPostFromAppearing(t *testing.T) {
	_, founderPriv := withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)
	p := newSignedPost(authorPriv, authorPub, "Genel", "başlık", "gövde")

	// Post f'de HENÜZ bilinmiyor — doğrudan mod-delete uygula.
	d := SignFounderDelete(founderPriv, p.ID, "önceden engellendi")
	if !f.applyModDelete(&d) {
		t.Fatal("bilinmeyen bir post için önleyici silme kabul edilmeliydi")
	}

	// Şimdi gerçek post "gelsin" (canlı gossip'i simüle eden storePost).
	f.storePost(p)

	if f.PostByID(p.ID) != nil {
		t.Fatal("önceden tombstone'lanmış bir post, sonradan gelince yine de saklanmış — eski hata geri döndü")
	}
}

// ─── Hız sınırı ──────────────────────────────────────────────────────────────

func TestApplyModDelete_RateLimited(t *testing.T) {
	_, founderPriv := withTestFounderKey(t)
	f := newTestForumWithModerator(t)

	authorPub, authorPriv := genPostKey(t)

	ok := 0
	for i := 0; i < rlMaxDeletes+5; i++ {
		// Başlık indeksle değişmeli: applyModDelete artık zaten uygulanmış
		// bir tombstone'u (aynı postID) hız sınırından erken çıkarıyor
		// (bkz. applyModDelete'in "already" kontrolü) — sabit başlık/gövde
		// kullanılsaydı, bu döngüdeki ardışık storeTestPost çağrıları aynı
		// saniye/nanosaniye içinde çalışıp CreatedAt'i aynı üretebilir ve bu
		// da derivePostID'nin (CreatedAt dahil) aynı postID'yi üretmesine,
		// dolayısıyla testin gerçekte kaç FARKLI silme denediğini
		// ölçmemesine yol açardı.
		p := newSignedPost(authorPriv, authorPub, "Genel", fmt.Sprintf("başlık %d", i), "gövde")
		f.storePost(p)
		d := SignFounderDelete(founderPriv, p.ID, "spam")
		if f.applyModDelete(&d) {
			ok++
		}
	}
	if ok != rlMaxDeletes {
		t.Fatalf("rlMaxDeletes (%d) sonrası silmeler reddedilmeliydi, got %d başarılı", rlMaxDeletes, ok)
	}
}

// ─── IssueModDelete: gerçek ağ üzerinden uçtan uca ──────────────────────────

func TestIssueModDelete_FounderPath_AppliesLocallyAndPublishes(t *testing.T) {
	// idA'nın kendisi kurucu kimliği — IssueModDelete'in isFounder=true
	// yolu fA.identity (=idA) ile imzalar, bu yüzden doğrulamanın
	// geçmesi için moderation.FounderPubKeyHex idA'nın public key'i
	// olmalı.
	idA, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (A): %v", err)
	}
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(idA.PublicKey())
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })

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
	topicA, err := network.JoinTopic(psA, nodeA.Host, "test/forum-mod-delete-issue")
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	t.Cleanup(topicA.Close)
	topicB, err := network.JoinTopic(psB, nodeB.Host, "test/forum-mod-delete-issue")
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	t.Cleanup(topicB.Close)
	// Sabit bir süre uyumak yerine iki tarafın birbirinin aboneliğini görmesi
	// beklenir: yük altında GossipSub aboneliği o süreden geç duyurabilir.
	if err := topicA.AboneyiBekle(ctx, nodeB.Host.ID()); err != nil {
		t.Fatal(err)
	}
	if err := topicB.AboneyiBekle(ctx, nodeA.Host.ID()); err != nil {
		t.Fatal(err)
	}

	fA, err := New(idA, topicA, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	t.Cleanup(func() { _ = fA.db.Close() })
	modA, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (A): %v", err)
	}
	fA.SetModerator(modA)

	fB, err := New(idB, topicB, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	t.Cleanup(func() { _ = fB.db.Close() })
	modB, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (B): %v", err)
	}
	fB.SetModerator(modB)
	go fB.Run(ctx)

	authorPub, authorPriv := genPostKey(t)
	p := newSignedPost(authorPriv, authorPub, "Genel", "başlık", "gövde")
	fA.storePost(p)
	fB.storePost(p)

	if err := fA.IssueModDelete(ctx, p.ID, "kötüye kullanım", true, nil); err != nil {
		t.Fatalf("IssueModDelete: %v", err)
	}
	if fA.PostByID(p.ID) != nil {
		t.Fatal("IssueModDelete, kendi düğümünde (A) hemen etkili olmalıydı")
	}

	deadline := time.Now().Add(10 * time.Second)
	for fB.PostByID(p.ID) != nil {
		if time.Now().After(deadline) {
			t.Fatal("silme, IssueModDelete üzerinden yayınlandıktan sonra B'de de uygulanmalıydı")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSyncWithPeer_ModDeleteTombstonePropagates, admin panelinin ESKİ
// DeletePost'unun senkronda "geri gelen" post sorununu doğrudan ağ
// senkronu üzerinden doğrular: A bir postu mod-delete ile siler (kalıcı
// tombstone), B hâlâ postu elinde tutuyor; B, A'dan senkronize ettiğinde
// A'nın tombstone'unu almalı ve postu KENDİ tarafında da silmeli.
func TestSyncWithPeer_ModDeleteTombstonePropagates(t *testing.T) {
	idA, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity (A): %v", err)
	}
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(idA.PublicKey())
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := nodeA.Host.Connect(ctx, peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	forumA, err := New(idA, nil, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	t.Cleanup(func() { _ = forumA.db.Close() })
	modA, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (A): %v", err)
	}
	forumA.SetModerator(modA)

	forumB, err := New(idB, nil, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	t.Cleanup(func() { _ = forumB.db.Close() })
	modB, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New (B): %v", err)
	}
	forumB.SetModerator(modB)

	authorPub, authorPriv := genPostKey(t)
	p := newSignedPost(authorPriv, authorPub, "Go", "spam başlık", "spam içerik")
	forumA.storePost(p)
	forumB.storePost(p) // B, A henüz silmeden ÖNCE bu postu zaten almış

	d := SignFounderDelete(idA.PrivateKey(), p.ID, "spam")
	if !forumA.applyModDelete(&d) {
		t.Fatal("kurulum: mod-delete A'da uygulanmalıydı")
	}
	if forumB.PostByID(p.ID) == nil {
		t.Fatal("kurulum: B senkrondan önce hâlâ postu elinde tutmalıydı")
	}

	forumA.RegisterSync(nodeA.Host)
	if err := forumB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	if forumB.PostByID(p.ID) != nil {
		t.Fatal("SyncWithPeer sonrası B, A'nın mod-delete tombstone'unu uygulayıp postu silmeliydi — eski senkron-geri-getirme hatası geri döndü")
	}
}
