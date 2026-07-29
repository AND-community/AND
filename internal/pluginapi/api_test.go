package pluginapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func (m *mockID) Name() string               { return m.name }
func (m *mockID) PubKeyHex() string          { return m.pubKeyHex }
func (m *mockID) PeerIDStr() string          { return m.peerID }
func (m *mockID) IsFounder() bool            { return m.founder }
func (m *mockID) IsModerator() bool          { return m.moderator }
func (m *mockID) ConnectedPeers() []PeerInfo { return nil }

type mockForum struct {
	created   []CreatePostReq
	createErr error
}

func (m *mockForum) CreatePost(_ context.Context, cat, title, body string) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, CreatePostReq{Category: cat, Title: title, Body: body})
	return nil
}
func (m *mockForum) AllPosts() ([]PostSummary, error) { return nil, nil }
func (m *mockForum) DeletePost(_ string) error        { return nil }
func (m *mockForum) GetPost(id string) (*PostDetail, error) {
	return &PostDetail{ID: id, Replies: []ReplyInfo{}}, nil
}
func (m *mockForum) CreateReply(_ context.Context, _, _ string) error { return nil }

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

	srv := NewServer(id, f, dm, nil, nil, "", nil)
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

func startTestServerWithFile(t *testing.T, id IdentityBackend, f ForumBackend, dm DMBackend, file FileBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(id, f, dm, file, nil, "", nil)
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
	mf := &mockFile{}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", "/tmp/rapor.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if len(mf.sent) != 1 {
		t.Fatalf("expected 1 sent file, got %d", len(mf.sent))
	}
	if mf.sent[0].localPath != "/tmp/rapor.pdf" {
		t.Errorf("unexpected localPath: %q", mf.sent[0].localPath)
	}
	if mf.sent[0].peerID != "12D3KooWXxx" {
		t.Errorf("unexpected peerID: %q", mf.sent[0].peerID)
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
	mf := &mockFile{sendErr: errors.New("peer erişilemiyor")}
	c := startTestServerWithFile(t, &mockID{name: "bob"}, &mockForum{}, nil, mf)
	err := c.SendFile("12D3KooWXxx", "test.txt")
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
		{"/internal-only", func(http.ResponseWriter, *http.Request) { internalCalled = true }, false},
		{"/shared", func(http.ResponseWriter, *http.Request) { sharedCalled = true }, true},
	}

	externalMux := buildMuxFromRoutes(routes, true)

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
	internalMux := buildMuxFromRoutes(routes, false)
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
	srv := NewServer(id, &mockForum{}, nil, nil, nil, "", nil)
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

func startTestServerWithChat(t *testing.T, chat ChatBackend) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	srv := NewServer(&mockID{name: "ali"}, &mockForum{}, nil, nil, chat, "", nil)
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
	if got != defaultTheme {
		t.Errorf("GetTheme: got %+v, want default %+v", got, defaultTheme)
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
	srv := NewServer(id, &mockForum{}, nil, nil, nil, dataDir, nil)
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
	srv := NewServer(id, &mockForum{}, nil, nil, nil, "", themeCh)
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
