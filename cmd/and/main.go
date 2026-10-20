package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/dmmgr"
	"github.com/lucian95511/and/internal/filemgr"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/peersync"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/site"
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

// kutuphaneGunlugu, kütüphane günlüklerinin yazılacağı dosyayı ekleme
// kipinde açar; 5 MB'ı aşmışsa önce sıfırlar. Açılamazsa satırlar atılır —
// hiçbir durumda terminale gitmezler. Dosya süreç boyunca açık kalır.
func kutuphaneGunlugu(yol string) io.Writer {
	if st, err := os.Stat(yol); err == nil && st.Size() > 5<<20 {
		_ = os.Remove(yol)
	}
	f, err := os.OpenFile(yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return io.Discard
	}
	return f
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

	siteTopic, err := network.JoinTopic(ps, node.Host, site.SiteTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "site topic:", err)
		siteTopic = nil
	} else {
		defer siteTopic.Close()
	}
	siteStore, err := site.New(id, siteTopic, forumStore.DB())
	if err != nil {
		return fmt.Errorf("init site: %w", err)
	}
	go siteStore.Run(ctx)
	siteStore.RegisterSync(node.Host)

	// Geçmiş senkronizasyonu: yeni bağlanan, uzun aradan sonra yeniden
	// bağlanan ve periyodik olarak en uzun süredir senkronlanmamış AND
	// eşleriyle forum/sohbet/site geçmişi alışverişi (bkz. internal/peersync).
	// Yalnızca AND protokolünü konuşan eşlerle akış açılır; genel DHT
	// düğümleri atlanır.
	senkron := peersync.New(peersync.Config{
		Sync: func(ctx context.Context, pid peer.ID) error {
			ferr := forumStore.SyncWithPeer(ctx, node.Host, pid)
			cerr := chatStore.SyncWithPeer(ctx, node.Host, pid)
			serr := siteStore.SyncWithPeer(ctx, node.Host, pid)
			if ferr != nil && cerr != nil && serr != nil {
				return errors.Join(ferr, cerr, serr)
			}
			return nil
		},
		Connected: func() []peer.ID { return node.Host.Network().Peers() },
		Supports: func(pid peer.ID) peersync.Support {
			return peersync.ProtocolSupport(node.Host.Peerstore(), pid, tui.ANDSyncProtocols()...)
		},
	})
	go senkron.Run(ctx, discovery.PeerConnectedCh())
	tui.SyncStatus = senkron.Status

	dmBroker := dmmgr.New(node)
	dmBroker.SetModerator(mod)
	// DM geçmişi, giden kutusu ve okundu bilgisi kimlikten türetilen
	// anahtarla şifreli olarak dm_gecmis.bin'de saklanır; AND_DM_GECMISI=0
	// ile yalnızca bellekte tutulur (eski davranış).
	if os.Getenv("AND_DM_GECMISI") != "0" {
		if anahtar, err := dmmgr.GecmisAnahtari(id.PrivateKey()); err != nil {
			log.Printf("dm geçmişi: %v", err)
		} else if err := dmBroker.KaliciAc(filepath.Join(dir, "dm_gecmis.bin"), anahtar); err != nil {
			log.Printf("dm geçmişi: %v", err)
		}
	}
	tui.OkunmamisDM = dmBroker.OkunmamisSayisi
	dosyalarDir := filepath.Join(dir, "dosyalar")
	_ = os.MkdirAll(dosyalarDir, 0o700)
	fileBroker := filemgr.New(node, dosyalarDir)
	fileBroker.SetModerator(mod)

	// Ayrı bir chat topic aboneliği oluştur: plugin API ile TUI aynı
	// subscription'ı paylaşmaz. network.ChatTopic zaten yukarıda (chatTopic)
	// join edilmiş durumda — aynı adı ikinci kez JoinTopic ile join etmeye
	// çalışmak go-libp2p-pubsub'da her zaman "topic already exists" hatası
	// döner; bunun yerine aynı join edilmiş topic üzerinde BAĞIMSIZ bir
	// ikinci abonelik açıyoruz (bkz. Topic.NewSubscriber'ın doc yorumu).
	pluginChatTopic, err := chatTopic.NewSubscriber(node.Host)
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
	siteBackend := &siteAdapter{store: siteStore, id: id}
	mBackend := &modAdapter{
		mod:       mod,
		modTopic:  modTopic,
		chat:      chatStore,
		identity:  id,
		dir:       dir,
		isFounder: isFounder,
		myPubHex:  myPubHex,
	}

	var chatBackend pluginapi.ChatBackend
	if pluginChatTopic != nil {
		chatBackend = newChatAdapter(ctx, pluginChatTopic, chatStore, id.Name())
	}

	themeCh := make(chan pluginapi.ThemeReq, 1)
	apiSrv := pluginapi.NewServer(idBackend, fBackend, siteBackend, dmBroker, fileBroker, chatBackend, mBackend, dir, themeCh)
	// Start'ın döndürdüğü tam yetkili ana anahtar eklentilere verilmez: her
	// eklenti başlatılırken yalnızca kullanıcının onayladığı izinleri taşıyan
	// kendi anahtarını alır (bkz. tui.EklentiAnahtari).
	apiAddr, _, err := apiSrv.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin API sunucusu başlatılamadı:", err)
		apiAddr = ""
	} else {
		tui.EklentiAnahtari = apiSrv.EklentiAnahtari
	}

	plugins := pluginmgr.DiscoverWithState(dir)

	updateCh := make(chan tui.UpdateReadyMsg, 1)
	restartCh := make(chan struct{}, 1)
	// AND_AUTO_UPDATE=0 ile kapatılabilir; varsayılan (tanımsız/başka değer) açıktır.
	if os.Getenv("AND_AUTO_UPDATE") != "0" {
		go autoUpdate(ctx, updateCh, restartCh, eklentiHedefleri(plugins))
	}

	// Tam ekran TUI açıkken terminale yazılan her satır ekranı bozar:
	// kütüphane günlükleri (mDNS uyarıları, libp2p hataları) bu süre boyunca
	// veri dizinindeki and.log'a gider.
	network.RedirectLibraryLogs(kutuphaneGunlugu(filepath.Join(dir, "and.log")))

	if err := tui.Run(ctx, id, node, plugins, apiAddr, forumStore, profileStore, siteStore, dir, chatStore, mod, modTopic, updateCh, themeCh); err != nil {
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

// eklentiHedefleri, otomatik güncellemenin AND ile birlikte güncelleyeceği
// kurulu eklentilerdir.
func eklentiHedefleri(plugins []pluginmgr.Plugin) []updater.EklentiHedefi {
	out := make([]updater.EklentiHedefi, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, updater.EklentiHedefi{Ad: p.Name(), ExeYolu: p.ExePath})
	}
	return out
}

