package pluginmgr

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/lucian95511/and/pluginapi"
)

var (
	testProgramiBir  sync.Once
	testProgramiVeri []byte
)

// testProgrami, bu platform için gerçek bir programdır: testin kendi ikilisi.
// Kurulum programı hiç çalıştırmaz; yalnızca başlığını okur.
func testProgrami(t *testing.T) []byte {
	t.Helper()
	testProgramiBir.Do(func() {
		yol, err := os.Executable()
		if err == nil {
			testProgramiVeri, _ = os.ReadFile(yol)
		}
	})
	if len(testProgramiVeri) == 0 {
		t.Skip("test ikilisi okunamadı")
	}
	return testProgramiVeri
}

func exeEki() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func manifestJSON(t *testing.T, m pluginapi.Manifest) []byte {
	t.Helper()
	v, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

var havaManifest = pluginapi.Manifest{
	Name: "hava", Label: "Hava Durumu", Version: "1.0.0", Author: "Deneme",
	Description: "Forum için hava durumu", Permissions: []string{pluginapi.IzinForum},
}

// paketKlasoru, kaynak bir eklenti klasörü yazar.
func paketKlasoru(t *testing.T, m pluginapi.Manifest, program []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestDosyasi), manifestJSON(t, m), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "and-plugin-"+m.Name+exeEki()), program, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

type arsivGirdisi struct {
	ad    string
	veri  []byte
	kip   os.FileMode
	bag   string // tar: sembolik bağlantı hedefi
	klsor bool
}

