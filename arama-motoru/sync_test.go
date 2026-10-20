package main

// sync_test.go — forumIleSenkronizeOl/chatIleSenkronizeOl'un CLIENT
// tarafını, o protokolü konuşan sahte bir "AND eşi" (bu testte elle
// yazılmış bir stream handler) karşısında uçtan uca test eder.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
)

func TestForumIleSenkronizeOl_GecerliPostlariIndeksler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)

	// h1'i, gerçek bir AND düğümünün forum sync sunucusu gibi davranacak
	// şekilde ayarla.
	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req forumSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		resp := forumSyncYanit{Posts: []*Post{p}, Replies: map[string][]*Reply{}}
		_ = json.NewEncoder(st).Encode(resp)
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := depo.Ara("başlık", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("sync'ten gelen geçerli post indekslenmedi: %+v", sonuclar)
	}
}

func TestForumIleSenkronizeOl_GecersizImzaliPostuAtlar(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	p.Body = "kurcalanmış" // imzadan sonra değiştirildi — kötü niyetli/bozuk bir eşi simüle eder

	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req forumSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(forumSyncYanit{Posts: []*Post{p}, Replies: map[string][]*Reply{}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	if n, _ := depo.SayimAl(); n != 0 {
		t.Fatalf("geçersiz imzalı sync sonucu indekslenmemeliydi, sayı=%d", n)
	}
}

func TestChatIleSenkronizeOl_GecerliMesajlariIndeksler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	m := signedChat(t, priv, pub)

	h1.SetStreamHandler(chatSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req chatSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(chatSyncYanit{Messages: []*ChatMessage{m}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := chatIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := depo.Ara("merhaba", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].Tip != TipSohbet {
		t.Fatalf("sync'ten gelen geçerli sohbet mesajı indekslenmedi: %+v", sonuclar)
	}
}

func TestForumIleSenkronizeOl_BilinenIDleriGonderir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	if err := depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "zaten-bilinen", Baslik: "x", Govde: "y"}); err != nil {
		t.Fatal(err)
	}

	var alinanIstek forumSyncIstek
	istekAlindi := make(chan struct{})
	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_ = json.NewDecoder(st).Decode(&alinanIstek)
		close(istekAlindi)
		_ = json.NewEncoder(st).Encode(forumSyncYanit{})
	})

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	<-istekAlindi

	if len(alinanIstek.PostIDs) != 1 || alinanIstek.PostIDs[0] != "zaten-bilinen" {
		t.Fatalf("bilinen ID listesi doğru gönderilmedi: %+v", alinanIstek.PostIDs)
	}
}

