package main

// eklenti.go — "and eklenti …" komutları: dışarıdan eklenti kurma, kaldırma,
// listeleme, bir paketi kurmadan denetleme ve yeni bir eklenti iskeleti
// oluşturma. Hiçbiri AND'ın arayüzünü açmaz; kurulan eklentiler AND'ın bir
// sonraki açılışında menüde görünür.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/eklentisablon"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/pluginui"
)

// eklentiVeriDizini, eklentilerin kurulduğu veri dizinidir; testler
// değiştirir.
var eklentiVeriDizini = veriDiziniYolu

// eklentiBayraklari, bir alt komutun ayrıştırılmış argümanlarıdır.
type eklentiBayraklari struct {
	konum  []string
	evet   bool
	sha256 string
	modul  string
	and    string
	yazar  string
	etiket string
}

// eklentiArgumanlari, bayrakları konumlu argümanlardan ayırır; bayraklar
// argümanların önünde de arkasında da olabilir.
func eklentiArgumanlari(args []string) (eklentiBayraklari, error) {
	var b eklentiBayraklari
	degerli := map[string]*string{
		"--sha256": &b.sha256, "--modul": &b.modul, "--module": &b.modul,
		"--and": &b.and, "--yazar": &b.yazar, "--author": &b.yazar, "--etiket": &b.etiket, "--label": &b.etiket,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			b.konum = append(b.konum, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			b.konum = append(b.konum, a)
			continue
		}
		ad, deger, esittir := strings.Cut(a, "=")
		switch ad {
		case "-e", "--evet", "-y", "--yes":
			b.evet = true
			continue
		}
		hedef, ok := degerli[ad]
		if !ok {
			return b, errors.New(dil.F("bilinmeyen bayrak: %s", a))
		}
		if !esittir {
			if i+1 >= len(args) {
				return b, errors.New(dil.F("%s bir değer ister", ad))
			}
			i++
			deger = args[i]
		}
		*hedef = deger
	}
	return b, nil
}

// eklentiKomutu, "and eklenti <komut>" (İngilizcesi "and plugin <command>")
// komutlarını işler.
func eklentiKomutu(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		_, err := io.WriteString(out, eklentiYardimi())
		return err
	}
	komut := args[0]
	b, err := eklentiArgumanlari(args[1:])
	if err != nil {
		return err
	}
	switch komut {
	case "liste", "list", "ls":
		return eklentiListe(out)
	case "kur", "install":
		if len(b.konum) != 1 {
			return kullanim("kur", kaynakYeri(), "[--sha256 "+yer(dil.T("özet"))+"]", "[-e]")
		}
		return eklentiKur(b.konum[0], b, in, out)
	case "kaldir", "kaldır", "remove", "uninstall":
		if len(b.konum) != 1 {
			return kullanim("kaldir", yer(dil.T("ad")), "[-e]")
		}
		return eklentiKaldir(b.konum[0], b.evet, in, out)
	case "dogrula", "doğrula", "check":
		if len(b.konum) != 1 {
			return kullanim("dogrula", kaynakYeri(), "[--sha256 "+yer(dil.T("özet"))+"]")
		}
		return eklentiDogrula(b.konum[0], b.sha256, out)
	case "yeni", "new":
		if len(b.konum) < 1 || len(b.konum) > 2 {
			return kullanim("yeni", yer(dil.T("ad")), "["+dil.T("klasör")+"]", "[--modul "+yer(dil.T("yol"))+"]", "[--and "+yer(dil.T("klasör"))+"]")
		}
		return eklentiYeni(b, out)
	case "yardim", "yardım", "help", "-h", "--help":
		_, err := io.WriteString(out, eklentiYardimi())
		return err
	}
	return errors.New(dil.F("bilinmeyen eklenti komutu: %s (yardım: %s)", komut, pluginmgr.Komut("yardim")))
}

// yer, komut yazılışındaki bir yer tutucudur: "<ad>".
func yer(s string) string { return "<" + s + ">" }

// kaynakYeri, kurulacak paketin yer tutucusudur. "|" çeviri metinlerinde
// çoğul ayırıcısı olduğu için parçalar ayrı çevrilir.
func kaynakYeri() string {
	return "<" + dil.T("klasör") + "|" + dil.T("arşiv") + "|" + dil.T("https-adresi") + ">"
}

