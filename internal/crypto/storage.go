package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

// File layout for identity.dat: salt(16) | nonce(12) | ciphertext(...).
// The plaintext sealed inside the ciphertext is itself
// name(...) | nameSep(1) | mnemonic(...) — the display name travels
// alongside the mnemonic so it round-trips through the same passphrase
// unlock instead of needing its own storage. The mnemonic is never written
// to disk in plaintext — only this passphrase-encrypted blob is.
const (
	saltSize = 16

	// nameSep separates the display name from the mnemonic in the
	// encrypted plaintext. A NUL byte can't appear in a user-typed name
	// (stripped at input time) or in a BIP-39 mnemonic, so it's an
	// unambiguous delimiter.
	nameSep = 0x00

	// Argon2id parameters. Tuned for an interactive local unlock (roughly
	// tens of milliseconds on modern hardware) while still being far
	// stronger than a bare password hash.
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32 // AES-256
)

// SaveEncrypted writes this identity's mnemonic to path, encrypted with a
// key derived from passphrase via Argon2id + AES-256-GCM. The passphrase
// itself is never stored — losing it means the file is unrecoverable
// (the mnemonic remains the only true backup).
func (id *Identity) SaveEncrypted(path, passphrase string) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("crypto: generate salt: %w", err)
	}

	gcm, err := newGCM(passphrase, salt)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("crypto: generate nonce: %w", err)
	}

	plaintext := make([]byte, 0, len(id.name)+1+len(id.mnemonic))
	plaintext = append(plaintext, id.name...)
	plaintext = append(plaintext, nameSep)
	plaintext = append(plaintext, id.mnemonic...)

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	blob := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)

	if err := os.WriteFile(path, blob, 0o600); err != nil {
		return fmt.Errorf("crypto: write %s: %w", path, err)
	}
	return nil
}

// LoadEncrypted reads and decrypts an identity.dat file written by
// SaveEncrypted, rebuilding the full Identity (mnemonic + keypair).
func LoadEncrypted(path, passphrase string) (*Identity, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read %s: %w", path, err)
	}

	gcm, err := newGCM(passphrase, sliceOrEmpty(blob, 0, saltSize))
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(blob) < saltSize+nonceSize {
		return nil, fmt.Errorf("crypto: %s is corrupted or truncated", path)
	}

	nonce := blob[saltSize : saltSize+nonceSize]
	ciphertext := blob[saltSize+nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: wrong passphrase or corrupted file")
	}

	name, mnemonic := "", string(plaintext)
	if i := bytes.IndexByte(plaintext, nameSep); i >= 0 {
		// Identities saved before the name field existed have no nameSep
		// in their plaintext, so this branch is skipped and the whole
		// thing is treated as a bare mnemonic with an empty name.
		name = string(plaintext[:i])
		mnemonic = string(plaintext[i+1:])
	}

	id, err := identityFromMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}
	id.SetName(name)
	return id, nil
}

// newGCM derives an AES-256-GCM cipher from passphrase and salt via
// Argon2id.
func newGCM(passphrase string, salt []byte) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(passphrase), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: init cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: init GCM: %w", err)
	}
	return gcm, nil
}

// sliceOrEmpty safely slices b[start:end], returning an empty (rather than
// panicking) slice if b is too short. This lets newGCM produce a "wrong
// passphrase" style error from Open instead of an index-out-of-range panic
// when handed a corrupted/truncated file.
func sliceOrEmpty(b []byte, start, end int) []byte {
	if len(b) < end {
		return nil
	}
	return b[start:end]
}
