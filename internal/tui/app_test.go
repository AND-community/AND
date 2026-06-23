package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/pluginmgr"
)

// keyMsg builds the tea.KeyMsg for a named special key, for tests that
// drive the model's key handlers directly without a running program.
func keyMsg(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
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
	return newAppModel(context.Background(), id, nil, []pluginmgr.Plugin{}, "", nil, nil, "", nil)
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
