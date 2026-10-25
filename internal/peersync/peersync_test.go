package peersync

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// fakeClock, testlerin zamanı elle ilerletmesi içindir.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type recorder struct {
	mu    sync.Mutex
	calls []peer.ID
	fail  map[peer.ID]bool
}

func (r *recorder) sync(_ context.Context, p peer.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, p)
	if r.fail[p] {
		return errors.New("bağlantı koptu")
	}
	return nil
}

func (r *recorder) count(p peer.ID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c == p {
			n++
		}
	}
	return n
}

func newTestScheduler(r *recorder, clk *fakeClock, support map[peer.ID]Support, connected []peer.ID) *Scheduler {
	return New(Config{
		Sync:         r.sync,
		Connected:    func() []peer.ID { return connected },
		Supports:     func(p peer.ID) Support { return support[p] },
		ConnectDelay: time.Millisecond,
		ResyncAfter:  2 * time.Minute,
		RetryAfter:   time.Minute,
		Interval:     10 * time.Minute,
		PerRound:     2,
		MaxParallel:  2,
		Now:          clk.Now,
	})
}

func TestConnect_SyncsOnceThenAgainAfterResyncWindow(t *testing.T) {
	r := &recorder{}
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newTestScheduler(r, clk, map[peer.ID]Support{"a": SupportYes}, nil)
	ctx := context.Background()

	s.onConnect(ctx, "a")
	s.wait()
	if got := r.count("a"); got != 1 {
		t.Fatalf("ilk bağlantıda 1 senkron bekleniyordu, got %d", got)
	}
	// Kısa süre içinde kopup yeniden bağlanma (bağlantı dalgalanması): tekrar yok.
	clk.Add(30 * time.Second)
	s.onConnect(ctx, "a")
	s.wait()
	if got := r.count("a"); got != 1 {
		t.Fatalf("2 dk içinde yeniden bağlanma senkron tetiklememeli, got %d", got)
	}
	// Uzun bir aradan sonra yeniden bağlanma: aradaki içerik için tekrar senkron.
	clk.Add(5 * time.Minute)
	s.onConnect(ctx, "a")
	s.wait()
	if got := r.count("a"); got != 2 {
		t.Fatalf("aradan sonra yeniden bağlanmada senkron bekleniyordu, got %d", got)
	}
}

func TestConnect_SkipsNonANDPeersButTriesUnknown(t *testing.T) {
	r := &recorder{}
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newTestScheduler(r, clk, map[peer.ID]Support{"dht": SupportNo, "yeni": SupportUnknown}, nil)
	s.onConnect(context.Background(), "dht")
	s.onConnect(context.Background(), "yeni")
	s.wait()
	if r.count("dht") != 0 {
		t.Fatal("AND protokolü konuşmayan eşle senkron denenmemeli")
	}
	if r.count("yeni") != 1 {
		t.Fatal("protokolleri henüz bilinmeyen eşle senkron denenmeli")
	}
}

func TestFailure_RetriedOnlyAfterRetryWindow(t *testing.T) {
	r := &recorder{fail: map[peer.ID]bool{"a": true}}
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newTestScheduler(r, clk, map[peer.ID]Support{"a": SupportYes}, nil)
	ctx := context.Background()

	s.onConnect(ctx, "a")
	s.wait()
	clk.Add(20 * time.Second)
	s.onConnect(ctx, "a")
	s.wait()
	if got := r.count("a"); got != 1 {
		t.Fatalf("başarısızlıktan hemen sonra tekrar denenmemeli, got %d", got)
	}
	clk.Add(2 * time.Minute)
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	s.onConnect(ctx, "a")
	s.wait()
	if got := r.count("a"); got != 2 {
		t.Fatalf("bekleme süresi dolunca tekrar denenmeli, got %d", got)
	}
	if st := s.Status(); st.LastSuccess.IsZero() || st.Failures != 1 {
		t.Fatalf("durum özeti beklenmedik: %+v", st)
	}
}

