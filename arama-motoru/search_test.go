package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testPeerID(t *testing.T) peer.ID {
	t.Helper()
	pub, _ := genKey(t)
	lp2pPub, err := lp2pcrypto.UnmarshalEd25519PublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPublicKey(lp2pPub)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestTtlDenetle_SinirlarUygulanir(t *testing.T) {
	if err := ttlDenetle(aramaTTLMin); err != nil {
		t.Fatalf("alt sınır (%d) kabul edilmeliydi: %v", aramaTTLMin, err)
	}
	if err := ttlDenetle(aramaTTLMax); err != nil {
		t.Fatalf("üst sınır (%d) kabul edilmeliydi: %v", aramaTTLMax, err)
	}
	if err := ttlDenetle(aramaTTLMin - 1); err == nil {
		t.Fatal("alt sınırın altındaki bir değer reddedilmeliydi")
	}
	if err := ttlDenetle(aramaTTLMax + 1); err == nil {
		t.Fatal("üst sınırın üstündeki bir değer reddedilmeliydi")
	}
}

// TestRecoverGoroutine_PanigiYutar, recoverGoroutine ile korunan bir
// fonksiyonun paniklediğinde çağıran goroutine'i ÇÖKERTMEDEN akışın
// normal şekilde tamamlandığını doğrular — search.go'nun (ve main.go'nun)
// güvenilmeyen ağ içeriğini işleyen goroutine'lerinde kullandığı kalkan.
func TestRecoverGoroutine_PanigiYutar(t *testing.T) {
	tamamlandi := false
	func() {
		defer recoverGoroutine("test")
		defer func() { tamamlandi = true }()
		panic("test paniği")
	}()
	if !tamamlandi {
		t.Fatal("panik sonrası defer zinciri tamamlanmamış")
	}
}

func TestHizSiniri_LimitiUygular(t *testing.T) {
	h := yeniHizSiniri()
	pid := testPeerID(t)

	for i := 0; i < hizSiniriMax; i++ {
		if !h.izinVer(pid) {
			t.Fatalf("istek %d izin verilmeliydi", i+1)
		}
	}
	if h.izinVer(pid) {
		t.Fatal("limit aşıldıktan sonra izin verilmemeliydi")
	}
}

func TestHizSiniri_PencereDisindakilerSayilmaz(t *testing.T) {
	h := yeniHizSiniri()
	pid := testPeerID(t)
	h.gecmis[pid] = []time.Time{time.Now().Add(-2 * hizSiniriPenceresi)}
	if !h.izinVer(pid) {
		t.Fatal("pencere dışındaki eski istekler kotayı doldurmamalıydı")
	}
}

// TestAramaProtokolu_UctanUca, arama sunucusunu bir host'a bağlayıp diğer
// host'tan gerçek bir libp2p stream'i üzerinden sorgu gönderir — search.go
// tasarımının (DM değil, doğrudan stream; boyut sınırı yok) fiilen
// çalıştığını kanıtlar.
func TestAramaProtokolu_UctanUca(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub, priv := genKey(t)
	if err := depo.Ekle(gecerliKonuSonucu(t, pub, priv, "p1", "aranan başlık", "aranan gövde metni")); err != nil {
		t.Fatal(err)
	}

	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "aranan", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("beklenmeyen sonuçlar: %+v", sonuclar)
	}
	// Govde artık vurgu işaretleyicileri taşıyabilir (bkz. index.go'nun
	// highlight() kullanımı) — düz metne indirgeyip karşılaştır.
	if duzMetin(sonuclar[0].Govde) != "aranan gövde metni" {
		t.Fatalf("tam gövde taşınmalıydı (DM'in aksine kısaltma yok): %q", sonuclar[0].Govde)
	}
}

func TestAramaProtokolu_SonucYoksaBosDoner(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "hiçbirşeyeuymayansorgu", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 0 {
		t.Fatalf("boş sonuç bekleniyordu, %d geldi", len(sonuclar))
	}
}

