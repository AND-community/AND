package network

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
	ma "github.com/multiformats/go-multiaddr"
)

// imzaliKayit, anahtar ile imzalanmış ve kimlik'i anlatan bir adres kaydı üretir.
func imzaliKayit(t testing.TB, anahtar lp2pcrypto.PrivKey, kimlik peer.ID, adr ...string) []byte {
	t.Helper()
	rec := peer.NewPeerRecord()
	rec.PeerID = kimlik
	for _, s := range adr {
		a, err := ma.NewMultiaddr(s)
		if err != nil {
			t.Fatal(err)
		}
		rec.Addrs = append(rec.Addrs, a)
	}
	zarf, err := record.Seal(rec, anahtar)
	if err != nil {
		t.Fatal(err)
	}
	b, err := zarf.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func yeniAnahtar(t testing.TB) (lp2pcrypto.PrivKey, peer.ID) {
	t.Helper()
	priv, pub, err := lp2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return priv, id
}

// Tek bir AND düğümüne bağlanan, onun bağlı olduğu öteki AND düğümlerini
// imzalı kayıtlarıyla öğrenir.
func TestEsDegisimi_UcuncuDugumuOgrenir(t *testing.T) {
	a, b, c := andDugumu(t), andDugumu(t), andDugumu(t)
	genel := duzDugum(t)
	bagla(t, c, b)
	bagla(t, genel, b)
	bagla(t, a, b)
	bekle(t, 5*time.Second, func() bool { return b.Esler.Say() == 2 }, "B iki AND eşini tanımalı")

	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	gelen, err := a.EsleriIste(ctx, b.Host.ID())
	if err != nil {
		t.Fatal(err)
	}
	// Yalnızca C: soranın kendisi (A) ve AND olmayan düğüm verilmez.
	if len(gelen) != 1 || gelen[0].ID != c.Host.ID() {
		t.Fatalf("gelen eşler %+v; yalnızca C beklenirdi", gelen)
	}
	if len(gelen[0].Addrs) == 0 {
		t.Fatal("C'nin adresi gelmeli (yanıtlayan aynı makinede: loopback adresi kabul edilir)")
	}
	// Öğrenilen adresle C'ye bağlanılabilir.
	if err := a.Host.Connect(ctx, gelen[0]); err != nil {
		t.Fatalf("öğrenilen adresle bağlanılamadı: %v", err)
	}
	// Öğrenilen eş, bağlanılıp AND düğümü olduğu görülmeden bilinenlere girmez…
	bekle(t, 5*time.Second, func() bool { return a.Esler.Say() == 2 }, "A, C'yi de tanımalı")
	// …bağlanılınca girer.
	bekle(t, 5*time.Second, func() bool { return len(a.Bilinen.Adresler(c.Host.ID())) > 0 }, "bağlanılan C bilinenlere yazılmalı")
}

// Aynı eş art arda sorarsa geri çevrilir.
func TestEsDegisimi_SikSoranGeriCevrilir(t *testing.T) {
	a, b := andDugumu(t), andDugumu(t)
	bagla(t, a, b)
	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	if _, err := a.EsleriIste(ctx, b.Host.ID()); err != nil {
		t.Fatalf("ilk istek: %v", err)
	}
	if _, err := a.EsleriIste(ctx, b.Host.ID()); err == nil {
		t.Fatal("hemen yinelenen istek geri çevrilmeli")
	}
}

func TestDegisimSiniri(t *testing.T) {
	var s degisimSiniri
	p, q := rastgeleEs(t), rastgeleEs(t)
	an := time.Unix(1_000_000, 0)
	if !s.izinVer(p, an) || !s.izinVer(q, an) {
		t.Fatal("ilk istekler kabul edilmeli")
	}
	if s.izinVer(p, an.Add(degisimEnAzAralik-time.Second)) {
		t.Fatal("aralık dolmadan ikinci istek kabul edilmemeli")
	}
	if !s.izinVer(p, an.Add(degisimEnAzAralik)) {
		t.Fatal("aralık dolunca kabul edilmeli")
	}
	// İzlenen eş sayısı sınırlıdır: tablo dolunca süresi geçenler atılır,
	// hepsi tazeyse yeni eş geri çevrilir (bellek büyümez).
	s = degisimSiniri{}
	for range degisimIzlenenEs {
		if !s.izinVer(rastgeleEs(t), an) {
			t.Fatal("sınıra kadar kabul edilmeli")
		}
	}
	if s.izinVer(rastgeleEs(t), an.Add(time.Second)) {
		t.Fatal("tablo tazeyken doluysa yeni eş geri çevrilmeli")
	}
	if !s.izinVer(rastgeleEs(t), an.Add(degisimEnAzAralik)) || len(s.son) != 1 {
		t.Fatalf("süresi geçenler atılıp yeni eş kabul edilmeli (tablo %d)", len(s.son))
	}
}

// Sunucu, istenen sayıyı kendi sınırıyla keser ve bozuk isteği yanıtlamaz.
func TestEsDegisimi_SunucuSinirlari(t *testing.T) {
	b := andDugumu(t)
	// B'ye sınırdan fazla "bağlı AND eşi" kaydı ver.
	b.Esler.mu.Lock()
	for range degisimEnFazlaKayit + 10 {
		anahtar, kimlik := yeniAnahtar(t)
		b.Esler.esler[kimlik] = &esKaydi{bilgi: EsBilgisi{ID: kimlik}, kayit: imzaliKayit(t, anahtar, kimlik, "/ip4/1.2.3.4/tcp/4001")}
	}
	b.Esler.mu.Unlock()

	iste := func(t *testing.T, istek string) (degisimYaniti, error) {
		t.Helper()
		a := duzDugum(t)
		bagla(t, a, b)
		ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
		defer iptal()
		s, err := a.Host.NewStream(ctx, b.Host.ID(), EsDegisimProtokolu)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		_ = s.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := io.WriteString(s, istek); err != nil {
			t.Fatal(err)
		}
		_ = s.CloseWrite()
		var y degisimYaniti
		err = json.NewDecoder(s).Decode(&y)
		return y, err
	}

	for _, istek := range []string{`{"v":1}`, `{"v":1,"n":100000}`, `{"v":1,"n":-3}`} {
		y, err := iste(t, istek)
		if err != nil || len(y.Esler) != degisimEnFazlaKayit || y.V != degisimSurumu {
			t.Errorf("istek %s: %d kayıt (hata %v); %d beklenirdi", istek, len(y.Esler), err, degisimEnFazlaKayit)
		}
	}
	if y, err := iste(t, `{"v":1,"n":3}`); err != nil || len(y.Esler) != 3 {
		t.Errorf("n=3: %d kayıt (hata %v)", len(y.Esler), err)
	}
	// Bilinmeyen alanlar yok sayılır (ileri uyumluluk).
	if y, err := iste(t, `{"v":2,"n":2,"yeni_alan":"x"}`); err != nil || len(y.Esler) != 2 {
		t.Errorf("bilinmeyen alanlı istek: %d kayıt (hata %v)", len(y.Esler), err)
	}
	if _, err := iste(t, `bu json değil`); err == nil {
		t.Error("bozuk istek yanıtlanmamalı")
	}
}

// Yanıttaki kayıtlar doğrulanır: imzası tutmayan, başkası adına imzalanmış,
// bozuk, kendimize ait ya da çevrilemeyen kayıtlar atılır.
func TestKayitlariCoz(t *testing.T) {
	_, kendim := yeniAnahtar(t)
	anahtar1, es1 := yeniAnahtar(t)
	anahtar2, _ := yeniAnahtar(t)
	anahtar3, es3 := yeniAnahtar(t)

	gecerli := imzaliKayit(t, anahtar1, es1, "/ip4/1.2.3.4/tcp/4001", "/ip4/192.168.1.9/tcp/4001", "/ip4/127.0.0.1/tcp/4001", "/ip4/169.254.1.1/tcp/1")
	// Başka bir anahtarla imzalanmış ama es3'ü anlatan kayıt: başkası adına adres.
	sahte := imzaliKayit(t, anahtar2, es3, "/ip4/6.6.6.6/tcp/1")
	bozulmus := append([]byte(nil), gecerli...)
	bozulmus[len(bozulmus)-3] ^= 0xFF
	yalnizYerel := imzaliKayit(t, anahtar3, es3, "/ip4/10.0.0.5/tcp/4001")

	kayitlar := [][]byte{
		nil, []byte("çöp"), sahte, bozulmus,
		gecerli, gecerli, // aynı düğüm iki kez
		yalnizYerel,
		make([]byte, degisimKayitSiniri+1),
	}

	// İnternetteki bir eşten: yalnızca açık adres.
	acik := kayitlariCoz(kayitlar, kendim, yalnizAcik)
	if len(acik) != 1 || acik[0].ID != es1 || adresMetinleri(acik[0].Addrs) != "/ip4/1.2.3.4/tcp/4001" {
		t.Fatalf("yalnizAcik: %+v", acik)
	}
	// Yerel ağdaki bir eşten: özel adresler de; loopback yine değil.
	yerel := kayitlariCoz(kayitlar, kendim, yerelDahil)
	if len(yerel) != 2 || adresMetinleri(yerel[0].Addrs) != "/ip4/1.2.3.4/tcp/4001 /ip4/192.168.1.9/tcp/4001" || yerel[1].ID != es3 {
		t.Fatalf("yerelDahil: %+v", yerel)
	}
	// Aynı makinedeki bir eşten: loopback de; bağlantı-yerel adres hiçbir zaman.
	hepsi := kayitlariCoz(kayitlar, kendim, loopbackDahil)
	if len(hepsi) != 2 || len(hepsi[0].Addrs) != 3 {
		t.Fatalf("loopbackDahil: %+v", hepsi)
	}
	// Kendi kaydımız atılır.
	if got := kayitlariCoz([][]byte{gecerli}, es1, loopbackDahil); len(got) != 0 {
		t.Fatalf("kendi kaydımız dönmemeli: %+v", got)
	}

	// Sınırdan fazla kayıt işlenmez.
	var cok [][]byte
	for range degisimEnFazlaKayit + 5 {
		a, id := yeniAnahtar(t)
		cok = append(cok, imzaliKayit(t, a, id, "/ip4/1.2.3.4/tcp/4001"))
	}
	if got := kayitlariCoz(cok, kendim, yalnizAcik); len(got) != degisimEnFazlaKayit {
		t.Fatalf("%d kayıt işlendi; sınır %d", len(got), degisimEnFazlaKayit)
	}
	// Bir kayıttan en fazla degisimAdresSiniri adres kullanılır.
	var adr []string
	for i := range 40 {
		adr = append(adr, "/ip4/1.2.3.4/tcp/"+string(rune('0'+i%10))+"000")
	}
	a4, id4 := yeniAnahtar(t)
	if got := kayitlariCoz([][]byte{imzaliKayit(t, a4, id4, adr...)}, kendim, yalnizAcik); len(got) != 1 || len(got[0].Addrs) > degisimAdresSiniri {
		t.Fatalf("adres sınırı aşıldı: %+v", got)
	}
}

// FuzzDegisimYaniti: bir eşin gönderdiği yanıt ne olursa olsun düğüm
// çökmez ve doğrulanmamış bir kayıt dışarı çıkmaz.
func FuzzDegisimYaniti(f *testing.F) {
	anahtar, id := yeniAnahtar(f)
	gecerli := imzaliKayit(f, anahtar, id, "/ip4/1.2.3.4/tcp/4001")
	iyi, _ := json.Marshal(degisimYaniti{V: 1, Esler: [][]byte{gecerli}})
	f.Add(iyi)
	f.Add([]byte(`{"v":1,"esler":["AAAA",""]}`))
	f.Add([]byte(`{"esler":null}`))
	f.Add([]byte(`[]`))
	f.Add(gecerli)
	_, kendim := yeniAnahtar(f)
	f.Fuzz(func(t *testing.T, veri []byte) {
		var y degisimYaniti
		if json.Unmarshal(veri, &y) == nil {
			for _, ai := range kayitlariCoz(y.Esler, kendim, loopbackDahil) {
				if ai.ID == "" || ai.ID == kendim || len(ai.Addrs) == 0 || len(ai.Addrs) > degisimAdresSiniri {
					t.Fatalf("geçersiz sonuç: %+v", ai)
				}
			}
		}
		// Ham baytlar doğrudan kayıt olarak da denenir.
		kayitlariCoz([][]byte{veri}, kendim, yalnizAcik)
	})
}
