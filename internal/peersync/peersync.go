// Package peersync, AND'ın eşlerle geçmiş senkronizasyonunu (forum, sohbet,
// kişisel site) NE ZAMAN ve KİMLE yapacağına karar verir. Senkronun kendisi
// her deponun SyncWithPeer'ındadır; bu paket yalnızca zamanlar:
//
//   - Bir eş bağlandığında (identify'ın protokolleri öğrenmesi için kısa bir
//     beklemeden sonra) senkron yapılır. AND protokolünü konuşmadığı bilinen
//     eşler (ör. genel IPFS/DHT düğümleri) atlanır; bunlarla akış açmak her
//     seferinde "protokol desteklenmiyor" ile biterdi.
//   - Kopup yeniden bağlanan bir eşle, son başarılı senkronun üzerinden
//     ResyncAfter geçtiyse yeniden senkron yapılır: arada kaçırılan gossip
//     mesajları böylece telafi edilir. Önceden her eşle süreç ömründe yalnızca
//     BİR kez senkron yapılıyordu.
//   - Başarısız bir senkron RetryAfter sonra yeniden denenir.
//   - Her Interval'da bağlı AND eşlerinden en uzun süredir senkronlanmamış
//     PerRound tanesiyle senkron yapılır (anti-entropy) — bağlantı hiç
//     kopmasa da kaybolan gossip mesajları zamanla gelir.
//   - Aynı anda en fazla MaxParallel senkron çalışır; aynı eşle iki senkron
//     asla eşzamanlı açılmaz. Açılışta yüzlerce eş birden bağlandığında
//     yüzlerce akış birden açılmaz.
package peersync

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/lucian95511/and/internal/guvenli"
)

// Support, bir eşin AND'ın senkron protokollerini konuşup konuşmadığıdır.
type Support int

const (
	// SupportUnknown: eşin protokolleri henüz bilinmiyor (identify sürüyor) —
	// bağlantı anında denemeye değer, periyodik turlarda atlanır.
	SupportUnknown Support = iota
	SupportYes
	SupportNo
)

// ProtocolSupport, eşin protos'tan en az birini konuşup konuşmadığını
// peerstore'daki identify bilgisinden okur. Protokolleri henüz bilinmeyen
// (identify sürüyor) bir eş için SupportUnknown döner.
func ProtocolSupport(ps peerstore.Peerstore, p peer.ID, protos ...protocol.ID) Support {
	known, err := ps.GetProtocols(p)
	if err != nil || len(known) == 0 {
		return SupportUnknown
	}
	for _, k := range known {
		if slices.Contains(protos, k) {
			return SupportYes
		}
	}
	return SupportNo
}

// Config, Scheduler'ın bağımlılıkları ve süreleridir; sıfır süreler/sayılar
// varsayılan değerlerini alır.
type Config struct {
	// Sync, tek bir eşle tam senkron yapar; senkron hiç başarılamadıysa hata döner.
	Sync func(ctx context.Context, p peer.ID) error
	// Connected, şu an bağlı eşleri döner (periyodik turlar için).
	Connected func() []peer.ID
	// Supports, eşin AND senkron protokollerini konuşup konuşmadığını söyler.
	Supports func(p peer.ID) Support

	ConnectDelay time.Duration // bağlantıdan sonra bekleme (varsayılan 2 sn)
	ResyncAfter  time.Duration // yeniden bağlanmada tekrar senkron eşiği (varsayılan 2 dk)
	RetryAfter   time.Duration // başarısızlıktan sonra yeniden deneme (varsayılan 1 dk)
	Interval     time.Duration // periyodik tur aralığı (varsayılan 10 dk)
	PerRound     int           // periyodik turda en fazla eş (varsayılan 3)
	MaxParallel  int           // eşzamanlı senkron sınırı (varsayılan 4)

	Now func() time.Time // test için; varsayılan time.Now
}

// Status, senkronizasyonun kısa bir özetidir (arayüzde göstermek için).
type Status struct {
	LastSuccess time.Time // en son başarılı senkronun zamanı (hiç yoksa sıfır)
	Peers       int       // en az bir kez başarıyla senkronlanmış farklı eş sayısı
	Failures    int       // toplam başarısız deneme
}

type peerState struct {
	lastOK, lastFail time.Time
	running          bool
}

// Scheduler, senkron zamanlamasını yürütür; New ile kurulur.
type Scheduler struct {
	cfg Config
	sem chan struct{}
	wg  sync.WaitGroup

	mu       sync.Mutex
	peers    map[peer.ID]*peerState
	lastOK   time.Time
	failures int
}

// New, eksik ayarları varsayılanlarla doldurup bir Scheduler kurar.
func New(cfg Config) *Scheduler {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&cfg.ConnectDelay, 2*time.Second)
	def(&cfg.ResyncAfter, 2*time.Minute)
	def(&cfg.RetryAfter, time.Minute)
	def(&cfg.Interval, 10*time.Minute)
	if cfg.PerRound <= 0 {
		cfg.PerRound = 3
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 4
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Supports == nil {
		cfg.Supports = func(peer.ID) Support { return SupportUnknown }
	}
	if cfg.Connected == nil {
		cfg.Connected = func() []peer.ID { return nil }
	}
	return &Scheduler{cfg: cfg, sem: make(chan struct{}, cfg.MaxParallel), peers: make(map[peer.ID]*peerState)}
}

