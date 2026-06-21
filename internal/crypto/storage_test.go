package crypto

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadEncrypted_RoundTrip(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	path := filepath.Join(t.TempDir(), "identity.dat")
	const passphrase = "correct-horse-battery-staple"

	if err := original.SaveEncrypted(path, passphrase); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}

	loaded, err := LoadEncrypted(path, passphrase)
	if err != nil {
		t.Fatalf("LoadEncrypted: %v", err)
	}

	if loaded.Mnemonic() != original.Mnemonic() {
		t.Fatal("loaded mnemonic does not match the original")
	}
	if !loaded.PublicKey().Equal(original.PublicKey()) {
		t.Fatal("loaded public key does not match the original")
	}
}

func TestSaveAndLoadEncrypted_NameRoundTrip(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	original.SetName("nakamoto")

	path := filepath.Join(t.TempDir(), "identity.dat")
	const passphrase = "correct-horse-battery-staple"

	if err := original.SaveEncrypted(path, passphrase); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}

	loaded, err := LoadEncrypted(path, passphrase)
	if err != nil {
		t.Fatalf("LoadEncrypted: %v", err)
	}

	if loaded.Name() != "nakamoto" {
		t.Fatalf("expected name %q, got %q", "nakamoto", loaded.Name())
	}
	if loaded.Mnemonic() != original.Mnemonic() {
		t.Fatal("loaded mnemonic does not match the original")
	}
}

func TestLoadEncrypted_LegacyFileWithoutName(t *testing.T) {
	// Simulates an identity.dat saved before the name field existed: the
	// sealed plaintext is a bare mnemonic with no nameSep byte in it.
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	path := filepath.Join(t.TempDir(), "identity.dat")
	const passphrase = "correct-horse-battery-staple"

	salt := make([]byte, saltSize)
	gcm, err := newGCM(passphrase, salt)
	if err != nil {
		t.Fatalf("newGCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	ciphertext := gcm.Seal(nil, nonce, []byte(original.Mnemonic()), nil)

	blob := append(append(salt, nonce...), ciphertext...)
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := LoadEncrypted(path, passphrase)
	if err != nil {
		t.Fatalf("LoadEncrypted: %v", err)
	}
	if loaded.Name() != "" {
		t.Fatalf("expected empty name for legacy file, got %q", loaded.Name())
	}
	if loaded.Mnemonic() != original.Mnemonic() {
		t.Fatal("loaded mnemonic does not match the original")
	}
}

func TestLoadEncrypted_WrongPassphrase(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	path := filepath.Join(t.TempDir(), "identity.dat")
	if err := original.SaveEncrypted(path, "right-passphrase"); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}

	if _, err := LoadEncrypted(path, "wrong-passphrase"); err == nil {
		t.Fatal("expected an error when loading with the wrong passphrase, got nil")
	}
}

func TestLoadEncrypted_CorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.dat")
	if err := os.WriteFile(path, []byte{0x01, 0x02, 0x03}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := LoadEncrypted(path, "any-passphrase"); err == nil {
		t.Fatal("expected an error when loading a truncated file, got nil")
	}
}
