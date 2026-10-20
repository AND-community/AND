package forum

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

func benchForum(b *testing.B) *Forum {
	b.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		b.Fatal(err)
	}
	f, err := New(id, nil, filepath.Join(b.TempDir(), "forum.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = f.db.Close() })
	return f
}

// BenchmarkHandleMessage_Konu, ağdan gelen bir konunun çözülüp imzasının
// doğrulanması, hız sınırından geçmesi ve SQLite'a yazılmasıdır. Her konu ayrı
// bir yazardan gelir (yazar başına sınıra takılmasın); ağ geneli canlı bütçe
// (saatte rlGlobalMaxPosts) dolmasın diye her parti yeni bir forumda işlenir.
func BenchmarkHandleMessage_Konu(b *testing.B) {
	const parti = rlGlobalMaxPosts / 2
	iletiler := make([][]byte, b.N)
	for i := range iletiler {
		pub, priv, _ := ed25519.GenerateKey(nil)
		p := newSignedPost(priv, pub, Categories[i%len(Categories)], fmt.Sprintf("Konu %d", i), "Gövde metni, birkaç cümle uzunluğunda örnek içerik.")
		iletiler[i], _ = json.Marshal(envelope{Type: "post", Post: p})
	}
	f := benchForum(b)
	saklanan := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%parti == 0 {
			b.StopTimer()
			saklanan += f.TotalPosts()
			f = benchForum(b)
			b.StartTimer()
		}
		f.handleMessage(iletiler[i])
	}
	b.StopTimer()
	if saklanan += f.TotalPosts(); saklanan != b.N {
		b.Fatalf("%d konu bekleniyordu, %d saklandı", b.N, saklanan)
	}
}

// konularlaForum, bellekte n konusu olan bir forumdur (sayım ve listeleme
// kıyasları için).
func konularlaForum(b *testing.B, n int) *Forum {
	b.Helper()
	f := benchForum(b)
	pub, priv, _ := ed25519.GenerateKey(nil)
	for i := 0; i < n; i++ {
		f.storePost(newSignedPost(priv, pub, Categories[i%len(Categories)], fmt.Sprintf("Konu %d", i), "gövde"))
	}
	return f
}

func BenchmarkCategoryCounts_5000(b *testing.B) {
	f := konularlaForum(b, 5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.CategoryCounts()
	}
}

func BenchmarkPostsByCategory_5000(b *testing.B) {
	f := konularlaForum(b, 5000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.PostsByCategory("Go")
	}
}
