package pluginapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lucian95511/and/internal/ui"
)

// ─── Mock backends ────────────────────────────────────────────────────────────

type mockID struct {
	name      string
	pubKeyHex string
	peerID    string
	founder   bool
	moderator bool
}

func (m *mockID) Name() string               { return m.name }
func (m *mockID) PubKeyHex() string          { return m.pubKeyHex }
func (m *mockID) PeerIDStr() string          { return m.peerID }
func (m *mockID) IsFounder() bool            { return m.founder }
func (m *mockID) IsModerator() bool          { return m.moderator }
func (m *mockID) ConnectedPeers() []PeerInfo { return nil }

type deletedPost struct {
	postID string
	reason string
}

type mockForum struct {
	created   []CreatePostReq
	createErr error
	deleted   []deletedPost
	deleteErr error
}

func (m *mockForum) CreatePost(_ context.Context, cat, title, body string) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, CreatePostReq{Category: cat, Title: title, Body: body})
	return nil
}
func (m *mockForum) AllPosts() ([]PostSummary, error) { return nil, nil }
func (m *mockForum) DeletePost(_ context.Context, postID, reason string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleted = append(m.deleted, deletedPost{postID: postID, reason: reason})
	return nil
}
func (m *mockForum) GetPost(id string) (*PostDetail, error) {
	return &PostDetail{ID: id, Replies: []ReplyInfo{}}, nil
}
func (m *mockForum) CreateReply(_ context.Context, _, _ string) error { return nil }

type publishedSitePage struct {
	slug, title, body string
}

type mockSite struct {
	pages       []SitePageInfo
	published   []publishedSitePage
	publishErr  error
	deletedSlug []string
	deleteErr   error
}

func (m *mockSite) MyPages() ([]SitePageInfo, error) { return m.pages, nil }
func (m *mockSite) PublishPage(_ context.Context, slug, title, body string) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, publishedSitePage{slug: slug, title: title, body: body})
	return nil
}
func (m *mockSite) DeletePage(_ context.Context, slug string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedSlug = append(m.deletedSlug, slug)
	return nil
}

type mockModeration struct {
	bans      []BanEntryInfo
	unbanned  []string
	unbanErr  error
	history   []HistoryEntryInfo
	grants    []GrantedModInfo
	grantErr  error
	grantPath string
	revoked   []string
	revokeErr error
}

func (m *mockModeration) AllBans() ([]BanEntryInfo, error) { return m.bans, nil }
func (m *mockModeration) Unban(_ context.Context, authorKey string) error {
	if m.unbanErr != nil {
		return m.unbanErr
	}
	m.unbanned = append(m.unbanned, authorKey)
	return nil
}
func (m *mockModeration) History() ([]HistoryEntryInfo, error)   { return m.history, nil }
func (m *mockModeration) GrantedMods() ([]GrantedModInfo, error) { return m.grants, nil }
func (m *mockModeration) Grant(_ context.Context, targetKeyHex string, _ time.Duration, _ bool) (string, error) {
	if m.grantErr != nil {
		return "", m.grantErr
	}
	m.grants = append(m.grants, GrantedModInfo{Key: targetKeyHex})
	return m.grantPath, nil
}
func (m *mockModeration) Revoke(_ context.Context, targetKeyHex string) error {
	if m.revokeErr != nil {
		return m.revokeErr
	}
	m.revoked = append(m.revoked, targetKeyHex)
	return nil
}

type mockDM struct {
	subs    []chan DMMsg
	sendErr error
}

