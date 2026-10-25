package moderation

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/guvenli"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/control"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	lp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/lucian95511/and/protokol"
)

const ModerationTopic = protokol.ModerasyonKonusu

const FounderKeyFile = "founder.key"

const (
	maxCertDays       = 7
	maxBanDays        = 30
	banRateWindow     = time.Hour
	banRateLimit      = 20
	seenCacheSize     = 4096
	ratesSaveInterval = 30 * time.Second

	// minBanDuration, geçici (Permanent olmayan) bir banın ExpiresAt'inin
	// imzalandığı andan en az ne kadar ileride olması gerektiğidir — bir
	// banın anında (ya da geçmişte) sona ermesini önler. andmod'un
	// --duration ile kabul ettiği en kısa süre de budur (ör. "1h" bir chat
	// yasağı için geçerli, "10s" değil).
	minBanDuration = time.Minute

	// maxHistoryEntries, dataDir/mod_history.json'da tutulan denetim
	// kaydının üst sınırı — sınırsız büyümesini önler (bkz. recordHistory).
	maxHistoryEntries = 500
)

// MaxCertDays, verifyCert'in kabul ettiği (kalıcı olmayan) bir sertifikanın
// en uzun geçerlilik süresidir (gün).
const MaxCertDays = maxCertDays

// defaultFounderPubKeyHex, derleme sırasında -ldflags ile GERÇEK bir kurucu
// anahtarı gömülmediğini gösteren, kasıtlı olarak geçersiz (32 bayttan farklı
// uzunlukta) bir yer tutucudur. LoadFounderKey, "derleme zamanında bir kurucu
// anahtarı ayarlandı mı?" sorusuna yanıt vermek için FounderPubKeyHex'in hâlâ
// bu değere eşit olup olmadığına bakar.
const defaultFounderPubKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"

var FounderPubKeyHex = defaultFounderPubKeyHex