// autoUpdate, yeni bir sürüm varsa AND'ı ve aynı sürümde yayınlanan
// eklentileri günceller. Hatalar log'a yazılır: TUI açıkken log and.log'a
// gider, stderr'e yazmak tam ekran arayüzü bozardı.
func autoUpdate(ctx context.Context, updateCh chan<- tui.UpdateReadyMsg, restartCh chan<- struct{}, eklentiler []updater.EklentiHedefi) {
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
		log.Printf("updater: %v", err)
		return
	}
	// Eklentiler AND'la aynı sürümden güncellenir; böylece eklenti API'si ile
	// eklentiler birbirinden kopmaz. Bir eklentinin güncellenememesi AND'ın
	// güncellemesini geri almaz.
	guncellenen, hatalar := updater.ApplyPlugins(applyCtx, info, eklentiler)
	for _, e := range hatalar {
		log.Printf("updater: eklenti: %v", e)
	}
	select {
	case updateCh <- tui.UpdateReadyMsg{Version: info.TagName, Eklentiler: len(guncellenen)}:
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

// IsModerator reports whether this identity has founder or currently-valid
// (bkz. resolveOwnModAuthority'nin iptal kontrolü) moderator authority.
func (a *identityAdapter) IsModerator() bool {
	canAct, _ := resolveOwnModAuthority(a.dir, a.mod, a.isFounder, hex.EncodeToString(a.id.PublicKey()))
	return canAct
}
func (a *identityAdapter) ConnectedPeers() []pluginapi.PeerInfo {
	if a.node == nil {
		return nil
	}
	peers := a.node.Host.Network().Peers()
	out := make([]pluginapi.PeerInfo, 0, len(peers))
	ps, protos := a.node.Host.Peerstore(), tui.ANDSyncProtocols()
	for _, pid := range peers {
		info := ps.PeerInfo(pid)
		addrs := make([]string, len(info.Addrs))
		for i, ma := range info.Addrs {
			addrs[i] = ma.String()
		}
		out = append(out, pluginapi.PeerInfo{
			PeerID: pid.String(),
			Addrs:  addrs,
			AND:    peersync.ProtocolSupport(ps, pid, protos...) == peersync.SupportYes,
		})
	}
	return out
}

// resolveOwnModAuthority reports whether THIS node may act with founder or
// moderator authority (delete a post, ban/unban, grant/revoke another
// moderator) and, if the latter, the currently-valid (bkz.
// VerifyModeratorCert, iptal durumu dahil) cert to sign with. Shared by
// every adapter that needs to know "can I act as a moderator right now",
// so the FindModCert-existence-only mistake (fixed once already in
// identityAdapter.IsModerator) can't quietly reappear in a new call site.
func resolveOwnModAuthority(dir string, mod *moderation.Moderator, isFounder bool, myPubHex string) (canAct bool, cert *moderation.ModeratorCert) {
	return moderation.ResolveAuthority(dir, mod, isFounder, myPubHex)
}

type forumAdapter struct {
	store     *forum.Forum
	dir       string
	mod       *moderation.Moderator
	isFounder bool
	myPubHex  string
}

// DeletePost resolves the local identity's moderation authority and, if
// any, issues a real network-wide, tombstoned deletion via IssueModDelete.
// Kurucu/moderatör olmayan bir kimlik zaten buraya ulaşamaz (bkz.
// pluginapi.Server.enforceModeratorRole), ama resolveOwnModAuthority'nin
// iptal kontrolü olmadan iptal edilmiş bir moderatör hâlâ silme yapabilirdi.
func (a *forumAdapter) DeletePost(ctx context.Context, postID, reason string) error {
	canAct, cert := resolveOwnModAuthority(a.dir, a.mod, a.isFounder, a.myPubHex)
	if !canAct {
		return fmt.Errorf("forum: moderatör yetkisi bulunamadı")
	}
	return a.store.IssueModDelete(ctx, postID, reason, a.isFounder, cert)
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

// siteAdapter implements pluginapi.SiteBackend by calling into the real
// site.Store — mirrors modAdapter's "thin delegation" pattern (bkz. onun
// doc yorumu). Every method is implicitly scoped to a.id's own public
// key, exactly as site.Store.PublishPage/DeletePage/PagesByAuthor already
// are — there is no separate ownership check to add here.
type siteAdapter struct {
	store *site.Store
	id    *stdcrypto.Identity
}

func (a *siteAdapter) MyPages() ([]pluginapi.SitePageInfo, error) {
	myKey := hex.EncodeToString(a.id.PublicKey())
	pages := a.store.PagesByAuthor(myKey)
	out := make([]pluginapi.SitePageInfo, 0, len(pages))
	for _, p := range pages {
		out = append(out, pluginapi.SitePageInfo{
			Slug: p.Slug, Title: p.Title, Body: p.Body, UpdatedAt: p.UpdatedAt,
		})
	}
	return out, nil
}

func (a *siteAdapter) PublishPage(ctx context.Context, slug, title, body string) error {
	return a.store.PublishPage(ctx, slug, title, body)
}

func (a *siteAdapter) DeletePage(ctx context.Context, slug string) error {
	return a.store.DeletePage(ctx, slug)
}

// modAdapter implements pluginapi.ModerationBackend for the admin plugin's
// Banlar/Geçmiş/Moderatörler sekmeleri — bkz. Eklentiler/admin. Signing
// happens here (server-side, with the real private key); the plugin only
// ever sees structured requests/responses over the local HTTP API, never
// key material.
type modAdapter struct {
	mod       *moderation.Moderator
	modTopic  *network.Topic
	chat      *chatlog.Store
	identity  *stdcrypto.Identity
	dir       string
	isFounder bool
	myPubHex  string
}

func (a *modAdapter) AllBans() ([]pluginapi.BanEntryInfo, error) {
	entries := a.mod.AllBans()
	founderHex := moderation.FounderPubKeyHex
	out := make([]pluginapi.BanEntryInfo, 0, len(entries))
	for _, e := range entries {
		info := pluginapi.BanEntryInfo{
			PeerID: e.PeerID, AuthorKey: e.AuthorKey,
			ExpiresAt: e.ExpiresAt, Permanent: e.Permanent,
			ModeratorKey: e.ModeratorKey, Reason: e.Reason,
			IsFounderMod: strings.EqualFold(e.ModeratorKey, founderHex),
		}
		if a.chat != nil && e.AuthorKey != "" {
			if msgs := a.chat.ByAuthor(e.AuthorKey, 1); len(msgs) > 0 {
				info.DisplayName = msgs[0].AuthorName
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func (a *modAdapter) Unban(ctx context.Context, authorKey string) error {
	canAct, cert := resolveOwnModAuthority(a.dir, a.mod, a.isFounder, a.myPubHex)
	if !canAct {
		return fmt.Errorf("moderasyon: yetkiniz yok")
	}
	return moderation.IssueUnbanFor(ctx, a.mod, a.modTopic, a.identity.PrivateKey(), authorKey, a.isFounder, cert)
}

func (a *modAdapter) History() ([]pluginapi.HistoryEntryInfo, error) {
	entries := a.mod.History()
	founderHex := moderation.FounderPubKeyHex
	out := make([]pluginapi.HistoryEntryInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, pluginapi.HistoryEntryInfo{
			Action: string(e.Action), PeerID: e.PeerID, Reason: e.Reason,
			ModeratorKey: e.ModeratorKey, IsFounderMod: strings.EqualFold(e.ModeratorKey, founderHex),
			At: e.At, ExpiresAt: e.ExpiresAt, Permanent: e.Permanent,
		})
	}
	return out, nil
}

func (a *modAdapter) GrantedMods() ([]pluginapi.GrantedModInfo, error) {
	grants := a.mod.GrantedMods()
	out := make([]pluginapi.GrantedModInfo, 0, len(grants))
	for _, g := range grants {
		out = append(out, pluginapi.GrantedModInfo{
			Key: g.Key, GrantedAt: g.GrantedAt, ExpiresAt: g.ExpiresAt,
			Permanent: g.Permanent, Revoked: g.Revoked, RevokedAt: g.RevokedAt,
		})
	}
	return out, nil
}

func (a *modAdapter) Grant(ctx context.Context, targetKeyHex string, duration time.Duration, permanent bool) (string, error) {
	if !a.isFounder {
		return "", fmt.Errorf("moderasyon: yalnızca kurucu moderatör atayabilir")
	}
	var expires time.Time
	if !permanent {
		expires = time.Now().UTC().Add(duration)
	}
	_, certPath, err := a.mod.IssueGrant(a.identity.PrivateKey(), targetKeyHex, expires, permanent)
	return certPath, err
}

func (a *modAdapter) Revoke(ctx context.Context, targetKeyHex string) error {
	if !a.isFounder {
		return fmt.Errorf("moderasyon: yalnızca kurucu moderatör yetkisini iptal edebilir")
	}
	rev := moderation.SignRevoke(a.identity.PrivateKey(), targetKeyHex)
	return a.mod.IssueRevoke(ctx, a.modTopic, rev)
}

// chatAdapter fans out chat topic messages to plugin subscribers. It fans
// out over its own gossipsub subscription (a separate one from the TUI's,
// see the call site's comment) but delegates AllChat to the TUI's real
// chatlog.Store, which is the only component that actually persists chat
// history to disk — the adapter's own live fan-out has no memory of
// anything before this process started.
type chatAdapter struct {
	ctx   context.Context
	topic interface {
		Publish(ctx context.Context, data []byte) error
		Messages(ctx context.Context) <-chan []byte
	}
	store *chatlog.Store
	name  string
	mu    sync.Mutex
	subs  []chan pluginapi.ChatMsg
}

func newChatAdapter(ctx context.Context, topic interface {
	Publish(ctx context.Context, data []byte) error
	Messages(ctx context.Context) <-chan []byte
}, store *chatlog.Store, senderName string) *chatAdapter {
	a := &chatAdapter{ctx: ctx, topic: topic, store: store, name: senderName}
	go a.fanOut()
	return a
}

func (a *chatAdapter) AllChat() []pluginapi.ChatMsg {
	if a.store == nil {
		return nil
	}
	all := a.store.All()
	out := make([]pluginapi.ChatMsg, len(all))
	for i, m := range all {
		out[i] = pluginapi.ChatMsg{ID: m.ID, From: m.AuthorName, Text: m.Text, SentAt: m.SentAt}
	}
	return out
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