func zipYaz(t *testing.T, girdiler []arsivGirdisi) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, g := range girdiler {
		fh := &zip.FileHeader{Name: g.ad, Method: zip.Deflate}
		kip := g.kip
		if kip == 0 {
			kip = 0o644
		}
		fh.SetMode(kip)
		w, err := z.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(g.veri); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func tarGzYaz(t *testing.T, girdiler []arsivGirdisi) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, g := range girdiler {
		h := &tar.Header{Name: g.ad, Mode: int64(g.kip), Size: int64(len(g.veri)), Typeflag: tar.TypeReg}
		if h.Mode == 0 {
			h.Mode = 0o644
		}
		switch {
		case g.bag != "":
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, g.bag, 0
		case g.klsor:
			h.Typeflag, h.Size, h.Mode = tar.TypeDir, 0, 0o755
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(g.veri); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func dosyayaYaz(t *testing.T, ad string, veri []byte) string {
	t.Helper()
	yol := filepath.Join(t.TempDir(), ad)
	if err := os.WriteFile(yol, veri, 0o644); err != nil {
		t.Fatal(err)
	}
	return yol
}

func yerlesikYok(t *testing.T, adlar ...string) {
	t.Helper()
	eski := yerlesikAdlar
	yerlesikAdlar = func() []string { return adlar }
	t.Cleanup(func() { yerlesikAdlar = eski })
}

func hazirla(t *testing.T, kaynak, dataDir, ozet string) *Paket {
	t.Helper()
	p, err := Hazirla(kaynak, KullaniciDizini(dataDir), ozet)
	if err != nil {
		t.Fatalf("Hazirla(%s): %v", kaynak, err)
	}
	t.Cleanup(func() { _ = p.Kapat() })
	return p
}

// Bir eklenti klasörden kurulur, AND onu bulur, değişirse başlatmaz,
// kaldırılınca iz bırakmaz.
func TestKurulum_YasamDongusu(t *testing.T) {
	yerlesikYok(t)
	dataDir := t.TempDir()
	m := havaManifest
	m.Permissions = []string{pluginapi.IzinForum, "bilinmeyen-izin"}
	src := paketKlasoru(t, m, testProgrami(t))
	_ = os.MkdirAll(filepath.Join(src, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(src, ".git", "config"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "README.md"), []byte("# hava"), 0o644)

	p := hazirla(t, src, dataDir, "")
	if p.Manifest.Name != "hava" || p.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("paket = %+v", p)
	}
	if len(p.Manifest.Permissions) != 1 || !strings.Contains(strings.Join(p.Uyarilar, " "), "bilinmeyen-izin") {
		t.Fatalf("izinler/uyarılar = %v / %v", p.Manifest.Permissions, p.Uyarilar)
	}
	if _, err := os.Stat(filepath.Join(p.Kok, ".git")); !os.IsNotExist(err) {
		t.Fatal(".git klasörü kopyalanmamalı")
	}

	k, guncelleme, err := Kur(dataDir, p)
	if err != nil || guncelleme {
		t.Fatalf("Kur: %v (güncelleme=%v)", err, guncelleme)
	}
	if k.Ozet != p.Ozet || k.Program != "and-plugin-hava"+exeEki() {
		t.Fatalf("kayıt = %+v", k)
	}

	bulunan := discoverHarici(dataDir, nil)
	if len(bulunan) != 1 || !bulunan[0].Harici || bulunan[0].Name() != "hava" {
		t.Fatalf("keşif = %+v", bulunan)
	}
	pl := bulunan[0]
	if err := Dogrula(pl); err != nil {
		t.Fatalf("Dogrula: %v", err)
	}
	if kb, ok := pl.KurulumBilgisi(); !ok || kb.Surum != "1.0.0" {
		t.Fatalf("KurulumBilgisi = %+v %v", kb, ok)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(pl.ExePath), "README.md")); err != nil {
		t.Fatal("paketin öteki dosyaları da kurulmalı")
	}

	// Program kurulumdan sonra değişirse başlatılmaz.
	orijinal, _ := os.ReadFile(pl.ExePath)
	if err := os.WriteFile(pl.ExePath, append(append([]byte{}, orijinal...), 0), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Dogrula(pl); err == nil {
		t.Fatal("değişen program reddedilmeli")
	}
	_ = os.WriteFile(pl.ExePath, orijinal, 0o755)
	// plugin.json değişirse de (ör. yeni izin, başka ad) başlatılmaz.
	mYol := filepath.Join(filepath.Dir(pl.ExePath), ManifestDosyasi)
	mVeri, _ := os.ReadFile(mYol)
	_ = os.WriteFile(mYol, bytes.Replace(mVeri, []byte("Hava Durumu"), []byte("Başka Ad"), 1), 0o644)
	if err := Dogrula(pl); err == nil {
		t.Fatal("değişen plugin.json reddedilmeli")
	}
	_ = os.WriteFile(mYol, mVeri, 0o644)
	if err := Dogrula(pl); err != nil {
		t.Fatalf("geri alınınca geçmeli: %v", err)
	}

	if err := Kaldir(dataDir, "hava"); err != nil {
		t.Fatalf("Kaldir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(KullaniciDizini(dataDir), "hava")); !os.IsNotExist(err) {
		t.Fatal("klasör silinmeli")
	}
	if len(Kurulumlar(dataDir)) != 0 || len(discoverHarici(dataDir, nil)) != 0 {
		t.Fatal("kayıt silinmeli")
	}
	if err := Kaldir(dataDir, "hava"); err != ErrKuruluDegil {
		t.Fatalf("ikinci kaldırma = %v", err)
	}
}

// İlk kurulum aynı adla önceden verilmiş izinleri silmeli (başka bir eklenti
// onları devralmasın); güncelleme ise korumalı.
func TestKurulum_IzinOnaylari(t *testing.T) {
	yerlesikYok(t)
	dataDir := t.TempDir()
	if err := IzinOnayla(dataDir, "hava", []string{pluginapi.IzinForum}); err != nil {
		t.Fatal(err)
	}
	st := loadState(dataDir)
	st.Disabled = []string{"hava", "baska"}
	_ = writeState(dataDir, st)

	src := paketKlasoru(t, havaManifest, testProgrami(t))
	if _, _, err := Kur(dataDir, hazirla(t, src, dataDir, "")); err != nil {
		t.Fatal(err)
	}
	if IzinOnayli(dataDir, "hava", havaManifest.Permissions) {
		t.Fatal("ilk kurulum eski izin onayını silmeli")
	}
	if d := loadState(dataDir).Disabled; len(d) != 1 || d[0] != "baska" {
		t.Fatalf("kapalı listesi = %v", d)
	}

	if err := IzinOnayla(dataDir, "hava", havaManifest.Permissions); err != nil {
		t.Fatal(err)
	}
	m := havaManifest
	m.Version = "1.1.0"
	src2 := paketKlasoru(t, m, testProgrami(t))
	k, guncelleme, err := Kur(dataDir, hazirla(t, src2, dataDir, ""))
	if err != nil || !guncelleme || k.Surum != "1.1.0" {
		t.Fatalf("güncelleme: %+v %v %v", k, guncelleme, err)
	}
	if !IzinOnayli(dataDir, "hava", havaManifest.Permissions) {
		t.Fatal("güncelleme izin onayını korumalı")
	}
	if n := len(discoverHarici(dataDir, nil)); n != 1 {
		t.Fatalf("güncellemeden sonra %d eklenti", n)
	}
}

func TestKurulum_YerlesikAdlaCakisma(t *testing.T) {
	yerlesikYok(t, "DeltaX")
	dataDir := t.TempDir()
	m := havaManifest
	m.Name = "deltax"
	src := paketKlasoru(t, m, testProgrami(t))
	if _, _, err := Kur(dataDir, hazirla(t, src, dataDir, "")); err == nil {
		t.Fatal("AND'la gelen bir eklentinin adıyla kurulum reddedilmeli")
	}
	if err := Kaldir(dataDir, "DeltaX"); err == nil || err == ErrKuruluDegil {
		t.Fatalf("yerleşik eklenti kaldırılamaz demeli, dönen: %v", err)
	}
	// Keşifte de yerleşik ad önce gelir.
	yerlesikYok(t)
	if _, _, err := Kur(dataDir, hazirla(t, src, dataDir, "")); err != nil {
		t.Fatal(err)
	}
	if n := len(discoverHarici(dataDir, []Plugin{{Manifest: pluginapi.Manifest{Name: "DeltaX"}}})); n != 0 {
		t.Fatal("yerleşik eklentiyle aynı adlı dış eklenti atlanmalı")
	}
}

// Sürüm arşivlerindeki gibi tek üst klasörlü, çok platformlu bir zip.
func TestHazirla_CokPlatformluZip(t *testing.T) {
	dataDir := t.TempDir()
	ek := exeEki()
	arsiv := zipYaz(t, []arsivGirdisi{
		{ad: "hava-1.0.0/plugin.json", veri: manifestJSON(t, havaManifest)},
		{ad: "hava-1.0.0/and-plugin-hava_" + runtime.GOOS + "_" + runtime.GOARCH + ek, veri: testProgrami(t), kip: 0o755},
		{ad: "hava-1.0.0/and-plugin-hava_plan9_amd64", veri: []byte("başka platform"), kip: 0o755},
		{ad: "hava-1.0.0/LICENSE", veri: []byte("lisans")},
	})
	p := hazirla(t, dosyayaYaz(t, "hava.zip", arsiv), dataDir, "")
	if filepath.Base(p.Program) != "and-plugin-hava"+ek {
		t.Fatalf("program = %s", p.Program)
	}
	if _, err := os.Stat(filepath.Join(p.Kok, "and-plugin-hava_plan9_amd64")); !os.IsNotExist(err) {
		t.Fatal("öteki platformun programı kurulmamalı")
	}
	if p.ArsivOzeti == "" || p.DosyaSayisi != 3 {
		t.Fatalf("özet/dosya sayısı = %q/%d", p.ArsivOzeti, p.DosyaSayisi)
	}
	// Doğru özetle kurulur, yanlışla reddedilir.
	hazirla(t, dosyayaYaz(t, "hava.zip", arsiv), dataDir, strings.ToUpper(p.ArsivOzeti))
	if _, err := Hazirla(dosyayaYaz(t, "hava.zip", arsiv), "", strings.Repeat("0", 64)); err == nil {
		t.Fatal("yanlış SHA-256 reddedilmeli")
	}
}

func TestHazirla_TarGz(t *testing.T) {
	ek := exeEki()
	arsiv := tarGzYaz(t, []arsivGirdisi{
		{ad: "./", klsor: true},
		{ad: "./plugin.json", veri: manifestJSON(t, havaManifest)},
		{ad: "./and-plugin-hava" + ek, veri: testProgrami(t), kip: 0o755},
	})
	p := hazirla(t, dosyayaYaz(t, "hava.tar.gz", arsiv), t.TempDir(), "")
	if p.Manifest.Label != "Hava Durumu" {
		t.Fatalf("manifest = %+v", p.Manifest)
	}
	if runtime.GOOS != "windows" {
		if b, _ := os.Stat(p.Program); b.Mode()&0o111 == 0 {
			t.Fatal("program çalıştırılabilir olmalı")
		}
	}
}

func TestHazirla_GuvensizArsivler(t *testing.T) {
	dis := t.TempDir()
	kacak := filepath.Join(dis, "kacak.txt")
	durumlar := map[string][]byte{
		"üst dizine çıkan zip": zipYaz(t, []arsivGirdisi{{ad: "../" + filepath.Base(dis) + "/kacak.txt", veri: []byte("x")}}),
		"mutlak yollu zip":     zipYaz(t, []arsivGirdisi{{ad: "/tmp/kacak.txt", veri: []byte("x")}}),
		"sürücü harfli zip":    zipYaz(t, []arsivGirdisi{{ad: "C:/kacak.txt", veri: []byte("x")}}),
		"ters bölüdü zip":      zipYaz(t, []arsivGirdisi{{ad: "a\\..\\..\\kacak.txt", veri: []byte("x")}}),
		"bağlantılı tar":       tarGzYaz(t, []arsivGirdisi{{ad: "plugin.json", bag: "/etc/passwd"}}),
		"üst dizine çıkan tar": tarGzYaz(t, []arsivGirdisi{{ad: "../../kacak.txt", veri: []byte("x")}}),
		"arşiv değil":          []byte("sadece metin"),
	}
	for ad, arsiv := range durumlar {
		t.Run(ad, func(t *testing.T) {
			if p, err := Hazirla(dosyayaYaz(t, "paket.zip", arsiv), t.TempDir(), ""); err == nil {
				p.Kapat()
				t.Fatal("reddedilmeliydi")
			}
		})
	}
	if _, err := os.Stat(kacak); !os.IsNotExist(err) {
		t.Fatal("arşiv hedef dizinin dışına yazdı")
	}
}

func TestHazirla_Reddedilenler(t *testing.T) {
	durumlar := []struct {
		ad      string
		m       pluginapi.Manifest
		program []byte
		icerir  string
	}{
		{"boş ad", pluginapi.Manifest{Label: "x"}, nil, "name"},
		{"yol içeren ad", pluginapi.Manifest{Name: "../x"}, nil, "geçersiz eklenti adı"},
		{"boşluklu ad", pluginapi.Manifest{Name: "a b"}, nil, "geçersiz eklenti adı"},
		{"yeni AND isteyen", pluginapi.Manifest{Name: "x", MinAPI: pluginapi.APIVersion + 1}, nil, "daha yeni bir AND"},
		{"program değil", pluginapi.Manifest{Name: "x"}, []byte("#!/bin/sh\necho merhaba\n"), "program değil"},
		{"başka platform", pluginapi.Manifest{Name: "x"}, mipsELF(), "için derlenmiş"},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			dir := t.TempDir()
			_ = os.WriteFile(filepath.Join(dir, ManifestDosyasi), manifestJSON(t, d.m), 0o644)
			if d.program != nil {
				_ = os.WriteFile(filepath.Join(dir, "and-plugin-x"+exeEki()), d.program, 0o755)
			}
			p, err := Hazirla(dir, t.TempDir(), "")
			if err == nil {
				p.Kapat()
				t.Fatal("reddedilmeliydi")
			}
			if !strings.Contains(err.Error(), d.icerir) {
				t.Fatalf("hata %q, %q içermeli", err, d.icerir)
			}
		})
	}
	// Program hiç yoksa beklenen adlar söylenir.
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ManifestDosyasi), manifestJSON(t, havaManifest), 0o644)
	_, err := Hazirla(dir, t.TempDir(), "")
	if err == nil || !strings.Contains(err.Error(), "and-plugin-hava") {
		t.Fatalf("programsız paket: %v", err)
	}
	// Klasörde plugin.json yoksa.
	if _, err := Hazirla(t.TempDir(), "", ""); err == nil {
		t.Fatal("plugin.json'sız klasör reddedilmeli")
	}
}

