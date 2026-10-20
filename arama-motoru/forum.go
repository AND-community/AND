package main

// forum.go — AND'ın forum protokolünün BAĞIMSIZ bir yeniden uygulaması:
// aynı Post/Reply/envelope JSON biçimi, aynı imza-payload'ı, aynı
// GossipSub konusu (forumTopicAdi) ve aynı senkronizasyon protokolü
// (/and/sync/1.0.0) — bkz. AND/internal/forum/forum.go + sync.go, bu
// dosya onların bire bir karşılığıdır (yalnızca CLIENT tarafı: bu program
// gelen postları/yanıtları doğrulayıp indeksler, ama kendi başına post
// oluşturmaz ve başka eşlerin senkronizasyon isteklerine YANIT vermez —
// yalnızca okuyucu/indeksleyici bir eş).
//
// Güvenlik: her post/yanıt, tıpkı AND'ın kendisinin yaptığı gibi
// ed25519 imzası doğrulanmadan asla indekslenmez — bir saldırganın sahte
// içerik enjekte edip arama sonuçlarını kirletmesi bu sayede engellenir.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const forumSyncProtokolu = "/and/sync/1.0.0"
const forumSyncZamanAsimi = 30 * time.Second
const maxForumSyncYanitBayt = 48 * 1024 * 1024

type Post struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Sig        string    `json:"sig"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

type Reply struct {
	ID         string    `json:"id"`
	PostID     string    `json:"post_id"`
	AuthorName string    `json:"author_name"`
	AuthorKey  string    `json:"author_key"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	Sig        string    `json:"sig"`
}

type deleteMsg struct {
	PostID    string    `json:"post_id"`
	AuthorKey string    `json:"author_key"`
	DeletedAt time.Time `json:"deleted_at"`
	Sig       string    `json:"sig"`
}

type forumZarf struct {
	Type   string     `json:"type"`
	Post   *Post      `json:"post,omitempty"`
	Reply  *Reply     `json:"reply,omitempty"`
	Delete *deleteMsg `json:"delete,omitempty"`
	// mod_delete kasıtlı olarak atlandı: içeriği yalnızca founder/moderatör
	// sertifikası doğrulanınca silinmesi gerekir, bu program AND'ın
	// moderasyon zincirini (internal/moderation) uygulamıyor — bkz.
	// README'nin kapsam notu.
}

func postPayload(p *Post) []byte {
	return []byte(p.ID + "|" + p.Category + "|" + p.AuthorKey + "|" + p.Title + "|" + p.Body + "|" + p.CreatedAt.String())
}

func replyPayload(r *Reply) []byte {
	return []byte(r.ID + "|" + r.PostID + "|" + r.AuthorKey + "|" + r.Body + "|" + r.CreatedAt.String())
}

func deleteMsgPayload(d *deleteMsg) []byte {
	return []byte("delete|" + d.PostID + "|" + d.AuthorKey + "|" + fmt.Sprint(d.DeletedAt.Unix()))
}

func verifyEd25519(authorKeyHex, sigHex string, payload []byte) bool {
	pub, err := hex.DecodeString(authorKeyHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), payload, sig)
}

func verifyPost(p *Post) bool   { return verifyEd25519(p.AuthorKey, p.Sig, postPayload(p)) }
func verifyReply(r *Reply) bool { return verifyEd25519(r.AuthorKey, r.Sig, replyPayload(r)) }
func verifyDelete(d *deleteMsg) bool {
	return verifyEd25519(d.AuthorKey, d.Sig, deleteMsgPayload(d))
}

