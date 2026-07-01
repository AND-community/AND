package pluginapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const DMProtocol = "/and/dm/1.0.0"
const FileProtocol = "/and/file/2.0.0"

// MaxFileBytes is the maximum raw file size accepted for transfer (2 GB).
const MaxFileBytes = 20 * 1024 * 1024 * 1024

type Manifest struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Category    string `json:"category,omitempty"`
}

type IdentityInfo struct {
	Name   string `json:"name"`
	PubKey string `json:"pub_key"`
	PeerID string `json:"peer_id"`
}

type RoleInfo struct {
	IsFounder   bool `json:"is_founder"`
	IsModerator bool `json:"is_moderator"`
}

type PendingPost struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	AuthorName   string    `json:"author_name"`
	AuthorKey    string    `json:"author_key"`
	Category     string    `json:"category"`
	Body         string    `json:"body"`
	ExpiresAt    time.Time `json:"expires_at"`
	PermanentReq bool      `json:"permanent_req"`
}

type ApproveReq struct{ PostID string `json:"post_id"` }
type RejectReq struct{ PostID string `json:"post_id"` }
type DeletePostReq struct{ PostID string `json:"post_id"` }
type ApproveAuthorReq struct{ AuthorKey string `json:"author_key"` }

type PostSummary struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	AuthorKey  string `json:"author_key"`
	Approved   bool   `json:"approved"`
}
type CreatePostReq struct {
	Category     string `json:"category"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	PermanentReq bool   `json:"permanent_req,omitempty"`
}
type SendDMReq struct {
	PeerID  string `json:"peer_id"`
	Message string `json:"message"`
}

type DMMsg struct {
	From       string    `json:"from"`
	Text       string    `json:"text"`
	ReceivedAt time.Time `json:"received_at"`
}

// SendFileReq is the request body for POST /api/v1/file/send.
// The plugin supplies a local file path; AND reads and streams the file — no data in the request.
type SendFileReq struct {
	PeerID    string `json:"peer_id"`
	LocalPath string `json:"local_path"`
}

// FileMsg is delivered to plugins when a file arrives from a remote peer.
// AND saves the file to disk and reports the path; no file bytes are carried here.
type FileMsg struct {
	From       string    `json:"from"`
	Filename   string    `json:"filename"`
	Size       int64     `json:"size"`
	SavePath   string    `json:"save_path"`
	ReceivedAt time.Time `json:"received_at"`
}

// FileConsentReq is sent to plugins when a remote peer wants to send a file.
// The plugin must call /api/v1/file/consent to accept or reject.
type FileConsentReq struct {
	TransferID string `json:"transfer_id"`
	SenderID   string `json:"sender_id"` // peer ID
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
}

type ConsentResp struct {
	TransferID string `json:"transfer_id"`
	Accept     bool   `json:"accept"`
}

type errResp struct{ Error string `json:"error"` }

// ThemeReq is the request body for POST /api/v1/theme.
// Color values are terminal palette numbers ("63") or hex codes ("#ff6600").
type ThemeReq struct {
	ThemeName string `json:"theme_name,omitempty"`
	Accent    string `json:"accent,omitempty"`
	SelBG     string `json:"sel_bg,omitempty"`
	SelFG     string `json:"sel_fg,omitempty"`
	Muted     string `json:"muted,omitempty"`
	Name      string `json:"name,omitempty"`
	Badge     string `json:"badge,omitempty"`
}

// PeerInfo represents a connected libp2p peer.
type PeerInfo struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`
}

// ChatMsg is a general chat message received from the shared channel.
type ChatMsg struct {
	From   string    `json:"from"`
	Text   string    `json:"text"`
	SentAt time.Time `json:"sent_at"`
}

// SendChatReq is the request body for POST /api/v1/chat/send.
type SendChatReq struct {
	Message string `json:"message"`
}

// ReplyInfo is a single reply to a forum post.
type ReplyInfo struct {
	ID         string    `json:"id"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// PostDetail is a forum post with its full body and replies.
type PostDetail struct {
	ID         string      `json:"id"`
	Category   string      `json:"category"`
	AuthorName string      `json:"author_name"`
	AuthorKey  string      `json:"author_key"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	CreatedAt  time.Time   `json:"created_at"`
	Approved   bool        `json:"approved"`
	Replies    []ReplyInfo `json:"replies"`
}

