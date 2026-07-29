package dmmgr

// wire_test.go — dm_test.go'daki (Subscribe/Unsubscribe/Deliver süreç-içi
// pub/sub testleri) YANINDA, gerçek wire protokolünü (SendDM → libp2p stream
// → handleStream) iki gerçek (loopback) libp2p düğümü arasında uçtan uca
// test eder — bu, önceki denetimde kapsam dışı kalan yoldu. Ayrıca yeni
// combineIdentity (gönderen adı taklidi düzeltmesi) davranışını doğrular.

import (
	"context"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ─── combineIdentity ─────────────────────────────────────────────────────────

func TestCombineIdentity(t *testing.T) {
	cases := []struct {
		claimed, verified, want string
	}{
		{"", "12D3KooWabc", "12D3KooWabc"},
		{"12D3KooWabc", "12D3KooWabc", "12D3KooWabc"},            // öz-bildirilen ad = doğrulanmış ID → yalnızca ID
		{"Alice", "12D3KooWabcdefghij", "Alice (12D3KooWabcd…)"}, // ikisi de gösterilir, taklit görünür kalır
		{"  Alice  ", "12D3KooWabcdefghij", "Alice (12D3KooWabcd…)"},
	}
	for _, c := range cases {
		if got := combineIdentity(c.claimed, c.verified); got != c.want {
			t.Errorf("combineIdentity(%q, %q) = %q, want %q", c.claimed, c.verified, got, c.want)
		}
	}
}

// ─── Uçtan uca (gerçek iki libp2p düğümü, loopback) ─────────────────────────

func newTestNode(t *testing.T) *network.Node {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New: %v", err)
	}
	t.Cleanup(func() { _ = node.Close() })
	return node
}

func connectedPair(t *testing.T) (*network.Node, *network.Node) {
	t.Helper()
	nodeA := newTestNode(t)
	nodeB := newTestNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addrInfoB := peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}
	if err := nodeA.Host.Connect(ctx, addrInfoB); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return nodeA, nodeB
}

func TestSendDM_EndToEnd(t *testing.T) {
	nodeA, nodeB := connectedPair(t)
	brokerA := New(nodeA)
	brokerB := New(nodeB)

	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Alice", "merhaba"); err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	select {
	case msg := <-ch:
		if msg.Text != "merhaba" {
			t.Fatalf("Text = %q, want %q", msg.Text, "merhaba")
		}
		wantPrefix := "Alice ("
		if len(msg.From) < len(wantPrefix) || msg.From[:len(wantPrefix)] != wantPrefix {
			t.Fatalf("From = %q — göndericinin doğrulanmış Peer ID'sini de içermeliydi", msg.From)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mesaj zaman aşımına uğradı")
	}
}

func TestSendDM_EmptyTextIgnored(t *testing.T) {
	nodeA, nodeB := connectedPair(t)
	brokerA := New(nodeA)
	brokerB := New(nodeB)

	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Alice", ""); err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	select {
	case msg := <-ch:
		t.Fatalf("boş metinli DM teslim edilmemeliydi: %+v", msg)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestSendDM_InvalidPeerID(t *testing.T) {
	nodeA := newTestNode(t)
	broker := New(nodeA)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := broker.SendDM(ctx, "gecersiz-peer-id", "Alice", "merhaba"); err == nil {
		t.Fatal("geçersiz peer ID için SendDM hata döndürmeliydi")
	}
}
