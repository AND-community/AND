package forum

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SyncProtocol is the libp2p stream protocol AND nodes use to exchange
// forum history. When a peer connects, we open a stream, send them the
// post IDs we already have, and they reply with any posts/replies we're
// missing — catching us up on everything we missed while offline.
const SyncProtocol = "/and/sync/1.0.0"

// syncTimeout bounds how long a single sync exchange may take.
const syncTimeout = 30 * time.Second

// maxSyncPosts is the maximum number of posts sent in a single sync response.
// Limits memory and bandwidth for the initial sync of a new peer.
const maxSyncPosts = 500

type syncRequest struct {
	PostIDs    []string `json:"post_ids"`
	Tombstones []string `json:"tombstones,omitempty"` // imzalı DeleteMsg JSON'ları
}

type syncResponse struct {
	Posts      []*Post             `json:"posts"`
	Replies    map[string][]*Reply `json:"replies"`
	Tombstones []string            `json:"tombstones,omitempty"` // imzalı DeleteMsg JSON'ları
}

// RegisterSync registers the forum sync stream handler on h. Call this
// once after forum.New so incoming sync requests from peers are handled.
func (f *Forum) RegisterSync(h libp2phost.Host) {
	h.SetStreamHandler(SyncProtocol, f.handleSyncStream)
}

// handleSyncStream serves an incoming sync request.
// The remote peer sends their known post IDs and tombstones; we reply with
// everything they're missing and our own tombstones.
func (f *Forum) handleSyncStream(s libp2pnet.Stream) {
	defer s.Close()

	s.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	var req syncRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return
	}

	// Gelen tombstone'ları uygula: peer'ın sildiği postları biz de silelim.
	// Postlar önceden memory'de; tombstone doğrulaması author eşleşmesine bakabilir.
	for _, tsJSON := range req.Tombstones {
		var d DeleteMsg
		if json.Unmarshal([]byte(tsJSON), &d) == nil && verifyDeleteMsg(&d) {
			f.applyDelete(d.PostID, d.AuthorKey, []byte(tsJSON))
		}
	}

	have := make(map[string]bool, len(req.PostIDs))
	for _, id := range req.PostIDs {
		have[id] = true
	}

	f.mu.Lock()
	var newPosts []*Post
	for _, p := range f.posts {
		if !have[p.ID] {
			newPosts = append(newPosts, p)
			if len(newPosts) >= maxSyncPosts {
				break // tek seferde max post gönder; peer bir sonraki sync'te kalanları alır
			}
		}
	}
	// Sadece yeni postların reply'larını gönder; peer'ın zaten sahip olduğu
	// postların reply'ları GossipSub üzerinden canlı olarak iletilmiş olmalı.
	newReplies := make(map[string][]*Reply, len(newPosts))
	for _, p := range newPosts {
		if rs, ok := f.replies[p.ID]; ok {
			newReplies[p.ID] = rs
		}
	}
	f.mu.Unlock()

	// Kendi tombstone'larımızı gönder.
	ourTombs, _ := f.db.AllTombstoneJSON()

	resp := syncResponse{Posts: newPosts, Replies: newReplies, Tombstones: ourTombs}
	_ = json.NewEncoder(s).Encode(resp)
}

// SyncWithPeer opens a sync stream to peerID and stores any posts/replies
// we were missing. Errors are expected (peer may not support the protocol)
// and are returned so callers can log or ignore them.
func (f *Forum) SyncWithPeer(ctx context.Context, h libp2phost.Host, peerID peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	s, err := h.NewStream(ctx, peerID, SyncProtocol)
	if err != nil {
		return fmt.Errorf("sync: open stream to %s: %w", peerID.ShortString(), err)
	}
	defer s.Close()

	s.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	// Send our known post IDs so the peer can skip them.
	f.mu.Lock()
	ids := make([]string, 0, len(f.byID))
	for id := range f.byID {
		ids = append(ids, id)
	}
	f.mu.Unlock()

	// Kendi tombstone'larımızı gönder; peer bunları uygulayabilir.
	myTombs, _ := f.db.AllTombstoneJSON()

	if err := json.NewEncoder(s).Encode(syncRequest{PostIDs: ids, Tombstones: myTombs}); err != nil {
		return fmt.Errorf("sync: send request: %w", err)
	}
	s.CloseWrite() //nolint:errcheck

	var resp syncResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		return fmt.Errorf("sync: read response: %w", err)
	}

	// Postları önce ekle — ardından tombstone'lar gelecek.
	// Sıralama önemli: author doğrulaması post memory'de olmasını gerektirir.
	for _, p := range resp.Posts {
		if verifyPost(p) {
			f.storePost(p)
		}
	}
	for postID, replies := range resp.Replies {
		for _, r := range replies {
			if r.PostID == postID && verifyReply(r) {
				f.storeReply(r)
			}
		}
	}

	// Peer'ın tombstone'larını uygula (az önce eklediğimiz postlar dahil).
	for _, tsJSON := range resp.Tombstones {
		var d DeleteMsg
		if json.Unmarshal([]byte(tsJSON), &d) == nil && verifyDeleteMsg(&d) {
			f.applyDelete(d.PostID, d.AuthorKey, []byte(tsJSON))
		}
	}

	return nil
}
