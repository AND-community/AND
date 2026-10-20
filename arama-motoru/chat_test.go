package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func signedChat(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey) *ChatMessage {
	t.Helper()
	m := &ChatMessage{
		ID: "m1", AuthorKey: hex.EncodeToString(pub), AuthorName: "ali",
		Text: "merhaba herkese", SentAt: time.Now().UTC(),
	}
	m.Sig = hex.EncodeToString(ed25519.Sign(priv, chatPayload(m)))
	return m
}

func TestVerifyChat_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	m := signedChat(t, priv, pub)
	if !verifyChat(m) {
		t.Fatal("geçerli sohbet imzası reddedildi")
	}
}

func TestVerifyChat_KurcalanmisMetinReddedilir(t *testing.T) {
	pub, priv := genKey(t)
	m := signedChat(t, priv, pub)
	m.Text = "değiştirilmiş"
	if verifyChat(m) {
		t.Fatal("kurcalanmış sohbet mesajı kabul edildi")
	}
}

func TestChatMesajIsle_GecerliMesajIndekslenir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	m := signedChat(t, priv, pub)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	chatMesajIsle(d, data)

	sonuclar, err := d.Ara("merhaba", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].Tip != TipSohbet {
		t.Fatalf("geçerli sohbet mesajı indekslenmedi: %+v", sonuclar)
	}
}

func TestChatMesajIsle_ImzasizMesajReddedilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	m := &ChatMessage{ID: "m1", AuthorName: "biri", Text: "imzasız mesaj", SentAt: time.Now().UTC()}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	chatMesajIsle(d, data)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("imzasız mesaj indekslenmemeliydi (AND'ın kendisi de kalıcılaştırmıyor), sayı=%d", n)
	}
}

func TestChatMesajIsle_KurcalanmisImzaReddedilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	m := signedChat(t, priv, pub)
	m.Text = "saldırgan değişikliği"
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	chatMesajIsle(d, data)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("kurcalanmış imzalı mesaj indekslenmemeliydi, sayı=%d", n)
	}
}
