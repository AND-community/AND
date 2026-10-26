package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/lisans"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/updater"
)

func hakkindaModeli(t *testing.T, w, h int, dataDir string) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName("alice")
	m := newAppModel(context.Background(), id, nil, []pluginmgr.Plugin{}, "", nil, nil, nil, dataDir, nil, nil, nil)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mm.(appModel)
}

func bas(t *testing.T, m appModel, k tea.KeyMsg) (appModel, tea.Cmd) {
	t.Helper()
	mm, cmd := m.Update(k)
	return mm.(appModel), cmd
}

// menudenHakkindayaGit, ana menüde "AND hakkında"yı seçip enter'a basar.
func menudenHakkindayaGit(t *testing.T, m appModel) appModel {
	t.Helper()
	i := slices.Index(m.menuItems, "AND hakkında")
	if i < 0 {
		t.Fatalf("ana menüde AND hakkında yok: %v", m.menuItems)
	}
	m.menuIndex = i
	m, _ = bas(t, m, keyMsg("enter"))
	if m.screen != screenHakkinda {
		t.Fatalf("enter AND hakkında ekranını açmadı (ekran %d)", m.screen)
	}
	return m
}

func sekmeyeGit(t *testing.T, m appModel, sekme int) appModel {
	t.Helper()
	for range 10 {
		if m.hakkindaTab == sekme {
			return m
		}
		m, _ = bas(t, m, tea.KeyMsg{Type: tea.KeyRight})
	}
	t.Fatalf("→ ile %d. sekmeye ulaşılamadı (sekme %d)", sekme, m.hakkindaTab)
	return m
}

// duzMetin, çizilmiş ekranın çerçevesiz, boşlukları tekleştirilmiş metnidir:
// satır sonuna sarılan cümleler de aranabilir.
func duzMetin(ekran string) string {
	ekran = strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ").Replace(ansi.Strip(ekran))
	return strings.Join(strings.Fields(ekran), " ")
}

func TestHakkindaGenelSekmesiYasalBildirimiGosterir(t *testing.T) {
	m := menudenHakkindayaGit(t, hakkindaModeli(t, 100, 40, t.TempDir()))
	ekran := duzMetin(m.View())
	for _, parca := range []string{
		lisans.Ad, lisans.KaynakKodu, updater.Version,
		dil.T("Telif hakkı (C) 2026 AND katkıcıları"),
		dil.T("Yasaların izin verdiği ölçüde HİÇBİR GARANTİSİ YOKTUR."),
	} {
		if !strings.Contains(ekran, parca) {
			t.Errorf("Genel sekmesi %q göstermiyor:\n%s", parca, ekran)
		}
	}
}

func TestHakkindaSekmeleriLisansMetinleriniGosterir(t *testing.T) {
	m := menudenHakkindayaGit(t, hakkindaModeli(t, 100, 40, t.TempDir()))

	m = sekmeyeGit(t, m, hakkindaSekmeLisans)
	if ekran := duzMetin(m.View()); !strings.Contains(ekran, "GNU AFFERO GENERAL PUBLIC LICENSE") {
		t.Errorf("Lisans sekmesi AGPL metnini göstermiyor:\n%s", ekran)
	}
	m = sekmeyeGit(t, m, hakkindaSekmeUcuncuTaraf)
	ilk := lisans.Moduller()[1] // Go standart kütüphanesinden sonraki ilk bileşen
	if ekran := duzMetin(m.View()); !strings.Contains(ekran, ilk.Yol) {
		t.Errorf("Üçüncü taraf sekmesi %s bileşenini göstermiyor:\n%s", ilk.Yol, ekran)
	}
}

