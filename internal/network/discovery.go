package network

// discovery.go — AND düğümlerinin birbirini bulması.
//
// Bir düğüm öteki AND düğümlerine dört yoldan ulaşır:
//
//  1. Bilinen eşler: daha önce bağlandığı AND düğümlerine açılır açılmaz
//     doğrudan bağlanmayı dener (bkz. bilinen.go).
//  2. Yerel ağ: aynı ağdaki düğümler mDNS ile bulunur.
//  3. Buluşma noktası: her düğüm genel DHT'de AND'ın buluşma anahtarına
//     kendini duyurur ve orada ötekileri arar.
//  4. Eş değişimi: bağlanılan her AND düğümü, bağlı olduğu ötekileri söyler
//     (bkz. degisim.go).
//
// Duyuru ve arama birbirinden bağımsız döngülerdir. Eskiden tek döngüydü:
// arama, duyuru bitmeden başlamıyor; açılışta DHT daha boşken yapılan ilk
// duyuru anında başarısız olduğu için ilk arama en erken bir dakika sonra
// yapılıyor; sonra da her şey 30 saniyede bir, ağ ne durumda olursa olsun
// yineleniyordu. Artık arama DHT hazır olur olmaz başlar, aralığı bağlı AND
// düğümü sayısına göre açılır; duyuru ise yalnızca adresler değiştiğinde ve
// seyrek aralıklarla yenilenir.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	golog "github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/host/eventbus"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/lucian95511/and/protokol"
)

const Rendezvous = protokol.Rendezvous

const mdnsServiceTag = protokol.MDNSEtiketi

const (
	bootstrapConnectTimeout   = 5 * time.Second
	rebootstrapInterval       = 5 * time.Minute
	minPeersBeforeRebootstrap = 4

	// hedefANDEs, bağlı olunmak istenen AND düğümü sayısıdır: altındayken
	// keşif ve eş değişimi sık, üstündeyken seyrek çalışır.
	hedefANDEs = 8
	// ustANDEs, bu kadar AND düğümüne bağlıyken keşfin yeni bulduğu
	// düğümlere artık bağlanmadığı sayıdır (büyük bir ağda herkes herkese
	// bağlanmaya çalışmasın; iletiler zaten gossip ile yayılır).
	ustANDEs = 16
	// esZamanliCevirme, keşfin aynı anda çevirdiği en fazla eştir.
	esZamanliCevirme = 8
	// bilinenIlkTur, açılışta bilinen eşlerden kaçının çevrildiğidir.
	bilinenIlkTur = 16
	// bilinenTurBoyu, hiç AND eşi yokken her arama turunda bilinen eşlerden
	// kaçının yeniden denendiğidir.
	bilinenTurBoyu = 8

	// roleProtokolu, bir düğümün başkaları için röle olabildiğini gösterir.
	roleProtokolu = protocol.ID("/libp2p/circuit/relay/0.2.0/hop")
)

// kesifSureleri, keşfin zamanlamalarıdır (testler kısaltır).
type kesifSureleri struct {
	// aramaZamanAsimi, buluşma noktasındaki tek bir aramanın en uzun süresidir.
	aramaZamanAsimi time.Duration
	// Hiç AND eşi yokken arama aralığı aramaEnKisa'dan başlar ve her
	// sonuçsuz turda ikiye katlanır (en çok aramaEnUzun).
	aramaEnKisa, aramaEnUzun time.Duration
	// aramaAzEs: hedefin altında AND eşi varken; aramaYeterli: hedefe ulaşılmışken.
	aramaAzEs, aramaYeterli time.Duration

	// duyuruAraligi, buluşma noktasındaki kaydın yenilenme aralığıdır.
	duyuruAraligi time.Duration
	// Adresler değişince yeniden duyurulur: değişiklikler birikebilsin diye
	// duyuruGeciktirme kadar beklenir ve iki duyuru arasında en az duyuruEnAz olur.
	duyuruGeciktirme, duyuruEnAz time.Duration
	// Başarısız duyuru duyuruHataEnKisa'dan başlayıp katlanarak yeniden denenir.
	duyuruHataEnKisa, duyuruHataEnUzun time.Duration

	// dhtBekleme, açılışta DHT'nin ilk eşlerine bağlanması için en çok
	// beklenen süredir.
	dhtBekleme time.Duration
	// baglanma, bulunan bir eşe bağlanma denemesinin en uzun süresidir.
	baglanma time.Duration
	// adresCozme, adresi bilinmeyen bir eşin adresinin DHT'de aranma süresidir.
	adresCozme time.Duration

	// Eş değişimi: yeni bağlanan AND eşine degisimIlk sonra sorulur; hedefin
	// altındayken degisimAraligi'nda, hedefe ulaşılmışken degisimYeterli'de
	// bir, rastgele bir eşe sorulur.
	degisimIlk, degisimAraligi, degisimYeterli time.Duration
}

