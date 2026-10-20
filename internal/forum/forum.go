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
	"sync/atomic"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
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
	"C# / .NET",
	"PHP",
	"Swift / Objective-C",
	"Yazılım",
	"Web",
	"Mobil",
	"Yapay Zeka",
	"Veritabanı",
	"DevOps",
	"Bulut Bilişim",
	"Linux",
	"Bilişim",
	"Ağ ve Sistem Yönetimi",
	"Siber Güvenlik",
	"Donanım",
	"Gömülü Sistemler / IoT",
	"Robotik",
	"Blockchain / Web3",
	"Algoritma ve Veri Yapıları",
	"Test ve Kalite",
	"UI / UX Tasarım",
	"Oyun Geliştirme",
	"Açık Kaynak",
	"Kariyer",
	"Freelance",
	"Eğitim Kaynakları",
	"Etkinlikler ve Duyurular",
	"Genel",
}

const postTTL = 5 * 24 * time.Hour

// pendingDeleteTTL, bir postID için gelen ama henüz karşılığı yerel olarak
// bilinmeyen bir silme talebinin ne kadar süre "bekleyen/doğrulanmamış"
// olarak saklanacağını sınırlar — bkz. applyDelete ve storePost. Bu sınır
// olmasa, gerçek postu hiç göndermeyecek (ör. hedef ID'yi gözlemleyip
// tahmin eden) bir saldırganın taleplerinin belleği sınırsızca şişirmesi
// mümkün olurdu.
const pendingDeleteTTL = 24 * time.Hour

// pendingDelete, henüz yerel olarak bilinmeyen bir post için gelen bir
// silme talebini, gerçek post gelip yazar eşleşmesi doğrulanana kadar
// tutar. Bkz. applyDelete'in ok==false dalı ve storePost'un buradaki
// kaydı tüketen kontrolü.
type pendingDelete struct {
	authorKey string
	msgJSON   []byte
	at        time.Time
}

