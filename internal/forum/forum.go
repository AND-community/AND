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

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/storage"
)

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

const postTTL = 5 * 24 * time.Hour

type Post struct {
	ID                 string    `json:"id"`
	Category           string    `json:"category"`
	AuthorName         string    `json:"author_name"`
	AuthorKey          string    `json:"author_key"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	CreatedAt          time.Time `json:"created_at"`
	Sig                string    `json:"sig"`
	Approved           bool      `json:"approved,omitempty"`
	ExpiresAt          time.Time `json:"expires_at,omitempty"`
	PermanentRequested bool      `json:"permanent_requested,omitempty"`
}

type Reply struct {
	ID         string    `json:"id"`
	PostID     string    `json:"post_id"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Sig        string    `json:"sig"`
}

type DeleteMsg struct {
	PostID    string    `json:"post_id"`
	AuthorKey string    `json:"author_key"`
	DeletedAt time.Time `json:"deleted_at"`
	Sig       string    `json:"sig"`
}

type envelope struct {
	Type   string     `json:"type"`
	Post   *Post      `json:"post,omitempty"`
	Reply  *Reply     `json:"reply,omitempty"`
	Delete *DeleteMsg `json:"delete,omitempty"`
}

type TrustedChecker interface {
	IsTrustedAuthor(authorKey string) bool
}

type Forum struct {
	mu      sync.Mutex
	posts   []*Post
	byID    map[string]*Post
	replies map[string][]*Reply
	deleted map[string]struct{}

	identity  *stdcrypto.Identity
	topic     *network.Topic
	db        *storage.DB
	rl        *rateLimiter
	checker   TrustedChecker

	newPosts   chan *Post
	newReplies chan *Reply
}

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
		newPosts:   make(chan *Post, 512),
		newReplies: make(chan *Reply, 512),
	}

	if err := f.load(); err != nil {
		db.Close()
		return nil, fmt.Errorf("forum: load: %w", err)
	}

	jsonPath := filepath.Join(filepath.Dir(dbPath), "forum.json")
	if err := f.migrateFromJSON(jsonPath); err != nil {
		fmt.Fprintf(os.Stderr, "forum: json migration: %v\n", err)
	}

	return f, nil
}

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

var categorySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Categories))
	for _, c := range Categories {
		m[c] = struct{}{}
	}
	return m
}()

func validCategory(cat string) bool {
	_, ok := categorySet[cat]
	return ok
}

func (f *Forum) handleMessage(data []byte) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case "post":
		p := env.Post
		if p == nil || len(p.Body) > rlMaxBodyBytes || len(p.Title) > 512 || !validCategory(p.Category) {
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

func (f *Forum) AllInMemoryPosts() []*Post {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*Post, len(f.posts))
	copy(out, f.posts)
	return out
}

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

func (f *Forum) Replies(postID string) []*Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*Reply(nil), f.replies[postID]...)
}

func (f *Forum) ReplyCount(postID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.replies[postID])
}

func (f *Forum) PostByID(id string) *Post {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id]
}

func (f *Forum) NewPosts() <-chan *Post { return f.newPosts }

func (f *Forum) NewReplies() <-chan *Reply { return f.newReplies }

func (f *Forum) storePost(p *Post) {
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
		return
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

func (f *Forum) ApprovePost(postID string) {
	_ = f.db.ApprovePost(postID)
	f.mu.Lock()
	if p, ok := f.byID[postID]; ok {
		p.Approved = true
		p.ExpiresAt = time.Time{}
	}
	f.mu.Unlock()
}

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

func (f *Forum) PendingPosts() ([]storage.PendingPost, error) {
	return f.db.PendingPosts()
}

func (f *Forum) RejectPost(postID string) error {
	if err := f.db.DeletePost(postID); err != nil {
		return err
	}
	f.mu.Lock()
	kept := f.posts[:0]
	for _, p := range f.posts {
		if p.ID != postID {
			kept = append(kept, p)
		}
	}
	f.posts = kept
	delete(f.byID, postID)
	delete(f.replies, postID)
	f.mu.Unlock()
	return nil
}

func (f *Forum) storeReply(r *Reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[r.PostID]; !ok {
		return
	}
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

func (f *Forum) load() error {
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

func (f *Forum) migrateFromJSON(jsonPath string) error {
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	type legacyStore struct {
		Posts   []*Post             `json:"posts"`
		Replies map[string][]*Reply `json:"replies"`
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
		_ = os.Rename(jsonPath, jsonPath+".migrated")
	}
	return nil
}

func postPayload(p *Post) []byte {
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
	return hex.EncodeToString(h[:16])
}

func deriveReplyID(r *Reply) string {
	h := sha256.Sum256([]byte(r.PostID + "|" + r.AuthorKey + "|" + r.Body + "|" + r.CreatedAt.String()))
	return hex.EncodeToString(h[:16])
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

	if ok {
		_ = f.db.DeletePost(postID)
	}
	if newTombstone {
		_ = f.db.InsertTombstone(postID, string(msgJSON))
	}
}

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
	_ = f.topic.Publish(ctx, data)
	f.applyDelete(postID, myKey, dJSON)
	return nil
}
