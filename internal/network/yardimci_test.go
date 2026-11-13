package network

import (
	"context"
	"testing"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/protokol"
)

// duzDugum, yalnızca loopback'i dinleyen, AND protokolü SUNMAYAN bir düğüm
// kurar (genel bir libp2p düğümü gibi görünür).
func duzDugum(t *testing.T, ayarlar ...func(*Secenekler)) *Node {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	s := Secenekler{DinlemeAdresleri: []string{"/ip4/127.0.0.1/tcp/0"}}
	for _, f := range ayarlar {
		f(&s)
	}
	n, err := Yeni(id, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n
}

// andYap, düğüme AND'ın forum senkron protokolünü sundurur: öteki düğümler
// onu AND düğümü olarak tanır (gerçekte forum paketi kaydeder).
func andYap(n *Node) {
	n.Host.SetStreamHandler(protokol.ForumSenkronProtokolu, func(s libp2pnet.Stream) { _ = s.Reset() })
}

// andDugumu, AND düğümü olarak tanınan bir test düğümü kurar.
func andDugumu(t *testing.T, ayarlar ...func(*Secenekler)) *Node {
	t.Helper()
	n := duzDugum(t, ayarlar...)
	andYap(n)
	return n
}

func adresBilgisi(n *Node) peer.AddrInfo {
	return peer.AddrInfo{ID: n.Host.ID(), Addrs: n.Host.Addrs()}
}

// bagla, a'dan b'ye bağlanır.
func bagla(t *testing.T, a, b *Node) {
	t.Helper()
	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	if err := a.Host.Connect(ctx, adresBilgisi(b)); err != nil {
		t.Fatalf("bağlanılamadı: %v", err)
	}
}

// bekle, kosul doğru olana kadar (en çok sure) bekler; olmazsa testi durdurur.
func bekle(t *testing.T, sure time.Duration, kosul func() bool, mesaj string, args ...any) {
	t.Helper()
	bitis := time.Now().Add(sure)
	for !kosul() {
		if time.Now().After(bitis) {
			t.Fatalf("beklenen olmadı ("+sure.String()+"): "+mesaj, args...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// olmasin, sure boyunca kosul'un hiç doğru olmadığını denetler.
func olmasin(t *testing.T, sure time.Duration, kosul func() bool, mesaj string) {
	t.Helper()
	bitis := time.Now().Add(sure)
	for time.Now().Before(bitis) {
		if kosul() {
			t.Fatalf("olmaması gereken oldu: %s", mesaj)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
