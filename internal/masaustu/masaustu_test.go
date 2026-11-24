package masaustu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/ui"
)

// deneme, sunucunun işleyicisini ağ açmadan çağıran bir test istemcisidir.
type deneme struct {
	t     *testing.T
	u     *Uygulama
	cerez *http.Cookie
}

func yeniDeneme(t *testing.T) *deneme {
	t.Helper()
	dir := t.TempDir()
	u, err := yeniUygulama(dir, filepath.Join(dir, "identity.dat"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.sv.ln.Close() })
	d := &deneme{t: t, u: u}
	// Tek kullanımlık kodla oturum çerezi alınır.
	w := d.istek(httptest.NewRequest("GET", strings.TrimPrefix(u.sv.acilisAdresi(), "http://"+u.sv.adres), nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("açılış kodu: %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == cerezAdi {
			d.cerez = c
		}
	}
	if d.cerez == nil || !d.cerez.HttpOnly || d.cerez.SameSite != http.SameSiteStrictMode {
		t.Fatalf("oturum çerezi = %+v", d.cerez)
	}
	return d
}

func (d *deneme) istek(r *http.Request) *httptest.ResponseRecorder {
	if r.Host == "" || r.Host == "example.com" {
		r.Host = d.u.sv.adres
	}
	w := httptest.NewRecorder()
	d.u.sv.srv.Handler.ServeHTTP(w, r)
	return w
}

func (d *deneme) al(yol string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", yol, nil)
	r.AddCookie(d.cerez)
	return d.istek(r)
}

func (d *deneme) gonder(yol string, govde any) *httptest.ResponseRecorder {
	veri, _ := json.Marshal(govde)
	r := httptest.NewRequest("POST", yol, bytes.NewReader(veri))
	r.AddCookie(d.cerez)
	r.Header.Set(istekBasligi, "1")
	r.Header.Set("Content-Type", "application/json")
	return d.istek(r)
}

func jsonCoz(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("JSON değil (%d): %q", w.Code, w.Body.String())
	}
	return m
}

func TestSunucu_Korumalar(t *testing.T) {
	d := yeniDeneme(t)

	// Çerezsiz hiçbir şey (sayfa dahil) sunulmaz.
	if w := d.istek(httptest.NewRequest("GET", "/", nil)); w.Code != http.StatusForbidden {
		t.Errorf("çerezsiz / = %d", w.Code)
	}
	// Açılış kodu yalnızca bir kez geçerlidir.
	if w := d.istek(httptest.NewRequest("GET", "/ac?kod=yanlis", nil)); w.Code != http.StatusForbidden {
		t.Errorf("yanlış kod = %d", w.Code)
	}
	if d.u.sv.acilisKodu != "" {
		t.Error("kullanılan açılış kodu silinmeli")
	}
	// Başka bir Host (DNS yeniden bağlama) reddedilir.
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "kotu.example:80"
	r.AddCookie(d.cerez)
	if w := d.istek(r); w.Code != http.StatusMisdirectedRequest {
		t.Errorf("yabancı Host = %d", w.Code)
	}
	// Başka siteden gelen istek reddedilir.
	r = httptest.NewRequest("GET", "/api/baslangic", nil)
	r.AddCookie(d.cerez)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if w := d.istek(r); w.Code != http.StatusForbidden {
		t.Errorf("cross-site = %d", w.Code)
	}
	// Durum değiştiren istek X-AND başlığı olmadan ve yabancı kökenle reddedilir.
	r = httptest.NewRequest("POST", "/api/dil", strings.NewReader(`{"kod":"en"}`))
	r.AddCookie(d.cerez)
	if w := d.istek(r); w.Code != http.StatusForbidden {
		t.Errorf("X-AND'sız POST = %d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/dil", strings.NewReader(`{"kod":"en"}`))
	r.AddCookie(d.cerez)
	r.Header.Set(istekBasligi, "1")
	r.Header.Set("Origin", "https://kotu.example")
	if w := d.istek(r); w.Code != http.StatusForbidden {
		t.Errorf("yabancı Origin = %d", w.Code)
	}

	// Çerezle sayfa sunulur ve güvenlik başlıkları vardır.
	w := d.al("/")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "uygulama.js") {
		t.Fatalf("/ = %d", w.Code)
	}
	csp := w.Header().Get("Content-Security-Policy")
	for _, parca := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, parca) {
			t.Errorf("CSP'de %q yok: %q", parca, csp)
		}
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff yok")
	}
}

