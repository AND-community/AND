package site

// sync.go, forum'un /and/sync/1.0.0'ını izleyen bir senkronizasyon
// protokolüdür (bkz. site.go'nun paket yorumu — profile'ın SAHİP OLMADIĞI
// şey tam olarak bu): yeni bağlanan bir eşle konuşup, o an bilmediğimiz
// (ya da yalnızca ESKİ bir sürümünü bildiğimiz — bkz. syncRequest'in
// KnownVersions'ı) sayfaları/tombstone'ları geriye dönük toplarız.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/internal/guvenli"
	"github.com/lucian95511/and/protokol"
)

const (
	maxSyncRequestBytes  = 1 * 1024 * 1024  // istek: bilinen slug listesi + tombstone anahtarları
	maxSyncResponseBytes = 32 * 1024 * 1024 // yanıt: en fazla maxSyncPages sayfa (her biri en fazla MaxBodyBytes)
)

// SyncProtocol is the libp2p stream protocol AND nodes use to exchange
// personal-site history.
const SyncProtocol = protokol.SiteSenkronProtokolu

const syncTimeout = 30 * time.Second

// maxSyncPages bounds how many pages a single sync response may carry.
const maxSyncPages = 500

type syncRequest struct {
	// Known: yazar_key -> zaten sahip olunan slug listesi — yanıtı yalnızca
	// eksik sayfalarla sınırlamak için.
	Known map[string][]string `json:"known"`
	// KnownVersions: yazar_key -> slug -> elimizdeki sürümün UpdatedAt'i
	// (UnixNano). Forum post'larının aksine bir site sayfası DEĞİŞEBİLİR —
	// yalnızca slug listesiyle ("bu slug'ı biliyorum") yanıtlayan, o slug'ın
	// DAHA YENİ bir sürümünü elinde tutsa bile göndermezdi: bir sayfanın
	// düzenlenmiş hali senkronizasyonla hiç geriye dönük yakalanamaz, canlı
	// gossip'te minPageEditInterval yüzünden düşen bir düzenleme de asla
	// telafi edilmezdi. Bu alanla yanıtlayan, bilinen bir slug'ı da
	// elindeki sürüm DAHA YENİYSE gönderir.
	//
	// İki yönde de geriye dönük uyumludur: bu alanı tanımayan ESKİ bir
	// yanıtlayıcı (encoding/json bilinmeyen alanları yok sayar) Known'a
	// göre eskisi gibi davranır; bu alanı göndermeyen ESKİ bir istekçinin
	// Known'daki slug'ları ise "güncel" sayılır (bkz. newerThanKnown).
	KnownVersions map[string]map[string]int64 `json:"known_versions,omitempty"`
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
	// Eşin gönderdiği veri işleyicide bir hatayı tetiklerse yalnızca bu akış
	// kapanır; düğüm çökmez (bkz. guvenli).
	defer guvenli.Kurtar("site senkron akışı", func() { _ = st.Reset() })
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	var req syncRequest
	if err := json.NewDecoder(io.LimitReader(st, maxSyncRequestBytes)).Decode(&req); err != nil {
		return
	}

	_ = json.NewEncoder(st).Encode(s.syncResponseFor(req))
}

// syncResponseFor computes the response to req: every page the requester
// either doesn't know at all or (per KnownVersions) holds an older version
// of, plus every tombstone it doesn't know. Split out of handleSyncStream
// so the selection rules are testable without a libp2p stream.
func (s *Store) syncResponseFor(req syncRequest) syncResponse {
	knownTomb := make(map[string]bool, len(req.KnownTombstones))
	for _, k := range req.KnownTombstones {
		knownTomb[k] = true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	var resp syncResponse
outer:
	for authorKey, authorPages := range s.pages {
		haveSlugs := make(map[string]bool, len(req.Known[authorKey]))
		for _, slug := range req.Known[authorKey] {
			haveSlugs[slug] = true
		}
		versions := req.KnownVersions[authorKey]
		for slug, p := range authorPages {
			if haveSlugs[slug] && !newerThanKnown(p, versions) {
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
	return resp
}

// newerThanKnown reports whether p is newer than the version the requester
// reported for p.Slug in versions (one author's KnownVersions entry). A
// slug with NO reported version — an older requester that predates
// KnownVersions — counts as up to date, i.e. exactly the pre-versioning
// "known slug = skip" behaviour, so such requesters see no change.
func newerThanKnown(p *Page, versions map[string]int64) bool {
	known, ok := versions[p.Slug]
	if !ok {
		return false
	}
	mine, ok := versionOf(p)
	return ok && mine > known
}

// versionOf returns p.UpdatedAt as UnixNano for KnownVersions, or ok=false
// when it falls outside the int64-nanosecond range (before 1678 / after
// 2262), where UnixNano is undefined. handlePage only bounds timestamps
// from above (maxClockSkew), so an author could sign such a page; it is
// then just synced the pre-versioning way (known slug = up to date).
func versionOf(p *Page) (int64, bool) {
	if p.UpdatedAt.Before(minVersionTime) || p.UpdatedAt.After(maxVersionTime) {
		return 0, false
	}
	return p.UpdatedAt.UnixNano(), true
}

var (
	minVersionTime = time.Unix(0, math.MinInt64)
	maxVersionTime = time.Unix(0, math.MaxInt64)
)

// encodeSyncRequest marshals req, dropping KnownVersions if that would push
// the request past limit: an OLDER responder decodes at most
// maxSyncRequestBytes and fails the whole sync on a longer request, so on a
// very large network it's better to fall back to the plain (pre-versioning)
// request — losing only edit backfill — than to lose new pages too.
func encodeSyncRequest(req syncRequest, limit int) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil || len(data) <= limit || req.KnownVersions == nil {
		return data, err
	}
	req.KnownVersions = nil
	return json.Marshal(req)
}

// SyncWithPeer opens a sync stream to peerID and stores any pages/
// tombstones we were missing, plus newer versions of pages we only held an
// older version of. Errors are expected (peer may not support the
// protocol) and are returned so callers can log or ignore them.
func (s *Store) SyncWithPeer(ctx context.Context, h libp2phost.Host, peerID peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	s.mu.RLock()
	known := make(map[string][]string, len(s.pages))
	knownVersions := make(map[string]map[string]int64, len(s.pages))
	for authorKey, authorPages := range s.pages {
		slugs := make([]string, 0, len(authorPages))
		versions := make(map[string]int64, len(authorPages))
		for slug, p := range authorPages {
			slugs = append(slugs, slug)
			if v, ok := versionOf(p); ok {
				versions[slug] = v
			}
		}
		known[authorKey] = slugs
		knownVersions[authorKey] = versions
	}
	knownTomb := make([]string, 0, len(s.deleted))
	for key := range s.deleted {
		knownTomb = append(knownTomb, key)
	}
	s.mu.RUnlock()

	reqData, err := encodeSyncRequest(syncRequest{Known: known, KnownVersions: knownVersions, KnownTombstones: knownTomb}, maxSyncRequestBytes)
	if err != nil {
		return fmt.Errorf("site sync: istek hazırlanamadı: %w", err)
	}

	st, err := h.NewStream(ctx, peerID, SyncProtocol)
	if err != nil {
		return fmt.Errorf("site sync: stream açılamadı %s: %w", peerID.ShortString(), err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(syncTimeout)) //nolint:errcheck

	if _, err := st.Write(reqData); err != nil {
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
