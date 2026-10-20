package main

// smoke_test.go — panik atmadığını doğrulayan temel duman testleri ile
// liste/düzenleme/silme akışları için hedefli davranış testleri. Ağ
// çağrıları (pagesCmd/publishCmd/deleteCmd) yalnızca tea.Cmd olarak
// üretilir ve bu testlerde hiçbir zaman çalıştırılmaz — bu yüzden sahte
// (zero-value) bir *pluginapi.Client ile güvenle test edilebilir.

import (
	"errors"
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
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestModel_InitViewUpdate_NoPanic(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "ali", "")

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	next, _ = m.Update(pagesMsg{pages: []pluginapi.SitePageInfo{{Slug: "home", Title: "Ana Sayfa"}}})
	m = next.(model)
	_ = m.View()

	for _, key := range []string{"down", "up", "n", "tab", "tab", "esc", "d", "esc"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}

func TestTusListe_NewPageOpensTemplatePicker(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	next, _ := m.tusListe(keyMsg("n"))
	m = next.(model)
	if m.ekran != ekSablon {
		t.Fatalf("'n' önce şablon seçim ekranını açmalıydı, got ekran=%v", m.ekran)
	}
	if m.sablonIdx != 0 {
		t.Fatalf("şablon seçimi 'Boş sayfa'da (idx 0) başlamalıydı, got %d", m.sablonIdx)
	}
}

func TestTusSablon_EnterOnBosSayfaOpensBlankEditFormWithSlugFocused(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekSablon
	m.sablonIdx = 0 // "Boş sayfa" — eski (şablonsuz) davranışla AYNI olmalı

	next, cmd := m.tusSablon(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekDuzenle || !m.yeniMi {
		t.Fatalf("enter sonrası yeni-sayfa düzenleme ekranına geçilmeliydi, got ekran=%v yeniMi=%v", m.ekran, m.yeniMi)
	}
	if m.odak != alanSlug || !m.slugAlan.Focused() {
		t.Fatalf("yeni sayfa formu slug alanına odaklanmış açılmalıydı")
	}
	if m.govdeAlan.Value() != "" {
		t.Fatalf("'Boş sayfa' şablonu gövdeyi ÖN DOLDURMAMALI, got %q", m.govdeAlan.Value())
	}
	if cmd == nil {
		t.Fatal("enter bir Cmd (textinput.Blink) döndürmeliydi")
	}
}

func TestTusSablon_EnterOnNonBlankTemplatePrefillsBody(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekSablon
	m.sablonIdx = 1 // "Hakkımda"

	next, _ := m.tusSablon(keyMsg("enter"))
	m = next.(model)
	if m.govdeAlan.Value() != sablonlar[1].Govde {
		t.Fatalf("gövde seçilen şablonla ön doldurulmalıydı, got %q", m.govdeAlan.Value())
	}
	if m.slugAlan.Value() != "" || m.baslikAlan.Value() != "" {
		t.Fatal("slug/başlık şablondan bağımsız olarak BOŞ bırakılmalıydı")
	}
}

func TestTusSablon_EscReturnsToListWithoutOpeningEditForm(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekSablon

	next, _ := m.tusSablon(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("esc listeye dönmeliydi, got ekran=%v", m.ekran)
	}
}

func TestTusSablon_UpDownNavigatesWithinBounds(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekSablon
	m.sablonIdx = 0

	next, _ := m.tusSablon(keyMsg("up")) // ilk satırda yukarı — no-op
	m = next.(model)
	if m.sablonIdx != 0 {
		t.Fatalf("ilk şablonda 'up' no-op olmalıydı, got idx=%d", m.sablonIdx)
	}

	for range sablonlar {
		next, _ = m.tusSablon(keyMsg("down"))
		m = next.(model)
	}
	if m.sablonIdx != len(sablonlar)-1 {
		t.Fatalf("son şablonun ötesine geçilmemeliydi, got idx=%d, want %d", m.sablonIdx, len(sablonlar)-1)
	}
}

func TestTusListe_MaxPagesBlocksNew(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	pages := make([]pluginapi.SitePageInfo, maxPagesPerAuthor)
	m.pages = pages

	next, cmd := m.tusListe(keyMsg("n"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("kota doluyken 'n' düzenleme ekranını AÇMAMALIYDI, got %v", m.ekran)
	}
	if m.errMsg == "" {
		t.Fatal("kota doluyken bir hata bildirimi gösterilmeliydi")
	}
	if cmd == nil {
		t.Fatal("hata bildirimi bir temizleme Cmd'i döndürmeliydi")
	}
}

func TestTusListe_EditExistingPage_SlugFixedTitleFocused(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.pages = []pluginapi.SitePageInfo{{Slug: "home", Title: "v1", Body: "içerik"}}

	next, _ := m.tusListe(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekDuzenle || m.yeniMi {
		t.Fatalf("enter, var olan sayfayı DÜZENLEME modunda açmalıydı, got ekran=%v yeniMi=%v", m.ekran, m.yeniMi)
	}
	if m.duzenlenenSlug != "home" {
		t.Fatalf("düzenlenen slug 'home' olmalıydı, got %q", m.duzenlenenSlug)
	}
	if m.odak != alanBaslik || !m.baslikAlan.Focused() {
		t.Fatal("düzenleme formu başlık alanına odaklanmış açılmalıydı (slug formda YOK)")
	}
	if m.baslikAlan.Value() != "v1" {
		t.Fatalf("form var olan başlıkla önceden doldurulmalıydı, got %q", m.baslikAlan.Value())
	}
	// Düzenleme modunda tab sırası yalnızca başlık/gövde arasında gezinir.
	sira := m.alanSirasi()
	if len(sira) != 2 || sira[0] != alanBaslik || sira[1] != alanGovde {
		t.Fatalf("düzenleme modu alan sırası [başlık, gövde] olmalı, got %v", sira)
	}
}

func TestTusListe_DeleteRequiresConfirmation(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.pages = []pluginapi.SitePageInfo{{Slug: "home"}}

	next, _ := m.tusListe(keyMsg("d"))
	m = next.(model)
	if m.ekran != ekSilOnay || m.silSlug != "home" {
		t.Fatalf("'d' silme onayı ekranına geçmeliydi, got ekran=%v silSlug=%q", m.ekran, m.silSlug)
	}

	// Onaylamadan başka bir tuşla vazgeçmek bir silme Cmd'i ÜRETMEMELİ.
	next, cmd := m.tusSilOnay(keyMsg("x"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("onaylamayınca liste ekranına dönülmeliydi, got %v", m.ekran)
	}
	if cmd != nil {
		t.Fatal("onaylanmamış bir silme isteği bir Cmd üretmemeliydi")
	}

	// Şimdi onayla.
	m.ekran = ekSilOnay
	m.silSlug = "home"
	next, cmd = m.tusSilOnay(keyMsg("y"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("onay sonrası liste ekranına dönülmeliydi, got %v", m.ekran)
	}
	if cmd == nil {
		t.Fatal("'y' bir silme Cmd'i üretmeliydi")
	}
}

func TestKaydet_EmptySlugRejected(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("   ")
	m.govdeAlan.SetValue("içerik")

	next, cmd := m.kaydet()
	m = next.(model)
	if m.errMsg == "" {
		t.Fatal("boş slug için bir hata bildirimi gösterilmeliydi")
	}
	if m.gonderiliyor {
		t.Fatal("doğrulama başarısızsa gonderiliyor=true olmamalıydı (ağa hiç çıkılmamalı)")
	}
	if cmd == nil {
		t.Fatal("hata bildirimi bir temizleme Cmd'i döndürmeliydi")
	}
}

func TestKaydet_EmptyBodyRejected(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.govdeAlan.SetValue("   ")

	next, _ := m.kaydet()
	m = next.(model)
	if m.errMsg == "" {
		t.Fatal("boş içerik için bir hata bildirimi gösterilmeliydi")
	}
}

func TestKaydet_ValidInputProducesPublishCmd(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.baslikAlan.SetValue("Başlık")
	m.govdeAlan.SetValue("gövde")

	next, cmd := m.kaydet()
	m = next.(model)
	if !m.gonderiliyor {
		t.Fatal("geçerli girdi sonrası gonderiliyor=true olmalıydı")
	}
	if cmd == nil {
		t.Fatal("geçerli girdi bir yayınlama Cmd'i üretmeliydi")
	}
}

func TestTusDuzenle_TabCyclesFocusForNewPage(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.alanOdakla(alanSlug)

	next, _ := m.tusDuzenle(keyMsg("tab"))
	m = next.(model)
	if m.odak != alanBaslik {
		t.Fatalf("slug'dan tab sonrası başlığa geçmeli, got %v", m.odak)
	}
	next, _ = m.tusDuzenle(keyMsg("tab"))
	m = next.(model)
	if m.odak != alanGovde {
		t.Fatalf("başlıktan tab sonrası gövdeye geçmeli, got %v", m.odak)
	}
	next, _ = m.tusDuzenle(keyMsg("tab"))
	m = next.(model)
	if m.odak != alanSlug {
		t.Fatalf("gövdeden tab sonrası döngüsel olarak slug'a dönmeli, got %v", m.odak)
	}
}

func TestTusDuzenle_EscReturnsToListWithoutSaving(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle
	next, cmd := m.tusDuzenle(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("esc listeye dönmeliydi, got %v", m.ekran)
	}
	if cmd != nil {
		t.Fatal("esc ile vazgeçmek bir Cmd üretmemeliydi")
	}
}

func TestUpdate_PublishedMsg_SuccessReturnsToListAndRefreshes(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle
	m.gonderiliyor = true

	next, cmd := m.Update(publishedMsg{err: nil})
	m = next.(model)
	if m.gonderiliyor {
		t.Fatal("başarılı yayından sonra gonderiliyor=false olmalıydı")
	}
	if m.ekran != ekListe {
		t.Fatalf("başarılı yayından sonra liste ekranına dönülmeliydi, got %v", m.ekran)
	}
	if m.okMsg == "" {
		t.Fatal("başarılı yayından sonra bir başarı bildirimi gösterilmeliydi")
	}
	if cmd == nil {
		t.Fatal("başarılı yayın hem listeyi yenilemeli hem bildirimi temizlemeli (tea.Batch)")
	}
}

func TestUpdate_PublishedMsg_ErrorStaysOnFormWithMessage(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle

	backendErr := errors.New("en fazla 20 sayfan olabilir")
	next, _ := m.Update(publishedMsg{err: backendErr})
	m = next.(model)
	if m.ekran != ekDuzenle {
		t.Fatalf("hata durumunda düzenleme ekranından ÇIKILMAMALIYDI, got %v", m.ekran)
	}
	if !strings.Contains(m.errMsg, backendErr.Error()) {
		t.Fatalf("hata mesajı sunucudan gelen hatayı içermeliydi, got %q", m.errMsg)
	}
}

// ─── formKirliMi / esc ile kaydedilmemiş değişiklik uyarısı ───────────────────

func TestFormKirliMi_NewPageEmptyIsNotDirty(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	if m.formKirliMi() {
		t.Fatal("hiçbir alana bir şey yazılmamışsa form kirli sayılmamalı")
	}
}

func TestFormKirliMi_NewPageWithContentIsDirty(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	if !m.formKirliMi() {
		t.Fatal("slug'a bir şey yazılmışsa form kirli sayılmalı")
	}
}

func TestFormKirliMi_ExistingPageUnchangedIsNotDirty(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = false
	m.duzenlenenBaslikOrig = "v1"
	m.duzenlenenGovdeOrig = "içerik"
	m.baslikAlan.SetValue("v1")
	m.govdeAlan.SetValue("içerik")
	if m.formKirliMi() {
		t.Fatal("başlık/gövde özgün değerinden farklı değilse form kirli sayılmamalı")
	}
}

func TestFormKirliMi_ExistingPageEditedIsDirty(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = false
	m.duzenlenenBaslikOrig = "v1"
	m.duzenlenenGovdeOrig = "içerik"
	m.baslikAlan.SetValue("v2")
	m.govdeAlan.SetValue("içerik")
	if !m.formKirliMi() {
		t.Fatal("başlık özgün değerinden farklıysa form kirli sayılmalı")
	}
}

func TestTusDuzenle_EscOnDirtyFormRequiresSecondEscToDiscard(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.slugAlan.Focus()

	// İlk esc: kirli form, ekrandan ÇIKMAMALI — yalnızca uyarı göstermeli.
	next, _ := m.tusDuzenle(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekDuzenle {
		t.Fatalf("kirli formda ilk esc düzenleme ekranından çıkmamalıydı, got %v", m.ekran)
	}
	if !m.vazgecOnayi {
		t.Fatal("kirli formda ilk esc vazgecOnayi=true yapmalıydı")
	}

	// İkinci esc: şimdi gerçekten çık.
	next, _ = m.tusDuzenle(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("ikinci esc listeye dönmeliydi, got %v", m.ekran)
	}
}

func TestTusDuzenle_TypingAfterEscCancelsDiscardWarning(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.slugAlan.Focus()

	next, _ := m.tusDuzenle(keyMsg("esc"))
	m = next.(model)
	if !m.vazgecOnayi {
		t.Fatal("kurulum: ilk esc vazgecOnayi=true yapmalıydı")
	}

	// esc DIŞINDA herhangi bir tuş (ör. devam eden yazı) uyarıyı iptal
	// etmeli ve düzenleme ekranında KALMALI.
	next, _ = m.tusDuzenle(runeKey("x"))
	m = next.(model)
	if m.vazgecOnayi {
		t.Fatal("esc dışında bir tuş vazgecOnayi'yi iptal etmeliydi")
	}
	if m.ekran != ekDuzenle {
		t.Fatalf("düzenleme ekranında kalınmalıydı, got %v", m.ekran)
	}
}

func TestTusDuzenle_EscWithoutChangesExitsImmediately(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.ekran = ekDuzenle
	m.yeniMi = true // hiçbir alan doldurulmadı — kirli değil

	next, _ := m.tusDuzenle(keyMsg("esc"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("değişiklik yokken ilk esc doğrudan listeye dönmeliydi, got %v", m.ekran)
	}
	if m.vazgecOnayi {
		t.Fatal("değişiklik yokken vazgecOnayi hiç true olmamalıydı")
	}
}

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// ─── kaydet: bayt sınırları (rune değil) ───────────────────────────────────────

func TestKaydet_ByteLimitExceededRejectedEvenWithinRuneLimit(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	// Türkçe özel karakterler UTF-8'de 2 bayt: 60 tekrar = 60 rune ama 120
	// bayt — CharLimit (rune bazlı, 100) bunu ENGELLEMEZ, ama gerçek bayt
	// sınırı (100) aşılmış olur. kaydet() bunu ağa hiç çıkmadan yakalamalı.
	m.baslikAlan.SetValue(strings.Repeat("ğ", 60))
	m.govdeAlan.SetValue("içerik")

	next, cmd := m.kaydet()
	m = next.(model)
	if m.gonderiliyor {
		t.Fatal("bayt sınırı aşılmışsa ağa hiç çıkılmamalı")
	}
	if !strings.Contains(m.errMsg, "bayt") {
		t.Fatalf("hata mesajı bayt sınırından bahsetmeliydi, got %q", m.errMsg)
	}
	if cmd == nil {
		t.Fatal("hata bildirimi bir temizleme Cmd'i döndürmeliydi")
	}
}

func TestKaydet_WithinByteLimitAccepted(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.baslikAlan.SetValue(strings.Repeat("ğ", 40)) // 40 rune, 80 bayt — sınır (100) içinde
	m.govdeAlan.SetValue("içerik")

	next, cmd := m.kaydet()
	m = next.(model)
	if !m.gonderiliyor {
		t.Fatal("bayt sınırı içindeyse yayınlanmaya çalışılmalıydı")
	}
	if cmd == nil {
		t.Fatal("geçerli girdi bir yayınlama Cmd'i üretmeliydi")
	}
}

// ─── Pencere boyutu: gövde alanı yüksekliği ölçeklenir ─────────────────────────

func TestWindowSizeMsg_ScalesGovdeHeightWithinBounds(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(model)
	if want := 40 - govdeDisiSatir; m.govdeAlan.Height() != want {
		t.Fatalf("40 satırlık pencerede gövde yüksekliği %d olmalıydı, got %d", want, m.govdeAlan.Height())
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = next.(model)
	if h := m.govdeAlan.Height(); h < 5 {
		t.Fatalf("küçük pencerede gövde yüksekliği alt sınırın (5) altına düşmemeliydi, got %d", h)
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 200})
	m = next.(model)
	if h := m.govdeAlan.Height(); h > 30 {
		t.Fatalf("çok büyük pencerede gövde yüksekliği üst sınırı (30) aşmamalıydı, got %d", h)
	}
}

// ─── Taslak (yerel, yayınlanmamış kayıt) ─────────────────────────────────────

func TestTusDuzenle_CtrlDSavesLocalDraftWithoutPublishing(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.ekran = ekDuzenle
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.baslikAlan.SetValue("Başlık")
	m.govdeAlan.SetValue("gövde")

	next, cmd := m.tusDuzenle(keyMsg("ctrl+d"))
	m = next.(model)
	if m.ekran != ekDuzenle {
		t.Fatalf("ctrl+d ekrandan ÇIKMAMALI (yalnızca yerel kaydeder), got ekran=%v", m.ekran)
	}
	if m.okMsg == "" {
		t.Fatal("başarılı taslak kaydı bir okMsg göstermeliydi")
	}
	if cmd == nil {
		t.Fatal("bildirim temizleme Cmd'i dönmeliydi")
	}

	ts := taslakOku(dataDir)
	if len(ts) != 1 || ts[0].Slug != "home" || ts[0].Baslik != "Başlık" || ts[0].Govde != "gövde" {
		t.Fatalf("diskteki taslak beklenmedik: %+v", ts)
	}
}

func TestTusDuzenle_CtrlDRejectsEmptySlugForNewPage(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.yeniMi = true
	// slugAlan boş bırakıldı.
	m.govdeAlan.SetValue("gövde")

	next, _ := m.tusDuzenle(keyMsg("ctrl+d"))
	m = next.(model)
	if m.errMsg == "" {
		t.Fatal("slug boşken ctrl+d bir hata göstermeliydi")
	}
	if ts := taslakOku(dataDir); len(ts) != 0 {
		t.Fatalf("slug boşken hiçbir taslak yazılmamalıydı, got %+v", ts)
	}
}

func TestTusDuzenle_CtrlDOverwritesExistingDraftForSameSlug(t *testing.T) {
	dataDir := t.TempDir()
	taslakKaydet(dataDir, "home", "eski başlık", "eski gövde")

	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.yeniMi = true
	m.slugAlan.SetValue("home")
	m.baslikAlan.SetValue("yeni başlık")
	m.govdeAlan.SetValue("yeni gövde")

	next, _ := m.tusDuzenle(keyMsg("ctrl+d"))
	m = next.(model)

	ts := taslakOku(dataDir)
	if len(ts) != 1 {
		t.Fatalf("aynı slug için İKİNCİ bir taslak satırı EKLENMEMELİ, got %d satır", len(ts))
	}
	if ts[0].Baslik != "yeni başlık" || ts[0].Govde != "yeni gövde" {
		t.Fatalf("taslak üzerine yazılmalıydı, got %+v", ts[0])
	}
}

func TestUpdate_PublishedMsg_SuccessDeletesMatchingLocalDraft(t *testing.T) {
	dataDir := t.TempDir()
	taslakKaydet(dataDir, "home", "b", "g")
	m := newModel(&pluginapi.Client{}, "", dataDir)

	next, _ := m.Update(publishedMsg{slug: "home", err: nil})
	_ = next.(model)

	if ts := taslakOku(dataDir); len(ts) != 0 {
		t.Fatalf("başarıyla yayınlanan slug için taslak silinmeliydi, got %+v", ts)
	}
}

func TestUpdate_PublishedMsg_ErrorKeepsLocalDraft(t *testing.T) {
	dataDir := t.TempDir()
	taslakKaydet(dataDir, "home", "b", "g")
	m := newModel(&pluginapi.Client{}, "", dataDir)

	next, _ := m.Update(publishedMsg{slug: "home", err: errors.New("ağ hatası")})
	_ = next.(model)

	if ts := taslakOku(dataDir); len(ts) != 1 {
		t.Fatalf("yayınlama BAŞARISIZ olduysa taslak SİLİNMEMELİYDİ, got %+v", ts)
	}
}

func TestTusListe_TKeyOpensDraftsScreenWithSavedDrafts(t *testing.T) {
	dataDir := t.TempDir()
	taslakKaydet(dataDir, "hakkimda", "Hakkımda", "merhaba")
	m := newModel(&pluginapi.Client{}, "", dataDir)

	next, _ := m.tusListe(keyMsg("t"))
	m = next.(model)
	if m.ekran != ekTaslaklar {
		t.Fatalf("'t' taslaklar ekranını açmalıydı, got ekran=%v", m.ekran)
	}
	if len(m.taslaklar) != 1 || m.taslaklar[0].Slug != "hakkimda" {
		t.Fatalf("taslaklar listesi diskten yüklenmeliydi, got %+v", m.taslaklar)
	}
}

func TestTusTaslaklar_EnterLoadsDraftAsNewPageWhenSlugUnknown(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.pages = nil // "hakkimda" HENÜZ yayınlanmış bir sayfa değil
	m.taslaklar = []taslak{{Slug: "hakkimda", Baslik: "Hakkımda", Govde: "merhaba"}}

	next, _ := m.tusTaslaklar(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekDuzenle || !m.yeniMi {
		t.Fatalf("bilinmeyen slug'lı bir taslak YENİ sayfa olarak açılmalıydı, got ekran=%v yeniMi=%v", m.ekran, m.yeniMi)
	}
	if m.slugAlan.Value() != "hakkimda" || m.baslikAlan.Value() != "Hakkımda" || m.govdeAlan.Value() != "merhaba" {
		t.Fatalf("form taslak içeriğiyle doldurulmalıydı: slug=%q baslik=%q govde=%q", m.slugAlan.Value(), m.baslikAlan.Value(), m.govdeAlan.Value())
	}
	if m.odak != alanSlug {
		t.Fatal("yeni sayfa olarak açılan taslakta odak slug alanında olmalıydı")
	}
}

func TestTusTaslaklar_EnterLoadsDraftAsEditWhenSlugMatchesPublishedPage(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	// "hakkimda" ZATEN yayınlanmış (eski içerikle) — taslak bunun ÜZERİNE
	// yazılacak DAHA YENİ bir taslak.
	m.pages = []pluginapi.SitePageInfo{{Slug: "hakkimda", Title: "eski başlık", Body: "eski gövde"}}
	m.taslaklar = []taslak{{Slug: "hakkimda", Baslik: "yeni başlık", Govde: "yeni gövde"}}

	next, _ := m.tusTaslaklar(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekDuzenle || m.yeniMi {
		t.Fatalf("yayınlanmış bir slug'a ait taslak DÜZENLEME modunda açılmalıydı, got ekran=%v yeniMi=%v", m.ekran, m.yeniMi)
	}
	if m.duzenlenenSlug != "hakkimda" {
		t.Fatalf("duzenlenenSlug = %q, want %q", m.duzenlenenSlug, "hakkimda")
	}
	if m.baslikAlan.Value() != "yeni başlık" || m.govdeAlan.Value() != "yeni gövde" {
		t.Fatalf("form TASLAĞIN içeriğiyle (yayınlanmış eski içerikle DEĞİL) doldurulmalıydı: baslik=%q govde=%q", m.baslikAlan.Value(), m.govdeAlan.Value())
	}
	// formKirliMi, taslağı YÜKLEDİKTEN HEMEN SONRA (henüz hiçbir tuşa
	// basılmadan) false olmalı — orijinal karşılaştırma noktası
	// yayınlanmış eski içerik değil, taslağın kendisi olmalı.
	if m.formKirliMi() {
		t.Fatal("taslak yüklendikten hemen sonra form 'kirli' sayılmamalıydı")
	}
}

func TestTusTaslaklar_DKeyDeletesDraftFromDisk(t *testing.T) {
	dataDir := t.TempDir()
	taslakKaydet(dataDir, "a", "A", "a")
	taslakKaydet(dataDir, "b", "B", "b")
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.taslaklar = taslakOku(dataDir)
	m.taslakIdx = 0 // "a"

	next, _ := m.tusTaslaklar(keyMsg("d"))
	m = next.(model)

	ts := taslakOku(dataDir)
	if len(ts) != 1 || ts[0].Slug != "b" {
		t.Fatalf("yalnızca seçili taslak ('a') silinmeliydi, got %+v", ts)
	}
	if len(m.taslaklar) != 1 {
		t.Fatalf("model'deki taslaklar listesi de güncellenmeliydi, got %+v", m.taslaklar)
	}
}

// ─── Sayfa sıralaması (yerel) ────────────────────────────────────────────

func TestSiralaSayfalar_KayitliSiraSirayiBelirler(t *testing.T) {
	pages := []pluginapi.SitePageInfo{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}
	got := siralaSayfalar(pages, []string{"c", "a", "b"})
	want := []string{"c", "a", "b"}
	for i, p := range got {
		if p.Slug != want[i] {
			t.Fatalf("got sıra %v, want %v", sluglar(got), want)
		}
	}
}

func TestSiralaSayfalar_SiralanmamisSayfalarSonaEklenir(t *testing.T) {
	// "d" hiçbir zaman sıralanmamış (ör. başka bir düğümde yeni yayınlanmış) —
	// listeden KAYBOLMAMALI, yalnızca sıralanmış olanların ALTINA düşmeli.
	pages := []pluginapi.SitePageInfo{{Slug: "a"}, {Slug: "d"}, {Slug: "b"}}
	got := siralaSayfalar(pages, []string{"b", "a"})
	want := []string{"b", "a", "d"}
	for i, p := range got {
		if p.Slug != want[i] {
			t.Fatalf("got sıra %v, want %v", sluglar(got), want)
		}
	}
}

func TestSiralaSayfalar_BosKayitliSiraOrijinalSirayiKorur(t *testing.T) {
	pages := []pluginapi.SitePageInfo{{Slug: "a"}, {Slug: "b"}}
	got := siralaSayfalar(pages, nil)
	if got[0].Slug != "a" || got[1].Slug != "b" {
		t.Fatalf("kayıtlı sıra yokken sunucudan gelen sıra korunmalıydı, got %v", sluglar(got))
	}
}

func sluglar(pages []pluginapi.SitePageInfo) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Slug
	}
	return out
}

func TestTusListe_KKeyMovesSelectedPageUpAndPersists(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.pages = []pluginapi.SitePageInfo{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}
	m.idx = 2 // "c" seçili

	next, _ := m.tusListe(keyMsg("K"))
	m = next.(model)
	if sluglar(m.pages)[1] != "c" {
		t.Fatalf("'c' bir konum yukarı taşınmalıydı, got sıra %v", sluglar(m.pages))
	}
	if m.idx != 1 {
		t.Fatalf("seçim taşınan sayfayı takip etmeliydi, got idx=%d", m.idx)
	}

	// Diske yazıldığını, disk YENİDEN OKUNARAK doğrula — taslak testlerindeki
	// aynı disiplin: model'in kendi bellek durumuna değil gerçek dosyaya bak.
	kayitli := siraOku(dataDir)
	want := []string{"a", "c", "b"}
	for i, s := range kayitli {
		if s != want[i] {
			t.Fatalf("diskteki sıra = %v, want %v", kayitli, want)
		}
	}
}

func TestTusListe_JKeyAtLastPageIsNoop(t *testing.T) {
	dataDir := t.TempDir()
	m := newModel(&pluginapi.Client{}, "", dataDir)
	m.pages = []pluginapi.SitePageInfo{{Slug: "a"}, {Slug: "b"}}
	m.idx = 1 // son sayfa zaten seçili

	next, _ := m.tusListe(keyMsg("J"))
	m = next.(model)
	if sluglar(m.pages)[0] != "a" || sluglar(m.pages)[1] != "b" {
		t.Fatalf("son sayfada aşağı taşıma no-op olmalıydı, got %v", sluglar(m.pages))
	}
}

// ─── Sayfa kopyalama (şablon olarak kullan) ─────────────────────────────────

func TestTusListe_CKeyDuplicatesSelectedPageAsNewPageTemplate(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	m.pages = []pluginapi.SitePageInfo{{Slug: "proje-1", Title: "Proje 1", Body: "detaylar"}}
	m.idx = 0

	next, _ := m.tusListe(keyMsg("c"))
	m = next.(model)
	if m.ekran != ekDuzenle || !m.yeniMi {
		t.Fatalf("'c' YENİ bir sayfa formu açmalıydı, got ekran=%v yeniMi=%v", m.ekran, m.yeniMi)
	}
	if m.slugAlan.Value() != "" {
		t.Fatalf("kopyalanan sayfanın slug'ı BOŞ bırakılmalıydı (her sayfa benzersiz slug gerektirir), got %q", m.slugAlan.Value())
	}
	if m.baslikAlan.Value() != "Proje 1" || m.govdeAlan.Value() != "detaylar" {
		t.Fatalf("başlık/içerik kaynak sayfadan kopyalanmalıydı: baslik=%q govde=%q", m.baslikAlan.Value(), m.govdeAlan.Value())
	}
	if m.odak != alanSlug || !m.slugAlan.Focused() {
		t.Fatal("kopyalama sonrası odak slug alanında olmalıydı (kullanıcı yeni bir slug yazmalı)")
	}
}

func TestTusListe_CKeyBlockedAtMaxPages(t *testing.T) {
	m := newModel(&pluginapi.Client{}, "", "")
	pages := make([]pluginapi.SitePageInfo, maxPagesPerAuthor)
	for i := range pages {
		pages[i] = pluginapi.SitePageInfo{Slug: strings.Repeat("x", i+1)}
	}
	m.pages = pages
	m.idx = 0

	next, cmd := m.tusListe(keyMsg("c"))
	m = next.(model)
	if m.ekran != ekListe {
		t.Fatalf("maksimum sayfa sayısındayken 'c' düzenleme ekranını AÇMAMALI, got ekran=%v", m.ekran)
	}
	if m.errMsg == "" || cmd == nil {
		t.Fatal("maksimum sayfa sayısındayken 'c' bir hata bildirimi göstermeliydi")
	}
}
