package network

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"

	"github.com/lucian95511/and/protokol"
)

const (
	ForumTopic = protokol.ForumKonusu
	ChatTopic  = protokol.SohbetKonusu
)

func NewPubSub(ctx context.Context, h host.Host) (*pubsub.PubSub, error) {
	ps, err := pubsub.NewGossipSub(ctx, h,
		pubsub.WithMaxMessageSize(64*1024),
		pubsub.WithMessageSigning(true),
		pubsub.WithStrictSignatureVerification(true),
	)
	if err != nil {
		return nil, fmt.Errorf("network: start pubsub: %w", err)
	}
	return ps, nil
}

type Topic struct {
	topic *pubsub.Topic
	sub   *pubsub.Subscription
	self  host.Host
}

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

func (t *Topic) Publish(ctx context.Context, data []byte) error {
	return t.topic.Publish(ctx, data)
}

func (t *Topic) Messages(ctx context.Context) <-chan []byte {
	out := make(chan []byte)

	go func() {
		defer close(out)
		for {
			msg, err := t.sub.Next(ctx)
			if err != nil {
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

func (t *Topic) Close() {
	t.sub.Cancel()
}

// NewSubscriber, AYNI zaten-join-edilmiş pubsub topic'i üzerinde BAĞIMSIZ
// bir ikinci abonelik (Subscription) açar. libp2p-pubsub bir *pubsub.Topic
// üzerinde Subscribe()'ın birden çok kez çağrılmasına izin verir (her
// abone TÜM mesajları alır) — ama aynı topic adını ikinci kez ps.Join()
// etmek her zaman "topic already exists" hatası döner. Bu yüzden aynı
// topic'e ikinci bir bağımsız okuyucu (ör. plugin API'nin TUI'den ayrı
// kendi Messages() akışı) eklemek isteyen çağıranlar JoinTopic'i tekrar
// çağırmak yerine bunu kullanmalı.
func (t *Topic) NewSubscriber(self host.Host) (*Topic, error) {
	sub, err := t.topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("network: subscribe to topic: %w", err)
	}
	return &Topic{topic: t.topic, sub: sub, self: self}, nil
}