var varsayilanSureler = kesifSureleri{
	aramaZamanAsimi:  60 * time.Second,
	aramaEnKisa:      30 * time.Second,
	aramaEnUzun:      5 * time.Minute,
	aramaAzEs:        5 * time.Minute,
	aramaYeterli:     15 * time.Minute,
	duyuruAraligi:    15 * time.Minute,
	duyuruGeciktirme: 20 * time.Second,
	duyuruEnAz:       time.Minute,
	duyuruHataEnKisa: 30 * time.Second,
	duyuruHataEnUzun: 5 * time.Minute,
	dhtBekleme:       30 * time.Second,
	baglanma:         20 * time.Second,
	adresCozme:       15 * time.Second,
	degisimIlk:       2 * time.Second,
	degisimAraligi:   3 * time.Minute,
	degisimYeterli:   20 * time.Minute,
}

// yonlendirici, buluşma noktasına duyuru ve orada aramadır
// (drouting.RoutingDiscovery; testlerde sahtesi).
type yonlendirici interface {
	Advertise(ctx context.Context, ns string, opts ...discovery.Option) (time.Duration, error)
	FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error)
}

type Discovery struct {
	node           *Node
	host           host.Host
	dht            *dual.DHT
	routing        *drouting.RoutingDiscovery
	mdns           mdns.Service
	eventSub       event.Subscription
	peerConnected  chan peer.ID
	extraBootstrap []peer.AddrInfo

	yon  yonlendirici
	sure kesifSureleri
	// dhtHazir, DHT'nin en az bir eş tanıdığıdır; nil ise beklenmez.
	dhtHazir func() bool
	// dhtEsleri, genel DHT'nin yönlendirme tablosundaki düğüm sayısıdır.
	dhtEsleri func() int
	// adresCoz, adresi bilinmeyen bir eşin adreslerini arar; nil olabilir.
	adresCoz func(ctx context.Context, p peer.ID) (peer.AddrInfo, error)

	// uyan, bağlantı geri geldiğinde ya da son AND eşi ayrıldığında arama
	// döngüsünü beklemeden çalıştırır (bkz. kesfiUyandir).
	uyan chan struct{}
	// adresDegisti, düğümün adresleri değiştiğinde duyuru döngüsünü dürter.
	adresDegisti chan struct{}
	// degisimIste, yeni bağlanan AND eşlerini eş değişimi döngüsüne iletir.
	degisimIste chan peer.ID
	// cevirSem, keşfin eşzamanlı bağlanma denemelerini sınırlar.
	cevirSem chan struct{}

	mu      sync.Mutex
	ist     KesifDurumu
	sonIlan string // son başarılı duyurudaki adreslerin parmak izi
}

func (d *Discovery) PeerConnectedCh() <-chan peer.ID {
	return d.peerConnected
}

func (d *Discovery) Routing() *drouting.RoutingDiscovery {
	return d.routing
}

// yeniKesif, döngüleri henüz başlatılmamış bir keşif kurar.
func yeniKesif(node *Node, yon yonlendirici, sure kesifSureleri) *Discovery {
	return &Discovery{
		node:          node,
		host:          node.Host,
		yon:           yon,
		sure:          sure,
		peerConnected: make(chan peer.ID, 64),
		uyan:          make(chan struct{}, 1),
		adresDegisti:  make(chan struct{}, 1),
		degisimIste:   make(chan peer.ID, 32),
		cevirSem:      make(chan struct{}, esZamanliCevirme),
	}
}

