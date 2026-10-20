package main

// durum_test.go — Durum ekranına GERÇEK menü navigasyonuyla (smoke_test.go'nun
// TestModel_DurumEkrani_NoPanic'inin aksine, o doğrudan m.ekran=ekDurum
// atayıp durumIstatistikCmd'yi hiç tetiklemiyordu — bkz. denetim) girip
// istatistik yükleme Cmd'inin GERÇEKTEN çalışıp modele yansıdığını
// doğrular.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_AnaMenudenDurumaGitIstatistikleriYukler(t *testing.T) {
	m := testModel(t)
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	// menuSecenekleri(): ["Ara", "Kodla Git", "Takip Edilenler", "Durum",
	// "Çıkış"] — üç kez aşağı inip "Durum"a ulaş, tıpkı gerçek bir
	// kullanıcının yapacağı gibi.
	for i := 0; i < 3; i++ {
		next, _ := m.Update(keyMsg("down"))
		m = next.(model)
	}
	if m.menuSecenekleri()[m.menuIdx] != "Durum" {
		t.Fatalf("menüde 'Durum' seçili olmalıydı, got %q", m.menuSecenekleri()[m.menuIdx])
	}

	next, cmd := m.Update(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekDurum {
		t.Fatalf("ekDurum'a geçilmeliydi, got %v", m.ekran)
	}
	if cmd == nil {
		t.Fatal("Durum'a girince durumIstatistikCmd tetiklenmeliydi")
	}

	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(model)

	if m.tipDagilimi == nil {
		t.Fatal("tipDagilimi doldurulmalıydı")
	}
	if m.tipDagilimi[TipKonu] != 1 || m.tipDagilimi[TipSohbet] != 1 {
		t.Fatalf("beklenmeyen tip dağılımı: %+v", m.tipDagilimi)
	}
	if m.isErr {
		t.Fatalf("başarılı istatistik yüklemede hata bildirimi olmamalıydı: %q", m.notice)
	}
	_ = m.View()
}

// TestViewAramaKonsolu_UcPaneliDeRenderEder, "en aktif yazarlar" ve
// "günlük içerik trendi" panellerinin — TestModel_AnaMenudenDurumaGit...
// testinin (yazar bilgisi olmayan içerikle) tetiklemediği iki panel —
// DOLU olduklarında doğru render edildiğini doğrular (bkz. denetim,
// viewAramaKonsolu %47.1 kapsamdaydı). kisaPeer'e düşen (YazarAd boş)
// dalı da kapsar.
func TestViewAramaKonsolu_UcPaneliDeRenderEder(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(model)

	m.tipDagilimi = map[SonucTip]int{TipKonu: 3, TipYanit: 1}
	m.enAktifYazarlar = []YazarIstatistigi{
		{YazarKey: "anahtar-ali", YazarAd: "ali", IcerikSayisi: 5},
		{YazarKey: "anahtar-uzun-peer-id-1234567890", YazarAd: "", IcerikSayisi: 2},
	}
	m.gunlukTrend = []GunlukSayim{{Gun: "2026-03-01", Sayi: 2}, {Gun: "2026-03-02", Sayi: 7}}

	out := m.viewAramaKonsolu(m.icerikGenisligi())
	if !strings.Contains(out, "En aktif yazarlar") || !strings.Contains(out, "ali") {
		t.Fatalf("en aktif yazarlar paneli eksik: %q", out)
	}
	if !strings.Contains(out, kisaPeer("anahtar-uzun-peer-id-1234567890")) {
		t.Fatalf("YazarAd boşken kisaPeer'e düşülmeliydi: %q", out)
	}
	if !strings.Contains(out, "Günlük içerik trendi") || !strings.Contains(out, "2026-03-02") {
		t.Fatalf("günlük içerik trendi paneli eksik: %q", out)
	}
}
