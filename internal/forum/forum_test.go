package forum

// forum_test.go — verifyPost/verifyReply/verifyDeleteMsg ve bunları kullanan
// handleMessage/storePost/applyDelete'in imza sahteciliği, tekrar (replay)
// ve TTL/onay davranışlarını kapsar. Önceki denetimde forum.go'nun test
// kapsamı %14.2'ydi ve bu paketin ürettiği/tükettiği veri (imzalı gönderiler,
// silme mesajları) hiç doğrudan test edilmiyordu.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

func newTestForum(t *testing.T) *Forum {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	f, err := New(id, nil, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	return f
}

func genPostKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func newSignedPost(priv ed25519.PrivateKey, pub ed25519.PublicKey, category, title, body string) *Post {
	p := &Post{
		Category:   category,
		AuthorName: "test-yazar",
		AuthorKey:  hex.EncodeToString(pub),
		Title:      title,
		Body:       body,
		CreatedAt:  time.Now().UTC(),
	}
	p.ID = derivePostID(p)
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, postPayload(p)))
	return p
}

func envJSON(t *testing.T, env envelope) []byte {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// ─── handleMessage: post sahteciliği ────────────────────────────────────────

func TestHandleMessage_ValidPostStored(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")

	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) == nil {
		t.Fatal("geçerli, imzalı bir post kabul edilmeliydi")
	}
}

func TestHandleMessage_TamperedBodyRejected(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")
	p.Body = "İçerik değiştirildi — imza artık geçersiz"

	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("imza payload'ıyla eşleşmeyen (değiştirilmiş) bir post kabul edilmemeliydi")
	}
}

func TestHandleMessage_WrongAuthorKeyRejected(t *testing.T) {
	f := newTestForum(t)
	_, priv := genPostKey(t)     // gerçek imzalayan
	otherPub, _ := genPostKey(t) // iddia edilen (ama imzayı atmayan) yazar
	p := newSignedPost(priv, otherPub, "Go", "Başlık", "İçerik")
	// newSignedPost, AuthorKey'i otherPub olarak ayarlayıp imzayı priv (farklı
	// bir anahtar çifti) ile atıyor — klasik "başkasının adına post" saldırısı.

	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("AuthorKey ile imzalayan anahtar uyuşmuyorsa post kabul edilmemeliydi")
	}
}

func TestHandleMessage_InvalidCategoryRejected(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Var-Olmayan-Kategori", "Başlık", "İçerik")

	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("bilinmeyen kategorideki bir post (imzası geçerli olsa da) kabul edilmemeliydi")
	}
}

func TestHandleMessage_OversizedBodyRejected(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	bigBody := make([]byte, rlMaxBodyBytes+1)
	for i := range bigBody {
		bigBody[i] = 'a'
	}
	p := newSignedPost(priv, pub, "Go", "Başlık", string(bigBody))

	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("boyut sınırını aşan bir post kabul edilmemeliydi")
	}
}

// ─── handleMessage: tekrar (replay) senaryoları ─────────────────────────────

func TestHandleMessage_ReplayedPostStoredOnce(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")
	data := envJSON(t, envelope{Type: "post", Post: p})

	f.handleMessage(data)
	f.handleMessage(data)
	f.handleMessage(data)

	if f.PostCount("Go") != 1 {
		t.Fatalf("aynı post 3 kez 'oynatıldı', 1 kopya bekleniyordu, %d geldi", f.PostCount("Go"))
	}
}

func TestHandleMessage_DeletedPostNotResurrectedByReplay(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")

	// Önce postu sil (tombstone oluştur) — post hiç depoda olmasa bile
	// applyDelete bir tombstone bırakır.
	authorKey := hex.EncodeToString(pub)
	d := &DeleteMsg{PostID: p.ID, AuthorKey: authorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(d)))
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))

	// Şimdi (ağdaki bir gecikme sonucu geç gelmiş gibi) orijinal postu "oynat".
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("tombstone'lanmış bir post, gecikmeli/tekrar gelen post mesajıyla yeniden canlanmamalı")
	}
}

