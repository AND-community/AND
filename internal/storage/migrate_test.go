package storage

// migrate_test.go — DeletePostWithTombstone'un atomikliği ve migrate()'in
// eski (yalnızca v1 uygulanmış) bir veritabanını sorunsuz yükseltebildiğini
// doğrular; her ikisi de db_test.go'daki mevcut CRUD testlerinin kapsamadığı
// yollardı.

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDeletePostWithTombstone_RemovesPostAndWritesTombstone(t *testing.T) {
	db := openTestDB(t)
	p := samplePost("p1")
	if err := db.InsertPost(p); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.InsertReply(RawReply{ID: "r1", PostID: "p1", AuthorName: "bob", AuthorKey: "bb", Body: "yanıt", Sig: "sig"}); err != nil {
		t.Fatalf("InsertReply: %v", err)
	}

	if err := db.DeletePostWithTombstone("p1", `{"post_id":"p1"}`); err != nil {
		t.Fatalf("DeletePostWithTombstone: %v", err)
	}

	posts, err := db.AllPosts()
	if err != nil {
		t.Fatalf("AllPosts: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("post silinmiş olmalıydı, %d kaldı", len(posts))
	}
	replies, err := db.AllReplies()
	if err != nil {
		t.Fatalf("AllReplies: %v", err)
	}
	if len(replies) != 0 {
		t.Fatalf("yanıtlar da silinmiş olmalıydı, %d kaldı", len(replies))
	}
	tombs, err := db.AllTombstoneJSON()
	if err != nil {
		t.Fatalf("AllTombstoneJSON: %v", err)
	}
	if len(tombs) != 1 {
		t.Fatalf("1 tombstone bekleniyordu, %d geldi", len(tombs))
	}
}

func TestDeletePostWithTombstone_Idempotent(t *testing.T) {
	db := openTestDB(t)
	if err := db.InsertPost(samplePost("p1")); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.DeletePostWithTombstone("p1", `{"post_id":"p1"}`); err != nil {
		t.Fatalf("DeletePostWithTombstone (1.): %v", err)
	}
	// Aynı postu ikinci kez "silmek" (ör. ağdan tekrar gelen bir silme
	// mesajı) hataya düşmemeli — INSERT OR IGNORE tombstone'da, DELETE
	// zaten var olmayan satırda no-op.
	if err := db.DeletePostWithTombstone("p1", `{"post_id":"p1","tekrar":true}`); err != nil {
		t.Fatalf("DeletePostWithTombstone (2., tekrar): %v", err)
	}
	tombs, err := db.AllTombstoneJSON()
	if err != nil {
		t.Fatalf("AllTombstoneJSON: %v", err)
	}
	if len(tombs) != 1 {
		t.Fatalf("tombstone hâlâ tek olmalı (INSERT OR IGNORE), %d geldi", len(tombs))
	}
}

// TestMigrate_LegacyV1UpgradesCleanly, yalnızca ilk (approved/expires_at/
// permanent_req sütunları eklenmeden önceki) v1 şeması uygulanmış eski bir
// veritabanının, migrate()'in v2-v6 adımlarıyla sorunsuz yükseltildiğini ve
// mevcut satırların hayatta kaldığını doğrular.
func TestMigrate_LegacyV1UpgradesCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(`
		CREATE TABLE schema_versions (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
		CREATE TABLE posts (
			id TEXT PRIMARY KEY, category TEXT NOT NULL, author_name TEXT NOT NULL,
			author_key TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL,
			created_at INTEGER NOT NULL, sig TEXT NOT NULL
		);
		CREATE TABLE replies (
			id TEXT PRIMARY KEY, post_id TEXT NOT NULL, author_name TEXT NOT NULL,
			author_key TEXT NOT NULL, body TEXT NOT NULL, created_at INTEGER NOT NULL, sig TEXT NOT NULL
		);
		CREATE TABLE tombstones (post_id TEXT PRIMARY KEY, msg_json TEXT NOT NULL);
		INSERT INTO schema_versions(version, applied_at) VALUES (1, 0);
		INSERT INTO posts(id, category, author_name, author_key, title, body, created_at, sig)
			VALUES ('legacy1', 'Go', 'alice', 'aabbcc', 'Eski Konu', 'Eski icerik', 1000, 'sig1');
	`); err != nil {
		t.Fatalf("eski şema kurulamadı: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open (yükseltme sırasında): %v", err)
	}
	defer db.Close()

	posts, err := db.AllPosts()
	if err != nil {
		t.Fatalf("AllPosts: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "legacy1" {
		t.Fatalf("eski post yükseltmeden sonra da bulunmalıydı: %+v", posts)
	}
	// v6 migrasyonu, expires_at=0 (eski şemada hiç yoktu → varsayılan 0,
	// v5'te "onaylı/kalıcı" sayılmıştı) olan eski postlara created_at'a göre
	// gerçek bir TTL atar — artık hiçbir konu süresiz kalmaz.
	if posts[0].ExpiresAt.IsZero() {
		t.Fatal("eski post, v6 migrasyonuyla created_at+postTTL bir expires_at almalıydı")
	}
	wantExpires := posts[0].CreatedAt.Add(postTTL)
	if !posts[0].ExpiresAt.Equal(wantExpires) {
		t.Fatalf("ExpiresAt = %v, want %v (created_at + postTTL)", posts[0].ExpiresAt, wantExpires)
	}

	// Yükseltilmiş şema üzerinde normal işlemler de sorunsuz çalışmalı.
	if err := db.InsertPost(samplePost("yeni1")); err != nil {
		t.Fatalf("yükseltilmiş şemada InsertPost: %v", err)
	}
}
