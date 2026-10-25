package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"

	"github.com/lucian95511/and/internal/dosya"
)

// File layout for identity.dat.
//
// SaveEncrypted always writes the CURRENT (versioned) format:
//
//	magic(8) | argon2Time(4,BE) | argon2Memory(4,BE) | argon2Threads(1) | salt(16) | nonce(12) | ciphertext(...)
//
// LoadEncrypted also accepts the LEGACY (pre-versioning) format, which has
// no header at all:
//
//	salt(16) | nonce(12) | ciphertext(...)
//
// This versioning exists because the Argon2id parameters (time/memory/
// threads) are baked into the derived key — any file encrypted under one
// set of parameters can ONLY be decrypted with that exact same set, not
// whatever the current constants happen to be. Before this format existed,
// the parameters were hardcoded (never stored in the file), so raising
// argon2Time from its original value of 1 to the current 3 (see
// currentArgon2Time's doc comment) silently locked every existing
// identity.dat out of its own correct passphrase: the file predated the
// change and was encrypted with time=1, but LoadEncrypted started
// unconditionally deriving with time=3, producing a different key and a
// "wrong passphrase" error despite the passphrase being right. Embedding
// the actual parameters used means a future change to the defaults can
// never repeat that break: old files keep decrypting with whatever they
// were actually encrypted with, and get upgraded to the current (stronger)
// parameters the next time they're saved.
//
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

	// currentArgon2Time is the Argon2id time cost SaveEncrypted uses for
	// every NEW write — m=64MiB, t=3, p=4, matching RFC 9106's recommended
	// low-memory profile (the original t=1 undershot that guidance and gave
	// an offline attacker against a stolen identity.dat roughly 3x more
	// guesses per second for the same hardware budget). LoadEncrypted never
	// reads this constant for an existing file — see the format-versioning
	// doc comment above — so bumping it again in the future is always safe.
	currentArgon2Time = 3
	argon2Memory      = 64 * 1024 // 64 MB
	argon2Threads     = 4
	argon2KeyLen      = 32 // AES-256

	// legacyArgon2Time is the time cost every identity.dat wat written with
	// before format versioning existed — needed to correctly decrypt those
	// pre-existing, header-less files (bkz. format doc yorumu). Must never
	// change: it's a historical fact about already-written files, not a
	// tunable.
	legacyArgon2Time = 1
)

// formatMagic marks a versioned (header-carrying) identity.dat. A
// header-less legacy file's first 8 bytes are a random salt, so the odds of
// a real one colliding with this exact marker are negligible (1 in 2^64).
var formatMagic = []byte("ANDID2\x00\x00")

const headerSize = 8 /* magic */ + 4 /* time */ + 4 /* memory */ + 1 /* threads */

// SaveEncrypted writes this identity's mnemonic to path, encrypted with a
// key derived from passphrase via Argon2id + AES-256-GCM. The passphrase
// itself is never stored — losing it means the file is unrecoverable
// (the mnemonic remains the only true backup).
func (id *Identity) SaveEncrypted(path, passphrase string) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("crypto: generate salt: %w", err)
	}

	gcm, err := newGCM(passphrase, salt, currentArgon2Time, argon2Memory, argon2Threads)
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

	header := make([]byte, 0, headerSize)
	header = append(header, formatMagic...)
	header = binary.BigEndian.AppendUint32(header, currentArgon2Time)
	header = binary.BigEndian.AppendUint32(header, argon2Memory)
	header = append(header, argon2Threads)

	blob := make([]byte, 0, len(header)+len(salt)+len(nonce)+len(ciphertext))
	blob = append(blob, header...)
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)

	if err := dosya.AtomikYaz(path, blob, 0o600); err != nil {
		return fmt.Errorf("crypto: write %s: %w", path, err)
	}
	return nil
}

// ErrWrongPassphrase is returned by LoadEncrypted when the file cannot be
// decrypted with the given passphrase. AES-GCM cannot tell a wrong
// passphrase from a tampered file, so it covers both — callers can show a
// "wrong password" message instead of the raw error text.
var ErrWrongPassphrase = errors.New("crypto: wrong passphrase or corrupted file")

// LoadEncrypted reads and decrypts an identity.dat file written by
// SaveEncrypted, rebuilding the full Identity (mnemonic + keypair).
func LoadEncrypted(path, passphrase string) (*Identity, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: read %s: %w", path, err)
	}

	// Versioned (header-carrying) dosya mı, yoksa format-versiyonlamadan
	// önceki header'sız eski bir dosya mı — bkz. dosya biçimi doc yorumu.
	// Argon2 parametreleri KDF'ye girer; yanlış parametrelerle türetilen
	// anahtar, doğru parolayla bile "yanlış parola" hatası üretir.
	time, memory, threads := uint32(legacyArgon2Time), uint32(argon2Memory), uint8(argon2Threads)
	rest := blob
	if len(blob) >= headerSize && bytes.Equal(blob[:len(formatMagic)], formatMagic) {
		time = binary.BigEndian.Uint32(blob[8:12])
		memory = binary.BigEndian.Uint32(blob[12:16])
		threads = blob[16]
		rest = blob[headerSize:]
	}

	gcm, err := newGCM(passphrase, sliceOrEmpty(rest, 0, saltSize), time, memory, threads)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(rest) < saltSize+nonceSize {
		return nil, fmt.Errorf("crypto: %s is corrupted or truncated", path)
	}

	nonce := rest[saltSize : saltSize+nonceSize]
	ciphertext := rest[saltSize+nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrWrongPassphrase
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
// Argon2id, using the given (time, memory, threads) cost parameters —
// callers pass either the current defaults (SaveEncrypted) or whatever was
// actually stored in (or implied by the absence of a header in) the file
// being decrypted (LoadEncrypted) — bkz. dosya biçimi doc yorumu.
func newGCM(passphrase string, salt []byte, time, memory uint32, threads uint8) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(passphrase), salt, time, memory, threads, argon2KeyLen)

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