func TestHandleMessage_DeleteByWrongAuthorRejected(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))
	if f.PostByID(p.ID) == nil {
		t.Fatal("test kurulumu: post önce başarıyla eklenmeliydi")
	}

	// Başka (ama kendi içinde geçerli şekilde imzalanmış) bir kullanıcı,
	// postun GERÇEK yazarıymış gibi silme mesajı gönderiyor.
	otherPub, otherPriv := genPostKey(t)
	d := &DeleteMsg{PostID: p.ID, AuthorKey: hex.EncodeToString(otherPub), DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(otherPriv, deleteMsgPayload(d)))

	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))

	if f.PostByID(p.ID) == nil {
		t.Fatal("postun gerçek yazarı olmayan biri postu silebilmemeliydi")
	}
}

// ─── applyDelete/storePost: bilinmeyen post için silme (tombstone sansürü) ──

func TestApplyDelete_ForgedDeleteForUnknownPostDoesNotBlockRealPost(t *testing.T) {
	f := newTestForum(t)
	victimPub, victimPriv := genPostKey(t)
	attackerPub, attackerPriv := genPostKey(t)

	realPost := newSignedPost(victimPriv, victimPub, "Go", "Gerçek Başlık", "Gerçek İçerik")

	// Saldırgan gerçek postun ID'sini bir şekilde gözlemlemiş (ör. başka bir
	// peer'dan görmüş) ve KENDİ anahtarıyla (kurbanın anahtarı DEĞİL) o ID
	// için kendi içinde geçerli şekilde imzalanmış sahte bir silme mesajı
	// yayınlıyor — gerçek post bize henüz ulaşmadan önce.
	fake := newSignedDelete(t, attackerPriv, hex.EncodeToString(attackerPub), realPost.ID)
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: fake}))

	if len(f.deleted) != 0 {
		t.Fatal("henüz görülmemiş bir post için gelen silme talebi kalıcı bir tombstone oluşturmamalı")
	}
	if _, pending := f.pendingDeletes[realPost.ID]; !pending {
		t.Fatal("talep, gerçek post gelene kadar bekleyen/doğrulanmamış olarak saklanmalıydı")
	}

	// Gerçek post şimdi (GERÇEK yazardan) geliyor.
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: realPost}))

	if f.PostByID(realPost.ID) == nil {
		t.Fatal("sahte silme, gerçek postun yazarıyla eşleşmediği için gerçek postu engellememeliydi")
	}
	if _, pending := f.pendingDeletes[realPost.ID]; pending {
		t.Fatal("gerçek post geldikten sonra bekleyen talep tüketilmiş/temizlenmiş olmalıydı")
	}
}

func TestApplyDelete_LegitimateDeleteBeforePostBlocksItWhenItArrives(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")

	// Gerçek yazarın kendisi, postu henüz kimse görmeden siliyor (ör. ağda
	// silme mesajı postun kendisinden önce yayılmış).
	d := newSignedDelete(t, priv, authorKey, p.ID)
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))

	// Şimdi gerçek post (AYNI yazardan) geliyor.
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("gerçek yazardan gelen, önceden bekleyen bir silme talebiyle eşleşen post saklanmamalıydı")
	}
	if _, tombstoned := f.deleted[p.ID]; !tombstoned {
		t.Fatal("yazar eşleşince bekleyen talep kalıcı bir tombstone'a dönüşmeliydi")
	}
}

func TestCleanup_PurgesExpiredPendingDeletes(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)

	d := newSignedDelete(t, priv, authorKey, "hic-gelmeyecek-post")
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))

	f.mu.Lock()
	pd, ok := f.pendingDeletes["hic-gelmeyecek-post"]
	if !ok {
		f.mu.Unlock()
		t.Fatal("bekleyen silme talebi kaydedilmiş olmalıydı")
	}
	pd.at = time.Now().Add(-pendingDeleteTTL - time.Hour)
	f.mu.Unlock()

	f.cleanup()

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.pendingDeletes["hic-gelmeyecek-post"]; ok {
		t.Fatal("süresi dolmuş bekleyen silme talebi cleanup() tarafından temizlenmeliydi")
	}
}

