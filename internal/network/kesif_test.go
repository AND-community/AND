package network

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	ma "github.com/multiformats/go-multiaddr"
)

// sahteYon, buluşma noktasının (DHT) yerine geçer.
type sahteYon struct {
	mu           sync.Mutex
	duyuru       int
	arama        int
	duyuruHatasi error
	duyuruEngeli chan struct{} // kapatılana kadar Advertise bloklar
	saglayicilar []peer.AddrInfo
}

func (y *sahteYon) Advertise(ctx context.Context, _ string, _ ...discovery.Option) (time.Duration, error) {
	y.mu.Lock()
	y.duyuru++
	engel, hata := y.duyuruEngeli, y.duyuruHatasi
	y.mu.Unlock()
	if engel != nil {
		select {
		case <-engel:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return time.Hour, hata
}

func (y *sahteYon) FindPeers(_ context.Context, _ string, _ ...discovery.Option) (<-chan peer.AddrInfo, error) {
	y.mu.Lock()
	y.arama++
	s := slices.Clone(y.saglayicilar)
	y.mu.Unlock()
	ch := make(chan peer.AddrInfo, len(s))
	for _, pi := range s {
		ch <- pi
	}
	close(ch)
	return ch, nil
}

func (y *sahteYon) sayilar() (duyuru, arama int) {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.duyuru, y.arama
}

// uzunSureler, hiçbir şeyin kendiliğinden yinelenmediği zamanlamalardır;
// testler yalnızca sınadıkları süreyi kısaltır.
func uzunSureler() kesifSureleri {
	return kesifSureleri{
		aramaZamanAsimi:  5 * time.Second,
		aramaEnKisa:      time.Hour,
		aramaEnUzun:      time.Hour,
		aramaAzEs:        time.Hour,
		aramaYeterli:     time.Hour,
		duyuruAraligi:    time.Hour,
		duyuruGeciktirme: 20 * time.Millisecond,
		duyuruEnAz:       40 * time.Millisecond,
		duyuruHataEnKisa: time.Hour,
		duyuruHataEnUzun: time.Hour,
		dhtBekleme:       5 * time.Second,
		baglanma:         5 * time.Second,
		adresCozme:       5 * time.Second,
		degisimIlk:       10 * time.Millisecond,
		degisimAraligi:   time.Hour,
		degisimYeterli:   time.Hour,
	}
}

// kesifKur, gerçek DHT ve mDNS olmadan keşfin döngülerini başlatır.
func kesifKur(t *testing.T, n *Node, yon yonlendirici, sure kesifSureleri, ayarla ...func(*Discovery)) *Discovery {
	t.Helper()
	d := yeniKesif(n, yon, sure)
	for _, f := range ayarla {
		f(d)
	}
	ctx, iptal := context.WithCancel(context.Background())
	t.Cleanup(iptal)
	d.baslat(ctx)
	return d
}

func bagliMi(a, b *Node) bool {
	return a.Host.Network().Connectedness(b.Host.ID()) == libp2pnet.Connected
}

// Arama, duyurunun bitmesini beklemez. Eskiden tek döngüydü: duyuru
// sürerken (gerçek DHT'de bir dakikaya kadar) ya da başarısız olduğunda hiç
// arama yapılmıyordu.
func TestKesif_AramaDuyuruyuBeklemez(t *testing.T) {
	for ad, yon := range map[string]*sahteYon{
		"duyuru sürerken":     {duyuruEngeli: make(chan struct{})},
		"duyuru başarısızken": {duyuruHatasi: errors.New("failed to find any peer in table")},
	} {
		t.Run(ad, func(t *testing.T) {
			a, b := andDugumu(t), andDugumu(t)
			yon.saglayicilar = []peer.AddrInfo{adresBilgisi(b), adresBilgisi(a)} // kendi kaydı da gelir
			d := kesifKur(t, a, yon, uzunSureler())
			bekle(t, 5*time.Second, func() bool { return bagliMi(a, b) }, "buluşma noktasındaki düğüme bağlanılmalı")
			bekle(t, 5*time.Second, func() bool { return d.durum().SonBulunan == 1 }, "arama sonucu kaydedilmeli (kendimiz sayılmaz)")
			if k := d.durum(); k.SonArama.IsZero() || !k.Calisiyor {
				t.Fatalf("keşif durumu: %+v", k)
			}
			if yon.duyuruHatasi != nil {
				bekle(t, 5*time.Second, func() bool { return d.durum().DuyuruHatasi != "" }, "duyuru hatası durumda görünmeli")
			}
		})
	}
}

// Açılışta DHT'nin yönlendirme tablosu boştur: o anda yapılan arama ve
// duyuru hiçbir yere gitmeden biter. Keşif DHT'nin ilk eşini bekler.
func TestKesif_DHTHazirOlmadanBaslamaz(t *testing.T) {
	a := andDugumu(t)
	yon := &sahteYon{}
	var hazir atomic.Bool
	kesifKur(t, a, yon, uzunSureler(), func(d *Discovery) { d.dhtHazir = hazir.Load })

	olmasin(t, 400*time.Millisecond, func() bool { du, ar := yon.sayilar(); return du+ar > 0 }, "DHT hazır değilken arama ya da duyuru yapıldı")
	hazir.Store(true)
	bekle(t, 5*time.Second, func() bool { du, ar := yon.sayilar(); return du == 1 && ar == 1 }, "DHT hazır olunca arama ve duyuru başlamalı")

	// DHT hiç hazır olmazsa (internet yok) bekleme süresi dolunca yine denenir:
	// yerel ağdaki DHT ile çalışabilir.
	b := andDugumu(t)
	yon2 := &sahteYon{}
	sure := uzunSureler()
	sure.dhtBekleme = 150 * time.Millisecond
	kesifKur(t, b, yon2, sure, func(d *Discovery) { d.dhtHazir = func() bool { return false } })
	bekle(t, 5*time.Second, func() bool { du, ar := yon2.sayilar(); return du == 1 && ar == 1 }, "bekleme dolunca arama ve duyuru yapılmalı")
}

func TestAramaAraligi(t *testing.T) {
	s := varsayilanSureler
	for _, tc := range []struct {
		and, bos int
		beklenen time.Duration
	}{
		{0, 0, 30 * time.Second},
		{0, 1, 30 * time.Second},
		{0, 2, time.Minute},
		{0, 3, 2 * time.Minute},
		{0, 4, 4 * time.Minute},
		{0, 5, 5 * time.Minute},
		{0, 500, 5 * time.Minute},
		{1, 0, 5 * time.Minute},
		{hedefANDEs - 1, 3, 5 * time.Minute},
		{hedefANDEs, 0, 15 * time.Minute},
		{hedefANDEs + 20, 0, 15 * time.Minute},
	} {
		if got := aramaAraligi(s, tc.and, tc.bos); got != tc.beklenen {
			t.Errorf("aramaAraligi(and=%d, bos=%d) = %v; %v beklenirdi", tc.and, tc.bos, got, tc.beklenen)
		}
	}
}

// Yalnız bir düğüm aramayı katlanarak seyrekleştirir ve bu arada bilinen
// eşlerini yeniden dener.
func TestKesif_YalnizkenSeyrekArar(t *testing.T) {
	a := andDugumu(t)
	yon := &sahteYon{}
	sure := uzunSureler()
	sure.aramaEnKisa, sure.aramaEnUzun = 60*time.Millisecond, 240*time.Millisecond
	kesifKur(t, a, yon, sure)
	// 60 + 120 + 240 + 240 ms: bir saniyede 5–6 arama; sabit 60 ms olsaydı ~16.
	time.Sleep(time.Second)
	if _, arama := yon.sayilar(); arama < 4 || arama > 8 {
		t.Fatalf("bir saniyede %d arama; aralık katlanarak 4–8 arası olmalı", arama)
	}
}

// Son AND eşi ayrılınca bir sonraki arama beklenmez.
func TestKesif_SonEsAyrilincaArar(t *testing.T) {
	a, b := andDugumu(t), andDugumu(t)
	yon := &sahteYon{}
	kesifKur(t, a, yon, uzunSureler())
	bekle(t, 5*time.Second, func() bool { _, ar := yon.sayilar(); return ar == 1 }, "ilk arama yapılmalı")

	bagla(t, a, b)
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 1 }, "B tanınmalı")
	olmasin(t, 200*time.Millisecond, func() bool { _, ar := yon.sayilar(); return ar > 1 }, "AND eşi varken erken arama yapıldı")

	_ = a.Host.Network().ClosePeer(b.Host.ID())
	bekle(t, 5*time.Second, func() bool { _, ar := yon.sayilar(); return ar == 2 }, "son AND eşi ayrılınca hemen aranmalı")
}