func TestSunucu_GovdeSiniri(t *testing.T) {
	d := yeniDeneme(t)
	w := d.gonder("/api/giris", map[string]string{"sifre": strings.Repeat("a", enBuyukGovde+10)})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("büyük gövde = %d", w.Code)
	}
}

func TestGiris_KayitVeDogrulama(t *testing.T) {
	d := yeniDeneme(t)
	if a := jsonCoz(t, d.al("/api/baslangic"))["asama"]; a != asamaKayit {
		t.Fatalf("ilk aşama = %v", a)
	}
	// Denetimler: kısa şifre, eşleşmeyen tekrar, boş ad.
	for _, g := range []map[string]string{
		{"ad": "Ayşe", "sifre": "kisa", "tekrar": "kisa"},
		{"ad": "Ayşe", "sifre": "uzun-bir-sifre", "tekrar": "baska-bir-sifre"},
		{"ad": "  ", "sifre": "uzun-bir-sifre", "tekrar": "uzun-bir-sifre"},
		{"ad": strings.Repeat("a", 33), "sifre": "uzun-bir-sifre", "tekrar": "uzun-bir-sifre"},
	} {
		if w := d.gonder("/api/kayit", g); w.Code != http.StatusBadRequest || jsonCoz(t, w)["hata"] == "" {
			t.Errorf("%v kabul edildi (%d)", g, w.Code)
		}
	}
	w := d.gonder("/api/kayit", map[string]string{"ad": "Ayşe", "sifre": "uzun-bir-sifre", "tekrar": "uzun-bir-sifre"})
	kod, _ := jsonCoz(t, w)["kod"].(string)
	if w.Code != http.StatusOK || kod == "" {
		t.Fatalf("kayıt = %d %q", w.Code, w.Body.String())
	}
	// Kod yeniden istenebilir (sayfa yenilenirse).
	if k := jsonCoz(t, d.al("/api/kayit/kod"))["kod"]; k != kod {
		t.Errorf("kod yeniden = %v", k)
	}
	if w := d.gonder("/api/kayit/dogrula", map[string]string{"kod": "yanlis"}); w.Code != http.StatusBadRequest {
		t.Errorf("yanlış kod = %d", w.Code)
	}
	if kimlikVar(d.u.giris.dosya) {
		t.Fatal("kimlik doğrulamadan önce kaydedilmemeli")
	}
	// Tireler, boşluklar ve büyük/küçük harf önemsizdir.
	if w := d.gonder("/api/kayit/dogrula", map[string]string{"kod": " " + strings.ToLower(strings.ReplaceAll(kod, "-", "")) + " "}); w.Code != http.StatusOK {
		t.Fatalf("doğrulama = %d %s", w.Code, w.Body.String())
	}
	id, err := d.u.Giris()
	if err != nil || id.Name() != "Ayşe" {
		t.Fatalf("Giris = %v, %v", id, err)
	}
	// Kaydedilen kimlik aynı şifreyle açılır.
	acilan, err := stdcrypto.LoadEncrypted(d.u.giris.dosya, "uzun-bir-sifre")
	if err != nil || acilan.Name() != "Ayşe" {
		t.Fatalf("kaydedilen kimlik: %v", err)
	}
	if d.u.giris.sifre != "" || d.u.giris.kimlik != nil {
		t.Error("teslimden sonra şifre ve kimlik bellekte tutulmamalı")
	}
}

func kayitliDeneme(t *testing.T, ad, sifre string) (*deneme, *stdcrypto.Identity) {
	t.Helper()
	dir := t.TempDir()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName(ad)
	yol := filepath.Join(dir, "identity.dat")
	if err := id.SaveEncrypted(yol, sifre); err != nil {
		t.Fatal(err)
	}
	u, err := yeniUygulama(dir, yol)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.sv.ln.Close() })
	d := &deneme{t: t, u: u}
	w := d.istek(httptest.NewRequest("GET", strings.TrimPrefix(u.sv.acilisAdresi(), "http://"+u.sv.adres), nil))
	d.cerez = w.Result().Cookies()[0]
	return d, id
}

