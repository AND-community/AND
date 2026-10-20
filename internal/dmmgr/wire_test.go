package dmmgr

// wire_test.go — dm_test.go'daki (Subscribe/Unsubscribe/Deliver süreç-içi
// pub/sub testleri) YANINDA, gerçek wire protokolünü (SendDM → libp2p stream
// → handleStream) iki gerçek (loopback) libp2p düğümü arasında uçtan uca
// test eder — bu, önceki denetimde kapsam dışı kalan yoldu. Ayrıca yeni
// combineIdentity (gönderen adı taklidi düzeltmesi) davranışını doğrular.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"

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

// ─── Ban entegrasyonu ────────────────────────────────────────────────────────

// TestSendDM_BannedSenderRejected, banlı bir peer'ın DM'inin bağlantı
// gater'ından (yalnızca YENİ bağlantıları engeller) bağımsız olarak, alıcı
// tarafında da (handleStream) reddedildiğini doğrular — bkz.
// Broker.SetModerator'ın doc yorumu: ban öncesi kurulmuş bir bağlantı
// üzerinden bile mesaj teslim edilmemeli.
func TestSendDM_BannedSenderRejected(t *testing.T) {
	nodeA, nodeB := connectedPair(t)
	brokerA := New(nodeA)
	brokerB := New(nodeB)

	founderPub, founderPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey (founder): %v", err)
	}
	old := moderation.FounderPubKeyHex
	moderation.FounderPubKeyHex = hex.EncodeToString(founderPub)
	t.Cleanup(func() { moderation.FounderPubKeyHex = old })

	mod, err := moderation.New(t.TempDir(), peer.ID(""))
	if err != nil {
		t.Fatalf("moderation.New: %v", err)
	}
	// IssueBan yayınlamadan önce yerel olarak uygular ve çalışan bir
	// topic gerektirir (Publish nil bir topic'te panic atar) — burada
	// hiçbir dinleyici olmasa da geçerli bir topic'e ihtiyaç var.
	ps, err := network.NewPubSub(context.Background(), nodeB.Host)
	if err != nil {
		t.Fatalf("NewPubSub: %v", err)
	}
	modTopic, err := network.JoinTopic(ps, nodeB.Host, "test/dmmgr-ban")
	if err != nil {
		t.Fatalf("JoinTopic: %v", err)
	}
	t.Cleanup(modTopic.Close)

	ban := moderation.SignFounderBan(founderPriv, nodeA.Host.ID().String(), "spam", time.Time{}, true)
	if err := mod.IssueBan(context.Background(), modTopic, ban); err != nil {
		t.Fatalf("IssueBan: %v", err)
	}
	if !mod.IsBanned(nodeA.Host.ID()) {
		t.Fatal("kurulum: A banlı olmalıydı")
	}
	brokerB.SetModerator(mod)

	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Alice", "merhaba"); err != nil {
		t.Fatalf("SendDM (transport düzeyinde başarılı olmalı — ban öncesi kurulmuş bağlantı): %v", err)
	}

	select {
	case msg := <-ch:
		t.Fatalf("banlı bir peer'ın DM'i teslim edilmemeliydi: %+v", msg)
	case <-time.After(500 * time.Millisecond):
	}
}

// TestHistory_KeepsDMsWithoutSubscriber: dinleyen eklenti yokken gelen bir DM
// eskiden sessizce kayboluyordu; artık geçmişte durur. Giden mesaj da
// alıcısıyla birlikte kaydedilir.
func TestHistory_KeepsDMsWithoutSubscriber(t *testing.T) {
	nodeA, nodeB := connectedPair(t)
	brokerA := New(nodeA)
	brokerB := New(nodeB) // hiçbir abone yok

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Alice", "kapalıyken gelen"); err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	sonZaman := time.Now().Add(5 * time.Second)
	for len(brokerB.History()) == 0 && time.Now().Before(sonZaman) {
		time.Sleep(20 * time.Millisecond)
	}
	gelen := brokerB.History()
	if len(gelen) != 1 || gelen[0].Text != "kapalıyken gelen" || gelen[0].Outgoing ||
		gelen[0].PeerID != nodeA.Host.ID().String() || gelen[0].Name != "Alice" {
		t.Fatalf("gelen DM geçmişte doğrulanmış gönderenle durmalı: %+v", gelen)
	}
	giden := brokerA.History()
	if len(giden) != 1 || !giden[0].Outgoing || giden[0].PeerID != nodeB.Host.ID().String() {
		t.Fatalf("giden DM alıcısıyla kaydedilmeli: %+v", giden)
	}
}

func TestHistory_BoundedAndCopied(t *testing.T) {
	b := &Broker{}
	for i := range historyLimit + 25 {
		b.record(pluginapi.DMRecord{PeerID: "p", Text: fmt.Sprint(i)})
	}
	h := b.History()
	if len(h) != historyLimit || h[0].Text != "25" || h[len(h)-1].Text != fmt.Sprint(historyLimit+24) {
		t.Fatalf("geçmiş en yeni %d mesajı tutmalı: ilk=%q son=%q n=%d", historyLimit, h[0].Text, h[len(h)-1].Text, len(h))
	}
	h[0].Text = "değişti"
	if b.History()[0].Text == "değişti" {
		t.Fatal("History bir kopya dönmeli")
	}
}
