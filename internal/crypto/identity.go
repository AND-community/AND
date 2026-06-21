// Package crypto implements AND's decentralized identity primitives.
//
// There is no central account database. A user's identity is a BIP-39
// mnemonic (12 words) that deterministically derives an Ed25519 keypair.
// The same keypair is reused as the node's libp2p identity, so a single
// seed phrase is enough to both authenticate the user and address their
// node on the P2P network.
package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tyler-smith/go-bip39"
)

// mnemonicEntropyBits controls the mnemonic length. 128 bits of entropy
// produces exactly 12 words, matching the product spec.
const mnemonicEntropyBits = 128

// Identity is a user's full decentralized identity: the human-readable
// mnemonic plus the Ed25519 keypair derived from it, plus a display name
// the user picks for themselves. The name is purely cosmetic (shown to
// other peers in the forum/chat) — it carries no authentication weight,
// unlike the mnemonic-derived keypair.
type Identity struct {
	name       string
	mnemonic   string
	entropy    []byte // raw BIP-39 entropy, used to produce the recovery code
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// GenerateIdentity creates a brand new identity: a random 12-word mnemonic
// and the Ed25519 keypair derived from it.
func GenerateIdentity() (*Identity, error) {
	entropy, err := bip39.NewEntropy(mnemonicEntropyBits)
	if err != nil {
		return nil, fmt.Errorf("crypto: generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, fmt.Errorf("crypto: generate mnemonic: %w", err)
	}

	return identityFromMnemonic(mnemonic)
}

// RestoreFromMnemonic rebuilds an identity from a previously issued 12-word
// mnemonic, e.g. when a user moves to a new device.
func RestoreFromMnemonic(words []string) (*Identity, error) {
	mnemonic := strings.Join(words, " ")
	return identityFromMnemonic(mnemonic)
}

// identityFromMnemonic validates a mnemonic and deterministically derives
// the Ed25519 keypair from it.
func identityFromMnemonic(mnemonic string) (*Identity, error) {
	mnemonic = strings.TrimSpace(mnemonic)

	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("crypto: invalid mnemonic")
	}

	// bip39.NewSeed never errors once the mnemonic has already been
	// validated above; the passphrase is left empty since AND's only
	// secret input is the mnemonic itself (the local passphrase is a
	// separate, device-only layer handled by storage.go).
	seed := bip39.NewSeed(mnemonic, "")

	// Ed25519 needs a 32-byte seed; BIP-39 seeds are 64 bytes, so the
	// first half is used as the deterministic Ed25519 seed.
	priv := ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize])

	entropy, err := bip39.EntropyFromMnemonic(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("crypto: extract entropy: %w", err)
	}

	return &Identity{
		mnemonic:   mnemonic,
		entropy:    entropy,
		privateKey: priv,
		publicKey:  priv.Public().(ed25519.PublicKey),
	}, nil
}

// Mnemonic returns the 12-word seed phrase. This is the user's full
// identity — callers must only ever display it locally, never transmit it.
func (id *Identity) Mnemonic() string {
	return id.mnemonic
}

// RecoveryCode encodes the identity's underlying entropy as a formatted
// uppercase hex string: XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX (32 chars +
// 3 dashes). This is shown to the user once at registration as their
// backup — it is equivalent to the 12-word mnemonic but easier to copy.
func (id *Identity) RecoveryCode() string {
	h := strings.ToUpper(hex.EncodeToString(id.entropy))
	return fmt.Sprintf("%s-%s-%s-%s", h[0:8], h[8:16], h[16:24], h[24:32])
}

// RestoreFromCode rebuilds an identity from a recovery code produced by
// RecoveryCode. Dashes and spaces in the code are ignored so the user
// can paste with or without separators.
func RestoreFromCode(code string) (*Identity, error) {
	clean := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.TrimSpace(code))

	entropy, err := hex.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid recovery code")
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid recovery code: %w", err)
	}

	return identityFromMnemonic(mnemonic)
}

// Name returns the user's chosen display name. It is empty for identities
// that predate this field or that haven't had SetName called yet.
func (id *Identity) Name() string {
	return id.name
}

// SetName sets the user's display name, e.g. right after they type it in
// on the registration screen.
func (id *Identity) SetName(name string) {
	id.name = name
}

// PrivateKey returns the Ed25519 private key derived from the mnemonic.
func (id *Identity) PrivateKey() ed25519.PrivateKey {
	return id.privateKey
}

// PublicKey returns the Ed25519 public key derived from the mnemonic. This
// is safe to share and is what other peers use to recognize this identity.
func (id *Identity) PublicKey() ed25519.PublicKey {
	return id.publicKey
}