func TestRound_PicksOldestConfirmedPeersUpToLimit(t *testing.T) {
	r := &recorder{}
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	support := map[peer.ID]Support{"a": SupportYes, "b": SupportYes, "c": SupportYes, "dht": SupportNo, "yeni": SupportUnknown}
	s := newTestScheduler(r, clk, support, []peer.ID{"a", "b", "c", "dht", "yeni"})
	ctx := context.Background()

	// "a" az önce senkronlandı; "b" ve "c" hiç senkronlanmadı.
	s.onConnect(ctx, "a")
	s.wait()
	clk.Add(time.Minute)

	s.round(ctx)
	s.wait()
	if r.count("b") != 1 || r.count("c") != 1 {
		t.Fatalf("hiç senkronlanmamış AND eşleri seçilmeli: %v", r.calls)
	}
	if r.count("a") != 1 || r.count("dht") != 0 || r.count("yeni") != 0 {
		t.Fatalf("yakın zamanda senkronlanan, AND olmayan ya da doğrulanmamış eş seçilmemeli: %v", r.calls)
	}

	// Aralık dolunca en eski senkronlanan önce gelir; tur başına en fazla PerRound eş.
	clk.Add(11 * time.Minute)
	s.round(ctx)
	s.wait()
	if len(r.calls) != 5 {
		t.Fatalf("ikinci turda 2 eş daha bekleniyordu: %v", r.calls)
	}
	if r.count("a") != 2 {
		t.Fatalf("en eski senkronlanan ('a') önce seçilmeli: %v", r.calls)
	}
}

func TestMaxParallelAndNoDuplicateInFlight(t *testing.T) {
	var running, peak int32
	release := make(chan struct{})
	var calls int32
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := New(Config{
		Sync: func(ctx context.Context, p peer.ID) error {
			atomic.AddInt32(&calls, 1)
			n := atomic.AddInt32(&running, 1)
			for {
				old := atomic.LoadInt32(&peak)
				if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
					break
				}
			}
			<-release
			atomic.AddInt32(&running, -1)
			return nil
		},
		Connected:    func() []peer.ID { return nil },
		Supports:     func(peer.ID) Support { return SupportYes },
		ConnectDelay: time.Millisecond,
		MaxParallel:  2,
		Now:          clk.Now,
	})
	ctx := context.Background()
	for _, p := range []peer.ID{"a", "a", "b", "c", "d"} {
		s.onConnect(ctx, p)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	s.wait()
	if peak > 2 {
		t.Fatalf("aynı anda en fazla 2 senkron çalışmalı, got %d", peak)
	}
	if calls != 4 {
		t.Fatalf("aynı eşe eşzamanlı ikinci senkron açılmamalı: %d çağrı (4 bekleniyordu)", calls)
	}
}

func TestRun_StopsWithContext(t *testing.T) {
	r := &recorder{}
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := newTestScheduler(r, clk, map[peer.ID]Support{"a": SupportYes}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan peer.ID, 1)
	done := make(chan struct{})
	go func() { s.Run(ctx, ch); close(done) }()
	ch <- "a"
	deadline := time.Now().Add(2 * time.Second)
	for r.count("a") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run, bağlam iptal edilince dönmeli")
	}
	if r.count("a") != 1 {
		t.Fatal("Run bağlantı olayını işlemeli")
	}
}

// TestPanik_BasarisizSayilirVeYuvaSerbestKalir: eşten gelen veriyi çözen
// senkron panik yaparsa düğüm çökmez; deneme başarısız sayılır ve eşzamanlı
// senkron yuvası serbest kalır (yoksa MaxParallel panikten sonra tükenirdi).
func TestPanik_BasarisizSayilirVeYuvaSerbestKalir(t *testing.T) {
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	var tamamlanan atomic.Int32
	s := New(Config{
		Sync: func(_ context.Context, p peer.ID) error {
			if p == "bozuk" {
				var m map[string]int
				m["x"] = 1 // nil haritaya yazma: panik
			}
			tamamlanan.Add(1)
			return nil
		},
		Supports:     func(peer.ID) Support { return SupportYes },
		ConnectDelay: time.Millisecond,
		MaxParallel:  1,
		Now:          clk.Now,
	})
	ctx := context.Background()
	s.onConnect(ctx, "bozuk")
	s.wait()
	if st := s.Status(); st.Failures != 1 {
		t.Fatalf("panik başarısız deneme sayılmalı: %+v", st)
	}
	s.onConnect(ctx, "saglam")
	s.wait()
	if tamamlanan.Load() != 1 {
		t.Fatal("panikten sonra yuva serbest kalmalı ve sonraki senkron çalışmalı")
	}
}