// mipsELF, MIPS için derlenmiş gibi görünen en küçük ELF başlığıdır.
func mipsELF() []byte {
	b := make([]byte, 64)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 2, 1, 1                // 64 bit, little endian, sürüm 1
	binary.LittleEndian.PutUint16(b[16:], 2)  // ET_EXEC
	binary.LittleEndian.PutUint16(b[18:], 8)  // EM_MIPS
	binary.LittleEndian.PutUint32(b[20:], 1)  // EV_CURRENT
	binary.LittleEndian.PutUint16(b[52:], 64) // e_ehsize
	binary.LittleEndian.PutUint16(b[54:], 56) // e_phentsize
	binary.LittleEndian.PutUint16(b[58:], 64) // e_shentsize
	return b
}

func TestHazirla_Https(t *testing.T) {
	ek := exeEki()
	arsiv := zipYaz(t, []arsivGirdisi{
		{ad: "plugin.json", veri: manifestJSON(t, havaManifest)},
		{ad: "and-plugin-hava" + ek, veri: testProgrami(t), kip: 0o755},
	})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hava.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(arsiv)
	}))
	defer srv.Close()
	eski := indirmeIstemcisi
	indirmeIstemcisi = srv.Client()
	t.Cleanup(func() { indirmeIstemcisi = eski })

	p := hazirla(t, srv.URL+"/hava.zip", t.TempDir(), "")
	if p.Manifest.Name != "hava" || p.ArsivOzeti != baytOzeti(arsiv) {
		t.Fatalf("paket = %+v", p)
	}
	if _, err := Hazirla(srv.URL+"/yok.zip", "", ""); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404: %v", err)
	}
	if _, err := Hazirla(strings.Replace(srv.URL, "https://", "http://", 1)+"/hava.zip", "", ""); err == nil {
		t.Fatal("http adresi reddedilmeli")
	}
}

