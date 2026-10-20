package main

// network_test.go — iki gerçek (yerel, 127.0.0.1) libp2p host'u kurup
// birbirine bağlayan paylaşılan test yardımcıları. DHT/mDNS keşfi
// (kesifBaslat) kasıtlı olarak KULLANILMIYOR — testler gerçek internet/LAN
// keşfine bağımlı olmamalı; bunun yerine iki host doğrudan Connect ile
// bağlanıyor, tıpkı protokol seviyesindeki (search/sync) davranışı izole
// test etmek için gereken şey bu.

import (
	"context"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testHost(t *testing.T) host.Host {
	t.Helper()
	k, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	priv, err := libp2pOzelAnahtar(k)
	if err != nil {
		t.Fatal(err)
	}
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// ikiBagliHost, iki gerçek libp2p host'u kurup birbirine doğrudan bağlar.
func ikiBagliHost(t *testing.T) (h1, h2 host.Host, ctx context.Context) {
	t.Helper()
	h1 = testHost(t)
	h2 = testHost(t)

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("host'lar bağlanamadı: %v", err)
	}
	return h1, h2, ctx
}

// ucBagliHost, ÜÇ gerçek libp2p host'unu ZİNCİR topolojisinde kurar:
// h1↔h2↔h3, ama h1 ile h3 DOĞRUDAN bağlı DEĞİLDİR — çok atlamalı
// (search.go'nun TTL'li yayılması) davranışını test etmek için, h1'in
// h3'teki içeriğe yalnızca h2 üzerinden İLETİLEREK ulaşabildiğini
// doğrulamaya yarar.
func ucBagliHost(t *testing.T) (h1, h2, h3 host.Host, ctx context.Context) {
	t.Helper()
	h1 = testHost(t)
	h2 = testHost(t)
	h3 = testHost(t)

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}); err != nil {
		t.Fatalf("h1-h2 bağlanamadı: %v", err)
	}
	if err := h2.Connect(ctx, peer.AddrInfo{ID: h3.ID(), Addrs: h3.Addrs()}); err != nil {
		t.Fatalf("h2-h3 bağlanamadı: %v", err)
	}
	return h1, h2, h3, ctx
}

// aramaProtokoluBekle, h'nin hedeflerin "ağda ara" protokolünü konuştuğunu
// (identify güncellemesiyle) öğrenmesini bekler. Testler sunucuyu eşler
// bağlandıktan SONRA kaydeder; protokol bilgisi o anda kısa bir süre eskidir
// ve aramaEsleri, protokolü konuşmadığı bilinen eşlere sorgu göndermez.
func aramaProtokoluBekle(t *testing.T, h host.Host, hedefler ...peer.ID) {
	t.Helper()
	sonZaman := time.Now().Add(5 * time.Second)
	for _, p := range hedefler {
		for {
			if protos, err := h.Peerstore().SupportsProtocols(p, aramaProtokolu); err == nil && len(protos) > 0 {
				break
			}
			if time.Now().After(sonZaman) {
				t.Fatalf("%s eşinin arama protokolü görünmedi", p)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
