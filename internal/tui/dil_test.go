package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/dil/diltest"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
)

// ozelAdlar, çevrilmeyen özel adlardır (eklenti ve tema adları).
var ozelAdlar = []string{"Pembiş"}

// ekranDokumDegiskeni bir dizinse TestEkranlarHerDilde çizdiği ekranları
// oraya düz metin olarak yazar (gözle denetim için):
//
//	AND_EKRAN_DOKUM=/tmp/ekranlar go test ./internal/tui -run HerDilde
const ekranDokumDegiskeni = "AND_EKRAN_DOKUM"

func dilTestModeli(t *testing.T, w, h int) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	id.SetName("alice")
	plugins := []pluginmgr.Plugin{
		{Manifest: pluginapi.Manifest{Name: "ozel_chat", Label: "Özel Chat", Version: "2.1.0", Author: "AND",
			Description: "Eşlerle doğrudan mesajlaş, dosya gönder ve gelen dosya isteklerini onayla",
			Permissions: []string{pluginapi.IzinDM, pluginapi.IzinDosya}},
			ExePath: "and-plugin-ozel_chat.exe", Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "site", Label: "Site", Version: "1.2.0", Author: "AND",
			Description: "Kendi çok sayfalı kişisel sitenizi oluşturun, düzenleyin ve yayınlayın",
			Permissions: []string{pluginapi.IzinSite}},
			ExePath: "and-plugin-site.exe", Enabled: false},
		{Manifest: pluginapi.Manifest{Name: "pembis", Label: "Pembiş Tema", Version: "1.0.0", Category: "theme",
			Description: "AND arayüzünü pembe tonlarda kız temasına boyar"},
			ExePath: "and-plugin-pembis.exe", Enabled: true},
	}
	m := newAppModel(context.Background(), id, nil, plugins, "127.0.0.1:1", nil, nil, nil, t.TempDir(), nil, nil, nil)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mm.(appModel)
}

// dilEkranlari, denetlenen ekranlardır: ad → çizilmiş ekran.
func dilEkranlari(t *testing.T, w, h int) map[string]string {
	t.Helper()
	boyut := tea.WindowSizeMsg{Width: w, Height: h}
	ekranlar := map[string]string{}

	m := dilTestModeli(t, w, h)
	for i, oge := range m.menuItems {
		m.menuIndex = i
		ekranlar["ana_"+strings.ReplaceAll(strings.ToLower(oge), " ", "_")] = m.View()
	}
	m.menuIndex = 0

	m.screen = screenSettings
	for sekme := range settingsTabSayisi {
		m.settingsTab, m.settingsIdx = sekme, 0
		m.syncSettingsVP()
		ekranlar["ayarlar_"+string(rune('0'+sekme))] = m.View()
	}

	m.screen = screenHakkinda
	for sekme := range hakkindaSekmeSayisi {
		m.hakkindaTab = sekme
		m.hakkindaSekmesiAc()
		ekranlar["hakkinda_"+string(rune('0'+sekme))] = m.View()
	}
	m.screen = screenKurallar
	m.syncKurallarVP()
	ekranlar["kurallar"] = m.View()

	m.screen = screenChat
	ekranlar["sohbet"] = m.View()

	mm, _ := m.eklentiAc(m.eklentiIndeksi("ozel_chat"), nil)
	ekranlar["izin"] = mm.(appModel).View()

	fm := newForumModel(context.Background(), nil, m.identity, nil, nil, nil, m.dataDir, true, nil, nil)
	f, _ := fm.Update(boyut)
	ekranlar["forum_kategoriler"] = f.View()

	giris := func(lm loginModel) string {
		g, _ := lm.Update(boyut)
		return g.View()
	}
	ekranlar["giris_kayit"] = giris(newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true))
	ekranlar["giris_kilit"] = giris(newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), false))
	kod := kayitliModel(t)
	ekranlar["giris_kod"] = giris(kod)
	dogrula, _ := kod.goToVerify()
	ekranlar["giris_dogrula"] = giris(dogrula.(loginModel))
	return ekranlar
}