func StartDiscovery(ctx context.Context, node *Node, extraBootstrap []peer.AddrInfo) (*Discovery, error) {
	h := node.Host

	kdht, err := dual.New(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("network: start DHT: %w", err)
	}

	connectToBootstrapPeers(ctx, h, extraBootstrap)

	if err := kdht.Bootstrap(ctx); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("network: bootstrap DHT: %w", err)
	}

	routing := drouting.NewRoutingDiscovery(kdht)

	d := yeniKesif(node, routing, varsayilanSureler)
	d.dht, d.routing, d.extraBootstrap = kdht, routing, extraBootstrap
	d.dhtHazir = func() bool { return kdht.WANActive() || kdht.LAN.RoutingTable().Size() > 0 }
	d.dhtEsleri = func() int { return kdht.WAN.RoutingTable().Size() }
	d.adresCoz = kdht.FindPeer

	node.SetRelaySource(d.roleAdaylari)

	muteNoisyMDNSLog()
	d.mdns = mdns.NewMdnsService(h, mdnsServiceTag, peerFoundNotifee{ctx: ctx, kesif: d})
	if err := d.mdns.Start(); err != nil {
		kdht.Close()
		return nil, fmt.Errorf("network: start mDNS: %w", err)
	}

	sub, err := h.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err == nil {
		d.eventSub = sub
		go d.peerEventLoop(ctx, sub)
	}

	d.baslat(ctx)
	go d.rebootstrapLoop(ctx)

	return d, nil
}

// baslat, keşfin döngülerini başlatır ve kendini düğüme tanıtır.
func (d *Discovery) baslat(ctx context.Context) {
	d.node.kesif.Store(d)
	d.node.Esler.Dinle(d.esOlayi)
	if sub, err := d.host.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated), eventbus.Name("and-kesif")); err == nil {
		go d.adresOlaylari(ctx, sub)
	}
	// Bilinen eşler DHT beklenmeden, hemen çevrilir.
	go d.bilinenlereBaglan(ctx, bilinenIlkTur)
	go d.duyuruDongusu(ctx)
	go d.aramaDongusu(ctx)
	go d.degisimDongusu(ctx)
}

func (d *Discovery) peerEventLoop(ctx context.Context, sub event.Subscription) {
	defer sub.Close()
	for {
		select {
		case evt, ok := <-sub.Out():
			if !ok {
				return
			}
			e, ok := evt.(event.EvtPeerConnectednessChanged)
			if ok && e.Connectedness == libp2pnet.Connected {
				select {
				case d.peerConnected <- e.Peer:
				default:
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) adresOlaylari(ctx context.Context, sub event.Subscription) {
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-sub.Out():
			if !ok {
				return
			}
			select {
			case d.adresDegisti <- struct{}{}:
			default:
			}
		}
	}
}

// esOlayi, bağlı AND eşlerindeki değişikliklere tepki verir. İzleyicinin
// olay döngüsünde çağrılır; bloklamaz.
func (d *Discovery) esOlayi(o EsOlayi) {
	switch o.Tur {
	case EsBaglandi:
		select {
		case d.degisimIste <- o.Es.ID:
		default:
		}
	case EsAyrildi:
		// Son AND eşi de gitti: bir sonraki aramayı bekleme.
		if d.node.Esler.Say() == 0 {
			d.kesfiUyandir()
		}
	}
}

// ─── Buluşma noktası: duyuru ────────────────────────────────────────────────

// duyuruDongusu, düğümü buluşma noktasına duyurur ve kaydı yeniler. Kayıt
// düğümün o anki adreslerini taşır: adresler değişince (ör. bir röleden yer
// ayırtılınca) beklemeden yenilenir.
func (d *Discovery) duyuruDongusu(ctx context.Context) {
	d.dhtBekle(ctx)
	hata := 0
	for ctx.Err() == nil {
		parmakIzi := ilanParmakIzi(d.host.Addrs())
		_, err := d.yon.Advertise(ctx, Rendezvous)
		if ctx.Err() != nil {
			return
		}
		simdi := time.Now()
		d.mu.Lock()
		if err != nil {
			d.ist.DuyuruHatasi = err.Error()
		} else {
			d.ist.SonDuyuru, d.ist.DuyuruHatasi, d.sonIlan = simdi, "", parmakIzi
		}
		d.mu.Unlock()

		bekle := d.sure.duyuruAraligi
		if err != nil {
			hata++
			bekle = min(d.sure.duyuruHataEnKisa<<min(hata-1, 6), d.sure.duyuruHataEnUzun)
		} else {
			hata = 0
		}
		if !d.duyuruyuBekle(ctx, bekle, simdi) {
			return
		}
	}
}

// duyuruyuBekle, bir sonraki duyuruya kadar bekler: en çok bekle kadar; daha
// önce duyurulan adresler değiştiyse daha erken. ctx bittiyse false döner.
func (d *Discovery) duyuruyuBekle(ctx context.Context, bekle time.Duration, sonDeneme time.Time) bool {
	bitis := time.Now().Add(bekle)
	zaman := time.NewTimer(bekle)
	defer zaman.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-zaman.C:
			return true
		case <-d.adresDegisti:
			d.mu.Lock()
			ayni := ilanParmakIzi(d.host.Addrs()) == d.sonIlan
			d.mu.Unlock()
			if ayni {
				continue
			}
			erken := max(d.sure.duyuruGeciktirme, d.sure.duyuruEnAz-time.Since(sonDeneme))
			if yeni := time.Now().Add(erken); yeni.Before(bitis) {
				bitis = yeni
				zaman.Reset(erken)
			}
		}
	}
}

