package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/storage"
)

// keyMsg builds the tea.KeyMsg for a named special key, for tests that
// drive the model's key handlers directly without a running program.
func keyMsg(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		panic("keyMsg: unsupported key " + name)
	}
}

func newTestModel(t *testing.T) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("alice")
	return newAppModel(context.Background(), id, nil, []pluginmgr.Plugin{}, "", nil, nil, nil, "", nil, nil, nil)
}

// newTestModelWithChatStore is like newTestModel but with a real (nil-topic,
// so purely local) chatlog.Store — needed by tests that exercise sendChat's
// actual signing/persisting path rather than its nil-store no-op guard.
func newTestModelWithChatStore(t *testing.T) appModel {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("alice")
	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cs, err := chatlog.New(id, nil, db, "")
	if err != nil {
		t.Fatalf("chatlog.New: %v", err)
	}
	return newAppModel(context.Background(), id, nil, []pluginmgr.Plugin{}, "", nil, nil, nil, "", cs, nil, nil)
}

func TestAppModel_SendChat_NoTopicIsNoop(t *testing.T) {
	m := newTestModel(t)
	m.chatInput.SetValue("hello")

	got, cmd := m.sendChat()
	result := got.(appModel)

	if cmd != nil {
		t.Fatal("expected no command when there's no chat topic")
	}
	if len(result.chatLines) != 0 {
		t.Fatalf("expected no local echo without a chat topic, got %v", result.chatLines)
	}
}

func TestAppModel_SendChat_EmptyIsNoop(t *testing.T) {
	m := newTestModel(t)
	m.chatInput.SetValue("   ")

	got, cmd := m.sendChat()
	result := got.(appModel)

	if cmd != nil {
		t.Fatal("expected no command for a blank message")
	}
	if len(result.chatLines) != 0 {
		t.Fatalf("expected no local echo for a blank message, got %v", result.chatLines)
	}
}

func TestAppModel_SendChat_OversizedTextShowsNoticeWithoutLocalEcho(t *testing.T) {
	// chatInput'un CharLimit'i (500 rune) MaxTextBytes'ı (2000 bayt)
	// gerçekçi bir girdiyle asla aşılamaz hale getiriyor — bu testte
	// yalnızca MaxTextBytes'ı geçici olarak küçültüp asıl hata yolunu
	// (sendChat'in bildirim gösterme mantığını) gerçekçi şekilde tetikliyoruz.
	old := chatlog.MaxTextBytes
	chatlog.MaxTextBytes = 10
	t.Cleanup(func() { chatlog.MaxTextBytes = old })

	m := newTestModelWithChatStore(t)
	m.chatInput.SetValue(strings.Repeat("a", 20))

	got, cmd := m.sendChat()
	result := got.(appModel)

	if cmd != nil {
		t.Fatal("sendChat artık senkron — hiçbir zaman bir tea.Cmd döndürmemeli")
	}
	if result.chatNotice == "" {
		t.Fatal("boyut sınırını aşan bir gönderim denemesinde bir hata bildirimi gösterilmeliydi")
	}
	if len(result.chatLines) != 0 {
		t.Fatalf("başarısız (hiç persist edilmemiş) bir gönderim yerel olarak eklenmemeliydi, got %v", result.chatLines)
	}
	// Gönderilemeyen mesaj kutuda kalır: kullanıcı kısaltıp yeniden
	// gönderebilir. (Eskiden kutu göndermeden önce boşaltılıyordu; hata
	// bildirimi çıkarken yazılan metin de kayboluyordu.)
	if got := result.chatInput.Value(); got != strings.Repeat("a", 20) {
		t.Fatalf("gönderilemeyen mesaj kutuda kalmalı, kutu: %q", got)
	}
}