func kullanim(alt string, args ...string) error {
	return errors.New(dil.F("kullanım: %s", pluginmgr.Komut(alt, args...)))
}

func eklentiYardimi() string {
	satir := func(k, a string) string { return fmt.Sprintf("  %-46s %s", k, a) }
	k := func(alt string, args ...string) string {
		return strings.TrimPrefix(pluginmgr.Komut(alt, args...), pluginmgr.Komut("")+" ")
	}
	return strings.Join([]string{
		dil.F("Kullanım: %s %s", pluginmgr.Komut(""), yer(dil.T("komut"))),
		dil.T("Komutların Türkçe ve İngilizce adları aynı işi görür: liste = list, kur = install, kaldir = remove, dogrula = check, yeni = new."),
		"",
		satir(k("liste"), dil.T("kurulu eklentileri listeler")),
		satir(k("kur", kaynakYeri()), dil.T("dışarıdan bir eklenti kurar ya da günceller")),
		satir("    --sha256 "+yer(dil.T("özet")), dil.T("arşivin yayımlanan SHA-256 özetiyle eşleşmesini ister")),
		satir("    -e, --evet, -y, --yes", dil.T("onay sormaz")),
		satir(k("kaldir", yer(dil.T("ad"))), dil.T("dışarıdan kurulmuş bir eklentiyi kaldırır")),
		satir(k("dogrula", kaynakYeri()), dil.T("bir paketi kurmadan denetler")),
		satir(k("yeni", yer(dil.T("ad")), "["+dil.T("klasör")+"]"), dil.T("yeni bir eklenti projesi oluşturur")),
		satir("    --modul "+yer(dil.T("yol")), dil.F("Go modül yolu (varsayılan %s)", "example.com/"+yer(dil.T("ad")))),
		satir("    --and "+yer(dil.T("klasör")), dil.T("AND'ın yerel bir kopyasına karşı geliştir")),
		satir("    --yazar "+yer(dil.T("ad"))+", --etiket "+yer(dil.T("ad")), dil.T("manifest'teki yazar ve menüde görünen ad")),
		"",
		dil.T("Eklenti paketi: plugin.json ile and-plugin-<ad>[.exe] programını taşıyan bir klasör, .zip ya da .tar.gz. Birden çok platform için programlar and-plugin-<ad>_<işletim>_<mimari>[.exe] adını taşır."),
		dil.T("Ayrıntılar: PLUGINS.md (İngilizce), EKLENTILER.md (Türkçe)"),
	}, "\n") + "\n"
}