// Duyurulan adresler (açık ve röle adresleri) değişince kayıt beklemeden
// yenilenir; değişmediyse yenilenmez.
func TestKesif_AdresDegisinceYenidenDuyurur(t *testing.T) {
	var ek atomic.Pointer[[]ma.Multiaddr]
	a := andDugumu(t, func(s *Secenekler) {
		s.Libp2p = append(s.Libp2p, libp2p.AddrsFactory(func(adr []ma.Multiaddr) []ma.Multiaddr {
			if e := ek.Load(); e != nil {
				return append(slices.Clone(adr), *e...)
			}
			return adr
		}))
	})
	yon := &sahteYon{}
	d := kesifKur(t, a, yon, uzunSureler())
	bekle(t, 5*time.Second, func() bool { du, _ := yon.sayilar(); return du == 1 }, "ilk duyuru yapılmalı")
	bekle(t, 5*time.Second, func() bool { return !d.durum().SonDuyuru.IsZero() }, "duyuru zamanı kaydedilmeli")

	durt := func() {
		select {
		case d.adresDegisti <- struct{}{}:
		default:
		}
	}
	// Adresler aynı: yeniden duyurulmaz.
	durt()
	olmasin(t, 250*time.Millisecond, func() bool { du, _ := yon.sayilar(); return du > 1 }, "adres değişmeden yeniden duyuruldu")

	// Yalnızca yerel bir adres eklendi (ör. sanal ağ arayüzü): genel DHT onu
	// taşımaz, yeniden duyurmaya gerek yok.
	yerel := adresler(t, "/ip4/192.168.77.5/tcp/4001")
	ek.Store(&yerel)
	durt()
	olmasin(t, 250*time.Millisecond, func() bool { du, _ := yon.sayilar(); return du > 1 }, "yalnızca yerel adres değişince yeniden duyuruldu")

	// Bir röle adresi geldi: kayıt yenilenir.
	role := adresler(t, "/ip4/192.168.77.5/tcp/4001", "/ip4/9.9.9.9/tcp/4001/p2p/"+rastgeleEs(t).String()+"/p2p-circuit")
	ek.Store(&role)
	durt()
	bekle(t, 5*time.Second, func() bool { du, _ := yon.sayilar(); return du == 2 }, "röle adresi gelince yeniden duyurulmalı")
	durt()
	olmasin(t, 250*time.Millisecond, func() bool { du, _ := yon.sayilar(); return du > 2 }, "aynı adreslerle üçüncü kez duyuruldu")
}

