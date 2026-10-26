// Komut uret, AND'ın ikili dosyalarına giren üçüncü taraf bileşenlerin lisans
// bildirimlerini üretir. internal/lisans paketindeki go:generate satırıyla
// çalışır:
//
//	go generate ./internal/lisans
//
// Bileşen kümesi go.mod'daki gereksinimlerdir. Go, modül grafiğini budadığı
// için (go 1.17+) ana modülün içe aktardığı her paketi sağlayan modülü go.mod'a
// yazar; bu küme her sürüm hedefinde (windows, linux, darwin) ikili dosyalara
// girenlerin üst kümesidir ve hangi işletim sisteminde ya da hangi eklenti
// klasörleri varken üretildiğine bağlı değildir. Go standart kütüphanesi her
// ikili dosyaya girdiği için listenin başına eklenir.
//
// Üretilen dosyalar (internal/lisans altında):
//
//	AGPL-3.0.txt      depo kökündeki LICENSE'ın kopyası (gömülmek için)
//	ucuncu_taraf.txt  özet ve tam lisans metinleri (İngilizce)
//	moduller.go       özet listesi (Go kaynağı)
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// lisansDosyasi, bir bileşenin lisansla ilgili bir dosyasıdır (LICENSE,
// NOTICE, PATENTS …).
type lisansDosyasi struct {
	Ad    string
	Metin string
}

// modul, bildirimdeki bir bileşendir.
type modul struct {
	Yol      string
	Surum    string
	Aciklama string // özette yolun yanında gösterilir (ör. "Go standard library")
	Not      string // elle doğrulanan bileşenlerin açıklaması
	Dosyalar []lisansDosyasi
}

type surumluModul struct {
	Yol   string
	Surum string
}

// grup, aynı lisans metinlerini taşıyan bileşenlerdir; metinler bir kez yazılır.
type grup struct {
	Moduller []modul
	Dosyalar []lisansDosyasi
}

// elleDogrulanan, modülde lisans dosyası bulunmayan bileşenlerdir. Lisansları
// başka bir kaynaktan (README) elle doğrulanmıştır; yeni bir bileşen buraya
// ancak lisansı gerçekten doğrulandıktan sonra eklenir.
var elleDogrulanan = map[string]struct{ ad, not, metin string }{
	"github.com/mattn/go-localereader": {
		ad: "LICENSE (MIT, as stated in README.md)",
		not: `The module contains no license file. Its README.md states "License: MIT" ` +
			`and names Yasuhiro Matsumoto (a.k.a. mattn) as the author; the standard MIT ` +
			`license text is reproduced below on that basis.`,
		metin: mitMetni("Yasuhiro Matsumoto (a.k.a. mattn)"),
	},
}

func mitMetni(sahip string) string {
	return `MIT License

Copyright (c) ` + sahip + `

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`
}

var lisansAdiDeseni = regexp.MustCompile(`(?i)^(licen[cs]e|copying|copyright|notice|patents|unlicense)([-._][a-z0-9._-]+)?$`)

// lisansDosyasiMi, modül kökündeki bir dosyanın bildirime girip girmeyeceğidir.
func lisansDosyasiMi(ad string) bool {
	return !strings.HasSuffix(strings.ToLower(ad), ".go") && lisansAdiDeseni.MatchString(ad)
}

