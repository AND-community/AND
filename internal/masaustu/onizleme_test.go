//go:build onizleme

package masaustu

// onizleme_test.go — arayüzü örnek verilerle, ağ ve pencere açmadan sunar;
// tarayıcıda bakıp ekran görüntüsü almak için. Normal testlerde derlenmez:
//
//	ONIZLEME_SURE=120s go test -tags onizleme -run TestOnizleme -v ./internal/masaustu
//
// Adres http://127.0.0.1:<port>/ac?kod=onizleme biçimindedir; kod her
// kullanımdan sonra yenilenir. ONIZLEME_ASAMA: kayit | giris | (boş: uygulama).
// ONIZLEME_KURALLAR=1 kurallar onayını gösterir. ONIZLEME_DIL: tr, en, …
// ONIZLEME_VITRIN=1 bütün arayüz bileşenlerini gösteren sahte bir arayüz
// eklentisini ("Vitrin", Araçlar) başlatır. ONIZLEME_ARAYUZ=<exe> gerçek bir
// arayüz eklentisini (ör. DeltaX) sahte bir AND API'siyle "DeltaX" olarak
// başlatır.

import (
	"cmp"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucian95511/and/arayuz"
	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/storage"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginapi/pluginapitest"
	"github.com/lucian95511/and/ui"
)

func ornekKimlik(t *testing.T, ad string) (*stdcrypto.Identity, string) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName(ad)
	return id, hex.EncodeToString(id.PublicKey())
}