func TestAppModel_SendChat_SuccessAppendsLocalEchoAndClearsNotice(t *testing.T) {
	m := newTestModelWithChatStore(t)
	m.chatNotice = "önceki hata"
	m.chatFollow = false
	m.chatInput.SetValue("merhaba")

	got, cmd := m.sendChat()
	result := got.(appModel)

	if cmd != nil {
		t.Fatal("sendChat artık senkron — hiçbir zaman bir tea.Cmd döndürmemeli")
	}
	if result.chatNotice != "" {
		t.Fatalf("başarılı bir gönderim önceki bildirimi temizlemeliydi, got %q", result.chatNotice)
	}
	if len(result.chatLines) != 1 || !strings.Contains(result.chatLines[0], "merhaba") {
		t.Fatalf("başarılı gönderim yerel olarak eklenmeliydi, got %v", result.chatLines)
	}
	if len(result.chatOwn) != 1 || !result.chatOwn[0] {
		t.Error("gönderilen mesaj chatOwn'da true olarak işaretlenmeliydi")
	}
	if !result.chatFollow {
		t.Error("kendi mesajını göndermek chatFollow'u true'ya döndürmeliydi")
	}
}

// ─── Chat mesaj imleci + "profili gör" (ctrl+p) ─────────────────────────────

func TestAppModel_ChatCursor_MovesWithUpDownAndClamps(t *testing.T) {
	m := newTestModel(t)
	m.chatLines = []string{"msg0", "msg1", "msg2"}
	m.chatOwn = []bool{false, false, false}
	m.chatAuthorKeys = []string{"k0", "k1", "k2"}
	m.chatAuthorNames = []string{"a0", "a1", "a2"}
	m.chatCursor = 2

	got, _ := m.handleChatKey(keyMsg("up"))
	result := got.(appModel)
	if result.chatCursor != 1 {
		t.Fatalf("chatCursor = %d, want 1", result.chatCursor)
	}
	if result.chatFollow {
		t.Fatal("yukarı taşındıktan sonra chatFollow false olmalı")
	}

	got, _ = result.handleChatKey(keyMsg("up"))
	result = got.(appModel)
	got, _ = result.handleChatKey(keyMsg("up")) // 0'ın altına inmeye çalış
	result = got.(appModel)
	if result.chatCursor != 0 {
		t.Fatalf("chatCursor 0'ın altına inmemeliydi, got %d", result.chatCursor)
	}

	got, _ = result.handleChatKey(keyMsg("down"))
	result = got.(appModel)
	got, _ = result.handleChatKey(keyMsg("down"))
	result = got.(appModel)
	if result.chatCursor != 2 {
		t.Fatalf("chatCursor = %d, want 2", result.chatCursor)
	}
	if !result.chatFollow {
		t.Fatal("en yeni mesaja dönünce chatFollow true olmalı")
	}
}

func TestAppModel_ChatCtrlP_OpensProfileForSelectedAuthor(t *testing.T) {
	m := newTestModel(t)
	m.chatLines = []string{"[10:00] Bob: merhaba"}
	m.chatOwn = []bool{false}
	m.chatAuthorKeys = []string{"bobkey"}
	m.chatAuthorNames = []string{"Bob"}
	m.chatCursor = 0
	m.width, m.height = 100, 40

	got, _ := m.handleChatKey(keyMsg("ctrl+p"))
	result := got.(appModel)

	if result.screen != screenChatProfile {
		t.Fatalf("screen = %v, want screenChatProfile", result.screen)
	}
	if result.chatProfilKey != "bobkey" {
		t.Fatalf("chatProfilKey = %q, want %q", result.chatProfilKey, "bobkey")
	}
	if result.chatProfilAd != "Bob" {
		t.Fatalf("chatProfilAd = %q, want %q", result.chatProfilAd, "Bob")
	}
}

