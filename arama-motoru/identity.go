package main

// identity.go — bu programın kendi, AND'ın kimliğinden bağımsız ağ
// kimliği. AND ile AYNI türetme şemasını kullanır (BIP39 mnemonic → seed →
// ed25519 anahtar çifti — bkz. AND/internal/crypto/identity.go) ki
// üretilen anahtar libp2p'nin ed25519 kimlikleriyle bire bir uyumlu olsun;
// ama BAĞIMSIZ bir kimliktir — kullanıcının asıl AND kimliğiyle (ve onun
// parolasıyla) hiçbir ilişkisi yoktur. Bu program ağda kendi başına,
// ayrı bir eş (peer) olarak görünür.

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tyler-smith/go-bip39"
)

const kimlikDosyaAdi = "kimlik.json"

const mnemonicEntropyBits = 128

type kimlikDosyasi struct {
	Mnemonic string `json:"mnemonic"`
}

type Kimlik struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

func (k *Kimlik) PrivateKey() ed25519.PrivateKey { return k.privateKey }
func (k *Kimlik) PublicKey() ed25519.PublicKey   { return k.publicKey }
func (k *Kimlik) PubKeyHex() string              { return hex.EncodeToString(k.publicKey) }

func kimlikFromMnemonic(mnemonic string) (*Kimlik, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("kimlik: geçersiz mnemonic")
	}
	seed := bip39.NewSeed(mnemonic, "")
	priv := ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize])
	return &Kimlik{privateKey: priv, publicKey: priv.Public().(ed25519.PublicKey)}, nil
}

// kimlikYukleYaOlustur, dataDir/kimlik.json içindeki mnemonic'ten kimliği
// yükler; dosya yoksa yeni bir kimlik üretip kaydeder. Dönen bool, yeni bir
// kimliğin üretilip üretilmediğini bildirir (ilk çalıştırma mesajı için).
func kimlikYukleYaOlustur(dataDir string) (k *Kimlik, yeni bool, err error) {
	yol := filepath.Join(dataDir, kimlikDosyaAdi)
	data, err := os.ReadFile(yol)
	if err == nil {
		var kd kimlikDosyasi
		if jerr := json.Unmarshal(data, &kd); jerr == nil {
			if k, kerr := kimlikFromMnemonic(kd.Mnemonic); kerr == nil {
				return k, false, nil
			}
		}
		return nil, false, fmt.Errorf("kimlik: %s bozuk, elle silip yeniden başlatman gerekebilir", yol)
	}
	if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("kimlik dosyası okunamadı: %w", err)
	}

	entropy, err := bip39.NewEntropy(mnemonicEntropyBits)
	if err != nil {
		return nil, false, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, false, err
	}
	k, err = kimlikFromMnemonic(mnemonic)
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, false, err
	}
	out, err := json.MarshalIndent(kimlikDosyasi{Mnemonic: mnemonic}, "", "  ")
	if err != nil {
		return nil, false, err
	}
	if err := atomicYaz(yol, out, 0o600); err != nil {
		return nil, false, err
	}
	return k, true, nil
}
