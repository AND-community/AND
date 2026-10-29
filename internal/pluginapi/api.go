package pluginapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lucian95511/and/internal/ui"
	"github.com/lucian95511/and/protokol"
)

const DMProtocol = protokol.DMProtokolu
const FileProtocol = protokol.DosyaProtokolu

// MaxFileBytes is the maximum raw file size accepted for transfer (20 GB).
const MaxFileBytes = 20 * 1024 * 1024 * 1024

type Manifest struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Category    string `json:"category,omitempty"`
	// Permissions, eklentinin istediği izinlerdir (bkz. IzinBilgileri). AND
	// ilk açılışta kullanıcıya sorar; verilmeyen izni gerektiren uç noktalar
	// 403 döner. Alan yoksa eklenti yalnızca herkese açık okumaları yapabilir.
	Permissions []string `json:"permissions,omitempty"`
	// MinAPI, eklentinin çalışmak için gereken en düşük APIVersion'dır.
	MinAPI int `json:"min_api,omitempty"`
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
	Reason string `json:"reason,omitempty"`
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

// SitePageInfo is one page of the CALLER's OWN personal site — every site
// endpoint is implicitly scoped to the calling identity (mirrors
// internal/site.Store's own PublishPage/DeletePage/PagesByAuthor contract),
// so unlike PostSummary there is no AuthorKey field: it would always equal
// the caller's own key.
type SitePageInfo struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PublishSitePageReq is the request body for POST /api/v1/site/publish —
// creates a new page (if Slug is unused) or wholesale-replaces an existing
// one (if Slug already belongs to the caller); there is no partial update.
type PublishSitePageReq struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// DeleteSitePageReq is the request body for POST /api/v1/site/delete.
type DeleteSitePageReq struct {
	Slug string `json:"slug"`
}

type SendDMReq struct {
	PeerID  string `json:"peer_id"`
	Message string `json:"message"`
	// QueueIfOffline, alıcıya şu an ulaşılamıyorsa mesajın hata yerine giden
	// kutusuna alınmasını ister: alıcı bağlanınca gönderilir, geçmişte o
	// zamana kadar Pending görünür. Alanı bilmeyen eski bir AND yok sayar
	// (eski davranış: ulaşılamazsa hata).
	QueueIfOffline bool `json:"queue_if_offline,omitempty"`
}

// DMReadReq, POST /api/v1/dm/read gövdesidir: PeerID ile konuşmanın At'a
// kadar okunduğunu bildirir.
type DMReadReq struct {
	PeerID string    `json:"peer_id"`
	At     time.Time `json:"at"`
}

type DMMsg struct {
	// From is a display string: the sender's self-declared name, or (if
	// they set one) that name combined with a truncated prefix of their
	// verified peer ID — see internal/dmmgr's combineIdentity. It is meant
	// for showing to a human, NOT for authentication: a plugin that needs
	// to check "did this specific peer ID really send this" must use
	// VerifiedPeerID instead.
	From string `json:"from"`
	// VerifiedPeerID is the sender's full libp2p peer ID as established by
	// the authenticated connection the DM arrived on (cryptographically
	// bound, unlike From's self-declared name). Plugins implementing any
	// kind of "only accept this action from a specific known peer" check
	// (e.g. group membership/roster control messages) must compare
	// against this field, not against an identity claimed inside the
	// message payload itself — otherwise any peer can impersonate any
	// other by simply writing someone else's ID into the payload.
	VerifiedPeerID string    `json:"verified_peer_id"`
	Text           string    `json:"text"`
	ReceivedAt     time.Time `json:"received_at"`
}

// DMRecord, bu düğümün gördüğü bir özel mesajdır (gelen ya da giden) — GET
// /api/v1/dm/history. /dm/poll yalnızca o anda dinleyen bir eklentiye
// iletir; geçmiş, eklenti kapalıyken gelen mesajların kaybolmamasını sağlar.
// Kayıtlar geliş sırasıyladır. AND geçmişi kimliğin anahtarından türetilen
// bir anahtarla şifreleyip diske de yazar (bkz. dmmgr/kalici.go); insanlar
// arası mesajlar ve eklentilerin makine mesajları (bkz.
// protokol.MakineIletisiMi) ayrı sınırlarla tutulur.
type DMRecord struct {
	// PeerID, konuşulan eştir: gelen mesajda gönderen, giden mesajda alıcı.
	// Gelende bağlantının kendisinden doğrulanmıştır.
	PeerID string `json:"peer_id"`
	// Name, gelen mesajda gönderenin KENDİ beyan ettiği görünen addır
	// (doğrulanmamış); giden mesajda boştur.
	Name     string    `json:"name,omitempty"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"` // bu düğümün saatiyle alma/gönderme zamanı
	Outgoing bool      `json:"outgoing,omitempty"`
	// ID, kaydın kimliğidir (giden kutusundaki mesajın durumu bu kimlikle
	// güncellenir). Eski bir AND göndermez.
	ID string `json:"id,omitempty"`
	// Pending, giden kutusunda alıcının bağlanmasını bekleyen mesajdır.
	Pending bool `json:"pending,omitempty"`
	// Failed, giden kutusunda süresi dolup gönderilemeyen mesajdır.
	Failed bool `json:"failed,omitempty"`
}

// DMQueueBackend, alıcıya ulaşılamadığında mesajı giden kutusuna alabilen
// bir DMBackend'dir (isteğe bağlı).
type DMQueueBackend interface {
	SendDMQueued(ctx context.Context, peerID, senderName, message string) error
}