func (m *mockDM) Subscribe() chan DMMsg {
	ch := make(chan DMMsg, 8)
	m.subs = append(m.subs, ch)
	return ch
}
func (m *mockDM) Unsubscribe(ch chan DMMsg) {
	for i, c := range m.subs {
		if c == ch {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}
func (m *mockDM) SendDM(_ context.Context, _, _, _ string) error { return m.sendErr }

// ─── Helpers ──────────────────────────────────────────────────────────────────

func startTestServer(t *testing.T, id IdentityBackend, f ForumBackend, dm DMBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	srv := NewServer(id, f, nil, dm, nil, nil, nil, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

// ─── Identity & Role ──────────────────────────────────────────────────────────

func TestGetIdentity(t *testing.T) {
	id := &mockID{name: "alice", pubKeyHex: "aabbcc", peerID: "12D3KooWTest"}
	c := startTestServer(t, id, &mockForum{}, nil)

	info, err := c.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "alice" {
		t.Errorf("Name: got %q, want %q", info.Name, "alice")
	}
	if info.PubKey != "aabbcc" {
		t.Errorf("PubKey: got %q, want %q", info.PubKey, "aabbcc")
	}
	if info.PeerID != "12D3KooWTest" {
		t.Errorf("PeerID: got %q, want %q", info.PeerID, "12D3KooWTest")
	}
}

func TestGetRole_Founder(t *testing.T) {
	id := &mockID{founder: true, moderator: true}
	c := startTestServer(t, id, &mockForum{}, nil)

	role, err := c.Role()
	if err != nil {
		t.Fatal(err)
	}
	if !role.IsFounder {
		t.Error("expected IsFounder=true")
	}
	if !role.IsModerator {
		t.Error("expected IsModerator=true")
	}
}

func TestGetRole_Regular(t *testing.T) {
	id := &mockID{}
	c := startTestServer(t, id, &mockForum{}, nil)

	role, err := c.Role()
	if err != nil {
		t.Fatal(err)
	}
	if role.IsFounder || role.IsModerator {
		t.Errorf("expected no roles, got %+v", role)
	}
}

// ─── Forum endpoints ──────────────────────────────────────────────────────────

// testPostID is a valid 32-char hex post ID used in tests.
const testPostID = "a1b2c3d4e5f60718a9b0c1d2e3f4a5b6"

// founderID returns a mockID representing a founder (required for protected endpoints).
func founderID() *mockID { return &mockID{founder: true, moderator: true} }

func TestCreatePost(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, &mockID{}, f, nil)

	if err := c.CreatePost("genel", "Test Başlık", "Test içerik"); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 {
		t.Fatalf("expected 1 created post, got %d", len(f.created))
	}
	p := f.created[0]
	if p.Category != "genel" || p.Title != "Test Başlık" {
		t.Errorf("unexpected post: %+v", p)
	}
}

func TestCreatePost_Error(t *testing.T) {
	f := &mockForum{createErr: errors.New("kategori bulunamadı")}
	c := startTestServer(t, &mockID{}, f, nil)

	if err := c.CreatePost("yok", "x", "y"); err == nil {
		t.Fatal("expected error")
	}
}

// TestCreatePost_RateLimited, bir eklentinin (hatalı ya da kötü niyetli)
// dakikada contentActionLimit'ten fazla gönderi oluşturmaya çalışırsa
// reddedildiğini doğrular — bkz. ratelimit.go'daki localActionLimiter'ın
// doc yorumu: bu sınır olmadan böyle bir eklenti, düğümün kendi GEÇERLİ
// imzasıyla ağı hızlıca doldurabilirdi.
func TestCreatePost_RateLimited(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, &mockID{}, f, nil)

	for i := 0; i < contentActionLimit; i++ {
		if err := c.CreatePost("genel", "başlık", "gövde"); err != nil {
			t.Fatalf("istek %d sınır (%d) içinde olmalıydı ama reddedildi: %v", i+1, contentActionLimit, err)
		}
	}
	if err := c.CreatePost("genel", "başlık", "gövde"); err == nil {
		t.Fatalf("sınır (%d) aşıldıktan sonraki istek reddedilmeliydi", contentActionLimit)
	}
	if len(f.created) != contentActionLimit {
		t.Fatalf("backend'e yalnızca sınır kadar (%d) çağrı ulaşmalıydı, %d ulaştı", contentActionLimit, len(f.created))
	}
}

// ─── Site endpoints (Eklentiler/site eklentisi) ───────────────────────────────
//
// Her uç nokta ÖRTÜK olarak çağıranın kendi kimliğine sınırlıdır (bkz.
// SiteBackend'in doc yorumu) — forum'un getAllPosts/postDeletePost'unun
// aksine burada rol denetimi YOKTUR: kendi sayfalarını yönetmek, foruma
// gönderi atmaktan daha fazla yetki gerektirmez.

func startTestServerWithSite(t *testing.T, site SiteBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(&mockID{}, &mockForum{}, site, nil, nil, nil, nil, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

func TestSitePages_ReturnsOwnPages(t *testing.T) {
	s := &mockSite{pages: []SitePageInfo{{Slug: "home", Title: "Ana Sayfa", Body: "merhaba"}}}
	c := startTestServerWithSite(t, s)

	pages, err := c.SitePages()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Slug != "home" || pages[0].Title != "Ana Sayfa" {
		t.Fatalf("unexpected pages: %+v", pages)
	}
}

func TestSitePages_EmptyIsEmptyArrayNotNull(t *testing.T) {
	c := startTestServerWithSite(t, &mockSite{})
	pages, err := c.SitePages()
	if err != nil {
		t.Fatal(err)
	}
	if pages == nil || len(pages) != 0 {
		t.Fatalf("expected empty (non-nil) slice, got %+v", pages)
	}
}

func TestSitePages_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if _, err := c.SitePages(); err == nil {
		t.Fatal("expected error with nil site backend")
	}
}

func TestPublishSitePage_Success(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	if err := c.PublishSitePage("home", "Başlık", "gövde"); err != nil {
		t.Fatal(err)
	}
	if len(s.published) != 1 {
		t.Fatalf("expected 1 published page, got %d", len(s.published))
	}
	p := s.published[0]
	if p.slug != "home" || p.title != "Başlık" || p.body != "gövde" {
		t.Errorf("unexpected publish: %+v", p)
	}
}

func TestPublishSitePage_EmptySlugRejected(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	if err := c.PublishSitePage("", "t", "b"); err == nil {
		t.Fatal("expected 400 for empty slug")
	}
	if len(s.published) != 0 {
		t.Fatal("backend should never be called for a rejected request")
	}
}

func TestPublishSitePage_BackendErrorSurfaces(t *testing.T) {
	s := &mockSite{publishErr: errors.New("en fazla 20 sayfan olabilir")}
	c := startTestServerWithSite(t, s)

	if err := c.PublishSitePage("home", "t", "b"); err == nil {
		t.Fatal("expected error from backend")
	}
}

func TestPublishSitePage_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.PublishSitePage("home", "t", "b"); err == nil {
		t.Fatal("expected error with nil site backend")
	}
}

func TestPublishSitePage_RateLimited(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	for i := 0; i < contentActionLimit; i++ {
		if err := c.PublishSitePage("home", "t", "b"); err != nil {
			t.Fatalf("istek %d sınır içinde olmalıydı ama reddedildi: %v", i+1, err)
		}
	}
	if err := c.PublishSitePage("home", "t", "b"); err == nil {
		t.Fatal("sınır aşıldıktan sonraki istek reddedilmeliydi")
	}
}

// TestSiteRateLimit_DoesNotShareBudgetWithForum, forumLimiter/siteLimiter'ın
// KASITLI OLARAK AYRI örnekler olduğunu doğrular (bkz. Server struct'ındaki
// doc yorumu): hatalı/kötü niyetli bir eklentinin site/publish'i kötüye
// kullanıp KENDİ bütçesini tüketmesi, İLGİSİZ forum/post uç noktasını hiç
// etkilememelidir — startTestServerWithSite AYNI Server örneğinde hem bir
// mockForum hem site backend'i barındırdığından bu ikisi arasındaki
// izolasyonu gerçek HTTP katmanı üzerinden kanıtlar.
func TestSiteRateLimit_DoesNotShareBudgetWithForum(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	for i := 0; i < contentActionLimit; i++ {
		if err := c.PublishSitePage("home", "t", "b"); err != nil {
			t.Fatalf("kurulum: istek %d başarılı olmalıydı: %v", i+1, err)
		}
	}
	if err := c.PublishSitePage("home", "t", "b"); err == nil {
		t.Fatal("kurulum: site bütçesi burada tükenmiş olmalıydı")
	}

	// site/publish'in bütçesi tükendi — forum/post AYRI bir bütçeye sahip
	// olduğundan hâlâ çalışmalı.
	if err := c.CreatePost("genel", "başlık", "gövde"); err != nil {
		t.Fatalf("forum/post, site'ın hız sınırından ETKİLENMEMELİYDİ, ama hata verdi: %v", err)
	}
}

func TestDeleteSitePage_Success(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	if err := c.DeleteSitePage("home"); err != nil {
		t.Fatal(err)
	}
	if len(s.deletedSlug) != 1 || s.deletedSlug[0] != "home" {
		t.Fatalf("unexpected delete calls: %+v", s.deletedSlug)
	}
}

func TestDeleteSitePage_EmptySlugRejected(t *testing.T) {
	s := &mockSite{}
	c := startTestServerWithSite(t, s)

	if err := c.DeleteSitePage(""); err == nil {
		t.Fatal("expected 400 for empty slug")
	}
	if len(s.deletedSlug) != 0 {
		t.Fatal("backend should never be called for a rejected request")
	}
}

func TestDeleteSitePage_BackendErrorSurfaces(t *testing.T) {
	s := &mockSite{deleteErr: errors.New("böyle bir sayfa yok")}
	c := startTestServerWithSite(t, s)

	if err := c.DeleteSitePage("home"); err == nil {
		t.Fatal("expected error from backend")
	}
}

func TestDeleteSitePage_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.DeleteSitePage("home"); err == nil {
		t.Fatal("expected error with nil site backend")
	}
}

// ─── DeletePost: yalnızca kurucu/moderatör ────────────────────────────────────

func TestDeletePost_RegularUserRejected(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, &mockID{}, f, nil) // ne founder ne moderator

	if err := c.DeletePost(testPostID); err == nil {
		t.Fatal("sıradan bir kullanıcının silme isteği kabul edilmemeliydi")
	}
	if len(f.deleted) != 0 {
		t.Fatalf("backend'e hiç ulaşmamalıydı, got %+v", f.deleted)
	}
}