// lisansTurleri, metnin içerdiği lisansların SPDX kısaltmalarıdır (sıralı);
// tanınmayan ya da lisans olmayan metinler (NOTICE, PATENTS) için nil.
func lisansTurleri(metin string) []string {
	m := strings.Join(strings.Fields(strings.ToLower(metin)), " ")
	bas := m[:min(len(m), 300)]
	var bulunan []string
	icerir := func(ifadeler ...string) bool {
		for _, i := range ifadeler {
			if strings.Contains(m, i) {
				return true
			}
		}
		return false
	}

	if strings.Contains(bas, "apache license") ||
		icerir("terms and conditions for use, reproduction, and distribution", "licensed under the apache license, version 2.0") {
		bulunan = append(bulunan, "Apache-2.0")
	}
	mpl := strings.Contains(bas, "mozilla public license") && icerir("version 2.0", "v. 2.0")
	if mpl {
		bulunan = append(bulunan, "MPL-2.0")
	}
	// MPL metni "Secondary License" tanımında GNU lisanslarını adıyla anar; GNU
	// ailesi yalnızca metnin başlığına bakılarak tanınır.
	if !mpl {
		switch {
		case strings.Contains(bas, "gnu affero general public license"):
			bulunan = append(bulunan, "AGPL-3.0")
		case strings.Contains(bas, "gnu lesser general public license"), strings.Contains(bas, "gnu library general public license"):
			bulunan = append(bulunan, "LGPL")
		case strings.Contains(bas, "gnu general public license") && strings.Contains(bas, "version 3"):
			bulunan = append(bulunan, "GPL-3.0")
		case strings.Contains(bas, "gnu general public license"):
			bulunan = append(bulunan, "GPL-2.0")
		}
	}
	if icerir("permission is hereby granted, free of charge, to any person obtaining a copy") {
		bulunan = append(bulunan, "MIT")
	}
	if icerir("permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted",
		"permission to use, copy, modify, and distribute this software for any purpose with or without fee is hereby granted") {
		bulunan = append(bulunan, "ISC")
	}
	if icerir("redistribution and use in source and binary forms") {
		switch {
		case icerir("all advertising materials mentioning"):
			bulunan = append(bulunan, "BSD-4-Clause")
		case icerir("neither the name", "may be used to endorse or promote", "may not be used to endorse or promote"):
			bulunan = append(bulunan, "BSD-3-Clause")
		default:
			bulunan = append(bulunan, "BSD-2-Clause")
		}
	}
	if icerir("this is free and unencumbered software released into the public domain") {
		bulunan = append(bulunan, "Unlicense")
	}
	if icerir("cc0 1.0 universal") {
		bulunan = append(bulunan, "CC0-1.0")
	}
	if icerir("altered source versions must be plainly marked as such") && icerir("this software is provided 'as-is'") {
		bulunan = append(bulunan, "Zlib")
	}
	bulunan = append(bulunan, spdxKimlikleri(metin)...)
	if len(bulunan) == 0 {
		return nil
	}
	slices.Sort(bulunan)
	return slices.Compact(bulunan)
}

var spdxDeseni = regexp.MustCompile(`(?i)SPDX-License-Identifier:\s*([^\n]+)`)

// spdxKimlikleri, metindeki "SPDX-License-Identifier:" ifadelerinde geçen
// lisans kimlikleridir; OR/AND işleçleri ve WITH ile verilen istisnalar atılır.
func spdxKimlikleri(metin string) []string {
	var kimlikler []string
	for _, e := range spdxDeseni.FindAllStringSubmatch(metin, -1) {
		alanlar := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(e[1]))
		for i := 0; i < len(alanlar); i++ {
			k := strings.Trim(alanlar[i], "`'\"<>!-*/")
			switch strings.ToUpper(k) {
			case "", "OR", "AND":
			case "WITH":
				i++ // istisnanın adı (ör. LLVM-exception) bir lisans değildir
			default:
				kimlikler = append(kimlikler, k)
			}
		}
	}
	return kimlikler
}

// lisans, bileşenin bütün dosyalarındaki lisansların birleşimidir
// ("Apache-2.0, MIT"); hiçbiri tanınmadıysa "".
func (m modul) lisans() string {
	var hepsi []string
	for _, d := range m.Dosyalar {
		hepsi = append(hepsi, lisansTurleri(d.Metin)...)
	}
	slices.Sort(hepsi)
	return strings.Join(slices.Compact(hepsi), ", ")
}

// once, sıralamada Go standart kütüphanesini öne alır; diğerleri yola göre dizilir.
func once(a, b string) bool {
	if (a == "go") != (b == "go") {
		return a == "go"
	}
	return a < b
}

