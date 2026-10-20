package main

// yardim_test.go — ekYardim'in (f1 ile her ekrandan erişilebilen, README'de
// belgelenen ama uygulama içinde önceden hiçbir yerde gösterilmeyen
// kısayol/sözdizimi başvurusu) testleri: her ekrandan f1 ile girilip esc
// ile GİRİLDİĞİ ekrana (durumu KAYBETMEDEN) dönüldüğü, ve f1'in Arama/
// Kodla Git'in metin kutularına YAZILMADIĞI (bkz. handleKey'in f1'i
// KASITLI olarak metin üretmeyen bir tuş seçme gerekçesi).

import "testing"

func TestModel_F1AnaMenudenYardimaGirerEscIleGeriDoner(t *testing.T) {
	m := testModel(t)

	next, _ := m.Update(keyMsg("f1"))
	m = next.(model)
	if m.ekran != ekYardim {
		t.Fatalf("f1 sonrası ekYardim'e geçilmeliydi, got %v", m.ekran)
	}
	_ = m.View()

	next, _ = m.Update(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekAnaMenu {
		t.Fatalf("esc sonrası ana menüye dönülmeliydi, got %v", m.ekran)
	}
}

// TestModel_F1AramaEkranindaSorguyuKorur, Arama ekranında bir sorgu
// yazılıp f1'e basılınca yardıma girildiğini, esc ile dönünce YAZILAN
// SORGUNUN KAYBOLMADIĞINI (yardimOncesiEkran'ın doğru çalıştığını) ve
// "f1" karakterlerinin arama kutusuna YAZILMADIĞINI doğrular.
func TestModel_F1AramaEkranindaSorguyuKorur(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	m.aramaIn.Focus()
	for _, r := range "kelebek" {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}
	if m.aramaIn.Value() != "kelebek" {
		t.Fatalf("kurulum: sorgu doğru yazılmalıydı, got %q", m.aramaIn.Value())
	}

	next, _ := m.Update(keyMsg("f1"))
	m = next.(model)
	if m.ekran != ekYardim {
		t.Fatalf("f1 sonrası ekYardim'e geçilmeliydi, got %v", m.ekran)
	}
	if m.aramaIn.Value() != "kelebek" {
		t.Fatalf("f1'e basmak sorguyu bozmamalıydı, got %q", m.aramaIn.Value())
	}
	_ = m.View()

	next, _ = m.Update(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekArama {
		t.Fatalf("esc sonrası Arama ekranına dönülmeliydi, got %v", m.ekran)
	}
	if m.aramaIn.Value() != "kelebek" {
		t.Fatalf("geri dönüldüğünde sorgu HÂLÂ 'kelebek' olmalıydı, got %q", m.aramaIn.Value())
	}
}

// TestModel_F1KodlaGitEkranindaKoduKorur, AYNI davranışı Kodla Git
// ekranındaki metin girişi için doğrular.
func TestModel_F1KodlaGitEkranindaKoduKorur(t *testing.T) {
	m := testModel(t)
	m.ekran = ekKodlaGit
	m.kodIn.Focus()
	for _, r := range "anahtar/slug" {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(model)
	}

	next, _ := m.Update(keyMsg("f1"))
	m = next.(model)
	if m.ekran != ekYardim {
		t.Fatalf("f1 sonrası ekYardim'e geçilmeliydi, got %v", m.ekran)
	}
	if m.kodIn.Value() != "anahtar/slug" {
		t.Fatalf("f1'e basmak kodu bozmamalıydı, got %q", m.kodIn.Value())
	}

	next, _ = m.Update(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekKodlaGit || m.kodIn.Value() != "anahtar/slug" {
		t.Fatalf("Kodla Git'e kod korunarak dönülmeliydi: ekran=%v kod=%q", m.ekran, m.kodIn.Value())
	}
}

// TestModel_F1YardimdayKenTekrarF1eBasmakDaGeriDoner, "f1" ile açıp "f1"
// ile kapatma beklentisini (esc'e ek olarak) doğrular.
func TestModel_F1YardimdayKenTekrarF1eBasmakDaGeriDoner(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDurum

	next, _ := m.Update(keyMsg("f1"))
	m = next.(model)
	if m.ekran != ekYardim {
		t.Fatalf("f1 sonrası ekYardim'e geçilmeliydi, got %v", m.ekran)
	}

	next, _ = m.Update(keyMsg("f1"))
	m = next.(model)
	if m.ekran != ekDurum {
		t.Fatalf("yardımdayken tekrar f1'e basmak Durum'a dönmeliydi, got %v", m.ekran)
	}
}

// TestModel_YardimEkraniQIleDeKapanir, "q" tuşunun da (esc gibi) yardımı
// kapattığını doğrular.
func TestModel_YardimEkraniQIleDeKapanir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekDetay
	next, _ := m.Update(keyMsg("f1"))
	m = next.(model)

	next, _ = m.Update(keyMsg("q"))
	m = next.(model)
	if m.ekran != ekDetay {
		t.Fatalf("q sonrası Detay'a dönülmeliydi, got %v", m.ekran)
	}
}

func TestModel_YardimEkranindaCtrlCCikar(t *testing.T) {
	m := testModel(t)
	m.ekran = ekYardim
	m.yardimOncesiEkran = ekAnaMenu

	_, cmd := m.Update(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c tea.Quit döndürmeliydi")
	}
}
