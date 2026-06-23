package dmmgr

import (
	"sync"
	"testing"
	"time"

	"github.com/lucian95511/and/internal/pluginapi"
)

// brokerNoNode creates a Broker without a real network.Node for unit tests
// that only exercise the pub/sub logic (Subscribe/Unsubscribe/Deliver).
func newBrokerForTest() *Broker {
	return &Broker{}
}

func TestSubscribe_ReceivesDelivered(t *testing.T) {
	b := newBrokerForTest()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	want := pluginapi.DMMsg{From: "alice", Text: "merhaba", ReceivedAt: time.Now()}
	b.Deliver(want)

	select {
	case got := <-ch:
		if got.From != want.From || got.Text != want.Text {
			t.Errorf("got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: message not received")
	}
}

func TestUnsubscribe_ChannelClosed(t *testing.T) {
	b := newBrokerForTest()
	ch := b.Subscribe()
	b.Unsubscribe(ch)

	// After Unsubscribe, the channel is closed; reading should not block.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after Unsubscribe")
	}
}

func TestUnsubscribe_RemovesFromList(t *testing.T) {
	b := newBrokerForTest()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()
	b.Unsubscribe(ch1)

	if len(b.subs) != 1 {
		t.Fatalf("expected 1 subscriber after unsubscribe, got %d", len(b.subs))
	}
	_ = ch2
	b.Unsubscribe(ch2)
}

func TestDeliver_NoSubscribers_NoPanic(t *testing.T) {
	b := newBrokerForTest()
	// Should not panic with no subscribers.
	b.Deliver(pluginapi.DMMsg{From: "x", Text: "y"})
}

func TestDeliver_MultipleSubscribers(t *testing.T) {
	b := newBrokerForTest()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()
	defer b.Unsubscribe(ch1)
	defer b.Unsubscribe(ch2)

	msg := pluginapi.DMMsg{From: "bob", Text: "herkese"}
	b.Deliver(msg)

	for _, ch := range []chan pluginapi.DMMsg{ch1, ch2} {
		select {
		case got := <-ch:
			if got.Text != "herkese" {
				t.Errorf("got %q, want %q", got.Text, "herkese")
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestDeliver_FullChannel_NonBlocking(t *testing.T) {
	b := newBrokerForTest()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Fill the channel buffer (capacity 32).
	for i := 0; i < 32; i++ {
		b.Deliver(pluginapi.DMMsg{From: "x", Text: "y"})
	}
	// This must not block even though the channel is full.
	done := make(chan struct{})
	go func() {
		b.Deliver(pluginapi.DMMsg{From: "overflow", Text: "drop"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Deliver blocked on full channel")
	}
}

func TestDeliver_Concurrent(t *testing.T) {
	b := newBrokerForTest()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			b.Deliver(pluginapi.DMMsg{From: "g", Text: "parallel"})
		}()
	}
	wg.Wait()
}

func TestSubscribeUnsubscribe_Idempotent(t *testing.T) {
	b := newBrokerForTest()
	ch := b.Subscribe()
	b.Unsubscribe(ch)
	// Second Unsubscribe should not panic.
	b.Unsubscribe(ch)
}
