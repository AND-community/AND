package main

// verifyedge_test.go — verifyEd25519 (forum.go, chat.go/site.go'nun da
// AYNI fonksiyonu paylaştığı imza doğrulama çekirdeği) ve doğrudan
// türevlerinin (verifyChat/verifySitePage/verifySiteDelete) GEÇERSİZ
// GİRDİ (bozuk hex, yanlış uzunlukta anahtar) dallarının testleri — önceki
// testler yalnızca "geçerli hex ama kurcalanmış/yanlış anahtarla imzalı"
// durumunu kapsıyordu, hex.DecodeString'in kendisinin başarısız olduğu
// durumu değil (bkz. denetim).

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"
)

func TestVerifyEd25519_GecersizHexAnahtarReddedilir(t *testing.T) {
	if verifyEd25519("bu-hex-degil", "aa", []byte("payload")) {
		t.Fatal("geçersiz hex anahtar kabul edilmemeliydi")
	}
}

func TestVerifyEd25519_YanlisUzunluktaAnahtarReddedilir(t *testing.T) {
	// Geçerli hex ama ed25519.PublicKeySize (32 bayt) DEĞİL.
	kisaAnahtar := hex.EncodeToString([]byte("çok kısa"))
	if verifyEd25519(kisaAnahtar, "aa", []byte("payload")) {
		t.Fatal("yanlış uzunlukta anahtar kabul edilmemeliydi")
	}
}

func TestVerifyEd25519_GecersizHexImzaReddedilir(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if verifyEd25519(hex.EncodeToString(pub), "bu-da-hex-degil", []byte("payload")) {
		t.Fatal("geçersiz hex imza kabul edilmemeliydi")
	}
}

func TestVerifyPost_GecersizHexAnahtarReddedilir(t *testing.T) {
	p := &Post{ID: "p1", AuthorKey: "gecersiz-hex", Sig: "aa", CreatedAt: time.Now()}
	if verifyPost(p) {
		t.Fatal("geçersiz hex anahtarlı post kabul edilmemeliydi")
	}
}

func TestVerifyChat_GecersizHexAnahtarReddedilir(t *testing.T) {
	m := &ChatMessage{ID: "m1", AuthorKey: "gecersiz-hex", Sig: "aa", SentAt: time.Now()}
	if verifyChat(m) {
		t.Fatal("geçersiz hex anahtarlı sohbet mesajı kabul edilmemeliydi")
	}
}

func TestVerifySitePage_GecersizHexAnahtarReddedilir(t *testing.T) {
	p := &SitePage{AuthorKey: "gecersiz-hex", Slug: "s", Sig: "aa", UpdatedAt: time.Now()}
	if verifySitePage(p) {
		t.Fatal("geçersiz hex anahtarlı site sayfası kabul edilmemeliydi")
	}
}

func TestVerifySiteDelete_GecersizHexAnahtarReddedilir(t *testing.T) {
	d := &siteDeleteMsg{AuthorKey: "gecersiz-hex", Slug: "s", Sig: "aa", DeletedAt: time.Now()}
	if verifySiteDelete(d) {
		t.Fatal("geçersiz hex anahtarlı silme isteği kabul edilmemeliydi")
	}
}
