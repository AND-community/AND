package tui

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
)

// turkceHarfler yalnızca Türkçede (desteklenen diller arasında) bulunan
// harflerdir: başka bir dilde çizilen ekranda görünürlerse bir metin
// çevrilmeden kalmıştır.
const turkceHarfler = "ğĞıİşŞ"

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

// katalogKaynaklari, dil kodu → ekranda o dilde görünmemesi gereken Türkçe
// kaynak metinlerdir: çevirisi kendisinden farklı, en az dört harfli ve o
// dildeki hiçbir çeviride kelime olarak geçmeyen (ör. Fransızca "Site web"
// içindeki "Site") katalog metinleri. Biçim fiili içerenler ekranda olduğu
// gibi görünmediği için atlanır.
func katalogKaynaklari(t *testing.T) map[string][]string {
	t.Helper()
	dosyalar, err := filepath.Glob(filepath.Join("..", "..", "dil", "ceviriler", "*.json"))
	if err != nil || len(dosyalar) == 0 {
		t.Fatalf("çeviri dosyaları bulunamadı: %v", err)
	}
	katalog := map[string]map[string]string{}
	for _, d := range dosyalar {
		veri, err := os.ReadFile(d)
		if err != nil {
			t.Fatal(err)
		}
		var k map[string]map[string]string
		if err := json.Unmarshal(veri, &k); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		maps.Copy(katalog, k)
	}
	kaynaklar := map[string][]string{}
	for _, d := range dil.Diller[1:] {
		var ceviriler []string
		for _, c := range katalog {
			ceviriler = append(ceviriler, c[d.Kod])
		}
		for kaynak, c := range katalog {
			k := strings.TrimSpace(kaynak)
			if strings.Contains(k, "%") || len([]rune(k)) < 4 || c[d.Kod] == kaynak {
				continue
			}
			ortak := false
			for _, ceviri := range ceviriler {
				if kelimeOlarakGecer(ceviri, k) {
					ortak = true
					break
				}
			}
			if !ortak {
				kaynaklar[d.Kod] = append(kaynaklar[d.Kod], k)
			}
		}
	}
	return kaynaklar
}

// kelimeOlarakGecer, kelimenin metinde iki yanı harf olmayan bir yerde
// geçip geçmediğidir ("Program", "Programm" içinde geçmez).
func kelimeOlarakGecer(metin, kelime string) bool {
	for i := 0; i < len(metin); {
		j := strings.Index(metin[i:], kelime)
		if j < 0 {
			return false
		}
		bas, son := i+j, i+j+len(kelime)
		onceki, _ := utf8.DecodeLastRuneInString(metin[:bas])
		sonraki, _ := utf8.DecodeRuneInString(metin[son:])
		if !unicode.IsLetter(onceki) && !unicode.IsLetter(sonraki) {
			return true
		}
		i = bas + 1
	}
	return false
}

// TestEkranlarHerDilde, ana ekranların her arayüz dilinde tam ekranı
// kapladığını (her satır terminal genişliğinde, satır sayısı yükseklikte)
// ve Türkçe dışındaki dillerde çevrilmemiş metin kalmadığını doğrular.
func TestEkranlarHerDilde(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	dokum := os.Getenv(ekranDokumDegiskeni)
	yasakli := katalogKaynaklari(t)
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
				if d.Kod != dil.Kaynak {
					denetlenen := duz
					for _, o := range ozelAdlar {
						denetlenen = strings.ReplaceAll(denetlenen, o, "")
					}
					for _, s := range strings.Split(denetlenen, "\n") {
						if strings.ContainsAny(s, turkceHarfler) {
							t.Errorf("%s %s: çevrilmemiş metin: %q", d.Kod, ad, strings.TrimSpace(s))
						}
						for _, k := range yasakli[d.Kod] {
							if kelimeOlarakGecer(s, k) {
								t.Errorf("%s %s: %q çevrilmemiş: %q", d.Kod, ad, k, strings.TrimSpace(s))
							}
						}
					}
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
