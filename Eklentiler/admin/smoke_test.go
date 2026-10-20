package main

// smoke_test.go — panik atmadığını doğrulayan temel duman testleri ile
// sekme geçişi / onay akışları için hedefli davranış testleri. Ağ
// çağrıları (deleteCmd/unbanCmd/grantCmd/revokeCmd vb.) yalnızca tea.Cmd
// olarak üretilir ve bu testlerde hiçbir zaman çalıştırılmaz — bu yüzden
// sahte (zero-value) bir *pluginapi.Client ile güvenle test edilebilir.

import (
	"strings"
	"testing"
	"time"

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
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestModel_InitViewUpdate_NoPanic(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})

	_ = m.Init()
	_ = m.View()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	_ = m.View()

	for _, key := range []string{"down", "up", "tab", "tab", "tab", "tab", "esc"} {
		next, _ = m.Update(keyMsg(key))
		if mm, ok := next.(model); ok {
			m = mm
		}
		_ = m.View()
	}
}

func TestTabCount_ModeratorExcludesModeratorlerFounderIncludes(t *testing.T) {
	mod := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsModerator: true})
	if mod.tabCount() != 3 {
		t.Fatalf("moderatör için sekme sayısı 3 olmalı, %d geldi", mod.tabCount())
	}
	founder := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	if founder.tabCount() != 4 {
		t.Fatalf("kurucu için sekme sayısı 4 olmalı, %d geldi", founder.tabCount())
	}
}

func TestSwitchTab_ResetsSubStatesAndSetsLoading(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	m.confirming = true
	m.unbanConfirming = true
	m.granting = true
	m.grantStep = 1
	m.revokeConfirming = true
	m.loading = false

	next, cmd := m.switchTab(tabBanlar)
	m = next.(model)
	if m.tab != tabBanlar {
		t.Fatalf("tab tabBanlar olmalıydı, %v geldi", m.tab)
	}
	if m.confirming || m.unbanConfirming || m.granting || m.revokeConfirming {
		t.Fatalf("switchTab tüm alt-durumları sıfırlamalı: %+v", m)
	}
	if !m.loading {
		t.Fatalf("switchTab loading=true set etmeli")
	}
	if cmd == nil {
		t.Fatalf("switchTab bir yükleme komutu döndürmeli")
	}
}

func TestKonularKey_NavigationBounds(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	m.allPosts = []pluginapi.PostSummary{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	next, _ := m.handleKonularKey(keyMsg("down"))
	m = next.(model)
	if m.postIdx != 1 {
		t.Fatalf("aşağı sonrası postIdx=1 olmalı, %d geldi", m.postIdx)
	}
	next, _ = m.handleKonularKey(keyMsg("down"))
	m = next.(model)
	next, _ = m.handleKonularKey(keyMsg("down")) // sınırda kalmalı
	m = next.(model)
	if m.postIdx != 2 {
		t.Fatalf("postIdx listenin sonunda sabit kalmalı (2), %d geldi", m.postIdx)
	}
	next, _ = m.handleKonularKey(keyMsg("up"))
	m = next.(model)
	if m.postIdx != 1 {
		t.Fatalf("yukarı sonrası postIdx=1 olmalı, %d geldi", m.postIdx)
	}
}

func TestKonularConfirmFlow(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	m.allPosts = []pluginapi.PostSummary{{ID: "post-1", Title: "Başlık"}}

	next, _ := m.handleKonularKey(keyMsg("x"))
	m = next.(model)
	if !m.confirming {
		t.Fatalf("'x' sonrası confirming=true olmalı")
	}

	next, _ = m.handleKonularConfirmKey(keyMsg("esc"))
	m = next.(model)
	if m.confirming {
		t.Fatalf("esc sonrası confirming=false olmalı")
	}
	if m.pending {
		t.Fatalf("esc ile vazgeçildiğinde pending=true olmamalı")
	}

	next, _ = m.handleKonularKey(keyMsg("x"))
	m = next.(model)
	next, cmd := m.handleKonularConfirmKey(keyMsg("e"))
	m = next.(model)
	if m.confirming {
		t.Fatalf("'e' sonrası confirming=false olmalı")
	}
	if !m.pending {
		t.Fatalf("'e' sonrası pending=true olmalı")
	}
	if cmd == nil {
		t.Fatalf("'e' sonrası bir silme komutu döndürülmeli")
	}
}

func TestBanlarKey_UnbanRequiresAuthorKey(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	m.bans = []pluginapi.BanEntryInfo{{PeerID: "peer-only"}}

	next, _ := m.handleBanlarKey(keyMsg("u"))
	m = next.(model)
	if m.unbanConfirming {
		t.Fatalf("author_key olmadan unbanConfirming açılmamalı")
	}
	if !m.isErr || m.notice == "" {
		t.Fatalf("author_key eksikken bir hata bildirimi gösterilmeli")
	}

	m.bans[0].AuthorKey = "abcd"
	next, _ = m.handleBanlarKey(keyMsg("u"))
	m = next.(model)
	if !m.unbanConfirming {
		t.Fatalf("author_key varken 'u' unbanConfirming=true yapmalı")
	}

	next, cmd := m.handleBanlarConfirmKey(keyMsg("y"))
	m = next.(model)
	if m.unbanConfirming || !m.pending || cmd == nil {
		t.Fatalf("'y' onayı pending=true yapıp bir komut döndürmeli: %+v cmd=%v", m, cmd)
	}
}

func TestModeratorlerGrantFlow_KeyValidationThenDurationSubmit(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})

	next, _ := m.handleModeratorlerKey(keyMsg("g"))
	m = next.(model)
	if !m.granting || m.grantStep != 0 {
		t.Fatalf("'g' sonrası granting=true, grantStep=0 olmalı: %+v", m)
	}

	// Geçersiz anahtar reddedilmeli, adım ilerlememeli.
	next, _ = m.handleGrantKey(keyMsg("z"))
	m = next.(model)
	next, _ = m.handleGrantKey(keyMsg("enter"))
	m = next.(model)
	if m.grantStep != 0 || !m.isErr {
		t.Fatalf("geçersiz anahtar ile grantStep ilerlememeli ve hata gösterilmeli: %+v", m)
	}

	validHex := strings.Repeat("ab", 32) // 64 hex karakter = 32 bayt
	m.grantKeyInput.SetValue(validHex)
	next, _ = m.handleGrantKey(keyMsg("enter"))
	m = next.(model)
	if m.grantStep != 1 {
		t.Fatalf("geçerli anahtar sonrası grantStep=1 olmalı, %d geldi", m.grantStep)
	}

	next, _ = m.handleGrantKey(keyMsg("right"))
	m = next.(model)
	if m.grantDurIdx != 1 {
		t.Fatalf("'right' sonrası grantDurIdx=1 olmalı, %d geldi", m.grantDurIdx)
	}

	next, cmd := m.handleGrantKey(keyMsg("enter"))
	m = next.(model)
	if m.granting || !m.pending || cmd == nil {
		t.Fatalf("süre seçip enter sonrası granting=false, pending=true ve bir komut olmalı: %+v cmd=%v", m, cmd)
	}
}

