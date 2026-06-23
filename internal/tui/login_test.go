package tui

import (
	"path/filepath"
	"testing"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

func TestLoginModel_RegisterValidation(t *testing.T) {
	m := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)

	got, _ := m.submitForm()
	result := got.(loginModel)
	if result.err == nil {
		t.Fatal("expected an error for an empty name, got nil")
	}
	if result.stage != stageForm {
		t.Fatalf("expected to stay on stageForm, got %v", result.stage)
	}
}

func TestLoginModel_RegisterPassphraseMismatch(t *testing.T) {
	m := newLoginModel(filepath.Join(t.TempDir(), "identity.dat"), true)
	m.inputs[fieldName].SetValue("alice")
	m.inputs[fieldPass].SetValue("one-passphrase")
	m.inputs[fieldConfirm].SetValue("a-different-one")

	got, _ := m.submitForm()
	result := got.(loginModel)
	if result.err == nil {
		t.Fatal("expected an error for mismatched passphrases, got nil")
	}
	if result.stage != stageForm {
		t.Fatalf("expected to stay on stageForm, got %v", result.stage)
	}
}

func TestLoginModel_RegisterThenSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.dat")

	m := newLoginModel(path, true)
	m.inputs[fieldName].SetValue("alice")
	m.inputs[fieldPass].SetValue("correct-horse-battery-staple")
	m.inputs[fieldConfirm].SetValue("correct-horse-battery-staple")

	got, _ := m.submitForm()
	result := got.(loginModel)
	if result.err != nil {
		t.Fatalf("submitForm: %v", result.err)
	}
	if result.stage != stageMnemonic {
		t.Fatalf("expected stageMnemonic, got %v", result.stage)
	}
	if result.identity.Name() != "alice" {
		t.Fatalf("expected identity name %q, got %q", "alice", result.identity.Name())
	}

	got, _ = result.confirmMnemonic()
	result = got.(loginModel)
	if result.err != nil {
		t.Fatalf("confirmMnemonic: %v", result.err)
	}
	if result.stage != stageDone {
		t.Fatalf("expected stageDone, got %v", result.stage)
	}

	loaded, err := stdcrypto.LoadEncrypted(path, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("LoadEncrypted: %v", err)
	}
	if loaded.Name() != "alice" {
		t.Fatalf("expected saved name %q, got %q", "alice", loaded.Name())
	}
}

func TestLoginModel_Unlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.dat")

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("bob")
	if err := id.SaveEncrypted(path, "the-passphrase"); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}

	m := newLoginModel(path, false)
	m.inputs[0].SetValue("the-passphrase")

	got, _ := m.submitForm()
	result := got.(loginModel)
	if result.err != nil {
		t.Fatalf("submitForm: %v", result.err)
	}
	if result.stage != stageDone {
		t.Fatalf("expected stageDone, got %v", result.stage)
	}
	if result.identity.Name() != "bob" {
		t.Fatalf("expected unlocked identity name %q, got %q", "bob", result.identity.Name())
	}
}

func TestLoginModel_UnlockWrongPassphrase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.dat")

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	if err := id.SaveEncrypted(path, "the-passphrase"); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}

	m := newLoginModel(path, false)
	m.inputs[0].SetValue("wrong-passphrase")

	got, _ := m.submitForm()
	result := got.(loginModel)
	if result.err == nil {
		t.Fatal("expected an error for the wrong passphrase, got nil")
	}
	if result.stage != stageForm {
		t.Fatalf("expected to stay on stageForm, got %v", result.stage)
	}
}
