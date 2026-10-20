package main

// smoke_test.go — model'in temel oluşturma/render/tuş-işleme akışının
// panik atmadığını doğrulayan hafif bir duman testi.

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+y":
		return tea.KeyMsg{Type: tea.KeyCtrlY}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func testModel(t *testing.T) model {
	t.Helper()
	depo, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = depo.Kapat() })
	k, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := testHost(t)
	return yeniModel(context.Background(), h, depo, k)
}

func TestModel_InitViewUpdate_AnaMenu_NoPanic(t *testing.T) {
	m := testModel(t)

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	for _, key := range []string{"down", "up", "esc"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}

func TestModel_AramaEkraniYazmaVeGezinme_NoPanic(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama

	for _, r := range "merhaba dünya" {
		next, cmd := m.Update(keyMsg(string(r)))
		m = next.(model)
		if cmd != nil {
			_ = cmd()
		}
		_ = m.View()
	}

	for _, key := range []string{"down", "up", "enter", "esc"} {
		next, _ := m.Update(keyMsg(key))
		m = next.(model)
		_ = m.View()
	}
}

func TestModel_DurumEkrani_NoPanic(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDurum
	_ = m.View()

	next, _ := m.Update(keyMsg("esc"))
	m = next.(model)
	_ = m.View()
}

// TestModel_DetayEkraniBaglantilar_NoPanic, bir sonucun gövdesinde
// bağlantı varken detay ekranının render'ının ve o/1-9 tuş işleyicisinin
// panik atmadığını doğrular. baglantiAcCmd'nin DÖNDÜĞÜ Cmd bilerek hiç
// ÇAĞRILMIYOR — çağrılması gerçekten bir tarayıcı açardı (gerçek bir yan
// etki), bu bir birim testinde uygun değil; bkz. links_test.go'nun
// baglantiAc'ın kendisini (URL doğrulama mantığını) ayrıca test etmesi.
func TestModel_DetayEkraniBaglantilar_NoPanic(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{
		Tip: TipKonu, KaynakID: "p1", Baslik: "başlık",
		Govde: "bak https://example.com/bir ve https://example.com/iki",
	}
	m.seciliSonuc = &s
	m.baglantilar = baglantilariBul(s.Govde)
	if len(m.baglantilar) != 2 {
		t.Fatalf("2 bağlantı bekleniyordu, %d geldi", len(m.baglantilar))
	}
	_ = m.View()

	for _, key := range []string{"1", "2", "9", "o"} {
		next, cmd := m.Update(keyMsg(key))
		mm, ok := next.(model)
		if !ok {
			t.Fatalf("beklenmeyen mesaj tipi: %T", next)
		}
		m = mm
		if key == "1" || key == "2" || key == "o" {
			if cmd == nil {
				t.Errorf("%q için bir Cmd üretilmeliydi (geçerli bir bağlantı indeksi)", key)
			}
		} else if cmd != nil {
			t.Errorf("%q için Cmd üretilmemeliydi (indeks dışı)", key)
		}
		_ = m.View()
	}

	next, _ := m.Update(keyMsg("esc"))
	m = next.(model)
	_ = m.View()
}

func TestModel_GorunenSonuclar_YerelVeAgBirlestirir(t *testing.T) {
	m := testModel(t)
	m.yerelSonuclar = []AramaSonucu{{KaynakID: "yerel1"}}
	m.agSonuclari = []AramaSonucu{{KaynakID: "ag1", KaynakPeer: "peerX"}}

	liste := m.gorunenSonuclar()
	if len(liste) != 2 {
		t.Fatalf("2 sonuç bekleniyordu, %d geldi", len(liste))
	}
	if liste[0].KaynakID != "yerel1" || liste[1].KaynakID != "ag1" {
		t.Fatalf("beklenmeyen sıra: %+v", liste)
	}
}

// TestModel_GorunenSonuclar_AyniKaynakIDTekillestirilir, aynı içeriğin
// hem yerel indekste hem de bir ağ yanıtında (ör. o eş de aynı gönderiyi
// zaten senkronize etmişse) görünmesi durumunda İKİ KEZ listelenmediğini
// doğrular — yerel kopya önceliklidir.
func TestModel_GorunenSonuclar_AyniKaynakIDTekillestirilir(t *testing.T) {
	m := testModel(t)
	m.yerelSonuclar = []AramaSonucu{{KaynakID: "ortak", Baslik: "yerel kopya"}}
	m.agSonuclari = []AramaSonucu{{KaynakID: "ortak", Baslik: "ağdan gelen kopya", KaynakPeer: "peerX"}}

	liste := m.gorunenSonuclar()
	if len(liste) != 1 {
		t.Fatalf("tekilleştirme sonrası 1 sonuç bekleniyordu, %d geldi: %+v", len(liste), liste)
	}
	if liste[0].KaynakPeer != "" {
		t.Fatalf("yerel kopya öncelikli olmalıydı (KaynakPeer boş), geldi: %q", liste[0].KaynakPeer)
	}
}

// TestModel_TabTusuFiltreyiDegistirir, "tab" tuşunun filtreyi
// Tümü→Konu→Yanıt→Sohbet→Site→Tümü döngüsünde ilerlettiğini doğrular.
func TestModel_TabTusuFiltreyiDegistirir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama

	beklenen := []SonucTip{TipKonu, TipYanit, TipSohbet, TipSite, ""}
	for i, b := range beklenen {
		next, cmd := m.Update(keyMsg("tab"))
		m = next.(model)
		if cmd != nil {
			_ = cmd()
		}
		if m.filtre != b {
			t.Fatalf("adım %d: filtre %q bekleniyordu, %q geldi", i, b, m.filtre)
		}
		_ = m.View()
	}
}

