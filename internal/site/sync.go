package site

// sync.go, forum'un /and/sync/1.0.0'ını izleyen bir senkronizasyon
// protokolüdür (bkz. site.go'nun paket yorumu — profile'ın SAHİP OLMADIĞI
// şey tam olarak bu): yeni bağlanan bir eşle konuşup, o an bilmediğimiz
// sayfaları/tombstone'ları geriye dönük toplarız.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	maxSyncRequestBytes  = 1 * 1024 * 1024  // istek: bilinen slug listesi + tombstone anahtarları
	maxSyncResponseBytes = 32 * 1024 * 1024 // yanıt: en fazla maxSyncPages sayfa (her biri en fazla MaxBodyBytes)
)

// SyncProtocol is the libp2p stream protocol AND nodes use to exchange
// personal-site history.
const SyncProtocol = "/and/sitesync/1.0.0"

const syncTimeout = 30 * time.Second

// maxSyncPages bounds how many pages a single sync response may carry.
const maxSyncPages = 500

type syncRequest struct {
	// Known: yazar_key -> zaten sahip olunan slug listesi — yanıtı yalnızca
	// eksik sayfalarla sınırlamak için.
	Known map[string][]string `json:"known"`
	// KnownTombstones: zaten bilinen "yazar_key/slug" tombstone anahtarları
	// — yanıtlayan bunları tekrar göndermez.
	KnownTombstones []string `json:"known_tombstones,omitempty"`
}

type syncResponse struct {
	Pages      []*Page       `json:"pages"`
	Tombstones []*DeletePage `json:"tombstones,omitempty"`
}

// RegisterSync registers the site sync stream handler on h. Call this once
// after site.New so incoming sync requests from peers are handled.
func (s *Store) RegisterSync(h libp2phost.Host) {
	h.SetStreamHandler(SyncProtocol, s.handleSyncStream)
}

func (s *Store) handleSyncStream(st libp2pnet.Stream) {
	defer st.Close()
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	var req syncRequest
	if err := json.NewDecoder(io.LimitReader(st, maxSyncRequestBytes)).Decode(&req); err != nil {
		return
	}

	knownTomb := make(map[string]bool, len(req.KnownTombstones))
	for _, k := range req.KnownTombstones {
		knownTomb[k] = true
	}

	s.mu.RLock()
	var resp syncResponse
outer:
	for authorKey, authorPages := range s.pages {
		haveSlugs := make(map[string]bool, len(req.Known[authorKey]))
		for _, slug := range req.Known[authorKey] {
			haveSlugs[slug] = true
		}
		for slug, p := range authorPages {
			if haveSlugs[slug] {
				continue
			}
			cp := *p
			resp.Pages = append(resp.Pages, &cp)
			if len(resp.Pages) >= maxSyncPages {
				break outer
			}
		}
	}
	for key, d := range s.deleted {
		if knownTomb[key] {
			continue
		}
		cp := *d
		resp.Tombstones = append(resp.Tombstones, &cp)
	}
	s.mu.RUnlock()

	_ = json.NewEncoder(st).Encode(resp)
}

// SyncWithPeer opens a sync stream to peerID and stores any pages/
// tombstones we were missing. Errors are expected (peer may not support
// the protocol) and are returned so callers can log or ignore them.
func (s *Store) SyncWithPeer(ctx context.Context, h libp2phost.Host, peerID peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	s.mu.RLock()
	known := make(map[string][]string, len(s.pages))
	for authorKey, authorPages := range s.pages {
		slugs := make([]string, 0, len(authorPages))
		for slug := range authorPages {
			slugs = append(slugs, slug)
		}
		known[authorKey] = slugs
	}
	knownTomb := make([]string, 0, len(s.deleted))
	for key := range s.deleted {
		knownTomb = append(knownTomb, key)
	}
	s.mu.RUnlock()

	st, err := h.NewStream(ctx, peerID, SyncProtocol)
	if err != nil {
		return fmt.Errorf("site sync: stream açılamadı %s: %w", peerID.ShortString(), err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	if err := json.NewEncoder(st).Encode(syncRequest{Known: known, KnownTombstones: knownTomb}); err != nil {
		return fmt.Errorf("site sync: istek gönderilemedi: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var resp syncResponse
	if err := json.NewDecoder(io.LimitReader(st, maxSyncResponseBytes)).Decode(&resp); err != nil {
		return fmt.Errorf("site sync: yanıt okunamadı: %w", err)
	}
	if len(resp.Pages) > maxSyncPages {
		resp.Pages = resp.Pages[:maxSyncPages]
	}

	// fromSync=true: canlı gossip'e uygulanan minPageEditInterval'dan MUAF
	// (bir kerelik geçmiş toplu aktarımı, "spam" değil) ama Sybil/sayfa-
	// sayısı sınırları hâlâ geçerli — bkz. handlePage'in doc yorumu. Hiçbir
	// mesaja "sync'ten geldi" diye körü körüne güvenilmez: handlePage yine
	// de imzayı/uzunluk sınırlarını/saat sapmasını en baştan denetler.
	for _, p := range resp.Pages {
		s.handlePage(p, true)
	}
	for _, d := range resp.Tombstones {
		s.handleDelete(d)
	}
	return nil
}