// eklentiListe, AND'la gelen ve dışarıdan kurulan eklentileri yazar.
func eklentiListe(out io.Writer) error {
	dir, err := eklentiVeriDizini()
	if err != nil {
		return err
	}
	var yerlesik, harici []pluginmgr.Plugin
	for _, p := range pluginmgr.DiscoverWithState(dir) {
		if p.Harici {
			harici = append(harici, p)
		} else {
			yerlesik = append(yerlesik, p)
		}
	}
	tablo := func(baslik string, liste []pluginmgr.Plugin) {
		fmt.Fprintln(out, baslik)
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, p := range liste {
			durum := dil.T("açık")
			if !p.Enabled {
				durum = dil.T("kapalı")
			}
			surum := ""
			if p.Manifest.Version != "" {
				surum = "v" + strings.TrimPrefix(p.Manifest.Version, "v")
			}
			etiket := p.Label()
			if etiket == "" {
				etiket = dil.T("(menüde görünmez)")
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", p.Name(), surum, durum, dil.T(pluginui.Safe(etiket)), izinAdlari(p.Manifest.Permissions))
		}
		_ = tw.Flush()
	}
	if len(yerlesik) > 0 {
		tablo(dil.T("AND'la gelen eklentiler:"), yerlesik)
		fmt.Fprintln(out)
	}
	if len(harici) == 0 {
		fmt.Fprintln(out, dil.F("Dışarıdan kurulmuş eklenti yok. Kurmak için: %s", pluginmgr.Komut("kur", kaynakYeri())))
		return nil
	}
	tablo(dil.F("Dışarıdan kurulanlar (%s):", pluginmgr.KullaniciDizini(dir)), harici)
	return nil
}

// izinAdlari, izinlerin kullanıcıya gösterilen adlarıdır.
func izinAdlari(izinler []string) string {
	var adlar []string
	for _, iz := range izinler {
		if b, ok := pluginapi.IzinBul(iz); ok {
			adlar = append(adlar, dil.T(b.Baslik))
		}
	}
	if len(adlar) == 0 {
		return dil.T("izin istemiyor")
	}
	return strings.Join(adlar, ", ")
}

// paketOzeti, kurulacak ya da denetlenen paketi kullanıcıya anlatır.
func paketOzeti(out io.Writer, p *pluginmgr.Paket) {
	m := p.Manifest
	etiket := m.Label
	if etiket == "" {
		etiket = m.Name
	}
	lw := etiketGenisligi(9, "Eklenti", "Yazar", "Açıklama", "Program", "SHA-256", "Paket", "İzinler")
	kv := func(k, v string) { fmt.Fprintf(out, "%-*s %s\n", lw, dil.T(k)+":", v) }
	surum := ""
	if m.Version != "" {
		surum = " v" + strings.TrimPrefix(m.Version, "v")
	}
	kv("Eklenti", fmt.Sprintf("%s (%s)%s", etiket, m.Name, surum))
	if m.Author != "" {
		kv("Yazar", m.Author)
	}
	if m.Description != "" {
		kv("Açıklama", m.Description)
	}
	kv("Program", fmt.Sprintf("%s  %s  %s", filepath.Base(p.Program), p.Platform, dil.F("%d dosya, %s", p.DosyaSayisi, boyutYazisi(p.Boyut))))
	kv("SHA-256", p.Ozet)
	if p.ArsivOzeti != "" {
		kv("Paket", "SHA-256 "+p.ArsivOzeti)
	}
	if len(m.Permissions) == 0 {
		kv("İzinler", dil.T("izin istemiyor: yalnızca herkese açık içeriği (forum, siteler, genel sohbet, kimliğin) okuyabilir"))
	} else {
		kv("İzinler", "")
		for _, iz := range m.Permissions {
			if b, ok := pluginapi.IzinBul(iz); ok {
				fmt.Fprintf(out, "  ● %s — %s\n", dil.T(b.Baslik), dil.T(b.Aciklama))
			}
		}
	}
	for _, u := range p.Uyarilar {
		fmt.Fprintln(out, "! "+u)
	}
}

// etiketGenisligi, etiketlerin bu dildeki en geniş halinden (ve en az
// enAz'dan) bir sütun genişliği seçer; ":" dahildir.
func etiketGenisligi(enAz int, etiketler ...string) int {
	w := enAz
	for _, e := range etiketler {
		w = max(w, len([]rune(dil.T(e)))+1)
	}
	return w
}

func boyutYazisi(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// onayla, kullanıcıya evet/hayır sorar; yalnızca açık bir "evet" kabul edilir.
func onayla(in io.Reader, out io.Writer, soru string) bool {
	fmt.Fprint(out, soru+" "+dil.T("[e/H]")+" ")
	satir, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(satir)) {
	case "e", "evet", "y", "yes", "j", "ja", "s", "si", "sí", "o", "oui", "д", "да":
		return true
	}
	return false
}

func eklentiKur(kaynak string, b eklentiBayraklari, in io.Reader, out io.Writer) error {
	dir, err := eklentiVeriDizini()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p, err := pluginmgr.Hazirla(kaynak, pluginmgr.KullaniciDizini(dir), b.sha256)
	if err != nil {
		return err
	}
	defer p.Kapat()

	paketOzeti(out, p)
	fmt.Fprintln(out)
	if eski, ok := pluginmgr.Kurulumlar(dir)[p.Manifest.Name]; ok {
		fmt.Fprintln(out, dil.F("%s zaten kurulu (v%s); bu paketle değiştirilecek. Verilen izinler korunur, yeni bir izin istenirse AND yeniden sorar.",
			p.Manifest.Name, strings.TrimPrefix(eski.Surum, "v")))
	}
	fmt.Fprintln(out, dil.T("Bu eklenti AND'la birlikte gelmiyor ve AND geliştiricileri onu incelemedi. Yalnızca güvendiğin kaynaklardan kur."))
	if !b.evet && !onayla(in, out, dil.T("Kurulsun mu?")) {
		fmt.Fprintln(out, dil.T("Kurulmadı."))
		return nil
	}

	k, guncelleme, err := pluginmgr.Kur(dir, p)
	if err != nil {
		return err
	}
	klasor := filepath.Join(pluginmgr.KullaniciDizini(dir), k.Ad)
	if guncelleme {
		fmt.Fprintln(out, "✓ "+dil.F("%s güncellendi: %s", k.Ad, klasor))
	} else {
		fmt.Fprintln(out, "✓ "+dil.F("%s kuruldu: %s", k.Ad, klasor))
	}
	if len(p.Manifest.Permissions) > 0 {
		fmt.Fprintln(out, dil.T("İstediği izinler eklentiyi ilk açtığında sorulacak."))
	}
	if andCalisiyor(dir) {
		fmt.Fprintln(out, dil.T("AND şu an açık: eklentiyi menüde görmek için AND'ı yeniden başlat."))
	}
	return nil
}

func eklentiKaldir(ad string, evet bool, in io.Reader, out io.Writer) error {
	dir, err := eklentiVeriDizini()
	if err != nil {
		return err
	}
	if !evet && !onayla(in, out, dil.F("%s kaldırılsın mı?", ad)) {
		fmt.Fprintln(out, dil.T("Kaldırılmadı."))
		return nil
	}
	if err := pluginmgr.Kaldir(dir, ad); err != nil {
		if errors.Is(err, pluginmgr.ErrKuruluDegil) {
			return errors.New(dil.F("%s adlı dışarıdan kurulmuş bir eklenti yok (liste: %s)", ad, pluginmgr.Komut("liste")))
		}
		return err
	}
	fmt.Fprintln(out, "✓ "+dil.F("%s kaldırıldı.", ad))
	fmt.Fprintln(out, dil.F("Eklentinin kendi kaydettiği dosyalar silinmedi; varsa %s içindedir.", dir))
	return nil
}

func eklentiDogrula(kaynak, sha string, out io.Writer) error {
	p, err := pluginmgr.Hazirla(kaynak, "", sha)
	if err != nil {
		return err
	}
	defer p.Kapat()
	paketOzeti(out, p)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "✓ "+dil.T("Paket bu bilgisayara kurulabilir."))
	return nil
}

