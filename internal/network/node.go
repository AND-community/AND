package network

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"

	libp2p "github.com/libp2p/go-libp2p"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"

	lp2pconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
)

// DefaultListenAddrs, port verilmediğinde dinlenen adreslerdir: TCP ve QUIC,
// IPv4 ve IPv6, işletim sisteminin seçtiği portlarda. AND düğümleri
// birbirine yalnızca bu iki taşıyıcıyla bağlanır; tarayıcılar için olan
// WebSocket ve WebTransport dinleyicileri (eskiden açıktı) duyurulan adres
// sayısını ikiye katlamaktan başka işe yaramıyordu.
var DefaultListenAddrs = []string{
	"/ip4/0.0.0.0/tcp/0",
	"/ip4/0.0.0.0/udp/0/quic-v1",
	"/ip6/::/tcp/0",
	"/ip6/::/udp/0/quic-v1",
}

const (
	connLow   = 20
	connHigh  = 40
	connGrace = 30 * time.Second

	// andEtiketi, bağlantı yöneticisinde AND eşlerine verilen etikettir.
	// Bağlı eşlerin çoğu genel DHT düğümüdür (DHT onlara 5 puan verir);
	// bağlantı sayısı sınırı aşıp budama yapılırken AND eşleri en sona kalır.
	andEtiketi     = "and-esi"
	andEtiketPuani = 100

	// bilinenKayitAraligi, bilinen eşler listesinin değiştiyse diske
	// yazılma aralığıdır (kapanışta ayrıca yazılır).
	bilinenKayitAraligi = 5 * time.Minute
)

type lazyRelaySource struct {
	mu sync.RWMutex
	fn func(context.Context, int) <-chan peer.AddrInfo
}

func (s *lazyRelaySource) source(ctx context.Context, num int) <-chan peer.AddrInfo {
	s.mu.RLock()
	fn := s.fn
	s.mu.RUnlock()
	if fn == nil {
		ch := make(chan peer.AddrInfo)
		close(ch)
		return ch
	}
	return fn(ctx, num)
}

// Secenekler, düğümün nasıl kurulacağıdır; sıfır değeri kullanılabilir.
type Secenekler struct {
	// Gater, yasaklı eşlerle bağlantıyı engeller (bkz. moderation); nil olabilir.
	Gater lp2pconnmgr.ConnectionGater
	// DinlemeAdresleri verilirse aynen kullanılır (testler). Verilmezse
	// VeriDizini'ndeki kalıcı porttan türetilir (bkz. dinlemePortu); o da
	// yoksa DefaultListenAddrs.
	DinlemeAdresleri []string
	// VeriDizini verilirse bilinen eşler (bilinen_esler.json) ve dinleme
	// portu (ag.json) orada saklanır.
	VeriDizini string
	// Surum, öteki düğümlere bildirilen AND sürümüdür ("AND/<Surum>").
	Surum string
	// Libp2p, sona eklenen libp2p seçenekleridir (testler: erişilebilirliği
	// zorlamak, sabit röle vermek).
	Libp2p []libp2p.Option

	// roleSecenekleri, röle aramasının zamanlamalarını değiştirir (paketin
	// kendi testleri: libp2p'nin yarım dakikalık bekleme sürelerini kısaltmak).
	roleSecenekleri []autorelay.Option
}

type Node struct {
	Host host.Host
	// Esler, şu an bağlı AND düğümlerini izler.
	Esler *EsIzleyici
	// Bilinen, daha önce bağlanılmış AND düğümlerinin kalıcı listesidir.
	Bilinen *BilinenEsler

	relaySource *lazyRelaySource
	durum       *durumIzleyici
	degisim     *degisimSiniri
	port        int
	kesif       atomic.Pointer[Discovery]
	kapat       context.CancelFunc
}

func (n *Node) SetRelaySource(fn func(context.Context, int) <-chan peer.AddrInfo) {
	n.relaySource.mu.Lock()
	n.relaySource.fn = fn
	n.relaySource.mu.Unlock()
}

// New, verilen adresleri (yoksa DefaultListenAddrs) dinleyen bir düğüm kurar;
// hiçbir şeyi diske yazmaz. AND'ın kendisi Yeni'yi kullanır.
func New(id *stdcrypto.Identity, gater lp2pconnmgr.ConnectionGater, listenAddrs ...string) (*Node, error) {
	return Yeni(id, Secenekler{Gater: gater, DinlemeAdresleri: listenAddrs})
}

// Yeni, ayarlara göre bir düğüm kurar.
func Yeni(id *stdcrypto.Identity, ayar Secenekler) (*Node, error) {
	priv, err := libp2pPrivateKey(id)
	if err != nil {
		return nil, err
	}

	dinleme, port := dinlemeSec(ayar)

	h, rs, err := hostKur(priv, ayar, dinleme)
	if err != nil && port > 0 {
		// Port denetimle açılış arasında başkasınca alınmış olabilir:
		// rastgele portlarla yeniden dene.
		log.Printf("ağ: %d portu dinlenemedi (%v); bu oturumda rastgele port kullanılıyor", port, err)
		port = 0
		h, rs, err = hostKur(priv, ayar, DefaultListenAddrs)
	}
	if err != nil {
		return nil, fmt.Errorf("network: create libp2p host: %w", err)
	}

	ctx, kapat := context.WithCancel(context.Background())
	n := &Node{Host: h, relaySource: rs, port: port, kapat: kapat, degisim: &degisimSiniri{}}
	bilinenYolu := ""
	if ayar.VeriDizini != "" {
		bilinenYolu = filepath.Join(ayar.VeriDizini, bilinenDosyaAdi)
	}
	n.Bilinen = BilinenAc(bilinenYolu)
	n.durum = yeniDurumIzleyici(ctx, h)
	n.Esler = yeniEsIzleyici(ctx, h)
	n.Esler.Dinle(n.esOlayi)
	h.SetStreamHandler(EsDegisimProtokolu, n.degisimIsle)
	go n.bilinenleriKaydet(ctx)
	return n, nil
}

