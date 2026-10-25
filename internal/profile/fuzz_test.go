package profile

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// Ağdan gelen profil güncellemeleri işleyicide panik yaratmamalıdır (bkz.
// forum/fuzz_test.go). Uzun deneme:
//
//	go test ./internal/profile -run '^$' -fuzz FuzzImzaliProfil -fuzztime 30s

func FuzzHandleMessage(f *testing.F) {
	for _, s := range []string{
		"", "null", "{}", "{", `{"author_key":"zz"}`, `{"author_key":"00","sig":"00","tags":["Genel","Genel"]}`,
		`{"author_key":"00","mod_cert":{}}`, `{"author_key":"00","updated_at":"9999-12-31T23:59:59Z"}`,
	} {
		f.Add([]byte(s))
	}
	env := newTestStore(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		env.store.handleMessage(data)
	})
}

// FuzzImzaliProfil: saldırganın kendi anahtarıyla geçerli imzaladığı uç
// değerler de panik yaratmamalı.
func FuzzImzaliProfil(f *testing.F) {
	f.Add("bio", "durum", "Genel", "https://x", "kullanici", int64(0))
	f.Add("", "‮", "", "javascript:x", "-", int64(-1<<62))
	pub, priv, _ := ed25519.GenerateKey(nil)
	env := newTestStore(f)
	f.Fuzz(func(t *testing.T, bio, durum, etiket, baglanti, github string, saniye int64) {
		p := &Profile{AuthorKey: hex.EncodeToString(pub), Bio: bio, Status: durum, Tags: []string{etiket},
			Link: baglanti, GitHubUser: github, UpdatedAt: time.Unix(saniye%(1<<40), 0).UTC()}
		p.Sig = hex.EncodeToString(ed25519.Sign(priv, profilePayload(p)))
		data, _ := json.Marshal(p)
		env.store.handleMessage(data)
	})
}
