package main

// site.go — AND'ın kişisel site protokolünün BAĞIMSIZ bir yeniden
// uygulaması (bkz. AND/internal/site/site.go + sync.go, bu dosya onların
// bire bir karşılığıdır — forum.go/chat.go'nun izlediği AYNI desen):
// aynı Page/DeletePage/envelope JSON biçimi, aynı imza-payload'ı, aynı
// GossipSub konusu (siteTopicAdi) ve aynı senkronizasyon protokolü
// (/and/sitesync/1.0.0). Yalnızca CLIENT tarafı — bu program kendi
// sayfasını yayınlamaz, başka eşlerin sync isteklerine yanıt vermez.
//
// Forum post'larının/sohbet mesajlarının aksine, bir site sayfası
// DEĞİŞEBİLİR: aynı (AuthorKey, Slug) ikilisi yeniden yayınlanınca
// TAMAMEN değişir (bkz. AND'ın PublishPage'inin "slug bir kez seçilir,
// sonraki yayınlar aynı slug'ı TAMAMEN değiştirir" sözleşmesi). Bu yüzden
// burada depo.Ekle (yaz-bir-kez) değil, depo.EkleYaGuncelle (en-yeni-
// kazanır upsert) kullanılır — bkz. index.go'daki ikisinin doc yorumu.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/protokol"
)

const siteSyncProtokolu = protokol.SiteSenkronProtokolu
const siteSyncZamanAsimi = 30 * time.Second
const maxSiteSyncYanitBayt = 32 * 1024 * 1024

// SitePage, AND'ın site.Page'iyle bire bir aynı JSON alanlarına sahiptir.
type SitePage struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
	Sig       string    `json:"sig"`
}

type siteDeleteMsg struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	DeletedAt time.Time `json:"deleted_at"`
	Sig       string    `json:"sig"`
}

type siteZarf struct {
	Type   string         `json:"type"`
	Page   *SitePage      `json:"page,omitempty"`
	Delete *siteDeleteMsg `json:"delete,omitempty"`
}

// sitePagePayload/siteDeletePayload, AND'ın site.go'sundaki
// pageSignable/deletePageSignable'ın bire bir transkripsiyonudur —
// alan adları/JSON etiketleri/sırası eşleşmezse imza doğrulaması AND'ın
// ürettiği gerçek imzalarla asla tutmaz.
type sitePageSignable struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

func sitePagePayload(p *SitePage) []byte {
	data, _ := json.Marshal(sitePageSignable{
		AuthorKey: p.AuthorKey, Slug: p.Slug, Title: p.Title, Body: p.Body, UpdatedAt: p.UpdatedAt,
	})
	return data
}

func verifySitePage(p *SitePage) bool { return verifyEd25519(p.AuthorKey, p.Sig, sitePagePayload(p)) }

type siteDeleteSignable struct {
	AuthorKey string    `json:"author_key"`
	Slug      string    `json:"slug"`
	DeletedAt time.Time `json:"deleted_at"`
}

func siteDeletePayload(d *siteDeleteMsg) []byte {
	data, _ := json.Marshal(siteDeleteSignable{AuthorKey: d.AuthorKey, Slug: d.Slug, DeletedAt: d.DeletedAt})
	return data
}

func verifySiteDelete(d *siteDeleteMsg) bool {
	return verifyEd25519(d.AuthorKey, d.Sig, siteDeletePayload(d))
}

// siteKaynakID, bir site sayfasının bilinen/arama_fts tablolarındaki
// BENZERSİZ anahtarıdır. Forum post/yanıt/sohbet ID'lerinin aksine
// (rastgele UUID), bir sayfa (AuthorKey, Slug) ikilisiyle adreslenir; bu
// ikisini AND'ın kendi site.go'sundaki BİREBİR AYNI "authorKey+"/"+slug"
// biçimiyle birleştiriyoruz (bkz. AND/internal/site/site.go'nun deleted
// map anahtarı) — tesadüfen benzer değil, kasıtlı olarak aynı biçim.
func siteKaynakID(authorKey, slug string) string { return authorKey + "/" + slug }