func TestAppModel_ChatCtrlP_NoopForUnsignedMessage(t *testing.T) {
	m := newTestModel(t)
	m.chatLines = []string{"[10:00] eklenti: merhaba"}
	m.chatOwn = []bool{false}
	m.chatAuthorKeys = []string{""} // imzasız/eklenti kaynaklı — AuthorKey boş
	m.chatAuthorNames = []string{"eklenti"}
	m.chatCursor = 0

	got, _ := m.handleChatKey(keyMsg("ctrl+p"))
	result := got.(appModel)

	if result.screen == screenChatProfile {
		t.Fatal("imzasız bir mesajın yazarı için profil ekranı açılmamalıydı")
	}
}

func TestAppModel_ChatProfileEsc_ReturnsToChat(t *testing.T) {
	m := newTestModel(t)
	m.chatLines = []string{"[10:00] Bob: merhaba"}
	m.chatOwn = []bool{false}
	m.chatAuthorKeys = []string{"bobkey"}
	m.chatAuthorNames = []string{"Bob"}
	m.chatCursor = 0
	m.width, m.height = 100, 40

	got, _ := m.handleChatKey(keyMsg("ctrl+p"))
	result := got.(appModel)
	if result.screen != screenChatProfile {
		t.Fatal("kurulum: ctrl+p profil ekranını açmalıydı")
	}

	got, _ = result.handleChatProfileKey(keyMsg("esc"))
	result = got.(appModel)
	if result.screen != screenChat {
		t.Fatalf("esc sonrası screen = %v, want screenChat", result.screen)
	}
}

func TestAppModel_MenuNavigation(t *testing.T) {
	m := newTestModel(t)
	if m.menuIndex != 0 {
		t.Fatalf("expected menu to start at index 0, got %d", m.menuIndex)
	}

	got, _ := m.handleMenuKey(keyMsg("down"))
	result := got.(appModel)
	if result.menuIndex != 1 {
		t.Fatalf("expected menu index 1 after down, got %d", result.menuIndex)
	}

	got, _ = result.handleMenuKey(keyMsg("up"))
	result = got.(appModel)
	if result.menuIndex != 0 {
		t.Fatalf("expected menu index 0 after up, got %d", result.menuIndex)
	}
}

// TestAppModel_MenuGroups_TreeStructure, ana menünün Topluluk/Eklentiler/
// Sistem dallarına doğru gruplandığını ve viewMenu()'nün bu dalları
// ▾ başlık + ├──/└── bağlantı çizgileriyle çizdiğini doğrular — menüyü düz
// bir listeden ağaç görünümüne çeviren değişikliğin regresyon testi.
func TestAppModel_MenuGroups_TreeStructure(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("alice")
	plugins := []pluginmgr.Plugin{
		{Manifest: pluginapi.Manifest{Name: "admin", Label: "Yönetici Paneli"}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "DeltaX", Label: "DeltaX"}, Enabled: true},
	}
	m := newAppModel(context.Background(), id, nil, plugins, "", nil, nil, nil, "", nil, nil, nil)

	wantItems := []string{"Forum", "Sohbet", "Yönetici Paneli", "DeltaX", "Ayarlar", "AND hakkında", "Çıkış"}
	wantGroups := []string{"Topluluk", "Topluluk", "Eklentiler", "Eklentiler", "Sistem", "Sistem", "Sistem"}
	if len(m.menuItems) != len(wantItems) {
		t.Fatalf("menuItems = %v, want %v", m.menuItems, wantItems)
	}
	for i, item := range wantItems {
		if m.menuItems[i] != item {
			t.Errorf("menuItems[%d] = %q, want %q", i, m.menuItems[i], item)
		}
		if m.menuGroups[i] != wantGroups[i] {
			t.Errorf("menuGroups[%d] = %q, want %q", i, m.menuGroups[i], wantGroups[i])
		}
	}

	m.width, m.height = 100, 40
	out := m.viewMenu()
	for _, want := range []string{"▾ Topluluk", "▾ Eklentiler", "▾ Sistem", "├── ", "└── "} {
		if !strings.Contains(out, want) {
			t.Errorf("viewMenu() çıktısı %q içermiyor", want)
		}
	}
}
