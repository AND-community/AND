package forum

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucian95511/and/internal/hizsiniri"
	"github.com/lucian95511/and/internal/timewin"
)

// Spam protection constants.
// Yazar başına saatte en fazla bu kadar mesaj kabul edilir; geri kalanlar düşürülür.
const (
	rlPostWindow     = time.Hour
	rlMaxPosts       = 10  // saatte en fazla 10 konu (yazar başına)
	rlGlobalMaxPosts = 200 // saatte en fazla 200 konu (tüm ağdan) — Sybil koruması
	rlReplyWindow    = time.Hour
	rlMaxReplies     = 40   // saatte en fazla 40 yanıt
	rlMaxBodyBytes   = 8192 // ağ katmanında zorunlu tutulan max içerik boyutu (8 KB)

	// MaxBodyBytes, rlMaxBodyBytes'ın dışa açık takma adıdır — internal/tui
	// gibi çağıranların, CreatePost/CreateReply'a göndermeden ÖNCE bayt
	// (rune DEĞİL) bazlı bir istemci-taraflı doğrulama yapabilmesi için.
	// bkz. bu paketin handleMessage'daki len(Body) > rlMaxBodyBytes
	// kontrolüyle aynı sınır: bir metin girişi widget'ının rune sayan
	// CharLimit'i buradan daha büyük tutulursa, çok baytlı (ör. Türkçe)
	// karakterlerle yazılan içerik yerelde "gönderildi" görünüp ağdaki
	// diğer düğümlerce sessizce reddedilebilir.
	MaxBodyBytes = rlMaxBodyBytes

	// "delete" mesaj tipi eskiden HİÇBİR hız sınırına tabi değildi — bir
	// saldırgan, henüz yerel olarak görülmemiş rastgele/tahmin edilen
	// post ID'leri için sınırsız sayıda sahte (kendi imzasıyla atılmış,
	// ama gerçek postun yazarıyla eşleşmesi gerekmeyen) silme mesajı
	// yayınlayıp `deleted` tombstone haritasını doldurabilir ve gerçek
	// postlar daha sonra geldiğinde sessizce reddedilmesine (bir tür
	// sansür) yol açabilirdi — bkz. applyDelete. rlMaxDeletes/
	// rlGlobalMaxDeletes bunu allowPost/allowGlobalMaxPosts ile aynı
	// desende sınırlar.
	rlMaxDeletes       = 20  // saatte en fazla 20 silme (yazar başına)
	rlGlobalMaxDeletes = 200 // saatte en fazla 200 silme (tüm ağdan)

	// rlSaveInterval: dirty state ne sıklıkla diske yazılır.
	// Her izin verilen mesajda ayrı goroutine yerine tek bir arka plan döngüsü kullanılır.
	rlSaveInterval = 30 * time.Second

	// Senkronla gelen geçmiş içerik canlı sayaçlara sayılmaz (bkz.
	// hizsiniri.Gecmis): yazar başına sınır içeriğin yazıldığı saate göre
	// rlMaxPosts/rlMaxReplies'tır; toplam, varış zamanına göre ayrı ve daha
	// geniş bir saatlik bütçeyle sınırlanır (sahte kimliklerle yığılmaya karşı).
	rlSyncGlobalMaxPosts   = 5000
	rlSyncGlobalMaxReplies = 20000
)

// rlPostCooldown/rlReplyCooldown: art arda hızlı gönderime (patlama/burst
// spam) karşı ek koruma — saatlik toplam sınır (rlMaxPosts/rlMaxReplies)
// içinde bile biri birkaç saniyede tüm saatlik kotasını tüketebilir; bu,
// aynı yazardan ardışık iki kabul arasında minimum bir bekleme süresi
// zorunlu kılar. Vars (const değil) ki testler saf sayaç-tabanlı
// senaryolarda bunu sıfıra çekip zamandan bağımsız, deterministik kalabilsin.
var (
	rlPostCooldown  = 20 * time.Second
	rlReplyCooldown = 5 * time.Second
)

