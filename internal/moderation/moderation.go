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

	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/control"
	lp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const ModerationTopic = "github.com/lucian95511/and/moderation/1.0.0"

const FounderKeyFile = "founder.key"

const (
	maxCertDays       = 7
	maxBanDays        = 30
	banRateWindow     = time.Hour
	banRateLimit      = 20
	seenCacheSize     = 512
	ratesSaveInterval = 30 * time.Second
)

var FounderPubKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"

type ModeratorCert struct {
	ModeratorKey string    `json:"moderator_key"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Permanent    bool      `json:"permanent,omitempty"`
	Sig          string    `json:"sig"`
}

type BanMsg struct {
	BannedPeer string        `json:"banned_peer"`
	Reason     string        `json:"reason"`
	ExpiresAt  time.Time     `json:"expires_at"`
	Cert       ModeratorCert `json:"cert"`
	Sig        string        `json:"sig"`
}

type RevokeMsg struct {
	ModeratorKey string    `json:"moderator_key"`
	IssuedAt     time.Time `json:"issued_at"`
	Sig          string    `json:"sig"`
}

type TrustedAuthorCert struct {
	AuthorKey string    `json:"author_key"`
	IssuedAt  time.Time `json:"issued_at"`
	Permanent bool      `json:"permanent,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Sig       string    `json:"sig"`
}

type ApprovalMsg struct {
	PostID   string         `json:"post_id"`
	IssuedAt time.Time      `json:"issued_at"`
	Cert     *ModeratorCert `json:"cert,omitempty"`
	Sig      string         `json:"sig"`
}

type Envelope struct {
	Type    string             `json:"type"`
	Ban     *BanMsg            `json:"ban,omitempty"`
	Revoke  *RevokeMsg         `json:"revoke,omitempty"`
	Trusted *TrustedAuthorCert `json:"trusted,omitempty"`
	Approve *ApprovalMsg       `json:"approve,omitempty"`
}

type PersistedBan struct {
	PeerID       string    `json:"peer_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	ModeratorKey string    `json:"moderator_key,omitempty"`
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
	mu             sync.RWMutex
	bans           map[peer.ID]time.Time
	banMods        map[peer.ID]string
	revoked        map[string]time.Time
	trustedAuthors map[string]TrustedAuthorCert
	founder        ed25519.PublicKey
	founderID      peer.ID
	dataDir        string

	seenBans    *seenSet
	seenRevokes *seenSet

	rateMu     sync.Mutex
	rates      map[string][]time.Time
	ratesDirty bool

	onApprove       func(postID string)
	onTrustedAuthor func(authorKey string)
}

func LoadFounderKey(dataDir, myPubKeyHex string) (isFounder bool, err error) {
	path := filepath.Join(dataDir, FounderKeyFile)
	data, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
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
		seenBans:       newSeenSet(seenCacheSize),
		seenRevokes:    newSeenSet(seenCacheSize),
	}

	m.loadBans()
	m.loadRevocations()
	m.loadRates()
	m.loadTrustedAuthors()
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
	go m.cleanupLoop(ctx)
	go m.ratesSaveLoop(ctx)
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
	for pid, exp := range m.bans {
		if now.After(exp) {
			delete(m.bans, pid)
			delete(m.banMods, pid)
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

func (m *Moderator) SetOnApprove(cb func(postID string)) { m.onApprove = cb }

func (m *Moderator) SetOnTrustedAuthor(cb func(authorKey string)) { m.onTrustedAuthor = cb }

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
		return ed25519.Verify(m.founder, []byte(payload), sig)
	}
	if !m.verifyCert(*msg.Cert) {
		return false
	}
	modKey, err := hex.DecodeString(msg.Cert.ModeratorKey)
	if err != nil || len(modKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(modKey), []byte(payload), sig)
}

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
	if ban.BannedPeer == "" || ban.Reason == "" || ban.Sig == "" {
		return
	}
	if len(ban.Reason) > 256 {
		return
	}

	now := time.Now()
	if !ban.ExpiresAt.After(now.Add(time.Minute)) {
		return
	}
	if ban.ExpiresAt.After(now.Add(maxBanDays * 24 * time.Hour)) {
		return
	}

	pid, err := peer.Decode(ban.BannedPeer)
	if err != nil {
		return
	}

	if pid == m.founderID {
		return
	}

	if m.alreadySeen(ban.Sig) {
		return
	}

	if !m.verifyCert(ban.Cert) {
		return
	}

	if !m.verifyBan(ban) {
		return
	}

	if !m.allowBan(ban.Cert.ModeratorKey) {
		return
	}

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

	var payload string
	if cert.Permanent {
		payload = fmt.Sprintf("%s|permanent", cert.ModeratorKey)
	} else {
		payload = fmt.Sprintf("%s|%d", cert.ModeratorKey, cert.ExpiresAt.Unix())
	}
	if !ed25519.Verify(m.founder, []byte(payload), sig) {
		return false
	}

	m.mu.RLock()
	revokedAt, isRevoked := m.revoked[cert.ModeratorKey]
	m.mu.RUnlock()
	if isRevoked {
		if cert.Permanent {
			return false
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

func (m *Moderator) alreadySeen(sig string) bool {
	return m.seenBans.seen(sig)
}

func (m *Moderator) alreadySeenRevoke(sig string) bool {
	return m.seenRevokes.seen(sig)
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

func (m *Moderator) IsBanned(pid peer.ID) bool {
	if m.isDisabled() {
		return false
	}
	m.mu.RLock()
	expiry, ok := m.bans[pid]
	m.mu.RUnlock()
	return ok && time.Now().Before(expiry)
}

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
