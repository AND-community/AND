package forum

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Spam protection constants.
// Yazar başına saatte en fazla bu kadar mesaj kabul edilir; geri kalanlar düşürülür.
const (
	rlPostWindow   = time.Hour
	rlMaxPosts     = 10   // saatte en fazla 10 konu
	rlReplyWindow  = time.Hour
	rlMaxReplies   = 40   // saatte en fazla 40 yanıt
	rlMaxBodyBytes = 8192 // ağ katmanında zorunlu tutulan max içerik boyutu (8 KB)

	// rlSaveInterval: dirty state ne sıklıkla diske yazılır.
	// Her izin verilen mesajda ayrı goroutine yerine tek bir arka plan döngüsü kullanılır.
	rlSaveInterval = 30 * time.Second
)

type authorBucket struct {
	PostTimes  []time.Time `json:"post_times,omitempty"`
	ReplyTimes []time.Time `json:"reply_times,omitempty"`
}

// rateLimiter tracks per-author message rates using a sliding time window.
// Keyed by the author's hex-encoded Ed25519 public key so it's tied to a
// cryptographic identity rather than an IP address.
type rateLimiter struct {
	mu      sync.Mutex
	authors map[string]*authorBucket
	dirty   bool   // true → in-memory state is ahead of the on-disk state
	dataDir string
}

func newRateLimiter(dataDir string) *rateLimiter {
	rl := &rateLimiter{
		authors: make(map[string]*authorBucket),
		dataDir: dataDir,
	}
	rl.load()
	return rl
}

// Start launches the background save loop. Call it once after New with a
// context that lives as long as the forum.
func (rl *rateLimiter) Start(ctx context.Context) {
	go rl.saveLoop(ctx)
}

// saveLoop flushes dirty rate-limit state to disk at rlSaveInterval.
// Replaces the previous pattern of spawning a goroutine on every allowed message.
func (rl *rateLimiter) saveLoop(ctx context.Context) {
	ticker := time.NewTicker(rlSaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			dirty := rl.dirty
			rl.dirty = false
			rl.mu.Unlock()
			if dirty {
				rl.save()
			}
		case <-ctx.Done():
			// Final flush on shutdown.
			rl.mu.Lock()
			dirty := rl.dirty
			rl.dirty = false
			rl.mu.Unlock()
			if dirty {
				rl.save()
			}
			return
		}
	}
}

// allowPost returns true and records the attempt if the author is under
// the post rate limit. Returns false if the limit is exceeded.
func (rl *rateLimiter) allowPost(authorKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b := rl.bucket(authorKey)
	cutoff := time.Now().Add(-rlPostWindow)
	b.PostTimes = keepAfter(b.PostTimes, cutoff)

	if len(b.PostTimes) >= rlMaxPosts {
		return false
	}
	b.PostTimes = append(b.PostTimes, time.Now())
	rl.dirty = true
	return true
}

// allowReply returns true and records the attempt if the author is under
// the reply rate limit.
func (rl *rateLimiter) allowReply(authorKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b := rl.bucket(authorKey)
	cutoff := time.Now().Add(-rlReplyWindow)
	b.ReplyTimes = keepAfter(b.ReplyTimes, cutoff)

	if len(b.ReplyTimes) >= rlMaxReplies {
		return false
	}
	b.ReplyTimes = append(b.ReplyTimes, time.Now())
	rl.dirty = true
	return true
}

func (rl *rateLimiter) bucket(key string) *authorBucket {
	b, ok := rl.authors[key]
	if !ok {
		b = &authorBucket{}
		rl.authors[key] = b
	}
	return b
}

// save writes the current rate-limit state to disk. Must NOT be called
// while holding rl.mu (takes its own snapshot under lock, then writes outside).
func (rl *rateLimiter) save() {
	if rl.dataDir == "" {
		return
	}
	cutoff := time.Now().Add(-rlPostWindow)
	rl.mu.Lock()
	out := make(map[string]*authorBucket, len(rl.authors))
	for k, b := range rl.authors {
		fresh := &authorBucket{
			PostTimes:  keepAfter(b.PostTimes, cutoff),
			ReplyTimes: keepAfter(b.ReplyTimes, cutoff),
		}
		if len(fresh.PostTimes)+len(fresh.ReplyTimes) > 0 {
			out[k] = fresh
		}
	}
	rl.mu.Unlock()
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(rl.dataDir, "forum_rates.json"), data, 0o600)
}

func (rl *rateLimiter) load() {
	if rl.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(rl.dataDir, "forum_rates.json"))
	if err != nil {
		return
	}
	var stored map[string]*authorBucket
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	cutoff := time.Now().Add(-rlPostWindow)
	rl.mu.Lock()
	for k, b := range stored {
		fresh := &authorBucket{
			PostTimes:  keepAfter(b.PostTimes, cutoff),
			ReplyTimes: keepAfter(b.ReplyTimes, cutoff),
		}
		if len(fresh.PostTimes)+len(fresh.ReplyTimes) > 0 {
			rl.authors[k] = fresh
		}
	}
	rl.mu.Unlock()
}

func keepAfter(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && !ts[i].After(cutoff) {
		i++
	}
	return ts[i:]
}