// Yeni kimlik kısıtlaması: bu eş bir AuthorKey'i İLK KEZ gördüğünde
// (bkz. authorBucket.FirstSeen), newIdentityGracePeriod boyunca o yazar
// normal (rlMaxPosts/rlMaxReplies/rlPostCooldown) yerine çok daha sıkı bir
// hız sınırına tabi tutulur. Bu, susturulmuş/banlanmış birinin yeni bir
// ed25519 anahtarıyla ("Sybil") anında eski hızında devam etmesini
// engellemez — yeni bir kimlik üretmek hâlâ bedava — ama bunu YAPMANIN
// getirisini düşürür: yeni kimlik, en az bir gün boyunca eskisinden çok
// daha yavaş ilerlemek zorunda kalır. Vars ki testler küçültüp
// deterministik kalabilsin.
var (
	newIdentityGracePeriod        = 24 * time.Hour
	newIdentityMaxPosts           = 3  // rlMaxPosts (10) yerine
	newIdentityMaxReplies         = 10 // rlMaxReplies (40) yerine
	newIdentityCooldownMultiplier = 4  // rlPostCooldown/rlReplyCooldown'ı bu kadar çarpar
)

// isNewIdentity reports whether b's author is still within its
// newIdentityGracePeriod. A zero FirstSeen (state loaded from a
// forum_rates.json written before this field existed) is treated as an
// established identity, not a new one — the throttle is meant to slow down
// freshly-minted keys, not retroactively punish pre-existing authors just
// because we started tracking this field after they were already active.
func isNewIdentity(b *authorBucket, now time.Time) bool {
	return !b.FirstSeen.IsZero() && now.Sub(b.FirstSeen) < newIdentityGracePeriod
}

// Otomatik yerel susturma: ısrarla hız sınırını ihlal eden bir yazar,
// moderatör müdahalesi olmadan bir süreliğine görmezden gelinir — ağ
// geneli bir yasak DEĞİL, yalnızca BU eşin o yazardan gelen konu/yanıtları
// (imza doğrulaması dahil hiçbir işlem yapmadan) atlaması. Vars (const
// değil) ki testler bunları küçültüp deterministik kalabilsin.
//
// Bilinen sınır: bu, AuthorKey'e göre izleniyor — yeni bir ed25519 anahtar
// çifti üretmek bedavadır, dolayısıyla susturulmuş biri anında yeni bir
// kimlikle "temiz" başlayabilir. Bu, bireysel-kimlik susturmasının doğal
// bir zayıflığıdır; asıl Sybil koruması rlGlobalMaxPosts/rlGlobalMaxDeletes
// gibi kimlik sayısından bağımsız, ağ genelinde sabit toplam giriş oranı
// tavanlarıdır. Susturma, tek bir ısrarlı kimliğin gürültüsünü azaltmak
// içindir, kararlı bir Sybil savunması olarak tasarlanmamıştır.
var (
	muteViolationWindow    = 10 * time.Minute
	muteViolationThreshold = 10 // bu pencere içinde bu kadar ihlalden sonra sustur
	muteDuration           = 30 * time.Minute
)

// Kademeli susturma: aynı yazar tekrar tekrar susturma eşiğine ulaşırsa,
// her yeni susturma bir öncekinden daha uzun sürer (MuteCount'a göre
// muteDuration'ın katları, muteDurationCap'te sınırlanır) — "sustur, süresi
// dolunca aynı şekilde devam et" döngüsünü zorlaştırır. muteCountDecay
// kadar süre boyunca hiç yeni susturma tetiklenmezse sayaç sıfırlanır;
// yoksa geçmişte bir kez sorun çıkarmış biri sonsuza dek cezalandırılmış
// olurdu. Vars ki testler küçültüp deterministik kalabilsin.
var (
	muteDurationCap = 12 * time.Hour
	muteCountDecay  = 7 * 24 * time.Hour
)

// dupEntry tracks how many times (and when last) an author has sent one
// particular normalized post/reply body — see isDuplicate. Fields are
// exported (with lowercase JSON tags) so authorBucket.RecentTexts can be
// persisted to disk — see save/load.
type dupEntry struct {
	Count int       `json:"count"`
	At    time.Time `json:"at"` // en son görülme zamanı
}

// dupWindow/dupMaxRepeats bound repeat/near-duplicate content detection:
// the same (normalized) body from the same author — post or reply, they
// share one counter, since the goal is catching repeated content
// regardless of which form it takes — is allowed up to dupMaxRepeats times
// within dupWindow before further repeats are rejected. The per-body
// counter resets once dupWindow has elapsed since its last occurrence.
var (
	dupWindow     = 10 * time.Minute
	dupMaxRepeats = 6
)

