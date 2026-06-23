package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tyler-smith/go-bip39"
)

const mnemonicEntropyBits = 128

type Identity struct {
	name       string
	mnemonic   string
	entropy    []byte
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

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

func RestoreFromMnemonic(words []string) (*Identity, error) {
	mnemonic := strings.Join(words, " ")
	return identityFromMnemonic(mnemonic)
}

func identityFromMnemonic(mnemonic string) (*Identity, error) {
	mnemonic = strings.TrimSpace(mnemonic)

	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("crypto: invalid mnemonic")
	}

	seed := bip39.NewSeed(mnemonic, "")

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

func (id *Identity) Mnemonic() string {
	return id.mnemonic
}

func (id *Identity) RecoveryCode() string {
	h := strings.ToUpper(hex.EncodeToString(id.entropy))
	return fmt.Sprintf("%s-%s-%s-%s", h[0:8], h[8:16], h[16:24], h[24:32])
}

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

func (id *Identity) Name() string {
	return id.name
}

func (id *Identity) SetName(name string) {
	id.name = name
}

func (id *Identity) PrivateKey() ed25519.PrivateKey {
	return id.privateKey
}

func (id *Identity) PublicKey() ed25519.PublicKey {
	return id.publicKey
}
