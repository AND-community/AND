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

	"github.com/lucian95511/and/internal/dmmgr"
	"github.com/lucian95511/and/internal/filemgr"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/internal/updater"

	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
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

	return dir, nil
}

func deriveFounderPeerID(pubHex string) peer.ID {
	raw, err := hex.DecodeString(pubHex)
	if err != nil || len(raw) != 32 {
		return peer.ID("")
	}
	lp2pPub, err := lp2pcrypto.UnmarshalEd25519PublicKey(raw)
	if err != nil {
		return peer.ID("")
	}
	pid, err := peer.IDFromPublicKey(lp2pPub)
	if err != nil {
		return peer.ID("")
	}
	return pid
}

func publishSavedBans(ctx context.Context, topic *network.Topic, dataDir string) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}

	bansDir := filepath.Join(dataDir, "bans")
	entries, err := os.ReadDir(bansDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(bansDir, e.Name()))
		if err != nil {
			continue
		}
		_ = topic.Publish(ctx, data)
	}
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

func buildApprovalFn(
	ctx context.Context,
	dir string,
	id interface {
		PublicKey() ed25519.PublicKey
		PrivateKey() ed25519.PrivateKey
	},
	isFounder bool,
	localApprove func(postID string),
	modTopic *network.Topic,
) func(postID string) error {
	if modTopic == nil {
		return nil
	}
	myPubHex := hex.EncodeToString(id.PublicKey())
	bansDir := filepath.Join(dir, "bans")

	makePublish := func(certPtr *moderation.ModeratorCert) func(string) error {
		privKey := id.PrivateKey()
		return func(postID string) error {
			now := time.Now().UTC()
			payload := fmt.Sprintf("approve|%s|%d", postID, now.Unix())
			sig := ed25519.Sign(privKey, []byte(payload))
			msg := moderation.ApprovalMsg{PostID: postID, IssuedAt: now, Cert: certPtr, Sig: hex.EncodeToString(sig)}
			envelope := moderation.Envelope{Type: "approve", Approve: &msg}
			data, err := json.Marshal(envelope)
			if err != nil {
				return err
			}
			if err := modTopic.Publish(ctx, data); err != nil {
				return err
			}
			localApprove(postID)
			_ = os.MkdirAll(bansDir, 0o700)
			short := postID
			if len(short) > 8 {
				short = short[:8]
			}
			fd, _ := json.MarshalIndent(envelope, "", "  ")
			_ = os.WriteFile(filepath.Join(bansDir, "approve_"+short+".json"), fd, 0o600)
			return nil
		}
	}

	if isFounder {
		return makePublish(nil)
	}
	if cert := moderation.FindModCert(dir, myPubHex); cert != nil {
		return makePublish(cert)
	}
	return nil
}

func run() error {
	dir, err := appDir()
	if err != nil {
		return err
	}
	identityFile := filepath.Join(dir, "identity.dat")

	id, err := tui.Login(identityFile)
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
		go publishSavedBans(ctx, modTopic, dir)
	}

	forumStore, err := forum.New(id, forumTopic, filepath.Join(dir, "forum.db"), mod)
	if err != nil {
		return fmt.Errorf("init forum: %w", err)
	}

	mod.SetOnApprove(forumStore.ApprovePost)
	mod.SetOnTrustedAuthor(forumStore.ApprovePostsByAuthor)

	go forumStore.Run(ctx)

	forumStore.RegisterSync(node.Host)

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
				}(peerID)
			case <-ctx.Done():
				return
			}
		}
	}()

	dmBroker := dmmgr.New(node)
	dosyalarDir := filepath.Join(dir, "dosyalar")
	_ = os.MkdirAll(dosyalarDir, 0o700)
	fileBroker := filemgr.New(node, dosyalarDir)

	// Ayrı bir chat topic aboneliği oluştur: plugin API ile TUI aynı subscription'ı paylaşmaz.
	pluginChatTopic, err := network.JoinTopic(ps, node.Host, network.ChatTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin chat topic:", err)
		pluginChatTopic = nil
	} else {
		defer pluginChatTopic.Close()
	}

	approvalFn := buildApprovalFn(ctx, dir, id, isFounder, forumStore.ApprovePost, modTopic)

	idBackend := &identityAdapter{
		id:        id,
		node:      node,
		isFounder: isFounder,
		dir:       dir,
	}
	fBackend := &forumAdapter{
		store:      forumStore,
		approvalFn: approvalFn,
	}

	var chatBackend pluginapi.ChatBackend
	if pluginChatTopic != nil {
		chatBackend = newChatAdapter(ctx, pluginChatTopic, id.Name())
	}

	apiSrv := pluginapi.NewServer(idBackend, fBackend, dmBroker, fileBroker, chatBackend, dir)
	apiAddr, apiToken, err := apiSrv.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin API sunucusu başlatılamadı:", err)
		apiAddr = ""
		apiToken = ""
	}

	plugins := pluginmgr.DiscoverWithState(dir)

	updateCh := make(chan tui.UpdateReadyMsg, 1)
	restartCh := make(chan struct{}, 1)
	go autoUpdate(ctx, updateCh, restartCh)

	if err := tui.Run(ctx, id, node, plugins, apiAddr, apiToken, approvalFn, forumStore, dir, chatTopic, updateCh); err != nil {
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
	id        interface {
		Name() string
		PublicKey() ed25519.PublicKey
	}
	node      *network.Node
	isFounder bool
	dir       string
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
func (a *identityAdapter) IsModerator() bool {
	if a.isFounder {
		return true
	}
	myPubHex := hex.EncodeToString(a.id.PublicKey())
	return moderation.FindModCert(a.dir, myPubHex) != nil
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
	store      *forum.Forum
	approvalFn func(string) error
}

func (a *forumAdapter) PendingPosts() ([]pluginapi.PendingPost, error) {
	posts, err := a.store.PendingPosts()
	if err != nil {
		return nil, err
	}
	out := make([]pluginapi.PendingPost, len(posts))
	for i, p := range posts {
		out[i] = pluginapi.PendingPost{
			ID:         p.ID,
			Title:      p.Title,
			AuthorName: p.AuthorName,
			AuthorKey:  p.AuthorKey,
			Category:   p.Category,
			Body:       p.Body,
			ExpiresAt:  p.ExpiresAt,
		}
	}
	return out, nil
}

func (a *forumAdapter) ApprovePost(postID string) error {
	if a.approvalFn == nil {
		return fmt.Errorf("onay yetkisi yok")
	}
	return a.approvalFn(postID)
}

func (a *forumAdapter) RejectPost(postID string) error {
	return a.store.RejectPost(postID)
}

func (a *forumAdapter) DeletePost(postID string) error {
	return a.store.DeletePost(postID)
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
			Approved:   p.Approved,
		})
	}
	return out, nil
}

func (a *forumAdapter) ApproveAuthor(authorKey string) error {
	a.store.ApprovePostsByAuthor(authorKey)
	return nil
}

func (a *forumAdapter) CreatePost(ctx context.Context, category, title, body string, permanentReq bool) error {
	_, err := a.store.CreatePost(ctx, category, title, body, permanentReq)
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
		Approved:   p.Approved,
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
