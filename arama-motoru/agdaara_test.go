package main

// agdaara_test.go — model.go'nun agdaAraCmd'sinin (ctrl+n "ağda ara" 'nın
// model katmanındaki fan-out/tekilleştirme mantığı) testleri. search.go'nun
// KENDİ protokolü zaten search_test.go'da uçtan uca test edilmiş; buradaki
// testler o protokolü SARAN model.go kodunun (birden fazla eşe paralel
// gönderme, kaynak_id'ye göre tekilleştirme, KaynakPeer geri düşümü)
// DOĞRU çalıştığını doğrular — bkz. denetim (agdaAraCmd %0 kapsamdaydı).

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// gecerliKonuSonucu, aramaSunucusuKaydet üzerinden gerçekten ağa
// SERVİS EDİLEBİLECEK (dogrulaAgSonucu'yu geçen) imzalı bir AramaSonucu
// üretir — bkz. search.go'nun dogrulaAgSonucu'sunun doc yorumu: Sig'siz ya
// da alanları (Baslik/Govde/YazarKey/OlusturmaTS) imzalanan payload'la
// tutarsız bir sonuç artık "ağda ara" yanıtında asla görünmez, bu yüzden bu
// paketteki testlerin (forum_test.go'daki signedPost'un aksine, o yalnızca
// forumMesajIsle'ın kendi doğrulama yolunu test eder) network round-trip'ini
// sınayan sabit fikstürleri GERÇEK bir imzayla üretmesi gerekir. Anahtar
// çifti çağıran tarafından verilir ki testler birden fazla sonucu AYNI ya da
// FARKLI yazarla üretebilsin (bkz. YazarFiltresi testleri).
func gecerliKonuSonucu(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, id, baslik, govde string) AramaSonucu {
	t.Helper()
	p := &Post{
		ID: id, Category: "genel", AuthorKey: hex.EncodeToString(pub),
		AuthorName: "test", Title: baslik, Body: govde, CreatedAt: time.Now().UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, postPayload(p)))
	return AramaSonucu{
		Tip: TipKonu, KaynakID: p.ID, Kategori: p.Category, Baslik: p.Title,
		Govde: p.Body, YazarAd: p.AuthorName, YazarKey: p.AuthorKey, OlusturmaTS: p.CreatedAt,
		Sig: p.Sig,
	}
}

// gecerliSohbetSonucu, gecerliKonuSonucu'nun TipSohbet karşılığıdır.
func gecerliSohbetSonucu(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, id, govde string) AramaSonucu {
	t.Helper()
	m := &ChatMessage{
		ID: id, AuthorKey: hex.EncodeToString(pub), AuthorName: "test", Text: govde, SentAt: time.Now().UTC(),
	}
	m.Sig = hex.EncodeToString(ed25519.Sign(priv, chatPayload(m)))
	return AramaSonucu{
		Tip: TipSohbet, KaynakID: m.ID, Govde: m.Text, YazarAd: m.AuthorName, YazarKey: m.AuthorKey,
		OlusturmaTS: m.SentAt, Sig: m.Sig,
	}
}

func TestAgdaAraCmd_BagliEsYokHataDoner(t *testing.T) {
	h := testHost(t)
	ctx := context.Background()

	msg := agdaAraCmd(ctx, h, "sorgu", "", "")()
	m, ok := msg.(agAramaSonucuMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agAramaSonucuMsg, got %T", msg)
	}
	if m.err == nil {
		t.Fatal("bağlı eş yokken bir hata bekleniyordu")
	}
}

// ucYildizHost, h1'i HEM h2 HEM h3'e DOĞRUDAN bağlayan bir yıldız
// topolojisi kurar (network_test.go'nun ucBagliHost'undaki zincirin
// aksine — agdaAraCmd yalnızca DOĞRUDAN bağlı eşlere (h.Network().Peers())
// fan-out yaptığından, h1'in test edilebilmesi için ikisine de doğrudan
// bağlı olması gerekir).
func ucYildizHost(t *testing.T) (h1, h2, h3 host.Host, ctx context.Context) {
	t.Helper()
	h1 = testHost(t)
	h2 = testHost(t)
	h3 = testHost(t)

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("h1-h2 bağlanamadı: %v", err)
	}
	if err := h1.Connect(ctx, peer.AddrInfo{ID: h3.ID(), Addrs: h3.Addrs()}); err != nil {
		t.Fatalf("h1-h3 bağlanamadı: %v", err)
	}
	return h1, h2, h3, ctx
}