func TestDeletePost_FounderAllowed(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.DeletePost(testPostID); err != nil {
		t.Fatalf("kurucu silebilmeli: %v", err)
	}
	if len(f.deleted) != 1 || f.deleted[0].postID != testPostID {
		t.Fatalf("beklenmeyen silme kaydı: %+v", f.deleted)
	}
}

func TestDeletePost_ModeratorAllowed(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, &mockID{moderator: true}, f, nil)

	if err := c.DeletePost(testPostID); err != nil {
		t.Fatalf("moderatör silebilmeli: %v", err)
	}
	if len(f.deleted) != 1 {
		t.Fatalf("beklenmeyen silme kaydı: %+v", f.deleted)
	}
}

func TestDeletePost_ReasonPassesThrough(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.post("/api/v1/forum/delete", DeletePostReq{PostID: testPostID, Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || f.deleted[0].reason != "spam" {
		t.Fatalf("sebep backend'e iletilmeliydi, got %+v", f.deleted)
	}
}

func TestDeletePost_InvalidPostIDRejected(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.post("/api/v1/forum/delete", DeletePostReq{PostID: "../../etc/passwd"}); err == nil {
		t.Fatal("geçersiz post_id biçimi reddedilmeliydi")
	}
	if len(f.deleted) != 0 {
		t.Fatalf("backend'e hiç ulaşmamalıydı, got %+v", f.deleted)
	}
}

func TestDeletePost_BackendErrorSurfaces(t *testing.T) {
	f := &mockForum{deleteErr: errors.New("silme reddedildi")}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.DeletePost(testPostID); err == nil {
		t.Fatal("backend hatası istemciye ulaşmalıydı")
	}
}

// ─── Moderasyon (mod/*) ─────────────────────────────────────────────────────

func TestAllBans_RegularUserRejected(t *testing.T) {
	mb := &mockModeration{bans: []BanEntryInfo{{PeerID: "p1"}}}
	c := startTestServerWithMod(t, &mockID{}, mb)

	if _, err := c.AllBans(); err == nil {
		t.Fatal("sıradan bir kullanıcı ban listesini görebilmemeliydi")
	}
}

func TestAllBans_ModeratorAllowed(t *testing.T) {
	mb := &mockModeration{bans: []BanEntryInfo{{PeerID: "p1", Reason: "spam"}}}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	bans, err := c.AllBans()
	if err != nil {
		t.Fatalf("moderatör ban listesini görebilmeli: %v", err)
	}
	if len(bans) != 1 || bans[0].PeerID != "p1" {
		t.Fatalf("beklenmeyen liste: %+v", bans)
	}
}

func TestUnban_ModeratorAllowed(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	key := strings.Repeat("aabbccdd", 8)
	if err := c.Unban(key); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	if len(mb.unbanned) != 1 || mb.unbanned[0] != key {
		t.Fatalf("beklenmeyen unban kaydı: %+v", mb.unbanned)
	}
}

func TestUnban_InvalidKeyRejectedAtAPIBoundary(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	if err := c.Unban("aabbcc"); err == nil {
		t.Fatal("kısa/hex-olmayan bir author_key kabul edilmemeliydi")
	}
	if len(mb.unbanned) != 0 {
		t.Fatalf("geçersiz biçim backend'e hiç ulaşmamalıydı, got %+v", mb.unbanned)
	}
}

func TestUnban_RegularUserRejected(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, &mockID{}, mb)

	if err := c.Unban("aabbcc"); err == nil {
		t.Fatal("sıradan bir kullanıcı unban yapabilmemeliydi")
	}
	if len(mb.unbanned) != 0 {
		t.Fatalf("backend'e hiç ulaşmamalıydı, got %+v", mb.unbanned)
	}
}

func TestModHistory_ModeratorAllowed(t *testing.T) {
	mb := &mockModeration{history: []HistoryEntryInfo{{Action: "ban", PeerID: "p1"}}}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	h, err := c.ModHistory()
	if err != nil {
		t.Fatalf("ModHistory: %v", err)
	}
	if len(h) != 1 || h[0].Action != "ban" {
		t.Fatalf("beklenmeyen geçmiş: %+v", h)
	}
}

func TestGrantedMods_ModeratorRejectedFounderOnly(t *testing.T) {
	mb := &mockModeration{grants: []GrantedModInfo{{Key: "x"}}}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	if _, err := c.GrantedMods(); err == nil {
		t.Fatal("Moderatörler listesi yalnızca kurucuya açık olmalıydı")
	}
}

func TestGrantedMods_FounderAllowed(t *testing.T) {
	mb := &mockModeration{grants: []GrantedModInfo{{Key: "x"}}}
	c := startTestServerWithMod(t, founderID(), mb)

	g, err := c.GrantedMods()
	if err != nil {
		t.Fatalf("GrantedMods: %v", err)
	}
	if len(g) != 1 || g[0].Key != "x" {
		t.Fatalf("beklenmeyen liste: %+v", g)
	}
}

func TestGrantModerator_FounderOnly(t *testing.T) {
	mb := &mockModeration{grantPath: "/tmp/mod_aabbcc.json"}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	if _, err := c.GrantModerator("aabbcc", time.Hour, false); err == nil {
		t.Fatal("moderatör atama yalnızca kurucuya açık olmalıydı")
	}
	if len(mb.grants) != 0 {
		t.Fatalf("backend'e hiç ulaşmamalıydı, got %+v", mb.grants)
	}
}

func TestGrantModerator_FounderIssuesAndReceivesPath(t *testing.T) {
	mb := &mockModeration{grantPath: "/tmp/mod_aabbcc.json"}
	c := startTestServerWithMod(t, founderID(), mb)

	key := strings.Repeat("aabbccdd", 8)
	path, err := c.GrantModerator(key, 24*time.Hour, false)
	if err != nil {
		t.Fatalf("GrantModerator: %v", err)
	}
	if path != "/tmp/mod_aabbcc.json" {
		t.Fatalf("cert yolu iletilmedi: got %q", path)
	}
	if len(mb.grants) != 1 || mb.grants[0].Key != key {
		t.Fatalf("beklenmeyen grant kaydı: %+v", mb.grants)
	}
}

func TestGrantModerator_InvalidKeyRejectedAtAPIBoundary(t *testing.T) {
	mb := &mockModeration{grantPath: "/tmp/mod_aabbcc.json"}
	c := startTestServerWithMod(t, founderID(), mb)

	if _, err := c.GrantModerator("aabbcc", time.Hour, false); err == nil {
		t.Fatal("kısa/hex-olmayan bir target_key_hex kabul edilmemeliydi")
	}
	if len(mb.grants) != 0 {
		t.Fatalf("geçersiz biçim backend'e hiç ulaşmamalıydı, got %+v", mb.grants)
	}
}

func TestRevokeModerator_InvalidKeyRejectedAtAPIBoundary(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, founderID(), mb)

	if err := c.RevokeModerator("aabbcc"); err == nil {
		t.Fatal("kısa/hex-olmayan bir target_key_hex kabul edilmemeliydi")
	}
	if len(mb.revoked) != 0 {
		t.Fatalf("geçersiz biçim backend'e hiç ulaşmamalıydı, got %+v", mb.revoked)
	}
}