// TestModel_ADetayindaYazarFiltresiUygular, Detay ekranındaki 'a' tuşunun
// seçili sonucun yazarına göre filtrelenmiş bir aramaya geçtiğini ve
// Arama ekranının bunu (anahtar kelimesiz bile) gösterebildiğini
// doğrular — "bu yazarın diğer içerikleri" özelliği.
func TestModel_ADetayindaYazarFiltresiUygular(t *testing.T) {
	m := testModel(t)
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "birinci", Govde: "gövde 1", YazarAd: "ali", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "sohbet", YazarAd: "ali", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "başkası", Govde: "gövde 2", YazarAd: "veli", YazarKey: "anahtar-veli"}); err != nil {
		t.Fatal(err)
	}

	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "birinci", YazarAd: "ali", YazarKey: "anahtar-ali"}
	m.seciliSonuc = &s

	next, cmd := m.Update(keyMsg("a"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("'a' bir Cmd üretmeliydi (yazar filtreli yerel arama)")
	}
	msg := cmd()
	next2, _ := m.Update(msg)
	m = next2.(model)

	if m.ekran != ekArama {
		t.Fatalf("'a' sonrası Arama ekranına geçilmeliydi, got %v", m.ekran)
	}
	if m.yazarFiltre != "anahtar-ali" || m.yazarFiltreAd != "ali" {
		t.Fatalf("yazar filtresi ayarlanmalıydı, got yazarFiltre=%q yazarFiltreAd=%q", m.yazarFiltre, m.yazarFiltreAd)
	}
	liste := m.gorunenSonuclar()
	if len(liste) != 2 {
		t.Fatalf("yazar filtresiyle (anahtar kelimesiz) 2 sonuç bekleniyordu, %d geldi: %+v", len(liste), liste)
	}
	_ = m.View()

	// ctrl+y filtreyi temizlemeli.
	next3, _ := m.Update(keyMsg("ctrl+y"))
	m = next3.(model)
	if m.yazarFiltre != "" {
		t.Fatalf("ctrl+y sonrası yazar filtresi temizlenmeliydi, got %q", m.yazarFiltre)
	}
}
