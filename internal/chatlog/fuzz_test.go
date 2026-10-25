package chatlog

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// Ağdan gelen sohbet iletileri işleyicide panik yaratmamalıdır (bkz.
// forum/fuzz_test.go). Uzun deneme:
//
//	go test ./internal/chatlog -run '^$' -fuzz FuzzHandleMessage -fuzztime 30s

func FuzzHandleMessage(f *testing.F) {
	for _, s := range []string{
		"", "null", "{}", "[]", "{", `{"t":""}`, `{"t":"merhaba"}`,
		`{"t":"x","author_key":"zz","sig":"zz"}`,
		`{"t":"x","a":"9999-12-31T23:59:59Z","author_key":"00","sig":"00"}`,
	} {
		f.Add([]byte(s))
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := &Message{AuthorKey: hex.EncodeToString(pub), AuthorName: "fuzz", Text: "merhaba", SentAt: time.Now().UTC()}
	m.ID = deriveID(m)
	m.Sig = hex.EncodeToString(ed25519.Sign(priv, payload(m)))
	if data, err := json.Marshal(m); err == nil {
		f.Add(data)
	}

	s, _ := newTestStore(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		s.handleMessage(data)
	})
}

// FuzzImzaliMesaj: saldırganın kendi anahtarıyla geçerli imzaladığı uç
// değerler de panik yaratmamalı.
func FuzzImzaliMesaj(f *testing.F) {
	f.Add("merhaba", "ad", int64(0))
	f.Add("\x00\xff‮", "", int64(-1<<62))
	pub, priv, _ := ed25519.GenerateKey(nil)
	s, _ := newTestStore(f)
	f.Fuzz(func(t *testing.T, metin, ad string, saniye int64) {
		m := &Message{AuthorKey: hex.EncodeToString(pub), AuthorName: ad, Text: metin,
			SentAt: time.Unix(saniye%(1<<40), 0).UTC()}
		m.ID = deriveID(m)
		m.Sig = hex.EncodeToString(ed25519.Sign(priv, payload(m)))
		data, _ := json.Marshal(m)
		s.handleMessage(data)
	})
}
