package pluginapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"
)

const DMProtocol = "/and/dm/1.0.0"

type Manifest struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
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
type ApproveAuthorReq struct{ AuthorKey string `json:"author_key"` }
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

type errResp struct{ Error string `json:"error"` }

type ForumBackend interface {
	PendingPosts() ([]PendingPost, error)
	ApprovePost(postID string) error
	RejectPost(postID string) error
	ApproveAuthor(authorKey string) error
	CreatePost(ctx context.Context, category, title, body string, permanentReq bool) error
}

type IdentityBackend interface {
	Name() string
	PubKeyHex() string
	PeerIDStr() string
	IsFounder() bool
	IsModerator() bool
}

type DMBackend interface {
	SendDM(ctx context.Context, peerID, senderName, message string) error
	Subscribe() chan DMMsg
	Unsubscribe(ch chan DMMsg)
}

var reHex = regexp.MustCompile(`^[0-9a-fA-F]{8,128}$`)

func validPostID(id string) bool    { return reHex.MatchString(id) }
func validAuthorKey(key string) bool { return len(key) == 64 && reHex.MatchString(key) }

type Server struct {
	addr string
	srv  *http.Server
	id   IdentityBackend
	f    ForumBackend
	dm   DMBackend
}

func NewServer(id IdentityBackend, f ForumBackend, dm DMBackend) *Server {
	return &Server{id: id, f: f, dm: dm}
}

func (s *Server) Start(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("pluginapi: listen: %w", err)
	}
	s.addr = ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/identity",            onlyMethod(http.MethodGet, s.getIdentity))
	mux.HandleFunc("/api/v1/role",                onlyMethod(http.MethodGet, s.getRole))
	mux.HandleFunc("/api/v1/forum/pending",       onlyMethod(http.MethodGet, s.getPending))
	mux.HandleFunc("/api/v1/forum/approve",       onlyMethod(http.MethodPost, s.postApprove))
	mux.HandleFunc("/api/v1/forum/approve-author", onlyMethod(http.MethodPost, s.postApproveAuthor))
	mux.HandleFunc("/api/v1/forum/reject",        onlyMethod(http.MethodPost, s.postReject))
	mux.HandleFunc("/api/v1/forum/post",          onlyMethod(http.MethodPost, s.postCreate))
	mux.HandleFunc("/api/v1/dm/send",             onlyMethod(http.MethodPost, s.postSendDM))
	mux.HandleFunc("/api/v1/dm/poll",             onlyMethod(http.MethodGet, s.getDMPoll))

	s.srv = &http.Server{Handler: mux}
	go s.srv.Serve(ln) //nolint:errcheck
	go func() {
		<-ctx.Done()
		s.srv.Close()
	}()
	return s.addr, nil
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(v); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errResp{Error: err.Error()}) //nolint:errcheck
		return false
	}
	return true
}

type Client struct {
	base string
	hc   *http.Client
}

func NewClientFromEnv() (*Client, error) {
	addr := os.Getenv("AND_API_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("AND_API_ADDR tanımlı değil — bu binary AND tarafından başlatılmalıdır")
	}
	return &Client{
		base: "http://" + addr,
		hc:   &http.Client{Timeout: 35 * time.Second},
	}, nil
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

func (c *Client) CreatePost(category, title, body string, permanentReq bool) error {
	return c.post("/api/v1/forum/post", CreatePostReq{
		Category: category, Title: title, Body: body, PermanentReq: permanentReq,
	})
}

func (c *Client) SendDM(peerID, message string) error {
	return c.post("/api/v1/dm/send", SendDMReq{PeerID: peerID, Message: message})
}

func (c *Client) PollDM() ([]DMMsg, error) {
	var v []DMMsg
	return v, c.get("/api/v1/dm/poll", &v)
}

func DataDir() string { return os.Getenv("AND_DATA_DIR") }

func Category() string { return os.Getenv("AND_CATEGORY") }

func (c *Client) get(path string, out any) error {
	resp, err := c.hc.Get(c.base + path)
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
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Post(c.base+path, "application/json", bytes.NewReader(data))
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
