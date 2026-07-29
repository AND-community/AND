package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func samplePost(id string) RawPost {
	return RawPost{
		ID:         id,
		Category:   "Go",
		AuthorName: "alice",
		AuthorKey:  "aabbcc",
		Title:      "Test Konusu",
		Body:       "Test içeriği",
		CreatedAt:  time.Now().UTC(),
		Sig:        "deadsig",
		ExpiresAt:  time.Now().Add(postTTL),
	}
}

// ─── InsertPost / AllPosts ────────────────────────────────────────────────────

func TestInsertPost_AllPosts(t *testing.T) {
	db := openTestDB(t)

	p := samplePost("post001")
	if err := db.InsertPost(p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	posts, err := db.AllPosts()
	if err != nil {
		t.Fatalf("AllPosts: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("beklenen 1 post, %d var", len(posts))
	}
	if posts[0].ID != "post001" {
		t.Errorf("ID: %q", posts[0].ID)
	}
	if posts[0].Category != "Go" {
		t.Errorf("Category: %q", posts[0].Category)
	}
}

func TestInsertPost_Idempotent(t *testing.T) {
	db := openTestDB(t)
	p := samplePost("post_dup")
	_ = db.InsertPost(p)
	_ = db.InsertPost(p) // INSERT OR IGNORE

	posts, _ := db.AllPosts()
	if len(posts) != 1 {
		t.Fatalf("tekrarlı insert 1 kayıt üretmeli, %d var", len(posts))
	}
}

// ─── DeletePost ───────────────────────────────────────────────────────────────

func TestDeletePost_RemovesPostAndReplies(t *testing.T) {
	db := openTestDB(t)
	_ = db.InsertPost(samplePost("post_del"))
	_ = db.InsertReply(RawReply{
		ID: "reply1", PostID: "post_del",
		AuthorName: "bob", AuthorKey: "bbcc", Body: "yanıt",
		CreatedAt: time.Now(), Sig: "sig",
	})

	if err := db.DeletePost("post_del"); err != nil {
		t.Fatalf("DeletePost: %v", err)
	}

	posts, _ := db.AllPosts()
	if len(posts) != 0 {
		t.Error("silinmiş post görünmemeli")
	}
	replies, _ := db.AllReplies()
	if len(replies) != 0 {
		t.Error("silinmiş postun yanıtları da silinmeli")
	}
}

// ─── InsertReply / AllReplies ─────────────────────────────────────────────────

func TestInsertReply_AllReplies(t *testing.T) {
	db := openTestDB(t)
	_ = db.InsertPost(samplePost("post_reply"))
	r := RawReply{
		ID: "reply_a", PostID: "post_reply",
		AuthorName: "bob", AuthorKey: "ccdd", Body: "güzel konu",
		CreatedAt: time.Now(), Sig: "sig2",
	}
	if err := db.InsertReply(r); err != nil {
		t.Fatalf("InsertReply: %v", err)
	}

	replies, err := db.AllReplies()
	if err != nil {
		t.Fatalf("AllReplies: %v", err)
	}
	if len(replies) != 1 || replies[0].Body != "güzel konu" {
		t.Errorf("beklenmedik yanıtlar: %+v", replies)
	}
}

func TestInsertReply_Idempotent(t *testing.T) {
	db := openTestDB(t)
	_ = db.InsertPost(samplePost("post_idem"))
	r := RawReply{ID: "reply_idem", PostID: "post_idem", Body: "x", CreatedAt: time.Now(), Sig: "s"}
	_ = db.InsertReply(r)
	_ = db.InsertReply(r)

	replies, _ := db.AllReplies()
	if len(replies) != 1 {
		t.Fatalf("tekrarlı yanıt insert 1 kayıt üretmeli, %d var", len(replies))
	}
}

// ─── Tombstones ───────────────────────────────────────────────────────────────

func TestTombstone_InsertAndRead(t *testing.T) {
	db := openTestDB(t)

	if err := db.InsertTombstone("post_tomb", `{"post_id":"post_tomb"}`); err != nil {
		t.Fatalf("InsertTombstone: %v", err)
	}

	jsons, err := db.AllTombstoneJSON()
	if err != nil {
		t.Fatalf("AllTombstoneJSON: %v", err)
	}
	if len(jsons) != 1 {
		t.Fatalf("1 tombstone bekleniyor, %d var", len(jsons))
	}
}

func TestTombstone_Idempotent(t *testing.T) {
	db := openTestDB(t)
	_ = db.InsertTombstone("pt", `{}`)
	_ = db.InsertTombstone("pt", `{}`)

	jsons, _ := db.AllTombstoneJSON()
	if len(jsons) != 1 {
		t.Fatalf("tekrarlı tombstone insert 1 kayıt üretmeli, %d var", len(jsons))
	}
}

// ─── DeleteExpiredPosts ───────────────────────────────────────────────────────

func TestDeleteExpiredPosts(t *testing.T) {
	db := openTestDB(t)

	expired := samplePost("exp_post")
	expired.ExpiresAt = time.Now().Add(-time.Hour)
	_ = db.InsertPost(expired)

	active := samplePost("active_post")
	active.ExpiresAt = time.Now().Add(time.Hour)
	_ = db.InsertPost(active)

	n, err := db.DeleteExpiredPosts()
	if err != nil {
		t.Fatalf("DeleteExpiredPosts: %v", err)
	}
	if n != 1 {
		t.Fatalf("1 post silinmeli, %d silindi", n)
	}

	posts, _ := db.AllPosts()
	if len(posts) != 1 || posts[0].ID != "active_post" {
		t.Errorf("yalnızca aktif post kalmalı: %+v", posts)
	}
}

// ─── UpsertProfile / AllProfiles ───────────────────────────────────────────────

func TestUpsertProfile_AllProfiles(t *testing.T) {
	db := openTestDB(t)

	p := RawProfile{
		AuthorKey: "aabbcc", Bio: "merhaba", UpdatedAt: time.Now().UTC(),
		ModCertJSON: "", Sig: "sig1",
	}
	if err := db.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles, err := db.AllProfiles()
	if err != nil {
		t.Fatalf("AllProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("1 profil bekleniyor, %d var", len(profiles))
	}
	if profiles[0].AuthorKey != "aabbcc" || profiles[0].Bio != "merhaba" || profiles[0].Sig != "sig1" {
		t.Errorf("beklenmedik profil: %+v", profiles[0])
	}
}

func TestUpsertProfile_ReplacesExisting(t *testing.T) {
	db := openTestDB(t)

	first := RawProfile{AuthorKey: "aabbcc", Bio: "eski bio", UpdatedAt: time.Now().UTC(), Sig: "sig1"}
	if err := db.UpsertProfile(first); err != nil {
		t.Fatalf("UpsertProfile (1.): %v", err)
	}
	second := RawProfile{AuthorKey: "aabbcc", Bio: "yeni bio", UpdatedAt: time.Now().Add(time.Minute).UTC(), Sig: "sig2"}
	if err := db.UpsertProfile(second); err != nil {
		t.Fatalf("UpsertProfile (2.): %v", err)
	}

	profiles, _ := db.AllProfiles()
	if len(profiles) != 1 {
		t.Fatalf("aynı author_key için 1 kayıt olmalı, %d var", len(profiles))
	}
	if profiles[0].Bio != "yeni bio" || profiles[0].Sig != "sig2" {
		t.Errorf("upsert eski kaydı yeniyle değiştirmemiş: %+v", profiles[0])
	}
}

func TestUpsertProfile_PersistsModCertJSON(t *testing.T) {
	db := openTestDB(t)

	p := RawProfile{
		AuthorKey: "aabbcc", Bio: "moderatörüm", UpdatedAt: time.Now().UTC(),
		ModCertJSON: `{"moderator_key":"aabbcc","permanent":true,"sig":"modsig"}`,
		Sig:         "sig1",
	}
	if err := db.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles, _ := db.AllProfiles()
	if len(profiles) != 1 || profiles[0].ModCertJSON != p.ModCertJSON {
		t.Errorf("mod_cert JSON'u korunmamış: %+v", profiles)
	}
}

func TestUpsertProfile_PersistsStatusTagsLink(t *testing.T) {
	db := openTestDB(t)

	p := RawProfile{
		AuthorKey: "aabbcc", Bio: "bio", Status: "kod yazıyorum",
		Tags: "Go,Rust,Siber Güvenlik", Link: "https://github.com/example",
		UpdatedAt: time.Now().UTC(), Sig: "sig1",
	}
	if err := db.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles, err := db.AllProfiles()
	if err != nil {
		t.Fatalf("AllProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("1 profil bekleniyor, %d var", len(profiles))
	}
	got := profiles[0]
	if got.Status != p.Status || got.Tags != p.Tags || got.Link != p.Link {
		t.Errorf("status/tags/link korunmamış: got %+v, want Status=%q Tags=%q Link=%q", got, p.Status, p.Tags, p.Link)
	}
}

func TestUpsertProfile_PersistsGitHubUser(t *testing.T) {
	db := openTestDB(t)

	p := RawProfile{
		AuthorKey: "aabbcc", Bio: "bio", GitHubUser: "octocat",
		UpdatedAt: time.Now().UTC(), Sig: "sig1",
	}
	if err := db.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles, err := db.AllProfiles()
	if err != nil {
		t.Fatalf("AllProfiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].GitHubUser != "octocat" {
		t.Errorf("github_user korunmamış: %+v", profiles)
	}

	// Bağlantıyı kaldırmayı da doğrula: boş GitHubUser'lı bir güncelleme
	// önceki değeri temizlemeli, "eskiyi koru" davranışına düşmemeli.
	p.GitHubUser = ""
	p.UpdatedAt = p.UpdatedAt.Add(time.Second)
	if err := db.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile (temizleme): %v", err)
	}
	profiles, _ = db.AllProfiles()
	if len(profiles) != 1 || profiles[0].GitHubUser != "" {
		t.Errorf("github_user temizlenmemiş: %+v", profiles)
	}
}

func TestInsertChatMessage_PersistsAndRoundTrips(t *testing.T) {
	db := openTestDB(t)

	m := RawChatMessage{
		ID: "msg1", AuthorKey: "aabbcc", AuthorName: "alice",
		Text: "merhaba", SentAt: time.Now().UTC(), Sig: "sig1",
	}
	if err := db.InsertChatMessage(m); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}

	got, err := db.AllChatMessages()
	if err != nil {
		t.Fatalf("AllChatMessages: %v", err)
	}
	if len(got) != 1 || got[0].Text != "merhaba" || got[0].AuthorKey != "aabbcc" {
		t.Errorf("mesaj korunmamış: %+v", got)
	}
}

func TestInsertChatMessage_DuplicateIDIgnored(t *testing.T) {
	db := openTestDB(t)
	m := RawChatMessage{ID: "msg1", AuthorKey: "aabbcc", AuthorName: "alice", Text: "ilk", SentAt: time.Now().UTC(), Sig: "sig1"}
	if err := db.InsertChatMessage(m); err != nil {
		t.Fatalf("InsertChatMessage: %v", err)
	}
	m.Text = "tekrar (farklı ID değil)"
	if err := db.InsertChatMessage(m); err != nil {
		t.Fatalf("InsertChatMessage (dup): %v", err)
	}
	got, _ := db.AllChatMessages()
	if len(got) != 1 || got[0].Text != "ilk" {
		t.Errorf("aynı ID'li ikinci ekleme yok sayılmalıydı (ilk kayıt korunmalı), got %+v", got)
	}
}

func TestDeleteExpiredChatMessages(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	fresh := RawChatMessage{ID: "fresh", AuthorKey: "aa", AuthorName: "a", Text: "taze", SentAt: now, ExpiresAt: now.Add(time.Hour), Sig: "s"}
	expired := RawChatMessage{ID: "old", AuthorKey: "aa", AuthorName: "a", Text: "eski", SentAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), Sig: "s"}
	if err := db.InsertChatMessage(fresh); err != nil {
		t.Fatalf("InsertChatMessage (fresh): %v", err)
	}
	if err := db.InsertChatMessage(expired); err != nil {
		t.Fatalf("InsertChatMessage (expired): %v", err)
	}

	n, err := db.DeleteExpiredChatMessages()
	if err != nil {
		t.Fatalf("DeleteExpiredChatMessages: %v", err)
	}
	if n != 1 {
		t.Fatalf("silinen kayıt sayısı = %d, want 1", n)
	}

	got, _ := db.AllChatMessages()
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Errorf("yalnızca süresi dolmayan kayıt kalmalıydı, got %+v", got)
	}
}

func TestDeleteChatMessage_RemovesOnlyThatMessage(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	a := RawChatMessage{ID: "a", AuthorKey: "aa", AuthorName: "a", Text: "a", SentAt: now, Sig: "s"}
	b := RawChatMessage{ID: "b", AuthorKey: "aa", AuthorName: "a", Text: "b", SentAt: now, Sig: "s"}
	if err := db.InsertChatMessage(a); err != nil {
		t.Fatalf("InsertChatMessage (a): %v", err)
	}
	if err := db.InsertChatMessage(b); err != nil {
		t.Fatalf("InsertChatMessage (b): %v", err)
	}

	if err := db.DeleteChatMessage("a"); err != nil {
		t.Fatalf("DeleteChatMessage: %v", err)
	}

	got, _ := db.AllChatMessages()
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("yalnızca 'a' silinmeliydi, 'b' kalmalıydı, got %+v", got)
	}
}
