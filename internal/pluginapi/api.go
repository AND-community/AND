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
	"runtime"
	"strings"
	"time"
)

const DMProtocol = "/and/dm/1.0.0"
const FileProtocol = "/and/file/2.0.0"

// MaxFileBytes is the maximum raw file size accepted for transfer (20 GB).
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

type DeletePostReq struct {
	PostID string `json:"post_id"`
}

type PostSummary struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	AuthorKey  string `json:"author_key"`
}
type CreatePostReq struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
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

type errResp struct {
	Error string `json:"error"`
}

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
	Replies    []ReplyInfo `json:"replies"`
}

// CreateReplyReq is the request body for POST /api/v1/forum/reply.
type CreateReplyReq struct {
	PostID string `json:"post_id"`
	Body   string `json:"body"`
}

type ForumBackend interface {
	AllPosts() ([]PostSummary, error)
	DeletePost(postID string) error
	CreatePost(ctx context.Context, category, title, body string) error
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

func validPostID(id string) bool { return reHex.MatchString(id) }

type Server struct {
	addr        string
	token       string
	dataDir     string
	srv         *http.Server
	externalLn  net.Listener
	externalSrv *http.Server
	id          IdentityBackend
	f           ForumBackend
	dm          DMBackend
	file        FileBackend
	chat        ChatBackend
	themeCh     chan<- ThemeReq
}

func NewServer(id IdentityBackend, f ForumBackend, dm DMBackend, file FileBackend, chat ChatBackend, dataDir string, themeCh chan<- ThemeReq) *Server {
	return &Server{id: id, f: f, dm: dm, file: file, chat: chat, dataDir: dataDir, themeCh: themeCh}
}

// enableExternalAPI, AND_ENABLE_EXTERNAL_API=1 ortam değişkeniyle
// etkinleştirilen, güvenilir harici araçlar (ör. bir editör eklentisi) için
// ikinci bir dinleyici açar (bkz. listenExternal / externalpipe_*.go).
//
// Bu ikinci dinleyici AND_API_ADDR/AND_API_TOKEN mekanizmasından TAMAMEN
// AYRIDIR ve bir bearer token KULLANMAZ: Windows'ta yalnızca pipe'ın
// sahibine (owner) erişim izni veren bir security descriptor'lı named pipe,
// diğer platformlarda ise 0600 izinli bir Unix domain socket olarak açılır.
// Böylece erişim, diskte/bellekte kopyalanabilecek bir sırla değil,
// doğrudan işletim sistemi tarafından ("yalnızca bu kullanıcı") zorlanır —
// bu yüzden bu dinleyiciye gelen istekler authMiddleware'den GEÇMEZ, token
// kontrolü yoktur (bkz. Start).
//
// Pipe/socket adı, AND_DATA_DIR'e göre türetilir ve sabittir (bir sır
// değildir) — bunu bilen bağımsız bir süreç yalnızca BAĞLANMAYI dener,
// bağlanabilmesi OS'in erişim denetimine bağlıdır.
func enableExternalAPI() bool {
	return os.Getenv("AND_ENABLE_EXTERNAL_API") == "1"
}

// route, tek bir HTTP endpoint'inin tanımıdır. allowExternal, bu endpoint'in
// enableExternalAPI() etkinken token GEREKTİRMEYEN harici dinleyicide
// (bkz. externalpipe_*.go) de sunulup sunulmayacağını belirler.
//
// allowExternal alanı KASITLI olarak "false" varsayılana sahiptir (Go'nun
// bool zero value'su) — routeTable'a eklenen yeni bir route, bu alan açıkça
// "true" yazılmadıkça yalnızca token'lı TCP dinleyicisinde sunulur. Böylece
// "harici erişime açık mı?" sorusu her yeni endpoint için BİLİNÇLİ, tek
// satırlık bir karar olmak zorundadır — yalnızca bir yorumu okuyup
// hatırlamaya güvenilmez (bkz. buildMux, Start).
type route struct {
	pattern       string
	handler       http.HandlerFunc
	allowExternal bool
}

// routeTable, Server'ın tüm HTTP endpoint'lerini tanımlayan TEK yerdir.
// Buradaki route'ların tamamı bugün allowExternal:true olarak işaretlidir
// çünkü harici dinleyicinin tüm amacı, VS Code gibi güvenilir bir aracın
// AND'ın normal eklenti API'siyle eşdeğer erişime sahip olmasıdır — ama
// ileride yalnızca token'lı erişime açık kalması gereken bir endpoint
// eklenirse (ör. kimlik anahtarı dışa aktarma), bu alanı yazmadan
// bırakmak yeterlidir.
func (s *Server) routeTable() []route {
	return []route{
		{"/api/v1/identity", onlyMethod(http.MethodGet, s.getIdentity), true},
		{"/api/v1/role", onlyMethod(http.MethodGet, s.getRole), true},
		{"/api/v1/peers", onlyMethod(http.MethodGet, s.getPeers), true},
		{"/api/v1/forum/all-posts", onlyMethod(http.MethodGet, s.getAllPosts), true},
		{"/api/v1/forum/post", s.handleForumPost, true},
		{"/api/v1/forum/reply", onlyMethod(http.MethodPost, s.postReply), true},
		{"/api/v1/forum/delete", onlyMethod(http.MethodPost, s.postDeletePost), true},
		{"/api/v1/dm/send", onlyMethod(http.MethodPost, s.postSendDM), true},
		{"/api/v1/dm/poll", onlyMethod(http.MethodGet, s.getDMPoll), true},
		{"/api/v1/chat/send", onlyMethod(http.MethodPost, s.postSendChat), true},
		{"/api/v1/chat/poll", onlyMethod(http.MethodGet, s.getChatPoll), true},
		{"/api/v1/file/send", onlyMethod(http.MethodPost, s.postSendFile), true},
		{"/api/v1/file/poll", onlyMethod(http.MethodGet, s.getFilePoll), true},
		{"/api/v1/file/consent-poll", onlyMethod(http.MethodGet, s.getConsentPoll), true},
		{"/api/v1/file/consent", onlyMethod(http.MethodPost, s.postConsent), true},
		{"/api/v1/theme", s.handleTheme, true},
	}
}

// buildMux, routeTable()'dan bir *http.ServeMux inşa eder. externalOnly
// true ise yalnızca allowExternal:true olan route'lar eklenir — bu,
// harici dinleyicinin (token'sız) hangi endpoint'lere sahip olduğunu
// routeTable() üzerinden STATİK ve DENETLENEBİLİR kılar.
func (s *Server) buildMux(externalOnly bool) *http.ServeMux {
	return buildMuxFromRoutes(s.routeTable(), externalOnly)
}

// buildMuxFromRoutes, buildMux'un asıl filtreleme mantığıdır — Server'dan
// bağımsız, sabit bir route listesi üzerinde çalışır. Bu ayrım, filtreleme
// davranışının (allowExternal:false bir route'un gerçekten externalOnly
// mux'ta bulunmadığının) gerçek routeTable()'a ihtiyaç duymadan, sabit/test
// amaçlı bir route listesiyle doğrulanabilmesini sağlar (bkz.
// TestBuildMuxExternalFiltering).
func buildMuxFromRoutes(routes []route, externalOnly bool) *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range routes {
		if externalOnly && !r.allowExternal {
			continue
		}
		mux.HandleFunc(r.pattern, r.handler)
	}
	return mux
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

	s.srv = &http.Server{Handler: s.authMiddleware(s.buildMux(false))}
	go s.srv.Serve(ln) //nolint:errcheck

	if enableExternalAPI() && s.dataDir != "" {
		if extLn, perr := listenExternal(s.dataDir); perr != nil {
			fmt.Fprintln(os.Stderr, "pluginapi: harici bağlantı dinleyicisi başlatılamadı:", perr)
		} else {
			s.externalLn = extLn
			// authMiddleware YOK: bu dinleyiciye erişim zaten OS tarafından
			// (yalnızca bu kullanıcı) kısıtlanmış durumda — bkz. enableExternalAPI.
			// buildMux(true), yalnızca routeTable()'da allowExternal:true
			// işaretli route'ları içerir.
			s.externalSrv = &http.Server{Handler: s.buildMux(true)}
			go s.externalSrv.Serve(extLn) //nolint:errcheck
			fmt.Fprintln(os.Stderr, "pluginapi: harici bağlantı etkin —", externalDescription(s.dataDir))
		}
	}

	go func() {
		<-ctx.Done()
		s.srv.Close()
		if s.externalSrv != nil {
			s.externalSrv.Close()
			cleanupExternal(s.dataDir)
		}
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

// nonNilSlice, nil bir dilimi boş (ama nil olmayan) bir dilime çevirir.
// json.Marshal nil bir dilimi "null" olarak kodlar, boş bir dilimi ise
// "[]" olarak — istemciler (ör. VS Code eklentisi) her zaman bir dizi
// bekler; tüm liste döndüren endpoint'ler bu yüzden yanıtlarını buradan
// geçirmelidir (bkz. getPeers, getAllPosts).
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (s *Server) getPeers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, nonNilSlice(s.id.ConnectedPeers()))
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

func (s *Server) getAllPosts(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	posts, err := s.f.AllPosts()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, nonNilSlice(posts))
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
	if err := s.f.CreatePost(r.Context(), req.Category, req.Title, req.Body); err != nil {
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

// pathWithinDir, target'ın dir'in içinde (ya da dir'in kendisi) olup
// olmadığını denetler.
//
// target ÖNCE mutlak yola çözülür (filepath.Abs, süreç çalışma dizinine
// göre) — aksi halde göreli bir yol (ör. "identity.dat" ya da
// "../and/identity.dat") filepath.Clean'den sonra bile dataDir'in mutlak
// yoluyla asla eşleşmez/prefix olmaz ve bu korumayı tamamen atlatır.
//
// Windows ve macOS'ta varsayılan dosya sistemleri büyük/küçük harfe
// duyarsızdır (case-insensitive) — bu yüzden bu platformlarda karşılaştırma
// harf duyarsız yapılır; aksi halde "C:\Users\bob\AppData\Roaming\and\
// identity.dat" ile aynı dosyaya işaret eden farklı harf biçimli bir yol
// (ör. "c:\users\bob\...\identity.dat") bu korumayı atlatabilir.
func pathWithinDir(target, dir string) bool {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		absTarget = filepath.Clean(target)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = filepath.Clean(dir)
	}

	// Sembolik bağları çöz: target metinsel olarak dataDir dışında görünse
	// bile, gerçekte dataDir içindeki bir dosyaya işaret eden bir sembolik
	// bağ olabilir (ör. dataDir dışına yerleştirilmiş, identity.dat'a
	// işaret eden bir link). EvalSymlinks başarısız olursa (ör. dosya henüz
	// yoksa ya da bir sembolik bağ değilse) mutlak yolu olduğu gibi
	// kullanmaya devam ederiz.
	if resolved, rerr := filepath.EvalSymlinks(absTarget); rerr == nil {
		absTarget = resolved
	}
	if resolved, rerr := filepath.EvalSymlinks(absDir); rerr == nil {
		absDir = resolved
	}

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		absTarget = strings.ToLower(absTarget)
		absDir = strings.ToLower(absDir)
	}
	return absTarget == absDir || strings.HasPrefix(absTarget, absDir+string(os.PathSeparator))
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
	if s.dataDir != "" && pathWithinDir(req.LocalPath, s.dataDir) {
		writeBadRequest(w, "güvenlik kısıtlaması: AND veri dizininden dosya gönderilemez")
		return
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

// handleTheme routes GET (read the active theme) and POST (set the active
// theme) on /api/v1/theme.
func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getTheme(w, r)
	case http.MethodPost:
		s.postTheme(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(errResp{Error: "yöntem izin verilmiyor"}) //nolint:errcheck
	}
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

// ThemeFile is the JSON file (under AND_DATA_DIR) that internal/tui persists
// the active theme to whenever it's changed (see internal/tui.SaveTheme).
// pluginapi reads it directly for GET /api/v1/theme instead of importing
// internal/tui, which would create an import cycle (tui already imports
// pluginapi for ThemeReq).
const ThemeFile = "theme.json"

// defaultTheme mirrors AND's built-in default colors (internal/tui/styles.go)
// so plugins that ask for the active theme before any custom theme has ever
// been set still get a theme that matches what the user actually sees.
var defaultTheme = ThemeReq{
	ThemeName: "Turkuaz",
	Accent:    "63",
	SelBG:     "57",
	SelFG:     "255",
	Muted:     "241",
	Name:      "86",
	Badge:     "33",
}

// getTheme returns the theme AND's main TUI currently has active, so
// plugins can match their own color scheme to it.
func (s *Server) getTheme(w http.ResponseWriter, _ *http.Request) {
	if s.dataDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.dataDir, ThemeFile)); err == nil {
			var req ThemeReq
			if json.Unmarshal(data, &req) == nil {
				writeJSON(w, req)
				return
			}
		}
	}
	writeJSON(w, defaultTheme)
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

func (c *Client) AllPosts() ([]PostSummary, error) {
	var v []PostSummary
	return v, c.get("/api/v1/forum/all-posts", &v)
}

func (c *Client) DeletePost(postID string) error {
	return c.post("/api/v1/forum/delete", DeletePostReq{PostID: postID})
}

func (c *Client) CreatePost(category, title, body string) error {
	return c.post("/api/v1/forum/post", CreatePostReq{
		Category: category, Title: title, Body: body,
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

// GetTheme returns the theme AND's main TUI currently has active. Plugins
// can use this to match their own color scheme to the user's chosen theme.
func (c *Client) GetTheme() (ThemeReq, error) {
	var v ThemeReq
	return v, c.get("/api/v1/theme", &v)
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
