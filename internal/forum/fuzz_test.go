package forum

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// Ağdan gelen forum iletileri güvenilmezdir: bozuk JSON, eksik ya da tuhaf
// alanlar, saldırganın kendi anahtarıyla geçerli imzaladığı uç değerler.
// Hiçbiri işleyicide panik yaratmamalıdır (bkz. guvenli.Isle — koruma son
// çaredir, hata olmamalı). Tohumlar her `go test`te çalışır; uzun deneme:
//
//	go test ./internal/forum -run '^$' -fuzz FuzzHandleMessage -fuzztime 30s

func fuzzTohumlari(f *testing.F, imzali ...envelope) {
	for _, s := range []string{
		"", "null", "{}", "[]", `"x"`, "123", "{",
		`{"type":"post"}`, `{"type":"post","post":null}`, `{"type":"post","post":{}}`,
		`{"type":"reply"}`, `{"type":"reply","reply":{}}`,
		`{"type":"delete"}`, `{"type":"delete","delete":{}}`,
		`{"type":"mod_delete"}`, `{"type":"mod_delete","mod_delete":{}}`,
		`{"type":"post","post":{"category":"Genel","author_key":"zz","sig":"zz","created_at":"9999-12-31T23:59:59Z"}}`,
		`{"type":"mod_delete","mod_delete":{"post_id":"x","cert":{}}}`,
	} {
		f.Add([]byte(s))
	}
	for _, e := range imzali {
		data, _ := json.Marshal(e)
		f.Add(data)
	}
}

// imzaliKonu, priv ile geçerli imzalanmış bir konudur.
func imzaliKonu(pub ed25519.PublicKey, priv ed25519.PrivateKey, kategori, baslik, govde string, zaman time.Time) *Post {
	p := &Post{Category: kategori, AuthorName: "fuzz", AuthorKey: hex.EncodeToString(pub),
		Title: baslik, Body: govde, CreatedAt: zaman}
	p.ID = derivePostID(p)
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, postPayload(p)))
	return p
}

func FuzzHandleMessage(f *testing.F) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := imzaliKonu(pub, priv, "Genel", "başlık", "gövde", time.Now().UTC())
	r := &Reply{PostID: p.ID, AuthorName: "fuzz", AuthorKey: p.AuthorKey, Body: "yanıt", CreatedAt: time.Now().UTC()}
	r.ID = deriveReplyID(r)
	r.Sig = hex.EncodeToString(ed25519.Sign(priv, replyPayload(r)))
	d := &DeleteMsg{PostID: p.ID, AuthorKey: p.AuthorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(d)))
	fuzzTohumlari(f,
		envelope{Type: "post", Post: p}, envelope{Type: "reply", Reply: r},
		envelope{Type: "delete", Delete: d}, envelope{Type: "mod_delete", ModDelete: &ModDeleteMsg{PostID: p.ID}})

	fr := newTestForum(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		fr.handleMessage(data)
	})
}

// FuzzImzaliKonu: saldırgan kendi anahtarıyla istediği alanları geçerli
// imzalayabilir; imza doğrulamasından sonraki yollar da panik yapmamalı.
func FuzzImzaliKonu(f *testing.F) {
	f.Add("Genel", "başlık", "gövde", "yanıt", int64(0))
	f.Add("Yapay Zeka", "", "[code]x", "[/code]", int64(-1<<62))
	f.Add("Genel", "\x00\xff", "‮", "", int64(1<<62))
	pub, priv, _ := ed25519.GenerateKey(nil)
	fr := newTestForum(f)
	f.Fuzz(func(t *testing.T, kategori, baslik, govde, yanit string, saniye int64) {
		zaman := time.Unix(saniye%(1<<40), 0).UTC()
		p := imzaliKonu(pub, priv, kategori, baslik, govde, zaman)
		data, _ := json.Marshal(envelope{Type: "post", Post: p})
		fr.handleMessage(data)
		r := &Reply{PostID: p.ID, AuthorName: "fuzz", AuthorKey: p.AuthorKey, Body: yanit, CreatedAt: zaman}
		r.ID = deriveReplyID(r)
		r.Sig = hex.EncodeToString(ed25519.Sign(priv, replyPayload(r)))
		data, _ = json.Marshal(envelope{Type: "reply", Reply: r})
		fr.handleMessage(data)
	})
}