func TestIlanParmakIzi(t *testing.T) {
	role := "/ip4/9.9.9.9/tcp/1/p2p/" + rastgeleEs(t).String() + "/p2p-circuit"
	bir := ilanParmakIzi(adresler(t, "/ip4/1.2.3.4/tcp/1", yerelAdres, "/ip4/127.0.0.1/tcp/1", role))
	// Sıra ve yerel adresler parmak izini değiştirmez.
	iki := ilanParmakIzi(adresler(t, role, "/ip4/10.1.1.1/tcp/9", "/ip4/1.2.3.4/tcp/1"))
	if bir != iki || bir == "" {
		t.Fatalf("parmak izleri farklı:\n%q\n%q", bir, iki)
	}
	if ilanParmakIzi(adresler(t, "/ip4/1.2.3.4/tcp/1")) == bir {
		t.Fatal("röle adresi kalkınca parmak izi değişmeli")
	}
	if ilanParmakIzi(adresler(t, yerelAdres)) != "" {
		t.Fatal("yalnızca yerel adresi olan düğümün parmak izi boş olmalı")
	}
}

// Buluşma kaydı adres taşımıyorsa adres aranır. Eskiden böyle bir düğüme
// hiç bağlanılamıyordu.
func TestKesif_AdressizSaglayici(t *testing.T) {
	t.Run("DHT'den çözülür", func(t *testing.T) {
		a, b := andDugumu(t), andDugumu(t)
		yon := &sahteYon{saglayicilar: []peer.AddrInfo{{ID: b.Host.ID()}}}
		var cozuldu atomic.Int32
		kesifKur(t, a, yon, uzunSureler(), func(d *Discovery) {
			d.adresCoz = func(_ context.Context, p peer.ID) (peer.AddrInfo, error) {
				cozuldu.Add(1)
				if p != b.Host.ID() {
					return peer.AddrInfo{}, errors.New("bilinmiyor")
				}
				return adresBilgisi(b), nil
			}
		})
		bekle(t, 5*time.Second, func() bool { return bagliMi(a, b) }, "adresi çözülen düğüme bağlanılmalı")
		if cozuldu.Load() != 1 {
			t.Fatalf("adres %d kez arandı", cozuldu.Load())
		}
	})
	t.Run("bilinen eşlerden alınır", func(t *testing.T) {
		a, b := andDugumu(t), andDugumu(t)
		// Açılışta çevrilmesin diye liste keşif başladıktan sonra doldurulur.
		yon := &sahteYon{}
		sure := uzunSureler()
		d := kesifKur(t, a, yon, sure, func(d *Discovery) {
			d.adresCoz = func(context.Context, peer.ID) (peer.AddrInfo, error) {
				t.Error("bilinen eşin adresi DHT'de aranmamalı")
				return peer.AddrInfo{}, errors.New("yok")
			}
		})
		a.Bilinen.Gor(b.Host.ID(), b.Host.Addrs())
		if err := d.baglan(context.Background(), peer.AddrInfo{ID: b.Host.ID()}); err != nil {
			t.Fatalf("bilinen adresle bağlanılamadı: %v", err)
		}
	})
	t.Run("çözülemezse hata", func(t *testing.T) {
		a := andDugumu(t)
		d := kesifKur(t, a, &sahteYon{}, uzunSureler(), func(d *Discovery) {
			d.adresCoz = func(context.Context, peer.ID) (peer.AddrInfo, error) {
				return peer.AddrInfo{}, errors.New("routing: not found")
			}
		})
		if err := d.baglan(context.Background(), peer.AddrInfo{ID: rastgeleEs(t)}); err == nil {
			t.Fatal("adresi bulunamayan düğüme bağlanma hata vermeli")
		}
	})
}

