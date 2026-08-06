package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lucian95511/and/internal/chatlog"
	"github.com/lucian95511/and/internal/dmmgr"
	"github.com/lucian95511/and/internal/filemgr"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/internal/updater"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "and:", err)
		os.Exit(1)
	}
}

func appDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config dir: %w", err)
	}
	dir := filepath.Join(base, "and")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create app dir: %w", err)
	}

	// Taşınabilir kurulum (ör. USB'den çalıştırma) için exe yanında/CWD'de
	// duran bir identity.dat'i AND'ın gerçek yapılandırma dizinine kopyalar.
	// Yalnızca AND_PORTABLE=1 açıkça ayarlandığında çalışır: varsayılan
	// olarak AÇIK olması, kullanıcının çalıştırdığı klasöre "identity.dat"
	// adlı bir dosya bırakan biri tarafından gerçek kimliğin sessizce ele
	// geçirilebilmesi (kimlik taklidi) anlamına gelirdi.
	if os.Getenv("AND_PORTABLE") == "1" {
		appDataID := filepath.Join(dir, "identity.dat")
		if _, err := os.Stat(appDataID); os.IsNotExist(err) {
			candidates := []string{}
			if exe, err2 := os.Executable(); err2 == nil {
				candidates = append(candidates, filepath.Join(filepath.Dir(exe), "identity.dat"))
			}
			if cwd, err2 := os.Getwd(); err2 == nil {
				candidates = append(candidates, filepath.Join(cwd, "identity.dat"))
			}
			for _, c := range candidates {
				if data, err2 := os.ReadFile(c); err2 == nil {
					_ = os.WriteFile(appDataID, data, 0o600)
					break
				}
			}
		}
	}

	return dir, nil
}

func deriveFounderPeerID(pubHex string) peer.ID {
	pid, err := moderation.AuthorKeyToPeerID(pubHex)
	if err != nil {
		return peer.ID("")
	}
	return pid
}

func loadExtraBootstrap(dataDir string) []peer.AddrInfo {
	path := filepath.Join(dataDir, "bootstrap.txt")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap: open:", err)
		return nil
	}
	defer f.Close()

	var peers []peer.AddrInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ma, err := multiaddr.NewMultiaddr(line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bootstrap: bad multiaddr:", line)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bootstrap: no peer ID in:", line)
			continue
		}
		peers = append(peers, *pi)
	}
	return peers
}