// siteKaynakIDCoz, siteKaynakID'nin tersini yapar — sync isteğinin Known
// alanı (yazar_key -> slug listesi) için yerel düz ID listesini yazara
// göre gruplamak amacıyla kullanılır. AuthorKey hex olduğundan ve slug
// asla "/" içermediğinden (bkz. AND'ın site.ValidSlug'ı), İLK "/"'den
// bölmek belirsizliksizdir.
func siteKaynakIDCoz(id string) (authorKey, slug string, ok bool) {
	i := strings.IndexByte(id, '/')
	if i < 0 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

// siteMesajIsle, gossip'ten (ya da sync yanıtından) gelen ham bir site
// zarfını doğrulayıp depoya uygular.
func siteMesajIsle(depo *Depo, data []byte) {
	var z siteZarf
	if err := json.Unmarshal(data, &z); err != nil {
		return
	}
	switch z.Type {
	case "page":
		if z.Page == nil {
			return
		}
		if !verifySitePage(z.Page) {
			gunlukUyari("geçersiz imzalı site sayfası reddedildi:", z.Page.AuthorKey, z.Page.Slug)
			return
		}
		if err := depo.EkleYaGuncelle(AramaSonucu{
			Tip: TipSite, KaynakID: siteKaynakID(z.Page.AuthorKey, z.Page.Slug),
			Baslik: z.Page.Title, Govde: z.Page.Body, YazarKey: z.Page.AuthorKey,
			OlusturmaTS: z.Page.UpdatedAt,
			Sig:         z.Page.Sig,
		}); err != nil {
			gunlukHata("site sayfası indekslenemedi:", z.Page.AuthorKey, z.Page.Slug, err)
		}
	case "delete":
		if z.Delete == nil {
			return
		}
		if !verifySiteDelete(z.Delete) {
			gunlukUyari("geçersiz imzalı site silme isteği reddedildi:", z.Delete.AuthorKey, z.Delete.Slug)
			return
		}
		_ = depo.Sil(siteKaynakID(z.Delete.AuthorKey, z.Delete.Slug), TipSite)
	}
}

// ─── Senkronizasyon (yalnızca istemci) ─────────────────────────────────────────

type siteSyncIstek struct {
	// Known: yazar_key -> zaten sahip olunan AKTİF slug listesi (tombstone
	//'lanmışlar HARİÇ — bkz. BilinenAktifVeSilinenIDler'in doc yorumu).
	Known map[string][]string `json:"known"`
	// KnownVersions: yazar_key -> slug -> elimizdeki sürümün UpdatedAt'i
	// (UnixNano) — AND'ın site/sync.go'sundaki syncRequest.KnownVersions'ın
	// bire bir karşılığı. Bu alan olmadan yanıtlayan, Known'da listelenen
	// bir slug'ı DAHA YENİ bir sürümü olsa bile hiç göndermez — yani bir
	// sayfanın DÜZENLENMİŞ hali bu düğüme senkronizasyonla asla ulaşmazdı.
	// Bu alanı tanımayan eski bir AND sürümü onu yok sayar (eski davranış).
	KnownVersions map[string]map[string]int64 `json:"known_versions,omitempty"`
	// KnownTombstones: zaten bilinen "yazar_key/slug" tombstone anahtarları.
	KnownTombstones []string `json:"known_tombstones,omitempty"`
}

// andSiteSyncIstekSiniri, AND'ın site/sync.go'sundaki maxSyncRequestBytes'ın
// karşılığıdır: bir AND düğümü bundan uzun bir sync isteğini okuyamaz ve
// TÜM senkronizasyon başarısız olur.
const andSiteSyncIstekSiniri = 1 * 1024 * 1024

// siteSyncIstekKodla, istek'i JSON'a çevirir; sonuç sinir'ı aşıyorsa
// KnownVersions'ı düşürüp yeniden kodlar — AND'ın encodeSyncRequest'iyle
// aynı gerekçe: çok büyük bir ağda yalnızca düzenleme yakalamayı kaybetmek,
// isteğin tamamen reddedilip YENİ sayfaları da kaybetmekten iyidir.
func siteSyncIstekKodla(istek siteSyncIstek, sinir int) ([]byte, error) {
	veri, err := json.Marshal(istek)
	if err != nil || len(veri) <= sinir || istek.KnownVersions == nil {
		return veri, err
	}
	istek.KnownVersions = nil
	return json.Marshal(istek)
}

// unixNanoGuvenli, t'yi KnownVersions için UnixNano'ya çevirir; t int64
// nanosaniye aralığının dışındaysa (1678 öncesi / 2262 sonrası — UnixNano
// orada tanımsızdır, ör. sıfır time.Time) ok=false döner ve o sayfa için
// sürüm hiç gönderilmez (AND'ın versionOf'uyla aynı kural).
func unixNanoGuvenli(t time.Time) (int64, bool) {
	if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
		return 0, false
	}
	return t.UnixNano(), true
}

