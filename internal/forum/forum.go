// Package forum implements AND's forum on top of the network package:
// composing/signing/verifying posts and replies, propagating them over
// ForumTopic, and persisting them locally in a SQLite database.
package forum

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	stdcrypto "and/internal/crypto"
	"and/internal/network"
	"and/internal/storage"
)

// Categories is the static list of forum sections.
var Categories = []string{
	"Python",
	"C / C++",
	"Rust",
	"Go",
	"JavaScript",
	"Java / Kotlin",
	"Yazılım",
	"Web",
	"Mobil",
	"Yapay Zeka",
	"Veritabanı",
	"DevOps",
	"Linux",
	"Bilişim",
	"Siber Güvenlik",
	"Donanım",
	"Oyun Geliştirme",
	"Açık Kaynak",
	"Kariyer",
	"Genel",
}

// postTTL is the default lifetime for posts that haven't been approved.
const postTTL = 5 * 24 * time.Hour

// Post is a top-level forum post.
type Post struct {
	ID                 string    `json:"id"`
	Category           string    `json:"category"`
	AuthorName         string    `json:"author_name"`
	AuthorKey          string    `json:"author_key"` // hex-encoded Ed25519 public key
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	CreatedAt          time.Time `json:"created_at"`
	Sig                string    `json:"sig"` // hex-encoded Ed25519 signature
	Approved           bool      `json:"approved,omitempty"`
	ExpiresAt          time.Time `json:"expires_at,omitempty"`
	PermanentRequested bool      `json:"permanent_requested,omitempty"` // kullanıcı kalıcılık talep etti
}