func TestGiris_SifreIle(t *testing.T) {
	d, id := kayitliDeneme(t, "Ali", "dogru-sifre")
	if a := jsonCoz(t, d.al("/api/baslangic"))["asama"]; a != asamaGiris {
		t.Fatalf("aşama = %v", a)
	}
	if w := d.gonder("/api/giris", map[string]string{"sifre": "yanlis-sifre"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("yanlış şifre = %d", w.Code)
	}
	if w := d.gonder("/api/giris", map[string]string{"sifre": "dogru-sifre"}); w.Code != http.StatusOK {
		t.Fatalf("doğru şifre = %d %s", w.Code, w.Body.String())
	}
	acilan, err := d.u.Giris()
	if err != nil || !bytes.Equal(acilan.PublicKey(), id.PublicKey()) {
		t.Fatalf("Giris = %v", err)
	}
	if a := d.u.giris.durum(); a != asamaBasliyor {
		t.Errorf("girişten sonra aşama = %s", a)
	}
}

func TestGeriYukleme(t *testing.T) {
	d, id := kayitliDeneme(t, "Eski", "unutulan-sifre")
	kod := id.RecoveryCode()
	if w := d.gonder("/api/geri-yukleme", map[string]bool{"geri": false}); jsonCoz(t, w)["asama"] != asamaGeriKod {
		t.Fatalf("geri yüklemeye geçilemedi: %s", w.Body.String())
	}
	if w := d.gonder("/api/geri-yukleme/kod", map[string]string{"kod": "gecersiz"}); w.Code != http.StatusBadRequest {
		t.Errorf("geçersiz kod = %d", w.Code)
	}
	w := d.gonder("/api/geri-yukleme/kod", map[string]string{"kod": kod})
	if w.Code != http.StatusOK || jsonCoz(t, w)["asama"] != asamaGeriForm {
		t.Fatalf("kod = %d %s", w.Code, w.Body.String())
	}
	if w := d.gonder("/api/geri-yukleme/kaydet", map[string]string{"ad": "Yeni", "sifre": "yeni-sifre-123", "tekrar": "yeni-sifre-123"}); w.Code != http.StatusOK {
		t.Fatalf("kaydet = %d %s", w.Code, w.Body.String())
	}
	acilan, err := d.u.Giris()
	if err != nil || !bytes.Equal(acilan.PublicKey(), id.PublicKey()) || acilan.Name() != "Yeni" {
		t.Fatalf("geri yüklenen kimlik yanlış: %v", err)
	}
	if _, err := stdcrypto.LoadEncrypted(d.u.giris.dosya, "yeni-sifre-123"); err != nil {
		t.Fatalf("yeni şifreyle açılmadı: %v", err)
	}
	// Eski dosya silinmedi, kenara alındı.
	girdiler, _ := os.ReadDir(filepath.Dir(d.u.giris.dosya))
	kenarda := false
	for _, g := range girdiler {
		if strings.HasPrefix(g.Name(), "identity.dat") && g.Name() != "identity.dat" {
			kenarda = true
		}
	}
	if !kenarda {
		t.Error("eski kimlik dosyası kenara alınmalı")
	}
}

func TestGiris_PencereKapaninca(t *testing.T) {
	d := yeniDeneme(t)
	go d.u.bitir()
	if _, err := d.u.Giris(); err != ErrKapandi {
		t.Fatalf("err = %v", err)
	}
}

// cekirdekli, gerçek bir forum ve sohbet deposuyla çalışan bir uygulamadır.
func cekirdekli(t *testing.T, eklentiler ...pluginmgr.Plugin) (*deneme, *forum.Forum) {
	t.Helper()
	d := yeniDeneme(t)
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName("Deniz")
	f, err := forum.New(id, nil, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	d.u.cek.Store(yeniCekirdek(d.u.dir, Cekirdek{Kimlik: id, Forum: f, Eklentiler: eklentiler}))
	return d, f
}

func TestForum_OzetVeDenetimler(t *testing.T) {
	d, _ := cekirdekli(t)
	o := jsonCoz(t, d.al("/api/forum"))
	if o["konuAcabilir"] != false || len(o["kategoriler"].([]any)) == 0 {
		t.Fatalf("özet = %v", o)
	}
	// Konu Aç kapalıyken yeni konu açılamaz.
	if w := d.gonder("/api/forum/konu", map[string]string{"kategori": "Genel", "baslik": "b", "metin": "m"}); w.Code != http.StatusForbidden {
		t.Errorf("konu_ac yokken = %d", w.Code)
	}

	d, _ = cekirdekli(t, pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "konu_ac", Label: "Konu Aç"}, Enabled: true})
	for _, g := range []map[string]string{
		{"kategori": "yok-boyle-kategori", "baslik": "b", "metin": "m"},
		{"kategori": "Genel", "baslik": " ", "metin": "m"},
		{"kategori": "Genel", "baslik": strings.Repeat("ş", enUzunBaslik+1), "metin": "m"},
		{"kategori": "Genel", "baslik": "b", "metin": "  "},
		{"kategori": "Genel", "baslik": "b", "metin": "[code]kapanmadı"},
		{"kategori": "Genel", "baslik": "b", "metin": strings.Repeat("x", forum.MaxBodyBytes+1)},
	} {
		if w := d.gonder("/api/forum/konu", g); w.Code != http.StatusBadRequest {
			t.Errorf("%.40v kabul edildi (%d)", g, w.Code)
		}
	}
	if w := d.gonder("/api/forum/yanit", map[string]string{"konu": "olmayan", "metin": "m"}); w.Code != http.StatusNotFound {
		t.Errorf("olmayan konuya yanıt = %d", w.Code)
	}
	if w := d.al("/api/forum/konu?id=olmayan"); w.Code != http.StatusNotFound {
		t.Errorf("olmayan konu = %d", w.Code)
	}
}

