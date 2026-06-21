package network

import (
	"context"
	"testing"
	"time"

	stdcrypto "and/internal/crypto"

	"github.com/libp2p/go-libp2p/core/peer"
)

// newTestNode creates a Node with a fresh, throwaway identity, listening
// only on loopback so tests don't depend on the network or DHT.
func newTestNode(t *testing.T) *Node {
	t.Helper()

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	node, err := New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = node.Close() })

	return node
}

func TestNew_DerivesPeerIDFromIdentity(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	a, err := New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer a.Close()

	b, err := New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.Close()

	// The same mnemonic must always produce the same PeerID, since that's
	// how a user's node stays addressable across devices/restarts.
	if a.Host.ID() != b.Host.ID() {
		t.Fatalf("expected identical PeerIDs for the same identity, got %s and %s", a.Host.ID(), b.Host.ID())
	}
}

func TestPubSub_PublishAndReceive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeA := newTestNode(t)
	nodeB := newTestNode(t)

	// Connect directly, bypassing DHT/mDNS discovery entirely — this test
	// only exercises the pubsub transport.
	addrInfoB := peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}
	if err := nodeA.Host.Connect(ctx, addrInfoB); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	psA, err := NewPubSub(ctx, nodeA.Host)
	if err != nil {
		t.Fatalf("NewPubSub (A): %v", err)
	}
	psB, err := NewPubSub(ctx, nodeB.Host)
	if err != nil {
		t.Fatalf("NewPubSub (B): %v", err)
	}

	topicA, err := JoinTopic(psA, nodeA.Host, ForumTopic)
	if err != nil {
		t.Fatalf("JoinTopic (A): %v", err)
	}
	defer topicA.Close()

	topicB, err := JoinTopic(psB, nodeB.Host, ForumTopic)
	if err != nil {
		t.Fatalf("JoinTopic (B): %v", err)
	}
	defer topicB.Close()

	// Give GossipSub's mesh a moment to form after the topic join before
	// publishing, otherwise the message can be sent before B is a known
	// subscriber.
	time.Sleep(500 * time.Millisecond)

	const want = "hello from node A"
	if err := topicA.Publish(ctx, []byte(want)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-topicB.Messages(ctx):
		if string(got) != want {
			t.Fatalf("got message %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message on node B")
	}
}
