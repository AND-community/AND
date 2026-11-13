package network

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func rastgeleEs(t testing.TB) peer.ID {
	t.Helper()
	_, pub, err := lp2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func adresler(t testing.TB, ss ...string) []ma.Multiaddr {
	t.Helper()
	out := make([]ma.Multiaddr, 0, len(ss))
	for _, s := range ss {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		out = append(out, a)
	}
	return out
}

func adresMetinleri(as []ma.Multiaddr) string {
	ss := make([]string, len(as))
	for i, a := range as {
		ss[i] = a.String()
	}
	return strings.Join(ss, " ")
}

const (
	acikAdres1 = "/ip4/1.2.3.4/tcp/4001"
	acikAdres2 = "/ip4/5.6.7.8/udp/4001/quic-v1"
	yerelAdres = "/ip4/192.168.1.20/tcp/4001"
)

// Liste kaydedilir, yeniden açılır ve aynı eşleri aynı sırayla verir.
func TestBilinen_KaydetVeYukle(t *testing.T) {
	yol := filepath.Join(t.TempDir(), bilinenDosyaAdi)
	b := BilinenAc(yol)
	an := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	b.simdi = func() time.Time { return an }

	eski, yeni := rastgeleEs(t), rastgeleEs(t)
	b.Gor(eski, adresler(t, acikAdres1))
	an = an.Add(time.Hour)
	b.Gor(yeni, adresler(t, yerelAdres, acikAdres2))
	if err := b.Kaydet(); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		if bilgi, _ := os.Stat(yol); bilgi.Mode().Perm() != 0o600 {
			t.Errorf("dosya izni %v; 0600 olmalı (eş adresleri içerir)", bilgi.Mode().Perm())
		}
	}
	var d bilinenDosya
	veri, _ := os.ReadFile(yol)
	if err := json.Unmarshal(veri, &d); err != nil || d.Surum != bilinenSurumu || len(d.Esler) != 2 {
		t.Fatalf("dosya: %v\n%s", err, veri)
	}

	c := BilinenAc(yol)
	c.simdi = func() time.Time { return an }
	adaylar := c.Adaylar(10, nil)
	if len(adaylar) != 2 || adaylar[0].ID != yeni || adaylar[1].ID != eski {
		t.Fatalf("adaylar en son görülenden başlamalı: %+v", adaylar)
	}
	// Açık adres yerel adresten önce saklanır.
	if got := adresMetinleri(adaylar[0].Addrs); got != acikAdres2+" "+yerelAdres {
		t.Errorf("adres sırası: %s", got)
	}
	// Değişiklik yokken yeniden yazılmaz.
	_ = os.Remove(yol)
	if err := c.Kaydet(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Error("değişmeyen liste diske yazılmamalı")
	}
}