func TestRevokeModerator_FounderOnly(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, &mockID{moderator: true}, mb)

	if err := c.RevokeModerator("aabbcc"); err == nil {
		t.Fatal("moderatör iptali yalnızca kurucuya açık olmalıydı")
	}
}

func TestRevokeModerator_FounderAllowed(t *testing.T) {
	mb := &mockModeration{}
	c := startTestServerWithMod(t, founderID(), mb)

	key := strings.Repeat("aabbccdd", 8)
	if err := c.RevokeModerator(key); err != nil {
		t.Fatalf("RevokeModerator: %v", err)
	}
	if len(mb.revoked) != 1 || mb.revoked[0] != key {
		t.Fatalf("beklenmeyen iptal kaydı: %+v", mb.revoked)
	}
}

func TestModEndpoints_NilBackendReturnsError(t *testing.T) {
	c := startTestServer(t, founderID(), &mockForum{}, nil) // mb=nil

	if _, err := c.AllBans(); err == nil {
		t.Fatal("mb=nil iken AllBans hata döndürmeliydi")
	}
	if err := c.Unban("x"); err == nil {
		t.Fatal("mb=nil iken Unban hata döndürmeliydi")
	}
}

// ─── DM endpoints ─────────────────────────────────────────────────────────────

func TestDMPoll_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)

	msgs, err := c.PollDM()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice with nil DM backend, got %v", msgs)
	}
}