// dinlemeSec, dinlenecek adresleri ve (kalıcı bir port kullanılıyorsa) portu
// seçer.
func dinlemeSec(ayar Secenekler) (adresler []string, port int) {
	if len(ayar.DinlemeAdresleri) > 0 {
		return ayar.DinlemeAdresleri, 0
	}
	if ayar.VeriDizini != "" {
		if port = dinlemePortu(ayar.VeriDizini); port > 0 {
			return portAdresleri(port), port
		}
	}
	return DefaultListenAddrs, 0
}

func hostKur(priv lp2pcrypto.PrivKey, ayar Secenekler, dinleme []string) (host.Host, *lazyRelaySource, error) {
	cm, err := connmgr.NewConnManager(connLow, connHigh,
		connmgr.WithGracePeriod(connGrace),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("network: create connection manager: %w", err)
	}

	rs := &lazyRelaySource{}
	surum := "AND"
	if ayar.Surum != "" {
		surum += "/" + ayar.Surum
	}

	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(dinleme...),
		libp2p.ConnectionManager(cm),
		libp2p.UserAgent(surum),
	}
	if ayar.Gater != nil {
		opts = append(opts, libp2p.ConnectionGater(ayar.Gater))
	}

	opts = append(opts,
		libp2p.NATPortMap(),
		libp2p.EnableNATService(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelayService(),
		// NAT arkasındaki düğüm, dışarıdan erişilebilmek için bir röleden yer
		// ayırtır; adaylar keşiften gelir (bkz. Discovery.roleAdaylari). Genel
		// röleler sık sık doludur: aday havuzu geniş tutulur ve yer vermeyen
		// röle bir süre yeniden denenmez.
		libp2p.EnableAutoRelayWithPeerSource(rs.source, append([]autorelay.Option{
			autorelay.WithMinCandidates(2),
			autorelay.WithNumRelays(2),
			autorelay.WithMaxCandidates(12),
			autorelay.WithBootDelay(10 * time.Second),
			autorelay.WithBackoff(15 * time.Minute),
		}, ayar.roleSecenekleri...)...),
	)
	opts = append(opts, ayar.Libp2p...)

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, nil, err
	}
	return h, rs, nil
}

// Port, düğümün dinlediği kalıcı porttur; port rastgele seçildiyse 0.
func (n *Node) Port() int { return n.port }

// AdresleriHatirla, p'ye bağlı değilken adresleri de bilinmiyorsa (libp2p
// kopan bir eşin adreslerini bir süre sonra unutur) bilinen eşler
// listesindeki adreslerini geçici olarak geri yükler; böylece p'ye akış
// açmak (ör. özel mesaj göndermek) onu yeniden çevirebilir. Adres
// yüklendiyse true döner.
func (n *Node) AdresleriHatirla(p peer.ID) bool {
	if n.Host.Network().Connectedness(p) != libp2pnet.NotConnected || len(n.Host.Peerstore().Addrs(p)) > 0 {
		return false
	}
	adresler := n.Bilinen.Adresler(p)
	if len(adresler) == 0 {
		return false
	}
	n.Host.Peerstore().AddAddrs(p, adresler, peerstore.TempAddrTTL)
	return true
}

// esOlayi, bağlı AND eşlerindeki değişiklikleri bilinen eşler listesine
// işler. İzleyicinin olay döngüsünde çağrılır; bloklamaz.
func (n *Node) esOlayi(o EsOlayi) {
	switch o.Tur {
	case EsBaglandi, EsGuncellendi:
		n.Bilinen.Gor(o.Es.ID, saklanacakAdresler(n.Host, o.Es.ID))
	case EsAyrildi:
		n.Bilinen.Goruldu(o.Es.ID)
	}
}

func (n *Node) bilinenleriKaydet(ctx context.Context) {
	t := time.NewTicker(bilinenKayitAraligi)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := n.Bilinen.Kaydet(); err != nil {
				log.Printf("ağ: bilinen eşler kaydedilemedi: %v", err)
			}
		}
	}
}

func (n *Node) Close() error {
	n.kapat()
	// Şu an bağlı AND eşleri "az önce görüldü" olarak yazılır: bir sonraki
	// açılışta ilk onlar denenir.
	for _, e := range n.Esler.Esler() {
		n.Bilinen.Goruldu(e.ID)
	}
	if err := n.Bilinen.Kaydet(); err != nil {
		log.Printf("ağ: bilinen eşler kaydedilemedi: %v", err)
	}
	return n.Host.Close()
}

func libp2pPrivateKey(id *stdcrypto.Identity) (lp2pcrypto.PrivKey, error) {
	priv, err := lp2pcrypto.UnmarshalEd25519PrivateKey(id.PrivateKey())
	if err != nil {
		return nil, fmt.Errorf("network: convert identity key: %w", err)
	}
	return priv, nil
}
