package network

// durum.go — ağın o anki durumunun özeti (arayüzdeki Ayarlar › Ağ sekmesi).
//
// Eşler arası bir ağda "neden kimseyi göremiyorum" sorusunun yanıtı çoğu
// zaman düğümün kendi ağ konumundadır: NAT arkasında mı, bir röleden yer
// ayırtabilmiş mi, kaç AND düğümüne bağlı, keşif en son ne zaman çalıştı.
// Bu bilgiler eskiden yalnızca günlükte, o da kısmen vardı.

import (
	"context"
	"log"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/eventbus"
	ma "github.com/multiformats/go-multiaddr"
)

// Erisilebilirlik, düğüme internetten ulaşılıp ulaşılamadığıdır.
type Erisilebilirlik int

const (
	// ErisimBilinmiyor: henüz belirlenemedi (açılıştan sonraki ilk ~20 saniye
	// ya da internet yok).
	ErisimBilinmiyor Erisilebilirlik = iota
	// ErisimAcik: öteki düğümler doğrudan bağlanabilir.
	ErisimAcik
	// ErisimRoleIle: NAT ya da güvenlik duvarı arkasında; bir röle üzerinden
	// ulaşılabilir ve doğrudan bağlantı delik açmayla kurulur.
	ErisimRoleIle
	// ErisimKapali: NAT ya da güvenlik duvarı arkasında ve henüz bir röleden
	// yer ayırtılamadı: dışarıdan kimse bağlanamaz, yalnızca bu düğüm
	// başkalarına bağlanabilir.
	ErisimKapali
)

// NATDurumu, NAT'ın delik açmaya (iki NAT arkasındaki düğümün doğrudan
// bağlanmasına) uygun olup olmadığıdır.
type NATDurumu int

const (
	NATBilinmiyor NATDurumu = iota
	// NATUygun: NAT her hedefe aynı dış portu kullanıyor; delik açma çalışır.
	NATUygun
	// NATZor: NAT her hedef için ayrı port seçiyor (simetrik NAT, operatör
	// NAT'ı); delik açma çoğunlukla başarısız olur.
	NATZor
)

// EsDurumu, bağlı bir AND düğümünün bağlantı ayrıntısıdır.
type EsDurumu struct {
	EsBilgisi
	// Adres, bağlantının karşı ucudur ("203.0.113.7:4001"); röle üzerinden
	// bağlantıda rölenin adresidir.
	Adres string
	// Tasiyici: "quic", "tcp" ya da röle üzerinden bağlantıda "role".
	Tasiyici string
	// Giden, bağlantıyı bu düğümün kurduğudur.
	Giden bool
	// Yerel, bağlantının yerel ağ (ya da aynı makine) üzerinden olduğudur.
	Yerel bool
}

// KesifDurumu, eş keşfinin son çalışmalarıdır.
type KesifDurumu struct {
	// Calisiyor, keşfin başlatıldığıdır (testlerde ve keşifsiz düğümlerde false).
	Calisiyor bool
	// DHTEsleri, genel DHT'nin yönlendirme tablosundaki düğüm sayısıdır; 0
	// ise DHT'ye (internete) ulaşılamıyor demektir.
	DHTEsleri int
	// SonDuyuru, buluşma noktasına son başarılı duyurunun zamanıdır.
	SonDuyuru time.Time
	// DuyuruHatasi, son duyuru denemesi başarısızsa nedenidir.
	DuyuruHatasi string
	// SonArama, buluşma noktasında son aramanın bittiği zamandır.
	SonArama time.Time
	// SonBulunan, o aramada bulunan (kendimiz dışındaki) düğüm sayısıdır.
	SonBulunan int
	// SonDegisim, son eş değişiminin zamanıdır.
	SonDegisim time.Time
	// DegisimdenGelen, eş değişimiyle bugüne dek öğrenilen düğüm sayısıdır.
	DegisimdenGelen int
}

// Durum, ağın o anki özetidir.
type Durum struct {
	Erisim Erisilebilirlik
	NAT    NATDurumu
	// Port, dinlenen kalıcı porttur (rastgeleyse 0).
	Port int
	// AcikAdresler, öteki düğümlere duyurulan, internetten erişilebilir
	// doğrudan adreslerdir.
	AcikAdresler []string
	// Roleler, yer ayırtılmış rölelerin sayısıdır.
	Roleler int
	// BagliEs, bağlı bütün eşlerdir (çoğu genel DHT düğümü).
	BagliEs int
	// ANDEs, doğrudan bağlı AND düğümleridir.
	ANDEs int
	// SinirliEs, yalnızca röle üzerinden ulaşılan AND düğümleridir.
	SinirliEs int
	// BilinenEs, bilinen eşler listesindeki düğüm sayısıdır.
	BilinenEs int
	// Esler, bağlı AND düğümleridir (doğrudan bağlılar önce).
	Esler []EsDurumu
	Kesif KesifDurumu
}

// durumIzleyici, libp2p'nin erişilebilirlik ve NAT türü olaylarını izler.
type durumIzleyici struct {
	mu              sync.RWMutex
	erisilebilirlik libp2pnet.Reachability
	nat             map[libp2pnet.NATTransportProtocol]libp2pnet.NATDeviceType
}