func TestDMPoll_ReceivesMessage(t *testing.T) {
	dm := &mockDM{}
	c := startTestServer(t, &mockID{name: "bob"}, &mockForum{}, dm)

	// Deliver a message shortly after the poll starts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, ch := range dm.subs {
			ch <- DMMsg{From: "alice", Text: "merhaba", ReceivedAt: time.Now()}
		}
	}()

	msgs, err := c.PollDM()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "alice" || msgs[0].Text != "merhaba" {
		t.Errorf("unexpected message: %+v", msgs[0])
	}
}

func TestSendDM_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{name: "bob"}, &mockForum{}, nil)
	err := c.SendDM("12D3KooWXxx", "deneme")
	if err == nil {
		t.Fatal("expected error with nil DM backend")
	}
}

func TestSendDM_Error(t *testing.T) {
	dm := &mockDM{sendErr: errors.New("peer erişilemiyor")}
	c := startTestServer(t, &mockID{name: "bob"}, &mockForum{}, dm)
	err := c.SendDM("12D3KooWXxx", "deneme")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── File endpoints ───────────────────────────────────────────────────────────

type mockFile struct {
	subs    []chan FileMsg
	sendErr error
	sent    []struct{ peerID, localPath string }
}

func (m *mockFile) Subscribe() chan FileMsg {
	ch := make(chan FileMsg, 8)
	m.subs = append(m.subs, ch)
	return ch
}
func (m *mockFile) Unsubscribe(ch chan FileMsg) {
	for i, c := range m.subs {
		if c == ch {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}
func (m *mockFile) SendFile(_ context.Context, peerID, _, localPath string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, struct{ peerID, localPath string }{peerID, localPath})
	return nil
}
func (m *mockFile) SubscribeConsent() chan FileConsentReq    { return make(chan FileConsentReq, 4) }
func (m *mockFile) UnsubscribeConsent(_ chan FileConsentReq) {}
func (m *mockFile) RespondConsent(_ string, _ bool)          {}

func startTestServerWithMod(t *testing.T, id IdentityBackend, mb ModerationBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, &mockForum{}, nil, nil, nil, nil, mb, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

func startTestServerWithFile(t *testing.T, id IdentityBackend, f ForumBackend, dm DMBackend, file FileBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, f, nil, dm, file, nil, nil, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

func TestFilePoll_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	msgs, err := c.PollFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice with nil file backend, got %v", msgs)
	}
}

func TestSendFile_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{name: "bob"}, &mockForum{}, nil)
	err := c.SendFile("12D3KooWXxx", "test.txt")
	if err == nil {
		t.Fatal("expected error with nil file backend")
	}
}

func TestSendFile_EmptyPeerID(t *testing.T) {
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("", "test.txt")
	if err == nil {
		t.Fatal("expected 400 for empty peer_id")
	}
}

func TestSendFile_EmptyLocalPath(t *testing.T) {
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", "")
	if err == nil {
		t.Fatal("expected 400 for empty local_path")
	}
}

func TestSendFile_Success(t *testing.T) {
	home := setFakeHome(t)
	localPath := filepath.Join(home, "belgeler", "rapor.pdf")

	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", localPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(mf.sent) != 1 {
		t.Fatalf("expected 1 sent file, got %d", len(mf.sent))
	}
	if mf.sent[0].localPath != localPath {
		t.Errorf("unexpected localPath: %q", mf.sent[0].localPath)
	}
	if mf.sent[0].peerID != "12D3KooWXxx" {
		t.Errorf("unexpected peerID: %q", mf.sent[0].peerID)
	}
}

// setFakeHome, os.UserHomeDir()'ın (hem Unix'te HOME hem Windows'ta
// USERPROFILE üzerinden) testin denetlediği geçici bir dizine çözülmesini
// sağlar — forbiddenSendPath'in "yalnızca ana dizin içi" kısıtlamasını
// platformdan bağımsız, gerçekçi biçimde test edebilmek için.
func setFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestSendFile_RejectsPathOutsideHome(t *testing.T) {
	setFakeHome(t)
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", filepath.Join(t.TempDir(), "disari.pdf"))
	if err == nil {
		t.Fatal("expected 400 for a path outside the user's home directory")
	}
	if len(mf.sent) != 0 {
		t.Fatalf("backend should never be called for a rejected path, got %d sends", len(mf.sent))
	}
}

