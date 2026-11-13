package network

import (
	"strings"
	"sync"
	"testing"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/protokol"
)

// olayKaydi, bir izleyicinin bildirdiği olayları toplar.
type olayKaydi struct {
	mu      sync.Mutex
	olaylar []EsOlayi
}

func (k *olayKaydi) ekle(o EsOlayi) {
	k.mu.Lock()
	k.olaylar = append(k.olaylar, o)
	k.mu.Unlock()
}

func (k *olayKaydi) say(tur EsOlayTuru, p peer.ID) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := 0
	for _, o := range k.olaylar {
		if o.Tur == tur && o.Es.ID == p {
			n++
		}
	}
	return n
}

// İzleyici bağlı eşlerden yalnızca AND düğümlerini sayar, onları bağlantı
// yöneticisinde etiketler ve gelip gidişlerini bildirir.
func TestEsIzleyici_ANDEsleriniTanir(t *testing.T) {
	a := andDugumu(t)
	b := andDugumu(t, func(s *Secenekler) { s.Surum = "v9.8.7" })
	genel := duzDugum(t) // AND protokolü sunmayan bir düğüm (ör. DHT düğümü)

	var kayit olayKaydi
	a.Esler.Dinle(kayit.ekle)

	bagla(t, a, genel)
	bagla(t, a, b)
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 1 }, "A, B'yi AND eşi olarak tanımalı")
	// Karşı taraf da tanır (bağlantıyı o kurmadı).
	bekle(t, 5*time.Second, func() bool { return b.Esler.Say() == 1 }, "B de A'yı tanımalı")

	if _, ok := a.Esler.Bilgi(genel.Host.ID()); ok {
		t.Fatal("AND protokolü sunmayan düğüm AND eşi sayılmamalı")
	}
	bilgi, ok := a.Esler.Bilgi(b.Host.ID())
	if !ok || bilgi.Sinirli || !bilgi.Degisim || bilgi.Baglandi.IsZero() {
		t.Fatalf("B'nin bilgisi: %+v (var=%v)", bilgi, ok)
	}
	if bilgi.Surum != "AND/v9.8.7" {
		t.Errorf("sürüm %q; AND/v9.8.7 beklenirdi", bilgi.Surum)
	}
	if esler := a.Esler.Esler(); len(esler) != 1 || esler[0].ID != b.Host.ID() {
		t.Fatalf("Esler() = %+v", esler)
	}
	if n := kayit.say(EsBaglandi, b.Host.ID()); n != 1 {
		t.Fatalf("EsBaglandi %d kez bildirildi; 1 beklenirdi", n)
	}

	// Bağlantı yöneticisi AND eşini etiketler; genel düğümü etiketlemez.
	if puan := a.Host.ConnManager().GetTagInfo(b.Host.ID()).Tags[andEtiketi]; puan != andEtiketPuani {
		t.Errorf("AND eşinin etiket puanı %d; %d beklenirdi", puan, andEtiketPuani)
	}
	if ti := a.Host.ConnManager().GetTagInfo(genel.Host.ID()); ti != nil && ti.Tags[andEtiketi] != 0 {
		t.Error("genel düğüm AND eşi olarak etiketlenmemeli")
	}

	// Gecikme bağlanınca ölçülür.
	bekle(t, 5*time.Second, func() bool {
		bilgi, _ := a.Esler.Bilgi(b.Host.ID())
		return bilgi.Gecikme > 0
	}, "bağlanan AND eşinin gecikmesi ölçülmeli")

	// Bağlantı kopunca eş listeden çıkar ve bildirilir.
	if err := a.Host.Network().ClosePeer(b.Host.ID()); err != nil {
		t.Fatal(err)
	}
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 0 && kayit.say(EsAyrildi, b.Host.ID()) == 1 },
		"kopan eş listeden çıkmalı ve bildirilmeli")
}

