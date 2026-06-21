package network

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

// ForumTopic and ChatTopic are the two GossipSub topics AND nodes
// communicate over. The forum/ and tui/ packages publish/consume on these
// once they're built; this package only provides the transport.
const (
	ForumTopic = "and/forum"
	ChatTopic  = "and/chat"
)

// NewPubSub starts a GossipSub router on host h. Every AND node runs this
// so forum posts and chat messages can "gossip" peer-to-peer across the
// whole network without any central broker.
//
// A 64 KB per-message cap stops runaway payloads from flooding the
// network; forum posts are at most a few kilobytes so this is generous.
func NewPubSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMaxMessageSize(64*1024),
	)
	if err != nil {
		return nil, fmt.Errorf("network: start pubsub: %w", err)
	}
	return ps, nil
}

// Topic wraps a joined GossipSub topic and its local subscription, giving
// callers a minimal publish/receive surface without exposing pubsub's
// wider API.
type Topic struct {
	topic *pubsub.Topic
	sub   *pubsub.Subscription
	self  host.Host
}

// JoinTopic joins name on ps and subscribes to it, so messages other peers
// publish start arriving immediately.
func JoinTopic(ps *pubsub.PubSub, self host.Host, name string) (*Topic, error) {
	t, err := ps.Join(name)
	if err != nil {
		return nil, fmt.Errorf("network: join topic %q: %w", name, err)
	}

	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("network: subscribe to topic %q: %w", name, err)
	}

	return &Topic{topic: t, sub: sub, self: self}, nil
}

// Publish broadcasts data to every peer subscribed to this topic.
func (t *Topic) Publish(ctx context.Context, data []byte) error {
	return t.topic.Publish(ctx, data)
}

// Messages returns a channel of message payloads received on this topic
// from other peers. It closes the channel when ctx is done or the
// subscription ends. Messages this node published itself are not
// delivered back.
func (t *Topic) Messages(ctx context.Context) <-chan []byte {
	out := make(chan []byte)

	go func() {
		defer close(out)
		for {
			msg, err := t.sub.Next(ctx)
			if err != nil {
				// Subscription canceled or ctx done; nothing more to do.
				return
			}
			if msg.GetFrom() == t.self.ID() {
				continue
			}
			select {
			case out <- msg.Data:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Close cancels the local subscription to this topic.
func (t *Topic) Close() {
	t.sub.Cancel()
}