type Post struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Sig        string    `json:"sig"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
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

// ModDeleteMsg is a moderator- or founder-issued post deletion — unlike
// DeleteMsg (only the post's own author can sign one, verified via
// author-key match), this lets a moderator/founder remove ANY post
// network-wide, mirroring moderation.BanMsg's founder/cert dual-path
// pattern (Cert.ModeratorKey=="" marks the founder path).
type ModDeleteMsg struct {
	PostID    string                   `json:"post_id"`
	Reason    string                   `json:"reason,omitempty"`
	DeletedAt time.Time                `json:"deleted_at"`
	Cert      moderation.ModeratorCert `json:"cert"`
	Sig       string                   `json:"sig"`
}

type envelope struct {
	Type      string        `json:"type"`
	Post      *Post         `json:"post,omitempty"`
	Reply     *Reply        `json:"reply,omitempty"`
	Delete    *DeleteMsg    `json:"delete,omitempty"`
	ModDelete *ModDeleteMsg `json:"mod_delete,omitempty"`
}

// authorAgg is AuthorStats' incrementally-maintained per-author tally.
// Keeping this up to date on every store/delete turns AuthorStats from an
// O(posts+replies) scan (taken under the same lock everything else needs)
// into an O(1) map lookup — see storePost/storeReply/DeletePost/applyDelete/
// cleanup for the increment/decrement sides.
type authorAgg struct {
	postCount  int
	replyCount int
	joinDate   time.Time
}

type Forum struct {
	mu             sync.RWMutex
	posts          []*Post
	byID           map[string]*Post
	replies        map[string][]*Reply
	deleted        map[string]struct{}
	pendingDeletes map[string]*pendingDelete
	authorStats    map[string]*authorAgg

	identity *stdcrypto.Identity
	topic    *network.Topic
	db       *storage.DB
	rl       *rateLimiter
	mod      atomic.Pointer[moderation.Moderator]

	newPosts   chan *Post
	newReplies chan *Reply
}

// SetModerator bağlar (veya nil ile çözer) bir *moderation.Moderator'ı bu
// Forum'a — bağlıysa handleMessage, moderatörlerin verdiği ağ-seviyesi
// peer banlarını, o kişinin AuthorKey'i üzerinden forum içeriğine de
// uygular (bkz. moderation.AuthorKeyToPeerID/IsAuthorBanned'in doc yorumu:
// AND'de peer.ID ve AuthorKey aynı ed25519 anahtarından türetilir). New
// sonrası, Run'dan önce çağrılması yeterli; atomik olduğundan sonradan da
// güvenle değiştirilebilir. Ayarlanmazsa (nil kalırsa) davranış tamamen
// eskisiyle aynıdır — bu yüzden mevcut testler etkilenmez.
func (f *Forum) SetModerator(mod *moderation.Moderator) {
	f.mod.Store(mod)
}

func (f *Forum) authorBanned(authorKey string) bool {
	mod := f.mod.Load()
	return mod != nil && mod.IsAuthorBanned(authorKey)
}

func New(id *stdcrypto.Identity, topic *network.Topic, dbPath string) (*Forum, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("forum: open db: %w", err)
	}

	f := &Forum{
		byID:           make(map[string]*Post),
		replies:        make(map[string][]*Reply),
		deleted:        make(map[string]struct{}),
		pendingDeletes: make(map[string]*pendingDelete),
		authorStats:    make(map[string]*authorAgg),
		identity:       id,
		topic:          topic,
		db:             db,
		rl:             newRateLimiter(filepath.Dir(dbPath)),
		newPosts:       make(chan *Post, 512),
		newReplies:     make(chan *Reply, 512),
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
	if n > 0 {
		now := time.Now()
		f.mu.Lock()
		kept := f.posts[:0]
		for _, p := range f.posts {
			if p.ExpiresAt.IsZero() || p.ExpiresAt.After(now) {
				kept = append(kept, p)
			} else {
				delete(f.byID, p.ID)
				for _, r := range f.replies[p.ID] {
					f.decrAuthorReply(r.AuthorKey)
				}
				delete(f.replies, p.ID)
				f.decrAuthorPost(p.AuthorKey)
			}
		}
		f.posts = kept
		f.mu.Unlock()
	}

	// Süresi dolmuş bekleyen (doğrulanmamış) silme taleplerini temizle —
	// bkz. pendingDeleteTTL. n==0 olsa bile (silinecek post olmasa da) bu
	// adım her zaman çalışmalı, yoksa bu temizlik hiç tetiklenmeyebilir.
	cutoff := time.Now().Add(-pendingDeleteTTL)
	f.mu.Lock()
	for id, pd := range f.pendingDeletes {
		if pd.at.Before(cutoff) {
			delete(f.pendingDeletes, id)
		}
	}
	f.mu.Unlock()
}

// incrAuthorPost/incrAuthorReply/decrAuthorPost/decrAuthorReply maintain
// authorStats. Callers must hold f.mu (write-locked) — these are plain map
// mutations with no locking of their own.
func (f *Forum) incrAuthorPost(authorKey string, createdAt time.Time) {
	agg, ok := f.authorStats[authorKey]
	if !ok {
		agg = &authorAgg{}
		f.authorStats[authorKey] = agg
	}
	agg.postCount++
	if agg.joinDate.IsZero() || createdAt.Before(agg.joinDate) {
		agg.joinDate = createdAt
	}
}

func (f *Forum) incrAuthorReply(authorKey string, createdAt time.Time) {
	agg, ok := f.authorStats[authorKey]
	if !ok {
		agg = &authorAgg{}
		f.authorStats[authorKey] = agg
	}
	agg.replyCount++
	if agg.joinDate.IsZero() || createdAt.Before(agg.joinDate) {
		agg.joinDate = createdAt
	}
}

// decrAuthorPost/decrAuthorReply intentionally do NOT recompute joinDate on
// deletion (that would require rescanning the author's remaining activity —
// exactly the O(N) cost this index exists to avoid). joinDate may therefore
// keep reflecting a since-deleted post/reply's timestamp until the author's
// last remaining post/reply is also removed, at which point the whole entry
// is garbage-collected and joinDate correctly reports as unknown again.
func (f *Forum) decrAuthorPost(authorKey string) {
	agg, ok := f.authorStats[authorKey]
	if !ok {
		return
	}
	if agg.postCount > 0 {
		agg.postCount--
	}
	if agg.postCount == 0 && agg.replyCount == 0 {
		delete(f.authorStats, authorKey)
	}
}

func (f *Forum) decrAuthorReply(authorKey string) {
	agg, ok := f.authorStats[authorKey]
	if !ok {
		return
	}
	if agg.replyCount > 0 {
		agg.replyCount--
	}
	if agg.postCount == 0 && agg.replyCount == 0 {
		delete(f.authorStats, authorKey)
	}
}

var categorySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Categories))
	for _, c := range Categories {
		m[c] = struct{}{}
	}
	return m
}()

// ValidCategory reports whether cat is one of Categories. Exported so other
// packages (e.g. profile, validating self-chosen interest tags) can reuse
// the same closed set instead of duplicating or drifting from it.
func ValidCategory(cat string) bool {
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
		if p == nil || len(p.Body) > rlMaxBodyBytes || len(p.Title) > 512 || !ValidCategory(p.Category) {
			return
		}
		if f.rl.muted(p.AuthorKey) {
			// Israrlı ihlalci — imza doğrulaması dahil hiçbir işe girmeden atla.
			return
		}
		if f.authorBanned(p.AuthorKey) {
			// Bir moderatörün ağ-seviyesinde banladığı peer'ın AuthorKey'i —
			// bkz. SetModerator'ın doc yorumu.
			return
		}
		// ID/tombstone tekrarı kontrolü, allowPost()/isDuplicate()'ten ÖNCE
		// yapılıyor — kasıtlı sıralama: libp2p'nin kendi gossip "seen"
		// önbelleği zamanla dolar, bundan sonra biri daha önce yakaladığı
		// GEÇERLİ İMZALI bir postu (özel anahtara ihtiyaç duymadan) aynen
		// tekrar yayınlayabilir. Bu kontrol sonradan gelseydi (eskiden
		// storePost'un İÇİNDE, her şeyden SONRA yapılıyordu), her replay
		// kurbanın hız-sınırı/kopya-içerik kotasını gerçekten tüketir,
		// sonunda masum bir yazarı otomatik susturabilirdi.
		f.mu.RLock()
		_, exists := f.byID[p.ID]
		_, tombstoned := f.deleted[p.ID]
		f.mu.RUnlock()
		if exists || tombstoned {
			return
		}
		if !verifyPost(p) {
			return
		}
		if !f.rl.allowPost(p.AuthorKey, true) {
			f.rl.recordViolation(p.AuthorKey)
			return
		}
		if f.rl.isDuplicate(p.AuthorKey, p.Body) {
			// Not: kopya-içerik reddi kasıtlı olarak recordViolation'ı
			// TETİKLEMİYOR — dupMaxRepeats zaten her tekrarı ayrı ayrı
			// engelliyor; bunu ayrıca bir "ihlal" sayıp otomatik susturma
			// eşiğine eklemek, aynı kısa yanıtı birkaç konuda tekrarlayan
			// sıradan/masum bir kullanıcının tüm trafiğinin susturulmasına
			// yol açabilir. Gerçek flood/bot davranışı zaten allowPost'un
			// hız sınırını (recordViolation'ı TETİKLEYEN tek yol) aşar.
			return
		}
		f.storePost(p)
	case "reply":
		r := env.Reply
		if r == nil || len(r.Body) > rlMaxBodyBytes {
			return
		}
		if f.rl.muted(r.AuthorKey) {
			return
		}
		if f.authorBanned(r.AuthorKey) {
			return
		}
		// bkz. "post" case'indeki aynı sıralama açıklaması.
		f.mu.RLock()
		_, postExists := f.byID[r.PostID]
		replyExists := false
		for _, ex := range f.replies[r.PostID] {
			if ex.ID == r.ID {
				replyExists = true
				break
			}
		}
		f.mu.RUnlock()
		if !postExists || replyExists {
			return
		}
		if !verifyReply(r) {
			return
		}
		if !f.rl.allowReply(r.AuthorKey, true) {
			f.rl.recordViolation(r.AuthorKey)
			return
		}
		if f.rl.isDuplicate(r.AuthorKey, r.Body) {
			return
		}
		f.storeReply(r)
	case "delete":
		d := env.Delete
		if d == nil {
			return
		}
		if f.rl.muted(d.AuthorKey) {
			return
		}
		if f.authorBanned(d.AuthorKey) {
			return
		}
		if !verifyDeleteMsg(d) {
			return
		}
		if !f.rl.allowDelete(d.AuthorKey) {
			f.rl.recordViolation(d.AuthorKey)
			return
		}
		dJSON, _ := json.Marshal(d)
		f.applyDelete(d.PostID, d.AuthorKey, dJSON)
	case "mod_delete":
		md := env.ModDelete
		if md == nil {
			return
		}
		f.applyModDelete(md)
	}
}

func (f *Forum) CreatePost(ctx context.Context, category, title, body string) (*Post, error) {
	name := f.identity.Name()
	if name == "" {
		name = "anonymous"
	}
	p := &Post{
		Category:   category,
		AuthorName: name,
		AuthorKey:  hex.EncodeToString(f.identity.PublicKey()),
		Title:      title,
		Body:       body,
		CreatedAt:  time.Now().UTC(),
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
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Post, len(f.posts))
	copy(out, f.posts)
	return out
}

func (f *Forum) PostsByCategory(category string) []*Post {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var out []*Post
	for i := len(f.posts) - 1; i >= 0; i-- {
		if f.posts[i].Category == category {
			out = append(out, f.posts[i])
		}
	}
	return out
}

// TotalPosts, bellekteki konu sayısıdır (listeyi kopyalamadan).
func (f *Forum) TotalPosts() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.posts)
}

// CategoryCounts, her kategorideki konu sayısını TEK geçişte döner — ekranlar
// her kategori için ayrı ayrı PostCount çağırınca bütün konular kategori
// sayısı kadar (34 kez) taranıyordu.
func (f *Forum) CategoryCounts() map[string]int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]int, len(Categories))
	for _, p := range f.posts {
		out[p.Category]++
	}
	return out
}

func (f *Forum) PostCount(category string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	n := 0
	for _, p := range f.posts {
		if p.Category == category {
			n++
		}
	}
	return n
}

func (f *Forum) Replies(postID string) []*Reply {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]*Reply(nil), f.replies[postID]...)
}

func (f *Forum) ReplyCount(postID string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.replies[postID])
}

func (f *Forum) PostByID(id string) *Post {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.byID[id]
}

// DB exposes the underlying storage handle so other packages that need to
// persist their own data alongside the forum's (e.g. profile) can share the
// single SQLite connection rather than opening a second one on the same file.
func (f *Forum) DB() *storage.DB { return f.db }

// AuthorStats reports what the forum currently knows about authorKey's
// activity: how many topics and replies they've posted, and the earliest
// CreatedAt seen across either (the zero Time if we have nothing from
// them). O(1) — backed by the incrementally-maintained authorStats index,
// not a scan (see incrAuthorPost/incrAuthorReply and their decr counterparts).
func (f *Forum) AuthorStats(authorKey string) (postCount, replyCount int, joinDate time.Time) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	agg, ok := f.authorStats[authorKey]
	if !ok {
		return 0, 0, time.Time{}
	}
	return agg.postCount, agg.replyCount, agg.joinDate
}

func (f *Forum) NewPosts() <-chan *Post { return f.newPosts }

func (f *Forum) NewReplies() <-chan *Reply { return f.newReplies }

func (f *Forum) storePost(p *Post) {
	if p.ExpiresAt.IsZero() {
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
	if pd, pending := f.pendingDeletes[p.ID]; pending {
		delete(f.pendingDeletes, p.ID)
		if pd.authorKey == p.AuthorKey {
			// Bekleyen silme talebi gerçekten bu postun yazarından
			// geliyormuş (talep, gerçek post gelmeden önce ağda öne
			// geçmiş) — postu hiç saklamadan doğrudan tombstone'a
			// dönüştür.
			f.deleted[p.ID] = struct{}{}
			_ = f.db.InsertTombstone(p.ID, string(pd.msgJSON))
			return
		}
		// Yazar eşleşmiyor — talep sahte ya da başka bir posta aitmiş
		// (imza kendi içinde geçerli ama iddia ettiği yazar bu postun
		// gerçek yazarı değil). Görmezden gel, postu normal sakla.
	}
	f.posts = append(f.posts, p)
	f.byID[p.ID] = p
	f.incrAuthorPost(p.AuthorKey, p.CreatedAt)
	_ = f.db.InsertPost(storage.RawPost{
		ID: p.ID, Category: p.Category,
		AuthorName: p.AuthorName, AuthorKey: p.AuthorKey,
		Title: p.Title, Body: p.Body,
		CreatedAt: p.CreatedAt, Sig: p.Sig,
		ExpiresAt: p.ExpiresAt,
	})
	select {
	case f.newPosts <- p:
	default:
	}
}

// removePostLocked removes postID from f.posts/f.byID/f.replies and
// decrements author stats — caller must hold f.mu. Returns the removed
// post, or nil if postID wasn't known. Shared by DeletePost/applyDelete/
// applyModDelete so the three delete paths can never drift apart on what
// "remove a post from memory" actually means.
func (f *Forum) removePostLocked(postID string) *Post {
	p, existed := f.byID[postID]
	if !existed {
		return nil
	}
	delete(f.byID, postID)
	for _, r := range f.replies[postID] {
		f.decrAuthorReply(r.AuthorKey)
	}
	delete(f.replies, postID)
	f.decrAuthorPost(p.AuthorKey)
	kept := f.posts[:0]
	for _, post := range f.posts {
		if post.ID != postID {
			kept = append(kept, post)
		}
	}
	f.posts = kept
	return p
}

// DeletePost is a low-level, LOCAL-ONLY removal primitive — no signature,
// no network propagation, no tombstone. It does not survive a sync with
// any peer that still has the post (bkz. storePost'un f.deleted kontrolü,
// bu burada hiç set edilmiyor). Not a moderation action — see
// IssueModDelete for the real, network-wide, authorized delete.
func (f *Forum) DeletePost(postID string) error {
	if err := f.db.DeletePost(postID); err != nil {
		return err
	}
	f.mu.Lock()
	f.removePostLocked(postID)
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
	f.incrAuthorReply(r.AuthorKey, r.CreatedAt)
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
		if !rp.ExpiresAt.IsZero() && rp.ExpiresAt.Before(now) {
			continue
		}
		p := &Post{
			ID: rp.ID, Category: rp.Category,
			AuthorName: rp.AuthorName, AuthorKey: rp.AuthorKey,
			Title: rp.Title, Body: rp.Body,
			CreatedAt: rp.CreatedAt, Sig: rp.Sig,
			ExpiresAt: rp.ExpiresAt,
		}
		f.posts = append(f.posts, p)
		f.byID[p.ID] = p
		f.incrAuthorPost(p.AuthorKey, p.CreatedAt)
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
		f.incrAuthorReply(r.AuthorKey, r.CreatedAt)
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
		if p.ExpiresAt.IsZero() {
			p.ExpiresAt = p.CreatedAt.Add(postTTL)
		}
		f.posts = append(f.posts, p)
		f.byID[p.ID] = p
		f.incrAuthorPost(p.AuthorKey, p.CreatedAt)
		_ = f.db.InsertPost(storage.RawPost{
			ID: p.ID, Category: p.Category,
			AuthorName: p.AuthorName, AuthorKey: p.AuthorKey,
			Title: p.Title, Body: p.Body,
			CreatedAt: p.CreatedAt, Sig: p.Sig,
			ExpiresAt: p.ExpiresAt,
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
			f.incrAuthorReply(r.AuthorKey, r.CreatedAt)
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
	if !ok {
		// Post henüz yerel olarak bilinmiyor — silme talebinin iddia
		// ettiği authorKey'in gerçek postun yazarıyla eşleştiğini şu an
		// DOĞRULAYAMAYIZ (gerçek postun kendisi henüz elimizde değil).
		// Burada hemen kalıcı bir tombstone yazmak, herhangi birinin
		// kendi (geçerli ama alakasız) anahtarıyla imzaladığı bir talep
		// göndererek, gerçek post daha ulaşmadan onun postID'sini kalıcı
		// olarak "silinmiş" işaretlemesine izin verirdi — bu da postu,
		// hiçbir zaman geçerli bir sahibi olmadan sonsuza dek engeller
		// (bkz. storePost'un tombstone kontrolü). Bunun yerine, talebi
		// bekleyen/doğrulanmamış olarak sakla; storePost gerçek post
		// gelince yazar eşleşmesini kontrol edip karar verecek.
		f.pendingDeletes[postID] = &pendingDelete{
			authorKey: authorKey,
			msgJSON:   append([]byte(nil), msgJSON...),
			at:        time.Now(),
		}
		f.mu.Unlock()
		return
	}
	if p.AuthorKey != authorKey {
		f.mu.Unlock()
		return
	}
	f.removePostLocked(postID)
	delete(f.pendingDeletes, postID)

	newTombstone := false
	if _, already := f.deleted[postID]; !already {
		f.deleted[postID] = struct{}{}
		newTombstone = true
	}
	f.mu.Unlock()

	if newTombstone {
		// İkisi de gerekiyorsa tek transaction'da atomik olarak yapılır —
		// aradaki bir çökme "tombstone'suz silinmiş post" bırakmasın diye.
		_ = f.db.DeletePostWithTombstone(postID, string(msgJSON))
	} else {
		_ = f.db.DeletePost(postID)
	}
}

// ModDeletePayload deterministically encodes what a moderator-issued
// ModDeleteMsg's Sig must cover — shared by verifyModDelete and
// SignModeratorDelete.
func ModDeletePayload(postID, reason string, deletedAt time.Time) []byte {
	return []byte(fmt.Sprintf("moddelete|%s|%s|%d", postID, reason, deletedAt.Unix()))
}

// FounderDeletePayload deterministically encodes what a founder-issued
// ModDeleteMsg's Sig must cover — a distinct prefix from ModDeletePayload
// (and from deleteMsgPayload) so a signature valid for one can never be
// replayed as another.
func FounderDeletePayload(postID, reason string, deletedAt time.Time) []byte {
	return []byte(fmt.Sprintf("founder-moddelete|%s|%s|%d", postID, reason, deletedAt.Unix()))
}

// SignModeratorDelete builds a ModDeleteMsg signed by a moderator's own
// key (modPriv) using their granted cert.
func SignModeratorDelete(modPriv ed25519.PrivateKey, cert moderation.ModeratorCert, postID, reason string) ModDeleteMsg {
	deletedAt := time.Now().UTC()
	sig := ed25519.Sign(modPriv, ModDeletePayload(postID, reason, deletedAt))
	return ModDeleteMsg{PostID: postID, Reason: reason, DeletedAt: deletedAt, Cert: cert, Sig: hex.EncodeToString(sig)}
}

// SignFounderDelete builds a ModDeleteMsg signed directly by the founder's
// own key (founderPriv), with no ModeratorCert attached.
func SignFounderDelete(founderPriv ed25519.PrivateKey, postID, reason string) ModDeleteMsg {
	deletedAt := time.Now().UTC()
	sig := ed25519.Sign(founderPriv, FounderDeletePayload(postID, reason, deletedAt))
	return ModDeleteMsg{PostID: postID, Reason: reason, DeletedAt: deletedAt, Sig: hex.EncodeToString(sig)}
}

// verifyModDelete checks d's signature — either against the founder
// directly (Cert.ModeratorKey=="") or against a currently-valid (bkz.
// moderation.VerifyModeratorCert, iptal durumu dahil) moderator cert.
func (f *Forum) verifyModDelete(d *ModDeleteMsg) bool {
	mod := f.mod.Load()
	if mod == nil {
		return false
	}
	sig, err := hex.DecodeString(d.Sig)
	if err != nil {
		return false
	}
	if d.Cert.ModeratorKey == "" {
		return mod.VerifyFounderSignature(FounderDeletePayload(d.PostID, d.Reason, d.DeletedAt), sig)
	}
	if !mod.VerifyModeratorCert(d.Cert) {
		return false
	}
	modKey, err := hex.DecodeString(d.Cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(modKey), ModDeletePayload(d.PostID, d.Reason, d.DeletedAt), sig)
}

// applyModDelete validates d and, if it passes, tombstones postID locally
// — regardless of whether the post is currently known — and persists,
// reporting whether it was applied. Unlike applyDelete (self-delete), no
// author match is required: a valid moderator/founder signature
// authorizes deleting ANY post, and can pre-emptively tombstone a postID
// before the post itself has arrived, so a later gossip replay or sync
// can never resurrect it (bkz. storePost'un f.deleted kontrolü). Shared
// by handleMessage's "mod_delete" case (network-received) and
// IssueModDelete (locally-originated), mirroring moderation.applyBan.
func (f *Forum) applyModDelete(d *ModDeleteMsg) bool {
	if d.PostID == "" || d.Sig == "" || len(d.Reason) > 256 {
		return false
	}
	if !f.verifyModDelete(d) {
		return false
	}

	// Bu tombstone zaten uygulanmışsa (ör. sync her seferinde tüm geçmişi
	// tekrar gönderdiği için tekrar tekrar gelir), aşağıdaki hız sınırından
	// hiç geçirmeden erken çık — aksi halde eski, zaten no-op olan
	// silmelerin tekrar oynatılması, gerçek yeni bir silme için gereken
	// moderatör/kurucu kotasını (rlMaxDeletes) tüketir.
	f.mu.RLock()
	_, already := f.deleted[d.PostID]
	f.mu.RUnlock()
	if already {
		return true
	}

	rateLimitKey := d.Cert.ModeratorKey
	if rateLimitKey == "" {
		rateLimitKey = moderation.FounderPubKeyHex
	}
	// "mod:" öneki, bu kotayı aynı kişinin kendi postlarını silerken
	// tükettiği self-delete kotasından (handleMessage'daki "delete" dalı,
	// f.rl.allowDelete(d.AuthorKey) — önek YOK) ayrı bir kovaya koyar. Aynı
	// pubkey iki farklı yetkiyle (sıradan yazar + moderatör/kurucu) hareket
	// edebildiğinden, önek olmadan bir moderatörün kendi eski postlarını
	// temizlemesi, az sonra ihtiyaç duyacağı moderasyon silme kotasını
	// tüketmiş olurdu.
	if !f.rl.allowDelete("mod:" + rateLimitKey) {
		return false
	}

	msgJSON, err := json.Marshal(d)
	if err != nil {
		return false
	}

	f.mu.Lock()
	if _, already := f.deleted[d.PostID]; already {
		f.mu.Unlock()
		return true
	}
	f.deleted[d.PostID] = struct{}{}
	existed := f.removePostLocked(d.PostID) != nil
	delete(f.pendingDeletes, d.PostID)
	f.mu.Unlock()

	if existed {
		_ = f.db.DeletePostWithTombstone(d.PostID, string(msgJSON))
	} else {
		_ = f.db.InsertTombstone(d.PostID, string(msgJSON))
	}
	return true
}

// IssueModDelete signs and issues a moderator/founder-authorized deletion
// of postID, originated by THIS node — either as founder (cert==nil) or
// as a moderator (cert!=nil), mirroring moderation's ban/unban issuance
// pattern. Applies locally first (so it takes effect immediately, without
// waiting for GossipSub's self-echo), then publishes.
func (f *Forum) IssueModDelete(ctx context.Context, postID, reason string, isFounder bool, cert *moderation.ModeratorCert) error {
	var d ModDeleteMsg
	switch {
	case isFounder:
		d = SignFounderDelete(f.identity.PrivateKey(), postID, reason)
	case cert != nil:
		d = SignModeratorDelete(f.identity.PrivateKey(), *cert, postID, reason)
	default:
		return fmt.Errorf("forum: moderatör yetkisi bulunamadı")
	}
	if !f.applyModDelete(&d) {
		return fmt.Errorf("forum: silme reddedildi (imza, sertifika ya da hız sınırı geçersiz)")
	}
	data, err := json.Marshal(envelope{Type: "mod_delete", ModDelete: &d})
	if err != nil {
		return fmt.Errorf("forum: silme mesajı kodlanamadı: %w", err)
	}
	if err := f.topic.Publish(ctx, data); err != nil {
		return fmt.Errorf("forum: silme yayınlanamadı: %w", err)
	}
	return nil
}

func (f *Forum) DeleteOwnPost(ctx context.Context, postID string) error {
	myKey := hex.EncodeToString(f.identity.PublicKey())
	f.mu.RLock()
	p, ok := f.byID[postID]
	if !ok {
		f.mu.RUnlock()
		return fmt.Errorf("forum: post not found")
	}
	if p.AuthorKey != myKey {
		f.mu.RUnlock()
		return fmt.Errorf("forum: bu konu size ait değil")
	}
	f.mu.RUnlock()

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