func TestSiteIleSenkronizeOl_GecerliSayfalariIndeksler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "Senkron Başlık", "Senkron İçerik", time.Now())

	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req siteSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(siteSyncYanit{Pages: []*SitePage{p}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := depo.Ara("senkron", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].Tip != TipSite {
		t.Fatalf("sync'ten gelen geçerli site sayfası indekslenmedi: %+v", sonuclar)
	}
}

func TestSiteIleSenkronizeOl_GecersizImzaliSayfayiAtlar(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	p := signedSitePage(t, priv, pub, "home", "t", "orijinal", time.Now())
	p.Body = "kurcalanmış"

	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req siteSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(siteSyncYanit{Pages: []*SitePage{p}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	if n, _ := depo.SayimAl(); n != 0 {
		t.Fatalf("geçersiz imzalı sync sonucu indekslenmemeliydi, sayı=%d", n)
	}
}

// TestSiteIleSenkronizeOl_TombstoneUygular, sync yanıtındaki bir
// site.DeletePage'in yerel olarak zaten bilinen bir sayfayı sildiğini
// doğrular.
func TestSiteIleSenkronizeOl_TombstoneUygular(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	del := signedSiteDelete(t, priv, pub, "home", time.Now())

	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req siteSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(siteSyncYanit{Tombstones: []*siteDeleteMsg{del}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	kaynakID := siteKaynakID(hex.EncodeToString(pub), "home")
	if err := depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: kaynakID, Baslik: "t", Govde: "g", YazarKey: hex.EncodeToString(pub)}); err != nil {
		t.Fatal(err)
	}

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	if n, _ := depo.SayimAl(); n != 0 {
		t.Fatalf("sync'ten gelen tombstone yerel sayfayı silmeliydi, kalan kayıt=%d", n)
	}
}

// TestSiteIleSenkronizeOl_BilinenAktifSlugYazaraGoreGruplanir, giden
// isteğin Known alanının (yazar_key -> slug listesi) AND'ın kendi
// site/sync.go'suyla AYNI biçimde, yazara göre gruplanmış olarak
// gönderildiğini doğrular.
func TestSiteIleSenkronizeOl_BilinenAktifSlugYazaraGoreGruplanir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)
	if err := depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: siteKaynakID(authorKey, "home"), Baslik: "x", Govde: "y", YazarKey: authorKey}); err != nil {
		t.Fatal(err)
	}
	if err := depo.Ekle(AramaSonucu{Tip: TipSite, KaynakID: siteKaynakID(authorKey, "projeler"), Baslik: "x2", Govde: "y2", YazarKey: authorKey}); err != nil {
		t.Fatal(err)
	}

	var alinanIstek siteSyncIstek
	istekAlindi := make(chan struct{})
	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_ = json.NewDecoder(st).Decode(&alinanIstek)
		close(istekAlindi)
		_ = json.NewEncoder(st).Encode(siteSyncYanit{})
	})

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	<-istekAlindi

	slugs := alinanIstek.Known[authorKey]
	if len(slugs) != 2 {
		t.Fatalf("yazarın 2 slug'ı da Known altında gruplanmalıydı: %+v", alinanIstek.Known)
	}
}

// TestForumIleSenkronizeOl_GecerliYanitlariIndeksler, sync yanıtındaki
// Replies map'inde gelen GEÇERLİ imzalı bir yanıtın indekslendiğini
// doğrular — önceki testler yalnızca Posts'u kapsıyordu (bkz. denetim).
func TestForumIleSenkronizeOl_GecerliYanitlariIndeksler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	r := signedReply(t, priv, pub)

	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req forumSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(forumSyncYanit{Replies: map[string][]*Reply{"p1": {r}}})
	})

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	sonuclar, err := depo.Ara("yanıt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].UstID != "p1" {
		t.Fatalf("sync'ten gelen geçerli yanıt indekslenmedi: %+v", sonuclar)
	}
}

// TestForumIleSenkronizeOl_TombstoneUygular, sync yanıtındaki Tombstones
// listesinde (her biri JSON-kodlanmış bir deleteMsg dizgesi) gelen
// GEÇERLİ imzalı bir silme isteğinin yerel bir postu sildiğini doğrular.
func TestForumIleSenkronizeOl_TombstoneUygular(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	if err := depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "silinecek", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	del := &deleteMsg{PostID: "p1", AuthorKey: hex.EncodeToString(pub), DeletedAt: time.Now().UTC()}
	del.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(del)))
	delJSON, err := json.Marshal(del)
	if err != nil {
		t.Fatal(err)
	}

	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req forumSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		_ = json.NewEncoder(st).Encode(forumSyncYanit{Tombstones: []string{string(delJSON)}})
	})

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	if n, _ := depo.SayimAl(); n != 0 {
		t.Fatalf("sync'ten gelen tombstone yerel postu silmeliydi, kalan=%d", n)
	}
}

// ─── Bağlantı/kod çözme hataları (üç protokol için de aynı desen) ──────────

func TestForumIleSenkronizeOl_BagliOlmayanEseStreamAcilamaz(t *testing.T) {
	h2 := testHost(t)
	h3 := testHost(t) // h2'ye HİÇ bağlanmadı, h2 onu nasıl bulacağını bilmiyor
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := forumIleSenkronizeOl(context.Background(), h2, h3.ID(), depo); err == nil {
		t.Fatal("bağlı olmayan bir eşe sync denemesi hata dönmeliydi")
	}
}