func TestCekirdekYokken(t *testing.T) {
	d := yeniDeneme(t)
	if w := d.al("/api/forum"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("çekirdeksiz = %d", w.Code)
	}
}

func TestOlaylar_Akis(t *testing.T) {
	d := yeniDeneme(t)
	ctx, iptal := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/api/olaylar", nil).WithContext(ctx)
	r.AddCookie(d.cerez)
	w := httptest.NewRecorder()
	bitti := make(chan struct{})
	go func() {
		r.Host = d.u.sv.adres
		d.u.sv.srv.Handler.ServeHTTP(w, r)
		close(bitti)
	}()
	for i := 0; !d.u.yayin.baglandiMi(); i++ {
		if i > 200 {
			t.Fatal("abone olunmadı")
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.u.yayin.gonder("hazir", true)
	time.Sleep(50 * time.Millisecond)
	iptal()
	<-bitti
	if !strings.Contains(w.Body.String(), "event: hazir\ndata: true\n\n") {
		t.Fatalf("olay akışı: %q", w.Body.String())
	}
	if !d.u.yayin.terkEdildi(0) {
		t.Error("bağlantı kapanınca terk edilmiş sayılmalı")
	}
}

// TestArayuzMetinleri: metinler.go, uygulama.js'deki t/tf/tb/K metinleriyle
// bire bir aynı olmalı (yoksa metin çevrilmeden görünür ya da kullanılmayan
// bir çeviri kalır).
func TestArayuzMetinleri(t *testing.T) {
	js, err := webDosyalari.ReadFile("web/uygulama.js")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\b(?:t|tf|tb|K)\("((?:[^"\\]|\\.)*)"`)
	jsde := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(js), -1) {
		s, err := strconv.Unquote(`"` + m[1] + `"`)
		if err != nil {
			t.Fatalf("%q: %v", m[1], err)
		}
		jsde[s] = true
	}
	listede := map[string]bool{}
	for _, s := range arayuzKaynaklari {
		listede[s] = true
		if !jsde[s] {
			t.Errorf("metinler.go'da var, uygulama.js'de yok: %q", s)
		}
	}
	for s := range jsde {
		if !listede[s] {
			t.Errorf("uygulama.js'de var, metinler.go'da yok: %q (metinler.go'yu yeniden üret)", s)
		}
	}
}

