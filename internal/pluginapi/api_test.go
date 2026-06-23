package pluginapi

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// ─── Mock backends ────────────────────────────────────────────────────────────

type mockID struct {
	name      string
	pubKeyHex string
	peerID    string
	founder   bool
	moderator bool
}

func (m *mockID) Name() string       { return m.name }
func (m *mockID) PubKeyHex() string  { return m.pubKeyHex }
func (m *mockID) PeerIDStr() string  { return m.peerID }
func (m *mockID) IsFounder() bool    { return m.founder }
func (m *mockID) IsModerator() bool  { return m.moderator }

type mockForum struct {
	posts        []PendingPost
	approved     []string
	rejected     []string
	authorKeys   []string
	created      []CreatePostReq
	approveErr   error
	rejectErr    error
	pendingErr   error
	createErr    error
}

func (m *mockForum) PendingPosts() ([]PendingPost, error) {
	if m.pendingErr != nil {
		return nil, m.pendingErr
	}
	return m.posts, nil
}
func (m *mockForum) ApprovePost(id string) error {
	if m.approveErr != nil {
		return m.approveErr
	}
	m.approved = append(m.approved, id)
	return nil
}
func (m *mockForum) RejectPost(id string) error {
	if m.rejectErr != nil {
		return m.rejectErr
	}
	m.rejected = append(m.rejected, id)
	return nil
}
func (m *mockForum) ApproveAuthor(key string) error {
	m.authorKeys = append(m.authorKeys, key)
	return nil
}
func (m *mockForum) CreatePost(_ context.Context, cat, title, body string, perm bool) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, CreatePostReq{Category: cat, Title: title, Body: body, PermanentReq: perm})
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

	srv := NewServer(id, f, dm)
	addr, err := srv.Start(ctx)
	if err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Setenv("AND_API_ADDR", addr)
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

// testAuthorKey is a valid 64-char hex Ed25519 public key used in tests.
const testAuthorKey = "a1b2c3d4e5f60718a9b0c1d2e3f4a5b6a1b2c3d4e5f60718a9b0c1d2e3f4a5b6"

// founderID returns a mockID representing a founder (required for protected endpoints).
func founderID() *mockID { return &mockID{founder: true, moderator: true} }

func TestGetPending_Empty(t *testing.T) {
	// Pending list requires moderator/founder role.
	c := startTestServer(t, founderID(), &mockForum{}, nil)

	posts, err := c.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Errorf("expected 0 posts, got %d", len(posts))
	}
}

func TestGetPending_WithPosts(t *testing.T) {
	f := &mockForum{
		posts: []PendingPost{
			{ID: "p1", Title: "Başlık 1", AuthorName: "ali", Category: "genel"},
			{ID: "p2", Title: "Başlık 2", AuthorName: "veli", Category: "duyuru", PermanentReq: true},
		},
	}
	c := startTestServer(t, founderID(), f, nil)

	posts, err := c.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].ID != "p1" || posts[1].ID != "p2" {
		t.Errorf("unexpected posts: %+v", posts)
	}
	if !posts[1].PermanentReq {
		t.Error("expected PermanentReq=true for p2")
	}
}

func TestGetPending_Error(t *testing.T) {
	// Use founder so the role check passes and the DB error is actually reached.
	f := &mockForum{pendingErr: errors.New("db hatası")}
	c := startTestServer(t, founderID(), f, nil)

	_, err := c.Pending()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPending_Forbidden(t *testing.T) {
	// A regular user (no roles) must receive 403 from getPending.
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	_, err := c.Pending()
	if err == nil {
		t.Fatal("expected 403 error for non-moderator, got nil")
	}
}

func TestApprovePost(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.Approve(testPostID); err != nil {
		t.Fatal(err)
	}
	if len(f.approved) != 1 || f.approved[0] != testPostID {
		t.Errorf("unexpected approved: %v", f.approved)
	}
}

func TestApprovePost_Error(t *testing.T) {
	// Forum returns an error; use founder so the role check passes first.
	f := &mockForum{approveErr: errors.New("yetki yok")}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.Approve(testPostID); err == nil {
		t.Fatal("expected error")
	}
}

func TestApprovePost_Forbidden(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.Approve(testPostID); err == nil {
		t.Fatal("expected 403 for non-moderator")
	}
}

func TestApprovePost_InvalidPostID(t *testing.T) {
	// "post-abc" contains "-" which is not hex → must be rejected with 400.
	c := startTestServer(t, founderID(), &mockForum{}, nil)
	if err := c.Approve("post-abc"); err == nil {
		t.Fatal("expected 400 for invalid PostID")
	}
}

func TestRejectPost(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.Reject(testPostID); err != nil {
		t.Fatal(err)
	}
	if len(f.rejected) != 1 || f.rejected[0] != testPostID {
		t.Errorf("unexpected rejected: %v", f.rejected)
	}
}

func TestRejectPost_Forbidden(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.Reject(testPostID); err == nil {
		t.Fatal("expected 403 for non-moderator")
	}
}

func TestRejectPost_InvalidPostID(t *testing.T) {
	c := startTestServer(t, founderID(), &mockForum{}, nil)
	if err := c.Reject("not-hex!"); err == nil {
		t.Fatal("expected 400 for invalid PostID")
	}
}

func TestApproveAuthor(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, founderID(), f, nil)

	if err := c.ApproveAuthor(testAuthorKey); err != nil {
		t.Fatal(err)
	}
	if len(f.authorKeys) != 1 || f.authorKeys[0] != testAuthorKey {
		t.Errorf("unexpected authorKeys: %v", f.authorKeys)
	}
}

func TestApproveAuthor_Forbidden(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, nil)
	if err := c.ApproveAuthor(testAuthorKey); err == nil {
		t.Fatal("expected 403 for non-moderator")
	}
}

func TestApproveAuthor_InvalidKey(t *testing.T) {
	// "pubkey-abc" is not 64 hex chars → must be rejected with 400.
	c := startTestServer(t, founderID(), &mockForum{}, nil)
	if err := c.ApproveAuthor("pubkey-abc"); err == nil {
		t.Fatal("expected 400 for invalid AuthorKey")
	}
}

func TestCreatePost(t *testing.T) {
	f := &mockForum{}
	c := startTestServer(t, &mockID{}, f, nil)

	if err := c.CreatePost("genel", "Test Başlık", "Test içerik", true); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 {
		t.Fatalf("expected 1 created post, got %d", len(f.created))
	}
	p := f.created[0]
	if p.Category != "genel" || p.Title != "Test Başlık" || !p.PermanentReq {
		t.Errorf("unexpected post: %+v", p)
	}
}

func TestCreatePost_Error(t *testing.T) {
	f := &mockForum{createErr: errors.New("kategori bulunamadı")}
	c := startTestServer(t, &mockID{}, f, nil)

	if err := c.CreatePost("yok", "x", "y", false); err == nil {
		t.Fatal("expected error")
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

// ─── Client from env ──────────────────────────────────────────────────────────

func TestNewClientFromEnv_MissingVar(t *testing.T) {
	os.Unsetenv("AND_API_ADDR")
	_, err := NewClientFromEnv()
	if err == nil {
		t.Fatal("expected error when AND_API_ADDR is not set")
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