// grupla, aynı lisans metinlerini taşıyan bileşenleri bir araya toplar.
func grupla(moduller []modul) []grup {
	anahtar := func(m modul) string {
		var metinler []string
		for _, d := range m.Dosyalar {
			metinler = append(metinler, lfYap(d.Metin))
		}
		slices.Sort(metinler)
		return strings.Join(metinler, "\x00")
	}
	indeks := map[string]int{}
	var gruplar []grup
	for _, m := range moduller {
		k := anahtar(m)
		i, ok := indeks[k]
		if !ok {
			dosyalar := slices.Clone(m.Dosyalar)
			sort.SliceStable(dosyalar, func(a, b int) bool { return dosyalar[a].Ad < dosyalar[b].Ad })
			gruplar = append(gruplar, grup{Dosyalar: dosyalar})
			i = len(gruplar) - 1
			indeks[k] = i
		}
		gruplar[i].Moduller = append(gruplar[i].Moduller, m)
	}
	for i := range gruplar {
		g := gruplar[i].Moduller
		sort.SliceStable(g, func(a, b int) bool { return once(g[a].Yol, g[b].Yol) })
	}
	sort.SliceStable(gruplar, func(a, b int) bool { return once(gruplar[a].Moduller[0].Yol, gruplar[b].Moduller[0].Yol) })
	return gruplar
}

const bildirimBasligi = `AND - THIRD-PARTY SOFTWARE NOTICES
==================================

This file is generated by "go generate ./internal/lisans"; do not edit it by
hand.

AND is licensed under the GNU Affero General Public License, version 3 (see
LICENSE). The AND programs and the plugins built from this repository also
contain the open source components listed below. Each component is
distributed under its own license; the license texts follow the summary. The
source code of every component is available at the listed version from its
module path (for example through https://proxy.golang.org).
`

var (
	ciftCizgi = strings.Repeat("=", 78)
	tekCizgi  = strings.Repeat("-", 78)
)