// Düğüm açılır açılmaz, buluşma noktası hiçbir şey döndürmese de, daha önce
// bağlandığı AND düğümlerine bağlanır; ulaşılamayanlar listede geriye düşer.
func TestKesif_BilinenEslereBaglanir(t *testing.T) {
	a, b := andDugumu(t), andDugumu(t)
	olu := rastgeleEs(t)
	a.Bilinen.Gor(b.Host.ID(), b.Host.Addrs())
	a.Bilinen.Gor(olu, adresler(t, "/ip4/127.0.0.1/tcp/1")) // kimse dinlemiyor

	// DHT hiç hazır olmuyor: bilinen eşler ondan bağımsız çevrilir.
	sure := uzunSureler()
	sure.dhtBekleme = time.Hour
	kesifKur(t, a, &sahteYon{}, sure, func(d *Discovery) { d.dhtHazir = func() bool { return false } })

	bekle(t, 5*time.Second, func() bool { return bagliMi(a, b) }, "bilinen eşe DHT beklenmeden bağlanılmalı")
	bekle(t, 10*time.Second, func() bool {
		a.Bilinen.mu.Lock()
		defer a.Bilinen.mu.Unlock()
		k := a.Bilinen.esler[olu]
		return k != nil && k.basarisiz == 1
	}, "ulaşılamayan bilinen eşin başarısızlığı sayılmalı")
	a.Bilinen.mu.Lock()
	basarili := a.Bilinen.esler[b.Host.ID()].basarisiz
	a.Bilinen.mu.Unlock()
	if basarili != 0 {
		t.Fatal("bağlanılan eşin başarısızlık sayısı artmamalı")
	}
}