type ModeratorCert struct {
	ModeratorKey string `json:"moderator_key"`
	// IssuedAt imzanın kapsadığı bir alandır (bkz. ModeratorCertPayload) —
	// sertifikayı elinde tutan taraf bunu değiştirip iptali (revoke)
	// atlatamaz. Bir sertifika, IssuedAt bir iptalden ÖNCEYSE geçersiz
	// sayılır; SONRA yeniden verilmişse geçerlidir.
	IssuedAt  time.Time `json:"issued_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Permanent bool      `json:"permanent,omitempty"`
	Sig       string    `json:"sig"`
}

type BanMsg struct {
	BannedPeer string        `json:"banned_peer"`
	Reason     string        `json:"reason"`
	ExpiresAt  time.Time     `json:"expires_at,omitempty"`
	Permanent  bool          `json:"permanent,omitempty"`
	Cert       ModeratorCert `json:"cert"`
	// IssuedAt, bu banın imzalandığı andır — bkz. Moderator.unbannedSince'in
	// doc yorumu: bunsuz, süresi dolmadan geri alınmış (unban edilmiş) bir
	// banın ESKİ dosyası (ör. bans/ban_<peer>.json), banı VEREN moderatörün
	// kendi düğümü yeniden başladığında (PublishSavedBans ile) yeniden
	// yayınlanır ve gerçek unban'ı görmemiş her düğümde sessizce yeniden
	// uygulanırdı — alreadySeen yalnızca BELLEK İÇİ bir tekrar önleme
	// önbelleğidir, yeniden başlatmada sıfırlanır, bu yüzden tek başına
	// yeterli değildir.
	IssuedAt time.Time `json:"issued_at,omitempty"`
	Sig      string    `json:"sig"`
}

type RevokeMsg struct {
	ModeratorKey string    `json:"moderator_key"`
	IssuedAt     time.Time `json:"issued_at"`
	Sig          string    `json:"sig"`
}

// UnbanMsg, bir moderatörün (geçerli bir sertifikayla) daha önce verdiği
// bir banı geri almasını sağlar — BanMsg'in kalıcılığının tek istisnası:
// aksi halde bir ban, süresi (en fazla maxBanDays) dolana kadar geri
// alınamazdı; hatalı ya da haksız bir ban için tek çare beklemek olurdu.
type UnbanMsg struct {
	BannedPeer string        `json:"banned_peer"`
	Cert       ModeratorCert `json:"cert"`
	IssuedAt   time.Time     `json:"issued_at"`
	Sig        string        `json:"sig"`
}

type Envelope struct {
	Type   string     `json:"type"`
	Ban    *BanMsg    `json:"ban,omitempty"`
	Revoke *RevokeMsg `json:"revoke,omitempty"`
	Unban  *UnbanMsg  `json:"unban,omitempty"`
}

type PersistedBan struct {
	PeerID       string    `json:"peer_id"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Permanent    bool      `json:"permanent,omitempty"`
	ModeratorKey string    `json:"moderator_key,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}

// banRecord holds one active ban's state — mirrors ModeratorCert's
// Permanent pattern: a permanent ban's ExpiresAt is unused/zero, checked
// via Permanent instead (see IsBanned), rather than encoding "forever" as
// some far-future sentinel timestamp.
type banRecord struct {
	ExpiresAt    time.Time
	Permanent    bool
	ModeratorKey string
	Reason       string
}

type seenSet struct {
	mu   sync.Mutex
	m    map[string]struct{}
	ring []string
	pos  int
}

func newSeenSet(size int) *seenSet {
	return &seenSet{
		m:    make(map[string]struct{}, size),
		ring: make([]string, size),
	}
}

func (s *seenSet) seen(sig string) bool {
	h := sha256.Sum256([]byte(sig))
	fp := hex.EncodeToString(h[:16])

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.m[fp]; ok {
		return true
	}
	if old := s.ring[s.pos]; old != "" {
		delete(s.m, old)
	}
	s.m[fp] = struct{}{}
	s.ring[s.pos] = fp
	s.pos = (s.pos + 1) % len(s.ring)
	return false
}

type Moderator struct {
	mu        sync.RWMutex
	bans      map[peer.ID]*banRecord
	revoked   map[string]time.Time
	founder   ed25519.PublicKey
	founderID peer.ID
	dataDir   string

	// unbannedSince, peer başına en son GEÇERLİ (imza/sertifika/hız sınırı
	// doğrulanmış) unban'ın IssuedAt'ini tutar — m.bans'tan (yalnızca GÜNCEL
	// durumu tutar, bir ban kaldırılınca haritadan tamamen silinir) BİLEREK
	// AYRIDIR ve bir peer artık banlı olmasa bile SİLİNMEZ. applyBan, yeni
	// gelen bir banın IssuedAt'ini burada kayıtlı watermark'a karşı
	// denetler — watermark'tan eski ya da eşit bir ban reddedilir. Bunsuz,
	// unban'ı hiç görmemiş (ör. yeniden başlamış, alreadySeen önbelleği
	// sıfırlanmış) bir düğüm, banı VEREN moderatörün eski ban_<peer>.json
	// dosyasını (PublishSavedBans ile açılışta yeniden yayınlanan) tekrar
	// uygulayıp banı sessizce diriltebilirdi.
	unbannedSince map[peer.ID]time.Time

	seenBans    *seenSet
	seenRevokes *seenSet
	seenUnbans  *seenSet

	rateMu     sync.Mutex
	rates      map[string][]time.Time
	ratesDirty bool

	// history, bans/revoked'tan ayrı bir kalıcı denetim kaydıdır — bunlar
	// yalnızca GÜNCEL durumu tutar (bir unban bans'tan silinir), history
	// hiçbir zaman silinmeyen, en yeni önce sıralı bir olay listesidir.
	historyMu sync.Mutex
	history   []HistoryEntry

	// grants, bu düğüm kurucu olarak IssueGrant ile verdiği sertifikaların
	// yerel defteridir — ağda "kim moderatör" diye yayınlanan bir kayıt
	// olmadığından, yalnızca kurucunun kendi geçmişi için tutulur.
	grantsMu sync.Mutex
	grants   []GrantedMod

	// certCache, FindModCert'in bans/ dizinini tam tarayan sonucunu KISA
	// bir süre önbelleğe alır — bkz. findCertCached'in doc yorumu.
	certCacheMu sync.Mutex
	certCache   map[string]certCacheEntry

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// LoadFounderKey, dataDir/founder.key dosyasını okuyup FounderPubKeyHex'i
// buna göre ayarlar ve bu düğümün kurucu olup olmadığını döner.
//
// founder.key henüz yoksa iki durum vardır:
//   - Binary -ldflags "-X .../FounderPubKeyHex=<hex>" ile GERÇEK bir kurucu
//     anahtarıyla derlendiyse (FounderPubKeyHex artık defaultFounderPubKeyHex
//     DEĞİLSE), bu değer OTORİTERDİR ve dosyaya AYNEN yazılır — myPubKeyHex
//     yalnızca bununla karşılaştırılıp isFounder'ı belirlemek için kullanılır.
//     Aksi davranış (myPubKeyHex'i doğrudan dosyaya yazmak), derleme
//     zamanında gömülü gerçek kurucu anahtarını YOK SAYAR ve ilk çalıştıran
//     HERKESİ kendi kurucusu ilan eder — moderasyonun tüm güven zincirini
//     kırar (gerçek kurucunun ban/moderatör mesajları artık hiç kabul
//     edilmez, çünkü karşılaştırılan anahtar artık o kullanıcının kendi
//     anahtarıdır).
//   - Hiçbir kurucu anahtarı gömülmediyse (geliştirme/tekli-düğüm testi),
//     bu düğüm kendi kimliğini kurucu olarak başlatır (eski davranış).
//
// os.O_EXCL ile atomik oluşturma, aynı veri dizinini paylaşan iki sürecin
// aynı anda ilk kez başlamasında birinin yazdığı değerin diğerince
// sessizce ezilmesini önler — yarışı kaybeden taraf dosyayı yeniden okur.
func LoadFounderKey(dataDir, myPubKeyHex string) (isFounder bool, err error) {
	path := filepath.Join(dataDir, FounderKeyFile)

	key := FounderPubKeyHex
	if key == defaultFounderPubKeyHex {
		key = myPubKeyHex
	}
	f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if createErr == nil {
		_, werr := f.WriteString(key)
		cerr := f.Close()
		if werr != nil {
			return false, fmt.Errorf("moderation: founder.key yazılamadı: %w", werr)
		}
		if cerr != nil {
			return false, fmt.Errorf("moderation: founder.key yazılamadı: %w", cerr)
		}
		FounderPubKeyHex = key
		return strings.EqualFold(key, myPubKeyHex), nil
	}
	if !os.IsExist(createErr) {
		return false, fmt.Errorf("moderation: founder.key yazılamadı: %w", createErr)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return false, fmt.Errorf("moderation: founder.key okunamadı: %w", readErr)
	}

	fileKey := strings.TrimSpace(string(data))
	if len(fileKey) != 64 {
		return false, fmt.Errorf("moderation: founder.key geçersiz (64 hex karakter olmalı, %d var)", len(fileKey))
	}
	if _, decErr := hex.DecodeString(fileKey); decErr != nil {
		return false, fmt.Errorf("moderation: founder.key geçersiz hex: %w", decErr)
	}

	FounderPubKeyHex = fileKey
	return strings.EqualFold(fileKey, myPubKeyHex), nil
}

func New(dataDir string, founderPeerID peer.ID) (*Moderator, error) {
	pub, err := hex.DecodeString(FounderPubKeyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("moderation: geçersiz kurucu key: %w", err)
	}

	m := &Moderator{
		bans:          make(map[peer.ID]*banRecord),
		revoked:       make(map[string]time.Time),
		unbannedSince: make(map[peer.ID]time.Time),
		founder:       ed25519.PublicKey(pub),
		founderID:     founderPeerID,
		dataDir:       dataDir,
		rates:         make(map[string][]time.Time),
		seenBans:      newSeenSet(seenCacheSize),
		seenRevokes:   newSeenSet(seenCacheSize),
		seenUnbans:    newSeenSet(seenCacheSize),
	}

	m.loadBans()
	m.loadRevocations()
	m.loadUnbanWatermarks()
	m.loadRates()
	m.loadHistory()
	m.loadGrants()
	return m, nil
}

func (m *Moderator) isDisabled() bool {
	for _, b := range m.founder {
		if b != 0 {
			return false
		}
	}
	return true
}

func (m *Moderator) Start(ctx context.Context, topic *network.Topic) {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	ch := topic.Messages(ctx)

	m.wg.Add(3)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				guvenli.Isle("moderasyon iletisi", func() { m.handleEnvelope(data) })
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		defer m.wg.Done()
		m.cleanupLoop(ctx)
	}()
	go func() {
		defer m.wg.Done()
		m.ratesSaveLoop(ctx)
	}()
}

// Stop, Start tarafından başlatılan arka plan goroutine'lerini durdurur ve
// hepsi tamamen sonlanana kadar bloklar. Özellikle testlerde, dataDir'i
// silmeden (ör. t.TempDir() temizliği) önce çağrılmalıdır — aksi halde hâlâ
// süren bir dosya yazımı (saveBans/saveRates/appendHistory) dizin silme
// işlemiyle yarışabilir ve Windows'ta "directory not empty" hatasına yol
// açabilir.
func (m *Moderator) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

func (m *Moderator) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredBans()
		case <-ctx.Done():
			return
		}
	}
}

func (m *Moderator) cleanupExpiredBans() {
	now := time.Now()
	m.mu.Lock()
	changed := false
	for pid, rec := range m.bans {
		if !rec.Permanent && now.After(rec.ExpiresAt) {
			delete(m.bans, pid)
			changed = true
		}
	}
	m.mu.Unlock()
	if changed {
		m.saveBans()
	}
}

func (m *Moderator) ratesSaveLoop(ctx context.Context) {
	ticker := time.NewTicker(ratesSaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.rateMu.Lock()
			dirty := m.ratesDirty
			m.ratesDirty = false
			m.rateMu.Unlock()
			if dirty {
				m.saveRates()
			}
		case <-ctx.Done():
			m.rateMu.Lock()
			dirty := m.ratesDirty
			m.ratesDirty = false
			m.rateMu.Unlock()
			if dirty {
				m.saveRates()
			}
			return
		}
	}
}

func (m *Moderator) PublishBan(ctx context.Context, topic *network.Topic, ban BanMsg) error {
	env := Envelope{Type: "ban", Ban: &ban}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return topic.Publish(ctx, data)
}

func (m *Moderator) PublishRevoke(ctx context.Context, topic *network.Topic, rev RevokeMsg) error {
	env := Envelope{Type: "revoke", Revoke: &rev}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return topic.Publish(ctx, data)
}

func (m *Moderator) PublishUnban(ctx context.Context, topic *network.Topic, u UnbanMsg) error {
	env := Envelope{Type: "unban", Unban: &u}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return topic.Publish(ctx, data)
}

// PublishSavedBans, dataDir/bans altındaki "ban_*.json"/"revoke_*.json"/
// "unban_*.json" dosyalarını (andmod'un ürettiği ham kayıtlar) Envelope'a
// sarıp moderasyon topic'ine yayınlar. AND her başlangıçta bunu çağırır —
// böylece düğüm yeniden başladığında ya da ağa yeni katılan peer'lar bu
// eylemleri (yeniden) öğrenir.
func (m *Moderator) PublishSavedBans(ctx context.Context, topic *network.Topic, dataDir string) {
	entries, err := os.ReadDir(filepath.Join(dataDir, "bans"))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		// mod_*.json (moderatör sertifikaları) burada yayınlanmaz: onlar
		// bağımsız bir protokol mesajı değil, yerel diskten
		// FindModCert/FindOwnCert ile okunan ve yalnızca bir BanMsg içine
		// gömülüp o üzerinden ağa taşınan verilerdir.
		switch {
		case strings.HasPrefix(name, "ban_"):
			data, err := os.ReadFile(filepath.Join(dataDir, "bans", name))
			if err != nil {
				continue
			}
			var ban BanMsg
			if err := json.Unmarshal(data, &ban); err != nil {
				continue
			}
			_ = m.PublishBan(ctx, topic, ban)
		case strings.HasPrefix(name, "revoke_"):
			data, err := os.ReadFile(filepath.Join(dataDir, "bans", name))
			if err != nil {
				continue
			}
			var rev RevokeMsg
			if err := json.Unmarshal(data, &rev); err != nil {
				continue
			}
			_ = m.PublishRevoke(ctx, topic, rev)
		case strings.HasPrefix(name, "unban_"):
			data, err := os.ReadFile(filepath.Join(dataDir, "bans", name))
			if err != nil {
				continue
			}
			var u UnbanMsg
			if err := json.Unmarshal(data, &u); err != nil {
				continue
			}
			_ = m.PublishUnban(ctx, topic, u)
		}
	}
}

func (m *Moderator) handleEnvelope(data []byte) {
	if m.isDisabled() {
		return
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case "ban":
		if env.Ban != nil {
			m.handleBanMsg(env.Ban)
		}
	case "revoke":
		if env.Revoke != nil {
			m.handleRevokeMsg(env.Revoke)
		}
	case "unban":
		if env.Unban != nil {
			m.handleUnbanMsg(env.Unban)
		}
	}
}

func (m *Moderator) handleBanMsg(ban *BanMsg) {
	m.applyBan(ban)
}

// applyBan validates ban and, if it passes, applies it locally (m.bans)
// and persists to disk, reporting whether it was applied. Shared by
// handleBanMsg (network-received) and IssueBan (locally-originated) so a
// locally-issued ban is held to exactly the rules a receiving peer would
// enforce.
//
// ban.Cert.ModeratorKey == "" marks a founder-issued ban, verified
// directly against m.founder via verifyFounderBan — verifyCert always
// rejects a cert keyed to the founder, so this is a separate path.
func (m *Moderator) applyBan(ban *BanMsg) bool {
	if ban.BannedPeer == "" || ban.Reason == "" || ban.Sig == "" {
		return false
	}
	if len(ban.Reason) > 256 {
		return false
	}

	now := time.Now()
	if !ban.Permanent {
		// minBanDuration/maxBanDays, banın İMZALANDIĞI ANDA seçilen sürenin
		// makul olduğunu doğrular — bu yüzden referans noktası ban.IssuedAt
		// olmalı, "now" (uygulanma anı) DEĞİL. handleSyncStream/PublishSavedBans
		// gibi yollarda imzalama ile uygulama arasında saniyeler-dakikalar
		// geçebilir; referans "now" olsaydı, --duration tam olarak
		// minBanDuration (andmod'un izin verdiği en kısa süre) ile verilen
		// HER ban, aradan geçen süre yüzünden her zaman reddedilirdi. Alt
		// sınır KAPSAYICIDIR (Before, After değil): andmod'un CLI doğrulaması
		// "d < time.Minute" ile d==minBanDuration'ı GEÇERLİ kabul ediyor
		// (bkz. cmd/andmod/main.go), bu yüzden burası da tam olarak
		// minBanDuration'a eşit bir süreyi reddetmemeli.
		if ban.ExpiresAt.Before(ban.IssuedAt.Add(minBanDuration)) {
			return false
		}
		if ban.ExpiresAt.After(ban.IssuedAt.Add(maxBanDays * 24 * time.Hour)) {
			return false
		}
	}

	pid, err := peer.Decode(ban.BannedPeer)
	if err != nil {
		return false
	}

	if pid == m.founderID {
		return false
	}

	// bkz. Moderator.unbannedSince'in doc yorumu: bu peer daha önce
	// GERÇEKTEN unban edildiyse, o unban'dan ÖNCE (ya da onunla aynı anda)
	// imzalanmış bir ban asla yeniden uygulanmamalı — aksi halde banı VEREN
	// moderatörün eski, hâlâ diskte duran ban_<peer>.json'ı (ör. kendi
	// düğümü yeniden başladığında PublishSavedBans ile) banı diriltirdi.
	m.mu.RLock()
	since, hasWatermark := m.unbannedSince[pid]
	m.mu.RUnlock()
	if hasWatermark && !ban.IssuedAt.After(since) {
		return false
	}

	if m.alreadySeen(ban.Sig) {
		return false
	}

	var rateLimitKey string
	if ban.Cert.ModeratorKey == "" {
		if !m.verifyFounderBan(ban) {
			return false
		}
		rateLimitKey = hex.EncodeToString(m.founder)
	} else {
		if !m.verifyCert(ban.Cert) {
			return false
		}
		if !m.verifyBan(ban) {
			return false
		}
		rateLimitKey = ban.Cert.ModeratorKey
	}

	if !m.allowBan(rateLimitKey) {
		return false
	}

	m.mu.Lock()
	m.bans[pid] = &banRecord{ExpiresAt: ban.ExpiresAt, Permanent: ban.Permanent, ModeratorKey: rateLimitKey, Reason: ban.Reason}
	m.mu.Unlock()
	m.saveBans()
	m.recordHistory(HistoryEntry{
		Action: HistoryBan, PeerID: ban.BannedPeer, Reason: ban.Reason,
		ModeratorKey: rateLimitKey, At: now, ExpiresAt: ban.ExpiresAt, Permanent: ban.Permanent,
	})
	return true
}

// IssueBan applies a ban originated by THIS node locally via applyBan
// (so it takes effect immediately, without waiting for GossipSub's
// self-echo, which isn't guaranteed), then publishes it to the network.
// ban should come from SignFounderBan or SignModeratorBan.
func (m *Moderator) IssueBan(ctx context.Context, topic *network.Topic, ban BanMsg) error {
	if !m.applyBan(&ban) {
		return fmt.Errorf("moderation: ban reddedildi (imza, sertifika, hız sınırı ya da süre parametreleri geçersiz)")
	}
	if err := m.PublishBan(ctx, topic, ban); err != nil {
		return fmt.Errorf("moderation: ban yayınlanamadı: %w", err)
	}
	return nil
}

func (m *Moderator) handleRevokeMsg(rev *RevokeMsg) {
	m.applyRevoke(rev)
}

// applyRevoke validates rev and, if it passes, applies it locally
// (m.revoked) and persists to disk, reporting whether it was applied —
// mirrors applyBan/applyUnban's shared-validation pattern so a
// locally-issued revoke (IssueRevoke) is held to exactly the rules a
// receiving peer would enforce.
func (m *Moderator) applyRevoke(rev *RevokeMsg) bool {
	if rev.ModeratorKey == "" || rev.Sig == "" {
		return false
	}
	if m.alreadySeenRevoke(rev.Sig) {
		return false
	}
	sig, err := hex.DecodeString(rev.Sig)
	if err != nil {
		return false
	}
	payload := fmt.Sprintf("revoke|%s|%d", rev.ModeratorKey, rev.IssuedAt.Unix())
	if !ed25519.Verify(m.founder, []byte(payload), sig) {
		return false
	}
	m.mu.Lock()
	m.revoked[rev.ModeratorKey] = rev.IssuedAt
	m.mu.Unlock()
	m.saveRevocations()
	return true
}

// IssueRevoke applies a revoke originated by THIS node locally first (see
// IssueBan), publishes it, then marks the matching local GrantedMods
// entry (if any) as revoked.
func (m *Moderator) IssueRevoke(ctx context.Context, topic *network.Topic, rev RevokeMsg) error {
	if m == nil || topic == nil {
		return fmt.Errorf("moderation: moderasyon sistemi kullanılamıyor")
	}
	if !m.applyRevoke(&rev) {
		return fmt.Errorf("moderation: iptal reddedildi (imza geçersiz ya da tekrar)")
	}
	if err := m.PublishRevoke(ctx, topic, rev); err != nil {
		return fmt.Errorf("moderation: iptal yayınlanamadı: %w", err)
	}
	m.markGrantRevoked(rev.ModeratorKey)
	return nil
}

// handleUnbanMsg processes an UnbanMsg — a moderator (still holding a
// currently-valid, non-revoked cert) undoing a previously-issued ban. Any
// moderator may lift any ban, mirroring the existing "any moderator may ban
// any peer" model (bans/unbans aren't scoped to whoever issued them).
func (m *Moderator) handleUnbanMsg(u *UnbanMsg) {
	m.applyUnban(u)
}

// applyUnban validates u and, if it passes, lifts the ban locally
// (deletes from m.bans) and persists to disk, reporting whether a ban was
// actually lifted. Mirrors applyBan's shared handleUnbanMsg/IssueUnban
// entry points, including the founder-path (Cert.ModeratorKey == "")
// verified via verifyFounderUnban instead of a ModeratorCert.
func (m *Moderator) applyUnban(u *UnbanMsg) bool {
	if u.BannedPeer == "" || u.Sig == "" {
		return false
	}
	pid, err := peer.Decode(u.BannedPeer)
	if err != nil {
		return false
	}
	if m.alreadySeenUnban(u.Sig) {
		return false
	}

	var rateLimitKey string
	if u.Cert.ModeratorKey == "" {
		if !m.verifyFounderUnban(u) {
			return false
		}
		rateLimitKey = hex.EncodeToString(m.founder)
	} else {
		if !m.verifyCert(u.Cert) {
			return false
		}
		if !m.verifyUnban(u) {
			return false
		}
		rateLimitKey = u.Cert.ModeratorKey
	}

	// Ban/unban eylemleri aynı saatlik moderatör kotasını (banRateLimit)
	// paylaşır — bir moderatörün toplam moderasyon-eylemi hacmini, yönünden
	// bağımsız olarak sınırlamak için.
	if !m.allowBan(rateLimitKey) {
		return false
	}

	m.mu.Lock()
	rec, wasBanned := m.bans[pid]
	delete(m.bans, pid)
	// bkz. Moderator.unbannedSince'in doc yorumu — bu peer o an banlı
	// OLMASA bile (ör. ban zaten süresi dolmuştu ya da bu düğüm hiç
	// bilmiyordu) watermark'ı yükseltmek güvenlidir: bu, bundan SONRA
	// gelecek, bu unban'dan ÖNCE imzalanmış herhangi bir bayat banı
	// reddetmek için doğru referans noktasıdır. Yalnızca DAHA YENİ bir
	// watermark'ı geriye almamak için mevcutla karşılaştırılır.
	if existing, ok := m.unbannedSince[pid]; !ok || u.IssuedAt.After(existing) {
		m.unbannedSince[pid] = u.IssuedAt
	}
	m.mu.Unlock()
	m.saveUnbanWatermarks()
	if wasBanned {
		m.saveBans()
		reason := ""
		if rec != nil {
			reason = rec.Reason
		}
		m.recordHistory(HistoryEntry{
			Action: HistoryUnban, PeerID: u.BannedPeer, Reason: reason,
			ModeratorKey: rateLimitKey, At: time.Now(),
		})
	}
	return wasBanned
}

// IssueUnban applies an unban originated by THIS node locally first (see
// IssueBan), then publishes it.
func (m *Moderator) IssueUnban(ctx context.Context, topic *network.Topic, u UnbanMsg) error {
	if !m.applyUnban(&u) {
		return fmt.Errorf("moderation: ban kaldırma reddedildi (imza, sertifika ya da hız sınırı geçersiz, ya da hedef zaten banlı değil)")
	}
	if err := m.PublishUnban(ctx, topic, u); err != nil {
		return fmt.Errorf("moderation: ban kaldırma yayınlanamadı: %w", err)
	}
	return nil
}

// IssueBanFor builds and issues a ban for targetAuthorKey signed by
// signerPriv — either as founder (cert==nil) or as a moderator (cert!=nil).
// Shared by the TUI's profile-screen ban action and the admin plugin's
// moderation backend (bkz. cmd/and'in modAdapter'ı) so both sign/apply/
// publish identically instead of duplicating this orchestration twice.
func IssueBanFor(ctx context.Context, mod *Moderator, topic *network.Topic, signerPriv ed25519.PrivateKey,
	targetAuthorKey string, isFounder bool, cert *ModeratorCert, reason string, expiresAt time.Time, permanent bool) error {
	if mod == nil || topic == nil {
		return fmt.Errorf("moderation: moderasyon sistemi kullanılamıyor")
	}
	pid, err := AuthorKeyToPeerID(targetAuthorKey)
	if err != nil {
		return fmt.Errorf("moderation: geçersiz hedef: %w", err)
	}
	var ban BanMsg
	switch {
	case isFounder:
		ban = SignFounderBan(signerPriv, pid.String(), reason, expiresAt, permanent)
	case cert != nil:
		ban = SignModeratorBan(signerPriv, *cert, pid.String(), reason, expiresAt, permanent)
	default:
		return fmt.Errorf("moderation: moderatör yetkisi bulunamadı")
	}
	return mod.IssueBan(ctx, topic, ban)
}

// IssueUnbanFor lifts an active ban on targetAuthorKey, signed by
// signerPriv — either as founder (cert==nil) or as a moderator (cert!=nil).
// Uses the same authority as banning: any identity that could ban a target
// can also unban them. Shared by the TUI's profile-screen unban action and
// the admin plugin's moderation backend.
func IssueUnbanFor(ctx context.Context, mod *Moderator, topic *network.Topic, signerPriv ed25519.PrivateKey,
	targetAuthorKey string, isFounder bool, cert *ModeratorCert) error {
	if mod == nil || topic == nil {
		return fmt.Errorf("moderation: moderasyon sistemi kullanılamıyor")
	}
	pid, err := AuthorKeyToPeerID(targetAuthorKey)
	if err != nil {
		return fmt.Errorf("moderation: geçersiz hedef: %w", err)
	}
	var u UnbanMsg
	switch {
	case isFounder:
		u = SignFounderUnban(signerPriv, pid.String())
	case cert != nil:
		u = SignModeratorUnban(signerPriv, *cert, pid.String())
	default:
		return fmt.Errorf("moderation: moderatör yetkisi bulunamadı")
	}
	return mod.IssueUnban(ctx, topic, u)
}

// VerifyModeratorCert reports whether cert is a currently-valid moderator
// certificate signed by the founder — not expired, not exceeding
// maxCertDays, and not revoked. Exported so other packages (e.g. profile)
// can let a peer prove their moderator role without re-implementing the
// founder-signature/revocation checks.
func (m *Moderator) VerifyModeratorCert(cert ModeratorCert) bool {
	return m.verifyCert(cert)
}

// VerifyFounderSignature reports whether sig is a valid Ed25519 signature
// over payload made with the founder's own private key. Exported so other
// packages (forum, chatlog) can build their own founder-authorized message
// types (e.g. a moderator-issued post deletion) without duplicating access
// to the founder's public key.
func (m *Moderator) VerifyFounderSignature(payload, sig []byte) bool {
	if m.isDisabled() {
		return false
	}
	return ed25519.Verify(m.founder, payload, sig)
}

// ModeratorCertPayload deterministically encodes what a ModeratorCert's Sig
// must cover — shared by verifyCert, FindModCert and SignModeratorCert so
// signer and every verifier can never drift apart. issuedAt is included so
// the holder of a cert can't rewrite it to dodge a revoke.
func ModeratorCertPayload(targetKeyHex string, issuedAt, expiresAt time.Time, permanent bool) []byte {
	if permanent {
		return []byte(fmt.Sprintf("%s|%d|permanent", targetKeyHex, issuedAt.Unix()))
	}
	return []byte(fmt.Sprintf("%s|%d|%d", targetKeyHex, issuedAt.Unix(), expiresAt.Unix()))
}

// SignModeratorCert builds a ModeratorCert granting targetKeyHex moderator
// authority, signed by founderPriv — only verifies anywhere on the
// network if founderPriv genuinely is the founder's key.
func SignModeratorCert(founderPriv ed25519.PrivateKey, targetKeyHex string, expiresAt time.Time, permanent bool) ModeratorCert {
	issuedAt := time.Now().UTC()
	sig := ed25519.Sign(founderPriv, ModeratorCertPayload(targetKeyHex, issuedAt, expiresAt, permanent))
	return ModeratorCert{ModeratorKey: targetKeyHex, IssuedAt: issuedAt, ExpiresAt: expiresAt, Permanent: permanent, Sig: hex.EncodeToString(sig)}
}

// SignRevoke builds a RevokeMsg withdrawing targetKeyHex's moderator
// authority, signed by founderPriv.
func SignRevoke(founderPriv ed25519.PrivateKey, targetKeyHex string) RevokeMsg {
	issuedAt := time.Now().UTC()
	payload := fmt.Sprintf("revoke|%s|%d", targetKeyHex, issuedAt.Unix())
	sig := ed25519.Sign(founderPriv, []byte(payload))
	return RevokeMsg{ModeratorKey: targetKeyHex, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}
}

func (m *Moderator) verifyCert(cert ModeratorCert) bool {
	modKey, err := hex.DecodeString(cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}

	if strings.EqualFold(cert.ModeratorKey, hex.EncodeToString(m.founder)) {
		return false
	}

	if !cert.Permanent {
		now := time.Now()
		if now.After(cert.ExpiresAt) {
			return false
		}
		if cert.ExpiresAt.After(now.Add(maxCertDays * 24 * time.Hour)) {
			return false
		}
	}

	sig, err := hex.DecodeString(cert.Sig)
	if err != nil {
		return false
	}

	if !ed25519.Verify(m.founder, ModeratorCertPayload(cert.ModeratorKey, cert.IssuedAt, cert.ExpiresAt, cert.Permanent), sig) {
		return false
	}

	m.mu.RLock()
	revokedAt, isRevoked := m.revoked[cert.ModeratorKey]
	m.mu.RUnlock()
	// IssuedAt <= revokedAt: bu sertifika iptalden ÖNCE (ya da aynı anda)
	// verilmiş, dolayısıyla iptalin kapsamında — reddedilir. Permanent'tan
	// bağımsız olarak aynı kural geçerli: kalıcı bir sertifika bile,
	// iptalden SONRA yeniden verilmişse (IssuedAt > revokedAt) tekrar
	// geçerli sayılır — bkz. ModeratorCert.IssuedAt'in doc yorumu.
	if isRevoked && !cert.IssuedAt.After(revokedAt) {
		return false
	}

	return true
}

// ModeratorBanPayload deterministically encodes what a moderator-issued
// BanMsg's Sig must cover — shared by verifyBan and SignModeratorBan (the
// TUI and andmod both call the latter) so signer and verifier can never
// drift apart.
func ModeratorBanPayload(bannedPeer, reason string, expiresAt time.Time, permanent bool, issuedAt time.Time) []byte {
	if permanent {
		return []byte(fmt.Sprintf("%s|%s|permanent|%d", bannedPeer, reason, issuedAt.Unix()))
	}
	return []byte(fmt.Sprintf("%s|%s|%d|%d", bannedPeer, reason, expiresAt.Unix(), issuedAt.Unix()))
}

// FounderBanPayload deterministically encodes what a founder-issued
// BanMsg's Sig must cover — shared by verifyFounderBan and SignFounderBan.
// The "founder|" prefix keeps this byte-distinct from
// ModeratorBanPayload's format (same BannedPeer/Reason/ExpiresAt could
// otherwise coincide), so a signature valid for one can never be replayed
// as the other.
func FounderBanPayload(bannedPeer, reason string, expiresAt time.Time, permanent bool, issuedAt time.Time) []byte {
	if permanent {
		return []byte(fmt.Sprintf("founder|%s|%s|permanent|%d", bannedPeer, reason, issuedAt.Unix()))
	}
	return []byte(fmt.Sprintf("founder|%s|%s|%d|%d", bannedPeer, reason, expiresAt.Unix(), issuedAt.Unix()))
}

func (m *Moderator) verifyBan(ban *BanMsg) bool {
	modKey, err := hex.DecodeString(ban.Cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(ban.Sig)
	if err != nil {
		return false
	}
	payload := ModeratorBanPayload(ban.BannedPeer, ban.Reason, ban.ExpiresAt, ban.Permanent, ban.IssuedAt)
	return ed25519.Verify(ed25519.PublicKey(modKey), payload, sig)
}

// verifyFounderBan checks a founder-issued ban's (Cert.ModeratorKey=="")
// signature directly against m.founder — see applyBan's doc comment for
// why this bypasses verifyCert entirely.
func (m *Moderator) verifyFounderBan(ban *BanMsg) bool {
	sig, err := hex.DecodeString(ban.Sig)
	if err != nil {
		return false
	}
	payload := FounderBanPayload(ban.BannedPeer, ban.Reason, ban.ExpiresAt, ban.Permanent, ban.IssuedAt)
	return ed25519.Verify(m.founder, payload, sig)
}

// SignModeratorBan builds a BanMsg signed by a moderator's own key
// (modPriv) using their granted cert — the shared reference
// implementation for both andmod's CLI and the TUI's in-app ban action.
// cert must belong to the same key as modPriv (cert.ModeratorKey ==
// hex(modPriv's public key)); callers are responsible for that match
// (see FindModCert, which only ever returns certs for the key you ask
// it for).
func SignModeratorBan(modPriv ed25519.PrivateKey, cert ModeratorCert, bannedPeer, reason string, expiresAt time.Time, permanent bool) BanMsg {
	issuedAt := time.Now().UTC()
	sig := ed25519.Sign(modPriv, ModeratorBanPayload(bannedPeer, reason, expiresAt, permanent, issuedAt))
	return BanMsg{
		BannedPeer: bannedPeer,
		Reason:     reason,
		ExpiresAt:  expiresAt,
		Permanent:  permanent,
		Cert:       cert,
		IssuedAt:   issuedAt,
		Sig:        hex.EncodeToString(sig),
	}
}

// SignFounderBan builds a BanMsg signed directly by the founder's own key
// (founderPriv), with no ModeratorCert attached — see applyBan's doc
// comment for why the founder needs this separate path.
func SignFounderBan(founderPriv ed25519.PrivateKey, bannedPeer, reason string, expiresAt time.Time, permanent bool) BanMsg {
	issuedAt := time.Now().UTC()
	sig := ed25519.Sign(founderPriv, FounderBanPayload(bannedPeer, reason, expiresAt, permanent, issuedAt))
	return BanMsg{
		BannedPeer: bannedPeer,
		Reason:     reason,
		ExpiresAt:  expiresAt,
		IssuedAt:   issuedAt,
		Permanent:  permanent,
		Sig:        hex.EncodeToString(sig),
	}
}

// ModeratorUnbanPayload deterministically encodes what a moderator-issued
// UnbanMsg's Sig must cover — shared by verifyUnban and SignModeratorUnban.
func ModeratorUnbanPayload(bannedPeer string, issuedAt time.Time) []byte {
	return []byte(fmt.Sprintf("unban|%s|%d", bannedPeer, issuedAt.Unix()))
}

// FounderUnbanPayload deterministically encodes what a founder-issued
// UnbanMsg's Sig must cover — shared by verifyFounderUnban and
// SignFounderUnban. The "founder-unban|" prefix keeps this byte-distinct
// from ModeratorUnbanPayload's format (and from every other payload kind
// in this file), so a signature valid for one can never be replayed as
// another.
func FounderUnbanPayload(bannedPeer string, issuedAt time.Time) []byte {
	return []byte(fmt.Sprintf("founder-unban|%s|%d", bannedPeer, issuedAt.Unix()))
}

// verifyUnban checks u's signature against its claimed moderator cert's
// key. The "unban|" prefix keeps this payload's byte layout distinct from
// verifyBan's/handleRevokeMsg's, so a signature valid for one message type
// can never be replayed as another.
func (m *Moderator) verifyUnban(u *UnbanMsg) bool {
	modKey, err := hex.DecodeString(u.Cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(u.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(modKey), ModeratorUnbanPayload(u.BannedPeer, u.IssuedAt), sig)
}

// verifyFounderUnban checks a founder-issued unban's (Cert.ModeratorKey==
// "") signature directly against m.founder — mirrors verifyFounderBan.
func (m *Moderator) verifyFounderUnban(u *UnbanMsg) bool {
	sig, err := hex.DecodeString(u.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(m.founder, FounderUnbanPayload(u.BannedPeer, u.IssuedAt), sig)
}

// SignModeratorUnban builds an UnbanMsg signed by a moderator's own key
// (modPriv) using their granted cert — the shared reference implementation
// for both andmod's CLI and the TUI's in-app unban action.
func SignModeratorUnban(modPriv ed25519.PrivateKey, cert ModeratorCert, bannedPeer string) UnbanMsg {
	issuedAt := time.Now().UTC()
	sig := ed25519.Sign(modPriv, ModeratorUnbanPayload(bannedPeer, issuedAt))
	return UnbanMsg{BannedPeer: bannedPeer, Cert: cert, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}
}

// SignFounderUnban builds an UnbanMsg signed directly by the founder's own
// key (founderPriv), with no ModeratorCert attached.
func SignFounderUnban(founderPriv ed25519.PrivateKey, bannedPeer string) UnbanMsg {
	issuedAt := time.Now().UTC()
	sig := ed25519.Sign(founderPriv, FounderUnbanPayload(bannedPeer, issuedAt))
	return UnbanMsg{BannedPeer: bannedPeer, IssuedAt: issuedAt, Sig: hex.EncodeToString(sig)}
}

func (m *Moderator) alreadySeen(sig string) bool {
	return m.seenBans.seen(sig)
}

func (m *Moderator) alreadySeenRevoke(sig string) bool {
	return m.seenRevokes.seen(sig)
}

func (m *Moderator) alreadySeenUnban(sig string) bool {
	return m.seenUnbans.seen(sig)
}

func (m *Moderator) allowBan(modKey string) bool {
	m.rateMu.Lock()
	defer m.rateMu.Unlock()

	cutoff := time.Now().Add(-banRateWindow)
	times := m.rates[modKey]

	fresh := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}

	if len(fresh) >= banRateLimit {
		return false
	}
	m.rates[modKey] = append(fresh, time.Now())
	m.ratesDirty = true
	return true
}

type persistedRate struct {
	ModeratorKey string      `json:"moderator_key"`
	Times        []time.Time `json:"times"`
}

func (m *Moderator) saveRates() {
	if m.dataDir == "" {
		return
	}
	cutoff := time.Now().Add(-banRateWindow)
	m.rateMu.Lock()
	var list []persistedRate
	for k, times := range m.rates {
		var fresh []time.Time
		for _, t := range times {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) > 0 {
			list = append(list, persistedRate{ModeratorKey: k, Times: fresh})
		}
	}
	m.rateMu.Unlock()
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(filepath.Join(m.dataDir, "mod_rates.json"), data, 0o600)
}

func (m *Moderator) loadRates() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, "mod_rates.json"))
	if err != nil {
		return
	}
	var list []persistedRate
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	cutoff := time.Now().Add(-banRateWindow)
	m.rateMu.Lock()
	for _, r := range list {
		var fresh []time.Time
		for _, t := range r.Times {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) > 0 {
			m.rates[r.ModeratorKey] = fresh
		}
	}
	m.rateMu.Unlock()
}

func (m *Moderator) IsBanned(pid peer.ID) bool {
	if m.isDisabled() {
		return false
	}
	m.mu.RLock()
	rec, ok := m.bans[pid]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return rec.Permanent || time.Now().Before(rec.ExpiresAt)
}

// AuthorKeyToPeerID, forum/chat/DM AuthorKey'i olarak kullanılan hex-encoded
// ed25519 public key'i, aynı anahtarın libp2p ağ kimliği (peer.ID)
// karşılığına çevirir.
//
// AND'de her düğüm TEK bir ed25519 anahtar çifti kullanır: bu anahtar hem
// libp2p Peer ID'sini (bkz. internal/network.libp2pPrivateKey) hem de
// forum/chat/DM imzalarındaki AuthorKey'i (bkz. internal/crypto.Identity)
// üretir. Yani bir AuthorKey'den, o yazarın gerçek ağ kimliğine (peer.ID)
// geri dönmek mümkündür — bu da bir moderatör banının (peer.ID bazlı) aynı
// kişinin forum/chat içeriğine karşı da uygulanabilmesini sağlar (bkz.
// IsAuthorBanned).
func AuthorKeyToPeerID(authorKeyHex string) (peer.ID, error) {
	raw, err := hex.DecodeString(authorKeyHex)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("moderation: geçersiz author key hex")
	}
	lp2pPub, err := lp2pcrypto.UnmarshalEd25519PublicKey(raw)
	if err != nil {
		return "", fmt.Errorf("moderation: author key libp2p anahtarına çevrilemedi: %w", err)
	}
	return peer.IDFromPublicKey(lp2pPub)
}

// IsAuthorBanned, forum/chat'te AuthorKey olarak kullanılan hex-encoded
// ed25519 public key'in, ağ seviyesinde banlı bir peer'a ait olup olmadığını
// bildirir — bkz. AuthorKeyToPeerID. forum.Forum ve chatlog.Store bunu
// SetModerator ile bağlandıklarında handleMessage içinde çağırır; böylece bir
// moderatörün verdiği peer banı yalnızca yeni bağlantıları değil, o kişinin
// forum konularını/yanıtlarını ve sohbet mesajlarını da engeller.
func (m *Moderator) IsAuthorBanned(authorKeyHex string) bool {
	pid, err := AuthorKeyToPeerID(authorKeyHex)
	if err != nil {
		return false
	}
	return m.IsBanned(pid)
}

// BanInfo reports the details of one active ban — used to show ban status
// (kalan süre/ömür boyu/sebep) on a profile screen, as opposed to
// IsBanned/IsAuthorBanned's plain yes/no.
type BanInfo struct {
	ExpiresAt    time.Time // Permanent ise sıfır değer, kullanılmaz
	Permanent    bool
	ModeratorKey string
	Reason       string
}

// PeerBanInfo reports pid's current ban, if any. ok is false if pid isn't
// banned (including an expired, non-permanent ban that just hasn't been
// swept by cleanupExpiredBans yet — see IsBanned's identical expiry check).
func (m *Moderator) PeerBanInfo(pid peer.ID) (info BanInfo, ok bool) {
	if m.isDisabled() {
		return BanInfo{}, false
	}
	m.mu.RLock()
	rec, exists := m.bans[pid]
	m.mu.RUnlock()
	if !exists || (!rec.Permanent && !time.Now().Before(rec.ExpiresAt)) {
		return BanInfo{}, false
	}
	return BanInfo{ExpiresAt: rec.ExpiresAt, Permanent: rec.Permanent, ModeratorKey: rec.ModeratorKey, Reason: rec.Reason}, true
}

// AuthorBanInfo is PeerBanInfo for an AuthorKey — see AuthorKeyToPeerID's
// doc comment for why forum/chat AuthorKeys and peer.IDs are the same
// underlying identity in AND.
func (m *Moderator) AuthorBanInfo(authorKeyHex string) (info BanInfo, ok bool) {
	pid, err := AuthorKeyToPeerID(authorKeyHex)
	if err != nil {
		return BanInfo{}, false
	}
	return m.PeerBanInfo(pid)
}

// PeerIDToAuthorKey is the inverse of AuthorKeyToPeerID: it recovers the
// hex-encoded ed25519 public key embedded in an AND peer.ID (libp2p embeds
// the raw key for keys this small, rather than hashing it).
func PeerIDToAuthorKey(pid peer.ID) (string, error) {
	pub, err := pid.ExtractPublicKey()
	if err != nil {
		return "", fmt.Errorf("moderation: peer id'den public key çıkarılamadı: %w", err)
	}
	raw, err := pub.Raw()
	if err != nil {
		return "", fmt.Errorf("moderation: public key ham baytlara çevrilemedi: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// BanEntry is one row of AllBans' snapshot — a currently-active ban with
// enough detail (target, remaining time, issuer, reason) to render the Ban
// Listesi screen without a second lookup.
type BanEntry struct {
	PeerID       string
	AuthorKey    string // PeerIDToAuthorKey başarısız olursa boş
	ExpiresAt    time.Time
	Permanent    bool
	ModeratorKey string
	Reason       string
}

// AllBans returns a snapshot of every currently-active ban (permanent, or
// temporary and not yet expired), sorted by PeerID for a stable render
// order across calls. Powers the Ban Listesi screen.
func (m *Moderator) AllBans() []BanEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	out := make([]BanEntry, 0, len(m.bans))
	for pid, rec := range m.bans {
		if !rec.Permanent && !rec.ExpiresAt.After(now) {
			continue
		}
		authorKey, _ := PeerIDToAuthorKey(pid)
		out = append(out, BanEntry{
			PeerID:       pid.String(),
			AuthorKey:    authorKey,
			ExpiresAt:    rec.ExpiresAt,
			Permanent:    rec.Permanent,
			ModeratorKey: rec.ModeratorKey,
			Reason:       rec.Reason,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PeerID < out[j].PeerID })
	return out
}

// HistoryAction identifies which kind of moderation event a HistoryEntry
// records.
type HistoryAction string

const (
	HistoryBan   HistoryAction = "ban"
	HistoryUnban HistoryAction = "unban"
)

// HistoryEntry is one row of this node's local moderation audit log —
// unlike m.bans (current state only), history never removes entries.
type HistoryEntry struct {
	Action       HistoryAction `json:"action"`
	PeerID       string        `json:"peer_id"`
	Reason       string        `json:"reason,omitempty"`
	ModeratorKey string        `json:"moderator_key"`
	At           time.Time     `json:"at"`
	ExpiresAt    time.Time     `json:"expires_at,omitempty"`
	Permanent    bool          `json:"permanent,omitempty"`
}

// recordHistory prepends e to the in-memory history (newest first),
// trims to maxHistoryEntries, and persists.
func (m *Moderator) recordHistory(e HistoryEntry) {
	m.historyMu.Lock()
	m.history = append([]HistoryEntry{e}, m.history...)
	if len(m.history) > maxHistoryEntries {
		m.history = m.history[:maxHistoryEntries]
	}
	m.historyMu.Unlock()
	m.saveHistory()
}

// History returns a copy of the local moderation audit log, newest first.
func (m *Moderator) History() []HistoryEntry {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	out := make([]HistoryEntry, len(m.history))
	copy(out, m.history)
	return out
}

func (m *Moderator) historyPath() string {
	return filepath.Join(m.dataDir, "mod_history.json")
}

func (m *Moderator) saveHistory() {
	if m.dataDir == "" {
		return
	}
	m.historyMu.Lock()
	data, err := json.MarshalIndent(m.history, "", "  ")
	m.historyMu.Unlock()
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(m.historyPath(), data, 0o600)
}

func (m *Moderator) loadHistory() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(m.historyPath())
	if err != nil {
		return
	}
	var h []HistoryEntry
	if err := json.Unmarshal(data, &h); err != nil {
		return
	}
	m.historyMu.Lock()
	m.history = h
	m.historyMu.Unlock()
}

// GrantedMod is one row of this node's local record of moderator
// certificates IT (as founder) has issued via IssueGrant.
type GrantedMod struct {
	Key       string    `json:"key"`
	GrantedAt time.Time `json:"granted_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Permanent bool      `json:"permanent,omitempty"`
	Revoked   bool      `json:"revoked,omitempty"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
}

// GrantedMods returns a copy of this node's local grant record, newest
// first.
func (m *Moderator) GrantedMods() []GrantedMod {
	m.grantsMu.Lock()
	defer m.grantsMu.Unlock()
	out := make([]GrantedMod, len(m.grants))
	copy(out, m.grants)
	return out
}

// IssueGrant builds and signs a ModeratorCert for targetKeyHex using
// founderPriv, records it in the local GrantedMods log, and writes a copy
// to dataDir/bans/mod_<ilk8>.json (the same on-disk convention andmod
// grant already uses). The founder is responsible for delivering the
// returned cert (or the written file) to the target out-of-band (e.g. via
// Özel Chat) — certs are bearer tokens, never broadcast over the network.
//
// The freshly-signed cert is verified against m.founder via verifyCert
// before anything is recorded/written — mirroring the self-check every
// other Issue* method performs, so a wrong signing key fails loudly here
// instead of silently reporting success for a cert nobody else accepts.
func (m *Moderator) IssueGrant(founderPriv ed25519.PrivateKey, targetKeyHex string, expiresAt time.Time, permanent bool) (cert ModeratorCert, certPath string, err error) {
	raw, err := hex.DecodeString(targetKeyHex)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return ModeratorCert{}, "", fmt.Errorf("moderation: geçersiz hedef public key (32 bayt / 64 hex karakter olmalı)")
	}
	if !permanent && !expiresAt.After(time.Now()) {
		return ModeratorCert{}, "", fmt.Errorf("moderation: geçerlilik süresi geçmişte olamaz")
	}

	cert = SignModeratorCert(founderPriv, targetKeyHex, expiresAt, permanent)
	if !m.verifyCert(cert) {
		return ModeratorCert{}, "", fmt.Errorf("moderation: üretilen sertifika doğrulanamadı (bu kimlik gerçek kurucu olmayabilir ya da süre sınırı aşılmış olabilir)")
	}

	if m.dataDir != "" {
		outDir := filepath.Join(m.dataDir, "bans")
		if err := os.MkdirAll(outDir, 0o700); err == nil {
			shortKey := targetKeyHex
			if len(shortKey) > 8 {
				shortKey = shortKey[:8]
			}
			outPath := filepath.Join(outDir, "mod_"+shortKey+".json")
			if data, merr := json.MarshalIndent(cert, "", "  "); merr == nil {
				if dosya.AtomikYaz(outPath, data, 0o600) == nil {
					certPath = outPath
				}
			}
		}
	}

	m.grantsMu.Lock()
	m.grants = append([]GrantedMod{{
		Key: targetKeyHex, GrantedAt: time.Now(), ExpiresAt: expiresAt, Permanent: permanent,
	}}, m.grants...)
	m.grantsMu.Unlock()
	m.saveGrants()

	return cert, certPath, nil
}

// markGrantRevoked flags targetKeyHex's most recent non-revoked GrantedMods
// entry (if any) as revoked — called by IssueRevoke after a revoke is
// successfully applied/published, so the Moderatör Yönetimi screen reflects
// it immediately without waiting for a reload.
func (m *Moderator) markGrantRevoked(targetKeyHex string) {
	m.grantsMu.Lock()
	for i := range m.grants {
		if strings.EqualFold(m.grants[i].Key, targetKeyHex) && !m.grants[i].Revoked {
			m.grants[i].Revoked = true
			m.grants[i].RevokedAt = time.Now()
		}
	}
	m.grantsMu.Unlock()
	m.saveGrants()
}

func (m *Moderator) grantsPath() string {
	return filepath.Join(m.dataDir, "granted_mods.json")
}

func (m *Moderator) saveGrants() {
	if m.dataDir == "" {
		return
	}
	m.grantsMu.Lock()
	data, err := json.MarshalIndent(m.grants, "", "  ")
	m.grantsMu.Unlock()
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(m.grantsPath(), data, 0o600)
}

func (m *Moderator) loadGrants() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(m.grantsPath())
	if err != nil {
		return
	}
	var g []GrantedMod
	if err := json.Unmarshal(data, &g); err != nil {
		return
	}
	m.grantsMu.Lock()
	m.grants = g
	m.grantsMu.Unlock()
}

func (m *Moderator) saveBans() {
	if m.dataDir == "" {
		return
	}
	m.mu.RLock()
	now := time.Now()
	var active []PersistedBan
	for pid, rec := range m.bans {
		if rec.Permanent || rec.ExpiresAt.After(now) {
			active = append(active, PersistedBan{
				PeerID:       pid.String(),
				ExpiresAt:    rec.ExpiresAt,
				Permanent:    rec.Permanent,
				ModeratorKey: rec.ModeratorKey,
				Reason:       rec.Reason,
			})
		}
	}
	m.mu.RUnlock()

	data, err := json.Marshal(active)
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(filepath.Join(m.dataDir, "active_bans.json"), data, 0o600)
}

func (m *Moderator) loadBans() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, "active_bans.json"))
	if err != nil {
		return
	}
	var stored []PersistedBan
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	now := time.Now()
	m.mu.Lock()
	for _, b := range stored {
		if !b.Permanent && !b.ExpiresAt.After(now) {
			continue
		}
		if b.ModeratorKey != "" {
			if _, revoked := m.revoked[b.ModeratorKey]; revoked {
				continue
			}
		}
		pid, err := peer.Decode(b.PeerID)
		if err == nil {
			m.bans[pid] = &banRecord{ExpiresAt: b.ExpiresAt, Permanent: b.Permanent, ModeratorKey: b.ModeratorKey, Reason: b.Reason}
		}
	}
	m.mu.Unlock()
}

type persistedRevocation struct {
	ModeratorKey string    `json:"moderator_key"`
	IssuedAt     time.Time `json:"issued_at"`
}

func (m *Moderator) saveRevocations() {
	if m.dataDir == "" {
		return
	}
	m.mu.RLock()
	var list []persistedRevocation
	for k, t := range m.revoked {
		list = append(list, persistedRevocation{ModeratorKey: k, IssuedAt: t})
	}
	m.mu.RUnlock()

	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(filepath.Join(m.dataDir, "active_revocations.json"), data, 0o600)
}

func (m *Moderator) loadRevocations() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, "active_revocations.json"))
	if err != nil {
		return
	}
	var list []persistedRevocation
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	m.mu.Lock()
	for _, r := range list {
		m.revoked[r.ModeratorKey] = r.IssuedAt
	}
	m.mu.Unlock()
}

type persistedUnbanWatermark struct {
	PeerID   string    `json:"peer_id"`
	IssuedAt time.Time `json:"issued_at"`
}

// saveUnbanWatermarks/loadUnbanWatermarks persist Moderator.unbannedSince
// across restarts — bkz. oradaki doc yorumu: bu, in-memory alreadySeen
// önbelleğinin (yeniden başlamada sıfırlanan) tek başına ÖNLEYEMEDİĞİ, eski
// bir ban dosyasının restart sonrası yeniden yayınlanıp banı diriltmesi
// senaryosuna karşı KALICI korumadır.
func (m *Moderator) saveUnbanWatermarks() {
	if m.dataDir == "" {
		return
	}
	m.mu.RLock()
	list := make([]persistedUnbanWatermark, 0, len(m.unbannedSince))
	for pid, t := range m.unbannedSince {
		list = append(list, persistedUnbanWatermark{PeerID: pid.String(), IssuedAt: t})
	}
	m.mu.RUnlock()

	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = dosya.AtomikYaz(filepath.Join(m.dataDir, "unban_watermarks.json"), data, 0o600)
}

func (m *Moderator) loadUnbanWatermarks() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, "unban_watermarks.json"))
	if err != nil {
		return
	}
	var list []persistedUnbanWatermark
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	m.mu.Lock()
	for _, w := range list {
		pid, err := peer.Decode(w.PeerID)
		if err != nil {
			continue
		}
		if existing, ok := m.unbannedSince[pid]; !ok || w.IssuedAt.After(existing) {
			m.unbannedSince[pid] = w.IssuedAt
		}
	}
	m.mu.Unlock()
}

func (m *Moderator) InterceptPeerDial(pid peer.ID) bool {
	return !m.IsBanned(pid)
}

func (m *Moderator) InterceptAddrDial(pid peer.ID, _ ma.Multiaddr) bool {
	return !m.IsBanned(pid)
}

func (m *Moderator) InterceptAccept(_ lp2pnet.ConnMultiaddrs) bool {
	return true
}

func (m *Moderator) InterceptSecured(_ lp2pnet.Direction, pid peer.ID, _ lp2pnet.ConnMultiaddrs) bool {
	return !m.IsBanned(pid)
}

func (m *Moderator) InterceptUpgraded(_ lp2pnet.Conn) (bool, control.DisconnectReason) {
	return true, 0
}

// FindOwnCert is a convenience wrapper around FindModCert using this
// Moderator's own dataDir, so callers that already hold a *Moderator (e.g.
// profile.Store) don't need to separately thread a dataDir through just to
// look up a cert for a given pubkey.
func (m *Moderator) FindOwnCert(pubHex string) *ModeratorCert {
	return m.findCertCached(m.dataDir, pubHex)
}

type certCacheEntry struct {
	cert    *ModeratorCert
	expires time.Time
}

// certCacheTTL, findCertCached'in FindModCert sonucunu ne kadar süre
// önbellekte tutacağıdır. enforceModeratorRole artık 5 uç noktayı
// (getAllPosts, postDeletePost, getModBans, postModUnban, getModHistory)
// koruyor ve her biri her HTTP isteğinde ResolveAuthority'yi çağırıyor —
// önbellek olmadan bu, her istekte bans/ dizininin tam bir diskten
// taranmasına (+ ed25519 doğrulamasına) mal olur. Kısa bir TTL, bir
// iptal/atamadan sonra en fazla bu süre kadar bayat bir sonuç
// döndürebilir — bunu her grant/revoke çağrı noktasında elle geçersiz
// kılmaya çalışmaktan (ve birini kaçırma riskinden) daha güvenli bir
// tercih.
const certCacheTTL = 2 * time.Second

// findCertCached, FindModCert(dataDir, pubHex)'in KISA süreli önbellekli
// halidir — bkz. certCacheTTL'in doc yorumu.
func (m *Moderator) findCertCached(dataDir, pubHex string) *ModeratorCert {
	key := dataDir + "|" + pubHex
	m.certCacheMu.Lock()
	if e, ok := m.certCache[key]; ok && time.Now().Before(e.expires) {
		m.certCacheMu.Unlock()
		return e.cert
	}
	m.certCacheMu.Unlock()

	cert := FindModCert(dataDir, pubHex)

	m.certCacheMu.Lock()
	if m.certCache == nil {
		m.certCache = make(map[string]certCacheEntry)
	}
	m.certCache[key] = certCacheEntry{cert: cert, expires: time.Now().Add(certCacheTTL)}
	m.certCacheMu.Unlock()
	return cert
}

// ResolveAuthority reports whether myKeyHex currently holds valid
// founder or moderator authority: founder status is checked first, and
// otherwise a moderator certificate is looked up on disk and verified
// (which also checks revocation — see VerifyModeratorCert). This is the
// ONE place this check should be made; cmd/and/main.go's
// resolveOwnModAuthority (plugin API delete/mod actions) and
// internal/tui/forum.go's resolveBanAuthority (TUI ban/unban flow) both
// call into this instead of independently re-deriving the same
// FindModCert+VerifyModeratorCert pairing, so a future change to
// revocation semantics can't be patched into one call site and missed in
// the other.
func ResolveAuthority(dataDir string, mod *Moderator, isFounder bool, myKeyHex string) (canAct bool, cert *ModeratorCert) {
	if isFounder {
		return true, nil
	}
	var c *ModeratorCert
	if mod != nil {
		c = mod.findCertCached(dataDir, myKeyHex)
	} else {
		c = FindModCert(dataDir, myKeyHex)
	}
	if c != nil && mod != nil && mod.VerifyModeratorCert(*c) {
		return true, c
	}
	return false, nil
}

func FindModCert(dataDir, myPubHex string) *ModeratorCert {
	dir := filepath.Join(dataDir, "bans")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	founderRaw, err := hex.DecodeString(FounderPubKeyHex)
	if err != nil || len(founderRaw) != ed25519.PublicKeySize {
		return nil
	}
	founderPub := ed25519.PublicKey(founderRaw)
	// bans/ dizinindeki girdi sayısına makul bir üst sınır — bu dizin
	// yazılabilir bir konumdaysa (ör. paylaşımlı bir ortam) sınırsız bir
	// tarama küçük bir DoS yüzeyi olurdu.
	const maxScanEntries = 10_000
	if len(entries) > maxScanEntries {
		entries = entries[:maxScanEntries]
	}
	var best *ModeratorCert
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "mod_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(data) > 4096 {
			continue
		}
		var cert ModeratorCert
		if err := json.Unmarshal(data, &cert); err != nil {
			continue
		}
		if !strings.EqualFold(cert.ModeratorKey, myPubHex) {
			continue
		}
		if !cert.Permanent && time.Now().After(cert.ExpiresAt) {
			continue
		}
		sig, err := hex.DecodeString(cert.Sig)
		if err != nil || len(sig) != ed25519.SignatureSize {
			continue
		}
		if !ed25519.Verify(founderPub, ModeratorCertPayload(cert.ModeratorKey, cert.IssuedAt, cert.ExpiresAt, cert.Permanent), sig) {
			continue
		}
		c := cert
		if best == nil || cert.Permanent || (!best.Permanent && cert.ExpiresAt.After(best.ExpiresAt)) {
			best = &c
		}
	}
	return best
}
