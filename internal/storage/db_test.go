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
		Approved:   false,
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

// ─── ApprovePost ──────────────────────────────────────────────────────────────

func TestApprovePost(t *testing.T) {
	db := openTestDB(t)
	p := samplePost("post_approve")
	_ = db.InsertPost(p)

	if err := db.ApprovePost("post_approve"); err != nil {
		t.Fatalf("ApprovePost: %v", err)
	}

	posts, _ := db.AllPosts()
	if !posts[0].Approved {
		t.Error("post onaylanmış görünmeli")
	}
}

func TestApprovePostsByAuthor(t *testing.T) {
	db := openTestDB(t)
	p1 := samplePost("pa1")
	p1.AuthorKey = "author_x"
	p2 := samplePost("pa2")
	p2.AuthorKey = "author_x"
	p3 := samplePost("pa3")
	p3.AuthorKey = "author_y"
	_ = db.InsertPost(p1)
	_ = db.InsertPost(p2)
	_ = db.InsertPost(p3)

	if err := db.ApprovePostsByAuthor("author_x"); err != nil {
		t.Fatalf("ApprovePostsByAuthor: %v", err)
	}

	posts, _ := db.AllPosts()
	byID := make(map[string]bool)
	for _, p := range posts {
		byID[p.ID] = p.Approved
	}
	if !byID["pa1"] || !byID["pa2"] {
		t.Error("author_x'in postları onaylanmalı")
	}
	if byID["pa3"] {
		t.Error("author_y'nin postu onaylanmamalı")
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

// ─── PendingPosts ─────────────────────────────────────────────────────────────

func TestPendingPosts_OnlyPermanentReq(t *testing.T) {
	db := openTestDB(t)

	normal := samplePost("pend_normal")
	normal.PermanentReq = false
	_ = db.InsertPost(normal)

	permReq := samplePost("pend_perm")
	permReq.PermanentReq = true
	_ = db.InsertPost(permReq)

	pending, err := db.PendingPosts()
	if err != nil {
		t.Fatalf("PendingPosts: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "pend_perm" {
		t.Errorf("yalnızca permanent_req=1 postlar beklemede görünmeli: %+v", pending)
	}
}

func TestPendingPosts_ExcludesApproved(t *testing.T) {
	db := openTestDB(t)

	p := samplePost("pend_appr")
	p.PermanentReq = true
	_ = db.InsertPost(p)
	_ = db.ApprovePost("pend_appr")

	pending, _ := db.PendingPosts()
	if len(pending) != 0 {
		t.Error("onaylanmış post beklemede görünmemeli")
	}
}

func TestPendingPosts_ExcludesExpired(t *testing.T) {
	db := openTestDB(t)

	p := samplePost("pend_exp")
	p.PermanentReq = true
	p.ExpiresAt = time.Now().Add(-time.Hour)
	_ = db.InsertPost(p)

	pending, _ := db.PendingPosts()
	if len(pending) != 0 {
		t.Error("süresi dolmuş post beklemede görünmemeli")
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

func TestDeleteExpiredPosts_SkipsApproved(t *testing.T) {
	db := openTestDB(t)

	p := samplePost("appr_never_exp")
	p.ExpiresAt = time.Now().Add(-time.Hour)
	p.Approved = true
	_ = db.InsertPost(p)

	n, _ := db.DeleteExpiredPosts()
	if n != 0 {
		t.Fatal("onaylanmış post süresi dolsa da silinmemeli")
	}
}