func run() error {
	dir, err := appDir()
	if err != nil {
		return err
	}
	identityFile := filepath.Join(dir, "identity.dat")

	id, err := tui.Login(identityFile, dir)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	myPubHex := hex.EncodeToString(id.PublicKey())
	isFounder, err := moderation.LoadFounderKey(dir, myPubHex)
	if err != nil {
		return fmt.Errorf("load founder key: %w", err)
	}
	if isFounder {
		fmt.Fprintln(os.Stderr, "[AND] Bu düğüm kurucu kimliğiyle çalışıyor.")
	}

	founderPeerID := deriveFounderPeerID(moderation.FounderPubKeyHex)

	mod, err := moderation.New(dir, founderPeerID)
	if err != nil {
		return fmt.Errorf("init moderation: %w", err)
	}

	node, err := network.New(id, mod)
	if err != nil {
		return fmt.Errorf("start network node: %w", err)
	}
	defer node.Close()

	extraBootstrap := loadExtraBootstrap(dir)

	discovery, err := network.StartDiscovery(ctx, node, extraBootstrap)
	if err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}
	defer discovery.Close()

	ps, err := network.NewPubSub(ctx, node.Host)
	if err != nil {
		return fmt.Errorf("start pubsub: %w", err)
	}

	forumTopic, err := network.JoinTopic(ps, node.Host, network.ForumTopic)
	if err != nil {
		return fmt.Errorf("join forum topic: %w", err)
	}
	defer forumTopic.Close()

	chatTopic, err := network.JoinTopic(ps, node.Host, network.ChatTopic)
	if err != nil {
		return fmt.Errorf("join chat topic: %w", err)
	}
	defer chatTopic.Close()

	modTopic, err := network.JoinTopic(ps, node.Host, moderation.ModerationTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "moderation topic:", err)
	} else {
		mod.Start(ctx, modTopic)
		defer modTopic.Close()
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			mod.PublishSavedBans(ctx, modTopic, dir)
		}()
	}

	forumStore, err := forum.New(id, forumTopic, filepath.Join(dir, "forum.db"))
	if err != nil {
		return fmt.Errorf("init forum: %w", err)
	}
	forumStore.SetModerator(mod)

	go forumStore.Run(ctx)

	forumStore.RegisterSync(node.Host)

	chatStore, err := chatlog.New(id, chatTopic, forumStore.DB(), dir)
	if err != nil {
		return fmt.Errorf("init chatlog: %w", err)
	}
	chatStore.SetModerator(mod)
	go chatStore.Run(ctx)
	chatStore.RegisterSync(node.Host)

	profileTopic, err := network.JoinTopic(ps, node.Host, profile.ProfileTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "profile topic:", err)
		profileTopic = nil
	} else {
		defer profileTopic.Close()
	}
	profileStore, err := profile.New(id, profileTopic, forumStore.DB(), mod)
	if err != nil {
		return fmt.Errorf("init profile: %w", err)
	}
	go profileStore.Run(ctx)

	// Kendi profilimizi (varsa) her başlangıçta bir kez yeniden yayınla —
	// profil güncellemelerinin forum konuları gibi bir sync protokolü yoktur,
	// bu yüzden yeni bağlanan bir peer'ın bunu görebilmesi ancak canlı bir
	// gossip yayınıyla mümkündür (bkz. publishSavedBans'teki aynı desen).
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		myKey := hex.EncodeToString(id.PublicKey())
		if existing := profileStore.Get(myKey); existing != nil {
			_ = profileStore.PublishOwn(ctx, profile.OwnUpdate{
				Bio: existing.Bio, Status: existing.Status, Tags: existing.Tags, Link: existing.Link,
				GitHubUser: existing.GitHubUser,
			})
		}
	}()

	go func() {
		synced := make(map[peer.ID]bool)
		for {
			select {
			case peerID := <-discovery.PeerConnectedCh():
				if synced[peerID] {
					continue
				}
				synced[peerID] = true
				go func(pid peer.ID) {
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					_ = forumStore.SyncWithPeer(ctx, node.Host, pid)
					_ = chatStore.SyncWithPeer(ctx, node.Host, pid)
				}(peerID)
			case <-ctx.Done():
				return
			}
		}
	}()

	dmBroker := dmmgr.New(node)
	dmBroker.SetModerator(mod)
	dosyalarDir := filepath.Join(dir, "dosyalar")
	_ = os.MkdirAll(dosyalarDir, 0o700)
	fileBroker := filemgr.New(node, dosyalarDir)
	fileBroker.SetModerator(mod)

	// Ayrı bir chat topic aboneliği oluştur: plugin API ile TUI aynı subscription'ı paylaşmaz.
	pluginChatTopic, err := network.JoinTopic(ps, node.Host, network.ChatTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin chat topic:", err)
		pluginChatTopic = nil
	} else {
		defer pluginChatTopic.Close()
	}

	idBackend := &identityAdapter{
		id:        id,
		node:      node,
		isFounder: isFounder,
		dir:       dir,
		mod:       mod,
	}
	fBackend := &forumAdapter{
		store:     forumStore,
		dir:       dir,
		mod:       mod,
		isFounder: isFounder,
		myPubHex:  myPubHex,
	}

	var chatBackend pluginapi.ChatBackend
	if pluginChatTopic != nil {
		chatBackend = newChatAdapter(ctx, pluginChatTopic, id.Name())
	}

	themeCh := make(chan pluginapi.ThemeReq, 1)
	apiSrv := pluginapi.NewServer(idBackend, fBackend, dmBroker, fileBroker, chatBackend, dir, themeCh)
	apiAddr, apiToken, err := apiSrv.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin API sunucusu başlatılamadı:", err)
		apiAddr = ""
		apiToken = ""
	}

	plugins := pluginmgr.DiscoverWithState(dir)

	updateCh := make(chan tui.UpdateReadyMsg, 1)
	restartCh := make(chan struct{}, 1)
	// AND_AUTO_UPDATE=0 ile kapatılabilir; varsayılan (tanımsız/başka değer) açıktır.
	if os.Getenv("AND_AUTO_UPDATE") != "0" {
		go autoUpdate(ctx, updateCh, restartCh)
	}

	if err := tui.Run(ctx, id, node, plugins, apiAddr, apiToken, forumStore, profileStore, dir, chatStore, mod, modTopic, updateCh, themeCh); err != nil {
		return err
	}

	select {
	case <-restartCh:
		fmt.Fprintln(os.Stderr, "Yeni sürüm yeniden başlatılıyor...")
		_ = updater.SelfRestart()
	default:
	}
	return nil
}