// CreateReplyReq is the request body for POST /api/v1/forum/reply.
type CreateReplyReq struct {
	PostID string `json:"post_id"`
	Body   string `json:"body"`
}

type ForumBackend interface {
	PendingPosts() ([]PendingPost, error)
	AllPosts() ([]PostSummary, error)
	ApprovePost(postID string) error
	RejectPost(postID string) error
	DeletePost(postID string) error
	ApproveAuthor(authorKey string) error
	CreatePost(ctx context.Context, category, title, body string, permanentReq bool) error
	GetPost(id string) (*PostDetail, error)
	CreateReply(ctx context.Context, postID, body string) error
}

type IdentityBackend interface {
	Name() string
	PubKeyHex() string
	PeerIDStr() string
	IsFounder() bool
	IsModerator() bool
	ConnectedPeers() []PeerInfo
}

type DMBackend interface {
	SendDM(ctx context.Context, peerID, senderName, message string) error
	Subscribe() chan DMMsg
	Unsubscribe(ch chan DMMsg)
}

type FileBackend interface {
	SendFile(ctx context.Context, peerID, senderName, localPath string) error
	Subscribe() chan FileMsg
	Unsubscribe(ch chan FileMsg)
	SubscribeConsent() chan FileConsentReq
	UnsubscribeConsent(ch chan FileConsentReq)
	RespondConsent(transferID string, accept bool)
}

type ChatBackend interface {
	SendChat(ctx context.Context, senderName, message string) error
	Subscribe() chan ChatMsg
	Unsubscribe(ch chan ChatMsg)
}

var reHex = regexp.MustCompile(`^[0-9a-fA-F]{8,128}$`)

func validPostID(id string) bool    { return reHex.MatchString(id) }
func validAuthorKey(key string) bool { return len(key) == 64 && reHex.MatchString(key) }

type Server struct {
	addr    string
	token   string
	dataDir string
	srv     *http.Server
	id      IdentityBackend
	f       ForumBackend
	dm      DMBackend
	file    FileBackend
	chat    ChatBackend
	themeCh chan<- ThemeReq
}

func NewServer(id IdentityBackend, f ForumBackend, dm DMBackend, file FileBackend, chat ChatBackend, dataDir string, themeCh chan<- ThemeReq) *Server {
	return &Server{id: id, f: f, dm: dm, file: file, chat: chat, dataDir: dataDir, themeCh: themeCh}
}

func (s *Server) Start(ctx context.Context) (addr, token string, err error) {
	ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		return "", "", fmt.Errorf("pluginapi: listen: %w", listenErr)
	}
	s.addr = ln.Addr().String()

	raw := make([]byte, 32)
	if _, randErr := rand.Read(raw); randErr != nil {
		ln.Close() //nolint:errcheck
		return "", "", fmt.Errorf("pluginapi: generate token: %w", randErr)
	}
	s.token = hex.EncodeToString(raw)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/identity",             onlyMethod(http.MethodGet, s.getIdentity))
	mux.HandleFunc("/api/v1/role",                 onlyMethod(http.MethodGet, s.getRole))
	mux.HandleFunc("/api/v1/peers",                onlyMethod(http.MethodGet, s.getPeers))
	mux.HandleFunc("/api/v1/forum/pending",        onlyMethod(http.MethodGet, s.getPending))
	mux.HandleFunc("/api/v1/forum/all-posts",      onlyMethod(http.MethodGet, s.getAllPosts))
	mux.HandleFunc("/api/v1/forum/post",           s.handleForumPost)
	mux.HandleFunc("/api/v1/forum/reply",          onlyMethod(http.MethodPost, s.postReply))
	mux.HandleFunc("/api/v1/forum/approve",        onlyMethod(http.MethodPost, s.postApprove))
	mux.HandleFunc("/api/v1/forum/approve-author", onlyMethod(http.MethodPost, s.postApproveAuthor))
	mux.HandleFunc("/api/v1/forum/reject",         onlyMethod(http.MethodPost, s.postReject))
	mux.HandleFunc("/api/v1/forum/delete",         onlyMethod(http.MethodPost, s.postDeletePost))
	mux.HandleFunc("/api/v1/dm/send",              onlyMethod(http.MethodPost, s.postSendDM))
	mux.HandleFunc("/api/v1/dm/poll",              onlyMethod(http.MethodGet, s.getDMPoll))
	mux.HandleFunc("/api/v1/chat/send",            onlyMethod(http.MethodPost, s.postSendChat))
	mux.HandleFunc("/api/v1/chat/poll",            onlyMethod(http.MethodGet, s.getChatPoll))
	mux.HandleFunc("/api/v1/file/send",            onlyMethod(http.MethodPost, s.postSendFile))
	mux.HandleFunc("/api/v1/file/poll",            onlyMethod(http.MethodGet, s.getFilePoll))
	mux.HandleFunc("/api/v1/file/consent-poll",    onlyMethod(http.MethodGet, s.getConsentPoll))
	mux.HandleFunc("/api/v1/file/consent",         onlyMethod(http.MethodPost, s.postConsent))
	mux.HandleFunc("/api/v1/theme",                onlyMethod(http.MethodPost, s.postTheme))

	s.srv = &http.Server{Handler: s.authMiddleware(mux)}
	go s.srv.Serve(ln) //nolint:errcheck
	go func() {
		<-ctx.Done()
		s.srv.Close()
	}()
	return s.addr, s.token, nil
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-And-Token") != s.token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(errResp{Error: "yetkisiz erişim"}) //nolint:errcheck
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Addr() string { return s.addr }

