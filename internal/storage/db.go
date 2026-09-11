// Package storage, forum gönderilerinin/yanıtlarının/moderasyon
// tombstone'larının kalıcılığını SQLite (WAL modu, tek yazarlı bağlantı
// havuzu) üzerinden sağlar. Şema, `schema_versions` tablosunda izlenen
// numaralı, idempotent migrasyonlarla evrilir (bkz. migrate()); zaman
// damgaları UnixNano olarak saklanır. Her konu oluşturulduktan postTTL (5
// gün) sonra süresi dolar; `expires_at=0` yalnızca henüz atanmamış (geçici)
// bir durumu ifade eder.
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const postTTL = 5 * 24 * time.Hour

type DB struct {
	db *sql.DB
}

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

func (s *DB) Close() error { return s.db.Close() }

func (s *DB) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_versions (
			version    INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("storage: create schema_versions: %w", err)
	}

	for _, step := range migrations {
		var count int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM schema_versions WHERE version=?", step.version).Scan(&count)
		if count > 0 {
			continue
		}
		_, execErr := s.db.Exec(step.sql)
		if execErr != nil && !step.ignoreErr {
			return fmt.Errorf("storage: migration v%d: %w", step.version, execErr)
		}
		s.db.Exec("INSERT INTO schema_versions(version,applied_at) VALUES(?,?)",
			step.version, time.Now().UnixNano())
	}
	return nil
}

type migration struct {
	version   int
	sql       string
	ignoreErr bool
}

var migrations = []migration{
	{version: 1, sql: `
		CREATE TABLE IF NOT EXISTS posts (
			id            TEXT PRIMARY KEY,
			category      TEXT NOT NULL,
			author_name   TEXT NOT NULL,
			author_key    TEXT NOT NULL,
			title         TEXT NOT NULL,
			body          TEXT NOT NULL,
			created_at    INTEGER NOT NULL,
			sig           TEXT NOT NULL,
			approved      INTEGER NOT NULL DEFAULT 0,
			expires_at    INTEGER NOT NULL DEFAULT 0,
			permanent_req INTEGER NOT NULL DEFAULT 0
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
		CREATE TABLE IF NOT EXISTS tombstones (
			post_id  TEXT PRIMARY KEY,
			msg_json TEXT NOT NULL
		);
	`},
	{version: 2, ignoreErr: true, sql: `ALTER TABLE posts ADD COLUMN approved      INTEGER NOT NULL DEFAULT 0`},
	{version: 3, ignoreErr: true, sql: `ALTER TABLE posts ADD COLUMN expires_at    INTEGER NOT NULL DEFAULT 0`},
	{version: 4, ignoreErr: true, sql: `ALTER TABLE posts ADD COLUMN permanent_req INTEGER NOT NULL DEFAULT 0`},
	{version: 5, sql: `UPDATE posts SET approved=1 WHERE approved=0 AND expires_at=0`},
	// v6: onay sistemi kaldırıldı — her konu, onay durumundan bağımsız,
	// oluşturulduktan postTTL (5 gün) sonra süresi dolar. Önceden onaylanmış
	// ya da hiç TTL atanmamış (expires_at=0, "kalıcı") eski kayıtlara da
	// created_at'a göre gerçek bir expires_at atanır.
	{version: 6, sql: fmt.Sprintf(
		`UPDATE posts SET expires_at = created_at + %d WHERE approved = 1 OR expires_at = 0;
		 UPDATE posts SET approved = 0;`, postTTL.Nanoseconds())},
	{version: 7, sql: `
		CREATE TABLE IF NOT EXISTS profiles (
			author_key TEXT PRIMARY KEY,
			bio        TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			mod_cert   TEXT NOT NULL DEFAULT '',
			sig        TEXT NOT NULL
		);
	`},
	{version: 8, ignoreErr: true, sql: `ALTER TABLE profiles ADD COLUMN status TEXT NOT NULL DEFAULT ''`},
	{version: 9, ignoreErr: true, sql: `ALTER TABLE profiles ADD COLUMN tags   TEXT NOT NULL DEFAULT ''`},
	{version: 10, ignoreErr: true, sql: `ALTER TABLE profiles ADD COLUMN link  TEXT NOT NULL DEFAULT ''`},
	{version: 11, ignoreErr: true, sql: `ALTER TABLE profiles ADD COLUMN github_user TEXT NOT NULL DEFAULT ''`},
	{version: 12, sql: `
		CREATE TABLE IF NOT EXISTS chat_messages (
			id          TEXT PRIMARY KEY,
			author_key  TEXT NOT NULL,
			author_name TEXT NOT NULL,
			text        TEXT NOT NULL,
			sent_at     INTEGER NOT NULL,
			expires_at  INTEGER NOT NULL DEFAULT 0,
			sig         TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_chat_messages_sent ON chat_messages(sent_at);
	`},
	{version: 13, sql: `
		CREATE TABLE IF NOT EXISTS site_pages (
			author_key TEXT NOT NULL,
			slug       TEXT NOT NULL,
			title      TEXT NOT NULL,
			body       TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			sig        TEXT NOT NULL,
			PRIMARY KEY (author_key, slug)
		);
		CREATE INDEX IF NOT EXISTS idx_site_pages_author ON site_pages(author_key);
		CREATE TABLE IF NOT EXISTS site_tombstones (
			author_key TEXT NOT NULL,
			slug       TEXT NOT NULL,
			msg_json   TEXT NOT NULL,
			PRIMARY KEY (author_key, slug)
		);
	`},
}

