// Command and is the entry point for the AND client: unlock or create a
// local encrypted identity through the TUI login screen, bring up a
// libp2p node addressed by that identity and join the network, then hand
// off to the TUI's main app (menu/plugins/chat).
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
	"syscall"
	"time"

	"and/internal/forum"
	"and/internal/moderation"
	"and/internal/network"
	"and/internal/plugin"
	"and/internal/tui"

	adminplugin "and/Eklentiler/admin"
	modplugin   "and/Eklentiler/moderator"
	ozelchat    "and/Eklentiler/ozel_chat"

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
	// Standart konum: %APPDATA%\and — her yerden çalıştırıldığında aynı yer.
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config dir: %w", err)
	}
	dir := filepath.Join(base, "and")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create app dir: %w", err)
	}

	// Geçiş: identity.dat %APPDATA%\and'de yoksa exe'nin yanına veya proje
	// klasörüne bak; bulursa %APPDATA%\and'e kopyala (bir kez çalışır).
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


// deriveFounderPeerID converts a hex Ed25519 public key to its libp2p peer.ID.
// Returns empty peer.ID on failure (moderation still works, just no founder protection).
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

// publishSavedBans reads every .json file in <dataDir>/bans/ and publishes
// them on the moderation topic so peers that missed the original broadcast
// can still enforce them. Runs once after a short delay to let GossipSub mesh form.
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

// loadExtraBootstrap reads additional bootstrap multiaddrs from
// <dataDir>/bootstrap.txt — one multiaddr per line, # comments allowed.
// This lets operators point AND nodes at AND-specific bootstrap servers
// without rebuilding the binary.
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

// buildApprovalFn kurucu veya moderatör sertifikasına göre onay fonksiyonu oluşturur.
// Kullanıcı onaylayamıyorsa nil döner (forum TUI bunu nil kontrolüyle kullanır).
func buildApprovalFn(
	ctx context.Context,
	dir string,
	id interface {
		PublicKey() ed25519.PublicKey
		PrivateKey() ed25519.PrivateKey
	},
	isFounder bool,
	localApprove func(postID string),
	modTopic *network.Topic, // zaten açık olan moderation topic
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
		return makePublish(nil) // kurucu: Cert alanı nil
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

	// Login blocks until the user unlocks (or creates) their identity.
	id, err := tui.Login(identityFile)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Kurucu anahtarını yükle. Dosya yoksa bu kullanıcı otomatik kurucu olur.
	myPubHex := hex.EncodeToString(id.PublicKey())
	isFounder, err := moderation.LoadFounderKey(dir, myPubHex)
	if err != nil {
		return fmt.Errorf("load founder key: %w", err)
	}
	if isFounder {
		fmt.Fprintln(os.Stderr, "[AND] Bu düğüm kurucu kimliğiyle çalışıyor.")
	}

	// Kurucunun libp2p peer ID'sini public key'den türet (banlanamaz peer).
	founderPeerID := deriveFounderPeerID(moderation.FounderPubKeyHex)

	// Moderasyon sistemi: ConnectionGater olarak libp2p'ye verilir.
	mod, err := moderation.New(dir, founderPeerID)
	if err != nil {
		return fmt.Errorf("init moderation: %w", err)
	}

	node, err := network.New(id, mod)
	if err != nil {
		return fmt.Errorf("start network node: %w", err)
	}
	defer node.Close()

	// Özel bootstrap node'ları yükle ve discovery'e geçir.
	extraBootstrap := loadExtraBootstrap(dir)

	// StartDiscovery artık *Node alıyor: DHT'yi kurar, relay peer source'u
	// bağlar ve yeni peer bağlantılarını PeerConnectedCh() üzerinden bildirir.
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

	// Moderasyon topic'ini başlat.
	modTopic, err := network.JoinTopic(ps, node.Host, moderation.ModerationTopic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "moderation topic:", err)
	} else {
		mod.Start(ctx, modTopic)
		defer modTopic.Close()
		// bans/ klasöründeki önceden oluşturulmuş ban mesajlarını yayınla.
		go publishSavedBans(ctx, modTopic, dir)
	}

	// Forum oluşturma herkese açık; sadece okunabilir mod için
	// bootstrap.txt yanına "readonly" dosyası koy.
	forum.PostCreationEnabled = true

	forumStore, err := forum.New(id, forumTopic, filepath.Join(dir, "forum.db"), mod)
	if err != nil {
		return fmt.Errorf("init forum: %w", err)
	}

	// Moderasyon → forum callback'leri: onay ve güvenilir yazar bildirimleri
	mod.SetOnApprove(forumStore.ApprovePost)
	mod.SetOnTrustedAuthor(forumStore.ApprovePostsByAuthor)

	go forumStore.Run(ctx)

	// Forum sync protokolünü kaydet: gelen peer'lar forum geçmişimizi isteyebilir.
	forumStore.RegisterSync(node.Host)

	// Yeni peer bağlandığında forum geçmişini senkronize et.
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
					// Birkaç saniye bekle: peer'ın sync handler'ını kaydetmesi için zaman tanı.
					select {
					case <-ctx.Done():
						return
					case <-time.After(2 * time.Second):
					}
					_ = forumStore.SyncWithPeer(ctx, node.Host, pid)
				}(peerID)
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Plugin registry ────────────────────────────────────────────────────
	// Önbellekli JoinTopic: aynı topic birden fazla kez açılmaya çalışılırsa
	// mevcut handle döner; GossipSub "already exists" hatası görünmez.
	topicCache := map[string]*network.Topic{
		network.ForumTopic:        forumTopic,
		network.ChatTopic:         chatTopic,
		moderation.ModerationTopic: modTopic,
	}
	cachedJoinTopic := func(name string) (*network.Topic, error) {
		if t, ok := topicCache[name]; ok {
			return t, nil
		}
		t, err := network.JoinTopic(ps, node.Host, name)
		if err != nil {
			return nil, err
		}
		topicCache[name] = t
		return t, nil
	}

	env := plugin.Env{
		Ctx:       ctx,
		Identity:  id,
		Node:      node,
		DataDir:   dir,
		Routing:   discovery.Routing(),
		JoinTopic: cachedJoinTopic,
		PendingForumPosts: func() []plugin.PendingPost {
			posts, _ := forumStore.PendingPosts()
			out := make([]plugin.PendingPost, len(posts))
			for i, p := range posts {
				out[i] = plugin.PendingPost{
					ID: p.ID, Title: p.Title,
					AuthorName: p.AuthorName, Category: p.Category,
					ExpiresAt: p.ExpiresAt,
				}
			}
			return out
		},
		LocalApprovePost:   forumStore.ApprovePost,
		LocalApproveAuthor: forumStore.ApprovePostsByAuthor,
	}

	// Konu onay fonksiyonu: kurucu veya geçerli moderatör sertifikasıyla.
	// Zaten açık olan modTopic'i kullan — tekrar Join çağrısı hata verir.
	env.PublishApproval = buildApprovalFn(ctx, dir, id, isFounder, forumStore.ApprovePost, modTopic)

	reg := plugin.New(env)

	for _, p := range []plugin.Plugin{
		adminplugin.New(),
		modplugin.New(),
		ozelchat.New(),
	} {
		if err := reg.Register(p); err != nil {
			fmt.Fprintln(os.Stderr, p.Name(), "eklentisi başlatılamadı:", err)
		}
	}

	return tui.Run(ctx, id, node, reg, forumStore, dir, chatTopic)
}