// Okunamayan dosya kenara alınır: bir sonraki kayıt üzerine yazmasın.
func TestBilinen_BozukDosyaKenaraAlinir(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, bilinenDosyaAdi)
	if err := os.WriteFile(yol, []byte(`{"surum":1,"esler":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	b := BilinenAc(yol)
	if b.Say() != 0 {
		t.Fatal("bozuk dosyadan eş yüklenmemeli")
	}
	if _, err := os.Stat(yol); !os.IsNotExist(err) {
		t.Fatal("bozuk dosya yerinde kalmamalı")
	}
	girdiler, _ := os.ReadDir(dir)
	if len(girdiler) != 1 || !strings.Contains(girdiler[0].Name(), "bozuk") {
		t.Fatalf("bozuk dosya kenara alınmalı: %v", girdiler)
	}
	// Liste yine de kullanılabilir ve yeniden kaydedilir.
	b.Gor(rastgeleEs(t), adresler(t, acikAdres1))
	if err := b.Kaydet(); err != nil {
		t.Fatal(err)
	}
}

// Yüklerken: süresi dolanlar, bozuk kimlikler ve kullanılamayan adresler
// atılır; gelecekteki bir zaman bugüne çekilir.
func TestBilinen_YuklerkenAyiklar(t *testing.T) {
	yol := filepath.Join(t.TempDir(), bilinenDosyaAdi)
	simdi := time.Now().UTC()
	gecerli, eskimis, gelecek, adressiz := rastgeleEs(t), rastgeleEs(t), rastgeleEs(t), rastgeleEs(t)
	d := bilinenDosya{Surum: 1, Esler: []bilinenDosyaKaydi{
		{ID: gecerli.String(), Adresler: []string{acikAdres1, "bu bir adres değil"}, Gorulme: simdi.Add(-time.Hour), Basarisiz: -4},
		{ID: eskimis.String(), Adresler: []string{acikAdres1}, Gorulme: simdi.Add(-bilinenOmru - time.Hour)},
		{ID: gelecek.String(), Adresler: []string{acikAdres2}, Gorulme: simdi.Add(240 * time.Hour)},
		{ID: adressiz.String(), Adresler: []string{"/ip4/0.0.0.0/tcp/1", "/ip4/169.254.3.4/tcp/1"}, Gorulme: simdi},
		{ID: "kimlik-degil", Adresler: []string{acikAdres1}, Gorulme: simdi},
	}}
	veri, _ := json.Marshal(d)
	if err := os.WriteFile(yol, veri, 0o600); err != nil {
		t.Fatal(err)
	}
	b := BilinenAc(yol)
	if b.Say() != 2 {
		t.Fatalf("%d eş yüklendi; 2 beklenirdi (geçerli ve geleceğe tarihli)", b.Say())
	}
	if got := adresMetinleri(b.Adresler(gecerli)); got != acikAdres1 {
		t.Errorf("geçerli eşin adresleri: %q", got)
	}
	b.mu.Lock()
	if g := b.esler[gelecek].gorulme; g.After(time.Now().Add(time.Minute)) {
		t.Errorf("gelecekteki zaman bugüne çekilmeli: %v", g)
	}
	if b.esler[gecerli].basarisiz != 0 {
		t.Error("eksi başarısızlık sayısı sıfırlanmalı")
	}
	b.mu.Unlock()
}

func TestBilinen_AdaylarSirasi(t *testing.T) {
	b := BilinenAc("")
	an := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	b.simdi = func() time.Time { return an }
	var idler []peer.ID
	for i := range 5 {
		id := rastgeleEs(t)
		idler = append(idler, id)
		b.Gor(id, adresler(t, acikAdres1))
		an = an.Add(time.Minute)
		_ = i
	}
	// En son görülen (4) üç kez bağlanılamadı: sona düşer.
	for range bilinenCokBasarisiz {
		b.Basarisiz(idler[4])
	}
	// 3 atlanır (ör. zaten bağlı).
	adaylar := b.Adaylar(10, func(p peer.ID) bool { return p == idler[3] })
	var sira []int
	for _, a := range adaylar {
		for i, id := range idler {
			if id == a.ID {
				sira = append(sira, i)
			}
		}
	}
	if fmt.Sprint(sira) != "[2 1 0 4]" {
		t.Fatalf("aday sırası %v; [2 1 0 4] beklenirdi", sira)
	}
	if n := len(b.Adaylar(2, nil)); n != 2 {
		t.Fatalf("n=2 iken %d aday", n)
	}
	// Yeniden görülünce başarısızlık sıfırlanır ve başa geçer.
	b.Gor(idler[4], adresler(t, acikAdres1))
	if b.Adaylar(1, nil)[0].ID != idler[4] {
		t.Fatal("yeniden bağlanılan eş başa geçmeli")
	}
	// Listede olmayan eş için Basarisiz ve Goruldu bir şey yapmaz.
	yabanci := rastgeleEs(t)
	b.Basarisiz(yabanci)
	b.Goruldu(yabanci)
	if b.Say() != 5 {
		t.Fatalf("liste %d eş; 5 olmalı", b.Say())
	}
}

func TestBilinen_SinirVeSure(t *testing.T) {
	b := BilinenAc("")
	an := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	b.simdi = func() time.Time { return an }
	ilk := rastgeleEs(t)
	b.Gor(ilk, adresler(t, acikAdres1))
	for range bilinenEnFazla + 10 {
		an = an.Add(time.Second)
		b.Gor(rastgeleEs(t), adresler(t, acikAdres1))
	}
	if b.Say() != bilinenEnFazla {
		t.Fatalf("liste %d eş; sınır %d", b.Say(), bilinenEnFazla)
	}
	if b.Adresler(ilk) != nil {
		t.Fatal("sınır aşılınca en eski eş çıkmalı")
	}
	// Otuz gün sonra hiçbiri kalmaz.
	an = an.Add(bilinenOmru + time.Hour)
	if n := len(b.Adaylar(10, nil)); n != 0 {
		t.Fatalf("süresi dolan eşler aday olmamalı; %d aday", n)
	}
	if b.Say() != 0 {
		t.Fatalf("süresi dolanlar çıkmalı; %d eş kaldı", b.Say())
	}
}

func TestBilinen_AdresBirlestirme(t *testing.T) {
	b := BilinenAc("")
	p := rastgeleEs(t)
	b.Gor(p, adresler(t, acikAdres1, yerelAdres))
	// Yeni adres başa geçer, eskiler korunur, tekrar eklenmez.
	b.Gor(p, adresler(t, acikAdres2, yerelAdres))
	if got := adresMetinleri(b.Adresler(p)); got != acikAdres2+" "+acikAdres1+" "+yerelAdres {
		t.Fatalf("birleşen adresler: %s", got)
	}
	// Adres verilmeden görülmek eski adresleri korur.
	b.Gor(p, nil)
	if len(b.Adresler(p)) != 3 {
		t.Fatal("adressiz görülme eski adresleri silmemeli")
	}
	// Hiç kullanılabilir adresi olmayan eş listeye girmez.
	q := rastgeleEs(t)
	b.Gor(q, adresler(t, "/ip4/0.0.0.0/tcp/4001", "/ip6/fe80::1/tcp/4001"))
	if b.Adresler(q) != nil || b.Say() != 1 {
		t.Fatal("adressiz eş listeye girmemeli")
	}
}

func TestAdresleriSec(t *testing.T) {
	role := rastgeleEs(t).String()
	roleAdresi := func(ip string) string { return "/ip4/" + ip + "/tcp/4001/p2p/" + role + "/p2p-circuit" }
	for _, tc := range []struct {
		ad      string
		girdi   []string
		beklene []string
	}{
		{"kullanılmayanlar atılır", []string{
			"/ip4/0.0.0.0/tcp/1", "/ip4/169.254.10.1/tcp/1", "/ip6/fe80::1/tcp/1", "/ip6/::/tcp/1", "/ip4/224.0.0.1/udp/1/quic-v1",
			"/p2p/" + role + "/p2p-circuit", // rölenin adresi bilinmiyor
			roleAdresi("192.168.1.5"),       // yerel ağdaki röle dışarıdan işe yaramaz
			roleAdresi("127.0.0.1"),
		}, nil},
		{"açık, yerel, röle sırası", []string{roleAdresi("9.9.9.1"), yerelAdres, "/ip4/127.0.0.1/tcp/9", acikAdres1, "/dns4/ornek.org/tcp/4001"},
			[]string{acikAdres1, "/dns4/ornek.org/tcp/4001", yerelAdres, "/ip4/127.0.0.1/tcp/9", roleAdresi("9.9.9.1")}},
		{"tekrarlar bir kez", []string{acikAdres1, acikAdres1, yerelAdres, acikAdres1}, []string{acikAdres1, yerelAdres}},
		{"en fazla iki röle adresi", []string{roleAdresi("9.9.9.1"), roleAdresi("9.9.9.2"), roleAdresi("9.9.9.3")},
			[]string{roleAdresi("9.9.9.1"), roleAdresi("9.9.9.2")}},
	} {
		t.Run(tc.ad, func(t *testing.T) {
			got := adresMetinleri(adresleriSec(adresler(t, tc.girdi...)))
			if got != strings.Join(tc.beklene, " ") {
				t.Errorf("\n got: %s\nwant: %s", got, strings.Join(tc.beklene, " "))
			}
		})
	}

	// Sınır dolsa da röle adresine yer kalır: NAT arkasındaki eşe ulaşmanın
	// tek yolu o olabilir.
	var cok []string
	for i := range 20 {
		cok = append(cok, fmt.Sprintf("/ip4/8.8.4.%d/tcp/4001", i+1))
	}
	cok = append(cok, roleAdresi("9.9.9.9"))
	secilen := adresleriSec(adresler(t, cok...))
	if len(secilen) != bilinenAdresSiniri || adresinTuru(secilen[len(secilen)-1]) != adresRole {
		t.Fatalf("sınırda %d adres, sonuncusu röle olmalı: %s", len(secilen), adresMetinleri(secilen))
	}
}

func TestAdresinTuru(t *testing.T) {
	role := rastgeleEs(t).String()
	for adres, beklenen := range map[string]adresTuru{
		acikAdres1:                        adresAcik,
		"/ip6/2a02:4780::1/udp/1/quic-v1": adresAcik,
		"/dns/ornek.org/tcp/443":          adresAcik,
		yerelAdres:                        adresYerel,
		"/ip4/10.20.30.40/tcp/1":          adresYerel,
		"/ip6/fd00::1/tcp/1":              adresYerel,
		"/ip4/127.0.0.1/tcp/1":            adresLoopback,
		"/ip6/::1/tcp/1":                  adresLoopback,
		"/ip4/169.254.1.1/tcp/1":          adresKullanilmaz,
		"/ip4/0.0.0.0/tcp/1":              adresKullanilmaz,
		"/ip4/9.9.9.1/tcp/1/p2p/" + role + "/p2p-circuit":  adresRole,
		"/ip4/203.0.113.7/tcp/1":                           adresKullanilmaz, // belgeleme aralığı: yönlendirilemez
		"/ip4/10.0.0.1/tcp/1/p2p/" + role + "/p2p-circuit": adresKullanilmaz,
	} {
		if got := adresinTuru(adresler(t, adres)[0]); got != beklenen {
			t.Errorf("adresinTuru(%s) = %d; %d beklenirdi", adres, got, beklenen)
		}
	}
	if adresinTuru(nil) != adresKullanilmaz {
		t.Error("nil adres kullanılmaz olmalı")
	}
}

// Bellekteki liste (veri dizini verilmemiş düğüm) diske hiçbir şey yazmaz.
func TestBilinen_BellekteKaydetmez(t *testing.T) {
	b := BilinenAc("")
	b.Gor(rastgeleEs(t), adresler(t, acikAdres1))
	if err := b.Kaydet(); err != nil {
		t.Fatal(err)
	}
}