// Run, ctx bitene kadar bağlantı olaylarını ve periyodik turları işler;
// dönmeden önce süren senkronların bitmesini bekler.
func (s *Scheduler) Run(ctx context.Context, connected <-chan peer.ID) {
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case p := <-connected:
			s.onConnect(ctx, p)
		case <-t.C:
			s.round(ctx)
		case <-ctx.Done():
			s.wg.Wait()
			return
		}
	}
}

// Status, senkronizasyonun özetini döner.
func (s *Scheduler) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{LastSuccess: s.lastOK, Failures: s.failures}
	for _, ps := range s.peers {
		if !ps.lastOK.IsZero() {
			st.Peers++
		}
	}
	return st
}

// onConnect, yeni bağlanan bir eş için (kısa bir beklemeden sonra) gerekirse
// senkron başlatır.
func (s *Scheduler) onConnect(ctx context.Context, p peer.ID) {
	s.wg.Go(func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.ConnectDelay):
		}
		if s.cfg.Supports(p) == SupportNo {
			return
		}
		if s.claim(p, s.cfg.ResyncAfter) {
			s.run(ctx, p)
		}
	})
}

// round, bağlı ve AND protokolünü konuştuğu doğrulanmış eşlerden en uzun
// süredir senkronlanmamış PerRound tanesiyle senkron başlatır.
func (s *Scheduler) round(ctx context.Context) {
	now := s.cfg.Now()
	type aday struct {
		p      peer.ID
		lastOK time.Time
	}
	var adaylar []aday
	s.mu.Lock()
	for _, p := range s.cfg.Connected() {
		if s.cfg.Supports(p) != SupportYes || !s.dueLocked(p, now, s.cfg.Interval) {
			continue
		}
		var last time.Time
		if ps := s.peers[p]; ps != nil {
			last = ps.lastOK
		}
		adaylar = append(adaylar, aday{p, last})
	}
	s.mu.Unlock()

	sort.SliceStable(adaylar, func(i, j int) bool { return adaylar[i].lastOK.Before(adaylar[j].lastOK) })
	for _, a := range adaylar[:min(len(adaylar), s.cfg.PerRound)] {
		if !s.claim(a.p, s.cfg.Interval) {
			continue
		}
		p := a.p
		s.wg.Go(func() { s.run(ctx, p) })
	}
}

// dueLocked, p ile şimdi senkron yapılıp yapılmayacağıdır: zaten
// sürmüyorsa, son başarıdan bu yana minGap ve son başarısızlıktan bu yana
// RetryAfter geçtiyse. s.mu tutulurken çağrılır.
func (s *Scheduler) dueLocked(p peer.ID, now time.Time, minGap time.Duration) bool {
	ps := s.peers[p]
	if ps == nil {
		return true
	}
	if ps.running {
		return false
	}
	if !ps.lastOK.IsZero() && now.Sub(ps.lastOK) < minGap {
		return false
	}
	if !ps.lastFail.IsZero() && ps.lastFail.After(ps.lastOK) && now.Sub(ps.lastFail) < s.cfg.RetryAfter {
		return false
	}
	return true
}

// claim, p için senkron gerekiyorsa onu "sürüyor" olarak işaretler — kontrol
// ve işaretleme tek kilit altında yapılır, aynı eşle iki senkron açılmaz.
func (s *Scheduler) claim(p peer.ID, minGap time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dueLocked(p, s.cfg.Now(), minGap) {
		return false
	}
	ps := s.peers[p]
	if ps == nil {
		ps = &peerState{}
		s.peers[p] = ps
	}
	ps.running = true
	return true
}

// run, claim edilmiş bir eşle (eşzamanlılık sınırı içinde) senkron yapar ve
// sonucu kaydeder.
func (s *Scheduler) run(ctx context.Context, p peer.ID) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		s.finish(p, ctx.Err())
		return
	}
	// Senkron, eşin gönderdiği veriyi çözer: orada tetiklenen bir hata düğümü
	// çökertmez, başarısız bir deneme sayılır (yuva da serbest kalır).
	var err error
	if !guvenli.Isle("eş senkronu", func() { err = s.cfg.Sync(ctx, p) }) {
		err = errSenkronPanik
	}
	<-s.sem
	s.finish(p, err)
}

// errSenkronPanik, panik yapan bir senkronun hatasıdır (ayrıntı günlükte).
var errSenkronPanik = errors.New("peersync: senkron beklenmedik biçimde durdu (bkz. and.log)")

func (s *Scheduler) finish(p peer.ID, err error) {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	ps := s.peers[p]
	ps.running = false
	if err != nil {
		ps.lastFail = now
		s.failures++
		return
	}
	ps.lastOK = now
	s.lastOK = now
}

// wait, başlatılmış bütün senkronların bitmesini bekler (testler için).
func (s *Scheduler) wait() { s.wg.Wait() }