// TestArayuzHTMLYorumlamaz: ağdan gelen içerik yalnızca metin düğümü olarak
// yazılır; HTML yorumlayan API'ler arayüzde hiç kullanılmamalı.
func TestArayuzHTMLYorumlamaz(t *testing.T) {
	js, _ := webDosyalari.ReadFile("web/uygulama.js")
	for _, yasak := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "srcdoc"} {
		if bytes.Contains(js, []byte(yasak)) {
			t.Errorf("uygulama.js %q kullanıyor", yasak)
		}
	}
}

func TestXtermHex(t *testing.T) {
	for kod, beklenen := range map[string]string{
		"63": "#5f5fff", "57": "#5f00ff", "255": "#eeeeee", "232": "#080808", "0": "#000000", "15": "#ffffff",
		"#ABCDEF": "#abcdef", "256": "yedek", "": "yedek", "kırmızı": "yedek",
	} {
		if got := xtermHex(kod, "yedek"); got != beklenen {
			t.Errorf("xtermHex(%q) = %q, beklenen %q", kod, got, beklenen)
		}
	}
}

func TestTema_OzelRenk(t *testing.T) {
	d := yeniDeneme(t)
	// Önizleme kaydetmez.
	w := d.al("/api/tema/ozel?renk=%23ff8800")
	if w.Code != http.StatusOK || jsonCoz(t, w)["vurgu"] != "#ff8800" {
		t.Fatalf("önizleme = %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(d.u.dir, "theme.json")); err == nil {
		t.Fatal("önizleme temayı kaydetmemeli")
	}
	if w := d.al("/api/tema/ozel?renk=turuncu"); w.Code != http.StatusBadRequest {
		t.Errorf("geçersiz renk = %d", w.Code)
	}
	// Uygulama kaydeder; terminal arayüzü de aynı dosyayı okur.
	if w := d.gonder("/api/tema/ozel", map[string]string{"renk": "#ff8800"}); w.Code != http.StatusOK {
		t.Fatalf("uygula = %d %s", w.Code, w.Body.String())
	}
	r := jsonCoz(t, d.al("/api/temalar"))
	etkin := r["etkin"].(map[string]any)
	if etkin["ad"] != r["ozelAd"] || etkin["vurgu"] != "#ff8800" {
		t.Fatalf("etkin tema = %v", etkin)
	}
}

func TestUzerindekiYazi(t *testing.T) {
	for zemin, beklenen := range map[string]string{
		"#5f5fff": "#ffffff", // Klasik: beyaz okunur
		"#00afff": "#0b0b0e", // Okyanus: açık, koyu yazı
		"#87d787": "#0b0b0e", // Orman
		"#eeeeee": "#0b0b0e", // Mono
		"#af005f": "#ffffff",
		"bozuk":   "#ffffff",
	} {
		if got := uzerindekiYazi(zemin); got != beklenen {
			t.Errorf("uzerindekiYazi(%s) = %s, beklenen %s", zemin, got, beklenen)
		}
	}
}

// Kenar çubuğu ve Ayarlar › Eklentiler, eklentileri kategorilerine göre
// bölümlere ayırır: liste bölüm sırasıyla (bkz. pluginapi.EklentiKategorileri),
// bölüm içinde ada göre gelir; kategorisi olmayanlar "Diğer"e, temalar menüye
// girmez.
func TestEklentiler_KategoriyeGoreBolumlenir(t *testing.T) {
	d, _ := cekirdekli(t,
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "hava", Label: "Hava"}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "DeltaX", Label: "DeltaX", Category: pluginapi.KategoriGelistirme}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "site", Label: "Site", Category: pluginapi.KategoriIcerik}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "ozel_chat", Label: "Özel Chat", Category: pluginapi.KategoriIletisim}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "konu_ac", Label: "Konu Aç", Category: pluginapi.KategoriIcerik}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "pembis", Label: "Pembiş", Category: pluginapi.KategoriTema}, Enabled: true},
		pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: "admin", Label: "Yönetici Paneli", Category: pluginapi.KategoriYonetim}, Enabled: false},
	)
	ozetle := func(liste []any) []string {
		var out []string
		for _, o := range liste {
			e := o.(map[string]any)
			out = append(out, e["kategori"].(string)+"/"+e["etiket"].(string))
		}
		return out
	}

	menu := ozetle(jsonCoz(t, d.al("/api/oturum"))["menu"].([]any))
	want := []string{"İletişim/Özel Chat", "İçerik/Konu Aç", "İçerik/Site", "Geliştirme/DeltaX", "Diğer/Hava"}
	if strings.Join(menu, ", ") != strings.Join(want, ", ") {
		t.Errorf("menü = %v\n  istenen %v", menu, want)
	}

	hepsi := ozetle(jsonCoz(t, d.al("/api/eklentiler"))["eklentiler"].([]any))
	want = []string{"İletişim/Özel Chat", "İçerik/Konu Aç", "İçerik/Site", "Yönetim/Yönetici Paneli", "Geliştirme/DeltaX", "Görünüm/Pembiş", "Diğer/Hava"}
	if strings.Join(hepsi, ", ") != strings.Join(want, ", ") {
		t.Errorf("ayarlar = %v\n  istenen %v", hepsi, want)
	}
}

