package network

// esler.go — bağlı AND düğümlerinin izlenmesi.
//
// Bir AND düğümünün bağlı olduğu eşlerin çoğu AND düğümü değil, eş bulmak
// için kullanılan genel DHT'nin düğümleridir. Hangi eşin AND düğümü olduğu
// identify'ın bildirdiği protokollerden anlaşılır; bu bilgi eskiden üç ayrı
// yerde, her seferinde yeniden hesaplanıyordu. EsIzleyici onu tek yerde
// tutar ve üzerine şunları kurar: AND eşleri bağlantı yöneticisinde
// etiketlenir (budamada en sona kalırlar), imzalı adres kayıtları eş
// değişimi için saklanır, gecikmeleri ölçülür ve gelip gidişleri
// dinleyicilere bildirilir.

import (
	"context"
	"log"
	"math/rand/v2"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/record"
	"github.com/libp2p/go-libp2p/p2p/host/eventbus"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"

	"github.com/lucian95511/and/internal/guvenli"
	"github.com/lucian95511/and/protokol"
)

// andProtokolleri, bir eşin AND düğümü olduğunu gösteren protokollerdir
// (geçmiş senkronu): her AND düğümü bunları sunar, genel DHT düğümleri sunmaz.
var andProtokolleri = []protocol.ID{
	protokol.ForumSenkronProtokolu,
	protokol.SohbetSenkronProtokolu,
	protokol.SiteSenkronProtokolu,
}

// ANDProtokolleri, bir eşin AND düğümü olduğunu gösteren protokollerdir.
func ANDProtokolleri() []protocol.ID { return slices.Clone(andProtokolleri) }

const (
	// surumSiniri, bir eşin bildirdiği sürüm metninden saklanan en fazla bayttır.
	surumSiniri = 48
	// gecikmeAraligi, bağlı AND eşlerinin gecikmesinin ölçülme aralığıdır.
	gecikmeAraligi = 90 * time.Second
	// gecikmeZamanAsimi, tek bir ölçümün en uzun süresidir.
	gecikmeZamanAsimi = 5 * time.Second
	// gecikmeEnFazlaEs, bir turda gecikmesi ölçülen en fazla eştir.
	gecikmeEnFazlaEs = 32
)

// EsBilgisi, bağlı bir AND düğümüdür.
type EsBilgisi struct {
	ID peer.ID
	// Surum, eşin identify'da bildirdiği program sürümüdür ("AND/v0.2.0").
	// Eşin kendi beyanıdır: ekrana basılmadan önce temizlenmelidir.
	Surum string
	// Baglandi, eşin bu bağlantıda AND düğümü olarak ilk görüldüğü andır.
	Baglandi time.Time
	// Sinirli, eşe yalnızca bir röle üzerinden, süresi ve veri miktarı
	// sınırlı bir bağlantı olduğudur. Böyle bir bağlantı üzerinden forum ve
	// sohbet akmaz; yalnızca özel mesaj gönderilebilir.
	Sinirli bool
	// Degisim, eşin eş değişimi protokolünü konuştuğudur (eski sürümler konuşmaz).
	Degisim bool
	// Gecikme, son ölçülen gidiş-dönüş süresidir (henüz ölçülmediyse 0).
	Gecikme time.Duration
}

// EsOlayTuru, bir EsOlayi'nin ne bildirdiğidir.
type EsOlayTuru int

const (
	// EsBaglandi: bir AND düğümü bağlandı (ya da bağlı bir eşin AND düğümü
	// olduğu anlaşıldı).
	EsBaglandi EsOlayTuru = iota
	// EsGuncellendi: bağlı bir AND düğümünün bilgisi (adresleri, bağlantı
	// türü, protokolleri) değişti.
	EsGuncellendi
	// EsAyrildi: bir AND düğümünün bağlantısı koptu.
	EsAyrildi
)

// EsOlayi, bağlı AND düğümlerindeki bir değişikliktir.
type EsOlayi struct {
	Tur EsOlayTuru
	Es  EsBilgisi
}

type esKaydi struct {
	bilgi EsBilgisi
	// kayit, eşin kendi imzaladığı adres kaydıdır (identify'dan); eş
	// değişiminde başkalarına aynen verilir. Eş göndermediyse nil.
	kayit []byte
}

// EsIzleyici, bağlı AND düğümlerini izler. Yöntemleri eşzamanlı çağrılabilir.
type EsIzleyici struct {
	host  host.Host
	ctx   context.Context
	simdi func() time.Time

	mu           sync.RWMutex
	esler        map[peer.ID]*esKaydi
	dinleyiciler []func(EsOlayi)
}

