// Package moderation implements AND's serverless moderation model.
//
// Trust chain (short → long lived):
//
//	Founder key (founder.key dosyası)
//	  └─ signs ModeratorCert  (≤7 gün)
//	        └─ moderator signs BanMsg (≤30 gün)
//
// Güvenlik özellikleri:
//   - Kurucu key sıfırsa moderasyon tamamen devre dışı (dev bypass yok)
//   - Ban listesi diske yazılır, restart'ta yüklenir
//   - Replay koruması: aynı ban imzası bir kez işlenir
//   - Moderatör rate limit: saatte max 20 ban
//   - Süre sınırları: cert ≤7 gün, ban ≤30 gün
//   - Kurucu banlanamaz
//   - Cert iptal: kurucu moderatör yetkisini geri alabilir (RevokeMsg)
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
	"strings"
	"sync"
	"time"

	"and/internal/network"

	"github.com/libp2p/go-libp2p/core/control"
	lp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// ModerationTopic is the GossipSub topic used to propagate moderation messages.
const ModerationTopic = "and/moderation/1.0.0"

// FounderKeyFile is the file that holds the founder's public key hex.
const FounderKeyFile = "founder.key"

const (
	maxCertDays    = 7  // moderatör sertifikası en fazla 7 gün
	maxBanDays     = 30 // ban en fazla 30 gün
	banRateWindow  = time.Hour
	banRateLimit   = 20  // moderatör başına saatte max ban
	seenCacheSize  = 512 // tekrar oynatmaya karşı son N imzayı hatırla
)

// FounderPubKeyHex is loaded at startup via LoadFounderKey.
// All-zeros → moderation disabled (no bypass, no certs accepted).
var FounderPubKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"

// ── Mesaj Tipleri ─────────────────────────────────────────────────────────────

// ModeratorCert grants moderation rights.
// Payload (non-permanent): "<moderator_key>|<expires_unix>"
// Payload (permanent):     "<moderator_key>|permanent"
type ModeratorCert struct {
	ModeratorKey string    `json:"moderator_key"` // hex Ed25519 pubkey
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Permanent    bool      `json:"permanent,omitempty"` // true → süresiz
	Sig          string    `json:"sig"`                 // founder signature (hex)
}

// BanMsg instructs every node to refuse connections from BannedPeer.
// Moderator payload: "<banned_peer>|<reason>|<expires_unix>"
type BanMsg struct {
	BannedPeer string        `json:"banned_peer"`
	Reason     string        `json:"reason"`
	ExpiresAt  time.Time     `json:"expires_at"`
	Cert       ModeratorCert `json:"cert"`
	Sig        string        `json:"sig"` // moderator signature (hex)
}

// RevokeMsg lets the founder immediately revoke a moderator certificate.
// Founder payload: "revoke|<moderator_key>|<issued_at_unix>"
type RevokeMsg struct {
	ModeratorKey string    `json:"moderator_key"`
	IssuedAt     time.Time `json:"issued_at"` // cert'in ExpiresAt'i — spesifik cert'i hedefler
	Sig          string    `json:"sig"`       // founder signature (hex)
}

// TrustedAuthorCert grants a user permanent forum post approval.
// Only the founder can issue these.
// Payload (permanent): "trusted|<author_key>|permanent"
// Payload (timed):     "trusted|<author_key>|<expires_unix>"
type TrustedAuthorCert struct {
	AuthorKey string    `json:"author_key"`
	IssuedAt  time.Time `json:"issued_at"`
	Permanent bool      `json:"permanent,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Sig       string    `json:"sig"` // founder Ed25519 signature
}

// ApprovalMsg marks a specific forum post as permanently approved.
// Can be signed by the founder (Cert == nil) or a moderator (Cert != nil).
// Payload: "approve|<post_id>|<issued_at_unix>"
type ApprovalMsg struct {
	PostID   string         `json:"post_id"`
	IssuedAt time.Time      `json:"issued_at"`
	Cert     *ModeratorCert `json:"cert,omitempty"` // nil → kurucu imzaladı
	Sig      string         `json:"sig"`
}

// Envelope wraps all moderation message types on the topic.
type Envelope struct {
	Type    string             `json:"type"` // "ban" | "revoke" | "trusted" | "approve"
	Ban     *BanMsg            `json:"ban,omitempty"`
	Revoke  *RevokeMsg         `json:"revoke,omitempty"`
	Trusted *TrustedAuthorCert `json:"trusted,omitempty"`
	Approve *ApprovalMsg       `json:"approve,omitempty"`
}

// PersistedBan is the on-disk format of an active ban (active_bans.json).
type PersistedBan struct {
	PeerID       string    `json:"peer_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	ModeratorKey string    `json:"moderator_key,omitempty"` // iptal kontrolü için
}