func eklentiYeni(b eklentiBayraklari, out io.Writer) error {
	ad := b.konum[0]
	hedef := ad
	if len(b.konum) == 2 {
		hedef = b.konum[1]
	}
	if b.and != "" {
		if _, err := eklentisablon.ANDKaynagiDenetle(b.and); err != nil {
			return err
		}
	}
	yazilan, err := eklentisablon.Olustur(hedef, eklentisablon.Secenekler{
		Ad: ad, Etiket: b.etiket, Modul: b.modul, Yazar: b.yazar, ANDKaynak: b.and,
	})
	if err != nil {
		return err
	}
	ek := ""
	if filepath.Separator == '\\' {
		ek = ".exe"
	}
	fmt.Fprintln(out, "✓ "+dil.F("%s eklentisi oluşturuldu: %s (%d dosya)", ad, hedef, len(yazilan)))
	fmt.Fprintln(out)
	fmt.Fprintln(out, dil.T("Sonraki adımlar:"))
	for _, s := range []string{
		"cd " + hedef,
		"go mod tidy",
		"go run . --deneme          # " + dil.T("AND olmadan, sahte verilerle dene"),
		"go test ./...",
		"go build -o and-plugin-" + ad + ek + " .",
		pluginmgr.Komut("kur", ".") + strings.Repeat(" ", max(1, 27-len(pluginmgr.Komut("kur", ".")))) + "# " + dil.T("AND'a kur"),
	} {
		fmt.Fprintln(out, "  "+s)
	}
	return nil
}

// andCalisiyor, bu veri dizinini şu an bir AND'ın kullanıp kullanmadığıdır.
func andCalisiyor(dir string) bool {
	k, err := dosya.KilitAl(dir)
	if err != nil {
		return errors.Is(err, dosya.ErrKilitli)
	}
	_ = k.Birak()
	return false
}
