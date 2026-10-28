package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
)

// taslakTestModeli, dataDir'i kullanan ve "konu1" konusunu açmış bir forum
// ekranı kurar. Forum gerçektir (ağsız): yalnızca yanıt listesi okunur.
func taslakTestModeli(t *testing.T, dataDir string) forumModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	f, err := forum.New(id, nil, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	fm := newForumModel(context.Background(), f, id, nil, nil, nil, dataDir, false, nil, nil)
	sized, _ := fm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := sized.(forumModel)
	m = m.konuyaGir(&forum.Post{ID: "konu1", Title: "Başlık", Category: "Go"})
	return m
}

func (m forumModel) konuyaGir(p *forum.Post) forumModel {
	m.aktifKonu = p
	m.ekran = fEkKonu
	return m
}

func basla(t *testing.T, m forumModel, tuslar ...tea.KeyMsg) forumModel {
	t.Helper()
	for _, k := range tuslar {
		yeni, _ := m.Update(k)
		m = yeni.(forumModel)
	}
	return m
}

func harfler(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	tusEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	tusCtrlS = tea.KeyMsg{Type: tea.KeyCtrlS}
	tusCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
)

// TestYanitTaslagi_EscVeYenidenYanitlaKorunur: esc ile konuya dönüp yeniden
// "r"ye basmak yazılanı silmez. (Eskiden "r" kutuyu her seferinde
// boşaltıyordu: esc'ye yanlışlıkla basan kullanıcının uzun yanıtı
// kayboluyordu.)
func TestYanitTaslagi_EscVeYenidenYanitlaKorunur(t *testing.T) {
	m := taslakTestModeli(t, t.TempDir())
	m = basla(t, m, harfler("r"), harfler("uzun bir yanıt"), tusEsc)
	if m.ekran != fEkKonu {
		t.Fatalf("esc konuya dönmeli, ekran %v", m.ekran)
	}
	m = basla(t, m, harfler("r"))
	if got := m.yanitAlan.Value(); got != "uzun bir yanıt" {
		t.Fatalf("taslak geri gelmeli, kutu: %q", got)
	}
}

// TestYanitTaslagi_KonularKarismaz: her konunun taslağı ayrıdır.
func TestYanitTaslagi_KonularKarismaz(t *testing.T) {
	m := taslakTestModeli(t, t.TempDir())
	m = basla(t, m, harfler("r"), harfler("birinci konuya"), tusEsc)
	m = m.konuyaGir(&forum.Post{ID: "konu2", Title: "İkinci", Category: "Go"})
	m = basla(t, m, harfler("r"))
	if got := m.yanitAlan.Value(); got != "" {
		t.Fatalf("başka konunun taslağı gelmemeli, kutu: %q", got)
	}
	m = basla(t, m, tusEsc)
	m = m.konuyaGir(&forum.Post{ID: "konu1", Title: "Başlık", Category: "Go"})
	m = basla(t, m, harfler("r"))
	if got := m.yanitAlan.Value(); got != "birinci konuya" {
		t.Fatalf("konu1'in taslağı geri gelmeli, kutu: %q", got)
	}
}

// TestYanitTaslagi_YenidenBaslatmadaGeriGelir: taslak diske yazılır; AND
// kapanıp açılınca (ya da ctrl+c ile çıkılınca) kaybolmaz.
func TestYanitTaslagi_YenidenBaslatmadaGeriGelir(t *testing.T) {
	for ad, cikis := range map[string]tea.KeyMsg{"esc": tusEsc, "ctrl+c": tusCtrlC} {
		t.Run(ad, func(t *testing.T) {
			dir := t.TempDir()
			m := taslakTestModeli(t, dir)
			basla(t, m, harfler("r"), harfler("kaybolmasın"), cikis)

			m2 := taslakTestModeli(t, dir)
			m2 = basla(t, m2, harfler("r"))
			if got := m2.yanitAlan.Value(); got != "kaybolmasın" {
				t.Fatalf("yeniden başlatmadan sonra taslak geri gelmeli, kutu: %q", got)
			}
		})
	}
}