// DMReadBackend, konuşma başına okundu bilgisini tutan bir DMBackend'dir
// (isteğe bağlı). AND'ın ana menüsü ve eklentiler aynı bilgiyi görür.
type DMReadBackend interface {
	MarkRead(peerID string, at time.Time)
	ReadMarks() map[string]time.Time
}

// DMHistoryBackend, DM geçmişini sunabilen bir DMBackend'dir (isteğe bağlı).
type DMHistoryBackend interface {
	History() []DMRecord
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
	// AND, eşin AND protokolünü konuştuğunun bilindiğidir (identify ile).
	// Bağlı eşlerin çoğu genel DHT düğümüdür; özel mesaj ya da dosya
	// gönderilebilecek eşler yalnızca AND olanlardır. Eski bir AND bu alanı
	// hiç göndermez (false).
	AND bool `json:"and,omitempty"`
}

// ChatMsg is a general chat message received from the shared channel.
type ChatMsg struct {
	// ID, when non-empty, is a stable reference for this message (matches
	// internal/chatlog.Message.ID) — set on messages returned by AllChat,
	// omitted on the live PollChat stream (which predates it and has no
	// equivalent stable identity to offer).
	ID     string    `json:"id,omitempty"`
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

// BanEntryInfo is one row of GET /api/v1/mod/bans — a currently-active ban.
type BanEntryInfo struct {
	PeerID       string    `json:"peer_id"`
	AuthorKey    string    `json:"author_key,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Permanent    bool      `json:"permanent"`
	ModeratorKey string    `json:"moderator_key"`
	IsFounderMod bool      `json:"is_founder_mod"`
	Reason       string    `json:"reason,omitempty"`
}

// UnbanReq is the request body for POST /api/v1/mod/unban.
type UnbanReq struct {
	AuthorKey string `json:"author_key"`
}

// HistoryEntryInfo is one row of GET /api/v1/mod/history — a past ban or
// unban action, kept even after the ban itself expired/was lifted.
type HistoryEntryInfo struct {
	Action       string    `json:"action"` // "ban" | "unban"
	PeerID       string    `json:"peer_id"`
	Reason       string    `json:"reason,omitempty"`
	ModeratorKey string    `json:"moderator_key"`
	IsFounderMod bool      `json:"is_founder_mod"`
	At           time.Time `json:"at"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Permanent    bool      `json:"permanent,omitempty"`
}

// GrantedModInfo is one row of GET /api/v1/mod/grants — a moderator
// certificate this node (as founder) has issued.
type GrantedModInfo struct {
	Key       string    `json:"key"`
	GrantedAt time.Time `json:"granted_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Permanent bool      `json:"permanent,omitempty"`
	Revoked   bool      `json:"revoked,omitempty"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
}

// GrantReq is the request body for POST /api/v1/mod/grant.
type GrantReq struct {
	TargetKeyHex string `json:"target_key_hex"`
	DurationSec  int64  `json:"duration_sec,omitempty"`
	Permanent    bool   `json:"permanent,omitempty"`
}

// GrantResp is the response body for POST /api/v1/mod/grant.
type GrantResp struct {
	CertPath string `json:"cert_path,omitempty"`
}

// RevokeReq is the request body for POST /api/v1/mod/revoke.
type RevokeReq struct {
	TargetKeyHex string `json:"target_key_hex"`
}

// ModerationBackend exposes admin/moderator tooling to plugins (bkz.
// Eklentiler/admin). Every endpoint built on this is gated by
// enforceModeratorRole at minimum; Grant/Revoke (moderator atama/kaldırma,
// yalnızca kurucuya özel) are additionally gated by enforceFounderRole.
type ModerationBackend interface {
	AllBans() ([]BanEntryInfo, error)
	Unban(ctx context.Context, authorKey string) error
	History() ([]HistoryEntryInfo, error)
	GrantedMods() ([]GrantedModInfo, error)
	// Grant returns the path the new certificate was written to (may be
	// empty if dataDir isn't set) — the founder delivers this file to the
	// target out-of-band (e.g. via Özel Chat).
	Grant(ctx context.Context, targetKeyHex string, duration time.Duration, permanent bool) (certPath string, err error)
	Revoke(ctx context.Context, targetKeyHex string) error
}

type ForumBackend interface {
	AllPosts() ([]PostSummary, error)
	// DeletePost issues an authorized (network-wide, tombstoned) deletion
	// of postID — the caller's moderator/founder authority is resolved
	// by the backend itself; reason may be empty.
	DeletePost(ctx context.Context, postID, reason string) error
	CreatePost(ctx context.Context, category, title, body string) error
	GetPost(id string) (*PostDetail, error)
	CreateReply(ctx context.Context, postID, body string) error
}

// SiteBackend exposes the caller's OWN personal-site pages to plugins.
// Every method is implicitly scoped to the calling node's identity — there
// is no "target author" parameter anywhere, mirroring internal/site.Store's
// own PublishPage/DeletePage/PagesByAuthor contract, so a plugin built on
// this can only ever manage its own pages, never someone else's.
type SiteBackend interface {
	MyPages() ([]SitePageInfo, error)
	// PublishPage creates slug (if unused) or wholesale-replaces it (if it
	// already belongs to the caller) — no partial-update semantics.
	PublishPage(ctx context.Context, slug, title, body string) error
	DeletePage(ctx context.Context, slug string) error
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
	// AllChat returns the full locally-known chat history (persisted,
	// not just what's arrived since this process started) — unlike
	// Subscribe, which only forwards new messages as they arrive live.
	AllChat() []ChatMsg
}

var reHex = regexp.MustCompile(`^[0-9a-fA-F]{8,128}$`)

// rePeerID, bir libp2p Peer ID'sinin biçimidir (base58: 0, O, I ve l yok).
// Burada libp2p'yi içe aktarmak her eklentiye libp2p'yi de taşırdı; tam
// doğrulamayı arka uç yapar.
var rePeerID = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{46,64}$`)

// GecerliPeerID, s'nin bir Peer ID biçiminde olup olmadığıdır — AND'ın
// kendisinin uyguladığı denetimin aynısı. Eklentiler, kullanıcının yazdığı
// bir Peer ID'yi göndermeden önce bununla sınayabilir.
func GecerliPeerID(s string) bool { return rePeerID.MatchString(s) }

func validPostID(id string) bool { return reHex.MatchString(id) }

// reAuthorKey, bir author_key/target_key_hex alanının tam olarak 64 hex
// karakter (32 baytlık bir ed25519 public key) olmasını zorunlu kılar.
var reAuthorKey = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// validAuthorKey, moderasyon uç noktalarına (mod/unban, mod/grant,
// mod/revoke) gelen hex-encoded author key alanlarını API sınırında
// doğrular. Bunsuz, biçimsiz bir değer (boş olmayan ama kısa/hex-olmayan)
// resolveOwnModAuthority/AuthorKeyToPeerID'e kadar ilerler ve orada genel,
// sarmalanmış bir hatayla reddedilir — bu da API'nin 400 sözleşmesini
// tutarsız hale getirir (bkz. eski postApproveAuthor'daki eşdeğer kontrol).
func validAuthorKey(key string) bool { return reAuthorKey.MatchString(key) }

type Server struct {
	addr    string
	token   string
	dataDir string
	// yetkiler, geçerli API anahtarlarının (özetleriyle) yetkileridir: Start'ın
	// döndürdüğü tam yetkili ana anahtar ve EklentiAnahtari'nın eklenti
	// başına verdikleri.
	yetkiMu     sync.RWMutex
	yetkiler    map[[sha256.Size]byte]yetki
	srv         *http.Server
	externalLn  net.Listener
	externalSrv *http.Server
	id          IdentityBackend
	f           ForumBackend
	site        SiteBackend
	dm          DMBackend
	file        FileBackend
	chat        ChatBackend
	mb          ModerationBackend
	themeCh     chan<- ThemeReq

	// Yerel hız sınırlayıcılar — yalnızca pluginapi ÜZERİNDEN (eklenti ya
	// da harici dinleyici) gelen çağrılara uygulanır, AND'ın kendi TUI'sini
	// hiç etkilemez. Bkz. ratelimit.go'daki localActionLimiter'ın doc
	// yorumu.
	// Her biri AYRI bir bütçedir — kasıtlı olarak PAYLAŞTIRILMAZ: hatalı ya
	// da kötü niyetli TEK bir eklentinin (ör. forum'u kötüye kullanan bir
	// eklenti) kendi bütçesini tüketmesi, İLGİSİZ bir özelliği (ör. site
	// sayfası yayınlamak isteyen BAŞKA bir eklenti) hiç etkilememelidir.
	forumLimiter *localActionLimiter // forum/post, forum/reply
	siteLimiter  *localActionLimiter // site/publish, site/delete
	chatLimiter  *localActionLimiter // chat/send
	dmLimiter    *localActionLimiter // dm/send
	fileLimiter  *localActionLimiter // file/send
}

// site may be nil (e.g. a minimal test server) — every site handler
// nil-checks it first, matching the dm/file optional-backend convention.
func NewServer(id IdentityBackend, f ForumBackend, site SiteBackend, dm DMBackend, file FileBackend, chat ChatBackend, mb ModerationBackend, dataDir string, themeCh chan<- ThemeReq) *Server {
	return &Server{
		id: id, f: f, site: site, dm: dm, file: file, chat: chat, mb: mb, dataDir: dataDir, themeCh: themeCh,
		forumLimiter: newLocalActionLimiter(contentActionLimit, localActionsWindow),
		siteLimiter:  newLocalActionLimiter(contentActionLimit, localActionsWindow),
		chatLimiter:  newLocalActionLimiter(chatActionLimit, localActionsWindow),
		dmLimiter:    newLocalActionLimiter(dmActionLimit, localActionsWindow),
		fileLimiter:  newLocalActionLimiter(fileActionLimit, localActionsWindow),
	}
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
//
// izin, uç noktanın gerektirdiği eklenti iznidir (IzinTemel ya da
// IzinBilgileri'nden biri); TestRouteTableIzinleri her route'un bilinçli
// bir izin taşıdığını denetler. okumaSerbest ise GET istekleri izinsizdir.
type route struct {
	pattern       string
	handler       http.HandlerFunc
	allowExternal bool
	izin          string
	okumaSerbest  bool
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
		{pattern: "/api/v1/version", handler: onlyMethod(http.MethodGet, s.getVersion), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/identity", handler: onlyMethod(http.MethodGet, s.getIdentity), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/role", handler: onlyMethod(http.MethodGet, s.getRole), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/peers", handler: onlyMethod(http.MethodGet, s.getPeers), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/forum/all-posts", handler: onlyMethod(http.MethodGet, s.getAllPosts), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/forum/post", handler: s.handleForumPost, allowExternal: true, izin: IzinForum, okumaSerbest: true},
		{pattern: "/api/v1/forum/reply", handler: onlyMethod(http.MethodPost, rateLimited(s.forumLimiter, s.postReply)), allowExternal: true, izin: IzinForum},
		{pattern: "/api/v1/forum/delete", handler: onlyMethod(http.MethodPost, s.postDeletePost), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/site/pages", handler: onlyMethod(http.MethodGet, s.getSitePages), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/site/publish", handler: onlyMethod(http.MethodPost, rateLimited(s.siteLimiter, s.postSitePublish)), allowExternal: true, izin: IzinSite},
		{pattern: "/api/v1/site/delete", handler: onlyMethod(http.MethodPost, rateLimited(s.siteLimiter, s.postSiteDelete)), allowExternal: true, izin: IzinSite},
		{pattern: "/api/v1/mod/bans", handler: onlyMethod(http.MethodGet, s.getModBans), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/mod/unban", handler: onlyMethod(http.MethodPost, s.postModUnban), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/mod/history", handler: onlyMethod(http.MethodGet, s.getModHistory), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/mod/grants", handler: onlyMethod(http.MethodGet, s.getModGrants), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/mod/grant", handler: onlyMethod(http.MethodPost, s.postModGrant), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/mod/revoke", handler: onlyMethod(http.MethodPost, s.postModRevoke), allowExternal: true, izin: IzinModerasyon},
		{pattern: "/api/v1/dm/send", handler: onlyMethod(http.MethodPost, rateLimited(s.dmLimiter, s.postSendDM)), allowExternal: true, izin: IzinDM},
		{pattern: "/api/v1/dm/poll", handler: onlyMethod(http.MethodGet, s.getDMPoll), allowExternal: true, izin: IzinDM},
		{pattern: "/api/v1/dm/history", handler: onlyMethod(http.MethodGet, s.getDMHistory), allowExternal: true, izin: IzinDM},
		{pattern: "/api/v1/dm/read", handler: s.handleDMRead, allowExternal: true, izin: IzinDM},
		{pattern: "/api/v1/chat/send", handler: onlyMethod(http.MethodPost, rateLimited(s.chatLimiter, s.postSendChat)), allowExternal: true, izin: IzinSohbet},
		{pattern: "/api/v1/chat/poll", handler: onlyMethod(http.MethodGet, s.getChatPoll), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/chat/all", handler: onlyMethod(http.MethodGet, s.getChatAll), allowExternal: true, izin: IzinTemel},
		{pattern: "/api/v1/file/send", handler: onlyMethod(http.MethodPost, rateLimited(s.fileLimiter, s.postSendFile)), allowExternal: true, izin: IzinDosya},
		{pattern: "/api/v1/file/poll", handler: onlyMethod(http.MethodGet, s.getFilePoll), allowExternal: true, izin: IzinDosya},
		{pattern: "/api/v1/file/consent-poll", handler: onlyMethod(http.MethodGet, s.getConsentPoll), allowExternal: true, izin: IzinDosya},
		{pattern: "/api/v1/file/consent", handler: onlyMethod(http.MethodPost, s.postConsent), allowExternal: true, izin: IzinDosya},
		{pattern: "/api/v1/theme", handler: s.handleTheme, allowExternal: true, izin: IzinTema, okumaSerbest: true},
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
		mux.HandleFunc(r.pattern, izinDenetle(r.izin, r.okumaSerbest, r.handler))
	}
	return mux
}

// newHTTPServer, API dinleyicilerinin ortak sunucu ayarlarıdır. Başlıklar
// için süre ve boyut sınırı vardır — başlığını hiç bitirmeyen ya da çok yavaş
// gönderen bir istemci bağlantıyı süresiz tutamaz; boştaki bağlantılar da
// kapanır. ReadTimeout ve WriteTimeout BİLEREK yoktur: ikisi de istek
// sürerken dolarsa uzun süre yanıt bekleyen (long-poll) uç noktaları
// (/chat/poll, /dm/poll, dosya onayı) keserdi. Gövdeler zaten her uç noktada
// LimitReader ile sınırlıdır (bkz. decodeBodyN).
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
}

func (s *Server) Start(ctx context.Context) (addr, token string, err error) {
	ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
	if listenErr != nil {
		return "", "", fmt.Errorf("pluginapi: listen: %w", listenErr)
	}
	s.addr = ln.Addr().String()

	// Ana anahtar tam yetkilidir; AND'ın kendisi ve testler kullanır. Eklentiler
	// bunu değil, EklentiAnahtari'nın verdiği izinli anahtarı alır.
	tok, randErr := yeniAnahtar()
	if randErr != nil {
		ln.Close() //nolint:errcheck
		return "", "", fmt.Errorf("pluginapi: generate token: %w", randErr)
	}
	s.token = tok
	s.yetkiKaydet(tok, yetki{tam: true})

	s.srv = newHTTPServer(s.authMiddleware(s.buildMux(false)))
	go s.srv.Serve(ln) //nolint:errcheck

	if enableExternalAPI() && s.dataDir != "" {
		if extLn, perr := listenExternal(s.dataDir); perr != nil {
			fmt.Fprintln(os.Stderr, "pluginapi: harici bağlantı dinleyicisi başlatılamadı:", perr)
		} else {
			s.externalLn = extLn
			// authMiddleware YOK: bu dinleyiciye erişim zaten OS tarafından
			// (yalnızca bu kullanıcı) kısıtlanmış durumda — bkz. enableExternalAPI.
			// buildMux(true), yalnızca routeTable()'da allowExternal:true
			// işaretli route'ları içerir; buradaki araç tam yetkilidir.
			s.externalSrv = newHTTPServer(tamYetkiyle(s.buildMux(true)))
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

// authMiddleware, X-And-Token'ı geçerli anahtarlar arasında arar ve
// bulduğu yetkiyi isteğe ekler; her uç nokta izni ona göre denetler (bkz.
// izinDenetle). Arama ham anahtarın değil özetinin üzerinden yapılır (bkz.
// anahtarOzeti).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		y, ok := s.yetkiBul(r.Header.Get("X-And-Token"))
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(errResp{Error: "yetkisiz erişim"}) //nolint:errcheck
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), yetkiAnahtari{}, y)))
	})
}

func (s *Server) Addr() string { return s.addr }

// rateLimited, h'yi çağırmadan ÖNCE limiter'ı kontrol eden bir sarmalayıcı
// döner — onlyMethod gibi routeTable() içinde birleştirilmek üzere
// tasarlanmıştır. Hız sınırlamasını (önceden her handler'ın gövdesinde
// elle tekrarlanan bir kontroldü) route tanımının kendisine taşımak,
// routeTable()'a eklenecek yeni bir içerik-oluşturma uç noktasının hız
// sınırını UNUTMASINI derleme zamanında engellemez ama en azından
// eksikliği tek bakışta görünür/denetlenebilir kılar — method kısıtlaması
// ve harici erişim gibi diğer route-seviyesi politikalarla aynı yerde.
func rateLimited(limiter *localActionLimiter, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow() {
			writeTooManyRequests(w, rateLimitErrMessage)
			return
		}
		h(w, r)
	}
}

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

// enforceFounderRole gates moderator-management actions (grant/revoke) —
// stricter than enforceModeratorRole, since only the founder may create or
// withdraw another moderator's authority.
func (s *Server) enforceFounderRole(w http.ResponseWriter) bool {
	if s.id.IsFounder() {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(errResp{Error: "yetersiz yetki — yalnızca kurucu"}) //nolint:errcheck
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
		rateLimited(s.forumLimiter, s.postCreate)(w, r)
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

func (s *Server) getChatAll(w http.ResponseWriter, _ *http.Request) {
	if s.chat == nil {
		writeJSON(w, []ChatMsg{})
		return
	}
	writeJSON(w, s.chat.AllChat())
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

// getAllPosts, moderatör rolüyle sınırlıdır: getForumPost'un aksine bu,
// TÜM forum içeriğinin toplu bir dökümüdür (moderasyon araçları için) —
// tek bir gönderiyi id ile okumak (getForumPost, ör. Eklentiler/ornek'in
// normal kullanımı) forumun P2P'de zaten herkese açık yayınlanan içeriği
// olduğu için bilerek rol kontrolü GEREKTİRMEZ; kısıtlanan şey "tek bir
// bilinen gönderiyi oku" değil "tüm forumu toplu indir" yeteneğidir.
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
	if err := s.f.DeletePost(r.Context(), req.PostID, req.Reason); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getSitePages/postSitePublish/postSiteDelete back the "site" plugin
// (Eklentiler/site) — every call is implicitly scoped to the CALLING
// node's own identity (bkz. SiteBackend'in doc yorumu), so unlike forum's
// moderator-only getAllPosts/postDeletePost there is no role check here:
// managing your own pages requires no more authority than posting to the
// forum does.
func (s *Server) getSitePages(w http.ResponseWriter, r *http.Request) {
	if s.site == nil {
		writeErr(w, fmt.Errorf("site özelliği kullanılamıyor"))
		return
	}
	pages, err := s.site.MyPages()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, nonNilSlice(pages))
}

func (s *Server) postSitePublish(w http.ResponseWriter, r *http.Request) {
	if s.site == nil {
		writeErr(w, fmt.Errorf("site özelliği kullanılamıyor"))
		return
	}
	var req PublishSitePageReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Slug == "" {
		writeBadRequest(w, "slug boş olamaz")
		return
	}
	if err := s.site.PublishPage(r.Context(), req.Slug, req.Title, req.Body); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) postSiteDelete(w http.ResponseWriter, r *http.Request) {
	if s.site == nil {
		writeErr(w, fmt.Errorf("site özelliği kullanılamıyor"))
		return
	}
	var req DeleteSitePageReq
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Slug == "" {
		writeBadRequest(w, "slug boş olamaz")
		return
	}
	if err := s.site.DeletePage(r.Context(), req.Slug); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getModBans(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	bans, err := s.mb.AllBans()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, nonNilSlice(bans))
}

func (s *Server) postModUnban(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	var req UnbanReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validAuthorKey(req.AuthorKey) {
		writeBadRequest(w, "geçersiz author_key biçimi")
		return
	}
	if err := s.mb.Unban(r.Context(), req.AuthorKey); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getModHistory(w http.ResponseWriter, r *http.Request) {
	if !s.enforceModeratorRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	h, err := s.mb.History()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, nonNilSlice(h))
}

func (s *Server) getModGrants(w http.ResponseWriter, r *http.Request) {
	if !s.enforceFounderRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	g, err := s.mb.GrantedMods()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, nonNilSlice(g))
}

func (s *Server) postModGrant(w http.ResponseWriter, r *http.Request) {
	if !s.enforceFounderRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	var req GrantReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validAuthorKey(req.TargetKeyHex) {
		writeBadRequest(w, "geçersiz target_key_hex biçimi")
		return
	}
	certPath, err := s.mb.Grant(r.Context(), req.TargetKeyHex, time.Duration(req.DurationSec)*time.Second, req.Permanent)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, GrantResp{CertPath: certPath})
}

func (s *Server) postModRevoke(w http.ResponseWriter, r *http.Request) {
	if !s.enforceFounderRole(w) {
		return
	}
	if s.mb == nil {
		writeErr(w, fmt.Errorf("moderasyon kullanılamıyor"))
		return
	}
	var req RevokeReq
	if !decodeBody(w, r, &req) {
		return
	}
	if !validAuthorKey(req.TargetKeyHex) {
		writeBadRequest(w, "geçersiz target_key_hex biçimi")
		return
	}
	if err := s.mb.Revoke(r.Context(), req.TargetKeyHex); err != nil {
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
	gonder := s.dm.SendDM
	if q, ok := s.dm.(DMQueueBackend); ok && req.QueueIfOffline {
		gonder = q.SendDMQueued
	}
	if err := gonder(r.Context(), req.PeerID, s.id.Name(), req.Message); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDMRead: GET okundu bilgisini (Peer ID → zaman) döner, POST bir
// konuşmayı okundu işaretler. Arka uç okundu bilgisi tutmuyorsa 404 döner;
// eklenti kendi kaydına düşer.
func (s *Server) handleDMRead(w http.ResponseWriter, r *http.Request) {
	rb, ok := s.dm.(DMReadBackend)
	if !ok || rb == nil {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, rb.ReadMarks())
	case http.MethodPost:
		var req DMReadReq
		if !decodeBody(w, r, &req) {
			return
		}
		if !rePeerID.MatchString(req.PeerID) || req.At.IsZero() {
			writeBadRequest(w, "geçerli peer_id ve at gerekli")
			return
		}
		rb.MarkRead(req.PeerID, req.At)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// getDMHistory, bu oturumdaki özel mesajları (en eskiden en yeniye) döner.
func (s *Server) getDMHistory(w http.ResponseWriter, r *http.Request) {
	hb, ok := s.dm.(DMHistoryBackend)
	if !ok || hb == nil {
		writeJSON(w, []DMRecord{})
		return
	}
	writeJSON(w, gecmisKesiti(hb.History(), r.URL.Query().Get("from")))
}

// gecmisKesiti, ?from=<kimlik> verildiyse geçmişi o kayıttan (dahil)
// itibaren keser: sürekli yoklayan bir eklenti (ör. DeltaX) her turda tüm
// geçmişi almaz. Kayıt geçmişte yoksa (sınırdan düşmüşse) tamamı döner;
// eklenti hangisi olduğunu, kaydın yanıtta olup olmamasından anlar.
func gecmisKesiti(h []DMRecord, from string) []DMRecord {
	if from == "" {
		return h
	}
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].ID == from {
			return h[i:]
		}
	}
	return h
}

// maxDMPollBatch, tek bir /dm/poll yanıtındaki en fazla mesaj sayısıdır.
const maxDMPollBatch = 32

func (s *Server) getDMPoll(w http.ResponseWriter, r *http.Request) {
	if s.dm == nil {
		writeJSON(w, []DMMsg{})
		return
	}
	ch := s.dm.Subscribe()
	defer s.dm.Unsubscribe(ch)
	select {
	case msg := <-ch:
		// Kanalda bekleyenler de bu yanıtla gider: abonelik istek bitince
		// kapanır ve aynı anda gelen ikinci mesaj bu eklenti için kaybolurdu.
		msgs := []DMMsg{msg}
	topla:
		for len(msgs) < maxDMPollBatch {
			select {
			case m := <-ch:
				msgs = append(msgs, m)
			default:
				break topla
			}
		}
		writeJSON(w, msgs)
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
	// işaret eden bir link). İki taraf da aynı biçimde çözülmeli: biri
	// çözülüp öbürü çözülmezse Windows'ta aynı klasörün kısa (8.3) ve uzun
	// adları farklı sayılır.
	absTarget = cozulmusYol(absTarget)
	absDir = cozulmusYol(absDir)

	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		absTarget = strings.ToLower(absTarget)
		absDir = strings.ToLower(absDir)
	}
	return absTarget == absDir || strings.HasPrefix(absTarget, absDir+string(os.PathSeparator))
}

// cozulmusYol, yolun var olan en uzun önekini EvalSymlinks ile çözüp kalan
// (henüz var olmayan) parçaları ekler. Hedef dosya yoksa da sembolik bağlar
// çözülür ve Windows'un kısa (8.3) adları uzun adlara çevrilir; hiçbir önek
// çözülemezse yol olduğu gibi döner.
func cozulmusYol(yol string) string {
	kalan := ""
	for p := yol; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, kalan)
		}
		ust := filepath.Dir(p)
		if ust == p {
			return yol
		}
		kalan = filepath.Join(filepath.Base(p), kalan)
		p = ust
	}
}

// sensitiveHomeSubdirs, kullanıcı ana dizini içinde olsa bile
// forbiddenSendPath'in her zaman reddettiği alt dizinlerdir — SSH/GPG/bulut
// kimlik bilgileri gibi, "kullanıcının kendi dosyası" görünümü altında P2P
// ağa sessizce sızdırılabilecek en yüksek değerli hedefler.
var sensitiveHomeSubdirs = []string{".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker"}

// forbiddenSendPath, localPath'in /api/v1/file/send ile gönderilmesine izin
// verilmeyen bir konumda olması durumunda boş olmayan bir sebep döner.
//
// Önceden yalnızca AND'ın kendi veri dizini (dataDir) engelleniyordu — bu,
// işletim sisteminin AND sürecine okuma izni verdiği HERHANGİ BİR dosyanın
// (SSH anahtarları, başka kullanıcıların dosyaları, sistem dosyaları) hiçbir
// kullanıcı onayı olmadan bir eklenti (ya da AND_ENABLE_EXTERNAL_API
// açıkken aynı makinedeki herhangi bir yerel süreç) tarafından ağa
// gönderilebileceği anlamına geliyordu. Bu tam bir çözüm DEĞİLDİR — asıl
// kalıcı çözüm gönderimi TUI'de görünür bir kullanıcı onayına bağlamaktır
// (ozel_chat'in kendi dosya seçici diyaloğunun zaten yaptığı gibi) — ama API
// sunucu tarafında hiçbir sınır yoktu; bu en azından kullanıcının ana dizini
// DIŞINDAKİ (sistem dosyaları, başka kullanıcıların verileri) ve ana dizin
// İÇİNDEKİ bilinen kimlik bilgisi konumlarını devre dışı bırakır.
func forbiddenSendPath(localPath string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Güvenli-kapalı (fail-closed): ana dizin belirlenemiyorsa bu
		// fonksiyonun var olma amacı olan sınırlama uygulanamaz demektir —
		// "" (kısıtlama yok) dönmek her şeye izin vermek anlamına gelir.
		// Bunun yerine gönderimi tamamen reddet.
		return "kullanıcı ana dizini belirlenemedi, dosya gönderimi reddedildi"
	}
	if !pathWithinDir(localPath, home) {
		return "yalnızca kullanıcı ana dizini içindeki dosyalar gönderilebilir"
	}
	for _, sub := range sensitiveHomeSubdirs {
		if pathWithinDir(localPath, filepath.Join(home, sub)) {
			return fmt.Sprintf("%q dizini içindeki dosyalar gönderilemez", sub)
		}
	}
	return ""
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
	if reason := forbiddenSendPath(req.LocalPath); reason != "" {
		log.Printf("[AND] engellendi: dosya gönderimi reddedildi (%s): %s -> peer %s", reason, req.LocalPath, req.PeerID)
		writeBadRequest(w, "güvenlik kısıtlaması: "+reason)
		return
	}
	// Her gönderim bir iz bırakır — plugin API'sinin bu uç noktası hiçbir onay
	// diyaloğu göstermiyor (bkz. forbiddenSendPath'in doc yorumu). Standart
	// log'a yazılır: TUI açıkken AND bunu veri dizinindeki and.log'a yönlendirir
	// (bkz. network.RedirectLibraryLogs) — doğrudan stderr'e yazmak, o sırada
	// terminali kullanan tam ekran eklentinin ekranını bozardı.
	log.Printf("[AND] dosya gönderiliyor: %s -> peer %s", req.LocalPath, req.PeerID)
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
			// Kanal dolu (tüketici henüz önceki isteği işlemedi) — themeCh
			// yalnızca gönderme yönlü (chan<-) olduğundan burada eski
			// değeri atıp yenisini koyamayız. Sessizce 204 dönmek yerine
			// isteğin uygulanmadığını açıkça bildir; aksi halde çağıran
			// tema değişikliğinin başarılı olduğunu sanır.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(errResp{Error: "tema isteği düşürüldü, lütfen tekrar deneyin"}) //nolint:errcheck
			return
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

// getTheme returns the theme AND's main TUI currently has active, so
// plugins can match their own color scheme to it. The built-in default and
// the legacy-default migration both come from internal/ui — the single
// source of truth the TUI itself uses — so what plugins (and and-vscode)
// get always matches what the user actually sees.
func (s *Server) getTheme(w http.ResponseWriter, _ *http.Request) {
	if s.dataDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.dataDir, ThemeFile)); err == nil {
			var req ThemeReq
			if json.Unmarshal(data, &req) == nil {
				writeJSON(w, ThemeReq(ui.Normalize(ui.Theme(req))))
				return
			}
		}
	}
	writeJSON(w, ThemeReq(ui.DefaultTheme()))
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

// SitePages returns the CALLER's OWN personal-site pages (bkz. SiteBackend'in doc yorumu).
func (c *Client) SitePages() ([]SitePageInfo, error) {
	var v []SitePageInfo
	return v, c.get("/api/v1/site/pages", &v)
}

// PublishSitePage creates slug (if unused) or wholesale-replaces it (if it
// already belongs to the caller).
func (c *Client) PublishSitePage(slug, title, body string) error {
	return c.post("/api/v1/site/publish", PublishSitePageReq{Slug: slug, Title: title, Body: body})
}

func (c *Client) DeleteSitePage(slug string) error {
	return c.post("/api/v1/site/delete", DeleteSitePageReq{Slug: slug})
}

func (c *Client) SendChat(message string) error {
	return c.post("/api/v1/chat/send", SendChatReq{Message: message})
}

func (c *Client) PollChat() ([]ChatMsg, error) {
	var v []ChatMsg
	return v, c.get("/api/v1/chat/poll", &v)
}

// AllChat returns the full locally-known chat history — unlike PollChat
// (which only returns new messages as they arrive live), this is a
// point-in-time snapshot of everything persisted so far.
func (c *Client) AllChat() ([]ChatMsg, error) {
	var v []ChatMsg
	return v, c.get("/api/v1/chat/all", &v)
}

func (c *Client) SendDM(peerID, message string) error {
	return c.post("/api/v1/dm/send", SendDMReq{PeerID: peerID, Message: message})
}

// SendDMQueued, SendDM gibidir ama alıcıya şu an ulaşılamıyorsa mesajı giden
// kutusuna aldırır (bkz. SendDMReq.QueueIfOffline). Eski bir AND'da SendDM
// gibi davranır.
func (c *Client) SendDMQueued(peerID, message string) error {
	return c.post("/api/v1/dm/send", SendDMReq{PeerID: peerID, Message: message, QueueIfOffline: true})
}

// DMReadMarks, konuşma başına okundu zamanlarını (Peer ID → zaman) döner.
// Okundu bilgisi tutmayan eski bir AND'da IsNotFound(err) doğrudur.
func (c *Client) DMReadMarks() (map[string]time.Time, error) {
	var v map[string]time.Time
	return v, c.get("/api/v1/dm/read", &v)
}

// MarkDMRead, peerID ile konuşmanın at'a kadar okunduğunu bildirir.
func (c *Client) MarkDMRead(peerID string, at time.Time) error {
	return c.post("/api/v1/dm/read", DMReadReq{PeerID: peerID, At: at})
}

// APIError, AND API'sinin 200 dışı bir yanıtıdır. Status, eklentinin eski bir
// AND'da bulunmayan bir uç noktayı (404) tanıyabilmesi içindir.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("api: http %d", e.Status)
	}
	return "api: " + e.Message
}

// IsNotFound, err'in "bu uç nokta yok" (404) olup olmadığıdır.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// DMHistory, bu AND oturumunda görülen özel mesajları döner (bkz. DMRecord).
// Eski bir AND'da bu uç nokta yoksa hata döner; çağıran boş geçmişle devam
// edebilir.
func (c *Client) DMHistory() ([]DMRecord, error) {
	var v []DMRecord
	return v, c.get("/api/v1/dm/history", &v)
}

// DMHistoryFrom, DMHistory gibidir ama yalnızca kimliği id olan kaydı ve
// sonrakileri ister. Kayıt geçmişte yoksa — ya da AND bu süzmeyi
// bilmiyorsa — tüm geçmiş döner; çağıran, id'li kaydı yanıtta arayarak
// hangisi olduğunu anlar. id boşsa tüm geçmiş döner.
func (c *Client) DMHistoryFrom(id string) ([]DMRecord, error) {
	if id == "" {
		return c.DMHistory()
	}
	var v []DMRecord
	return v, c.get("/api/v1/dm/history?from="+url.QueryEscape(id), &v)
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

// AllBans lists every currently-active ban — requires kurucu/moderatör
// yetkisi (bkz. ModerationBackend).
func (c *Client) AllBans() ([]BanEntryInfo, error) {
	var v []BanEntryInfo
	return v, c.get("/api/v1/mod/bans", &v)
}

// Unban lifts an active ban on authorKey.
func (c *Client) Unban(authorKey string) error {
	return c.post("/api/v1/mod/unban", UnbanReq{AuthorKey: authorKey})
}

// ModHistory returns the local moderation audit log, newest first.
func (c *Client) ModHistory() ([]HistoryEntryInfo, error) {
	var v []HistoryEntryInfo
	return v, c.get("/api/v1/mod/history", &v)
}

// GrantedMods lists moderator certificates this node (as founder) has
// issued — yalnızca kurucu kimlik için anlamlı sonuç döner.
func (c *Client) GrantedMods() ([]GrantedModInfo, error) {
	var v []GrantedModInfo
	return v, c.get("/api/v1/mod/grants", &v)
}

// GrantModerator issues a new moderator certificate to targetKeyHex —
// yalnızca kurucu kimlik çağırabilir. duration is ignored when permanent
// is true.
func (c *Client) GrantModerator(targetKeyHex string, duration time.Duration, permanent bool) (certPath string, err error) {
	var resp GrantResp
	req := GrantReq{TargetKeyHex: targetKeyHex, DurationSec: int64(duration.Seconds()), Permanent: permanent}
	err = c.postJSON("/api/v1/mod/grant", req, &resp)
	return resp.CertPath, err
}

// RevokeModerator withdraws targetKeyHex's moderator authority — yalnızca
// kurucu kimlik çağırabilir.
func (c *Client) RevokeModerator(targetKeyHex string) error {
	return c.post("/api/v1/mod/revoke", RevokeReq{TargetKeyHex: targetKeyHex})
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
		return &APIError{Status: resp.StatusCode, Message: e.Error}
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
	return &APIError{Status: resp.StatusCode, Message: e.Error}
}

// IsForbidden, err'in AND'ın bir izin eksikliği yüzünden reddettiği bir
// istek olup olmadığını söyler (eklentiye o izin verilmemiş).
func IsForbidden(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusForbidden
}

// postJSON is like post but decodes a JSON response body on success (200
// OK) instead of expecting 204 No Content — for the few endpoints (ör.
// GrantModerator) that return data.
func (c *Client) postJSON(path string, body, out any) error {
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
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e errResp
		json.NewDecoder(resp.Body).Decode(&e) //nolint:errcheck
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
