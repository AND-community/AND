package chatlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/protokol"
)

const (
	maxSyncRequestBytes  = 2 * 1024 * 1024  // istek: mesaj ID listesi, 2 MB yeterli
	maxSyncResponseBytes = 16 * 1024 * 1024 // yanıt: en fazla maxSyncMessages mesaj
)

// SyncProtocol is the libp2p stream protocol AND nodes use to exchange chat
// history, mirroring internal/forum's SyncProtocol/RegisterSync/SyncWithPeer
// shape exactly but for chat messages instead of posts/replies — see that
// package's sync.go for the reference pattern this was built from.
const SyncProtocol = protokol.SohbetSenkronProtokolu

const syncTimeout = 30 * time.Second

// maxSyncMessages caps how many messages a single sync response carries,
// bounding memory/bandwidth for a newly-joined peer's initial catch-up.
const maxSyncMessages = 500

type syncRequest struct {
	MessageIDs []string `json:"message_ids"`
}

type syncResponse struct {
	Messages []*Message `json:"messages"`
}

// RegisterSync registers the chat sync stream handler on h. Call once
// after chatlog.New so incoming sync requests from peers are handled.
func (s *Store) RegisterSync(h libp2phost.Host) {
	h.SetStreamHandler(SyncProtocol, s.handleSyncStream)
}

// handleSyncStream serves an incoming sync request: the remote peer sends
// the message IDs they already have, we reply with everything they're
// missing (signed messages only — see handleMessage/persist).
func (s *Store) handleSyncStream(st libp2pnet.Stream) {
	defer st.Close()
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	var req syncRequest
	if err := json.NewDecoder(io.LimitReader(st, maxSyncRequestBytes)).Decode(&req); err != nil {
		return
	}
	if len(req.MessageIDs) > maxSyncMessages*4 {
		return
	}

	have := make(map[string]bool, len(req.MessageIDs))
	for _, id := range req.MessageIDs {
		have[id] = true
	}

	s.mu.RLock()
	var missing []*Message
	for _, m := range s.messages {
		if !have[m.ID] {
			missing = append(missing, m)
			if len(missing) >= maxSyncMessages {
				break // bir sync turunda en fazla bu kadar; kalan sonraki peer bağlantısında gelir
			}
		}
	}
	s.mu.RUnlock()

	_ = json.NewEncoder(st).Encode(syncResponse{Messages: missing})
}

// SyncWithPeer opens a sync stream to peerID and stores any messages we
// were missing. Errors are expected (peer may not support the protocol)
// and returned so callers can log or ignore them.
func (s *Store) SyncWithPeer(ctx context.Context, h libp2phost.Host, peerID peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	st, err := h.NewStream(ctx, peerID, SyncProtocol)
	if err != nil {
		return fmt.Errorf("chatlog sync: open stream to %s: %w", peerID.ShortString(), err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	s.mu.RLock()
	ids := make([]string, 0, len(s.byID))
	for id := range s.byID {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	if err := json.NewEncoder(st).Encode(syncRequest{MessageIDs: ids}); err != nil {
		return fmt.Errorf("chatlog sync: send request: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var resp syncResponse
	if err := json.NewDecoder(io.LimitReader(st, maxSyncResponseBytes)).Decode(&resp); err != nil {
		return fmt.Errorf("chatlog sync: read response: %w", err)
	}
	if len(resp.Messages) > maxSyncMessages {
		resp.Messages = resp.Messages[:maxSyncMessages]
	}

	// SyncWithPeer bir peer'ın gönderdiği geçmiş mesajları da tam olarak
	// handleMessage'daki (canlı GossipSub alımı) kadar sıkı doğruluyor —
	// senkronizasyon, imza/boyut/saat-kayması/hız-sınırı kontrollerini
	// atlayabileceğin ayrı bir güven sınırı DEĞİL.
	applied := false
	for _, m := range resp.Messages {
		if m == nil || len(m.Text) > MaxTextBytes || !m.Signed() {
			continue
		}
		if s.muted(m.AuthorKey) {
			continue
		}
		if s.authorBanned(m.AuthorKey) {
			continue
		}
		if m.SentAt.After(time.Now().Add(maxClockSkew)) {
			continue
		}
		if !verify(m) || !s.allow(m.AuthorKey, false) {
			continue
		}
		if m.ExpiresAt.IsZero() {
			m.ExpiresAt = m.SentAt.Add(MessageTTL)
		}
		if s.persist(m) {
			applied = true
		}
	}
	if applied {
		// Bu mesajlar canlı GossipSub akışından değil geçmişten geldi —
		// zaten gösterilenlerden daha ESKİ olabilirler, bu yüzden
		// publishLive/NewMessages üzerinden (kronolojik sırayı bozacak
		// şekilde) değil, HistoryChanged sinyaliyle iletiliyorlar; dinleyen
		// taraf (TUI) All()'u yeniden okuyup zaten doğru sırada olan tam
		// geçmişi baştan oluşturabilir.
		select {
		case s.historyChanged <- struct{}{}:
		default:
		}
	}
	return nil
}