// TestAramaProtokolu_HizSiniriniUygular, gerçek stream'ler üzerinden hız
// sınırının uygulandığını doğrular.
func TestAramaProtokolu_HizSiniriniUygular(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub, priv := genKey(t)
	if err := depo.Ekle(gecerliKonuSonucu(t, pub, priv, "p1", "aranan", "gövde")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	basarili := 0
	for i := 0; i < hizSiniriMax+5; i++ {
		sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "aranan", "", "")
		if err == nil && len(sonuclar) == 1 {
			basarili++
		}
	}
	if basarili != hizSiniriMax {
		t.Fatalf("hız sınırı (%d) sonrası istekler reddedilmeliydi, %d başarılı oldu", hizSiniriMax, basarili)
	}
}

// TestAramaProtokolu_CokAtlamaliYayilma, h1↔h2↔h3 zincirinde (h1 ile h3
// DOĞRUDAN bağlı DEĞİL — bkz. ucBagliHost) h1'in, yalnızca h3'ün kendi
// yerel indeksinde bulunan bir içeriği h2 ÜZERİNDEN İLETİLEREK
// bulabildiğini kanıtlar. Bu, search.go'nun "yalnızca doğrudan eşlere
// ulaşır" eski sınırlamasının artık geçerli olmadığının kanıtıdır.
func TestAramaProtokolu_CokAtlamaliYayilma(t *testing.T) {
	h1, h2, h3, ctx := ucBagliHost(t)

	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck

	depo3, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo3.Kapat() //nolint:errcheck
	pub, priv := genKey(t)
	if err := depo3.Ekle(gecerliKonuSonucu(t, pub, priv, "uzak1", "uzaktaki başlık", "yalnızca h3'te olan içerik")); err != nil {
		t.Fatal(err)
	}

	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())
	aramaSunucusuKaydet(ctx, h3, depo3, yeniHizSiniri(), yeniSorguOnbellek())

	// h1, YALNIZCA doğrudan bağlı olduğu h2'ye sorar — h3'e hiç doğrudan
	// bağlı değil (ucBagliHost'un zincir topolojisi). Yanıt yalnızca h2
	// İLETİMİ sayesinde h3'ün sonucunu içerebilir.
	sonuclar, err := aramaIstekleGonder(ctx, h1, h2.ID(), "uzaktaki", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "uzak1" {
		t.Fatalf("h3'teki içerik h2 üzerinden iletilerek bulunmalıydı: %+v", sonuclar)
	}
	if sonuclar[0].KaynakPeer != h3.ID().String() {
		t.Fatalf("sonucun gerçek kaynağı h3 olarak etiketlenmeliydi, geldi: %q", sonuclar[0].KaynakPeer)
	}
}

// TestAramaProtokolu_TTL1IletilmezYalnizcaYerelYanit, TTL=1 ile gönderilen
// bir sorgunun İLERİ İLETİLMEDİĞİNİ (yalnızca alan düğümün kendi yerel
// sonucuyla yanıt verdiğini) doğrular — çok atlamalı yayılmanın sınırlı
// olduğunu, sınırsız taşmadığını kanıtlar.
func TestAramaProtokolu_TTL1IletilmezYalnizcaYerelYanit(t *testing.T) {
	h1, h2, h3, ctx := ucBagliHost(t)

	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck

	depo3, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo3.Kapat() //nolint:errcheck
	if err := depo3.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "uzak1", Baslik: "uzaktaki başlık", Govde: "yalnızca h3'te olan içerik"}); err != nil {
		t.Fatal(err)
	}

	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())
	aramaSunucusuKaydet(ctx, h3, depo3, yeniHizSiniri(), yeniSorguOnbellek())

	istek := aramaIstek{Metin: "uzaktaki", SorguID: yeniSorguID(), TTL: 1, SonZaman: time.Now().Add(5 * time.Second)}
	sonuclar, err := aramaIstekGonderRaw(ctx, h1, h2.ID(), istek)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 0 {
		t.Fatalf("TTL=1 ile sorgu iletilmemeliydi, ama sonuç geldi: %+v", sonuclar)
	}
}