func TestForumIleSenkronizeOl_BozukYanitHataDoner(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)
	h1.SetStreamHandler(forumSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_, _ = st.Write([]byte("bu geçerli bir json değil"))
	})
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := forumIleSenkronizeOl(ctx, h2, h1.ID(), depo); err == nil {
		t.Fatal("bozuk JSON yanıtı bir hata döndürmeliydi")
	}
}

func TestChatIleSenkronizeOl_BagliOlmayanEseStreamAcilamaz(t *testing.T) {
	h2 := testHost(t)
	h3 := testHost(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := chatIleSenkronizeOl(context.Background(), h2, h3.ID(), depo); err == nil {
		t.Fatal("bağlı olmayan bir eşe sync denemesi hata dönmeliydi")
	}
}

func TestChatIleSenkronizeOl_BozukYanitHataDoner(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)
	h1.SetStreamHandler(chatSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_, _ = st.Write([]byte("bu geçerli bir json değil"))
	})
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := chatIleSenkronizeOl(ctx, h2, h1.ID(), depo); err == nil {
		t.Fatal("bozuk JSON yanıtı bir hata döndürmeliydi")
	}
}

func TestSiteIleSenkronizeOl_BagliOlmayanEseStreamAcilamaz(t *testing.T) {
	h2 := testHost(t)
	h3 := testHost(t)
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := siteIleSenkronizeOl(context.Background(), h2, h3.ID(), depo); err == nil {
		t.Fatal("bağlı olmayan bir eşe sync denemesi hata dönmeliydi")
	}
}

func TestSiteIleSenkronizeOl_BozukYanitHataDoner(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)
	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_, _ = st.Write([]byte("bu geçerli bir json değil"))
	})
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err == nil {
		t.Fatal("bozuk JSON yanıtı bir hata döndürmeliydi")
	}
}

// ─── Sürüm farkına duyarlı site senkronu (KnownVersions) ─────────────────────

// TestSiteIleSenkronizeOl_BilinenSurumleriGonderir, giden isteğin, AND'ın
// site/sync.go'sundaki KnownVersions ile AYNI biçimde (yazar_key -> slug ->
// UnixNano) elimizdeki sayfa sürümlerini taşıdığını doğrular.
func TestSiteIleSenkronizeOl_BilinenSurumleriGonderir(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	pub, _ := genKey(t)
	authorKey := hex.EncodeToString(pub)
	ts := time.Date(2026, 9, 14, 12, 0, 0, 123456789, time.UTC)
	if err := depo.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: siteKaynakID(authorKey, "home"), Baslik: "x", Govde: "y", YazarKey: authorKey, OlusturmaTS: ts}); err != nil {
		t.Fatal(err)
	}

	var alinanIstek siteSyncIstek
	istekAlindi := make(chan struct{})
	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		_ = json.NewDecoder(st).Decode(&alinanIstek)
		close(istekAlindi)
		_ = json.NewEncoder(st).Encode(siteSyncYanit{})
	})

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}
	<-istekAlindi

	if got := alinanIstek.KnownVersions[authorKey]["home"]; got != ts.UnixNano() {
		t.Fatalf("istek, sayfanın sürümünü nanosaniye hassasiyetiyle taşımalıydı: got %d, want %d", got, ts.UnixNano())
	}
}