// TestEkranlarHerDilde, ana ekranların her arayüz dilinde tam ekranı
// kapladığını (her satır terminal genişliğinde, satır sayısı yükseklikte)
// ve Türkçe dışındaki dillerde çevrilmemiş metin kalmadığını doğrular.
func TestEkranlarHerDilde(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	dokum := os.Getenv(ekranDokumDegiskeni)
	denetci := diltest.YeniDenetci(t, ozelAdlar)
	for _, d := range dil.Diller {
		dil.Ayarla(d.Kod)
		for _, sz := range [][2]int{{120, 34}, {100, 30}, {80, 24}} {
			for ad, ekran := range dilEkranlari(t, sz[0], sz[1]) {
				satirlar := strings.Split(ekran, "\n")
				if len(satirlar) != sz[1] {
					t.Errorf("%s %s %dx%d: %d satır; %d beklenirdi", d.Kod, ad, sz[0], sz[1], len(satirlar), sz[1])
				}
				for i, s := range satirlar {
					if g := lipgloss.Width(s); g != sz[0] {
						t.Errorf("%s %s %dx%d: satır %d genişliği %d: %q", d.Kod, ad, sz[0], sz[1], i, g, ansi.Strip(s))
					}
				}
				duz := ansi.Strip(ekran)
				for _, k := range denetci.Kalintilar(d.Kod, duz) {
					t.Errorf("%s %s: %s", d.Kod, ad, k)
				}
				if dokum != "" && sz[0] == 100 {
					yol := filepath.Join(dokum, d.Kod+"_"+ad+".txt")
					if err := os.WriteFile(yol, []byte(duz), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

// TestAyarlarDilSekmesi: Dil sekmesine girince etkin dil seçili gelir;
// enter seçilen dili hemen uygular, kaydeder ve yer tutucuları çevirir.
func TestAyarlarDilSekmesi(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	m := dilTestModeli(t, 100, 30)
	m.screen = screenSettings
	tus := func(k tea.KeyType) {
		t.Helper()
		mm, _ := m.handleSettingsKey(tea.KeyMsg{Type: k})
		m = mm.(appModel)
	}
	for range 3 { // Bilgi → Eklentiler → Temalar → Dil
		tus(tea.KeyRight)
	}
	if m.settingsTab != 3 || m.dilIdx != dilSirasi(dil.Kaynak) {
		t.Fatalf("Dil sekmesi: sekme %d, seçili dil %d", m.settingsTab, m.dilIdx)
	}
	tus(tea.KeyDown)
	if dil.Etkin() != dil.Kaynak {
		t.Fatal("ok tuşu dili değiştirmemeli; enter değiştirir")
	}
	tus(tea.KeyEnter)
	if dil.Etkin() != "en" || dil.Yukle(m.dataDir) != "en" {
		t.Fatalf("enter İngilizceyi uygulamalı ve kaydetmeli: etkin %q, kayıtlı %q", dil.Etkin(), dil.Yukle(m.dataDir))
	}
	if m.settingsNotice != "Interface language: English" || m.chatInput.Placeholder != "message…" {
		t.Errorf("bildirim %q, yer tutucu %q", m.settingsNotice, m.chatInput.Placeholder)
	}
	if ekran := ansi.Strip(m.View()); !strings.Contains(ekran, "Settings") || !strings.Contains(ekran, "English") {
		t.Error("Ayarlar ekranı yeni dilde çizilmeli")
	}

	// Sekmeden çıkıp dönünce imleç yine etkin dilde olur.
	tus(tea.KeyLeft)
	tus(tea.KeyRight)
	if m.dilIdx != dilSirasi("en") {
		t.Errorf("sekmeye dönünce seçili dil %d; %d beklenirdi", m.dilIdx, dilSirasi("en"))
	}
}

// TestGirisDilDugmesi: giriş ekranında ctrl+l dili sıradakine çevirir,
// kaydeder ve açık formun yer tutucularını yeni dilde kurar.
func TestGirisDilDugmesi(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	lm := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	lm.dataDir = t.TempDir()
	mm, _ := lm.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	lm = mm.(loginModel)
	if dil.Etkin() != "en" || dil.Yukle(lm.dataDir) != "en" {
		t.Fatalf("ctrl+l İngilizceye geçmeli ve kaydetmeli: etkin %q", dil.Etkin())
	}
	if p := lm.inputs[fieldPass].Placeholder; p != "password" {
		t.Errorf("şifre yer tutucusu %q", p)
	}
	if ekran := ansi.Strip(lm.View()); !strings.Contains(ekran, "Create your identity") || !strings.Contains(ekran, "language: English") {
		t.Error("giriş ekranı yeni dilde çizilmeli")
	}
	for range len(dil.Diller) - 1 {
		mm, _ = lm.Update(tea.KeyMsg{Type: tea.KeyF2})
		lm = mm.(loginModel)
	}
	if dil.Etkin() != dil.Kaynak || lm.inputs[fieldPass].Placeholder != "şifre" {
		t.Errorf("bütün diller dolaşılınca Türkçeye dönmeli: %q", dil.Etkin())
	}
}