// Reply is a response to a Post.
type Reply struct {
	ID         string    `json:"id"`
	PostID     string    `json:"post_id"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Sig        string    `json:"sig"`
}

// DeleteMsg is published by the original author to remove a post from all peers.
type DeleteMsg struct {
	PostID    string    `json:"post_id"`
	AuthorKey string    `json:"author_key"`
	DeletedAt time.Time `json:"deleted_at"`
	Sig       string    `json:"sig"` // author Ed25519 signature
}

type envelope struct {
	Type   string     `json:"type"` // "post" | "reply" | "delete"
	Post   *Post      `json:"post,omitempty"`
	Reply  *Reply     `json:"reply,omitempty"`
	Delete *DeleteMsg `json:"delete,omitempty"`
}

// TrustedChecker is a minimal interface so forum doesn't import moderation.
type TrustedChecker interface {
	IsTrustedAuthor(authorKey string) bool
}

// Forum manages the local store and P2P propagation for posts and replies.
type Forum struct {
	mu      sync.Mutex
	posts   []*Post
	byID    map[string]*Post
	replies map[string][]*Reply // post ID → replies in order received
	deleted map[string]struct{} // tombstones: post ID'ler buraya girince bir daha eklenmez

	identity  *stdcrypto.Identity
	topic     *network.Topic
	db        *storage.DB
	rl        *rateLimiter
	checker   TrustedChecker // may be nil

	newPosts   chan *Post
	newReplies chan *Reply
}

// New creates a Forum backed by a SQLite database at dbPath.
// checker is optional (may be nil); when set, posts by trusted authors are
// auto-approved so they are never subject to the 5-day TTL.
func New(id *stdcrypto.Identity, topic *network.Topic, dbPath string, checker TrustedChecker) (*Forum, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("forum: open db: %w", err)
	}

	f := &Forum{
		byID:       make(map[string]*Post),
		replies:    make(map[string][]*Reply),
		deleted:    make(map[string]struct{}),
		identity:   id,
		topic:      topic,
		db:         db,
		rl:         newRateLimiter(filepath.Dir(dbPath)),
		checker:    checker,
		newPosts:   make(chan *Post, 64),
		newReplies: make(chan *Reply, 64),
	}

	if err := f.load(); err != nil {
		db.Close()
		return nil, fmt.Errorf("forum: load: %w", err)
	}

	// Eski forum.json varsa SQLite'a taşı.
	jsonPath := filepath.Join(filepath.Dir(dbPath), "forum.json")
	if err := f.migrateFromJSON(jsonPath); err != nil {
		fmt.Fprintf(os.Stderr, "forum: json migration: %v\n", err)
	}

	return f, nil
}

// Run listens for incoming forum messages until ctx is cancelled.
// Call it in a goroutine after New.
func (f *Forum) Run(ctx context.Context) {
	go f.runCleanup(ctx)
	go f.rl.saveLoop(ctx)
	ch := f.topic.Messages(ctx)
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			f.handleMessage(data)
		case <-ctx.Done():
			return
		}
	}
}

// runCleanup periodically removes expired unapproved posts from DB and memory.
func (f *Forum) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			f.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func (f *Forum) cleanup() {
	n, _ := f.db.DeleteExpiredPosts()
	if n == 0 {
		return
	}
	// Bellekteki listeden de kaldır
	now := time.Now()
	f.mu.Lock()
	kept := f.posts[:0]
	for _, p := range f.posts {
		if p.Approved || p.ExpiresAt.IsZero() || p.ExpiresAt.After(now) {
			kept = append(kept, p)
		} else {
			delete(f.byID, p.ID)
			delete(f.replies, p.ID)
		}
	}
	f.posts = kept
	f.mu.Unlock()
}

func (f *Forum) handleMessage(data []byte) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case "post":
		p := env.Post
		if p == nil || len(p.Body) > rlMaxBodyBytes {
			return
		}
		if !verifyPost(p) {
			return
		}
		if !f.rl.allowPost(p.AuthorKey) {
			return
		}
		f.storePost(p)
	case "reply":
		r := env.Reply
		if r == nil || len(r.Body) > rlMaxBodyBytes {
			return
		}
		if !verifyReply(r) {
			return
		}
		if !f.rl.allowReply(r.AuthorKey) {
			return
		}
		f.storeReply(r)
	case "delete":
		d := env.Delete
		if d == nil {
			return
		}
		if !verifyDeleteMsg(d) {
			return
		}
		dJSON, _ := json.Marshal(d)
		f.applyDelete(d.PostID, d.AuthorKey, dJSON)
	}
}

// CreatePost composes, signs, persists, and publishes a new post.
// permanentReq=true signals admin/mod to review whether this post should be kept permanently.
func (f *Forum) CreatePost(ctx context.Context, category, title, body string, permanentReq bool) (*Post, error) {
	name := f.identity.Name()
	if name == "" {
		name = "anonymous"
	}
	p := &Post{
		Category:           category,
		AuthorName:         name,
		AuthorKey:          hex.EncodeToString(f.identity.PublicKey()),
		Title:              title,
		Body:               body,
		CreatedAt:          time.Now().UTC(),
		PermanentRequested: permanentReq,
	}
	p.ID = derivePostID(p)
	sig := ed25519.Sign(f.identity.PrivateKey(), postPayload(p))
	p.Sig = hex.EncodeToString(sig)

	f.storePost(p)

	data, err := json.Marshal(envelope{Type: "post", Post: p})
	if err != nil {
		return nil, fmt.Errorf("forum: marshal post: %w", err)
	}
	if err := f.topic.Publish(ctx, data); err != nil {
		return nil, fmt.Errorf("forum: publish post: %w", err)
	}
	return p, nil
}

// CreateReply composes, signs, persists, and publishes a reply to postID.
func (f *Forum) CreateReply(ctx context.Context, postID, body string) (*Reply, error) {
	name := f.identity.Name()
	if name == "" {
		name = "anonymous"
	}
	r := &Reply{
		PostID:     postID,
		AuthorName: name,
		AuthorKey:  hex.EncodeToString(f.identity.PublicKey()),
		Body:       body,
		CreatedAt:  time.Now().UTC(),
	}
	r.ID = deriveReplyID(r)
	sig := ed25519.Sign(f.identity.PrivateKey(), replyPayload(r))
	r.Sig = hex.EncodeToString(sig)

	f.storeReply(r)

	data, err := json.Marshal(envelope{Type: "reply", Reply: r})
	if err != nil {
		return nil, fmt.Errorf("forum: marshal reply: %w", err)
	}
	if err := f.topic.Publish(ctx, data); err != nil {
		return nil, fmt.Errorf("forum: publish reply: %w", err)
	}
	return r, nil
}

// PostsByCategory returns all posts in category, newest first.
func (f *Forum) PostsByCategory(category string) []*Post {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Post
	for i := len(f.posts) - 1; i >= 0; i-- {
		if f.posts[i].Category == category {
			out = append(out, f.posts[i])
		}
	}
	return out
}

// PostCount returns the number of posts in category.
func (f *Forum) PostCount(category string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, p := range f.posts {
		if p.Category == category {
			n++
		}
	}
	return n
}

// Replies returns all replies to postID, oldest first.
func (f *Forum) Replies(postID string) []*Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*Reply(nil), f.replies[postID]...)
}

// ReplyCount returns the number of replies to postID.
func (f *Forum) ReplyCount(postID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.replies[postID])
}

// PostByID returns the post with the given ID, or nil if not found.
func (f *Forum) PostByID(id string) *Post {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id]
}

// NewPosts returns a channel that receives posts arriving from other peers.
func (f *Forum) NewPosts() <-chan *Post { return f.newPosts }

// NewReplies returns a channel that receives replies arriving from other peers.
func (f *Forum) NewReplies() <-chan *Reply { return f.newReplies }

func (f *Forum) storePost(p *Post) {
	// TTL / onay durumu: güvenilir yazar → kalıcı, değilse 5 günlük TTL
	if f.checker != nil && f.checker.IsTrustedAuthor(p.AuthorKey) {
		p.Approved = true
	} else if p.ExpiresAt.IsZero() {
		p.ExpiresAt = p.CreatedAt.Add(postTTL)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.byID[p.ID]; exists {
		return
	}
	if _, deleted := f.deleted[p.ID]; deleted {
		return // tombstoned — silinmiş post tekrar eklenmez
	}
	f.posts = append(f.posts, p)
	f.byID[p.ID] = p
	_ = f.db.InsertPost(storage.RawPost{
		ID: p.ID, Category: p.Category,
		AuthorName: p.AuthorName, AuthorKey: p.AuthorKey,
		Title: p.Title, Body: p.Body,
		CreatedAt:    p.CreatedAt, Sig: p.Sig,
		Approved:     p.Approved,
		ExpiresAt:    p.ExpiresAt,
		PermanentReq: p.PermanentRequested,
	})
	select {
	case f.newPosts <- p:
	default:
	}
}

// ApprovePost marks a post as permanently approved in DB and memory.
func (f *Forum) ApprovePost(postID string) {
	_ = f.db.ApprovePost(postID)
	f.mu.Lock()
	if p, ok := f.byID[postID]; ok {
		p.Approved = true
		p.ExpiresAt = time.Time{}
	}
	f.mu.Unlock()
}

// ApprovePostsByAuthor marks all posts by authorKey as approved.
func (f *Forum) ApprovePostsByAuthor(authorKey string) {
	_ = f.db.ApprovePostsByAuthor(authorKey)
	f.mu.Lock()
	for _, p := range f.posts {
		if p.AuthorKey == authorKey {
			p.Approved = true
			p.ExpiresAt = time.Time{}
		}
	}
	f.mu.Unlock()
}

// PendingPosts returns unapproved posts with an active TTL, soonest-expiring first.
func (f *Forum) PendingPosts() ([]storage.PendingPost, error) {
	return f.db.PendingPosts()
}

func (f *Forum) storeReply(r *Reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ex := range f.replies[r.PostID] {
		if ex.ID == r.ID {
			return
		}
	}
	f.replies[r.PostID] = append(f.replies[r.PostID], r)
	_ = f.db.InsertReply(storage.RawReply{
		ID: r.ID, PostID: r.PostID,
		AuthorName: r.AuthorName, AuthorKey: r.AuthorKey,
		Body: r.Body, CreatedAt: r.CreatedAt, Sig: r.Sig,
	})
	select {
	case f.newReplies <- r:
	default:
	}
}

// load reads all posts, replies, and tombstones from SQLite into the in-memory cache.
func (f *Forum) load() error {
	// Tombstone'ları önce yükle; böylece silinmiş postlar memory'e alınmaz.
	tombJSONs, err := f.db.AllTombstoneJSON()
	if err != nil {
		return fmt.Errorf("forum: load tombstones: %w", err)
	}
	for _, js := range tombJSONs {
		var d DeleteMsg
		if json.Unmarshal([]byte(js), &d) == nil && d.PostID != "" {
			f.deleted[d.PostID] = struct{}{}
		}
	}

	rawPosts, err := f.db.AllPosts()
	if err != nil {
		return err
	}
	now := time.Now()
	for _, rp := range rawPosts {
		// Süresi geçmiş ve onaysız postları belleğe yükleme
		if !rp.Approved && !rp.ExpiresAt.IsZero() && rp.ExpiresAt.Before(now) {
			continue
		}
		p := &Post{
			ID: rp.ID, Category: rp.Category,
			AuthorName: rp.AuthorName, AuthorKey: rp.AuthorKey,
			Title: rp.Title, Body: rp.Body,
			CreatedAt:          rp.CreatedAt, Sig: rp.Sig,
			Approved:           rp.Approved,
			ExpiresAt:          rp.ExpiresAt,
			PermanentRequested: rp.PermanentReq,
		}
		f.posts = append(f.posts, p)
		f.byID[p.ID] = p
	}

	rawReplies, err := f.db.AllReplies()
	if err != nil {
		return err
	}
	for _, rr := range rawReplies {
		r := &Reply{
			ID: rr.ID, PostID: rr.PostID,
			AuthorName: rr.AuthorName, AuthorKey: rr.AuthorKey,
			Body: rr.Body, CreatedAt: rr.CreatedAt, Sig: rr.Sig,
		}
		f.replies[r.PostID] = append(f.replies[r.PostID], r)
	}
	return nil
}

// migrateFromJSON imports posts and replies from a legacy forum.json file.
// Called once after the SQLite DB is loaded; no-op if the file doesn't exist
// or if it has already been migrated (data already in DB).
func (f *Forum) migrateFromJSON(jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	type legacyStore struct {
		Posts   []*Post              `json:"posts"`
		Replies map[string][]*Reply  `json:"replies"`
	}
	var legacy legacyStore
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	imported := 0
	for _, p := range legacy.Posts {
		if _, exists := f.byID[p.ID]; exists {
			continue
		}
		if !verifyPost(p) {
			continue
		}
		f.posts = append(f.posts, p)
		f.byID[p.ID] = p
		_ = f.db.InsertPost(storage.RawPost{
			ID: p.ID, Category: p.Category,
			AuthorName: p.AuthorName, AuthorKey: p.AuthorKey,
			Title: p.Title, Body: p.Body,
			CreatedAt: p.CreatedAt, Sig: p.Sig,
		})
		imported++
	}
	for postID, replies := range legacy.Replies {
		for _, r := range replies {
			dup := false
			for _, ex := range f.replies[postID] {
				if ex.ID == r.ID {
					dup = true
					break
				}
			}
			if dup || !verifyReply(r) {
				continue
			}
			f.replies[postID] = append(f.replies[postID], r)
			_ = f.db.InsertReply(storage.RawReply{
				ID: r.ID, PostID: r.PostID,
				AuthorName: r.AuthorName, AuthorKey: r.AuthorKey,
				Body: r.Body, CreatedAt: r.CreatedAt, Sig: r.Sig,
			})
			imported++
		}
	}

	if imported > 0 {
		// Eski dosyayı yedekle — bir daha okunmasın.
		_ = os.Rename(jsonPath, jsonPath+".migrated")
	}
	return nil
}

func postPayload(p *Post) []byte {
	// permanent_requested yalnızca true olduğunda payload'a eklenir.
	// Böylece önceki sürümle imzalanan (false) postlar doğrulamaya devam eder.
	// Relay false→true değişikliği yapamaz çünkü imza eşleşmez.
	s := p.ID + "|" + p.Category + "|" + p.AuthorKey + "|" + p.Title + "|" + p.Body + "|" + p.CreatedAt.String()
	if p.PermanentRequested {
		s += "|perm"
	}
	return []byte(s)
}

func replyPayload(r *Reply) []byte {
	return []byte(r.ID + "|" + r.PostID + "|" + r.AuthorKey + "|" + r.Body + "|" + r.CreatedAt.String())
}

func derivePostID(p *Post) string {
	h := sha256.Sum256([]byte(p.Category + "|" + p.AuthorKey + "|" + p.Title + "|" + p.Body + "|" + p.CreatedAt.String()))
	return hex.EncodeToString(h[:8])
}

func deriveReplyID(r *Reply) string {
	h := sha256.Sum256([]byte(r.PostID + "|" + r.AuthorKey + "|" + r.Body + "|" + r.CreatedAt.String()))
	return hex.EncodeToString(h[:8])
}

func verifyPost(p *Post) bool {
	pub, err := hex.DecodeString(p.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(p.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), postPayload(p), sig)
}

func verifyReply(r *Reply) bool {
	pub, err := hex.DecodeString(r.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(r.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), replyPayload(r), sig)
}

func deleteMsgPayload(d *DeleteMsg) []byte {
	return []byte(fmt.Sprintf("delete|%s|%s|%d", d.PostID, d.AuthorKey, d.DeletedAt.Unix()))
}

func verifyDeleteMsg(d *DeleteMsg) bool {
	pub, err := hex.DecodeString(d.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(d.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), deleteMsgPayload(d), sig)
}

// applyDelete removes a post from memory and DB if the authorKey matches,
// then records a tombstone so the post cannot be re-added via future syncs.
// msgJSON is the JSON-encoded DeleteMsg; it is stored as the tombstone payload.
func (f *Forum) applyDelete(postID, authorKey string, msgJSON []byte) {
	f.mu.Lock()
	p, ok := f.byID[postID]
	if ok {
		if p.AuthorKey != authorKey {
			f.mu.Unlock()
			return
		}
		delete(f.byID, postID)
		delete(f.replies, postID)
		kept := f.posts[:0]
		for _, post := range f.posts {
			if post.ID != postID {
				kept = append(kept, post)
			}
		}
		f.posts = kept
	}
	newTombstone := false
	if _, already := f.deleted[postID]; !already {
		f.deleted[postID] = struct{}{}
		newTombstone = true
	}
	f.mu.Unlock()

	// DB işlemleri mutex dışında: tek unlock path sayesinde net.
	if ok {
		_ = f.db.DeletePost(postID) // post memory'deyse DB'den de sil
	}
	if newTombstone {
		_ = f.db.InsertTombstone(postID, string(msgJSON))
	}
}

// DeleteOwnPost signs and broadcasts a delete message, then removes the post locally.
// Returns an error if the post doesn't exist or doesn't belong to this user.
func (f *Forum) DeleteOwnPost(ctx context.Context, postID string) error {
	myKey := hex.EncodeToString(f.identity.PublicKey())
	f.mu.Lock()
	p, ok := f.byID[postID]
	if !ok {
		f.mu.Unlock()
		return fmt.Errorf("forum: post not found")
	}
	if p.AuthorKey != myKey {
		f.mu.Unlock()
		return fmt.Errorf("forum: bu konu size ait değil")
	}
	f.mu.Unlock()

	now := time.Now().UTC()
	d := &DeleteMsg{PostID: postID, AuthorKey: myKey, DeletedAt: now}
	sig := ed25519.Sign(f.identity.PrivateKey(), deleteMsgPayload(d))
	d.Sig = hex.EncodeToString(sig)

	dJSON, err := json.Marshal(d)
	if err != nil {
		return err
	}
	data, err := json.Marshal(envelope{Type: "delete", Delete: d})
	if err != nil {
		return err
	}
	_ = f.topic.Publish(ctx, data) // hata olsa da yerel silme yapılır
	f.applyDelete(postID, myKey, dJSON)
	return nil
}