// ilanParmakIzi, buluşma kaydına giren adreslerin (internetten erişilebilir
// doğrudan adresler ve röle adresleri) sıralı özetidir. Yalnızca yerel
// adresler değiştiğinde (ör. sanal bir ağ arayüzü açılınca) kaydı yenilemeye
// gerek yoktur: genel DHT onları zaten taşımaz.
func ilanParmakIzi(adresler []ma.Multiaddr) string {
	var secilen []string
	for _, a := range adresler {
		if tur := adresinTuru(a); tur == adresAcik || tur == adresRole {
			secilen = append(secilen, a.String())
		}
	}
	sort.Strings(secilen)
	return strings.Join(secilen, "\n")
}

// dhtBekle, DHT en az bir eş tanıyana kadar (en çok dhtBekleme) bekler.
// Açılışta DHT'nin yönlendirme tablosu boştur; o anda yapılan arama da
// duyuru da hiçbir yere gitmeden biter.
func (d *Discovery) dhtBekle(ctx context.Context) {
	if d.dhtHazir == nil {
		return
	}
	bitis := time.NewTimer(d.sure.dhtBekleme)
	defer bitis.Stop()
	tik := time.NewTicker(200 * time.Millisecond)
	defer tik.Stop()
	for !d.dhtHazir() {
		select {
		case <-ctx.Done():
			return
		case <-bitis.C:
			return
		case <-tik.C:
		}
	}
}

// ─── Buluşma noktası: arama ─────────────────────────────────────────────────

// aramaDongusu, buluşma noktasında AND düğümlerini arar. Aralık, bağlı AND
// düğümü sayısına göre seçilir (bkz. aramaAraligi).
func (d *Discovery) aramaDongusu(ctx context.Context) {
	d.dhtBekle(ctx)
	bos := 0 // hiç AND eşi yokken üst üste biten tur sayısı
	for ctx.Err() == nil {
		// Hiç AND eşi yokken (ilk tur dışında: açılışta zaten çevrildiler)
		// bilinen eşler de yeniden denenir.
		if bos > 0 && d.node.Esler.Say() == 0 {
			go d.bilinenlereBaglan(ctx, bilinenTurBoyu)
		}
		d.ara(ctx)
		and := d.node.Esler.Say()
		if and == 0 {
			bos++
		} else {
			bos = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(aramaAraligi(d.sure, and, bos)):
		case <-d.uyan:
			bos = 0
		}
	}
}

// aramaAraligi, bir sonraki aramaya kadar beklenecek süredir.
//
// Hiç AND eşi yokken sık aranır; ama yalnız bir düğüm (küçük bir toplulukta
// çoğu zaman böyledir) DHT'yi sonsuza dek 30 saniyede bir yoklamasın diye
// aralık her sonuçsuz turda ikiye katlanır. Yeni açılan bir düğüm ilk
// aramasında yalnız düğümün kaydını bulup ona bağlanır; yalnız düğümün sık
// araması gerekmez.
func aramaAraligi(s kesifSureleri, andEs, bosTur int) time.Duration {
	switch {
	case andEs >= hedefANDEs:
		return s.aramaYeterli
	case andEs > 0:
		return s.aramaAzEs
	}
	return min(s.aramaEnKisa<<min(max(bosTur-1, 0), 8), s.aramaEnUzun)
}

// ara, buluşma noktasında bir kez arar ve bulunan düğümlere bağlanmayı
// başlatır; bulunan (kendimiz dışındaki) düğüm sayısını döndürür.
func (d *Discovery) ara(ctx context.Context) int {
	actx, iptal := context.WithTimeout(ctx, d.sure.aramaZamanAsimi)
	defer iptal()
	bulunan := 0
	if peers, err := d.yon.FindPeers(actx, Rendezvous); err == nil {
		for pi := range peers {
			if pi.ID == d.host.ID() {
				continue
			}
			bulunan++
			if d.node.Esler.Say() < ustANDEs {
				d.arkadaBaglan(ctx, pi)
			}
		}
	}
	if ctx.Err() != nil {
		return bulunan
	}
	d.mu.Lock()
	d.ist.SonArama, d.ist.SonBulunan = time.Now(), bulunan
	d.mu.Unlock()
	return bulunan
}

