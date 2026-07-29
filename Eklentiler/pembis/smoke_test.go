package main

// smoke_test.go — bu eklentinin (daha önce hiç testi olmayan) temel
// oluşturma/render/tuş-işleme akışının panic atmadığını doğrulayan hafif bir
// duman testi. Kapsamlı bir davranış testi değildir.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

func TestManifest_WellFormed(t *testing.T) {
	if manifest.Name == "" || manifest.Label == "" || manifest.Version == "" {
		t.Fatalf("manifest eksik alanlar içeriyor: %+v", manifest)
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestModel_InitViewUpdate_NoPanic(t *testing.T) {
	m := newModel(&pluginapi.Client{})

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	for _, key := range []string{"down", "up", "q"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}

func TestPembisTemaState_NotStringMatching(t *testing.T) {
	// P3.17 düzeltmesinin regresyon koruması: onay ekranı artık tipli
	// uygulananTema alanına bakıyor, mesaj metnindeki "pembe" kelimesine değil.
	// Mesaj metni kasıtlı olarak "pembe" kelimesini içeriyor ama uygulanan
	// tema varsayılan — eski string-eşleştirme mantığı burada yanlış dalı
	// (pembe rozetini) gösterirdi.
	m := newModel(&pluginapi.Client{})
	m.ekran = ekOnay
	m.mesaj = "içinde pembe geçen ama pembe temayla ilgisiz bir mesaj"
	m.uygulanan = temaVarsayilan
	m.isErr = false

	out := m.viewOnay()
	if strings.Contains(out, "Pembe tema hazır") {
		t.Fatalf("mesajda \"pembe\" geçmesine rağmen uygulanan=temaVarsayilan iken pembe dalı gösterildi: %q", out)
	}
	if !strings.Contains(out, "Varsayılan mavi-mor tema geri yüklendi") {
		t.Fatalf("uygulanan=temaVarsayilan iken varsayılan dal gösterilmedi: %q", out)
	}

	// Ters durum: mesajda "pembe" kelimesi HİÇ geçmiyor ama uygulanan tema
	// pembe — string eşleştirme olsaydı burada yanlışlıkla varsayılan dal
	// gösterilirdi.
	m.mesaj = "işlem tamamlandı"
	m.uygulanan = temaPembe
	out = m.viewOnay()
	if !strings.Contains(out, "Pembe tema hazır") {
		t.Fatalf("uygulanan=temaPembe iken pembe dalı gösterilmedi: %q", out)
	}
	if strings.Contains(out, "Varsayılan mavi-mor tema geri yüklendi") {
		t.Fatalf("uygulanan=temaPembe iken yanlışlıkla varsayılan dal gösterildi: %q", out)
	}
}