// sahteEklentiDegiskeni verilince test ikilisi bir eklenti gibi davranır
// (bkz. TestMain): AND'ın içinde çalışıp çalışmadığını yazar ve durdurulana
// dek bekler.
const sahteEklentiDegiskeni = "AND_SAHTE_EKLENTI"

func TestMain(m *testing.M) {
	if os.Getenv(sahteEklentiDegiskeni) != "" {
		fmt.Printf("sahte eklenti gomulu=%s\n", os.Getenv(ui.GomuluOrtamDegiskeni))
		time.Sleep(time.Hour)
		return
	}
	os.Exit(m.Run())
}

// AND açılınca yalnızca etkin, menüde görünen, uyumlu, doğrulanmış ve
// izinleri önceden onaylanmış eklentiler kendiliğinden başlar.
func TestEklentiler_KendiligindenBaslayacaklar(t *testing.T) {
	eklenti := func(ad, etiket string, izinler ...string) pluginmgr.Plugin {
		return pluginmgr.Plugin{Manifest: pluginapi.Manifest{Name: ad, Label: etiket, Permissions: izinler}, Enabled: true}
	}
	kapali := eklenti("admin", "Yönetici Paneli", "moderation")
	kapali.Enabled = false
	tema := eklenti("pembis", "Pembiş", "theme")
	tema.Manifest.Category = pluginapi.KategoriTema
	yeni := eklenti("yeni", "Yeni", "dm")
	yeni.Manifest.MinAPI = pluginapi.APIVersion + 1
	disari := eklenti("disari", "Dışarıdan", "dm")
	disari.Harici = true // kurulum kaydı yok: Dogrula reddeder
	d, _ := cekirdekli(t,
		eklenti("ozel_chat", "Özel Chat", "dm", "file"),
		eklenti("site", "Site", "site"), // izni onaylanmadı
		kapali,
		eklenti("konu_ac", "", "forum"), // menüde görünmez
		tema,
		yeni,
		disari,
		eklenti("izinsiz", "İzinsiz"), // onay kaydı olmaz
		eklenti("eksik", "Eksik", "dm", "file"),
	)
	for ad, izinler := range map[string][]string{
		"ozel_chat": {"dm", "file"}, "admin": {"moderation"}, "konu_ac": {"forum"}, "pembis": {"theme"},
		"yeni": {"dm"}, "disari": {"dm"}, "eksik": {"dm"}, // "file" onaylanmadı
	} {
		if err := pluginmgr.IzinOnayla(d.u.dir, ad, izinler); err != nil {
			t.Fatal(err)
		}
	}
	var adlar []string
	for _, p := range d.u.cek.Load().kendiligindenBaslayacaklar(d.u.dir) {
		adlar = append(adlar, p.Name())
	}
	if strings.Join(adlar, ",") != "ozel_chat" {
		t.Errorf("kendiliğinden başlayacaklar = %v, istenen [ozel_chat]", adlar)
	}

	// Eklenti API'si yoksa (ya da sözde terminal yoksa) hiçbiri başlamaz.
	d.u.eklentileriBaslat(d.u.cek.Load())
	if l := jsonCoz(t, d.al("/api/terminal/liste"))["terminaller"].([]any); len(l) != 0 {
		t.Errorf("API adresi yokken başlayan eklentiler: %v", l)
	}
}