// kesfiUyandir, arama döngüsünü (bekliyorsa) hemen çalıştırır.
func (d *Discovery) kesfiUyandir() {
	select {
	case d.uyan <- struct{}{}:
	default:
	}
}

// ─── Bağlanma ───────────────────────────────────────────────────────────────

func (d *Discovery) arkadaBaglan(ctx context.Context, pi peer.AddrInfo) {
	go func() { _ = d.baglan(ctx, pi) }()
}

// baglan, pi'ye bağlı değilse bağlanmayı dener. Aynı anda en fazla
// esZamanliCevirme deneme sürer; her denemenin süresi sınırlıdır.
func (d *Discovery) baglan(ctx context.Context, pi peer.AddrInfo) error {
	h := d.host
	if pi.ID == h.ID() || d.bagli(pi) {
		return nil
	}
	select {
	case d.cevirSem <- struct{}{}:
		defer func() { <-d.cevirSem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if d.bagli(pi) {
		return nil
	}
	// Buluşma kaydı adres taşımayabilir (kaydı tutan DHT düğümü adresleri
	// unutmuş olabilir). Eskiden böyle bir eşe hiç bağlanılamıyordu.
	if len(pi.Addrs) == 0 && len(h.Peerstore().Addrs(pi.ID)) == 0 {
		if bilinen := d.node.Bilinen.Adresler(pi.ID); len(bilinen) > 0 {
			pi.Addrs = bilinen
		} else if d.adresCoz != nil {
			cctx, iptal := context.WithTimeout(ctx, d.sure.adresCozme)
			cozulen, err := d.adresCoz(cctx, pi.ID)
			iptal()
			if err != nil {
				return err
			}
			pi.Addrs = cozulen.Addrs
		}
	}
	cctx, iptal := context.WithTimeout(ctx, d.sure.baglanma)
	defer iptal()
	return h.Connect(cctx, pi)
}

// bagli, pi için yeni bir bağlanma denemesinin gereksiz olduğudur: eşe
// doğrudan bağlıyız; ya da röle üzerinden sınırlı bağlıyız ve elimizde
// denenecek doğrudan bir adres yok (doğrudan bağlantıya geçmeyi libp2p'nin
// delik açması dener).
func (d *Discovery) bagli(pi peer.AddrInfo) bool {
	switch d.host.Network().Connectedness(pi.ID) {
	case libp2pnet.Connected:
		return true
	case libp2pnet.Limited:
		for _, a := range pi.Addrs {
			if tur := adresinTuru(a); tur != adresRole && tur != adresKullanilmaz {
				return false
			}
		}
		return true
	}
	return false
}

// bilinenlereBaglan, bilinen eşler listesinden bağlı olmayan en fazla n
// eşe bağlanmayı dener; bağlanılamayanlar listede geriye düşer.
func (d *Discovery) bilinenlereBaglan(ctx context.Context, n int) {
	adaylar := d.node.Bilinen.Adaylar(n, func(p peer.ID) bool {
		return p == d.host.ID() || d.host.Network().Connectedness(p) == libp2pnet.Connected
	})
	var wg sync.WaitGroup
	for _, ai := range adaylar {
		wg.Go(func() {
			if err := d.baglan(ctx, ai); err != nil && ctx.Err() == nil {
				d.node.Bilinen.Basarisiz(ai.ID)
			}
		})
	}
	wg.Wait()
}

// ─── Eş değişimi ────────────────────────────────────────────────────────────

// degisimDongusu, bağlı AND eşlerine bağlı oldukları öteki AND düğümlerini
// sorar: yeni bağlanan eşe hemen (hedefin altındaysak), sonra aralıklarla
// rastgele birine.
func (d *Discovery) degisimDongusu(ctx context.Context) {
	sorulan := map[peer.ID]time.Time{}
	zaman := time.NewTimer(d.sure.degisimAraligi)
	defer zaman.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-d.degisimIste:
			if d.node.Esler.Say() > hedefANDEs {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(d.sure.degisimIlk):
			}
			d.degis(ctx, p, sorulan)
		case <-zaman.C:
			aralik := d.sure.degisimAraligi
			if d.node.Esler.Say() >= hedefANDEs {
				aralik = d.sure.degisimYeterli
			}
			if p, ok := d.degisimAdayi(sorulan, aralik); ok {
				d.degis(ctx, p, sorulan)
			}
			zaman.Reset(aralik)
		}
	}
}