func autoUpdate(ctx context.Context, updateCh chan<- tui.UpdateReadyMsg, restartCh chan<- struct{}) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	checkCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := updater.Check(checkCtx)
	if err != nil || info == nil {
		return
	}
	applyCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel2()
	if err := updater.Apply(applyCtx, info); err != nil {
		fmt.Fprintln(os.Stderr, "updater:", err)
		return
	}
	select {
	case updateCh <- tui.UpdateReadyMsg{Version: info.TagName}:
	default:
	}
	select {
	case restartCh <- struct{}{}:
	default:
	}
}

type identityAdapter struct {
	id interface {
		Name() string
		PublicKey() ed25519.PublicKey
	}
	node      *network.Node
	isFounder bool
	dir       string
	mod       *moderation.Moderator
}

func (a *identityAdapter) Name() string      { return a.id.Name() }
func (a *identityAdapter) PubKeyHex() string { return hex.EncodeToString(a.id.PublicKey()) }
func (a *identityAdapter) PeerIDStr() string {
	if a.node == nil {
		return ""
	}
	return a.node.Host.ID().String()
}
func (a *identityAdapter) IsFounder() bool { return a.isFounder }

// IsModerator, FindModCert'in bulduğu sertifika dosyasını AYRICA
// mod.VerifyModeratorCert ile doğrular — FindModCert imza/süre kontrolü
// yapar ama iptal (revoke) durumundan habersizdir, bu ikinci geçiş
// olmadan iptal edilmiş bir moderatör Yönetici Paneli'nden konu silmeye
// devam edebilirdi.
func (a *identityAdapter) IsModerator() bool {
	if a.isFounder {
		return true
	}
	myPubHex := hex.EncodeToString(a.id.PublicKey())
	cert := moderation.FindModCert(a.dir, myPubHex)
	return cert != nil && a.mod != nil && a.mod.VerifyModeratorCert(*cert)
}
func (a *identityAdapter) ConnectedPeers() []pluginapi.PeerInfo {
	if a.node == nil {
		return nil
	}
	peers := a.node.Host.Network().Peers()
	out := make([]pluginapi.PeerInfo, 0, len(peers))
	for _, pid := range peers {
		info := a.node.Host.Peerstore().PeerInfo(pid)
		addrs := make([]string, len(info.Addrs))
		for i, ma := range info.Addrs {
			addrs[i] = ma.String()
		}
		out = append(out, pluginapi.PeerInfo{PeerID: pid.String(), Addrs: addrs})
	}
	return out
}

type forumAdapter struct {
	store     *forum.Forum
	dir       string
	mod       *moderation.Moderator
	isFounder bool
	myPubHex  string
}

// DeletePost resolves the local identity's moderation authority (founder,
// or a currently-valid — bkz. VerifyModeratorCert, iptal durumu dahil —
// moderator cert) and, if any, issues a real network-wide, tombstoned
// deletion via IssueModDelete. Kurucu/moderatör olmayan bir kimlik zaten
// buraya ulaşamaz (bkz. pluginapi.Server.enforceModeratorRole), ama bu
// ikinci geçiş olmadan iptal edilmiş bir moderatör hâlâ silme yapabilirdi
// (identityAdapter.IsModerator ile aynı savunma).
func (a *forumAdapter) DeletePost(ctx context.Context, postID, reason string) error {
	if a.isFounder {
		return a.store.IssueModDelete(ctx, postID, reason, true, nil)
	}
	cert := moderation.FindModCert(a.dir, a.myPubHex)
	if cert == nil || a.mod == nil || !a.mod.VerifyModeratorCert(*cert) {
		return fmt.Errorf("forum: moderatör yetkisi bulunamadı")
	}
	return a.store.IssueModDelete(ctx, postID, reason, false, cert)
}