// forumMesajIsle, gossip'ten (ya da sync yanıtından) gelen ham bir forum
// zarfını doğrulayıp depoya uygular.
func forumMesajIsle(depo *Depo, data []byte) {
	var z forumZarf
	if err := json.Unmarshal(data, &z); err != nil {
		return
	}
	switch z.Type {
	case "post":
		if z.Post == nil {
			return
		}
		if !verifyPost(z.Post) {
			gunlukUyari("geçersiz imzalı post reddedildi:", z.Post.ID)
			return
		}
		if err := depo.Ekle(AramaSonucu{
			Tip: TipKonu, KaynakID: z.Post.ID, Kategori: z.Post.Category, Baslik: z.Post.Title,
			Govde: z.Post.Body, YazarAd: z.Post.AuthorName, YazarKey: z.Post.AuthorKey,
			OlusturmaTS: z.Post.CreatedAt,
			Sig:         z.Post.Sig,
		}); err != nil {
			gunlukHata("post indekslenemedi:", z.Post.ID, err)
		}
	case "reply":
		if z.Reply == nil {
			return
		}
		if !verifyReply(z.Reply) {
			gunlukUyari("geçersiz imzalı yanıt reddedildi:", z.Reply.ID)
			return
		}
		if err := depo.Ekle(AramaSonucu{
			Tip: TipYanit, KaynakID: z.Reply.ID, UstID: z.Reply.PostID,
			Govde: z.Reply.Body, YazarAd: z.Reply.AuthorName, YazarKey: z.Reply.AuthorKey,
			OlusturmaTS: z.Reply.CreatedAt,
			Sig:         z.Reply.Sig,
		}); err != nil {
			gunlukHata("yanıt indekslenemedi:", z.Reply.ID, err)
		}
	case "delete":
		if z.Delete == nil {
			return
		}
		if !verifyDelete(z.Delete) {
			gunlukUyari("geçersiz imzalı silme isteği reddedildi:", z.Delete.PostID)
			return
		}
		_ = depo.Sil(z.Delete.PostID, TipKonu)
	}
}

// ─── Senkronizasyon (yalnızca istemci) ─────────────────────────────────────────

type forumSyncIstek struct {
	PostIDs    []string `json:"post_ids"`
	Tombstones []string `json:"tombstones,omitempty"`
}

type forumSyncYanit struct {
	Posts      []*Post             `json:"posts"`
	Replies    map[string][]*Reply `json:"replies"`
	Tombstones []string            `json:"tombstones,omitempty"`
}

// forumIleSenkronizeOl, peerID'ye bir forum sync stream'i açar, bilinen
// post ID'lerimizi gönderir, eksik olanları alıp doğrulayıp indeksler.
func forumIleSenkronizeOl(ctx context.Context, h libp2phost.Host, peerID peer.ID, depo *Depo) error {
	ctx, cancel := context.WithTimeout(ctx, forumSyncZamanAsimi)
	defer cancel()

	st, err := h.NewStream(ctx, peerID, forumSyncProtokolu)
	if err != nil {
		return fmt.Errorf("forum sync: stream açılamadı: %w", err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(forumSyncZamanAsimi)) //nolint:errcheck

	bilinenIDler, err := depo.BilinenIDler(TipKonu)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(st).Encode(forumSyncIstek{PostIDs: bilinenIDler}); err != nil {
		return fmt.Errorf("forum sync: istek gönderilemedi: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var resp forumSyncYanit
	if err := json.NewDecoder(io.LimitReader(st, maxForumSyncYanitBayt)).Decode(&resp); err != nil {
		return fmt.Errorf("forum sync: yanıt okunamadı: %w", err)
	}

	for _, p := range resp.Posts {
		if p == nil || !verifyPost(p) {
			continue
		}
		_ = depo.Ekle(AramaSonucu{
			Tip: TipKonu, KaynakID: p.ID, Kategori: p.Category, Baslik: p.Title,
			Govde: p.Body, YazarAd: p.AuthorName, YazarKey: p.AuthorKey, OlusturmaTS: p.CreatedAt,
			Sig: p.Sig,
		})
	}
	for _, rs := range resp.Replies {
		for _, r := range rs {
			if r == nil || !verifyReply(r) {
				continue
			}
			_ = depo.Ekle(AramaSonucu{
				Tip: TipYanit, KaynakID: r.ID, UstID: r.PostID,
				Govde: r.Body, YazarAd: r.AuthorName, YazarKey: r.AuthorKey, OlusturmaTS: r.CreatedAt,
				Sig: r.Sig,
			})
		}
	}
	for _, tsJSON := range resp.Tombstones {
		var d deleteMsg
		if json.Unmarshal([]byte(tsJSON), &d) == nil && verifyDelete(&d) {
			_ = depo.Sil(d.PostID, TipKonu)
		}
	}
	return nil
}