// ── Moderator ─────────────────────────────────────────────────────────────────

// Moderator enforces bans and implements libp2p's ConnectionGater.
type Moderator struct {
	mu          sync.RWMutex
	bans        map[peer.ID]time.Time  // peer → ban expiry
	banMods     map[peer.ID]string     // peer → moderatör key (revoke filtresi)
	revoked     map[string]time.Time   // moderator_key → revoke zamanı
	trustedAuthors map[string]TrustedAuthorCert // author_key → cert
	founder     ed25519.PublicKey
	founderID   peer.ID // kurucu banlanamaz
	dataDir     string

	// Replay koruması: ban ve revoke için ayrı ring buffer.
	seenMu     sync.Mutex
	seen       []string // ban imzaları
	seenRevoke []string // revoke imzaları

	// Rate limiting: moderatör başına sliding window.
	rateMu sync.Mutex
	rates  map[string][]time.Time // moderator_key → ban zamanları

	// Callbacks: forum katmanına bildir.
	onApprove       func(postID string)
	onTrustedAuthor func(authorKey string)
}

// LoadFounderKey reads <dataDir>/founder.key, sets FounderPubKeyHex, and
// returns whether the current user (myPubKeyHex) is the founder.
// If the file does not exist, myPubKeyHex is written there (first-run).
func LoadFounderKey(dataDir, myPubKeyHex string) (isFounder bool, err error) {
	path := filepath.Join(dataDir, FounderKeyFile)
	data, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		// İlk çalışma: bu kullanıcı kurucu oluyor.
		if werr := os.WriteFile(path, []byte(myPubKeyHex), 0o600); werr != nil {
			return false, fmt.Errorf("moderation: founder.key yazılamadı: %w", werr)
		}
		FounderPubKeyHex = myPubKeyHex
		return true, nil
	}
	if readErr != nil {
		return false, fmt.Errorf("moderation: founder.key okunamadı: %w", readErr)
	}

	key := strings.TrimSpace(string(data))
	if len(key) != 64 {
		return false, fmt.Errorf("moderation: founder.key geçersiz (64 hex karakter olmalı, %d var)", len(key))
	}
	if _, decErr := hex.DecodeString(key); decErr != nil {
		return false, fmt.Errorf("moderation: founder.key geçersiz hex: %w", decErr)
	}

	FounderPubKeyHex = key
	return strings.EqualFold(key, myPubKeyHex), nil
}

// New creates a Moderator. LoadFounderKey must be called before New so
// FounderPubKeyHex is set correctly.
func New(dataDir string, founderPeerID peer.ID) (*Moderator, error) {
	pub, err := hex.DecodeString(FounderPubKeyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("moderation: geçersiz kurucu key: %w", err)
	}

	m := &Moderator{
		bans:           make(map[peer.ID]time.Time),
		banMods:        make(map[peer.ID]string),
		revoked:        make(map[string]time.Time),
		trustedAuthors: make(map[string]TrustedAuthorCert),
		founder:        ed25519.PublicKey(pub),
		founderID:      founderPeerID,
		dataDir:        dataDir,
		rates:          make(map[string][]time.Time),
	}

	m.loadBans()
	m.loadRevocations()
	m.loadRates()
	m.loadTrustedAuthors()
	return m, nil
}

// isDisabled returns true when the founder key is all-zeros (moderation off).
func (m *Moderator) isDisabled() bool {
	for _, b := range m.founder {
		if b != 0 {
			return false
		}
	}
	return true
}