// bildirimMetni, ucuncu_taraf.txt'nin içeriğidir: özet tablo ve gruplanmış
// lisans metinleri. Çıktı yalnızca girdiye bağlıdır (tarih vb. yok).
func bildirimMetni(moduller []modul) string {
	sirali := slices.Clone(moduller)
	sort.SliceStable(sirali, func(a, b int) bool { return once(sirali[a].Yol, sirali[b].Yol) })

	ad := func(m modul) string {
		if m.Aciklama != "" {
			return m.Yol + " (" + m.Aciklama + ")"
		}
		return m.Yol
	}
	wAd, wSurum := 0, 0
	for _, m := range sirali {
		wAd = max(wAd, len(ad(m)))
		wSurum = max(wSurum, len(m.Surum))
	}

	var b strings.Builder
	b.WriteString(bildirimBasligi + "\n")
	ozet := fmt.Sprintf("SUMMARY: %d components", len(sirali))
	b.WriteString(ozet + "\n" + strings.Repeat("-", len(ozet)) + "\n")
	for _, m := range sirali {
		b.WriteString(strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %s", wAd, ad(m), wSurum, m.Surum, m.lisans()), " ") + "\n")
	}
	b.WriteString("\nLICENSE TEXTS\n-------------\n")
	for _, g := range grupla(sirali) {
		b.WriteString("\n" + ciftCizgi + "\n")
		for _, m := range g.Moduller {
			b.WriteString(m.Yol + " " + m.Surum + "\n")
		}
		for _, m := range g.Moduller {
			if m.Not != "" {
				b.WriteString(sar("Note: "+m.Not, 78, "      ") + "\n")
			}
		}
		for _, d := range g.Dosyalar {
			b.WriteString(tekCizgi + "\n" + d.Ad + "\n\n" + strings.Trim(lfYap(d.Metin), "\n") + "\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// sar, metni genişliğe göre sözcük sınırlarından sarar; sonraki satırlar
// girinti ile başlar.
func sar(metin string, genislik int, girinti string) string {
	var satirlar []string
	satir := ""
	for s := range strings.FieldsSeq(metin) {
		switch {
		case satir == "":
			satir = s
		case len(satir)+1+len(s) > genislik:
			satirlar = append(satirlar, satir)
			satir = girinti + s
		default:
			satir += " " + s
		}
	}
	return strings.Join(append(satirlar, satir), "\n")
}

func lfYap(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// goModGereksinimleri, go.mod'daki require satırlarını (yola göre sıralı) ve
// toolchain yönergesini döndürür.
func goModGereksinimleri(veri []byte) ([]surumluModul, string, error) {
	var gereksinimler []surumluModul
	toolchain := ""
	blokta := false
	for satir := range strings.SplitSeq(lfYap(string(veri)), "\n") {
		if i := strings.Index(satir, "//"); i >= 0 {
			satir = satir[:i]
		}
		alanlar := strings.Fields(satir)
		switch {
		case len(alanlar) == 0:
		case blokta && alanlar[0] == ")":
			blokta = false
		case blokta:
			if len(alanlar) < 2 {
				return nil, "", fmt.Errorf("go.mod: anlaşılamayan require satırı: %q", strings.TrimSpace(satir))
			}
			gereksinimler = append(gereksinimler, surumluModul{alanlar[0], alanlar[1]})
		case alanlar[0] == "require" && len(alanlar) == 2 && alanlar[1] == "(":
			blokta = true
		case alanlar[0] == "require" && len(alanlar) >= 3:
			gereksinimler = append(gereksinimler, surumluModul{alanlar[1], alanlar[2]})
		case alanlar[0] == "toolchain" && len(alanlar) == 2:
			toolchain = alanlar[1]
		}
	}
	sort.Slice(gereksinimler, func(a, b int) bool { return gereksinimler[a].Yol < gereksinimler[b].Yol })
	return gereksinimler, toolchain, nil
}

// modullerGoKaynagi, moduller.go'nun içeriğidir.
func modullerGoKaynagi(moduller []modul) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by \"go run ./uret\" (go generate ./internal/lisans); DO NOT EDIT.\n\n")
	b.WriteString("package lisans\n\n")
	b.WriteString("var moduller = []Modul{\n")
	for _, m := range moduller {
		fmt.Fprintf(&b, "\t{Yol: %q, Surum: %q, Lisans: %q},\n", m.Yol, m.Surum, m.lisans())
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// lisansDosyalari, dizindeki lisans dosyalarıdır (ada göre sıralı).
func lisansDosyalari(dizin string) ([]lisansDosyasi, error) {
	girdiler, err := os.ReadDir(dizin)
	if err != nil {
		return nil, err
	}
	var dosyalar []lisansDosyasi
	for _, g := range girdiler {
		if g.IsDir() || !lisansDosyasiMi(g.Name()) {
			continue
		}
		veri, err := os.ReadFile(filepath.Join(dizin, g.Name()))
		if err != nil {
			return nil, err
		}
		if metin := strings.Trim(lfYap(string(veri)), "\n"); strings.TrimSpace(metin) != "" {
			dosyalar = append(dosyalar, lisansDosyasi{Ad: g.Name(), Metin: metin})
		}
	}
	return dosyalar, nil
}

func goKomutu(dizin string, arg ...string) (string, error) {
	cmd := exec.Command("go", arg...)
	cmd.Dir = dizin
	var hata bytes.Buffer
	cmd.Stderr = &hata
	cikti, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %v: %s", strings.Join(arg, " "), err, strings.TrimSpace(hata.String()))
	}
	return strings.TrimSpace(string(cikti)), nil
}

// modulDizinleri, gereksinimlerin modül önbelleğindeki dizinleridir
// ("yol@sürüm" → dizin). Önbellekte olmayanlar indirilir.
func modulDizinleri(kok string, gereksinimler []surumluModul) (map[string]string, error) {
	arg := []string{"mod", "download", "-json"}
	for _, g := range gereksinimler {
		arg = append(arg, g.Yol+"@"+g.Surum)
	}
	cikti, err := goKomutu(kok, arg...)
	if err != nil {
		return nil, err
	}
	dizinler := map[string]string{}
	dec := json.NewDecoder(strings.NewReader(cikti))
	for {
		var m struct{ Path, Version, Dir, Error string }
		if err := dec.Decode(&m); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		if m.Error != "" {
			return nil, fmt.Errorf("%s@%s: %s", m.Path, m.Version, m.Error)
		}
		dizinler[m.Path+"@"+m.Version] = m.Dir
	}
	return dizinler, nil
}

func yaz(yol string, veri []byte) error {
	if err := os.WriteFile(yol, veri, 0o644); err != nil {
		return err
	}
	fmt.Println("yazıldı:", yol)
	return nil
}

func calistir() error {
	gomod, err := goKomutu("", "env", "GOMOD")
	if err != nil {
		return err
	}
	if gomod == "" || gomod == os.DevNull {
		return errors.New("go.mod bulunamadı; komut AND modülünün içinden çalıştırılmalı")
	}
	kok := filepath.Dir(gomod)
	veri, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	gereksinimler, toolchain, err := goModGereksinimleri(veri)
	if err != nil {
		return err
	}
	surum, err := goKomutu(kok, "env", "GOVERSION")
	if err != nil {
		return err
	}
	if toolchain != "" && surum != toolchain {
		return fmt.Errorf("çalışan Go %s, go.mod'daki toolchain %s; bildirim go.mod'un araç zinciriyle üretilmeli", surum, toolchain)
	}
	goroot, err := goKomutu(kok, "env", "GOROOT")
	if err != nil {
		return err
	}
	dizinler, err := modulDizinleri(kok, gereksinimler)
	if err != nil {
		return err
	}

	std := modul{Yol: "go", Surum: surum, Aciklama: "Go standard library"}
	if std.Dosyalar, err = lisansDosyalari(goroot); err != nil {
		return err
	}
	moduller := []modul{std}
	var gozden []string
	for _, g := range gereksinimler {
		m := modul{Yol: g.Yol, Surum: g.Surum}
		if ist, ok := elleDogrulanan[g.Yol]; ok {
			m.Not, m.Dosyalar = ist.not, []lisansDosyasi{{Ad: ist.ad, Metin: ist.metin}}
		} else if m.Dosyalar, err = lisansDosyalari(dizinler[g.Yol+"@"+g.Surum]); err != nil {
			return err
		}
		if m.lisans() == "" {
			gozden = append(gozden, g.Yol+"@"+g.Surum)
		}
		moduller = append(moduller, m)
	}
	if len(gozden) > 0 {
		return fmt.Errorf("lisansı tanınamayan bileşenler elle incelenmeli (bkz. elleDogrulanan):\n  %s", strings.Join(gozden, "\n  "))
	}

	hedef := filepath.Join(kok, "internal", "lisans")
	agpl, err := os.ReadFile(filepath.Join(kok, "LICENSE"))
	if err != nil {
		return err
	}
	if err := yaz(filepath.Join(hedef, "AGPL-3.0.txt"), []byte(strings.TrimRight(lfYap(string(agpl)), "\n")+"\n")); err != nil {
		return err
	}
	if err := yaz(filepath.Join(hedef, "ucuncu_taraf.txt"), []byte(bildirimMetni(moduller))); err != nil {
		return err
	}
	kaynak, err := modullerGoKaynagi(moduller)
	if err != nil {
		return err
	}
	if err := yaz(filepath.Join(hedef, "moduller.go"), kaynak); err != nil {
		return err
	}
	fmt.Printf("%d bileşen\n", len(moduller))
	return nil
}

func main() {
	if err := calistir(); err != nil {
		fmt.Fprintln(os.Stderr, "lisans/uret:", err)
		os.Exit(1)
	}
}
