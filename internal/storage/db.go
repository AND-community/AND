// Package storage provides AND's local persistence layer: a SQLite database
// that caches forum posts/replies received over the network. Uses a pure-Go
// driver (modernc.org/sqlite) so no C toolchain is required.
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// postTTL is the default lifetime for posts that have not been approved.
const postTTL = 5 * 24 * time.Hour

// DB wraps the SQLite database for forum persistence.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path. WAL journal mode is
// enabled for better concurrent read performance.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: set pragmas: %w", err)
	}
	s := &DB{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *DB) Close() error { return s.db.Close() }

func (s *DB) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS posts (
			id              TEXT PRIMARY KEY,
			category        TEXT NOT NULL,
			author_name     TEXT NOT NULL,
			author_key      TEXT NOT NULL,
			title           TEXT NOT NULL,
			body            TEXT NOT NULL,
			created_at      INTEGER NOT NULL,
			sig             TEXT NOT NULL,
			approved        INTEGER NOT NULL DEFAULT 0,
			expires_at      INTEGER NOT NULL DEFAULT 0,
			permanent_req   INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_posts_cat ON posts(category, created_at DESC);
		CREATE TABLE IF NOT EXISTS replies (
			id          TEXT PRIMARY KEY,
			post_id     TEXT NOT NULL,
			author_name TEXT NOT NULL,
			author_key  TEXT NOT NULL,
			body        TEXT NOT NULL,
			created_at  INTEGER NOT NULL,
			sig         TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_replies_post ON replies(post_id, created_at);
	`)
	if err != nil {
		return err
	}

	// Tombstone tablosu: silinmiş post'ların imzalı DeleteMsg'lerini saklar.
	// Peer sync sırasında bu postların tekrar eklenmesini önler.
	// Kayıtlar kalıcıdır; topluluk ölçeğinde (<1000 silme) boyut önemsizdir (~24KB).
	s.db.Exec(`CREATE TABLE IF NOT EXISTS tombstones (
		post_id  TEXT PRIMARY KEY,
		msg_json TEXT NOT NULL
	)`)

	// Var olan DB'ye yeni sütunlar ekle (zaten varsa hata yoksayılır).
	s.db.Exec("ALTER TABLE posts ADD COLUMN approved INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE posts ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE posts ADD COLUMN permanent_req INTEGER NOT NULL DEFAULT 0")

	// Bu özellikten önceki tüm postları otomatik onayla — sadece bir kez çalışır.
	s.db.Exec("UPDATE posts SET approved=1 WHERE approved=0 AND expires_at=0")
	return nil
}

// RawPost is the flat persistence representation of a forum post.
type RawPost struct {
	ID, Category, AuthorName, AuthorKey, Title, Body, Sig string
	CreatedAt    time.Time
	Approved     bool
	ExpiresAt    time.Time // zero = onaysız + süresi belirlenmemiş
	PermanentReq bool      // kullanıcı kalıcılık talep etti mi
}

// PendingPost is a post that requested permanent status, awaiting admin/mod approval.
type PendingPost struct {
	ID         string
	Title      string
	AuthorName string
	AuthorKey  string
	Category   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// RawReply is the flat persistence representation of a forum reply.
type RawReply struct {
	ID, PostID, AuthorName, AuthorKey, Body, Sig string
	CreatedAt                                    time.Time
}

// InsertPost persists a post; silently ignores duplicates.
func (s *DB) InsertPost(p RawPost) error {
	var expiresNano int64
	if !p.ExpiresAt.IsZero() {
		expiresNano = p.ExpiresAt.UnixNano()
	}
	approved := 0
	if p.Approved {
		approved = 1
	}
	permReq := 0
	if p.PermanentReq {
		permReq = 1
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO posts(id,category,author_name,author_key,title,body,created_at,sig,approved,expires_at,permanent_req)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Category, p.AuthorName, p.AuthorKey,
		p.Title, p.Body, p.CreatedAt.UnixNano(), p.Sig, approved, expiresNano, permReq,
	)
	return err
}

// InsertReply persists a reply; silently ignores duplicates.
func (s *DB) InsertReply(r RawReply) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO replies(id,post_id,author_name,author_key,body,created_at,sig)
		 VALUES(?,?,?,?,?,?,?)`,
		r.ID, r.PostID, r.AuthorName, r.AuthorKey,
		r.Body, r.CreatedAt.UnixNano(), r.Sig,
	)
	return err
}

