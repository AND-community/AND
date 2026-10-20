package forum

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// gecmisKonu, belirtilen zamanda yazılmış imzalı bir konudur.
func gecmisKonu(priv ed25519.PrivateKey, pub ed25519.PublicKey, baslik string, yazildi time.Time) *Post {
	p := &Post{
		Category:   "Go",
		AuthorName: "yazar",
		AuthorKey:  hex.EncodeToString(pub),
		Title:      baslik,
		Body:       "geçmiş gövde",
		CreatedAt:  yazildi.UTC(),
	}
	p.ID = derivePostID(p)
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, postPayload(p)))
	return p
}

// TestSenkron_BuyukGecmisTamGelirVeCanliKonulariEngellemez: önceden senkronla
// gelen her konu canlı sayaçlara (saatte ağdan 200) sayılıyordu; yeni katılan
// düğüm 300 konuluk bir geçmişin yalnızca 200'ünü alıyor, sonra bir saat
// boyunca ağdan gelen yeni konuları reddediyordu.
func TestSenkron_BuyukGecmisTamGelirVeCanliKonulariEngellemez(t *testing.T) {
	f := newTestForum(t)
	var resp syncResponse
	for i := 0; i < 300; i++ {
		pub, priv := genPostKey(t)
		resp.Posts = append(resp.Posts, gecmisKonu(priv, pub, fmt.Sprintf("eski %d", i), time.Now().Add(-time.Duration(i)*10*time.Minute)))
	}
	f.senkronYanitiUygula(resp)
	if f.TotalPosts() != 300 {
		t.Fatalf("geçmişin tamamı gelmeli, %d konu geldi", f.TotalPosts())
	}

	pub, priv := genPostKey(t)
	canli, _ := json.Marshal(envelope{Type: "post", Post: newSignedPost(priv, pub, "Go", "şimdi yazıldı", "canlı")})
	f.handleMessage(canli)
	if f.TotalPosts() != 301 {
		t.Fatal("büyük bir senkrondan sonra ağdan gelen yeni konu kabul edilmeli")
	}
}

// TestSenkron_YazarSiniriYazildigiSaateGore: aynı saatte yazılmış 15 konudan
// 10'u alınır (canlı yayında da o saatte 10'dan fazlası kabul edilmezdi);
// farklı saatlerde yazılmışlar ayrı sayılır.
func TestSenkron_YazarSiniriYazildigiSaateGore(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	saat := time.Now().Add(-30 * time.Hour).Truncate(time.Hour)
	var resp syncResponse
	for i := 0; i < 15; i++ {
		resp.Posts = append(resp.Posts, gecmisKonu(priv, pub, fmt.Sprintf("aynı saat %d", i), saat.Add(time.Duration(i)*time.Minute)))
	}
	for i := 0; i < 5; i++ {
		resp.Posts = append(resp.Posts, gecmisKonu(priv, pub, fmt.Sprintf("başka saat %d", i), saat.Add(-2*time.Hour).Add(time.Duration(i)*time.Minute)))
	}
	f.senkronYanitiUygula(resp)
	if got := f.TotalPosts(); got != rlMaxPosts+5 {
		t.Fatalf("aynı saatten %d, başka saatten 5 konu gelmeli; %d geldi", rlMaxPosts, got)
	}
}
