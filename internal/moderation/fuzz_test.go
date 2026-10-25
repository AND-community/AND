package moderation

import (
	"crypto/ed25519"
	"testing"
)

// Ağdan gelen moderasyon iletileri (ban/unban/revoke) işleyicide panik
// yaratmamalıdır (bkz. forum/fuzz_test.go). Uzun deneme:
//
//	go test ./internal/moderation -run '^$' -fuzz FuzzHandleEnvelope -fuzztime 30s
func FuzzHandleEnvelope(f *testing.F) {
	for _, s := range []string{
		"", "null", "{}", "{", `{"type":"ban"}`, `{"type":"ban","ban":null}`, `{"type":"ban","ban":{}}`,
		`{"type":"ban","ban":{"cert":{}}}`, `{"type":"unban","unban":{}}`, `{"type":"revoke","revoke":{}}`,
		`{"type":"ban","ban":{"banned_peer":"x","expires_at":"9999-12-31T23:59:59Z","founder_sig":"00"}}`,
	} {
		f.Add([]byte(s))
	}
	founderPub, _, _ := ed25519.GenerateKey(nil)
	m := newTestMod(f, founderPub)
	f.Fuzz(func(t *testing.T, data []byte) {
		m.handleEnvelope(data)
	})
}