// Start subscribes to ModerationTopic and enforces incoming messages.
func (m *Moderator) Start(ctx context.Context, topic *network.Topic) {
	ch := topic.Messages(ctx)
	go func() {
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				m.handleEnvelope(data)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// PublishBan publishes a pre-built BanMsg to the moderation topic.
func (m *Moderator) PublishBan(ctx context.Context, topic *network.Topic, ban BanMsg) error {
	env := Envelope{Type: "ban", Ban: &ban}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return topic.Publish(ctx, data)
}

// PublishRevoke publishes a RevokeMsg signed by the founder.
func (m *Moderator) PublishRevoke(ctx context.Context, topic *network.Topic, rev RevokeMsg) error {
	env := Envelope{Type: "revoke", Revoke: &rev}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return topic.Publish(ctx, data)
}

// ── Mesaj İşleme ─────────────────────────────────────────────────────────────

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
	case "trusted":
		if env.Trusted != nil {
			m.handleTrustedAuthorMsg(env.Trusted)
		}
	case "approve":
		if env.Approve != nil {
			m.handleApproveMsg(env.Approve)
		}
	}
}

func (m *Moderator) handleTrustedAuthorMsg(cert *TrustedAuthorCert) {
	if cert.AuthorKey == "" || cert.Sig == "" {
		return
	}
	if !m.verifyTrustedCert(cert) {
		return
	}
	m.mu.Lock()
	m.trustedAuthors[cert.AuthorKey] = *cert
	m.mu.Unlock()
	m.saveTrustedAuthors()
	if m.onTrustedAuthor != nil {
		m.onTrustedAuthor(cert.AuthorKey)
	}
}

func (m *Moderator) handleApproveMsg(msg *ApprovalMsg) {
	if msg.PostID == "" || msg.Sig == "" {
		return
	}
	if !m.verifyApproval(msg) {
		return
	}
	if m.onApprove != nil {
		m.onApprove(msg.PostID)
	}
}

// ── Güvenilir Yazar API ───────────────────────────────────────────────────────

// SetOnApprove registers a callback invoked when a valid ApprovalMsg is received.
func (m *Moderator) SetOnApprove(cb func(postID string)) { m.onApprove = cb }

// SetOnTrustedAuthor registers a callback invoked when a TrustedAuthorCert is received.
func (m *Moderator) SetOnTrustedAuthor(cb func(authorKey string)) { m.onTrustedAuthor = cb }

// IsTrustedAuthor returns true if authorKey has a valid, unexpired TrustedAuthorCert.
func (m *Moderator) IsTrustedAuthor(authorKey string) bool {
	m.mu.RLock()
	cert, ok := m.trustedAuthors[authorKey]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if cert.Permanent {
		return true
	}
	return time.Now().Before(cert.ExpiresAt)
}

// ── Doğrulama (Trusted + Approve) ────────────────────────────────────────────

func (m *Moderator) verifyTrustedCert(cert *TrustedAuthorCert) bool {
	sig, err := hex.DecodeString(cert.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	var payload string
	if cert.Permanent {
		payload = fmt.Sprintf("trusted|%s|permanent", cert.AuthorKey)
	} else {
		if time.Now().After(cert.ExpiresAt) {
			return false
		}
		payload = fmt.Sprintf("trusted|%s|%d", cert.AuthorKey, cert.ExpiresAt.Unix())
	}
	return ed25519.Verify(m.founder, []byte(payload), sig)
}

func (m *Moderator) verifyApproval(msg *ApprovalMsg) bool {
	sig, err := hex.DecodeString(msg.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	payload := fmt.Sprintf("approve|%s|%d", msg.PostID, msg.IssuedAt.Unix())

	if msg.Cert == nil {
		// Kurucu imzası
		return ed25519.Verify(m.founder, []byte(payload), sig)
	}
	// Moderatör imzası: önce cert'i doğrula, sonra imzayı
	if !m.verifyCert(*msg.Cert) {
		return false
	}
	modKey, err := hex.DecodeString(msg.Cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(modKey), []byte(payload), sig)
}

// ── TrustedAuthor Kalıcılığı ──────────────────────────────────────────────────

func (m *Moderator) saveTrustedAuthors() {
	if m.dataDir == "" {
		return
	}
	m.mu.RLock()
	certs := make([]TrustedAuthorCert, 0, len(m.trustedAuthors))
	for _, c := range m.trustedAuthors {
		certs = append(certs, c)
	}
	m.mu.RUnlock()
	data, err := json.Marshal(certs)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(m.dataDir, "trusted_authors.json"), data, 0o600)
}

func (m *Moderator) loadTrustedAuthors() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, "trusted_authors.json"))
	if err != nil {
		return
	}
	var certs []TrustedAuthorCert
	if err := json.Unmarshal(data, &certs); err != nil {
		return
	}
	m.mu.Lock()
	for _, c := range certs {
		if c.Permanent || time.Now().Before(c.ExpiresAt) {
			m.trustedAuthors[c.AuthorKey] = c
		}
	}
	m.mu.Unlock()
}