func (a *forumAdapter) AllPosts() ([]pluginapi.PostSummary, error) {
	posts := a.store.AllInMemoryPosts()
	out := make([]pluginapi.PostSummary, 0, len(posts))
	for _, p := range posts {
		out = append(out, pluginapi.PostSummary{
			ID:         p.ID,
			Category:   p.Category,
			Title:      p.Title,
			AuthorName: p.AuthorName,
			AuthorKey:  p.AuthorKey,
		})
	}
	return out, nil
}

func (a *forumAdapter) CreatePost(ctx context.Context, category, title, body string) error {
	_, err := a.store.CreatePost(ctx, category, title, body)
	return err
}

func (a *forumAdapter) GetPost(id string) (*pluginapi.PostDetail, error) {
	p := a.store.PostByID(id)
	if p == nil {
		return nil, nil
	}
	replies := a.store.Replies(id)
	replyInfos := make([]pluginapi.ReplyInfo, len(replies))
	for i, r := range replies {
		replyInfos[i] = pluginapi.ReplyInfo{
			ID:         r.ID,
			AuthorName: r.AuthorName,
			AuthorKey:  r.AuthorKey,
			Body:       r.Body,
			CreatedAt:  r.CreatedAt,
		}
	}
	return &pluginapi.PostDetail{
		ID:         p.ID,
		Category:   p.Category,
		AuthorName: p.AuthorName,
		AuthorKey:  p.AuthorKey,
		Title:      p.Title,
		Body:       p.Body,
		CreatedAt:  p.CreatedAt,
		Replies:    replyInfos,
	}, nil
}

func (a *forumAdapter) CreateReply(ctx context.Context, postID, body string) error {
	_, err := a.store.CreateReply(ctx, postID, body)
	return err
}

// chatAdapter fans out chat topic messages to plugin subscribers.
type chatAdapter struct {
	ctx   context.Context
	topic interface {
		Publish(ctx context.Context, data []byte) error
		Messages(ctx context.Context) <-chan []byte
	}
	name string
	mu   sync.Mutex
	subs []chan pluginapi.ChatMsg
}

func newChatAdapter(ctx context.Context, topic interface {
	Publish(ctx context.Context, data []byte) error
	Messages(ctx context.Context) <-chan []byte
}, senderName string) *chatAdapter {
	a := &chatAdapter{ctx: ctx, topic: topic, name: senderName}
	go a.fanOut()
	return a
}

func (a *chatAdapter) fanOut() {
	ch := a.topic.Messages(a.ctx)
	for data := range ch {
		var pkt struct {
			N string    `json:"n"`
			T string    `json:"t"`
			A time.Time `json:"a"`
		}
		if json.Unmarshal(data, &pkt) != nil || pkt.T == "" {
			continue
		}
		msg := pluginapi.ChatMsg{From: pkt.N, Text: pkt.T, SentAt: pkt.A}
		a.mu.Lock()
		for _, ch := range a.subs {
			select {
			case ch <- msg:
			default:
			}
		}
		a.mu.Unlock()
	}
}

func (a *chatAdapter) Subscribe() chan pluginapi.ChatMsg {
	ch := make(chan pluginapi.ChatMsg, 32)
	a.mu.Lock()
	a.subs = append(a.subs, ch)
	a.mu.Unlock()
	return ch
}

func (a *chatAdapter) Unsubscribe(ch chan pluginapi.ChatMsg) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, c := range a.subs {
		if c == ch {
			a.subs = append(a.subs[:i], a.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (a *chatAdapter) SendChat(ctx context.Context, senderName, message string) error {
	pkt := struct {
		N string    `json:"n"`
		T string    `json:"t"`
		A time.Time `json:"a"`
	}{N: senderName, T: message, A: time.Now().UTC()}
	data, err := json.Marshal(pkt)
	if err != nil {
		return err
	}
	return a.topic.Publish(ctx, data)
}