type authorBucket struct {
	PostTimes   []time.Time `json:"post_times,omitempty"`
	ReplyTimes  []time.Time `json:"reply_times,omitempty"`
	DeleteTimes []time.Time `json:"delete_times,omitempty"`

	// FirstSeen is when this eş first saw a message from this author —
	// see isNewIdentity. Persisted so a restart doesn't make every known
	// author look "new" again.
	FirstSeen time.Time `json:"first_seen,omitempty"`

	// ViolationTimes/MutedUntil: automatic local mute escalation — see
	// recordViolation/muted. Persisted to disk (see save/load) so a
	// restart doesn't hand a persistent spammer an instant clean slate.
	ViolationTimes []time.Time `json:"violation_times,omitempty"`
	MutedUntil     time.Time   `json:"muted_until,omitempty"`

	// MuteCount/LastMuteAt: how many times this author has been
	// auto-muted, and when the most recent one started — see
	// recordViolation's escalation logic.
	MuteCount  int       `json:"mute_count,omitempty"`
	LastMuteAt time.Time `json:"last_mute_at,omitempty"`

	// RecentTexts: per-normalized-body repeat counters — see isDuplicate.
	// Persisted alongside everything else in this bucket.
	RecentTexts map[string]dupEntry `json:"recent_texts,omitempty"`
}

// rateLimiter tracks per-author message rates using a sliding time window.
// Keyed by the author's hex-encoded Ed25519 public key so it's tied to a
// cryptographic identity rather than an IP address.
type rateLimiter struct {
	mu            sync.Mutex
	authors       map[string]*authorBucket
	globalPosts   []time.Time // tüm yazarlardan gelen postlar — Sybil koruması
	globalDeletes []time.Time // tüm yazarlardan gelen silmeler — Sybil koruması
	dirty         bool        // true → in-memory state is ahead of the on-disk state
	dataDir       string

	// Senkronla gelen geçmiş içeriğin sayaçları (canlı sayaçlardan ayrı).
	senkronKonu  hizsiniri.Gecmis
	senkronYanit hizsiniri.Gecmis
}

// allowSyncedPost, senkronla gelen bir konuyu yazıldığı saate göre sınar;
// canlı sayaçlara (allowPost) dokunmaz, böylece büyük bir geçmiş senkronu ağdan
// şimdi gelen konuları reddettirmez.
func (rl *rateLimiter) allowSyncedPost(authorKey string, createdAt time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.senkronKonu.Izin(authorKey, createdAt, time.Now(), rlMaxPosts, rlSyncGlobalMaxPosts, postTTL)
}

// allowSyncedReply, allowSyncedPost'un yanıtlar için karşılığıdır.
func (rl *rateLimiter) allowSyncedReply(authorKey string, createdAt time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.senkronYanit.Izin(authorKey, createdAt, time.Now(), rlMaxReplies, rlSyncGlobalMaxReplies, postTTL)
}

func newRateLimiter(dataDir string) *rateLimiter {
	rl := &rateLimiter{
		authors: make(map[string]*authorBucket),
		dataDir: dataDir,
	}
	rl.load()
	return rl
}

// muted reports whether authorKey is currently under a local auto-mute.
func (rl *rateLimiter) muted(authorKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.authors[authorKey]
	return ok && !b.MutedUntil.IsZero() && time.Now().Before(b.MutedUntil)
}

// recordViolation tracks a rate-limit rejection for authorKey — only ever
// called for live-received rejections (forum.go's handleMessage), never
// from sync backfill (SyncWithPeer), since a prolific author's legitimately
// old posts/replies shouldn't count against them just because sync
// processes many at once — and escalates to a temporary local mute if
// they've violated too many times too fast.
func (rl *rateLimiter) recordViolation(authorKey string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-muteViolationWindow)
	b := rl.bucket(authorKey)
	b.ViolationTimes = append(keepAfter(b.ViolationTimes, cutoff), now)
	if len(b.ViolationTimes) >= muteViolationThreshold {
		wasMuted := !b.MutedUntil.IsZero() && now.Before(b.MutedUntil)
		if !wasMuted {
			if !b.LastMuteAt.IsZero() && now.Sub(b.LastMuteAt) > muteCountDecay {
				b.MuteCount = 0
			}
			b.MuteCount++
			duration := muteDuration * time.Duration(b.MuteCount)
			if duration > muteDurationCap {
				duration = muteDurationCap
			}
			b.MutedUntil = now.Add(duration)
			b.LastMuteAt = now
		}
	}
	rl.dirty = true
}