func onlyMethod(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(errResp{Error: "yöntem izin verilmiyor"}) //nolint:errcheck
			return
		}
		h(w, r)
	}
}

func (s *Server) enforceModeratorRole(w http.ResponseWriter) bool {
	if s.id.IsFounder() || s.id.IsModerator() {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(errResp{Error: "yetersiz yetki — kurucu veya moderatör gerekir"}) //nolint:errcheck
	return false
}

func (s *Server) getPeers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.id.ConnectedPeers())
}

// handleForumPost routes GET (read post) and POST (create post) on /api/v1/forum/post.
func (s *Server) handleForumPost(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getForumPost(w, r)
	case http.MethodPost:
		s.postCreate(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(errResp{Error: "yöntem izin verilmiyor"}) //nolint:errcheck
	}
}

func (s *Server) getForumPost(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validPostID(id) {
		writeBadRequest(w, "geçersiz veya eksik id parametresi")
		return
	}
	detail, err := s.f.GetPost(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if detail == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errResp{Error: "konu bulunamadı"}) //nolint:errcheck
		return
	}
	writeJSON(w, detail)
}

func (s *Server) postReply(w http.ResponseWriter, r *http.Request) {
	var req CreateReplyReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validPostID(req.PostID) {
		writeBadRequest(w, "geçersiz post_id biçimi")
		return
	}
	if req.Body == "" {
		writeBadRequest(w, "body boş olamaz")
		return
	}
	if err := s.f.CreateReply(r.Context(), req.PostID, req.Body); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postSendChat(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeErr(w, fmt.Errorf("chat kullanılamıyor"))
		return
	}
	var req SendChatReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeBadRequest(w, "message boş olamaz")
		return
	}
	if err := s.chat.SendChat(r.Context(), s.id.Name(), req.Message); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getChatPoll(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeJSON(w, []ChatMsg{})
		return
	}
	ch := s.chat.Subscribe()
	defer s.chat.Unsubscribe(ch)
	select {
	case msg := <-ch:
		writeJSON(w, []ChatMsg{msg})
	case <-time.After(5 * time.Second):
		writeJSON(w, []ChatMsg{})
	case <-r.Context().Done():
		writeJSON(w, []ChatMsg{})
	}
}

func (s *Server) getIdentity(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, IdentityInfo{
		Name:   s.id.Name(),
		PubKey: s.id.PubKeyHex(),
		PeerID: s.id.PeerIDStr(),
	})
}

func (s *Server) getRole(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, RoleInfo{IsFounder: s.id.IsFounder(), IsModerator: s.id.IsModerator()})
}

func (s *Server) getPending(w http.ResponseWriter, _ *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	posts, err := s.f.PendingPosts()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, posts)
}