// TestYanitTaslagi_CokUzunHatasindanSonraDuzenlenebilir: gönderilemeyen
// (ör. bayt sınırını aşan) yanıt kutuda kalır ve düzenlenebilir. (Eskiden
// kutu denetimden önce odağını kaybediyordu: kullanıcı metni kısaltamıyor,
// esc + r ile de tamamen kaybediyordu.)
func TestYanitTaslagi_CokUzunHatasindanSonraDuzenlenebilir(t *testing.T) {
	m := taslakTestModeli(t, t.TempDir())
	m = basla(t, m, harfler("r"))
	uzun := strings.Repeat("€", 3000) // 9000 bayt > forum.MaxBodyBytes; 3000 karakter < CharLimit
	m.yanitAlan.SetValue(uzun)
	m = basla(t, m, tusCtrlS)
	if m.errMsg == "" {
		t.Fatal("çok uzun yanıt için hata gösterilmeli")
	}
	if m.gonderi {
		t.Fatal("çok uzun yanıt gönderilmemeli")
	}
	if !m.yanitAlan.Focused() {
		t.Fatal("hatadan sonra yanıt kutusu düzenlenebilir kalmalı")
	}
	m = basla(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if n := utf8.RuneCountInString(m.yanitAlan.Value()); n != 2999 {
		t.Fatalf("silme tuşu metni kısaltmalı; %d karakter kaldı", n)
	}
}

// TestYanitTaslagi_GonderilinceSilinir: yalnızca başarılı gönderim taslağı
// siler.
func TestYanitTaslagi_GonderilinceSilinir(t *testing.T) {
	dir := t.TempDir()
	m := taslakTestModeli(t, dir)
	m = basla(t, m, harfler("r"), harfler("gidecek"))

	hata, _ := m.Update(forumHataMsg("yayınlanamadı"))
	m = hata.(forumModel)
	m = basla(t, m, tusEsc, harfler("r"))
	if got := m.yanitAlan.Value(); got != "gidecek" {
		t.Fatalf("başarısız gönderim taslağı silmemeli, kutu: %q", got)
	}

	tamam, _ := m.Update(forumYanitGonderildiMsg{konuID: "konu1"})
	m = tamam.(forumModel)
	m = basla(t, m, harfler("r"))
	if got := m.yanitAlan.Value(); got != "" {
		t.Fatalf("gönderilen yanıtın taslağı silinmeli, kutu: %q", got)
	}
	m2 := taslakTestModeli(t, dir)
	m2 = basla(t, m2, harfler("r"))
	if got := m2.yanitAlan.Value(); got != "" {
		t.Fatalf("gönderilen yanıtın taslağı diskte de kalmamalı, kutu: %q", got)
	}
}

// TestCikistaTaslaklarKaydedilir: AND bir tuşla değil de bağlam iptaliyle
// (pencere kapatıldı, SIGTERM) kapanırsa son model üzerinden taslaklar yine
// diske işlenir; yazarken iki saniyelik kayıt aralığına düşen son harfler
// kaybolmaz.
func TestCikistaTaslaklarKaydedilir(t *testing.T) {
	dir := t.TempDir()
	fm := taslakTestModeli(t, dir)
	fm = basla(t, fm, harfler("r"), harfler("a"))
	fm.taslaklar.sonKayit = time.Now() // aralık dolmadı: belkiKaydet yazmaz
	fm = basla(t, fm, harfler("bc"))

	app := appModel{screen: screenForum, forumModel: fm}
	cikistaTaslaklariKaydet(app)

	m2 := taslakTestModeli(t, dir)
	m2 = basla(t, m2, harfler("r"))
	if got := m2.yanitAlan.Value(); got != "abc" {
		t.Fatalf("çıkışta taslağın son hâli yazılmalı, kutu: %q", got)
	}
}

// TestYanitTaslagi_IpuclariTaslagiSoyler: taslağı olan konuda "r" taslağa
// devam ettiğini, yanıt ekranında esc taslağı sakladığını söyler (eskiden
// "vazgeç" diyordu).
func TestYanitTaslagi_IpuclariTaslagiSoyler(t *testing.T) {
	m := taslakTestModeli(t, t.TempDir())
	if strings.Contains(ansi.Strip(m.View()), "taslağa devam et") {
		t.Fatal("taslak yokken \"taslağa devam et\" görünmemeli")
	}
	m = basla(t, m, harfler("r"), harfler("yarım"))
	if duz := ansi.Strip(m.View()); !strings.Contains(duz, "taslağı sakla") {
		t.Errorf("yanıt ekranında esc ipucu taslağın saklandığını söylemeli:\n%s", duz)
	}
	m = basla(t, m, tusEsc)
	if duz := ansi.Strip(m.View()); !strings.Contains(duz, "taslağa devam et") {
		t.Errorf("taslağı olan konuda r ipucu taslağı söylemeli:\n%s", duz)
	}
}