// degisimAdayi, eş değişimi sorulabilecek bağlı AND eşlerinden (protokolü
// konuşan, doğrudan bağlı, son aralik içinde sorulmamış) rastgele birini seçer.
func (d *Discovery) degisimAdayi(sorulan map[peer.ID]time.Time, aralik time.Duration) (peer.ID, bool) {
	var adaylar []peer.ID
	for _, e := range d.node.Esler.Esler() {
		if !e.Degisim || e.Sinirli {
			continue
		}
		if t, ok := sorulan[e.ID]; ok && time.Since(t) < aralik {
			continue
		}
		adaylar = append(adaylar, e.ID)
	}
	if len(adaylar) == 0 {
		return "", false
	}
	return adaylar[rand.IntN(len(adaylar))], true
}

// degis, p'ye bağlı olduğu AND düğümlerini sorar ve bağlı olmadıklarımıza
// bağlanmayı başlatır.
func (d *Discovery) degis(ctx context.Context, p peer.ID, sorulan map[peer.ID]time.Time) {
	bilgi, ok := d.node.Esler.Bilgi(p)
	if !ok || !bilgi.Degisim || bilgi.Sinirli {
		return
	}
	// Karşı taraf sık soranı geri çevirir (degisimEnAzAralik).
	if t, ok := sorulan[p]; ok && time.Since(t) < degisimEnAzAralik+time.Second {
		return
	}
	if len(sorulan) > 256 {
		for k := range sorulan {
			if _, bagli := d.node.Esler.Bilgi(k); !bagli {
				delete(sorulan, k)
			}
		}
	}
	sorulan[p] = time.Now()

	adaylar, err := d.node.EsleriIste(ctx, p)
	if err != nil {
		return
	}
	yeni := 0
	for _, ai := range adaylar {
		if d.bagli(ai) {
			continue
		}
		yeni++
		if d.node.Esler.Say() < ustANDEs {
			d.arkadaBaglan(ctx, ai)
		}
	}
	d.mu.Lock()
	d.ist.SonDegisim = time.Now()
	d.ist.DegisimdenGelen += yeni
	d.mu.Unlock()
}

// ─── Röle adayları ──────────────────────────────────────────────────────────