// TestAramaProtokolu_TipFiltresi, Tip alanı doluysa yalnızca o türdeki
// sonuçların döndüğünü doğrular.
func TestAramaProtokolu_TipFiltresi(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub1, priv1 := genKey(t)
	pub2, priv2 := genKey(t)
	if err := depo.Ekle(gecerliKonuSonucu(t, pub1, priv1, "k1", "ortak kelime konu", "gövde")); err != nil {
		t.Fatal(err)
	}
	if err := depo.Ekle(gecerliSohbetSonucu(t, pub2, priv2, "s1", "ortak kelime sohbet mesajı")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "ortak", TipSohbet, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "s1" {
		t.Fatalf("yalnızca sohbet türü beklenirdi: %+v", sonuclar)
	}
}

// TestAramaProtokolu_YazarFiltresi, aramaIstek.YazarKey'in gerçek bir
// stream üzerinden uçtan uca uygulandığını ve yanıttaki agSonucu.YazarKey'
// in taşındığını (bkz. model.go'nun "bu yazarın diğer içerikleri"
// özelliği — ağ sonuçları için de YazarKey gerekiyor) doğrular.
func TestAramaProtokolu_YazarFiltresi(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pubAli, privAli := genKey(t)
	pubVeli, privVeli := genKey(t)
	aliKey := hex.EncodeToString(pubAli)
	if err := depo.Ekle(gecerliKonuSonucu(t, pubAli, privAli, "k1", "ortak konu", "gövde")); err != nil {
		t.Fatal(err)
	}
	if err := depo.Ekle(gecerliKonuSonucu(t, pubVeli, privVeli, "k2", "ortak konu 2", "gövde 2")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "ortak", "", aliKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "k1" {
		t.Fatalf("yalnızca ali'nin sonucu beklenirdi: %+v", sonuclar)
	}
	if sonuclar[0].YazarKey != aliKey {
		t.Fatalf("yanıttaki YazarKey taşınmalıydı, got %q", sonuclar[0].YazarKey)
	}
}

// TestAramaProtokolu_BosSorguYazarFiltresiyleTumIcerigiListeler, ağ
// protokolünün de (yerel Depo.AraGelismis gibi) anahtar kelimesiz, yalnızca
// yazar filtresiyle çalışabildiğini doğrular.
func TestAramaProtokolu_BosSorguYazarFiltresiyleTumIcerigiListeler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pubAli, privAli := genKey(t)
	aliKey := hex.EncodeToString(pubAli)
	if err := depo.Ekle(gecerliKonuSonucu(t, pubAli, privAli, "k1", "başlık", "gövde")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h1, depo, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaIstekleGonder(ctx, h2, h1.ID(), "", "", aliKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "k1" {
		t.Fatalf("anahtar kelimesiz, yazar filtresiyle 1 sonuç bekleniyordu: %+v", sonuclar)
	}
}

// ─── dogrulaAgSonucu — güvenlik regresyon testleri ─────────────────────────────
//
// Bu testler, "AND Arama Motoru" ekosisteminin ağdan gelen arama
// sonuçlarına hiçbir ed25519 doğrulaması UYGULAMADIĞI (bir eşin sahte
// başlık/gövde/yazar ile meşru görünümlü, tıklanabilir bir sonuç enjekte
// edebildiği) güvenlik bulgusunun regresyonlarıdır.

func TestDogrulaAgSonucu_GecerliSonucKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	s := gecerliKonuSonucu(t, pub, priv, "k1", "gerçek başlık", "gerçek gövde")
	ag := agSonucu{
		Tip: s.Tip, KaynakID: s.KaynakID, Kategori: s.Kategori, Baslik: s.Baslik,
		Govde: s.Govde, YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS, Sig: s.Sig,
	}
	if !dogrulaAgSonucu(ag) {
		t.Fatal("gerçekten imzalanmış, hiç değiştirilmemiş bir sonuç kabul edilmeliydi")
	}
}

// TestDogrulaAgSonucu_UydurmaGovdeReddedilir, orijinal (SDK GEÇERLİ) bir
// imzayı KORUYUP yalnızca Govde'yi (ör. bir kimlik avı bağlantısıyla)
// değiştiren bir saldırının reddedildiğini doğrular — bu, denetimde
// bulunan asıl açığın tam olarak kendisidir: geçerli bir Sig, farklı bir
// içerikle eşleştirilerek meşru bir yazara sahte içerik atfedilebiliyordu.
func TestDogrulaAgSonucu_UydurmaGovdeReddedilir(t *testing.T) {
	pub, priv := genKey(t)
	s := gecerliKonuSonucu(t, pub, priv, "k1", "gerçek başlık", "gerçek gövde")
	ag := agSonucu{
		Tip: s.Tip, KaynakID: s.KaynakID, Kategori: s.Kategori, Baslik: s.Baslik,
		Govde: "buraya tıkla: http://kimlik-avi.example", // saldırgan tarafından değiştirildi
		YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS, Sig: s.Sig, // Sig GERÇEK ve GEÇERLİ kalır
	}
	if dogrulaAgSonucu(ag) {
		t.Fatal("geçerli bir imzayla eşleştirilmiş uydurma bir gövde reddedilmeliydi")
	}
}

// TestDogrulaAgSonucu_YanlisYazarAnahtariReddedilir, bir sonucun BAŞKA bir
// yazarın (gerçek, ama farklı) anahtarına atfedilmesinin reddedildiğini
// doğrular.
func TestDogrulaAgSonucu_YanlisYazarAnahtariReddedilir(t *testing.T) {
	pub, priv := genKey(t)
	baskaPub, _ := genKey(t)
	s := gecerliKonuSonucu(t, pub, priv, "k1", "başlık", "gövde")
	ag := agSonucu{
		Tip: s.Tip, KaynakID: s.KaynakID, Kategori: s.Kategori, Baslik: s.Baslik,
		Govde: s.Govde, YazarKey: hex.EncodeToString(baskaPub), OlusturmaTS: s.OlusturmaTS, Sig: s.Sig,
	}
	if dogrulaAgSonucu(ag) {
		t.Fatal("yanlış yazar anahtarına atfedilmiş bir sonuç reddedilmeliydi")
	}
}

func TestDogrulaAgSonucu_ImzasizSonucReddedilir(t *testing.T) {
	if dogrulaAgSonucu(agSonucu{Tip: TipKonu, KaynakID: "k1", Govde: "g", YazarKey: "abc"}) {
		t.Fatal("Sig boşsa sonuç reddedilmeliydi")
	}
}

// TestAramaProtokolu_UydurmaSonucAgUzerindenReddedilir, gerçek libp2p
// stream'i üzerinden UYDURULMUŞ (hiç imzalanmamış) bir arama sonucu
// gönderen kötü niyetli bir eşin yanıtının, aramaIstekGonderRaw'ın kendi
// doğrulama katmanında (bkz. dogrulaAgSonucu) sessizce elendiğini —
// çağıranın hiçbir zaman sahte içerik almadığını — kanıtlar.
func TestAramaProtokolu_UydurmaSonucAgUzerindenReddedilir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	h2.SetStreamHandler(aramaProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_ = json.NewEncoder(st).Encode(aramaYanit{Sonuclar: []agSonucu{
			{
				Tip: TipKonu, KaynakID: "sahte1", Baslik: "ÖNEMLİ DUYURU",
				Govde: "buraya tıkla: http://kimlik-avi.example", YazarAd: "Moderator",
				YazarKey: "0000000000000000000000000000000000000000000000000000000000000000",
				Sig:      "deadbeef",
			},
		}})
	})

	sonuclar, err := aramaIstekleGonder(ctx, h1, h2.ID(), "duyuru", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 0 {
		t.Fatalf("uydurma sonuç ELENMELİYDİ, ama geçti: %+v", sonuclar)
	}
}

// ─── Hedefli P2P sorgu (aramaTekEsIstegi) ──────────────────────────────────

// TestAramaTekEsIstegi_TTL1IletilmezSadeceHedefinKendiSonucu, h1'in h2'yi
// TEK hedef olarak (TTL=1) sorguladığında, h2'nin kendi zincirdeki eşi
// h3'e İLETMEDEN yalnızca KENDİ yerel sonucuyla yanıt verdiğini doğrular —
// bkz. aramaTekEsIstegi'nin doc yorumu.
func TestAramaTekEsIstegi_TTL1IletilmezSadeceHedefinKendiSonucu(t *testing.T) {
	h1, h2, h3, ctx := ucBagliHost(t)

	depo2, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo2.Kapat() //nolint:errcheck
	depo3, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo3.Kapat() //nolint:errcheck

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := depo2.Ekle(gecerliKonuSonucu(t, pub, priv, "h2icerik", "b", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	if err := depo3.Ekle(gecerliKonuSonucu(t, pub, priv, "h3icerik", "b", "aranan metin")); err != nil {
		t.Fatal(err)
	}

	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())
	aramaSunucusuKaydet(ctx, h3, depo3, yeniHizSiniri(), yeniSorguOnbellek())

	sonuclar, err := aramaTekEsIstegi(ctx, h1, h2.ID().String(), "aranan", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "h2icerik" {
		t.Fatalf("TTL=1 ile yalnızca hedefin (h2) KENDİ sonucu dönmeliydi, h3'e iletilmemeliydi: %+v", sonuclar)
	}
	if sonuclar[0].KaynakPeer != h2.ID().String() {
		t.Fatalf("KaynakPeer hedeflenen eş (h2) olmalıydı: %+v", sonuclar[0])
	}
}

func TestAramaTekEsIstegi_GecersizPeerIDHataDoner(t *testing.T) {
	h := testHost(t)
	if _, err := aramaTekEsIstegi(context.Background(), h, "gecerli-olmayan-bir-id", "sorgu", "", ""); err == nil {
		t.Fatal("geçersiz peer ID için bir hata bekleniyordu")
	}
}

// ─── Eş güven göstergesi (esGuvenDefteri) ──────────────────────────────────

func TestEsGuvenDefteri_KaydetBirikirVeEnCokEtkilesimliOndeSiralanir(t *testing.T) {
	e := yeniEsGuvenDefteri()
	p1 := testPeerID(t)
	p2 := testPeerID(t)

	e.kaydet(p1, 3, 1)
	e.kaydet(p1, 2, 0) // aynı eş için ikinci kayıt BİRİKMELİ
	e.kaydet(p2, 1, 0)

	liste := e.enIyiListele(5)
	if len(liste) != 2 {
		t.Fatalf("2 eş bekleniyordu: %+v", liste)
	}
	if liste[0].Peer != p1 || liste[0].Gecerli != 5 || liste[0].Gecersiz != 1 {
		t.Fatalf("en çok etkileşimli eş (p1, toplam 6) önde ve BİRİKMİŞ sayaçlarla olmalıydı: %+v", liste)
	}
	if liste[1].Peer != p2 {
		t.Fatalf("p2 ikinci sırada olmalıydı: %+v", liste)
	}
}

func TestEsGuvenDefteri_SifirGecerliSifirGecersizKaydaGirmez(t *testing.T) {
	e := yeniEsGuvenDefteri()
	e.kaydet(testPeerID(t), 0, 0)
	if liste := e.enIyiListele(5); len(liste) != 0 {
		t.Fatalf("0/0'lık bir çağrı hiçbir sayaç oluşturmamalıydı: %+v", liste)
	}
}

func TestEsGuvenDefteri_EnIyiListeleLimitiUygular(t *testing.T) {
	e := yeniEsGuvenDefteri()
	for i := 0; i < 5; i++ {
		e.kaydet(testPeerID(t), 1, 0)
	}
	if liste := e.enIyiListele(3); len(liste) != 3 {
		t.Fatalf("limit=3 bekleniyordu, %d geldi", len(liste))
	}
}

// TestAramaIstekGonderRaw_EsGuvenDefteriVarsayilaniGuncellenir, GERÇEK bir
// ağ round-trip'inin (aramaIstekGonderRaw'ın TEK boğaz noktası) süreç
// genelindeki paylaşılan defteri (esGuvenDefteriVarsayilan) fiilen
// güncellediğini doğrular — testHost'un her çağrıda YENİ/rastgele bir peer
// kimliği ürettiği için (bkz. testHost'un kimlikYukleYaOlustur'u) h2.ID()
// ile filtrelemek, aynı süreçte çalışan diğer testlerin bu PAYLAŞILAN
// deftere yazdığı verilerle çakışma riski taşımaz.
func TestAramaIstekGonderRaw_EsGuvenDefteriVarsayilaniGuncellenir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub, priv := genKey(t)
	if err := depo.Ekle(gecerliKonuSonucu(t, pub, priv, "p1", "b", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	aramaSunucusuKaydet(ctx, h2, depo, yeniHizSiniri(), yeniSorguOnbellek())

	if _, err := aramaIstekleGonder(ctx, h1, h2.ID(), "aranan", "", ""); err != nil {
		t.Fatal(err)
	}

	var bulundu *EsGuvenOzet
	for _, e := range esGuvenDefteriVarsayilan.enIyiListele(1000) {
		if e.Peer == h2.ID() {
			e := e
			bulundu = &e
		}
	}
	if bulundu == nil || bulundu.Gecerli < 1 {
		t.Fatalf("h2 için en az 1 geçerli kayıt bekleniyordu: %+v", bulundu)
	}
}