// TestSiteIleSenkronizeOl_DuzenlenmisSayfayiGunceller, sürüm-farkına duyarlı
// (yeni) bir AND yanıtlayıcısı karşısında, yerelde ESKİ sürümü bulunan bir
// sayfanın DÜZENLENMİŞ halinin senkronla indekse geldiğini uçtan uca
// doğrular. Sahte eş, AND'ın syncResponseFor'unun kuralını uygular: bilinen
// bir slug'ı yalnızca istekteki sürümden daha yeniyse gönderir.
func TestSiteIleSenkronizeOl_DuzenlenmisSayfayiGunceller(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)
	now := time.Now().UTC()
	v1 := signedSitePage(t, priv, pub, "home", "Eski Başlık", "eski içerik", now)
	v2 := signedSitePage(t, priv, pub, "home", "Yeni Başlık", "düzenlenmiş içerik", now.Add(time.Minute))

	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	if err := depo.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: siteKaynakID(authorKey, "home"), Baslik: v1.Title, Govde: v1.Body, YazarKey: authorKey, OlusturmaTS: v1.UpdatedAt, Sig: v1.Sig}); err != nil {
		t.Fatal(err)
	}

	h1.SetStreamHandler(siteSyncProtokolu, func(st libp2pnet.Stream) {
		defer st.Close()
		var req siteSyncIstek
		_ = json.NewDecoder(st).Decode(&req)
		var resp siteSyncYanit
		if v, ok := req.KnownVersions[authorKey]["home"]; !ok || v2.UpdatedAt.UnixNano() > v {
			resp.Pages = []*SitePage{v2}
		}
		_ = json.NewEncoder(st).Encode(resp)
	})

	if err := siteIleSenkronizeOl(ctx, h2, h1.ID(), depo); err != nil {
		t.Fatal(err)
	}

	sayfa, bulundu, err := depo.SiteSayfasiBul(siteKaynakID(authorKey, "home"))
	if err != nil || !bulundu {
		t.Fatalf("sayfa indekste bulunmalıydı: bulundu=%v err=%v", bulundu, err)
	}
	if sayfa.Baslik != "Yeni Başlık" || sayfa.Govde != "düzenlenmiş içerik" {
		t.Fatalf("sync sonrası sayfanın DÜZENLENMİŞ sürümü indekslenmeliydi, got %+v", sayfa)
	}
}

func TestSiteSyncIstekKodla_SinirAsilincaSurumleriDusurur(t *testing.T) {
	istek := siteSyncIstek{
		Known:           map[string][]string{"ab": {"home"}},
		KnownVersions:   map[string]map[string]int64{"ab": {"home": 42}},
		KnownTombstones: []string{"cd/eski"},
	}
	tam, err := siteSyncIstekKodla(istek, andSiteSyncIstekSiniri)
	if err != nil {
		t.Fatal(err)
	}
	var got siteSyncIstek
	if err := json.Unmarshal(tam, &got); err != nil {
		t.Fatal(err)
	}
	if got.KnownVersions["ab"]["home"] != 42 {
		t.Fatalf("sınırın altındaki istek sürümleri taşımalıydı: %s", tam)
	}

	kirpilmis, err := siteSyncIstekKodla(istek, len(tam)-1)
	if err != nil {
		t.Fatal(err)
	}
	got = siteSyncIstek{}
	if err := json.Unmarshal(kirpilmis, &got); err != nil {
		t.Fatal(err)
	}
	if got.KnownVersions != nil || len(got.Known["ab"]) != 1 || len(got.KnownTombstones) != 1 {
		t.Fatalf("sınırı aşan istekte yalnızca KnownVersions düşmeliydi, got %+v", got)
	}
}

// TestDepo_SiteSurumleriAl_AralikDisiZamaniAtlar, UnixNano'nun tanımsız
// olduğu bir zaman damgasının (ör. sıfır time.Time) sürüm haritasına hiç
// girmediğini — o sayfa için eski "bilinen slug = güncel" kuralına
// düşüldüğünü — ve olağan bir sayfanın girdiğini doğrular.
func TestDepo_SiteSurumleriAl_AralikDisiZamaniAtlar(t *testing.T) {
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer depo.Kapat() //nolint:errcheck
	ts := time.Now().UTC()
	if err := depo.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: "aa/normal", Baslik: "x", Govde: "y", YazarKey: "aa", OlusturmaTS: ts}); err != nil {
		t.Fatal(err)
	}
	if err := depo.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: "aa/sifir", Baslik: "x", Govde: "y", YazarKey: "aa"}); err != nil {
		t.Fatal(err)
	}
	if err := depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "forum", Govde: "g", OlusturmaTS: ts}); err != nil {
		t.Fatal(err)
	}

	surumler, err := depo.SiteSurumleriAl()
	if err != nil {
		t.Fatal(err)
	}
	if len(surumler) != 1 || surumler["aa/normal"] != ts.UnixNano() {
		t.Fatalf("yalnızca olağan zamanlı site sayfası sürüm haritasında olmalıydı, got %+v", surumler)
	}
}