func TestKurulumlar_BozukKayitKenaraAlinir(t *testing.T) {
	dataDir := t.TempDir()
	dizin := KullaniciDizini(dataDir)
	_ = os.MkdirAll(filepath.Join(dizin, "hava"), 0o755)
	_ = os.WriteFile(filepath.Join(dizin, kayitDosyaAdi), []byte(`{"eklentiler": {`), 0o600)
	if n := len(Kurulumlar(dataDir)); n != 0 {
		t.Fatalf("bozuk kayıt %d eklenti verdi", n)
	}
	if _, err := os.Stat(filepath.Join(dizin, kayitDosyaAdi)); !os.IsNotExist(err) {
		t.Fatal("bozuk kayıt kenara alınmalı")
	}
}

// Kaydı olmayan bir klasör (elle ya da başka bir programın bıraktığı) asla
// eklenti sayılmaz.
func TestDiscoverHarici_KayitsizKlasorAtlanir(t *testing.T) {
	dataDir := t.TempDir()
	klasor := filepath.Join(KullaniciDizini(dataDir), "hava")
	_ = os.MkdirAll(klasor, 0o755)
	_ = os.WriteFile(filepath.Join(klasor, ManifestDosyasi), manifestJSON(t, havaManifest), 0o644)
	_ = os.WriteFile(filepath.Join(klasor, "and-plugin-hava"+exeEki()), []byte("x"), 0o755)
	if n := len(discoverHarici(dataDir, nil)); n != 0 {
		t.Fatalf("kayıtsız klasör %d eklenti verdi", n)
	}
	if err := Dogrula(Plugin{Harici: true, ExePath: filepath.Join(klasor, "and-plugin-hava")}); err == nil {
		t.Fatal("kaydı olmayan dış eklenti doğrulanmamalı")
	}
	if err := Dogrula(Plugin{}); err != nil {
		t.Fatalf("yerleşik eklenti her zaman geçer: %v", err)
	}
}

func TestPlatformUyumlu(t *testing.T) {
	if !platformUyumlu(runtime.GOOS, runtime.GOARCH) || !platformUyumlu("", runtime.GOARCH) {
		t.Fatal("aynı platform uyumlu olmalı")
	}
	if platformUyumlu("plan9", runtime.GOARCH) || platformUyumlu(runtime.GOOS, "mips") {
		t.Fatal("başka platform uyumsuz olmalı")
	}
}

func TestGecerliAd(t *testing.T) {
	for _, ad := range []string{"hava", "DeltaX", "konu_ac", "a-b", "x1"} {
		if !GecerliAd(ad) {
			t.Errorf("%q geçerli olmalı", ad)
		}
	}
	for _, ad := range []string{"", "-x", "_x", "a b", "a/b", "a.b", "..", strings.Repeat("a", 49), "ğ"} {
		if GecerliAd(ad) {
			t.Errorf("%q geçersiz olmalı", ad)
		}
	}
}