func yeniEsIzleyici(ctx context.Context, h host.Host) *EsIzleyici {
	z := &EsIzleyici{host: h, ctx: ctx, simdi: time.Now, esler: map[peer.ID]*esKaydi{}}
	sub, err := h.EventBus().Subscribe([]any{
		new(event.EvtPeerIdentificationCompleted),
		new(event.EvtPeerProtocolsUpdated),
		new(event.EvtPeerConnectednessChanged),
	}, eventbus.BufSize(256), eventbus.Name("and-es-izleyici"))
	if err != nil {
		// Olay yolu yoksa AND eşleri hiç tanınmaz; düğüm yine de çalışır.
		log.Printf("ağ: eş izleyici başlatılamadı: %v", err)
		return z
	}
	go z.calis(sub)
	go z.gecikmeDongusu()
	return z
}

// Dinle, AND eşleri gelip gittikçe f'yi çağırır. f izleyicinin olay
// döngüsünde çalışır: bloklamamalıdır.
func (z *EsIzleyici) Dinle(f func(EsOlayi)) {
	z.mu.Lock()
	z.dinleyiciler = append(z.dinleyiciler, f)
	z.mu.Unlock()
}

// Esler, bağlı AND düğümleridir (en eski bağlanan önce).
func (z *EsIzleyici) Esler() []EsBilgisi {
	z.mu.RLock()
	out := make([]EsBilgisi, 0, len(z.esler))
	for _, k := range z.esler {
		out = append(out, k.bilgi)
	}
	z.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Baglandi.Equal(out[j].Baglandi) {
			return out[i].Baglandi.Before(out[j].Baglandi)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Say, doğrudan (sınırlı olmayan bir bağlantıyla) bağlı AND düğümlerinin
// sayısıdır: forum ve sohbetin aktığı eşler.
func (z *EsIzleyici) Say() int {
	z.mu.RLock()
	defer z.mu.RUnlock()
	n := 0
	for _, k := range z.esler {
		if !k.bilgi.Sinirli {
			n++
		}
	}
	return n
}

// Bilgi, p bağlı bir AND düğümüyse bilgisini verir.
func (z *EsIzleyici) Bilgi(p peer.ID) (EsBilgisi, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()
	if k := z.esler[p]; k != nil {
		return k.bilgi, true
	}
	return EsBilgisi{}, false
}

// imzaliKayitlar, haric dışındaki bağlı AND düğümlerinin imzalı adres
// kayıtlarından rastgele en fazla n tanesini verir (eş değişimi için).
func (z *EsIzleyici) imzaliKayitlar(haric peer.ID, n int) [][]byte {
	z.mu.RLock()
	out := make([][]byte, 0, len(z.esler))
	for p, k := range z.esler {
		if p != haric && len(k.kayit) > 0 {
			out = append(out, k.kayit)
		}
	}
	z.mu.RUnlock()
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out[:min(n, len(out))]
}

func (z *EsIzleyici) calis(sub event.Subscription) {
	defer sub.Close()
	for {
		select {
		case <-z.ctx.Done():
			return
		case e, ok := <-sub.Out():
			if !ok {
				return
			}
			// Olaylar eşin gönderdiği veriyi taşır (protokol listesi, sürüm
			// metni): işlenirken çıkan bir hata düğümü çökertmesin.
			guvenli.Isle("eş izleyici", func() { z.olay(e) })
		}
	}
}

func (z *EsIzleyici) olay(e any) {
	switch ev := e.(type) {
	case event.EvtPeerIdentificationCompleted:
		z.guncelle(ev.Peer, ev.Protocols, ev.SignedPeerRecord, ev.AgentVersion)
	case event.EvtPeerProtocolsUpdated:
		protolar, err := z.host.Peerstore().GetProtocols(ev.Peer)
		if err != nil {
			return
		}
		z.guncelle(ev.Peer, protolar, nil, "")
	case event.EvtPeerConnectednessChanged:
		switch ev.Connectedness {
		case libp2pnet.Connected, libp2pnet.Limited:
			z.baglantiDegisti(ev.Peer, ev.Connectedness == libp2pnet.Limited)
		default:
			z.cikar(ev.Peer)
		}
	}
}

// andMi, protokol listesinde AND'ın senkron protokollerinden biri olup
// olmadığıdır.
func andMi(protolar []protocol.ID) bool {
	for _, p := range protolar {
		if slices.Contains(andProtokolleri, p) {
			return true
		}
	}
	return false
}

// ANDEsiMi, p'nin AND düğümü olduğunun peerstore'daki identify bilgisinden
// bilinip bilinmediğidir (bağlı olması gerekmez).
func ANDEsiMi(ps peerstore.Peerstore, p peer.ID) bool {
	protolar, err := ps.GetProtocols(p)
	return err == nil && andMi(protolar)
}

// guncelle, p'nin bildirdiği protokollere göre kaydını ekler, yeniler ya da
// (artık AND protokolü sunmuyorsa) çıkarır.
func (z *EsIzleyici) guncelle(p peer.ID, protolar []protocol.ID, kayit *record.Envelope, surum string) {
	bagli := z.host.Network().Connectedness(p)
	if !andMi(protolar) || (bagli != libp2pnet.Connected && bagli != libp2pnet.Limited) {
		z.cikar(p)
		return
	}
	var kayitBaytlari []byte
	if kayit != nil {
		if b, err := kayit.Marshal(); err == nil && len(b) <= degisimKayitSiniri {
			kayitBaytlari = b
		}
	}

	z.mu.Lock()
	k, vardi := z.esler[p]
	if !vardi {
		k = &esKaydi{bilgi: EsBilgisi{ID: p, Baglandi: z.simdi()}}
		z.esler[p] = k
	}
	if surum != "" {
		k.bilgi.Surum = surumuKirp(surum)
	}
	k.bilgi.Sinirli = bagli == libp2pnet.Limited
	k.bilgi.Degisim = slices.Contains(protolar, EsDegisimProtokolu)
	if kayitBaytlari != nil {
		k.kayit = kayitBaytlari
	}
	bilgi := k.bilgi
	z.mu.Unlock()

	if vardi {
		z.bildir(EsOlayi{Tur: EsGuncellendi, Es: bilgi})
		return
	}
	z.host.ConnManager().TagPeer(p, andEtiketi, andEtiketPuani)
	z.bildir(EsOlayi{Tur: EsBaglandi, Es: bilgi})
	if !bilgi.Sinirli {
		go z.gecikmeOlc(p)
	}
}

// baglantiDegisti, bilinen bir AND eşinin bağlantı türünü (doğrudan ya da
// röle üzerinden sınırlı) günceller: delik açma başarılı olunca sınırlı
// bağlantı doğrudan bağlantıya döner.
func (z *EsIzleyici) baglantiDegisti(p peer.ID, sinirli bool) {
	z.mu.Lock()
	k := z.esler[p]
	if k == nil || k.bilgi.Sinirli == sinirli {
		z.mu.Unlock()
		return
	}
	k.bilgi.Sinirli = sinirli
	bilgi := k.bilgi
	z.mu.Unlock()
	z.bildir(EsOlayi{Tur: EsGuncellendi, Es: bilgi})
	if !sinirli {
		go z.gecikmeOlc(p)
	}
}

func (z *EsIzleyici) cikar(p peer.ID) {
	z.mu.Lock()
	k := z.esler[p]
	if k == nil {
		z.mu.Unlock()
		return
	}
	delete(z.esler, p)
	bilgi := k.bilgi
	z.mu.Unlock()
	z.host.ConnManager().UntagPeer(p, andEtiketi)
	z.bildir(EsOlayi{Tur: EsAyrildi, Es: bilgi})
}

func (z *EsIzleyici) bildir(o EsOlayi) {
	z.mu.RLock()
	dinleyiciler := slices.Clone(z.dinleyiciler)
	z.mu.RUnlock()
	for _, f := range dinleyiciler {
		f(o)
	}
}

// surumuKirp, eşin bildirdiği sürüm metnini saklanacak boya indirir.
func surumuKirp(s string) string {
	if len(s) <= surumSiniri {
		return s
	}
	// Çok baytlı bir karakterin ortasından kesme.
	kes := surumSiniri
	for kes > 0 && s[kes]&0xC0 == 0x80 {
		kes--
	}
	return s[:kes]
}

func (z *EsIzleyici) gecikmeDongusu() {
	t := time.NewTicker(gecikmeAraligi)
	defer t.Stop()
	for {
		select {
		case <-z.ctx.Done():
			return
		case <-t.C:
			esler := z.Esler()
			for _, e := range esler[:min(len(esler), gecikmeEnFazlaEs)] {
				if !e.Sinirli {
					z.gecikmeOlc(e.ID)
				}
			}
		}
	}
}

// gecikmeOlc, p'ye bir ping gönderip gidiş-dönüş süresini kaydeder.
func (z *EsIzleyici) gecikmeOlc(p peer.ID) {
	ctx, iptal := context.WithTimeout(z.ctx, gecikmeZamanAsimi)
	defer iptal()
	var sonuc ping.Result
	select {
	case sonuc = <-ping.Ping(ctx, z.host, p):
	case <-ctx.Done():
		return
	}
	if sonuc.Error != nil {
		return
	}
	// Aynı makinede ya da saatin kaba olduğu sistemlerde (Windows) süre 0
	// ölçülebilir; 0 "henüz ölçülmedi" demek olduğundan en az 1 µs yazılır.
	z.mu.Lock()
	if k := z.esler[p]; k != nil {
		k.bilgi.Gecikme = max(sonuc.RTT, time.Microsecond)
	}
	z.mu.Unlock()
}