// Bir AND düğümüne bağlanan, eş değişimiyle onun bağlı olduğu ötekilere de
// bağlanır: buluşma noktasına (DHT) hiç gerek kalmadan.
func TestKesif_EsDegisimiIleBulur(t *testing.T) {
	a, b, c, e := andDugumu(t), andDugumu(t), andDugumu(t), andDugumu(t)
	bagla(t, c, b)
	bagla(t, e, b)
	bekle(t, 5*time.Second, func() bool { return b.Esler.Say() == 2 }, "B, C ve E'yi tanımalı")

	d := kesifKur(t, a, &sahteYon{}, uzunSureler())
	bagla(t, a, b)
	bekle(t, 10*time.Second, func() bool { return bagliMi(a, c) && bagliMi(a, e) }, "A, B'nin söylediği C ve E'ye bağlanmalı")
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 3 }, "A üç AND eşini tanımalı")
	if k := d.durum(); k.DegisimdenGelen != 2 || k.SonDegisim.IsZero() {
		t.Fatalf("keşif durumu: %+v", k)
	}
}

// Eş değişimini konuşmayan (eski sürüm) bir AND düğümüne hiç sorulmaz.
func TestKesif_EskiSurumeSorulmaz(t *testing.T) {
	a, eski := andDugumu(t), andDugumu(t)
	eski.Host.RemoveStreamHandler(EsDegisimProtokolu)
	d := kesifKur(t, a, &sahteYon{}, uzunSureler())
	bagla(t, a, eski)
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 1 }, "eski sürüm de AND eşi sayılmalı")
	if bilgi, _ := a.Esler.Bilgi(eski.Host.ID()); bilgi.Degisim {
		t.Fatal("protokolü sunmayan eş, eş değişimi konuşuyor görünmemeli")
	}
	olmasin(t, 300*time.Millisecond, func() bool { return !d.durum().SonDegisim.IsZero() }, "eski sürüme eş değişimi soruldu")
	if _, ok := d.degisimAdayi(map[peer.ID]time.Time{}, time.Hour); ok {
		t.Fatal("eski sürüm eş değişimi adayı olmamalı")
	}
}