func (s *Server) postApprove(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	var req ApproveReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validPostID(req.PostID) {
		writeBadRequest(w, "geçersiz post_id biçimi")
		return
	}
	if err := s.f.ApprovePost(req.PostID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postApproveAuthor(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	var req ApproveAuthorReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validAuthorKey(req.AuthorKey) {
		writeBadRequest(w, "geçersiz author_key biçimi (64 karakter hex bekleniyor)")
		return
	}
	if err := s.f.ApproveAuthor(req.AuthorKey); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postReject(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	var req RejectReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validPostID(req.PostID) {
		writeBadRequest(w, "geçersiz post_id biçimi")
		return
	}
	if err := s.f.RejectPost(req.PostID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getAllPosts(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	posts, err := s.f.AllPosts()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, posts)
}

func (s *Server) postDeletePost(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	var req DeletePostReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validPostID(req.PostID) {
		writeBadRequest(w, "geçersiz post_id biçimi")
		return
	}
	if err := s.f.DeletePost(req.PostID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postCreate(w http.ResponseWriter, r *http.Request) {
	var req CreatePostReq
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.f.CreatePost(r.Context(), req.Category, req.Title, req.Body, req.PermanentReq); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postSendDM(w http.ResponseWriter, r *http.Request) {
	if s.dm == nil {
		writeErr(w, fmt.Errorf("DM kullanılamıyor"))
		return
	}
	var req SendDMReq
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.dm.SendDM(r.Context(), req.PeerID, s.id.Name(), req.Message); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getDMPoll(w http.ResponseWriter, r *http.Request) {
	if s.dm == nil {
		writeJSON(w, []DMMsg{})
		return
	}
	ch := s.dm.Subscribe()
	defer s.dm.Unsubscribe(ch)
	select {
	case msg := <-ch:
		writeJSON(w, []DMMsg{msg})
	case <-time.After(5 * time.Second):
		writeJSON(w, []DMMsg{})
	case <-r.Context().Done():
		writeJSON(w, []DMMsg{})
	}
}

func (s *Server) postSendFile(w http.ResponseWriter, r *http.Request) {
	if s.file == nil {
		writeErr(w, fmt.Errorf("dosya aktarımı kullanılamıyor"))
		return
	}
	var req SendFileReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.PeerID == "" {
		writeBadRequest(w, "peer_id boş olamaz")
		return
	}
	if req.LocalPath == "" {
		writeBadRequest(w, "local_path boş olamaz")
		return
	}
	if s.dataDir != "" {
		clean := filepath.Clean(req.LocalPath)
		base := filepath.Clean(s.dataDir)
		if clean == base || strings.HasPrefix(clean, base+string(os.PathSeparator)) {
			writeBadRequest(w, "güvenlik kısıtlaması: AND veri dizininden dosya gönderilemez")
			return
		}
	}
	if err := s.file.SendFile(r.Context(), req.PeerID, s.id.Name(), req.LocalPath); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getFilePoll(w http.ResponseWriter, r *http.Request) {
	if s.file == nil {
		writeJSON(w, []FileMsg{})
		return
	}
	ch := s.file.Subscribe()
	defer s.file.Unsubscribe(ch)
	select {
	case msg := <-ch:
		writeJSON(w, []FileMsg{msg})
	case <-time.After(5 * time.Second):
		writeJSON(w, []FileMsg{})
	case <-r.Context().Done():
		writeJSON(w, []FileMsg{})
	}
}

func (s *Server) getConsentPoll(w http.ResponseWriter, r *http.Request) {
	if s.file == nil {
		writeJSON(w, []FileConsentReq{})
		return
	}
	ch := s.file.SubscribeConsent()
	defer s.file.UnsubscribeConsent(ch)
	select {
	case req := <-ch:
		writeJSON(w, []FileConsentReq{req})
	case <-time.After(5 * time.Second):
		writeJSON(w, []FileConsentReq{})
	case <-r.Context().Done():
		writeJSON(w, []FileConsentReq{})
	}
}

func (s *Server) postConsent(w http.ResponseWriter, r *http.Request) {
	if s.file == nil {
		writeBadRequest(w, "dosya backend yok")
		return
	}
	var req ConsentResp
	if !decodeBody(w, r, &req) {
		return
	}
	if req.TransferID == "" {
		writeBadRequest(w, "transfer_id boş olamaz")
		return
	}
	s.file.RespondConsent(req.TransferID, req.Accept)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postTheme(w http.ResponseWriter, r *http.Request) {
	var req ThemeReq
	if !decodeBody(w, r, &req) {
		return
	}
	if s.themeCh != nil {
		select {
		case s.themeCh <- req:
		default:
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(errResp{Error: err.Error()}) //nolint:errcheck
}

func writeBadRequest(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(errResp{Error: msg}) //nolint:errcheck
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeBodyN(w, r, v, 1<<16)
}

func decodeBodyN(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBytes)).Decode(v); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errResp{Error: err.Error()}) //nolint:errcheck
		return false
	}
	return true
}

type Client struct {
	base   string
	token  string
	hc     *http.Client
	hcFile *http.Client // dosya transferleri için timeout yok — transfer süresi öngörülemiyor
}

func NewClientFromEnv() (*Client, error) {
	addr := os.Getenv("AND_API_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("AND_API_ADDR tanımlı değil — bu binary AND tarafından başlatılmalıdır")
	}
	token := os.Getenv("AND_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("AND_API_TOKEN tanımlı değil — bu binary AND tarafından başlatılmalıdır")
	}
	return &Client{
		base:   "http://" + addr,
		token:  token,
		hc:     &http.Client{Timeout: 35 * time.Second},
		hcFile: &http.Client{},
	}, nil
}

func (c *Client) Peers() ([]PeerInfo, error) {
	var v []PeerInfo
	return v, c.get("/api/v1/peers", &v)
}

func (c *Client) Identity() (IdentityInfo, error) {
	var v IdentityInfo
	return v, c.get("/api/v1/identity", &v)
}

func (c *Client) Role() (RoleInfo, error) {
	var v RoleInfo
	return v, c.get("/api/v1/role", &v)
}

func (c *Client) Pending() ([]PendingPost, error) {
	var v []PendingPost
	return v, c.get("/api/v1/forum/pending", &v)
}

func (c *Client) Approve(postID string) error {
	return c.post("/api/v1/forum/approve", ApproveReq{PostID: postID})
}

func (c *Client) ApproveAuthor(authorKey string) error {
	return c.post("/api/v1/forum/approve-author", ApproveAuthorReq{AuthorKey: authorKey})
}

func (c *Client) Reject(postID string) error {
	return c.post("/api/v1/forum/reject", RejectReq{PostID: postID})
}

func (c *Client) AllPosts() ([]PostSummary, error) {
	var v []PostSummary
	return v, c.get("/api/v1/forum/all-posts", &v)
}

func (c *Client) DeletePost(postID string) error {
	return c.post("/api/v1/forum/delete", DeletePostReq{PostID: postID})
}

func (c *Client) CreatePost(category, title, body string, permanentReq bool) error {
	return c.post("/api/v1/forum/post", CreatePostReq{
		Category: category, Title: title, Body: body, PermanentReq: permanentReq,
	})
}

func (c *Client) GetPost(id string) (*PostDetail, error) {
	var v PostDetail
	return &v, c.get("/api/v1/forum/post?id="+id, &v)
}

func (c *Client) CreateReply(postID, body string) error {
	return c.post("/api/v1/forum/reply", CreateReplyReq{PostID: postID, Body: body})
}

func (c *Client) SendChat(message string) error {
	return c.post("/api/v1/chat/send", SendChatReq{Message: message})
}

func (c *Client) PollChat() ([]ChatMsg, error) {
	var v []ChatMsg
	return v, c.get("/api/v1/chat/poll", &v)
}

func (c *Client) SendDM(peerID, message string) error {
	return c.post("/api/v1/dm/send", SendDMReq{PeerID: peerID, Message: message})
}

func (c *Client) PollDM() ([]DMMsg, error) {
	var v []DMMsg
	return v, c.get("/api/v1/dm/poll", &v)
}

func (c *Client) SendFile(peerID, localPath string) error {
	return c.postWith(c.hcFile, "/api/v1/file/send", SendFileReq{PeerID: peerID, LocalPath: localPath})
}

func (c *Client) PollFile() ([]FileMsg, error) {
	var v []FileMsg
	return v, c.get("/api/v1/file/poll", &v)
}

func (c *Client) PollConsent() ([]FileConsentReq, error) {
	var v []FileConsentReq
	return v, c.get("/api/v1/file/consent-poll", &v)
}

func (c *Client) RespondConsent(transferID string, accept bool) error {
	return c.post("/api/v1/file/consent", ConsentResp{TransferID: transferID, Accept: accept})
}

func (c *Client) SetTheme(req ThemeReq) error {
	return c.post("/api/v1/theme", req)
}

func DataDir() string { return os.Getenv("AND_DATA_DIR") }

func Category() string { return os.Getenv("AND_CATEGORY") }

func (c *Client) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-And-Token", c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e errResp
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return fmt.Errorf("api: %s", e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(path string, body any) error {
	return c.postWith(c.hc, path, body)
}

func (c *Client) postWith(hc *http.Client, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-And-Token", c.token)
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	var e errResp
	json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
	return fmt.Errorf("api: %s", e.Error)
}

