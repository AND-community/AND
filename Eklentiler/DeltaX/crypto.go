package main

// crypto.go — grup paketleri için AES-256-GCM şifreleme.
//
// Her grubun rastgele üretilmiş 32 baytlık bir "Sir" (secret) anahtarı
// vardır ve yalnızca davet edilen üyelerle paylaşılır. Proje paketleri
// (bkz. sync.go) bu anahtarla şifrelenip P2P dosya aktarımıyla gönderilir.
// libp2p taşıma katmanı zaten uçtan uca şifrelidir; bu ek katman, verinin
// yalnızca grup sırrını bilen üyeler tarafından çözülebilmesini sağlayarak
// içeriği merkezi ağdaki/aracı düğümlerdeki diğer bileşenlerden de izole eder.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

func sirUret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func gcmAc(sirHex string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(sirHex)
	if err != nil || len(key) != 32 {
		return nil, errors.New("geçersiz grup anahtarı")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sirliSifrele(sirHex string, acikVeri []byte) ([]byte, error) {
	gcm, err := gcmAc(sirHex)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, acikVeri, nil), nil
}

func sirliCoz(sirHex string, sifreliVeri []byte) ([]byte, error) {
	gcm, err := gcmAc(sirHex)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(sifreliVeri) < ns {
		return nil, errors.New("şifreli veri çok kısa")
	}
	nonce, ct := sifreliVeri[:ns], sifreliVeri[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}