// normalizeForDup collapses trivial formatting differences (case, leading/
// trailing whitespace) so near-identical repeats are recognized as such.
func normalizeForDup(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isDuplicate reports whether text (from authorKey) has already been
// repeated more than dupMaxRepeats times within dupWindow, recording this
// occurrence either way. Only ever called for live-received content,
// matching recordViolation's scope.
func (rl *rateLimiter) isDuplicate(authorKey, text string) bool {
	norm := normalizeForDup(text)
	if norm == "" {
		return false
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()

	b := rl.bucket(authorKey)
	byText := b.RecentTexts
	if byText == nil {
		byText = make(map[string]dupEntry)
		b.RecentTexts = byText
	} else {
		// Bu vesileyle süresi dolmuş kayıtları temizle — her benzersiz metin
		// için haritanın sonsuza dek büyümesini önler.
		for k, e := range byText {
			if now.Sub(e.At) > dupWindow {
				delete(byText, k)
			}
		}
	}

	entry, ok := byText[norm]
	if !ok {
		byText[norm] = dupEntry{Count: 1, At: now}
		rl.dirty = true
		return false
	}
	entry.Count++
	entry.At = now
	byText[norm] = entry
	rl.dirty = true
	return entry.Count > dupMaxRepeats
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
// the post rate limit. Returns false if the limit is exceeded. checkCooldown
// should be true for live GossipSub receipt (handleMessage) and false for
// sync backfill (SyncWithPeer) — a batch of a prolific author's legitimately
// old posts, replayed in a tight loop during sync, would otherwise trip the
// cooldown purely because of OUR processing speed, not theirs.
func (rl *rateLimiter) allowPost(authorKey string, checkCooldown bool) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rlPostWindow)

	rl.globalPosts = keepAfter(rl.globalPosts, cutoff)
	if len(rl.globalPosts) >= rlGlobalMaxPosts {
		return false
	}

	b := rl.bucket(authorKey)
	maxPosts, cooldown := rlMaxPosts, rlPostCooldown
	// Yeni-kimlik kısıtlaması yalnızca CANLI gossip alımında uygulanır
	// (checkCooldown==true) — sync backfill (checkCooldown==false), zaten
	// var olan (bu eş için sadece yeni "keşfedilen") üretken bir yazarın
	// geçmiş gönderilerini senkronize ederken onu da "yeni kimlik" sayıp
	// gereksiz yere kısıtlamamalı. Bkz. checkCooldown'ın kendi doc yorumu —
	// aynı gerekçe burada da geçerli.
	if checkCooldown && isNewIdentity(b, now) {
		maxPosts = newIdentityMaxPosts
		cooldown = rlPostCooldown * time.Duration(newIdentityCooldownMultiplier)
	}
	b.PostTimes = keepAfter(b.PostTimes, cutoff)
	if len(b.PostTimes) >= maxPosts {
		return false
	}
	if checkCooldown {
		if n := len(b.PostTimes); n > 0 && now.Sub(b.PostTimes[n-1]) < cooldown {
			return false
		}
	}

	b.PostTimes = append(b.PostTimes, now)
	rl.globalPosts = append(rl.globalPosts, now)
	rl.dirty = true
	return true
}

// allowReply returns true and records the attempt if the author is under
// the reply rate limit. See allowPost's checkCooldown doc.
func (rl *rateLimiter) allowReply(authorKey string, checkCooldown bool) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b := rl.bucket(authorKey)
	cutoff := now.Add(-rlReplyWindow)
	b.ReplyTimes = keepAfter(b.ReplyTimes, cutoff)

	maxReplies, cooldown := rlMaxReplies, rlReplyCooldown
	if checkCooldown && isNewIdentity(b, now) {
		maxReplies = newIdentityMaxReplies
		cooldown = rlReplyCooldown * time.Duration(newIdentityCooldownMultiplier)
	}
	if len(b.ReplyTimes) >= maxReplies {
		return false
	}
	if checkCooldown {
		if n := len(b.ReplyTimes); n > 0 && now.Sub(b.ReplyTimes[n-1]) < cooldown {
			return false
		}
	}
	b.ReplyTimes = append(b.ReplyTimes, now)
	rl.dirty = true
	return true
}