func TestSendFile_RejectsSensitiveHomeSubdir(t *testing.T) {
	home := setFakeHome(t)
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", filepath.Join(home, ".ssh", "id_ed25519"))
	if err == nil {
		t.Fatal("expected 400 for a file under ~/.ssh")
	}
	if len(mf.sent) != 0 {
		t.Fatalf("backend should never be called for a rejected path, got %d sends", len(mf.sent))
	}
}

func TestFilePoll_ReceivesMessage(t *testing.T) {
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)

	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, ch := range mf.subs {
			ch <- FileMsg{From: "alice", Filename: "foto.jpg", Size: 3, SavePath: "/tmp/foto.jpg", ReceivedAt: time.Now()}
		}
	}()

	msgs, err := c.PollFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 file message, got %d", len(msgs))
	}
	if msgs[0].From != "alice" || msgs[0].Filename != "foto.jpg" {
		t.Errorf("unexpected message: %+v", msgs[0])
	}
	if msgs[0].SavePath != "/tmp/foto.jpg" {
		t.Errorf("unexpected SavePath: %q", msgs[0].SavePath)
	}
}

func TestSendFile_BackendError(t *testing.T) {
	home := setFakeHome(t)
	mf := &mockFile{sendErr: errors.New("peer erişilemiyor")}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", filepath.Join(home, "test.txt"))
	if err == nil {
		t.Fatal("expected error from backend")
	}
}

// ─── pathWithinDir (postSendFile'ın AND_DATA_DIR dışa sızma koruması) ──────────

func TestPathWithinDir(t *testing.T) {
	dataDir := t.TempDir()
	protected := filepath.Join(dataDir, "identity.dat")
	if err := os.WriteFile(protected, []byte("gizli-anahtar"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dataDir, "alt", "dosya.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "rapor.pdf")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !pathWithinDir(protected, dataDir) {
		t.Error("dataDir'in doğrudan içindeki bir dosya 'içeride' olarak algılanmalı")
	}
	if !pathWithinDir(nested, dataDir) {
		t.Error("dataDir altındaki bir alt dizindeki dosya da 'içeride' olarak algılanmalı")
	}
	if !pathWithinDir(dataDir, dataDir) {
		t.Error("dataDir'in kendisi 'içeride' olarak algılanmalı")
	}
	if pathWithinDir(outsideFile, dataDir) {
		t.Error("dataDir dışındaki bir dosya 'dışarıda' olarak algılanmalı")
	}

	// Göreli yol ataklarına karşı: filepath.Abs olmadan filepath.Clean tek
	// başına göreli bir yolu (ör. "identity.dat") mutlak dataDir ile hiç
	// eşleştiremezdi — bu regresyon testi tam olarak bunu doğrular.
	relOK := func() bool {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dataDir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(wd) }()
		return pathWithinDir("identity.dat", dataDir)
	}()
	if !relOK {
		t.Error("çalışma dizini dataDir olduğunda göreli bir yol da 'içeride' olarak algılanmalı")
	}

	// Sembolik bağ testi: dataDir DIŞINDA duran, ama dataDir İÇİNDEKİ bir
	// dosyaya işaret eden bir sembolik bağ — pathWithinDir bunu da
	// "içeride" olarak tespit etmelidir, aksi halde bir sembolik bağ
	// aracılığıyla korumanın tamamı atlatılabilir.
	link := filepath.Join(outside, "link-to-identity")
	if err := os.Symlink(protected, link); err != nil {
		t.Skipf("bu ortamda sembolik bağ oluşturulamadı (izin?): %v", err)
	}
	if !pathWithinDir(link, dataDir) {
		t.Error("dataDir içindeki bir dosyaya işaret eden sembolik bağ, metinsel olarak dışarıda olsa bile 'içeride' olarak algılanmalı")
	}
}

// ─── buildMux / allowExternal filtrelemesi ─────────────────────────────────────

// TestBuildMuxExternalFiltering, allowExternal:false işaretli bir route'un
// externalOnly=true ile inşa edilen mux'ta GERÇEKTEN yok sayıldığını,
// externalOnly=false ile inşa edilen mux'ta ise normal şekilde
// çalıştığını doğrudan HTTP istekleriyle kanıtlar. Bu test olmadan
// allowExternal mekanizması yalnızca "aspirasyonel" kalır — bir route'u
// yanlışlıkla true bırakan (ya da buildMux'un filtre koşulunu ters çeviren)
// bir gelecek değişiklik hiçbir testte başarısız olmazdı.
func TestBuildMuxExternalFiltering(t *testing.T) {
	var internalCalled, sharedCalled bool
	routes := []route{
		{pattern: "/internal-only", handler: func(http.ResponseWriter, *http.Request) { internalCalled = true }, izin: IzinTemel},
		{pattern: "/shared", handler: func(http.ResponseWriter, *http.Request) { sharedCalled = true }, allowExternal: true, izin: IzinTemel},
	}

	// Sunucu, harici mux'u tam yetkiyle sarar (bkz. Start); burada da öyle.
	externalMux := tamYetkiyle(buildMuxFromRoutes(routes, true))

	rec := httptest.NewRecorder()
	externalMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal-only", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("allowExternal:false route harici mux'ta 404 dönmeli, %d döndü", rec.Code)
	}
	if internalCalled {
		t.Error("allowExternal:false route'un handler'ı harici mux üzerinden ÇAĞRILMAMALIYDI")
	}

	rec = httptest.NewRecorder()
	externalMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/shared", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("allowExternal:true route harici mux'ta çalışmalı, %d döndü", rec.Code)
	}
	if !sharedCalled {
		t.Error("allowExternal:true route'un handler'ı harici mux üzerinden çağrılmalıydı")
	}

	internalCalled = false
	internalMux := tamYetkiyle(buildMuxFromRoutes(routes, false))
	rec = httptest.NewRecorder()
	internalMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal-only", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("allowExternal:false route dahili (token'lı) mux'ta normal çalışmalı, %d döndü", rec.Code)
	}
	if !internalCalled {
		t.Error("allowExternal:false route'un handler'ı dahili mux üzerinden çağrılmalıydı")
	}
}