func (m *Moderator) handleBanMsg(ban *BanMsg) {
	// 1. Alan doğrulama
	if ban.BannedPeer == "" || ban.Reason == "" || ban.Sig == "" {
		return
	}
	if len(ban.Reason) > 256 {
		return
	}

	// 2. Süre sınırı: min 1 dakika, max 30 gün, geçmişte olamaz
	now := time.Now()
	if !ban.ExpiresAt.After(now.Add(time.Minute)) {
		return // çok kısa veya geçmişte — spam önleme
	}
	if ban.ExpiresAt.After(now.Add(maxBanDays * 24 * time.Hour)) {
		return
	}

	// 3. Geçerli peer ID
	pid, err := peer.Decode(ban.BannedPeer)
	if err != nil {
		return
	}

	// 4. Kurucu banlanamaz
	if pid == m.founderID {
		return
	}

	// 5. Replay koruması
	if m.alreadySeen(ban.Sig) {
		return
	}

	// 6. Moderatör sertifikası doğrula
	if !m.verifyCert(ban.Cert) {
		return
	}

	// 7. Ban imzasını doğrula
	if !m.verifyBan(ban) {
		return
	}

	// 8. Rate limiting
	if !m.allowBan(ban.Cert.ModeratorKey) {
		return
	}

	// 9. Ban uygula ve kaydet (moderatör key'i de sakla, iptal kontrolü için)
	m.mu.Lock()
	m.bans[pid] = ban.ExpiresAt
	m.banMods[pid] = ban.Cert.ModeratorKey
	m.mu.Unlock()
	m.saveBans()
}

func (m *Moderator) handleRevokeMsg(rev *RevokeMsg) {
	if rev.ModeratorKey == "" || rev.Sig == "" {
		return
	}
	// Replay koruması (revoke idempotent olsa da gereksiz yük önlenir)
	if m.alreadySeenRevoke(rev.Sig) {
		return
	}
	sig, err := hex.DecodeString(rev.Sig)
	if err != nil {
		return
	}
	payload := fmt.Sprintf("revoke|%s|%d", rev.ModeratorKey, rev.IssuedAt.Unix())
	if !ed25519.Verify(m.founder, []byte(payload), sig) {
		return
	}
	m.mu.Lock()
	m.revoked[rev.ModeratorKey] = rev.IssuedAt
	m.mu.Unlock()
	m.saveRevocations()
}

// ── Doğrulama ─────────────────────────────────────────────────────────────────

func (m *Moderator) verifyCert(cert ModeratorCert) bool {
	modKey, err := hex.DecodeString(cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}

	// Kurucu kendisine cert veremez
	if strings.EqualFold(cert.ModeratorKey, FounderPubKeyHex) {
		return false
	}

	// Süre kontrolü: sadece süreli certlerde yapılır
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

	// Payload: permanent ve süreli için farklı format
	var payload string
	if cert.Permanent {
		payload = fmt.Sprintf("%s|permanent", cert.ModeratorKey)
	} else {
		payload = fmt.Sprintf("%s|%d", cert.ModeratorKey, cert.ExpiresAt.Unix())
	}
	if !ed25519.Verify(m.founder, []byte(payload), sig) {
		return false
	}

	// İptal listesinde mi?
	m.mu.RLock()
	revokedAt, isRevoked := m.revoked[cert.ModeratorKey]
	m.mu.RUnlock()
	if isRevoked {
		if cert.Permanent {
			return false // süresiz cert de iptal edilebilir
		}
		if !cert.ExpiresAt.After(revokedAt) {
			return false
		}
	}

	return true
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
	payload := fmt.Sprintf("%s|%s|%d", ban.BannedPeer, ban.Reason, ban.ExpiresAt.Unix())
	return ed25519.Verify(ed25519.PublicKey(modKey), []byte(payload), sig)
}

