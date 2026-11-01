package main

import (
	"bytes"
	"debug/pe"
	"os"
	"path/filepath"
	"testing"
)

// ornekIco, verilen resimleri taşıyan bir .ico dosyası kurar; genişlik 0, 256 demektir.
func ornekIco(genislikler []uint8, veriler [][]byte) []byte {
	b := make([]byte, 6+16*len(veriler))
	le.PutUint16(b[2:], 1)
	le.PutUint16(b[4:], uint16(len(veriler)))
	konum := len(b)
	for i, v := range veriler {
		g := b[6+16*i:]
		g[0], g[1] = genislikler[i], genislikler[i]
		le.PutUint16(g[4:], 1)
		le.PutUint16(g[6:], 32)
		le.PutUint32(g[8:], uint32(len(v)))
		le.PutUint32(g[12:], uint32(konum))
		konum += len(v)
	}
	for _, v := range veriler {
		b = append(b, v...)
	}
	return b
}

// kaynak, kaynak ağacında bulunan bir veri girdisidir.
type kaynak struct {
	tur, ad, dil uint32
	girdi        uint32 // veri girdisinin bölüm içindeki konumu
	veri         []byte
}

// agacOku, .rsrc bölümündeki üç katlı kaynak ağacını gezer. Veri girdilerindeki
// adres, bağlayıcı bölüm adresini eklemeden önce bölüm içindeki konumdur.
func agacOku(t *testing.T, b []byte) []kaynak {
	t.Helper()
	var sonuc []kaynak
	var gez func(konum uint32, kat int, yol []uint32)
	gez = func(konum uint32, kat int, yol []uint32) {
		adli, kimlikli := le.Uint16(b[konum+12:]), le.Uint16(b[konum+14:])
		if adli != 0 {
			t.Fatalf("%d. katta adlı girdi var; yalnızca kimlik bekleniyordu", kat)
		}
		var onceki uint32
		for i := uint32(0); i < uint32(kimlikli); i++ {
			g := konum + dizinBoyu + i*girdiBoyu
			kimlik, hedef := le.Uint32(b[g:]), le.Uint32(b[g+4:])
			if i > 0 && kimlik <= onceki {
				t.Fatalf("%d. kattaki girdiler kimliğe göre artan sırada değil", kat)
			}
			onceki = kimlik
			alt := hedef&altDizin != 0
			if alt != (kat < 3) {
				t.Fatalf("%d. katta alt dizin işareti yanlış", kat)
			}
			if kat < 3 {
				gez(hedef&^altDizin, kat+1, append(yol, kimlik))
				continue
			}
			adres, boy := le.Uint32(b[hedef:]), le.Uint32(b[hedef+4:])
			sonuc = append(sonuc, kaynak{yol[0], yol[1], kimlik, hedef, b[adres : adres+boy]})
		}
	}
	gez(0, 1, nil)
	return sonuc
}