// ─── Peers ────────────────────────────────────────────────────────────────────

type mockIDWithPeers struct {
	mockID
	peers []PeerInfo
}

func (m *mockIDWithPeers) ConnectedPeers() []PeerInfo { return m.peers }

func TestGetPeers_Empty(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	peers, err := c.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestGetPeers_WithPeers(t *testing.T) {
	id := &mockIDWithPeers{
		peers: []PeerInfo{
			{PeerID: "12D3KooWAbc", Addrs: []string{"/ip4/127.0.0.1/tcp/4001"}},
			{PeerID: "12D3KooWDef", Addrs: []string{"/ip4/192.168.1.5/tcp/4001"}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, &mockForum{}, nil, nil, nil, nil, nil, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	peers, err := c.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if peers[0].PeerID != "12D3KooWAbc" {
		t.Errorf("unexpected peer[0]: %+v", peers[0])
	}
	if len(peers[1].Addrs) != 1 {
		t.Errorf("expected 1 addr for peer[1], got %d", len(peers[1].Addrs))
	}
}

// ─── Forum post detail + reply ─────────────────────────────────────────────────

func TestGetForumPost(t *testing.T) {
	c := startTestServer(t, founderID(), &mockForum{}, nil)
	detail, err := c.GetPost(testPostID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != testPostID {
		t.Errorf("got ID=%q, want %q", detail.ID, testPostID)
	}
}

func TestGetForumPost_InvalidID(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	_, err := c.GetPost("not-hex!")
	if err == nil {
		t.Fatal("expected 400 for invalid id")
	}
}

func TestCreateReply(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.CreateReply(testPostID, "harika konu!"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReply_EmptyBody(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.CreateReply(testPostID, ""); err == nil {
		t.Fatal("expected 400 for empty body")
	}
}

func TestCreateReply_InvalidPostID(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.CreateReply("not-hex!", "içerik"); err == nil {
		t.Fatal("expected 400 for invalid post_id")
	}
}

// ─── Chat endpoints ───────────────────────────────────────────────────────────

type mockChat struct {
	subs    []chan ChatMsg
	sendErr error
	sent    []ChatMsg
	all     []ChatMsg
}

func (m *mockChat) Subscribe() chan ChatMsg {
	ch := make(chan ChatMsg, 8)
	m.subs = append(m.subs, ch)
	return ch
}
func (m *mockChat) Unsubscribe(ch chan ChatMsg) {
	for i, c := range m.subs {
		if c == ch {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			return
		}
	}
}
func (m *mockChat) SendChat(_ context.Context, from, msg string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, ChatMsg{From: from, Text: msg})
	return nil
}
func (m *mockChat) AllChat() []ChatMsg { return m.all }

func startTestServerWithChat(t *testing.T, chat ChatBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(&mockID{name: "ali"}, &mockForum{}, nil, nil, nil, chat, nil, "", nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

func TestChatPoll_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	msgs, err := c.PollChat()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice with nil chat backend, got %v", msgs)
	}
}

func TestChatPoll_ReceivesMessage(t *testing.T) {
	chat := &mockChat{}
	c := startTestServerWithChat(t, chat)

	go func() {
		time.Sleep(50 * time.Millisecond)
		for _, ch := range chat.subs {
			ch <- ChatMsg{From: "veli", Text: "selam"}
		}
	}()

	msgs, err := c.PollChat()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 chat msg, got %d", len(msgs))
	}
	if msgs[0].From != "veli" || msgs[0].Text != "selam" {
		t.Errorf("unexpected msg: %+v", msgs[0])
	}
}

func TestChatAll_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	msgs, err := c.AllChat()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice with nil chat backend, got %v", msgs)
	}
}

func TestChatAll_ReturnsFullHistory(t *testing.T) {
	chat := &mockChat{all: []ChatMsg{
		{ID: "1", From: "ali", Text: "birinci"},
		{ID: "2", From: "veli", Text: "ikinci"},
	}}
	c := startTestServerWithChat(t, chat)

	msgs, err := c.AllChat()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 chat msgs, got %d", len(msgs))
	}
	if msgs[0].ID != "1" || msgs[1].ID != "2" {
		t.Errorf("unexpected order/content: %+v", msgs)
	}
}

func TestSendChat_Success(t *testing.T) {
	chat := &mockChat{}
	c := startTestServerWithChat(t, chat)

	if err := c.SendChat("herkese merhaba"); err != nil {
		t.Fatal(err)
	}
	if len(chat.sent) != 1 {
		t.Fatalf("expected 1 sent msg, got %d", len(chat.sent))
	}
	if chat.sent[0].Text != "herkese merhaba" {
		t.Errorf("unexpected text: %q", chat.sent[0].Text)
	}
}

func TestSendChat_EmptyMessage(t *testing.T) {
	chat := &mockChat{}
	c := startTestServerWithChat(t, chat)
	if err := c.SendChat(""); err == nil {
		t.Fatal("expected 400 for empty message")
	}
}

func TestSendChat_NilBackend(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.SendChat("test"); err == nil {
		t.Fatal("expected error with nil chat backend")
	}
}

// ─── Client from env ──────────────────────────────────────────────────────────

func TestNewClientFromEnv_MissingVar(t *testing.T) {
	os.Unsetenv("AND_API_ADDR")
	os.Unsetenv("AND_API_TOKEN")
	_, err := NewClientFromEnv()
	if err == nil {
		t.Fatal("expected error when AND_API_ADDR is not set")
	}
}

// ─── Theme ────────────────────────────────────────────────────────────────────

func TestGetTheme_DefaultWhenNoFile(t *testing.T) {
	id := &mockID{name: "alice", peerID: "12D3KooWTest"}
	c := startTestServer(t, id, &mockForum{}, nil)

	got, err := c.GetTheme()
	if err != nil {
		t.Fatal(err)
	}
	if want := ThemeReq(ui.DefaultTheme()); got != want {
		t.Errorf("GetTheme: got %+v, want default %+v", got, want)
	}
}

// TestGetTheme_LegacyDefaultMigrated, eski (mor) varsayılan temanın birebir
// kaydını taşıyan bir theme.json'ın — Pembiş'in eski "Varsayılan Temaya
// Dön"ü bunu yazıyordu — eklentilere güncel varsayılan olarak sunulduğunu
// doğrular; TUI de aynı dönüşümü uyguladığından ikisi ayrışmaz.
func TestGetTheme_LegacyDefaultMigrated(t *testing.T) {
	id := &mockID{name: "alice", peerID: "12D3KooWTest"}
	dataDir := t.TempDir()
	legacy := ThemeReq{ThemeName: "Turkuaz", Accent: "63", SelBG: "57", SelFG: "255", Muted: "241", Name: "86", Badge: "33"}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, ThemeFile), data, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, &mockForum{}, nil, nil, nil, nil, nil, dataDir, nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	got, err := c.GetTheme()
	if err != nil {
		t.Fatal(err)
	}
	if want := ThemeReq(ui.DefaultTheme()); got != want {
		t.Errorf("eski varsayılan güncel varsayılana çevrilmeliydi: got %+v, want %+v", got, want)
	}
}

func TestGetTheme_ReadsPersistedFile(t *testing.T) {
	id := &mockID{name: "alice", peerID: "12D3KooWTest"}
	dataDir := t.TempDir()
	want := ThemeReq{ThemeName: "Pembiş", Accent: "205", SelBG: "162", SelFG: "255", Muted: "182", Name: "218", Badge: "199"}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, ThemeFile), data, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, &mockForum{}, nil, nil, nil, nil, nil, dataDir, nil)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	got, err := c.GetTheme()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("GetTheme: got %+v, want %+v", got, want)
	}
}