// ─── storePost: TTL ──────────────────────────────────────────────────────────

func TestStorePost_GetsFiveDayTTL(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")

	f.storePost(p)

	if p.ExpiresAt.IsZero() {
		t.Fatal("her posta bir TTL atanmalı")
	}
	if !p.ExpiresAt.After(time.Now()) {
		t.Fatal("TTL şu andan sonra olmalı")
	}
	kalan := time.Until(p.ExpiresAt)
	if kalan > postTTL || kalan < postTTL-time.Minute {
		t.Fatalf("TTL ~%v olmalı, %v kaldı", postTTL, kalan)
	}
}

// ─── AuthorStats ─────────────────────────────────────────────────────────────

func TestAuthorStats_AggregatesPostsAndReplies(t *testing.T) {
	f := newTestForum(t)
	const authorKey = "aabbccddeeff"
	const otherKey = "112233445566"
	now := time.Now()

	f.storePost(&Post{ID: "p1", Category: "Go", AuthorKey: authorKey, AuthorName: "yazar", Title: "Konu 1", Body: "B1", CreatedAt: now.Add(-48 * time.Hour)})
	f.storePost(&Post{ID: "p2", Category: "Go", AuthorKey: authorKey, AuthorName: "yazar", Title: "Konu 2", Body: "B2", CreatedAt: now})
	f.storePost(&Post{ID: "p3", Category: "Go", AuthorKey: otherKey, AuthorName: "başkası", Title: "Başkasının konusu", Body: "B3", CreatedAt: now})

	oldestReply := now.Add(-72 * time.Hour)
	f.storeReply(&Reply{ID: "r1", PostID: "p2", AuthorKey: authorKey, AuthorName: "yazar", Body: "yanıt", CreatedAt: oldestReply})

	postCount, replyCount, joinDate := f.AuthorStats(authorKey)
	if postCount != 2 {
		t.Errorf("postCount = %d, want 2", postCount)
	}
	if replyCount != 1 {
		t.Errorf("replyCount = %d, want 1", replyCount)
	}
	if !joinDate.Equal(oldestReply) {
		t.Errorf("joinDate = %v, want %v (postlardan da eski olan yanıt)", joinDate, oldestReply)
	}

	if pc, rc, jd := f.AuthorStats("hic-yok"); pc != 0 || rc != 0 || !jd.IsZero() {
		t.Errorf("bilinmeyen yazar için sıfır değerler beklenir, got postCount=%d replyCount=%d joinDate=%v", pc, rc, jd)
	}
}

func TestAuthorStats_DeletePostDecrementsAndGarbageCollects(t *testing.T) {
	f := newTestForum(t)
	const authorKey = "aabbccddeeff"
	now := time.Now()

	f.storePost(&Post{ID: "p1", Category: "Go", AuthorKey: authorKey, AuthorName: "yazar", Title: "K1", Body: "B1", CreatedAt: now})
	f.storePost(&Post{ID: "p2", Category: "Go", AuthorKey: authorKey, AuthorName: "yazar", Title: "K2", Body: "B2", CreatedAt: now})
	f.storeReply(&Reply{ID: "r1", PostID: "p1", AuthorKey: authorKey, AuthorName: "yazar", Body: "yanıt", CreatedAt: now})

	if pc, rc, _ := f.AuthorStats(authorKey); pc != 2 || rc != 1 {
		t.Fatalf("kurulum: postCount=%d replyCount=%d, want 2/1", pc, rc)
	}

	if err := f.DeletePost("p1"); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	// p1 silinince o konunun yanıtı (r1) da f.replies'ten kalkar; her ikisi de
	// authorStats'ten düşürülmeli.
	if pc, rc, _ := f.AuthorStats(authorKey); pc != 1 || rc != 0 {
		t.Fatalf("DeletePost sonrası postCount=%d replyCount=%d, want 1/0", pc, rc)
	}

	if err := f.DeletePost("p2"); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}
	// Yazarın hiç aktivitesi kalmayınca authorStats girdisi tamamen
	// temizlenmeli (joinDate de sıfırlanmalı, eski bir tarihte takılı kalmamalı).
	if pc, rc, jd := f.AuthorStats(authorKey); pc != 0 || rc != 0 || !jd.IsZero() {
		t.Fatalf("tüm aktivite silindikten sonra postCount=%d replyCount=%d joinDate=%v, want 0/0/sıfır", pc, rc, jd)
	}
}