func yeniDurumIzleyici(ctx context.Context, h host.Host) *durumIzleyici {
	d := &durumIzleyici{nat: map[libp2pnet.NATTransportProtocol]libp2pnet.NATDeviceType{}}
	sub, err := h.EventBus().Subscribe([]any{
		new(event.EvtLocalReachabilityChanged),
		new(event.EvtNATDeviceTypeChanged),
	}, eventbus.Name("and-durum"))
	if err != nil {
		log.Printf("ağ: durum izleyici başlatılamadı: %v", err)
		return d
	}
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-sub.Out():
				if !ok {
					return
				}
				d.mu.Lock()
				switch ev := e.(type) {
				case event.EvtLocalReachabilityChanged:
					d.erisilebilirlik = ev.Reachability
				case event.EvtNATDeviceTypeChanged:
					d.nat[ev.TransportProtocol] = ev.NatDeviceType
				}
				d.mu.Unlock()
			}
		}
	}()
	return d
}

func (d *durumIzleyici) oku() (libp2pnet.Reachability, NATDurumu) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// Delik açma önce QUIC (UDP) ile denenir; UDP uygunsa yeterlidir.
	nat := NATBilinmiyor
	for _, tur := range d.nat {
		switch tur {
		case libp2pnet.NATDeviceTypeEndpointIndependent:
			nat = NATUygun
		case libp2pnet.NATDeviceTypeEndpointDependent:
			if nat == NATBilinmiyor {
				nat = NATZor
			}
		}
	}
	return d.erisilebilirlik, nat
}

// Durum, ağın o anki özetini verir. Ucuzdur: ağa istek göndermez.
func (n *Node) Durum() Durum {
	h := n.Host
	erisim, nat := n.durum.oku()
	d := Durum{NAT: nat, Port: n.port, BagliEs: len(h.Network().Peers()), BilinenEs: n.Bilinen.Say()}

	roleler := map[string]bool{}
	for _, a := range h.Addrs() {
		switch adresinTuru(a) {
		case adresAcik:
			d.AcikAdresler = append(d.AcikAdresler, a.String())
		case adresRole:
			if id := roleKimligi(a); id != "" {
				roleler[id] = true
			}
		}
	}
	d.Roleler = len(roleler)

	switch erisim {
	case libp2pnet.ReachabilityPublic:
		d.Erisim = ErisimAcik
	case libp2pnet.ReachabilityPrivate:
		d.Erisim = ErisimKapali
		if d.Roleler > 0 {
			d.Erisim = ErisimRoleIle
		}
		// NAT arkasındayken gözlenen dış adresler duyurulsa da çevrilemez.
		d.AcikAdresler = nil
	}
	if erisim != libp2pnet.ReachabilityPrivate {
		// NAT türü yalnızca NAT arkasındayken anlamlıdır.
		d.NAT = NATBilinmiyor
	}

	for _, e := range n.Esler.Esler() {
		es := EsDurumu{EsBilgisi: e}
		if c := enIyiBaglanti(h, e.ID); c != nil {
			es.Adres, es.Tasiyici = adresOzeti(c.RemoteMultiaddr())
			es.Giden = c.Stat().Direction == libp2pnet.DirOutbound
			tur := adresinTuru(c.RemoteMultiaddr())
			es.Yerel = tur == adresYerel || tur == adresLoopback
		}
		if e.Sinirli {
			d.SinirliEs++
		} else {
			d.ANDEs++
		}
		d.Esler = append(d.Esler, es)
	}
	sort.SliceStable(d.Esler, func(i, j int) bool { return !d.Esler[i].Sinirli && d.Esler[j].Sinirli })

	if k := n.kesif.Load(); k != nil {
		d.Kesif = k.durum()
	}
	return d
}

// enIyiBaglanti, p ile kurulu bağlantılardan gösterilecek olanı seçer:
// varsa doğrudan (sınırlı olmayan) bir bağlantı.
func enIyiBaglanti(h host.Host, p peer.ID) libp2pnet.Conn {
	var secilen libp2pnet.Conn
	for _, c := range h.Network().ConnsToPeer(p) {
		if !c.Stat().Limited {
			return c
		}
		if secilen == nil {
			secilen = c
		}
	}
	return secilen
}

// roleKimligi, bir röle adresindeki rölenin Peer ID'sidir
// (…/p2p/<röle>/p2p-circuit); bulunamazsa boş.
func roleKimligi(a ma.Multiaddr) string {
	son := ""
	bulunan := ""
	ma.ForEach(a, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_P2P:
			son = c.Value()
		case ma.P_CIRCUIT:
			bulunan = son
			return false
		}
		return true
	})
	return bulunan
}

// adresOzeti, bir bağlantı adresini "ip:port" ve taşıyıcı adı olarak özetler.
func adresOzeti(a ma.Multiaddr) (adres, tasiyici string) {
	if a == nil {
		return "", ""
	}
	var ip, port string
	ma.ForEach(a, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_IP4, ma.P_IP6, ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
			if ip == "" {
				ip = c.Value()
			}
		case ma.P_TCP:
			if port == "" {
				port = c.Value()
			}
			if tasiyici == "" {
				tasiyici = "tcp"
			}
		case ma.P_UDP:
			if port == "" {
				port = c.Value()
			}
		case ma.P_QUIC_V1:
			if tasiyici == "" {
				tasiyici = "quic"
			}
		case ma.P_CIRCUIT:
			tasiyici = "role"
			return false
		}
		return true
	})
	if ip == "" {
		return "", tasiyici
	}
	if _, err := strconv.Atoi(port); err != nil {
		return ip, tasiyici
	}
	return net.JoinHostPort(ip, port), tasiyici
}
