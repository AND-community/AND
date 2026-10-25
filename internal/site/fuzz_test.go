package site

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// Ağdan gelen site iletileri işleyicide panik yaratmamalıdır (bkz.
// forum/fuzz_test.go). Uzun deneme:
//
//	go test ./internal/site -run '^$' -fuzz FuzzImzaliSayfa -fuzztime 30s

func FuzzHandleWireMessage(f *testing.F) {
	for _, s := range []string{
		"", "null", "{}", "{", `{"type":"page"}`, `{"type":"page","page":null}`, `{"type":"page","page":{}}`,
		`{"type":"delete"}`, `{"type":"delete","delete":{}}`,
		`{"type":"page","page":{"author_key":"00","slug":"../x","sig":"00","updated_at":"9999-12-31T23:59:59Z"}}`,
	} {
		f.Add([]byte(s))
	}
	env := newTestStore(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		env.store.handleWireMessage(data)
	})
}

// FuzzImzaliSayfa: saldırganın kendi anahtarıyla geçerli imzaladığı uç
// değerlerdeki sayfa ve silme iletileri de panik yaratmamalı.
func FuzzImzaliSayfa(f *testing.F) {
	f.Add("home", "Başlık", "gövde", int64(0), true)
	f.Add("../../etc", "", "\x00‮", int64(-1<<62), false)
	pub, priv, _ := ed25519.GenerateKey(nil)
	env := newTestStore(f)
	f.Fuzz(func(t *testing.T, slug, baslik, govde string, saniye int64, sil bool) {
		zaman := time.Unix(saniye%(1<<40), 0).UTC()
		p := &Page{AuthorKey: hex.EncodeToString(pub), Slug: slug, Title: baslik, Body: govde, UpdatedAt: zaman}
		p.Sig = hex.EncodeToString(ed25519.Sign(priv, pagePayload(p)))
		data, _ := json.Marshal(envelope{Type: "page", Page: p})
		env.store.handleWireMessage(data)
		if sil {
			d := &DeletePage{AuthorKey: p.AuthorKey, Slug: slug, DeletedAt: zaman.Add(time.Second)}
			d.Sig = hex.EncodeToString(ed25519.Sign(priv, deletePagePayload(d)))
			data, _ = json.Marshal(envelope{Type: "delete", Delete: d})
			env.store.handleWireMessage(data)
		}
	})
}