func TestModeratorlerKey_RevokeBlockedForAlreadyRevoked(t *testing.T) {
	m := newModel(&pluginapi.Client{}, pluginapi.RoleInfo{IsFounder: true})
	m.grants = []pluginapi.GrantedModInfo{{Key: strings.Repeat("cd", 32), Revoked: true}}

	next, _ := m.handleModeratorlerKey(keyMsg("r"))
	m = next.(model)
	if m.revokeConfirming {
		t.Fatalf("zaten iptal edilmiş bir kayıt için revokeConfirming açılmamalı")
	}
}

func TestWindowSlice(t *testing.T) {
	if start, end := windowSlice(5, 2, 15); start != 0 || end != 5 {
		t.Fatalf("n<=size iken tüm liste dönmeli, %d..%d geldi", start, end)
	}
	if start, end := windowSlice(100, 0, 10); start != 0 || end != 10 {
		t.Fatalf("cursor başta iken start=0 olmalı, %d..%d geldi", start, end)
	}
	if start, end := windowSlice(100, 99, 10); start != 90 || end != 100 {
		t.Fatalf("cursor sonda iken end=n olmalı, %d..%d geldi", start, end)
	}
	if start, end := windowSlice(100, 50, 10); end-start != 10 {
		t.Fatalf("pencere boyu sabit kalmalı, %d..%d geldi", start, end)
	}
}

func TestKalanSure(t *testing.T) {
	if got := kalanSure(time.Now().Add(-time.Hour)); got != "süresi doldu" {
		t.Fatalf("geçmiş zaman için 'süresi doldu' beklenirdi, %q geldi", got)
	}
	if got := kalanSure(time.Now().Add(30 * time.Second)); got != "1 dakikadan az" {
		t.Fatalf("30sn için '1 dakikadan az' beklenirdi, %q geldi", got)
	}
	if got := kalanSure(time.Now().Add(2 * time.Hour)); got != "2 saat" {
		t.Fatalf("2 saat için '2 saat' beklenirdi, %q geldi", got)
	}
}

func TestShortKey(t *testing.T) {
	short := "abc123"
	if got := shortKey(short); got != short {
		t.Fatalf("kısa anahtar değişmemeli: %q", got)
	}
	long := strings.Repeat("f", 64)
	got := shortKey(long)
	if len(got) >= len(long) {
		t.Fatalf("uzun anahtar kısaltılmalı: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("kısaltılmış anahtar '…' içermeli: %q", got)
	}
}
