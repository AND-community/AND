package peersync

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
)

func TestProtocolSupport(t *testing.T) {
	ps, err := pstoremem.NewPeerstore()
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	and, dht, yeni := peer.ID("and-dugumu"), peer.ID("dht-dugumu"), peer.ID("yeni")
	if err := ps.AddProtocols(and, "/ipfs/id/1.0.0", "/and/sync/1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := ps.AddProtocols(dht, "/ipfs/kad/1.0.0", "/ipfs/id/1.0.0"); err != nil {
		t.Fatal(err)
	}
	if got := ProtocolSupport(ps, and, "/and/sync/1.0.0", "/and/chatsync/1.0.0"); got != SupportYes {
		t.Errorf("AND düğümü: got %v", got)
	}
	if got := ProtocolSupport(ps, dht, "/and/sync/1.0.0", "/and/chatsync/1.0.0"); got != SupportNo {
		t.Errorf("DHT düğümü: got %v", got)
	}
	if got := ProtocolSupport(ps, yeni, "/and/sync/1.0.0"); got != SupportUnknown {
		t.Errorf("protokolleri bilinmeyen eş: got %v", got)
	}
}