func TestAgdaAraCmd_BirdenFazlaEsdenSonucBirlestirirVeTekillestirir(t *testing.T) {
	h1, h2, h3, ctx := ucYildizHost(t)

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

	// "ortak" kaynak_id'si HER İKİ eşte de var (senkronize olmuş gibi) —
	// tekilleştirmenin bunu İKİ KEZ değil BİR KEZ döndürmesi gerekir.
	// "yalnizca2"/"yalnizca3" ise yalnızca kendi eşlerinde.
	if err := depo2.Ekle(gecerliKonuSonucu(t, pub, priv, "ortak", "ortak sonuç", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	if err := depo2.Ekle(gecerliKonuSonucu(t, pub, priv, "yalnizca2", "yalnızca ikide", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	if err := depo3.Ekle(gecerliKonuSonucu(t, pub, priv, "ortak", "ortak sonuç", "aranan metin")); err != nil {
		t.Fatal(err)
	}
	if err := depo3.Ekle(gecerliKonuSonucu(t, pub, priv, "yalnizca3", "yalnızca üçte", "aranan metin")); err != nil {
		t.Fatal(err)
	}

	aramaSunucusuKaydet(ctx, h2, depo2, yeniHizSiniri(), yeniSorguOnbellek())
	aramaSunucusuKaydet(ctx, h3, depo3, yeniHizSiniri(), yeniSorguOnbellek())

	aramaProtokoluBekle(t, h1, h2.ID(), h3.ID())

	msg := agdaAraCmd(ctx, h1, "aranan", "", "")()
	m, ok := msg.(agAramaSonucuMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agAramaSonucuMsg, got %T", msg)
	}
	if m.err != nil {
		t.Fatalf("hata beklenmiyordu: %v", m.err)
	}
	if len(m.sonuclar) != 3 {
		t.Fatalf("tekilleştirme sonrası 3 benzersiz sonuç bekleniyordu, %d geldi: %+v", len(m.sonuclar), m.sonuclar)
	}
	gorulen := map[string]bool{}
	for _, s := range m.sonuclar {
		if gorulen[s.KaynakID] {
			t.Fatalf("kaynak_id tekrarlanmamalıydı: %q", s.KaynakID)
		}
		gorulen[s.KaynakID] = true
		if s.KaynakPeer == "" {
			t.Fatalf("KaynakPeer boş olmamalıydı: %+v", s)
		}
	}
	if !gorulen["ortak"] || !gorulen["yalnizca2"] || !gorulen["yalnizca3"] {
		t.Fatalf("beklenen üç kaynak_id de bulunmalıydı: %+v", gorulen)
	}
}

// TestAgdaAraCmd_BosKaynakPeerGonderenPeerIleDoldurulur, agSonucu.KaynakPeer
// boş geldiğinde (aramaSunucusuKaydet'in normal yanıtları BUNU HİÇBİR ZAMAN
// yapmaz — kendi ID'sini her zaman doldurur; bu yalnızca savunma amaçlı bir
// dal) agdaAraCmd'nin onu YANITLAYAN eşin ID'siyle doldurduğunu doğrular.
// Bunu tetiklemek için gerçek aramaSunucusuKaydet yerine KASITLI OLARAK
// eksik bir yanıt döndüren minimal bir sahte sunucu kullanılır.
func TestAgdaAraCmd_BosKaynakPeerGonderenPeerIleDoldurulur(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := gecerliKonuSonucu(t, pub, priv, "k1", "b", "g")

	h2.SetStreamHandler(aramaProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_ = json.NewEncoder(st).Encode(aramaYanit{Sonuclar: []agSonucu{
			{
				Tip: s.Tip, KaynakID: s.KaynakID, Kategori: s.Kategori, Baslik: s.Baslik, Govde: s.Govde,
				YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS, Sig: s.Sig, KaynakPeer: "",
			},
		}})
	})

	aramaProtokoluBekle(t, h1, h2.ID())

	msg := agdaAraCmd(ctx, h1, "sorgu", "", "")()
	m, ok := msg.(agAramaSonucuMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi agAramaSonucuMsg, got %T", msg)
	}
	if len(m.sonuclar) != 1 {
		t.Fatalf("1 sonuç bekleniyordu, %d geldi", len(m.sonuclar))
	}
	if m.sonuclar[0].KaynakPeer != h2.ID().String() {
		t.Fatalf("boş KaynakPeer, yanıtlayan eşin ID'siyle doldurulmalıydı: got %q, want %q", m.sonuclar[0].KaynakPeer, h2.ID().String())
	}
}