func TestHakkindaUzunMetinKaydirilir(t *testing.T) {
	m := sekmeyeGit(t, menudenHakkindayaGit(t, hakkindaModeli(t, 80, 24, t.TempDir())), hakkindaSekmeLisans)
	if m.hakkindaVP.YOffset != 0 {
		t.Fatalf("sekme başta açılmadı: YOffset %d", m.hakkindaVP.YOffset)
	}
	m, _ = bas(t, m, keyMsg("pgdown"))
	if m.hakkindaVP.YOffset == 0 {
		t.Error("pgdown lisans metnini kaydırmadı")
	}
	// Sekme değişince yeni metin baştan gösterilir.
	m, _ = bas(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.hakkindaVP.YOffset != 0 {
		t.Errorf("yeni sekme kaydırılmış açıldı: YOffset %d", m.hakkindaVP.YOffset)
	}
}

func TestHakkindaEscAnaMenuyeDoner(t *testing.T) {
	m := menudenHakkindayaGit(t, hakkindaModeli(t, 100, 40, t.TempDir()))
	if m, _ = bas(t, m, keyMsg("esc")); m.screen != screenMenu {
		t.Errorf("esc ana menüye dönmedi (ekran %d)", m.screen)
	}
}

func TestAnaMenuHakkindaAyrintisiLisansiGosterir(t *testing.T) {
	m := hakkindaModeli(t, 140, 40, "")
	m.menuIndex = slices.Index(m.menuItems, "AND hakkında")
	ekran := duzMetin(m.viewMenu())
	for _, parca := range []string{lisans.Ad, lisans.KaynakKodu} {
		if !strings.Contains(ekran, parca) {
			t.Errorf("ana menü ayrıntısı %q göstermiyor:\n%s", parca, ekran)
		}
	}
}

func TestKurallarOnaylanmadanAcilistaOnayEkraniGelir(t *testing.T) {
	m := hakkindaModeli(t, 100, 30, t.TempDir()).kurallarOnayiIste()
	if m.screen != screenKurallar {
		t.Fatalf("onay ekranı gösterilmedi (ekran %d)", m.screen)
	}
	// Onay ekranı tuşlarla geçilemez: aşağı ok kabul sayılmaz.
	if m, _ = bas(t, m, keyMsg("down")); m.screen != screenKurallar {
		t.Error("aşağı ok onay ekranını kapattı")
	}
}

func TestKurallarKabulEdilinceKaydedilir(t *testing.T) {
	// Metnin tamamı ekrana sığar; kabul hemen geçerlidir.
	for _, tus := range []tea.KeyMsg{harf("e"), harf("y"), keyMsg("enter")} {
		dir := t.TempDir()
		m := hakkindaModeli(t, 120, 80, dir).kurallarOnayiIste()
		if m, _ = bas(t, m, tus); m.screen != screenMenu {
			t.Errorf("%s ile kabul ana menüye geçmedi (ekran %d)", tus, m.screen)
		}
		if !kurallarOnaylandi(dir) {
			t.Errorf("%s ile kabul kaydedilmedi", tus)
		}
		if s := hakkindaModeli(t, 120, 80, dir).kurallarOnayiIste().screen; s != screenMenu {
			t.Errorf("%s ile kabulden sonraki açılışta yine soruldu", tus)
		}
	}
}

func TestKurallarSonunaKadarOkunmadanKabulEdilemez(t *testing.T) {
	dir := t.TempDir()
	m := hakkindaModeli(t, 80, 24, dir).kurallarOnayiIste()
	if m.kurallarVP.AtBottom() {
		t.Fatal("bu boyutta metin ekrana sığmamalıydı")
	}
	if m, _ = bas(t, m, harf("e")); m.screen != screenKurallar || kurallarOnaylandi(dir) {
		t.Fatal("kurallar sonuna kadar okunmadan kabul edildi")
	}
	if !strings.Contains(duzMetin(m.View()), dil.T("Kabul etmeden önce metnin sonuna kadar kaydır.")) {
		t.Error("neden kabul edilmediği söylenmedi")
	}
	for i := 0; !m.kurallarVP.AtBottom(); i++ {
		if i > 100 {
			t.Fatal("pgdown metnin sonuna ulaşmadı")
		}
		m, _ = bas(t, m, keyMsg("pgdown"))
	}
	if m, _ = bas(t, m, harf("e")); m.screen != screenMenu || !kurallarOnaylandi(dir) {
		t.Error("sonuna kadar okunduktan sonra kabul geçmedi")
	}
}

func TestKurallarKabulEdilmedenCikilir(t *testing.T) {
	dir := t.TempDir()
	m := hakkindaModeli(t, 100, 30, dir).kurallarOnayiIste()
	_, cmd := bas(t, m, harf("q"))
	if cmd == nil {
		t.Fatal("q çıkış komutu döndürmedi")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q AND'dan çıkmadı")
	}
	if kurallarOnaylandi(dir) {
		t.Error("kabul edilmeden onay kaydedildi")
	}
}

func TestKurallarOnayiEskiSurumdeYaDaBozuksaGecersiz(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, kurallarOnayiDosyasi)
	for _, icerik := range []string{`{"surum":0,"zaman":"2026-01-01T00:00:00Z"}`, `bozuk`, ``} {
		if err := os.WriteFile(yol, []byte(icerik), 0o600); err != nil {
			t.Fatal(err)
		}
		if kurallarOnaylandi(dir) {
			t.Errorf("%q içerikli onay dosyası geçerli sayıldı", icerik)
		}
	}
}