type siteSyncYanit struct {
	Pages      []*SitePage      `json:"pages"`
	Tombstones []*siteDeleteMsg `json:"tombstones,omitempty"`
}

// siteIleSenkronizeOl, peerID'ye bir site sync stream'i açar, bilinen
// aktif slug'larımızı (sürümleriyle) ve tombstone'larımızı gönderir, eksik
// olan ya da elimizdekinden YENİ sürümlü sayfaları ve tombstone'ları alıp
// doğrulayıp indeksler.
func siteIleSenkronizeOl(ctx context.Context, h libp2phost.Host, peerID peer.ID, depo *Depo) error {
	ctx, cancel := context.WithTimeout(ctx, siteSyncZamanAsimi)
	defer cancel()

	st, err := h.NewStream(ctx, peerID, siteSyncProtokolu)
	if err != nil {
		return fmt.Errorf("site sync: stream açılamadı: %w", err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(siteSyncZamanAsimi)) //nolint:errcheck

	aktifIDler, silinenIDler, err := depo.BilinenAktifVeSilinenIDler(TipSite)
	if err != nil {
		return err
	}
	surumler, err := depo.SiteSurumleriAl()
	if err != nil {
		return err
	}
	known := make(map[string][]string, len(aktifIDler))
	knownVersions := make(map[string]map[string]int64, len(aktifIDler))
	for _, id := range aktifIDler {
		authorKey, slug, ok := siteKaynakIDCoz(id)
		if !ok {
			continue
		}
		known[authorKey] = append(known[authorKey], slug)
		if v, ok := surumler[id]; ok {
			if knownVersions[authorKey] == nil {
				knownVersions[authorKey] = make(map[string]int64)
			}
			knownVersions[authorKey][slug] = v
		}
	}

	istekVerisi, err := siteSyncIstekKodla(siteSyncIstek{Known: known, KnownVersions: knownVersions, KnownTombstones: silinenIDler}, andSiteSyncIstekSiniri)
	if err != nil {
		return fmt.Errorf("site sync: istek hazırlanamadı: %w", err)
	}
	if _, err := st.Write(istekVerisi); err != nil {
		return fmt.Errorf("site sync: istek gönderilemedi: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var resp siteSyncYanit
	if err := json.NewDecoder(io.LimitReader(st, maxSiteSyncYanitBayt)).Decode(&resp); err != nil {
		return fmt.Errorf("site sync: yanıt okunamadı: %w", err)
	}

	for _, p := range resp.Pages {
		if p == nil || !verifySitePage(p) {
			continue
		}
		_ = depo.EkleYaGuncelle(AramaSonucu{
			Tip: TipSite, KaynakID: siteKaynakID(p.AuthorKey, p.Slug),
			Baslik: p.Title, Govde: p.Body, YazarKey: p.AuthorKey, OlusturmaTS: p.UpdatedAt,
			Sig: p.Sig,
		})
	}
	for _, d := range resp.Tombstones {
		if d == nil || !verifySiteDelete(d) {
			continue
		}
		_ = depo.Sil(siteKaynakID(d.AuthorKey, d.Slug), TipSite)
	}
	return nil
}
