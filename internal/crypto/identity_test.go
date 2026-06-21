package crypto

import (
	"strings"
	"testing"
)

func TestGenerateIdentity_TwelveWords(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	words := strings.Fields(id.Mnemonic())
	if len(words) != 12 {
		t.Fatalf("expected 12-word mnemonic, got %d words: %q", len(words), id.Mnemonic())
	}

	if len(id.PrivateKey()) == 0 || len(id.PublicKey()) == 0 {
		t.Fatal("expected non-empty keypair")
	}
}

func TestGenerateIdentity_Unique(t *testing.T) {
	a, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	b, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	if a.Mnemonic() == b.Mnemonic() {
		t.Fatal("two generated identities produced the same mnemonic")
	}
}

func TestRestoreFromMnemonic_RoundTrip(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	restored, err := RestoreFromMnemonic(strings.Fields(original.Mnemonic()))
	if err != nil {
		t.Fatalf("RestoreFromMnemonic: %v", err)
	}

	if !restored.PublicKey().Equal(original.PublicKey()) {
		t.Fatal("restored identity has a different public key than the original")
	}
}

func TestRestoreFromMnemonic_InvalidMnemonic(t *testing.T) {
	_, err := RestoreFromMnemonic([]string{"not", "a", "valid", "mnemonic"})
	if err == nil {
		t.Fatal("expected an error for an invalid mnemonic, got nil")
	}
}