func TestNesne_KaynakAgaciVeYenidenKonumlandirmalar(t *testing.T) {
	kucuk := bytes.Repeat([]byte{0xa1}, 13)
	buyuk := []byte("\x89PNG büyük resim")
	ico := ornekIco([]uint8{16, 0}, [][]byte{kucuk, buyuk})

	nesneler, err := uret(ico, []string{"amd64", "arm64", "386"})
	if err != nil {
		t.Fatal(err)
	}
	for m, beklenen := range mimariler {
		t.Run(m, func(t *testing.T) {
			f, err := pe.NewFile(bytes.NewReader(nesneler[m]))
			if err != nil {
				t.Fatalf("debug/pe nesneyi okuyamadı: %v", err)
			}
			if f.Machine != beklenen.makine {
				t.Errorf("makine %#x, beklenen %#x", f.Machine, beklenen.makine)
			}
			if len(f.Sections) != 1 || f.Sections[0].Name != ".rsrc" {
				t.Fatalf("tek bir .rsrc bölümü bekleniyordu: %+v", f.Sections)
			}
			bolum := f.Sections[0]
			if bolum.Characteristics != 0x40000040 {
				t.Errorf("bölüm özellikleri %#x", bolum.Characteristics)
			}
			if len(f.COFFSymbols) != 1 {
				t.Fatalf("tek sembol bekleniyordu, %d var", len(f.COFFSymbols))
			}
			s := f.COFFSymbols[0]
			if ad, _ := s.FullName(f.StringTable); ad != ".rsrc" || s.SectionNumber != 1 || s.StorageClass != 3 || s.Type != 0 {
				t.Errorf("bölüm sembolü yanlış: %q %+v", ad, s)
			}

			veri, err := bolum.Data()
			if err != nil {
				t.Fatal(err)
			}
			kaynaklar := agacOku(t, veri)
			if len(kaynaklar) != 3 {
				t.Fatalf("3 kaynak bekleniyordu (2 RT_ICON + RT_GROUP_ICON), %d var", len(kaynaklar))
			}
			for i, v := range [][]byte{kucuk, buyuk} {
				k := kaynaklar[i]
				if k.tur != rtIcon || k.ad != uint32(i+1) || k.dil != 0 || !bytes.Equal(k.veri, v) {
					t.Errorf("%d. RT_ICON yanlış: tür %d, ad %d, dil %d, %d bayt", i+1, k.tur, k.ad, k.dil, len(k.veri))
				}
			}
			grup := kaynaklar[2]
			if grup.tur != rtGroupIcon || grup.ad != 1 {
				t.Fatalf("RT_GROUP_ICON 1 bekleniyordu: tür %d, ad %d", grup.tur, grup.ad)
			}
			g := grup.veri
			if len(g) != 6+14*2 || le.Uint16(g[2:]) != 1 || le.Uint16(g[4:]) != 2 {
				t.Fatalf("grup dizini başlığı yanlış: % x", g)
			}
			for i, v := range [][]byte{kucuk, buyuk} {
				e := g[6+14*i:]
				if e[0] != []uint8{16, 0}[i] || le.Uint32(e[8:]) != uint32(len(v)) || le.Uint16(e[12:]) != uint16(i+1) {
					t.Errorf("grup dizininin %d. girdisi yanlış: % x", i+1, e[:14])
				}
			}

			// Her veri girdisinin adres alanı için tam bir yeniden konumlandırma.
			if len(bolum.Relocs) != len(kaynaklar) {
				t.Fatalf("%d yeniden konumlandırma, beklenen %d", len(bolum.Relocs), len(kaynaklar))
			}
			for i, r := range bolum.Relocs {
				if r.VirtualAddress != kaynaklar[i].girdi || r.SymbolTableIndex != 0 || r.Type != beklenen.yer {
					t.Errorf("%d. yeniden konumlandırma yanlış: %+v (girdi %d)", i, r, kaynaklar[i].girdi)
				}
			}
		})
	}
}

func TestIcoOku_BozukDosya(t *testing.T) {
	gecerli := ornekIco([]uint8{16}, [][]byte{{1, 2, 3}})
	tasan := append([]byte(nil), gecerli...)
	le.PutUint32(tasan[6+8:], 1000)
	for ad, b := range map[string][]byte{
		"boş":            nil,
		"tür imleci":     {0, 0, 2, 0, 1, 0},
		"resimsiz":       {0, 0, 1, 0, 0, 0},
		"eksik dizin":    gecerli[:10],
		"dışarı taşıyor": tasan,
	} {
		if _, err := icoOku(b); err == nil {
			t.Errorf("%s: hata bekleniyordu", ad)
		}
	}
}

// TestNesnelerGuncel: depodaki .syso dosyaları winres/icon.ico'dan üretilenle
// aynı olmalı; simge değişip go generate çalıştırılmazsa test kırılır.
func TestNesnelerGuncel(t *testing.T) {
	ico, err := os.ReadFile(filepath.Join("..", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	nesneler, err := uret(ico, []string{"amd64", "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	for _, dizin := range []string{filepath.Join("..", ".."), filepath.Join("..", "..", "..", "andmod")} {
		for m, beklenen := range nesneler {
			yol := filepath.Join(dizin, "rsrc_windows_"+m+".syso")
			b, err := os.ReadFile(yol)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b, beklenen) {
				t.Errorf("%s eski; 'go generate ./cmd/and ./cmd/andmod' çalıştır", yol)
			}
		}
	}
}
