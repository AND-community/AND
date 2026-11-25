package pluginapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEklentiKategorisiBul(t *testing.T) {
	k, sira := EklentiKategorisiBul(KategoriYonetim)
	if k.Baslik != "Yönetim" || sira != 2 {
		t.Fatalf("moderation = %+v %d", k, sira)
	}
	for _, ad := range []string{"", "bilinmeyen", "Theme"} {
		if k, sira := EklentiKategorisiBul(ad); k != DigerKategori || sira != len(EklentiKategorileri) {
			t.Errorf("%q = %+v %d; Diğer ve en son olmalı", ad, k, sira)
		}
	}
	gorulen := map[string]bool{}
	for _, k := range EklentiKategorileri {
		if k.Ad == "" || gorulen[k.Ad] {
			t.Errorf("kategori değeri boş ya da yinelenmiş: %q", k.Ad)
		}
		gorulen[k.Ad] = true
	}
}

// TestPaketleEklentilerinKategorisiVar: AND'la gelen her eklentinin
// plugin.json'u bilinen bir kategori bildirir; menüde "Diğer"e düşmez.
func TestPaketleEklentilerinKategorisiVar(t *testing.T) {
	dosyalar, err := filepath.Glob(filepath.Join("..", "Eklentiler", "*", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dosyalar) == 0 {
		t.Skip("Eklentiler klasörü yok")
	}
	for _, d := range dosyalar {
		veri, err := os.ReadFile(d)
		if err != nil {
			t.Fatal(err)
		}
		var m Manifest
		if err := json.Unmarshal(veri, &m); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if k, _ := EklentiKategorisiBul(m.Category); k == DigerKategori {
			t.Errorf("%s: kategori %q bilinmiyor (bkz. EklentiKategorileri)", d, m.Category)
		}
	}
}

// TestForumEklentisiAndlaGelir: forum bir eklentidir ve AND'la gelir;
// yalnızca forum iznini ister, masaüstünde arayüz kipinde açılır.
func TestForumEklentisiAndlaGelir(t *testing.T) {
	veri, err := os.ReadFile(filepath.Join("..", "Eklentiler", "forum", "plugin.json"))
	if err != nil {
		t.Fatalf("Eklentiler/forum/plugin.json: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(veri, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "forum" || m.Label == "" || m.Category != KategoriIletisim || m.UI != 1 ||
		len(m.Permissions) != 1 || m.Permissions[0] != IzinForum {
		t.Fatalf("forum manifesti: %+v", m)
	}
}
