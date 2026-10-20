package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPanoyaKopyalaDizisi_OSC52BicimiDogru(t *testing.T) {
	got := panoyaKopyalaDizisi("merhaba dünya")
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("merhaba dünya")) + "\x07"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPanoyaKopyalaDizisi_Base64CozulebilirVeAyniMetniVerir(t *testing.T) {
	metin := "a3f9…92c1/hakkimda"
	dizi := panoyaKopyalaDizisi(metin)

	// "\x1b]52;c;" ile "\x07" arasındaki kısmı ayıklayıp çöz.
	govde := strings.TrimPrefix(dizi, "\x1b]52;c;")
	govde = strings.TrimSuffix(govde, "\x07")
	cozulmus, err := base64.StdEncoding.DecodeString(govde)
	if err != nil {
		t.Fatalf("base64 çözülemedi: %v", err)
	}
	if string(cozulmus) != metin {
		t.Fatalf("çözülen metin eşleşmedi: got %q, want %q", cozulmus, metin)
	}
}

// ─── model.go: keyDetay'ın "c" tuşu ─────────────────────────────────────────

func TestModel_KeyDetay_CTusuSiteSayfasindaPanoyaKopyalar(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipSite, KaynakID: "anahtar-ali/hakkimda", Baslik: "Hakkımda"}
	m.seciliSonuc = &s

	next, cmd := m.Update(keyMsg("c"))
	m = next.(model)
	if m.panoKopyalaDizisi == "" {
		t.Fatal("panoKopyalaDizisi doldurulmalıydı")
	}
	if !strings.Contains(m.panoKopyalaDizisi, base64.StdEncoding.EncodeToString([]byte(s.KaynakID))) {
		t.Fatalf("panoKopyalaDizisi doğru kodu içermeliydi: %q", m.panoKopyalaDizisi)
	}
	if m.isErr || m.notice == "" {
		t.Fatalf("başarı bildirimi bekleniyordu, got isErr=%v notice=%q", m.isErr, m.notice)
	}
	if cmd == nil {
		t.Fatal("panoTemizleCmd döndürülmeliydi")
	}

	// View() çıktısına gömülmeli.
	if !strings.Contains(m.View(), m.panoKopyalaDizisi) {
		t.Fatal("View() çıktısı pano dizisini içermeliydi")
	}

	// Cmd'in ürettiği mesaj işlenince temizlenmeli.
	msg := cmd()
	next2, _ := m.Update(msg)
	m = next2.(model)
	if m.panoKopyalaDizisi != "" {
		t.Fatalf("panoTemizleMsg sonrası panoKopyalaDizisi boş olmalıydı, got %q", m.panoKopyalaDizisi)
	}
}

// TestModel_KeyDetay_CTusuSiteOlmayanTurdeHicbirSeyYapmaz, "c" tuşunun
// yalnızca TipSite sonuçlarında anlamlı olduğunu — başka türlerde
// panoKopyalaDizisi'ni DOLDURMADIĞINI — doğrular.
func TestModel_KeyDetay_CTusuSiteOlmayanTurdeHicbirSeyYapmaz(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	s := AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "bir konu"}
	m.seciliSonuc = &s

	next, cmd := m.Update(keyMsg("c"))
	m = next.(model)
	if m.panoKopyalaDizisi != "" {
		t.Fatalf("TipKonu için panoKopyalaDizisi doldurulmamalıydı, got %q", m.panoKopyalaDizisi)
	}
	if cmd != nil {
		t.Fatal("TipKonu için bir Cmd üretilmemeliydi")
	}
}