type RawPost struct {
	ID, Category, AuthorName, AuthorKey, Title, Body, Sig string
	CreatedAt                                             time.Time
	ExpiresAt                                             time.Time
}

type RawReply struct {
	ID, PostID, AuthorName, AuthorKey, Body, Sig string
	CreatedAt                                    time.Time
}

func (s *DB) InsertPost(p RawPost) error {
	var expiresNano int64
	if !p.ExpiresAt.IsZero() {
		expiresNano = p.ExpiresAt.UnixNano()
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO posts(id,category,author_name,author_key,title,body,created_at,sig,expires_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Category, p.AuthorName, p.AuthorKey,
		p.Title, p.Body, p.CreatedAt.UnixNano(), p.Sig, expiresNano,
	)
	return err
}

func (s *DB) InsertReply(r RawReply) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO replies(id,post_id,author_name,author_key,body,created_at,sig)
		 VALUES(?,?,?,?,?,?,?)`,
		r.ID, r.PostID, r.AuthorName, r.AuthorKey,
		r.Body, r.CreatedAt.UnixNano(), r.Sig,
	)
	return err
}

func (s *DB) InsertTombstone(postID, msgJSON string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO tombstones(post_id, msg_json) VALUES(?, ?)`,
		postID, msgJSON,
	)
	return err
}

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

func (s *DB) DeletePost(postID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM replies WHERE post_id=?", postID); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	if _, err = tx.Exec("DELETE FROM posts WHERE id=?", postID); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	return tx.Commit()
}

// DeletePostWithTombstone, bir postu silme ve onun tombstone kaydını yazma
// işlemlerini TEK bir transaction'da yapar — DeletePost + InsertTombstone'u
// ayrı ayrı çağırmak, ikisi arasında bir çökme olursa "tombstone'suz silinmiş
// post" durumuna yol açabilirdi (bkz. forum.go: applyDelete).
func (s *DB) DeletePostWithTombstone(postID, msgJSON string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM replies WHERE post_id=?", postID); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	if _, err = tx.Exec("DELETE FROM posts WHERE id=?", postID); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO tombstones(post_id, msg_json) VALUES(?, ?)`, postID, msgJSON); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	return tx.Commit()
}

