package forum

import (
	"fmt"
	"testing"
)

func TestRateLimiter_AllowPost_UnderLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		if !rl.allowPost("key1") {
			t.Fatalf("post %d izin verilmeli (limit %d)", i+1, rlMaxPosts)
		}
	}
}

func TestRateLimiter_AllowPost_ExceedsLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1")
	}
	if rl.allowPost("key1") {
		t.Fatal("limit aşıldıktan sonra post reddedilmeli")
	}
}

func TestRateLimiter_AllowReply_UnderLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxReplies; i++ {
		if !rl.allowReply("key1") {
			t.Fatalf("yanıt %d izin verilmeli (limit %d)", i+1, rlMaxReplies)
		}
	}
}

func TestRateLimiter_AllowReply_ExceedsLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxReplies; i++ {
		rl.allowReply("key1")
	}
	if rl.allowReply("key1") {
		t.Fatal("yanıt limiti aşıldıktan sonra reddedilmeli")
	}
}

func TestRateLimiter_DifferentAuthors_Independent(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1")
	}
	// key2'nin limiti key1'den bağımsız olmalı
	if !rl.allowPost("key2") {
		t.Fatal("farklı yazar limitten etkilenmemeli")
	}
}

func TestRateLimiter_GlobalPostLimit_SybilProtection(t *testing.T) {
	rl := newRateLimiter("")
	// Her biri farklı anahtar, toplamda global limiti doldur
	for i := 0; i < rlGlobalMaxPosts; i++ {
		key := fmt.Sprintf("sybil_key_%d", i)
		if !rl.allowPost(key) {
			t.Fatalf("global limit dolmadan post %d reddedildi", i)
		}
	}
	// Yeni kimlik bile olsa global limit aşılınca reddedilmeli
	if rl.allowPost("brand_new_identity") {
		t.Fatal("global limit dolunca yeni kimlik de reddedilmeli (Sybil koruması)")
	}
}

func TestRateLimiter_GlobalLimit_IndependentOfPerAuthor(t *testing.T) {
	rl := newRateLimiter("")
	// Bir yazarın kişisel limitini dolduralım
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1")
	}
	// Global henüz dolmadı, başka yazar devam edebilmeli
	if !rl.allowPost("key2") {
		t.Fatal("global limit dolmamışken başka yazar post atabilmeli")
	}
}

func TestRateLimiter_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	rl := newRateLimiter(dir)

	// Bir post ekle ve kaydet
	if !rl.allowPost("key_persist") {
		t.Fatal("ilk post izin verilmeli")
	}
	rl.save()

	// Yeni rate limiter aynı dizinden yüklenmeli
	rl2 := newRateLimiter(dir)
	// key_persist zaten 1 post kullandı, 9 daha kalan
	count := 0
	for rl2.allowPost("key_persist") {
		count++
	}
	if count >= rlMaxPosts {
		t.Fatal("yüklenen durum önceki postu saymıyor (limit aşıldı)")
	}
}