// allowDelete returns true and records the attempt if authorKey is under
// the delete rate limit — mirrors allowPost/allowReply's per-author +
// global sliding-window caps (see rlMaxDeletes/rlGlobalMaxDeletes's doc
// comment for why this exists), reusing rlPostWindow since deletes are
// deliberate/rare enough not to need their own window. No cooldown
// parameter: a burst cooldown targets rapid-fire posting, not the
// "unbounded forged tombstones" concern a delete flood poses.
func (rl *rateLimiter) allowDelete(authorKey string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rlPostWindow)

	rl.globalDeletes = keepAfter(rl.globalDeletes, cutoff)
	if len(rl.globalDeletes) >= rlGlobalMaxDeletes {
		return false
	}

	b := rl.bucket(authorKey)
	b.DeleteTimes = keepAfter(b.DeleteTimes, cutoff)
	if len(b.DeleteTimes) >= rlMaxDeletes {
		return false
	}

	b.DeleteTimes = append(b.DeleteTimes, now)
	rl.globalDeletes = append(rl.globalDeletes, now)
	rl.dirty = true
	return true
}

func (rl *rateLimiter) bucket(key string) *authorBucket {
	b, ok := rl.authors[key]
	if !ok {
		b = &authorBucket{FirstSeen: time.Now()}
		rl.authors[key] = b
	}
	return b
}

// pruneBucket returns a fresh *authorBucket containing only b's still-live
// state as of now: sliding-window timestamps older than their own window
// dropped, an already-expired mute cleared, and stale duplicate-text
// entries removed. Shared by save (pruning before writing to disk) and
// load (pruning stale data read back from a possibly-old file).
func pruneBucket(b *authorBucket, now time.Time) *authorBucket {
	postCutoff := now.Add(-rlPostWindow)
	replyCutoff := now.Add(-rlReplyWindow)
	violationCutoff := now.Add(-muteViolationWindow)
	fresh := &authorBucket{
		PostTimes:      keepAfter(b.PostTimes, postCutoff),
		ReplyTimes:     keepAfter(b.ReplyTimes, replyCutoff),
		DeleteTimes:    keepAfter(b.DeleteTimes, postCutoff),
		FirstSeen:      b.FirstSeen,
		ViolationTimes: keepAfter(b.ViolationTimes, violationCutoff),
		MuteCount:      b.MuteCount,
		LastMuteAt:     b.LastMuteAt,
	}
	if b.MutedUntil.After(now) {
		fresh.MutedUntil = b.MutedUntil
	}
	for text, e := range b.RecentTexts {
		if now.Sub(e.At) <= dupWindow {
			if fresh.RecentTexts == nil {
				fresh.RecentTexts = make(map[string]dupEntry)
			}
			fresh.RecentTexts[text] = e
		}
	}
	return fresh
}

// bucketEmpty reports whether b has nothing left worth keeping/persisting,
// as of now. A live mute-escalation history (MuteCount, within
// muteCountDecay of LastMuteAt) is kept even once everything else has
// expired — otherwise a repeat offender's escalation would reset the
// moment their violation window/mute duration (minutes) elapses, long
// before muteCountDecay (days) actually intends it to.
func bucketEmpty(b *authorBucket, now time.Time) bool {
	hasMuteHistory := b.MuteCount > 0 && !b.LastMuteAt.IsZero() && now.Sub(b.LastMuteAt) <= muteCountDecay
	return len(b.PostTimes) == 0 && len(b.ReplyTimes) == 0 && len(b.DeleteTimes) == 0 &&
		len(b.ViolationTimes) == 0 && b.MutedUntil.IsZero() && len(b.RecentTexts) == 0 &&
		!hasMuteHistory
}

// save writes the current rate-limit state to disk. Must NOT be called
// while holding rl.mu (takes its own snapshot under lock, then writes outside).
func (rl *rateLimiter) save() {
	if rl.dataDir == "" {
		return
	}
	now := time.Now()
	rl.mu.Lock()
	out := make(map[string]*authorBucket, len(rl.authors))
	for k, b := range rl.authors {
		fresh := pruneBucket(b, now)
		if !bucketEmpty(fresh, now) {
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
	now := time.Now()
	rl.mu.Lock()
	for k, b := range stored {
		fresh := pruneBucket(b, now)
		if !bucketEmpty(fresh, now) {
			rl.authors[k] = fresh
		}
	}
	rl.mu.Unlock()
}

var keepAfter = timewin.KeepAfter