func TestAuthorStats_ApplyDeleteDecrementsStats(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")
	f.storePost(p)

	if pc, _, _ := f.AuthorStats(authorKey); pc != 1 {
		t.Fatalf("kurulum: postCount=%d, want 1", pc)
	}

	d := &DeleteMsg{PostID: p.ID, AuthorKey: authorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(d)))
	f.applyDelete(d.PostID, d.AuthorKey, envJSON(t, envelope{Delete: d}))

	if pc, _, jd := f.AuthorStats(authorKey); pc != 0 || !jd.IsZero() {
		t.Fatalf("applyDelete sonrası postCount=%d joinDate=%v, want 0/sıfır", pc, jd)
	}
}

// ─── handleMessage: reply ────────────────────────────────────────────────────

func TestHandleMessage_ReplyToUnknownPostIgnored(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	r := &Reply{
		PostID:     "olmayan-post-id",
		AuthorName: "yazar",
		AuthorKey:  hex.EncodeToString(pub),
		Body:       "yanıt",
		CreatedAt:  time.Now().UTC(),
	}
	r.ID = deriveReplyID(r)
	r.Sig = hex.EncodeToString(ed25519.Sign(priv, replyPayload(r)))

	f.handleMessage(envJSON(t, envelope{Type: "reply", Reply: r}))

	if f.ReplyCount("olmayan-post-id") != 0 {
		t.Fatal("var olmayan bir posta yanıt saklanmamalı")
	}
}

// ─── sync.go: uçtan uca (gerçek iki libp2p düğümü) mutlu yol ────────────────

func TestSyncWithPeer_TransfersPost(t *testing.T) {
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

	forumA, err := New(idA, nil, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("New (A): %v", err)
	}
	t.Cleanup(func() { _ = forumA.db.Close() })
	forumB, err := New(idB, nil, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatalf("New (B): %v", err)
	}
	t.Cleanup(func() { _ = forumB.db.Close() })

	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Senkron Başlık", "Senkron İçerik")
	forumA.storePost(p)

	forumA.RegisterSync(nodeA.Host)

	if err := forumB.SyncWithPeer(ctx, nodeB.Host, nodeA.Host.ID()); err != nil {
		t.Fatalf("SyncWithPeer: %v", err)
	}

	got := forumB.PostByID(p.ID)
	if got == nil {
		t.Fatal("SyncWithPeer sonrası B, A'daki postu almalıydı")
	}
	if got.Title != p.Title {
		t.Fatalf("Title = %q, want %q", got.Title, p.Title)
	}
}

func TestCategoryCounts_MatchesPostCount(t *testing.T) {
	f := &Forum{byID: map[string]*Post{}, replies: map[string][]*Reply{}}
	for i, kat := range []string{"Go", "Go", "Rust", "Python", "Go"} {
		f.posts = append(f.posts, &Post{ID: string(rune('a' + i)), Category: kat})
	}
	sayilar := f.CategoryCounts()
	for _, kat := range []string{"Go", "Rust", "Python", "Linux"} {
		if sayilar[kat] != f.PostCount(kat) {
			t.Errorf("%s: CategoryCounts %d, PostCount %d", kat, sayilar[kat], f.PostCount(kat))
		}
	}
	if sayilar["Go"] != 3 {
		t.Fatalf("Go için 3 konu bekleniyordu, got %d", sayilar["Go"])
	}
}