// ── Replay Koruması ───────────────────────────────────────────────────────────

// seenAdd arar ve yoksa ekler; true dönerse zaten görülmüş.
func seenAdd(buf *[]string, sig string) bool {
	h := sha256.Sum256([]byte(sig))
	fp := hex.EncodeToString(h[:]) // 256 bit — 64 hex karakter
	for _, s := range *buf {
		if s == fp {
			return true
		}
	}
	*buf = append(*buf, fp)
	if len(*buf) > seenCacheSize {
		*buf = (*buf)[1:]
	}
	return false
}

func (m *Moderator) alreadySeen(sig string) bool {
	m.seenMu.Lock()
	defer m.seenMu.Unlock()
	return seenAdd(&m.seen, sig)
}

func (m *Moderator) alreadySeenRevoke(sig string) bool {
	m.seenMu.Lock()
	defer m.seenMu.Unlock()
	return seenAdd(&m.seenRevoke, sig)
}

// ── Rate Limiting ─────────────────────────────────────────────────────────────

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
	go m.saveRates() // async — kritik yolda bloklamaz
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
	_ = os.WriteFile(filepath.Join(m.dataDir, "mod_rates.json"), data, 0o600)
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

// ── Ban Sorgu ─────────────────────────────────────────────────────────────────

// IsBanned returns true if pid is under an active ban.
func (m *Moderator) IsBanned(pid peer.ID) bool {
	if m.isDisabled() {
		return false
	}
	m.mu.RLock()
	expiry, ok := m.bans[pid]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		m.mu.Lock()
		delete(m.bans, pid)
		delete(m.banMods, pid)
		m.mu.Unlock()
		m.saveBans()
		return false
	}
	return true
}

// ── Kalıcılık ─────────────────────────────────────────────────────────────────

func (m *Moderator) saveBans() {
	if m.dataDir == "" {
		return
	}
	m.mu.RLock()
	now := time.Now()
	var active []PersistedBan
	for pid, exp := range m.bans {
		if exp.After(now) {
			active = append(active, PersistedBan{
				PeerID:       pid.String(),
				ExpiresAt:    exp,
				ModeratorKey: m.banMods[pid],
			})
		}
	}
	m.mu.RUnlock()

	data, err := json.Marshal(active)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(m.dataDir, "active_bans.json"), data, 0o600)
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
		if !b.ExpiresAt.After(now) {
			continue
		}
		// İptal edilmiş moderatörün banını yükleme
		if b.ModeratorKey != "" {
			if _, revoked := m.revoked[b.ModeratorKey]; revoked {
				continue
			}
		}
		pid, err := peer.Decode(b.PeerID)
		if err == nil {
			m.bans[pid] = b.ExpiresAt
			m.banMods[pid] = b.ModeratorKey
		}
	}
	m.mu.Unlock()
}

// ── Revokasyon Kalıcılığı ─────────────────────────────────────────────────────

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
	_ = os.WriteFile(filepath.Join(m.dataDir, "active_revocations.json"), data, 0o600)
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

// ── libp2p ConnectionGater ────────────────────────────────────────────────────

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

// FindModCert searches dataDir/bans/mod_*.json for a valid ModeratorCert
// whose ModeratorKey matches myPubHex. Returns nil if none found.
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
		var payload string
		if cert.Permanent {
			payload = fmt.Sprintf("%s|permanent", cert.ModeratorKey)
		} else {
			payload = fmt.Sprintf("%s|%d", cert.ModeratorKey, cert.ExpiresAt.Unix())
		}
		if !ed25519.Verify(founderPub, []byte(payload), sig) {
			continue
		}
		c := cert
		if best == nil || cert.Permanent || (!best.Permanent && cert.ExpiresAt.After(best.ExpiresAt)) {
			best = &c
		}
	}
	return best
}
