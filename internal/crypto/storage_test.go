package crypto

import (
	"crypto/rand"
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
	// sealed plaintext is a bare mnemonic with no nameSep byte in it. Also
	// header-less (pre format-versioning) — encrypted with legacyArgon2Time,
	// like every real file from before that change.
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	path := filepath.Join(t.TempDir(), "identity.dat")
	const passphrase = "correct-horse-battery-staple"

	salt := make([]byte, saltSize)
	gcm, err := newGCM(passphrase, salt, legacyArgon2Time, argon2Memory, argon2Threads)
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

// TestLoadEncrypted_LegacyFileDecryptsAfterArgon2TimeBump is the direct
// regression test for the real incident that motivated format versioning:
// currentArgon2Time was raised from 1 to 3 for RFC 9106 compliance, and
// because the old format never stored which parameters a file was actually
// encrypted with, LoadEncrypted started unconditionally deriving with the
// new time=3 — locking every existing identity.dat out of its own correct
// passphrase. This proves a file genuinely encrypted the OLD way (header-less,
// legacyArgon2Time) still decrypts correctly under the CURRENT LoadEncrypted,
// regardless of what currentArgon2Time is set to today.
func TestLoadEncrypted_LegacyFileDecryptsAfterArgon2TimeBump(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	original.SetName("eski-kullanici")

	path := filepath.Join(t.TempDir(), "identity.dat")
	const passphrase = "correct-horse-battery-staple"

	// Elle, format-versiyonlamadan ÖNCEKİ gerçek SaveEncrypted'in ürettiği
	// biçimi (header YOK, legacyArgon2Time ile şifrelenmiş) yeniden üretiyor.
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	gcm, err := newGCM(passphrase, salt, legacyArgon2Time, argon2Memory, argon2Threads)
	if err != nil {
		t.Fatalf("newGCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	plaintext := append(append([]byte(original.Name()), nameSep), []byte(original.Mnemonic())...)
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	blob := append(append(append([]byte{}, salt...), nonce...), ciphertext...)
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := LoadEncrypted(path, passphrase)
	if err != nil {
		t.Fatalf("doğru parolayla eski-biçim dosya açılamadı (tam olarak gerçekte yaşanan regresyon): %v", err)
	}
	if loaded.Name() != original.Name() || loaded.Mnemonic() != original.Mnemonic() {
		t.Fatal("çözülen kimlik orijinaliyle eşleşmiyor")
	}
}

// TestSaveEncrypted_WritesVersionedHeader doğrular ki her yeni SaveEncrypted
// çağrısı, gelecekte currentArgon2Time tekrar değiştirilirse aynı kilitlenme
// sınıfının bir daha asla yaşanmaması için gerçekte kullanılan parametreleri
// dosyanın kendisine gömer.
func TestSaveEncrypted_WritesVersionedHeader(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	path := filepath.Join(t.TempDir(), "identity.dat")
	if err := original.SaveEncrypted(path, "pass"); err != nil {
		t.Fatalf("SaveEncrypted: %v", err)
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(blob) < len(formatMagic) || string(blob[:len(formatMagic)]) != string(formatMagic) {
		t.Fatalf("SaveEncrypted çıktısı beklenen formatMagic ile başlamıyor: %x", blob[:min(len(blob), len(formatMagic))])
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