func TestSetTheme_ForwardsToChannel(t *testing.T) {
	id := &mockID{name: "alice", peerID: "12D3KooWTest"}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	themeCh := make(chan ThemeReq, 1)
	srv := NewServer(id, &mockForum{}, nil, nil, nil, nil, nil, "", themeCh)
	addr, token, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", token)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	req := ThemeReq{ThemeName: "Pembiş", Accent: "205"}
	if err := c.SetTheme(req); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-themeCh:
		if got != req {
			t.Errorf("themeCh: got %+v, want %+v", got, req)
		}
	case <-time.After(time.Second):
		t.Fatal("themeCh did not receive the request")
	}
}

func TestHelpers(t *testing.T) {
	t.Setenv("AND_DATA_DIR", "/tmp/test")
	t.Setenv("AND_CATEGORY", "genel")

	if got := DataDir(); got != "/tmp/test" {
		t.Errorf("DataDir: %q", got)
	}
	if got := Category(); got != "genel" {
		t.Errorf("Category: %q", got)
	}
}

// historyDM, geçmişi olan bir DM arka ucudur.
type historyDM struct {
	mockDM
	kayitlar []DMRecord
}

func (h *historyDM) History() []DMRecord { return h.kayitlar }

func TestDMHistory(t *testing.T) {
	id := &mockID{name: "alice", pubKeyHex: "aabbcc", peerID: "12D3KooWTest"}

	// Geçmişi olmayan (eski) bir arka uç boş liste döner, hata değil.
	c := startTestServer(t, id, &mockForum{}, &mockDM{})
	if got, err := c.DMHistory(); err != nil || len(got) != 0 {
		t.Fatalf("geçmişsiz arka uç boş liste dönmeli: %v %v", got, err)
	}

	dm := &historyDM{kayitlar: []DMRecord{
		{PeerID: "12D3KooWBob", Name: "bob", Text: "selam"},
		{PeerID: "12D3KooWBob", Text: "merhaba", Outgoing: true},
	}}
	c = startTestServer(t, id, &mockForum{}, dm)
	got, err := c.DMHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "bob" || !got[1].Outgoing || got[1].Text != "merhaba" {
		t.Fatalf("geçmiş beklenmedik: %+v", got)
	}
}