// InsertTombstone stores a signed DeleteMsg JSON for a post that was permanently deleted.
// Idempotent — re-inserting the same post_id is silently ignored.
func (s *DB) InsertTombstone(postID, msgJSON string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO tombstones(post_id, msg_json) VALUES(?, ?)`,
		postID, msgJSON,
	)
	return err
}

// AllTombstoneJSON returns the JSON-encoded DeleteMsg for every tombstoned post.
func (s *DB) AllTombstoneJSON() ([]string, error) {
	rows, err := s.db.Query(`SELECT msg_json FROM tombstones`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		out = append(out, js)
	}
	return out, rows.Err()
}

// DeletePost removes a post and all its replies from the database.
func (s *DB) DeletePost(postID string) error {
	_, err := s.db.Exec("DELETE FROM replies WHERE post_id=?", postID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM posts WHERE id=?", postID)
	return err
}

// ApprovePost marks a post as permanently approved.
func (s *DB) ApprovePost(postID string) error {
	_, err := s.db.Exec("UPDATE posts SET approved=1 WHERE id=?", postID)
	return err
}

// ApprovePostsByAuthor marks all posts by the given author key as approved.
func (s *DB) ApprovePostsByAuthor(authorKey string) error {
	_, err := s.db.Exec("UPDATE posts SET approved=1 WHERE author_key=?", authorKey)
	return err
}

// SetPostExpiry sets the TTL deadline for an unapproved post.
func (s *DB) SetPostExpiry(postID string, expiresAt time.Time) error {
	_, err := s.db.Exec("UPDATE posts SET expires_at=? WHERE id=?",
		expiresAt.UnixNano(), postID)
	return err
}

// PendingPosts returns unapproved posts where the user requested permanent status,
// soonest-expiring first. These are posts the admin/mod should review.
func (s *DB) PendingPosts() ([]PendingPost, error) {
	now := time.Now().UnixNano()
	rows, err := s.db.Query(`
		SELECT id, title, author_name, author_key, category, created_at, expires_at
		FROM posts
		WHERE approved=0 AND permanent_req=1 AND expires_at > ?
		ORDER BY expires_at ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingPost
	for rows.Next() {
		var p PendingPost
		var createdTs, expiresTs int64
		if err := rows.Scan(&p.ID, &p.Title, &p.AuthorName, &p.AuthorKey,
			&p.Category, &createdTs, &expiresTs); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(0, createdTs).UTC()
		p.ExpiresAt = time.Unix(0, expiresTs).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteExpiredPosts removes unapproved posts past their TTL. Returns count deleted.
func (s *DB) DeleteExpiredPosts() (int, error) {
	now := time.Now().UnixNano()
	res, err := s.db.Exec(`
		DELETE FROM posts WHERE approved=0 AND expires_at > 0 AND expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// AllPosts returns every post ordered by creation time (oldest first).
func (s *DB) AllPosts() ([]RawPost, error) {
	rows, err := s.db.Query(
		`SELECT id,category,author_name,author_key,title,body,created_at,sig,approved,expires_at,permanent_req
		 FROM posts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawPost
	for rows.Next() {
		var p RawPost
		var ts, expiresNano int64
		var approved, permReq int
		if err := rows.Scan(&p.ID, &p.Category, &p.AuthorName, &p.AuthorKey,
			&p.Title, &p.Body, &ts, &p.Sig, &approved, &expiresNano, &permReq); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(0, ts).UTC()
		p.Approved = approved != 0
		p.PermanentReq = permReq != 0
		if expiresNano > 0 {
			p.ExpiresAt = time.Unix(0, expiresNano).UTC()
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AllReplies returns every reply ordered by post_id then creation time.
func (s *DB) AllReplies() ([]RawReply, error) {
	rows, err := s.db.Query(
		`SELECT id,post_id,author_name,author_key,body,created_at,sig
		 FROM replies ORDER BY post_id, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawReply
	for rows.Next() {
		var r RawReply
		var ts int64
		if err := rows.Scan(&r.ID, &r.PostID, &r.AuthorName, &r.AuthorKey,
			&r.Body, &ts, &r.Sig); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(0, ts).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