func (s *DB) DeleteExpiredPosts() (int, error) {
	now := time.Now().UnixNano()
	res, err := s.db.Exec(`
		DELETE FROM posts WHERE expires_at > 0 AND expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *DB) AllPosts() ([]RawPost, error) {
	rows, err := s.db.Query(
		`SELECT id,category,author_name,author_key,title,body,created_at,sig,expires_at
		 FROM posts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawPost
	for rows.Next() {
		var p RawPost
		var ts, expiresNano int64
		if err := rows.Scan(&p.ID, &p.Category, &p.AuthorName, &p.AuthorKey,
			&p.Title, &p.Body, &ts, &p.Sig, &expiresNano); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(0, ts).UTC()
		if expiresNano > 0 {
			p.ExpiresAt = time.Unix(0, expiresNano).UTC()
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

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

// RawChatMessage is a signed chat message persisted so it survives restarts
// and can be exchanged with peers who were offline when it was sent (see
// internal/chatlog's sync protocol, mirroring how RawPost is synced by
// internal/forum). AuthorKey/Sig may be empty for messages relayed from
// sources that don't sign (e.g. the plugin API's chat adapter, which
// predates this table) — those are never inserted here; only chatlog
// persists messages it could verify.
type RawChatMessage struct {
	ID, AuthorKey, AuthorName, Text, Sig string
	SentAt                               time.Time
	ExpiresAt                            time.Time
}

func (s *DB) InsertChatMessage(m RawChatMessage) error {
	var expiresNano int64
	if !m.ExpiresAt.IsZero() {
		expiresNano = m.ExpiresAt.UnixNano()
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO chat_messages(id,author_key,author_name,text,sent_at,expires_at,sig)
		 VALUES(?,?,?,?,?,?,?)`,
		m.ID, m.AuthorKey, m.AuthorName, m.Text, m.SentAt.UnixNano(), expiresNano, m.Sig,
	)
	return err
}

func (s *DB) AllChatMessages() ([]RawChatMessage, error) {
	rows, err := s.db.Query(
		`SELECT id,author_key,author_name,text,sent_at,expires_at,sig
		 FROM chat_messages ORDER BY sent_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawChatMessage
	for rows.Next() {
		var m RawChatMessage
		var ts, expiresNano int64
		if err := rows.Scan(&m.ID, &m.AuthorKey, &m.AuthorName, &m.Text, &ts, &expiresNano, &m.Sig); err != nil {
			return nil, err
		}
		m.SentAt = time.Unix(0, ts).UTC()
		if expiresNano > 0 {
			m.ExpiresAt = time.Unix(0, expiresNano).UTC()
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *DB) DeleteExpiredChatMessages() (int, error) {
	now := time.Now().UnixNano()
	res, err := s.db.Exec(`DELETE FROM chat_messages WHERE expires_at > 0 AND expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteChatMessage removes a single message by ID — used when
// chatlog.Store's MaxStoredMessages cap evicts the oldest entries.
func (s *DB) DeleteChatMessage(id string) error {
	_, err := s.db.Exec(`DELETE FROM chat_messages WHERE id=?`, id)
	return err
}

// RawProfile is a peer's self-published profile: a free-text bio, a short
// status line, a comma-joined set of category tags, an optional contact
// link, an optional GitHub username (proven via githubverify's OAuth Device
// Flow, not just self-typed like Link), and optionally a moderator
// certificate (ModCertJSON) that proves the author currently holds
// moderator status — verification of that cert, of Tags against
// forum.Categories, and of GitHubUser's shape, happens in the profile
// package, not here; storage just persists whatever bytes it's given.
type RawProfile struct {
	AuthorKey   string
	Bio         string
	Status      string
	Tags        string // virgülle ayrılmış kategori adları, ör. "Go,Rust"
	Link        string
	GitHubUser  string
	UpdatedAt   time.Time
	ModCertJSON string
	Sig         string
}

// UpsertProfile stores p, replacing any previous profile for the same
// AuthorKey. Callers must ensure p.UpdatedAt is newer than what's already
// stored — this method does not itself enforce "newer wins".
func (s *DB) UpsertProfile(p RawProfile) error {
	_, err := s.db.Exec(
		`INSERT INTO profiles(author_key,bio,status,tags,link,github_user,updated_at,mod_cert,sig) VALUES(?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(author_key) DO UPDATE SET bio=excluded.bio, status=excluded.status, tags=excluded.tags,
		 	link=excluded.link, github_user=excluded.github_user, updated_at=excluded.updated_at,
		 	mod_cert=excluded.mod_cert, sig=excluded.sig`,
		p.AuthorKey, p.Bio, p.Status, p.Tags, p.Link, p.GitHubUser, p.UpdatedAt.UnixNano(), p.ModCertJSON, p.Sig,
	)
	return err
}

func (s *DB) AllProfiles() ([]RawProfile, error) {
	rows, err := s.db.Query(`SELECT author_key,bio,status,tags,link,github_user,updated_at,mod_cert,sig FROM profiles`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawProfile
	for rows.Next() {
		var p RawProfile
		var ts int64
		if err := rows.Scan(&p.AuthorKey, &p.Bio, &p.Status, &p.Tags, &p.Link, &p.GitHubUser, &ts, &p.ModCertJSON, &p.Sig); err != nil {
			return nil, err
		}
		p.UpdatedAt = time.Unix(0, ts).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// RawSitePage is one page of a peer's self-published multi-page "kişisel
// site" (see internal/site) — unlike RawProfile (one row per author, whole-
// record replace), a peer may have several RawSitePage rows, one per Slug,
// mirroring RawPost's per-item shape rather than RawProfile's per-author
// shape.
type RawSitePage struct {
	AuthorKey, Slug, Title, Body, Sig string
	UpdatedAt                         time.Time
}

// UpsertSitePage stores p, replacing any previous page with the same
// (AuthorKey, Slug). Callers must ensure p.UpdatedAt is newer than what's
// already stored — this method does not itself enforce "newer wins" (same
// contract as UpsertProfile).
func (s *DB) UpsertSitePage(p RawSitePage) error {
	_, err := s.db.Exec(
		`INSERT INTO site_pages(author_key,slug,title,body,updated_at,sig) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(author_key,slug) DO UPDATE SET title=excluded.title, body=excluded.body,
		 	updated_at=excluded.updated_at, sig=excluded.sig`,
		p.AuthorKey, p.Slug, p.Title, p.Body, p.UpdatedAt.UnixNano(), p.Sig,
	)
	return err
}

func (s *DB) AllSitePages() ([]RawSitePage, error) {
	rows, err := s.db.Query(`SELECT author_key,slug,title,body,updated_at,sig FROM site_pages`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RawSitePage
	for rows.Next() {
		var p RawSitePage
		var ts int64
		if err := rows.Scan(&p.AuthorKey, &p.Slug, &p.Title, &p.Body, &ts, &p.Sig); err != nil {
			return nil, err
		}
		p.UpdatedAt = time.Unix(0, ts).UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteSitePageWithTombstone removes the (authorKey, slug) page and records
// its tombstone in a single transaction — mirrors DeletePostWithTombstone's
// rationale: doing the two writes separately could crash between them and
// leave a "deleted but no tombstone" row a later sync would resurrect.
func (s *DB) DeleteSitePageWithTombstone(authorKey, slug, msgJSON string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM site_pages WHERE author_key=? AND slug=?", authorKey, slug); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	if _, err = tx.Exec(
		`INSERT INTO site_tombstones(author_key,slug,msg_json) VALUES(?,?,?)
		 ON CONFLICT(author_key,slug) DO UPDATE SET msg_json=excluded.msg_json`,
		authorKey, slug, msgJSON,
	); err != nil {
		tx.Rollback() //nolint:errcheck
		return err
	}
	return tx.Commit()
}

func (s *DB) AllSiteTombstoneJSON() ([]string, error) {
	rows, err := s.db.Query(`SELECT msg_json FROM site_tombstones`)
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