// Bağlandıktan SONRA AND protokolünü sunmaya başlayan düğüm de tanınır:
// gerçek açılışta bağlantılar forum senkronu kaydedilmeden önce kurulur ve
// protokol sonradan identify push ile duyurulur.
func TestEsIzleyici_SonradanDuyurulanProtokol(t *testing.T) {
	a := andDugumu(t)
	b := duzDugum(t)
	bagla(t, a, b)
	olmasin(t, 300*time.Millisecond, func() bool { return a.Esler.Say() > 0 }, "protokol duyurulmadan AND eşi sayıldı")

	andYap(b)
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 1 }, "sonradan duyurulan AND protokolü tanınmalı")

	// Protokolü bırakınca (ör. başka bir programa dönüşen kimlik) çıkar.
	b.Host.RemoveStreamHandler(protokol.ForumSenkronProtokolu)
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 0 }, "AND protokolünü bırakan eş listeden çıkmalı")
}

// Bağlı AND eşleri bilinen eşler listesine adresleriyle yazılır.
func TestEsIzleyici_BilinenEslereYazar(t *testing.T) {
	a := andDugumu(t)
	b := andDugumu(t)
	genel := duzDugum(t)
	bagla(t, a, genel)
	bagla(t, a, b)
	bekle(t, 5*time.Second, func() bool { return len(a.Bilinen.Adresler(b.Host.ID())) > 0 }, "bağlanan AND eşi bilinenlere yazılmalı")

	if a.Bilinen.Say() != 1 {
		t.Fatalf("bilinen eş sayısı %d; yalnızca AND eşi yazılmalı", a.Bilinen.Say())
	}
	// Saklanan adres B'nin gerçekten dinlediği adrestir.
	var dinlenen []string
	for _, ad := range b.Host.Addrs() {
		dinlenen = append(dinlenen, ad.String())
	}
	if got := a.Bilinen.Adresler(b.Host.ID())[0].String(); !strings.Contains(strings.Join(dinlenen, " "), got) {
		t.Errorf("saklanan adres %s, B'nin adresleri %v", got, dinlenen)
	}
	// Bağlantıyı kurmayan taraf, karşının bildirdiği adresi saklar (gelen
	// bağlantının kaynak portunu değil).
	bekle(t, 5*time.Second, func() bool { return len(b.Bilinen.Adresler(a.Host.ID())) > 0 }, "B de A'yı bilinenlere yazmalı")
	if got := b.Bilinen.Adresler(a.Host.ID())[0].String(); got != a.Host.Addrs()[0].String() {
		t.Errorf("B'nin sakladığı adres %s; A'nın dinlediği %s olmalı", got, a.Host.Addrs()[0])
	}
}

// Sürüm metni eşin beyanıdır: uzun bir metin kırpılır.
func TestSurumuKirp(t *testing.T) {
	if got := surumuKirp("AND/v1.0.0"); got != "AND/v1.0.0" {
		t.Errorf("kısa sürüm değişmemeli: %q", got)
	}
	uzun := strings.Repeat("ş", 100) // 2 baytlık karakterler
	got := surumuKirp(uzun)
	if len(got) > surumSiniri || len(got) < surumSiniri-1 {
		t.Errorf("kırpılan boy %d; sınır %d", len(got), surumSiniri)
	}
	if strings.ContainsRune(got, '�') || !strings.HasPrefix(uzun, got) {
		t.Errorf("çok baytlı karakter ortasından kesilmemeli: %q", got)
	}
}

func TestANDEsiMi(t *testing.T) {
	a := andDugumu(t)
	b := andDugumu(t)
	genel := duzDugum(t)
	bagla(t, a, b)
	bagla(t, a, genel)
	bekle(t, 5*time.Second, func() bool { return ANDEsiMi(a.Host.Peerstore(), b.Host.ID()) }, "B AND eşi olarak görülmeli")
	bekle(t, 5*time.Second, func() bool {
		p, err := a.Host.Peerstore().GetProtocols(genel.Host.ID())
		return err == nil && len(p) > 0
	}, "genel düğümün protokolleri öğrenilmeli")
	if ANDEsiMi(a.Host.Peerstore(), genel.Host.ID()) {
		t.Fatal("genel düğüm AND eşi sayılmamalı")
	}
	if a.Host.Network().Connectedness(b.Host.ID()) != libp2pnet.Connected {
		t.Fatal("bağlantı kopmamalı")
	}
}