func TestOnizleme(t *testing.T) {
	sure, err := time.ParseDuration(os.Getenv("ONIZLEME_SURE"))
	if err != nil {
		sure = 60 * time.Second
	}
	if kod := os.Getenv("ONIZLEME_DIL"); kod != "" {
		dil.Ayarla(kod)
	}
	dir := t.TempDir()
	kimlikYolu := filepath.Join(dir, "identity.dat")
	ben, benAnahtar := ornekKimlik(t, "Umut")
	if os.Getenv("ONIZLEME_ASAMA") == "giris" {
		if err := ben.SaveEncrypted(kimlikYolu, "onizleme-sifre"); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("ONIZLEME_KURALLAR") != "1" {
		_ = tui.KurallariOnayla(dir)
	}

	// Örnek forum: farklı yazarlar, kategoriler, yanıtlar, kod ve bağlantı.
	dbYolu := filepath.Join(dir, "forum.db")
	db, err := storage.Open(dbYolu)
	if err != nil {
		t.Fatal(err)
	}
	simdi := time.Now().UTC()
	yazarlar := map[string]string{}
	for _, ad := range []string{"Elif", "Can", "Zeynep", "Mert"} {
		_, a := ornekKimlik(t, ad)
		yazarlar[ad] = a
	}
	yazarlar["Umut"] = benAnahtar
	konular := []struct {
		id, kat, yazar, baslik, govde string
		once                          time.Duration
		yanitlar                      [][3]string // yazar, metin, dakika önce
	}{
		{"k1", "Yazılım", "Elif", "Go ile eşten eşe uygulama geliştirirken dikkat edilmesi gerekenler",
			"Merhaba! libp2p ile küçük bir proje yapıyorum. Bağlantı kurulduktan sonra mesajları nasıl doğruluyorsunuz?\n\nŞu an şöyle yapıyorum:\n[code=go]\nimza := ed25519.Sign(anahtar, veri)\nif !ed25519.Verify(genel, veri, imza) {\n\treturn errors.New(\"imza geçersiz\")\n}\n[/code]\nBelgeler için: https://docs.libp2p.io/concepts/",
			3 * time.Hour, [][3]string{{"Can", "Ben de aynı yöntemi kullanıyorum; ek olarak mesaj kimliğini imzalı içerikten türetmek tekrar saldırılarını kolayca ayıklıyor.", "150"}, {"Umut", "AND'da da öyle: her konu ve yanıt Ed25519 ile imzalanıyor, kimlik imzalı içerikten türetiliyor.", "95"}, {"Zeynep", "Teşekkürler, çok faydalı oldu!", "20"}}},
		{"k2", "Etkinlikler ve Duyurular", "Mert", "Bu hafta sonu açık kaynak oyun gecesi?", "Cumartesi akşamı birlikte Minetest oynamak isteyen var mı? Sunucuyu ben açarım.", 26 * time.Hour,
			[][3]string{{"Elif", "Ben varım!", "900"}}},
		{"k3", "Genel", "Zeynep", "AND'a hoş geldiniz — kendinizi tanıtın", "Yeni katılanlar buraya kısaca kendilerini tanıtabilir. Ben Zeynep, İzmir'den yazıyorum.", 2 * 24 * time.Hour, nil},
		{"k4", "Siber Güvenlik", "Can", "Kuantum bilgisayarlar şifrelemeyi ne zaman kırar?", "Son okuduğum makaleye göre RSA-2048 için milyonlarca kararlı kübit gerekiyor. Sizce ne kadar zamanımız var?", 40 * time.Minute, nil},
		{"k5", "Genel", "Elif", "Çalışırken dinlediğiniz albümler", "Ben son günlerde lo-fi ve klasik karışık dinliyorum. Önerilerinizi bekliyorum.", 3 * 24 * time.Hour,
			[][3]string{{"Mert", "Pink Floyd — The Dark Side of the Moon, her zaman.", "3000"}}},
	}
	for _, k := range konular {
		_ = db.InsertPost(storage.RawPost{ID: k.id, Category: k.kat, AuthorName: k.yazar, AuthorKey: yazarlar[k.yazar],
			Title: k.baslik, Body: k.govde, CreatedAt: simdi.Add(-k.once), Sig: "00"})
		for i, y := range k.yanitlar {
			var dk int
			fmt.Sscan(y[2], &dk)
			_ = db.InsertReply(storage.RawReply{ID: fmt.Sprintf("%s-y%d", k.id, i), PostID: k.id, AuthorName: y[0],
				AuthorKey: yazarlar[y[0]], Body: y[1], CreatedAt: simdi.Add(-time.Duration(dk) * time.Minute), Sig: "00"})
		}
	}
	db.Close()
	f, err := forum.New(ben, nil, dbYolu)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sohbet, err := chatlog.New(ben, nil, f.DB(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"Herkese merhaba!", "Masaüstü arayüzü nasıl görünüyor? https://and-community.github.io"} {
		if _, err := sohbet.Send(context.Background(), m); err != nil {
			t.Log(err)
		}
	}
	prof, err := profile.New(ben, nil, f.DB(), nil)
	if err != nil {
		t.Fatal(err)
	}
	eklentiler := []pluginmgr.Plugin{
		{Manifest: pluginapi.Manifest{Name: "konu_ac", Label: "Konu Aç", Version: "1.2.0", Author: "AND", Category: pluginapi.KategoriIcerik, Description: "Foruma yeni konu açar."}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "site", Label: "Site", Version: "1.0.0", Author: "AND", Category: pluginapi.KategoriIcerik, Description: "Forum konularını kişisel bir web sitesi olarak yayımlar."}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "ozel_chat", Label: "Özel Chat", Version: "1.0.0", Author: "AND", Category: pluginapi.KategoriIletisim, Description: "Uçtan uca şifreli özel mesajlar.", Permissions: []string{pluginapi.IzinDM}}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "DeltaX", Label: "DeltaX", Version: "0.9.0", Author: "AND", Category: pluginapi.KategoriGelistirme, Description: "Eşler arası sürüm denetimi."}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "pembis", Label: "Pembiş", Version: "1.0.0", Author: "AND", Category: pluginapi.KategoriTema, Description: "Tema paketi."}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "hava", Label: "Hava durumu", Version: "0.1.0", Author: "Ayşe"}, Enabled: false, Harici: true},
	}
	// ONIZLEME_TERMINAL, "--deneme" kipi olan bir eklenti programıdır (ör.
	// "and eklenti yeni" iskeletinden derlenen): AND'ın içinde, Araçlar
	// bölümünde "Örnek" adıyla çalışır.
	if os.Getenv("ONIZLEME_VITRIN") != "" {
		eklentiler = append(eklentiler, pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "vitrin", Label: "Vitrin", Version: "1.0.0", Author: "AND",
			Category: pluginapi.KategoriArac, Description: "Eklenti arayüzünün bütün bileşenleri.", UI: arayuz.Surum}, Enabled: true})
	}
	arayuzProgrami := os.Getenv("ONIZLEME_ARAYUZ")
	// ONIZLEME_ARAYUZ_AD, ONIZLEME_ARAYUZ'un hangi eklenti olarak açılacağıdır
	// (varsayılan DeltaX; ör. ozel_chat).
	arayuzAdi := cmp.Or(os.Getenv("ONIZLEME_ARAYUZ_AD"), "DeltaX")
	arayuzEtiketi := arayuzAdi
	if arayuzProgrami != "" {
		for i := range eklentiler {
			if eklentiler[i].Name() == arayuzAdi {
				eklentiler[i].Manifest.UI = arayuz.Surum
				arayuzEtiketi = eklentiler[i].Label()
			}
		}
	}
	terminalProgrami := os.Getenv("ONIZLEME_TERMINAL")
	if terminalProgrami != "" {
		eklentiler = append(eklentiler, pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "ornek", Label: "Örnek", Version: "0.1.0", Author: "Umut",
			Category: pluginapi.KategoriArac, Description: "Kimliğini, bağlı AND düğümlerini ve genel sohbeti gösteren bir AND eklentisi."}, Enabled: true})
	}

	u, err := yeniUygulama(dir, kimlikYolu)
	if err != nil {
		t.Fatal(err)
	}
	if terminalProgrami != "" {
		// Gerçek AND'daki gibi (bkz. eklentileriBaslat) eklenti açılışta,
		// arayüz yüklendikten biraz sonra arka planda başlar ve arayüze
		// "terminaller" olayı gider.
		go func() {
			time.Sleep(4 * time.Second)
			cmd := exec.Command(terminalProgrami, "--deneme")
			cmd.Dir = filepath.Dir(terminalProgrami)
			cmd.Env = append(os.Environ(), dil.OrtamDegiskeni+"="+dil.Etkin(), ui.GomuluOrtamDegiskeni+"=1")
			if _, err := u.term.baslat("ornek", "Örnek", cmd, kendiligindenSutun, kendiligindenSatir, nil); err != nil {
				t.Error(err)
				return
			}
			u.yayin.gonder("terminaller", true)
		}()
		defer u.term.hepsiniDurdur()
	}
	// Arayüz eklentileri de gerçek AND'daki gibi açılıştan biraz sonra
	// başlar (bkz. eklentileriBaslat).
	arayuzBaslat := func(ad, etiket string, cmd *exec.Cmd) {
		go func() {
			time.Sleep(2 * time.Second)
			if _, err := u.arayuz.baslat(ad, etiket, cmd, nil); err != nil {
				t.Error(err)
				return
			}
			u.yayin.gonder("terminaller", true)
		}()
	}
	defer u.arayuz.hepsiniDurdur()
	if os.Getenv("ONIZLEME_VITRIN") != "" {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), sahteArayuzDegiskeni+"=vitrin")
		arayuzBaslat("vitrin", "Vitrin", cmd)
	}
	if arayuzProgrami != "" {
		sahte := pluginapitest.Yeni().Ornek()
		if arayuzAdi == "ozel_chat" {
			for i := range 14 {
				sahte.DMGelsin("12D3KooWAzizeAbCdEfGhJkLmNpQrStUvWxYz123456789abcde", "Azize", fmt.Sprintf("Merhaba, %d. mesaj", i+1))
			}
			sahte.DMGelsin("12D3KooWBerkAbCdEfGhJkLmNpQrStUvWxYz123456789abcdef", "Berk", "Yarın toplantı var mı?")
		}
		sn, err := sahte.Baslat(t.TempDir(), pluginapi.IzinDM, pluginapi.IzinDosya)
		if err != nil {
			t.Fatal(err)
		}
		defer sn.Kapat()
		cmd := exec.Command(arayuzProgrami)
		cmd.Dir = filepath.Dir(arayuzProgrami)
		cmd.Env = append(append(os.Environ(), sn.Ortam()...), dil.OrtamDegiskeni+"="+dil.Etkin(),
			ui.GomuluOrtamDegiskeni+"=1", arayuz.OrtamDegiskeni+"=1")
		arayuzBaslat(arayuzAdi, arayuzEtiketi, cmd)
	}
	if os.Getenv("ONIZLEME_ASAMA") == "" {
		u.giris.mu.Lock()
		u.giris.asama = asamaBasliyor
		u.giris.mu.Unlock()
		u.cek.Store(yeniCekirdek(dir, Cekirdek{Kimlik: ben, Forum: f, Sohbet: sohbet, Profil: prof, Eklentiler: eklentiler,
			OkunmamisDM: func() int { return 2 }}))
		go u.olaylariIlet(context.Background(), u.cek.Load())
	}
	u.sv.baslat()
	defer u.sv.kapat()
	go func() {
		for {
			u.sv.mu.Lock()
			u.sv.acilisKodu = "onizleme"
			u.sv.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}
	}()
	t.Logf("ADRES http://%s/ac?kod=onizleme", u.sv.adres)
	fmt.Printf("ADRES http://%s/ac?kod=onizleme\n", u.sv.adres)
	time.Sleep(sure)
}