// roleAdaylari, NAT arkasındaki düğümün yer ayırtabileceği röle adaylarını
// verir (libp2p'nin autorelay'i çağırır): bağlı eşlerden röle hizmeti
// sunanlar; AND düğümleri önce.
//
// Eskiden adaylar yalnızca buluşma noktasındaki AND düğümleriydi. İnternetten
// doğrudan erişilebilen bir AND düğümü yokken (küçük bir toplulukta herkes ev
// modeminin arkasındadır) hiçbir düğüm röle bulamıyor, dolayısıyla NAT
// arkasındaki iki düğüm birbirine hiç bağlanamıyordu. Genel libp2p ağındaki
// düğümlerin çoğu sınırlı bir röle hizmeti sunar: kısa süreli ve az veri
// taşıyan bu bağlantı, iki düğümün birbirine doğrudan bağlanmasını (delik
// açma) başlatmaya yeter. İçerik röleden geçmez.
func (d *Discovery) roleAdaylari(ctx context.Context, num int) <-chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo, max(num, 1))
	go func() {
		defer close(ch)
		for _, pi := range bagliRoleler(d.host, num) {
			select {
			case ch <- pi:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// bagliRoleler, bağlı eşlerden röle hizmeti sunan ve internetten erişilebilir
// bir adresi olan en fazla n tanesini verir: AND düğümleri önce, ötekiler
// rastgele sırada (aynı rölelere yığılmamak için).
func bagliRoleler(h host.Host, n int) []peer.AddrInfo {
	var and, diger []peer.AddrInfo
	ps := h.Peerstore()
	for _, p := range h.Network().Peers() {
		if h.Network().Connectedness(p) != libp2pnet.Connected {
			continue // yalnızca röle üzerinden ulaşılan bir eş röle olamaz
		}
		if destek, err := ps.SupportsProtocols(p, roleProtokolu); err != nil || len(destek) == 0 {
			continue
		}
		adresler := ps.Addrs(p)
		acik := false
		for _, a := range adresler {
			if adresinTuru(a) == adresAcik {
				acik = true
				break
			}
		}
		if !acik {
			continue
		}
		pi := peer.AddrInfo{ID: p, Addrs: adresler}
		if ANDEsiMi(ps, p) {
			and = append(and, pi)
		} else {
			diger = append(diger, pi)
		}
	}
	rand.Shuffle(len(diger), func(i, j int) { diger[i], diger[j] = diger[j], diger[i] })
	out := append(and, diger...)
	return out[:min(n, len(out))]
}

// ─── Durum ──────────────────────────────────────────────────────────────────

func (d *Discovery) durum() KesifDurumu {
	d.mu.Lock()
	k := d.ist
	d.mu.Unlock()
	k.Calisiyor = true
	if d.dhtEsleri != nil {
		k.DHTEsleri = d.dhtEsleri()
	}
	return k
}

// ─── Bootstrap'a yeniden bağlanma ───────────────────────────────────────────

func (d *Discovery) rebootstrapLoop(ctx context.Context) {
	ticker := time.NewTicker(yenidenBaglanmaKontrolu)
	defer ticker.Stop()
	y := yenidenBaglanma{sonDeneme: time.Now()} // StartDiscovery az önce bağlandı
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			baglan, uyandir := y.adim(time.Now(), len(d.host.Network().Peers()))
			if uyandir {
				d.kesfiUyandir()
			}
			if baglan {
				connectToBootstrapPeers(ctx, d.host, d.extraBootstrap)
				// DHT'ye ulaşılamıyor olabilir: bilinen AND eşleri ondan
				// bağımsız denenir.
				if d.node.Esler.Say() == 0 {
					go d.bilinenlereBaglan(ctx, bilinenTurBoyu)
				}
			}
		}
	}
}

// yenidenBaglanmaKontrolu, bağlı eş sayısının ne sıklıkla denetlendiğidir
// (yalnızca sayıya bakılır; ucuzdur).
const yenidenBaglanmaKontrolu = 15 * time.Second

// yenidenBaglanmaBeklemesi, iki bootstrap denemesi arasındaki en kısa
// süredir. Hiç eş yokken (uykudan uyanma, ağ değişikliği, kablo çıkması)
// 15 sn'den başlar ve her sonuçsuz denemede ikiye katlanır (en çok
// rebootstrapInterval); birkaç eş varken rutin aralık geçerlidir.
func yenidenBaglanmaBeklemesi(esSayisi, ardisikBasarisiz int) time.Duration {
	if esSayisi > 0 {
		return rebootstrapInterval
	}
	return min(yenidenBaglanmaKontrolu<<min(ardisikBasarisiz, 5), rebootstrapInterval)
}

// yenidenBaglanma, bootstrap'a yeniden bağlanma kararlarıdır; zaman
// dışarıdan verildiği için doğrudan test edilir.
type yenidenBaglanma struct {
	sonDeneme time.Time
	ardisik   int  // üst üste sonuçsuz kalan deneme sayısı
	bekleyen  bool // son denetimde bağlanma denendi, sonucu bu denetimde görülür
	sifirdi   bool // önceki denetimde hiç eş yoktu
}

// adim, bir denetimde eş sayısına bakar: bootstrap'a bağlanılıp
// bağlanılmayacağını ve (bağlantı yeni geri geldiyse) keşfin uyandırılıp
// uyandırılmayacağını söyler.
func (y *yenidenBaglanma) adim(simdi time.Time, esSayisi int) (baglan, uyandir bool) {
	if y.bekleyen && esSayisi == 0 {
		y.ardisik++
	}
	y.bekleyen = false
	uyandir = y.sifirdi && esSayisi > 0
	y.sifirdi = esSayisi == 0
	if esSayisi > 0 {
		y.ardisik = 0
	}
	if esSayisi >= minPeersBeforeRebootstrap || simdi.Sub(y.sonDeneme) < yenidenBaglanmaBeklemesi(esSayisi, y.ardisik) {
		return false, uyandir
	}
	y.sonDeneme, y.bekleyen = simdi, true
	return true, uyandir
}

// ─── Kütüphane günlükleri ───────────────────────────────────────────────────

// quietMDNSLogOnce ensures muteNoisyMDNSLog only ever installs its filter
// once per process, even if StartDiscovery runs more than once (ör. testler).
var quietMDNSLogOnce sync.Once

// muteNoisyMDNSLog, standart kütüphanenin varsayılan logger çıktısını (bunu
// github.com/libp2p/zeroconf/v2 — go-libp2p'nin mDNS uygulamasının alt
// katmanı — go-libp2p'nin kendi seviyeli logger'ı ÜZERİNDEN DEĞİL, doğrudan
// stdlib log.Printf ile yazar) yalnızca "Failed to set multicast interface"
// uyarısını süzen bir filtreyle değiştirir. Windows'ta zeroconf'un
// linux/darwin-dışı geri dönüş yolu (server.go'daki writeResponse), gönderdiği
// HER mDNS yanıtında yerel makinede numaralandırılmış HER ağ arayüzü için bir
// kez net.PacketConn.SetMulticastInterface çağırır; kapalı, loopback ya da
// başka bir sebeple uygun olmayan arayüzler (VPN istemcileri, Hyper-V
// switch'leri, WSL, Bluetooth PAN gibi birçok sanal adaptöre sahip Windows
// makinelerinde yaygın) bu çağrıda başarısız olur ve HER başarısızlık ayrı
// bir satır loglar. Sonuç, saniyede onlarca neredeyse özdeş "[WARN] mdns:
// Failed to set multicast interface: no such interface" satırıdır — gerçek
// tanılama çıktısını boğar, ama mDNS başarılı olan arayüzlerde zaten normal
// çalışmaya devam eder ve DHT/bilinen-eş keşfi bundan hiç etkilenmez. Bu,
// AND'ın kendi kodunun kontrol edemediği, yukarı akış (upstream) zeroconf/v2
// davranışıdır — bu yüzden kaynağında "düzeltilmek" yerine burada süzülür.
// Standart log paketiyle yazılan BAŞKA her şey stderr'e değişmeden ulaşır.
func muteNoisyMDNSLog() {
	quietMDNSLogOnce.Do(func() {
		log.SetOutput(mdnsNoiseFilter{out: os.Stderr})
	})
}

// RedirectLibraryLogs, kütüphanelerin terminale yazdığı günlükleri w'ye
// yönlendirir: standart log paketi (zeroconf'un mDNS uyarısı yine süzülür)
// ve go-log (libp2p'nin kendi günlüğü; varsayılanı hata düzeyindeki
// satırları stderr'e yazmaktır). AND, tam ekran TUI açılmadan hemen önce
// çağırır — arayüzün üstüne terminale yazılan tek bir satır bile ekranı
// kaydırıp çerçeveyi bozar. Sonradan çalışan bir muteNoisyMDNSLog bu
// yönlendirmeyi geri almaz.
func RedirectLibraryLogs(w io.Writer) {
	quietMDNSLogOnce.Do(func() {})
	log.SetOutput(mdnsNoiseFilter{out: w})
	golog.SetPrimaryCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(w), zapcore.DebugLevel))
}