// testRolesi, röle hizmeti sunan ve "internetten erişilebilir" görünen bir
// düğümdür: loopback adresini, libp2p'nin açık adres saydığı bir alan adıyla
// duyurur (libp2p'nin kendi testlerindeki yöntem).
func testRolesi(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
		libp2p.EnableRelayService(),
		libp2p.ForceReachabilityPublic(),
		libp2p.AddrsFactory(func(adr []ma.Multiaddr) []ma.Multiaddr {
			out := make([]ma.Multiaddr, 0, len(adr))
			for _, a := range adr {
				s := a.String()
				if strings.HasPrefix(s, "/ip4/127.0.0.1/") {
					a = ma.StringCast("/dns/libp2p.internal" + strings.TrimPrefix(s, "/ip4/127.0.0.1"))
				}
				out = append(out, a)
			}
			return out
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	bekle(t, 5*time.Second, func() bool { return slices.Contains(h.Mux().Protocols(), roleProtokolu) }, "röle hizmeti başlamalı")
	return h
}

// yerelAdresBilgisi, h'nin gerçekten dinlediği (loopback) adresleridir.
func yerelAdresBilgisi(h host.Host) peer.AddrInfo {
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Network().ListenAddresses()}
}

// NAT arkasındaki düğüm, AND düğümü OLMAYAN bir röleden yer ayırtır.
// Eskiden yalnızca AND düğümleri röle adayıydı: erişilebilir bir AND düğümü
// yokken NAT arkasındaki hiçbir düğüme dışarıdan ulaşılamıyordu.
func TestKesif_NATArkasindaRoledenYerAyirtir(t *testing.T) {
	role := testRolesi(t)
	genel := duzDugum(t) // röle sunmayan sıradan bir eş
	b := andDugumu(t, func(s *Secenekler) {
		s.Libp2p = append(s.Libp2p, libp2p.ForceReachabilityPrivate())
		s.roleSecenekleri = []autorelay.Option{
			autorelay.WithBootDelay(0), autorelay.WithMinInterval(50 * time.Millisecond), autorelay.WithMinCandidates(1),
		}
	})
	d := kesifKur(t, b, &sahteYon{}, uzunSureler())
	b.SetRelaySource(d.roleAdaylari)

	bekle(t, 5*time.Second, func() bool { return b.Durum().Erisim == ErisimKapali }, "röle yokken düğüm dışarıdan erişilemez görünmeli")

	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	bagla(t, b, genel)
	if err := b.Host.Connect(ctx, yerelAdresBilgisi(role)); err != nil {
		t.Fatal(err)
	}

	bekle(t, 5*time.Second, func() bool {
		adaylar := bagliRoleler(b.Host, 8)
		return len(adaylar) == 1 && adaylar[0].ID == role.ID()
	}, "bağlı eşlerden yalnızca röle sunan aday olmalı")

	bekle(t, 15*time.Second, func() bool { return b.Durum().Roleler == 1 }, "röleden yer ayırtılmalı")
	durum := b.Durum()
	if durum.Erisim != ErisimRoleIle {
		t.Fatalf("erişim %d; röle üzerinden erişilebilir olmalı", durum.Erisim)
	}
	if len(durum.AcikAdresler) != 0 {
		t.Fatalf("NAT arkasındaki düğüm açık adres göstermemeli: %v", durum.AcikAdresler)
	}

	// Başka bir düğüm, röle adresi üzerinden bu düğüme ulaşır.
	a := andDugumu(t)
	if err := a.Host.Connect(ctx, yerelAdresBilgisi(role)); err != nil {
		t.Fatal(err)
	}
	roleAdresi := role.Network().ListenAddresses()[0].Encapsulate(ma.StringCast("/p2p/" + role.ID().String() + "/p2p-circuit"))
	if err := a.Host.Connect(ctx, peer.AddrInfo{ID: b.Host.ID(), Addrs: []ma.Multiaddr{roleAdresi}}); err != nil {
		t.Fatalf("röle üzerinden bağlanılamadı: %v", err)
	}
	if c := a.Host.Network().Connectedness(b.Host.ID()); c != libp2pnet.Limited {
		t.Fatalf("bağlantı türü %v; röle üzerinden sınırlı olmalı", c)
	}
	// AND eşi olarak tanınır ama "sınırlı" sayılır: forum ve sohbet bu
	// bağlantıdan akmaz.
	bekle(t, 5*time.Second, func() bool {
		bilgi, ok := a.Esler.Bilgi(b.Host.ID())
		return ok && bilgi.Sinirli
	}, "röle üzerinden bağlanılan AND eşi sınırlı olarak tanınmalı")
	if a.Esler.Say() != 0 {
		t.Fatal("sınırlı bağlantı doğrudan AND eşi sayılmamalı")
	}
	ad := a.Durum()
	if ad.SinirliEs != 1 || ad.ANDEs != 0 || len(ad.Esler) != 1 || ad.Esler[0].Tasiyici != "role" {
		t.Fatalf("A'nın durumu: %+v", ad)
	}
	// Röle adresi bilinen eşlere yazılmaz: rölenin (loopback) adresi dışarıdan
	// işe yaramaz. Gerçek ağda rölenin açık adresi yazılırdı.
	if d.bagli(peer.AddrInfo{ID: role.ID()}) != true {
		t.Fatal("röleye bağlı olunmalı")
	}
}

func TestBagli_SinirliBaglantidaDogrudanAdresDenenir(t *testing.T) {
	a := andDugumu(t)
	d := kesifKur(t, a, &sahteYon{}, uzunSureler())
	p := rastgeleEs(t)
	// Bağlı olmayan eş: her zaman denenir.
	if d.bagli(peer.AddrInfo{ID: p, Addrs: adresler(t, "/ip4/1.2.3.4/tcp/1")}) {
		t.Fatal("bağlı olmayan eş bağlı sayıldı")
	}
}

func TestDurum_Temel(t *testing.T) {
	a, b := andDugumu(t, func(s *Secenekler) { s.Surum = "v1.2.3" }), andDugumu(t)
	ilk := a.Durum()
	if ilk.Erisim != ErisimBilinmiyor || ilk.NAT != NATBilinmiyor || ilk.BagliEs != 0 || ilk.ANDEs != 0 || ilk.Kesif.Calisiyor || ilk.Port != 0 {
		t.Fatalf("yeni düğümün durumu: %+v", ilk)
	}

	d := kesifKur(t, a, &sahteYon{}, uzunSureler(), func(d *Discovery) { d.dhtEsleri = func() int { return 42 } })
	bagla(t, a, b)
	bekle(t, 5*time.Second, func() bool { return a.Durum().ANDEs == 1 && b.Durum().ANDEs == 1 }, "iki taraf da AND eşini görmeli")

	ad, bd := a.Durum(), b.Durum()
	if !ad.Kesif.Calisiyor || ad.Kesif.DHTEsleri != 42 || bd.Kesif.Calisiyor {
		t.Fatalf("keşif durumu: A %+v, B %+v", ad.Kesif, bd.Kesif)
	}
	if ad.BagliEs != 1 || ad.BilinenEs != 1 || len(ad.Esler) != 1 {
		t.Fatalf("A'nın durumu: %+v", ad)
	}
	es := ad.Esler[0]
	if es.ID != b.Host.ID() || es.Tasiyici != "tcp" || !es.Giden || !es.Yerel || !strings.HasPrefix(es.Adres, "127.0.0.1:") {
		t.Fatalf("A'daki eş: %+v", es)
	}
	if karsi := bd.Esler[0]; karsi.Giden || karsi.Surum != "AND/v1.2.3" {
		t.Fatalf("B'deki eş: %+v", karsi)
	}

	// Keşif kapanınca durum "çalışmıyor" olur.
	a.kesif.CompareAndSwap(d, nil)
	if a.Durum().Kesif.Calisiyor {
		t.Fatal("keşif kapandıktan sonra çalışıyor görünmemeli")
	}
}

func TestAdresOzeti(t *testing.T) {
	role := rastgeleEs(t).String()
	for adres, beklenen := range map[string][2]string{
		"/ip4/1.2.3.4/tcp/4001":                                   {"1.2.3.4:4001", "tcp"},
		"/ip4/1.2.3.4/udp/4001/quic-v1":                           {"1.2.3.4:4001", "quic"},
		"/ip6/2a02::1/udp/9/quic-v1":                              {"[2a02::1]:9", "quic"},
		"/dns4/ornek.org/tcp/443":                                 {"ornek.org:443", "tcp"},
		"/ip4/9.9.9.9/tcp/4001/p2p/" + role + "/p2p-circuit":      {"9.9.9.9:4001", "role"},
		"/ip4/9.9.9.9/udp/1/quic-v1/p2p/" + role + "/p2p-circuit": {"9.9.9.9:1", "role"},
	} {
		a, tas := adresOzeti(adresler(t, adres)[0])
		if a != beklenen[0] || tas != beklenen[1] {
			t.Errorf("adresOzeti(%s) = %q, %q; %q, %q beklenirdi", adres, a, tas, beklenen[0], beklenen[1])
		}
	}
	if a, tas := adresOzeti(nil); a != "" || tas != "" {
		t.Error("nil adresin özeti boş olmalı")
	}
	if got := roleKimligi(adresler(t, "/ip4/9.9.9.9/tcp/1/p2p/"+role+"/p2p-circuit")[0]); got != role {
		t.Errorf("roleKimligi = %q", got)
	}
	if got := roleKimligi(adresler(t, "/ip4/9.9.9.9/tcp/1")[0]); got != "" {
		t.Errorf("röle olmayan adresin röle kimliği olmamalı: %q", got)
	}
}