// mdnsNoiseFilter, yalnızca muteNoisyMDNSLog'un doc yorumunda açıklanan tek
// bir gürültülü satırı düşürür; başka her şeyi olduğu gibi alttaki writer'a
// iletir.
type mdnsNoiseFilter struct{ out io.Writer }

func (f mdnsNoiseFilter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("mdns: Failed to set multicast interface")) {
		return len(p), nil
	}
	return f.out.Write(p)
}

// connectToBootstrapPeers dials all bootstrap peers concurrently and returns
// immediately — it does not block startup waiting for slow/offline peers.
// The DHT bootstrap process handles reconnects automatically.
func connectToBootstrapPeers(ctx context.Context, h host.Host, extra []peer.AddrInfo) {
	peers := append(dht.GetDefaultBootstrapPeerAddrInfos(), extra...)
	for _, pi := range peers {
		go func(pi peer.AddrInfo) {
			cctx, cancel := context.WithTimeout(ctx, bootstrapConnectTimeout)
			defer cancel()
			_ = h.Connect(cctx, pi)
		}(pi)
	}
}

func (d *Discovery) Close() error {
	if d.eventSub != nil {
		d.eventSub.Close()
	}
	d.node.kesif.CompareAndSwap(d, nil)
	mdnsErr := d.mdns.Close()
	dhtErr := d.dht.Close()
	if mdnsErr != nil {
		return mdnsErr
	}
	return dhtErr
}

// peerFoundNotifee, yerel ağda mDNS ile bulunan düğümlere bağlanır.
type peerFoundNotifee struct {
	ctx   context.Context
	kesif *Discovery
}

func (n peerFoundNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.kesif.arkadaBaglan(n.ctx, pi)
}
