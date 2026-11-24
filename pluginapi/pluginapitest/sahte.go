// Package pluginapitest, bellekte çalışan sahte bir AND'dır. Eklenti
// yazarları eklentilerini AND'ı açmadan, ağa bağlanmadan ve gerçek bir
// kimlik kullanmadan dener ve test eder:
//
//	func TestKonuAcar(t *testing.T) {
//		and := pluginapitest.Yeni()
//		istemci := and.Test(t, pluginapi.IzinForum) // yalnızca manifest'teki izinler
//		// ... eklentinin kodu istemciyle (ya da NewClientFromEnv ile) çalışır
//		if len(and.Konular()) != 1 { t.Fatal("konu açılmadı") }
//	}
//
// Sahte AND, gerçek AND'ın HTTP sunucusunun (pluginapi.Server) kendisini
// çalıştırır; yalnızca arkasındaki forum, sohbet, özel mesaj, site ve dosya
// katmanları bellektedir. Bu yüzden uç noktalar, izin denetimi (verilmeyen
// izin 403), hız sınırları ve girdi doğrulaması gerçek AND'daki gibidir:
// manifest'ine yazmayı unuttuğun bir izin testte de 403 döner.
package pluginapitest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/protokol"
)

// Kimlik, sahte AND'daki kullanıcıdır.
type Kimlik struct {
	Ad        string
	PeerID    string
	PubKey    string // 64 haneli onaltılık Ed25519 açık anahtarı
	Kurucu    bool
	Moderator bool
}

// VarsayilanKimlik, Yeni'nin kullandığı kimliktir.
var VarsayilanKimlik = Kimlik{
	Ad:     "deneme",
	PeerID: "12D3KooWDenemeDugumu1111111111111111111111111111",
	PubKey: "00000000000000000000000000000000000000000000000000000000000000aa",
}

// GidenDosya, eklentinin gönderdiği bir dosyadır (dosya okunmaz, yalnızca
// isteğin kendisi kaydedilir).
type GidenDosya struct {
	PeerID string
	Yol    string
}

// Sahte, bellekte çalışan sahte bir AND'dır. Yöntemleri eşzamanlı
// çağrılabilir: eklenti sunucuya istek atarken test veri ekleyebilir ya da
// sonuçları okuyabilir.
type Sahte struct {
	mu            sync.Mutex
	kimlik        Kimlik
	esler         []pluginapi.PeerInfo
	konular       []pluginapi.PostDetail
	sohbet        []pluginapi.ChatMsg
	sayfalar      []pluginapi.SitePageInfo
	dmGecmisi     []pluginapi.DMRecord
	okundu        map[string]time.Time
	gidenDosyalar []GidenDosya
	onaylar       map[string]bool
	temalar       []pluginapi.ThemeReq
	yasaklar      []pluginapi.BanEntryInfo
	gecmis        []pluginapi.HistoryEntryInfo
	moderatorler  []pluginapi.GrantedModInfo
	sayac         int
	veriDizini    string

	dmAbone     map[chan pluginapi.DMMsg]struct{}
	sohbetAbone map[chan pluginapi.ChatMsg]struct{}
	dosyaAbone  map[chan pluginapi.FileMsg]struct{}
	onayAbone   map[chan pluginapi.FileConsentReq]struct{}
}

// Yeni, boş bir sahte AND'dır: VarsayilanKimlik, eş yok, içerik yok.
func Yeni() *Sahte {
	return &Sahte{
		kimlik:      VarsayilanKimlik,
		okundu:      map[string]time.Time{},
		onaylar:     map[string]bool{},
		dmAbone:     map[chan pluginapi.DMMsg]struct{}{},
		sohbetAbone: map[chan pluginapi.ChatMsg]struct{}{},
		dosyaAbone:  map[chan pluginapi.FileMsg]struct{}{},
		onayAbone:   map[chan pluginapi.FileConsentReq]struct{}{},
	}
}

// Ornek, sahte AND'ı birkaç eş, forum konusu, sohbet mesajı ve özel mesajla
// doldurur; "go run . --deneme" gibi elle denemeler içindir.
func (s *Sahte) Ornek() *Sahte {
	ayse := "12D3KooWAyseKardesDugum2222222222222222222222222"
	mert := "12D3KooWMertKardesDugum3333333333333333333333333"
	s.EsEkle(ayse, true)
	s.EsEkle(mert, true)
	s.EsEkle("12D3KooWGenisDHTDugumu4444444444444444444444444", false)
	id := s.KonuEkle("Genel", "AND'a hoş geldin", "Bu konu sahte AND'ın örnek verisidir.", "ayse")
	s.YanitEkle(id, "Eklentin bu konuyu okuyabiliyor.", "mert")
	s.KonuEkle("Yazılım", "İlk eklentim", "and eklenti yeni ile kuruldu.", "mert")
	s.SohbetEkle("ayse", "merhaba!")
	s.SohbetEkle("mert", "eklenti nasıl gidiyor?")
	s.DMGelsin(ayse, "ayse", "Selam, eklentini denedim.")
	return s
}

// KimlikAyarla, sahte AND'ın kullanıcısını değiştirir.
func (s *Sahte) KimlikAyarla(k Kimlik) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kimlik = k
}

// EsEkle, bağlı bir eş ekler; and, eşin AND düğümü olup olmadığıdır (özel
// mesaj ve dosya yalnızca AND düğümlerine gönderilebilir).
func (s *Sahte) EsEkle(peerID string, and bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.esler = append(s.esler, pluginapi.PeerInfo{PeerID: peerID, Addrs: []string{"/ip4/127.0.0.1/tcp/4001"}, AND: and})
}

// KonuEkle, başka birinin açtığı bir forum konusunu ekler ve kimliğini
// döndürür.
func (s *Sahte) KonuEkle(kategori, baslik, govde, yazar string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.konuEkle(kategori, baslik, govde, yazar, yazarAnahtari(yazar))
}

func (s *Sahte) konuEkle(kategori, baslik, govde, yazar, anahtar string) string {
	id := s.yeniKimlik()
	s.konular = append(s.konular, pluginapi.PostDetail{
		ID: id, Category: kategori, Title: baslik, Body: govde,
		AuthorName: yazar, AuthorKey: anahtar, CreatedAt: time.Now().UTC(),
		Replies: []pluginapi.ReplyInfo{},
	})
	return id
}

// YanitEkle, bir konuya başka birinin yanıtını ekler.
func (s *Sahte) YanitEkle(konuID, govde, yazar string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.yanitEkle(konuID, govde, yazar, yazarAnahtari(yazar))
}

func (s *Sahte) yanitEkle(konuID, govde, yazar, anahtar string) error {
	for i := range s.konular {
		if s.konular[i].ID == konuID {
			s.konular[i].Replies = append(s.konular[i].Replies, pluginapi.ReplyInfo{
				ID: s.yeniKimlik(), AuthorName: yazar, AuthorKey: anahtar, Body: govde, CreatedAt: time.Now().UTC(),
			})
			return nil
		}
	}
	return errors.New("konu bulunamadı")
}

// SohbetEkle, genel sohbete başka birinin mesajını ekler; o an dinleyen
// eklentiye (PollChat) de iletilir.
func (s *Sahte) SohbetEkle(kimden, metin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sohbetEkle(kimden, metin)
}

func (s *Sahte) sohbetEkle(kimden, metin string) {
	m := pluginapi.ChatMsg{ID: s.yeniKimlik(), From: kimden, Text: metin, SentAt: time.Now().UTC()}
	s.sohbet = append(s.sohbet, m)
	canli := m
	canli.ID = "" // canlı akış kimlik taşımaz (gerçek AND'daki gibi)
	for ch := range s.sohbetAbone {
		select {
		case ch <- canli:
		default:
		}
	}
}

// DMGelsin, peerID'den gelen bir özel mesajı canlandırır: geçmişe yazılır ve
// o an dinleyen eklentiye (PollDM) iletilir.
func (s *Sahte) DMGelsin(peerID, ad, metin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	simdi := time.Now().UTC()
	s.dmGecmisi = append(s.dmGecmisi, pluginapi.DMRecord{ID: s.yeniKimlik(), PeerID: peerID, Name: ad, Text: metin, At: simdi})
	m := pluginapi.DMMsg{From: ad, VerifiedPeerID: peerID, Text: metin, ReceivedAt: simdi}
	for ch := range s.dmAbone {
		select {
		case ch <- m:
		default:
		}
	}
}

// DosyaGelsin, gönderilmek istenen bir dosyanın onay isteğini canlandırır
// (PollConsent); transferID ile RespondConsent'in yanıtı OnayYaniti'ndan
// okunur.
func (s *Sahte) DosyaGelsin(transferID, gonderen, dosyaAdi string, boyut int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := pluginapi.FileConsentReq{TransferID: transferID, SenderID: gonderen, Filename: dosyaAdi, Size: boyut}
	for ch := range s.onayAbone {
		select {
		case ch <- r:
		default:
		}
	}
}

// Konular, forumdaki konulardır (eklentinin açtıkları dahil).
func (s *Sahte) Konular() []pluginapi.PostDetail {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]pluginapi.PostDetail, len(s.konular))
	for i, k := range s.konular {
		k.Replies = append([]pluginapi.ReplyInfo(nil), k.Replies...)
		out[i] = k
	}
	return out
}

// SohbetGecmisi, genel sohbetteki mesajlardır (eklentinin yazdıkları dahil).
func (s *Sahte) SohbetGecmisi() []pluginapi.ChatMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pluginapi.ChatMsg(nil), s.sohbet...)
}

// GidenDMler, eklentinin gönderdiği özel mesajlardır.
func (s *Sahte) GidenDMler() []pluginapi.DMRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []pluginapi.DMRecord
	for _, r := range s.dmGecmisi {
		if r.Outgoing {
			out = append(out, r)
		}
	}
	return out
}

// SiteSayfalari, kullanıcının kişisel sitesindeki sayfalardır.
func (s *Sahte) SiteSayfalari() []pluginapi.SitePageInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pluginapi.SitePageInfo(nil), s.sayfalar...)
}

// GidenDosyalar, eklentinin gönderdiği dosyalardır.
func (s *Sahte) GidenDosyalar() []GidenDosya {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]GidenDosya(nil), s.gidenDosyalar...)
}

// OnayYaniti, eklentinin transferID'li dosya isteğine verdiği yanıttır;
// yanıt yoksa ok false'tur.
func (s *Sahte) OnayYaniti(transferID string) (kabul, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kabul, ok = s.onaylar[transferID]
	return kabul, ok
}

// TemaIstekleri, eklentinin AND'ın temasını değiştirme istekleridir.
func (s *Sahte) TemaIstekleri() []pluginapi.ThemeReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pluginapi.ThemeReq(nil), s.temalar...)
}

// yeniKimlik, konu, yanıt ve mesajlar için onaltılık bir kimliktir (AND'ın
// uç noktaları onaltılık kimlik bekler). mu tutulurken çağrılır.
func (s *Sahte) yeniKimlik() string {
	s.sayac++
	return fmt.Sprintf("%032x", s.sayac)
}

// yazarAnahtari, sahte bir yazarın 64 haneli anahtarıdır; aynı ad hep aynı
// anahtarı verir.
func yazarAnahtari(ad string) string {
	var h uint64 = 1469598103934665603
	for _, c := range []byte(ad) {
		h = (h ^ uint64(c)) * 1099511628211
	}
	return fmt.Sprintf("%064x", h)
}

// Sunucu, çalışan bir sahte AND'dır.
type Sunucu struct {
	// Istemci, eklentiye verilen izinlerle konuşan istemcidir.
	Istemci *pluginapi.Client
	// Adres ve Anahtar, AND_API_ADDR ve AND_API_TOKEN değerleridir.
	Adres, Anahtar string
	// VeriDizini, AND_DATA_DIR değeridir.
	VeriDizini string
	iptal      context.CancelFunc
}

// Ortam, eklentiyi bu sahte AND'a bağlayan ortam değişkenleridir (AND bir
// eklentiyi başlatırken aynılarını verir).
func (sn *Sunucu) Ortam() []string {
	return []string{
		"AND_API_ADDR=" + sn.Adres,
		"AND_API_TOKEN=" + sn.Anahtar,
		"AND_DATA_DIR=" + sn.VeriDizini,
	}
}

// Kapat, sunucuyu durdurur.
func (sn *Sunucu) Kapat() { sn.iptal() }

// Baslat, sahte AND'ın API sunucusunu 127.0.0.1'de başlatır. Eklentiye
// verilen anahtar yalnızca izinler'i taşır (gerçek AND'da kullanıcının
// onayladığı izinler gibi). veriDizini AND_DATA_DIR olur; tema orada
// theme.json'a yazılır.
func (s *Sahte) Baslat(veriDizini string, izinler ...string) (*Sunucu, error) {
	if veriDizini == "" {
		return nil, errors.New("pluginapitest: veri dizini gerekli")
	}
	s.mu.Lock()
	s.veriDizini = veriDizini
	s.mu.Unlock()

	ctx, iptal := context.WithCancel(context.Background())
	temaCh := make(chan pluginapi.ThemeReq, 1)
	srv := pluginapi.NewServer(kimlikArka{s}, forumArka{s}, siteArka{s}, dmArka{s}, dosyaArka{s}, sohbetArka{s}, modArka{s}, veriDizini, temaCh)
	srv.HariciKapat()
	adres, _, err := srv.Start(ctx)
	if err != nil {
		iptal()
		return nil, err
	}
	anahtar, geriAl, err := srv.EklentiAnahtari("pluginapitest", izinler)
	if err != nil {
		iptal()
		return nil, err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				geriAl()
				return
			case t := <-temaCh:
				s.temaUygula(t)
			}
		}
	}()
	return &Sunucu{Istemci: pluginapi.NewClient(adres, anahtar), Adres: adres, Anahtar: anahtar, VeriDizini: veriDizini, iptal: iptal}, nil
}

// Test, sahte AND'ı test süresince başlatır, eklentinin NewClientFromEnv,
// DataDir ve dil.OrtamdanAyarla ile okuduğu ortam değişkenlerini ayarlar ve
// izinler'i taşıyan istemciyi döndürür. Test bitince sunucu kapanır.
func (s *Sahte) Test(t testing.TB, izinler ...string) *pluginapi.Client {
	t.Helper()
	sn, err := s.Baslat(t.TempDir(), izinler...)
	if err != nil {
		t.Fatalf("pluginapitest: sahte AND başlatılamadı: %v", err)
	}
	t.Cleanup(sn.Kapat)
	t.Setenv("AND_API_ADDR", sn.Adres)
	t.Setenv("AND_API_TOKEN", sn.Anahtar)
	t.Setenv("AND_DATA_DIR", sn.VeriDizini)
	return sn.Istemci
}

// temaUygula, gerçek AND gibi temayı theme.json'a yazar: GET /api/v1/theme
// onu okur.
func (s *Sahte) temaUygula(t pluginapi.ThemeReq) {
	s.mu.Lock()
	s.temalar = append(s.temalar, t)
	dizin := s.veriDizini
	s.mu.Unlock()
	if veri, err := json.Marshal(t); err == nil && dizin != "" {
		_ = os.WriteFile(filepath.Join(dizin, pluginapi.ThemeFile), veri, 0o600)
	}
}

// ─── pluginapi.Server'ın arka uçları ─────────────────────────────────────

type kimlikArka struct{ s *Sahte }

func (a kimlikArka) k() Kimlik {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return a.s.kimlik
}
func (a kimlikArka) Name() string      { return a.k().Ad }
func (a kimlikArka) PubKeyHex() string { return a.k().PubKey }
func (a kimlikArka) PeerIDStr() string { return a.k().PeerID }
func (a kimlikArka) IsFounder() bool   { return a.k().Kurucu }
func (a kimlikArka) IsModerator() bool { return a.k().Kurucu || a.k().Moderator }
func (a kimlikArka) ConnectedPeers() []pluginapi.PeerInfo {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return append([]pluginapi.PeerInfo(nil), a.s.esler...)
}

type forumArka struct{ s *Sahte }

func (a forumArka) AllPosts() ([]pluginapi.PostSummary, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	out := make([]pluginapi.PostSummary, 0, len(a.s.konular))
	for _, k := range a.s.konular {
		out = append(out, pluginapi.PostSummary{ID: k.ID, Category: k.Category, Title: k.Title, AuthorName: k.AuthorName, AuthorKey: k.AuthorKey})
	}
	return out, nil
}

func (a forumArka) DeletePost(_ context.Context, postID, _ string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	for i, k := range a.s.konular {
		if k.ID == postID {
			a.s.konular = append(a.s.konular[:i], a.s.konular[i+1:]...)
			return nil
		}
	}
	return errors.New("konu bulunamadı")
}

func (a forumArka) CreatePost(_ context.Context, kategori, baslik, govde string) error {
	// Gerçek AND'da bilinmeyen kategorideki konuyu eşler reddeder; eklenti
	// eski bir kategori adı gönderiyorsa bunu testte görsün.
	if !protokol.GecerliKategori(kategori) {
		return fmt.Errorf("bilinmeyen kategori: %q (bkz. protokol.Kategoriler)", kategori)
	}
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.konuEkle(kategori, baslik, govde, a.s.kimlik.Ad, a.s.kimlik.PubKey)
	return nil
}

func (a forumArka) GetPost(id string) (*pluginapi.PostDetail, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	for _, k := range a.s.konular {
		if k.ID == id {
			k.Replies = append([]pluginapi.ReplyInfo{}, k.Replies...)
			return &k, nil
		}
	}
	return nil, nil
}

func (a forumArka) CreateReply(_ context.Context, postID, govde string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return a.s.yanitEkle(postID, govde, a.s.kimlik.Ad, a.s.kimlik.PubKey)
}

type siteArka struct{ s *Sahte }

func (a siteArka) MyPages() ([]pluginapi.SitePageInfo, error) {
	return a.s.SiteSayfalari(), nil
}

func (a siteArka) PublishPage(_ context.Context, slug, baslik, govde string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	sayfa := pluginapi.SitePageInfo{Slug: slug, Title: baslik, Body: govde, UpdatedAt: time.Now().UTC()}
	for i := range a.s.sayfalar {
		if a.s.sayfalar[i].Slug == slug {
			a.s.sayfalar[i] = sayfa
			return nil
		}
	}
	a.s.sayfalar = append(a.s.sayfalar, sayfa)
	sort.Slice(a.s.sayfalar, func(i, j int) bool { return a.s.sayfalar[i].Slug < a.s.sayfalar[j].Slug })
	return nil
}

func (a siteArka) DeletePage(_ context.Context, slug string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	for i := range a.s.sayfalar {
		if a.s.sayfalar[i].Slug == slug {
			a.s.sayfalar = append(a.s.sayfalar[:i], a.s.sayfalar[i+1:]...)
			return nil
		}
	}
	return errors.New("sayfa bulunamadı")
}

type dmArka struct{ s *Sahte }

func (a dmArka) SendDM(_ context.Context, peerID, _, mesaj string) error {
	return a.gonder(peerID, mesaj, false)
}

func (a dmArka) SendDMQueued(_ context.Context, peerID, _, mesaj string) error {
	return a.gonder(peerID, mesaj, true)
}

func (a dmArka) gonder(peerID, mesaj string, kuyruk bool) error {
	if !pluginapi.GecerliPeerID(peerID) {
		return fmt.Errorf("geçersiz Peer ID: %q", peerID)
	}
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	bagli := false
	for _, e := range a.s.esler {
		if e.PeerID == peerID && e.AND {
			bagli = true
		}
	}
	if !bagli && !kuyruk {
		return fmt.Errorf("%s bağlı bir AND düğümü değil", peerID)
	}
	a.s.dmGecmisi = append(a.s.dmGecmisi, pluginapi.DMRecord{
		ID: a.s.yeniKimlik(), PeerID: peerID, Text: mesaj, At: time.Now().UTC(), Outgoing: true, Pending: !bagli,
	})
	return nil
}

func (a dmArka) Subscribe() chan pluginapi.DMMsg {
	ch := make(chan pluginapi.DMMsg, 32)
	a.s.mu.Lock()
	a.s.dmAbone[ch] = struct{}{}
	a.s.mu.Unlock()
	return ch
}

func (a dmArka) Unsubscribe(ch chan pluginapi.DMMsg) {
	a.s.mu.Lock()
	delete(a.s.dmAbone, ch)
	a.s.mu.Unlock()
}

func (a dmArka) History() []pluginapi.DMRecord {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return append([]pluginapi.DMRecord(nil), a.s.dmGecmisi...)
}

func (a dmArka) MarkRead(peerID string, at time.Time) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if at.After(a.s.okundu[peerID]) {
		a.s.okundu[peerID] = at
	}
}

func (a dmArka) ReadMarks() map[string]time.Time {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	out := make(map[string]time.Time, len(a.s.okundu))
	for k, v := range a.s.okundu {
		out[k] = v
	}
	return out
}

type dosyaArka struct{ s *Sahte }

func (a dosyaArka) SendFile(_ context.Context, peerID, _, yol string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.gidenDosyalar = append(a.s.gidenDosyalar, GidenDosya{PeerID: peerID, Yol: yol})
	return nil
}

func (a dosyaArka) Subscribe() chan pluginapi.FileMsg {
	ch := make(chan pluginapi.FileMsg, 8)
	a.s.mu.Lock()
	a.s.dosyaAbone[ch] = struct{}{}
	a.s.mu.Unlock()
	return ch
}

func (a dosyaArka) Unsubscribe(ch chan pluginapi.FileMsg) {
	a.s.mu.Lock()
	delete(a.s.dosyaAbone, ch)
	a.s.mu.Unlock()
}

func (a dosyaArka) SubscribeConsent() chan pluginapi.FileConsentReq {
	ch := make(chan pluginapi.FileConsentReq, 8)
	a.s.mu.Lock()
	a.s.onayAbone[ch] = struct{}{}
	a.s.mu.Unlock()
	return ch
}

func (a dosyaArka) UnsubscribeConsent(ch chan pluginapi.FileConsentReq) {
	a.s.mu.Lock()
	delete(a.s.onayAbone, ch)
	a.s.mu.Unlock()
}

func (a dosyaArka) RespondConsent(transferID string, kabul bool) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.onaylar[transferID] = kabul
}

type sohbetArka struct{ s *Sahte }

func (a sohbetArka) SendChat(_ context.Context, ad, mesaj string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.sohbetEkle(ad, mesaj)
	return nil
}

func (a sohbetArka) Subscribe() chan pluginapi.ChatMsg {
	ch := make(chan pluginapi.ChatMsg, 32)
	a.s.mu.Lock()
	a.s.sohbetAbone[ch] = struct{}{}
	a.s.mu.Unlock()
	return ch
}

func (a sohbetArka) Unsubscribe(ch chan pluginapi.ChatMsg) {
	a.s.mu.Lock()
	delete(a.s.sohbetAbone, ch)
	a.s.mu.Unlock()
}

func (a sohbetArka) AllChat() []pluginapi.ChatMsg { return a.s.SohbetGecmisi() }

// modArka, moderasyon uçlarının bellekteki karşılığıdır: yasaklar, geçmiş
// ve moderatör atamaları tutulur (ağa bir şey yayınlanmaz). Kimin neye
// yetkisi olduğunu gerçek AND gibi kimliğin Kurucu/Moderator alanları
// belirler (sunucu denetler); kendini yasaklamak reddedilir.
type modArka struct{ s *Sahte }

func (a modArka) AllBans() ([]pluginapi.BanEntryInfo, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return slices.Clone(a.s.yasaklar), nil
}

func (a modArka) Ban(_ context.Context, anahtar string, sure time.Duration, kalici bool, sebep string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if strings.EqualFold(anahtar, a.s.kimlik.PubKey) {
		return fmt.Errorf("%w: kendini yasaklayamazsın", pluginapi.ErrYetkisiz)
	}
	y := pluginapi.BanEntryInfo{AuthorKey: anahtar, Permanent: kalici, Reason: sebep, ModeratorKey: a.s.kimlik.PubKey,
		IsFounderMod: a.s.kimlik.Kurucu}
	if !kalici {
		y.ExpiresAt = time.Now().Add(sure)
	}
	a.s.yasaklar = append(slices.DeleteFunc(a.s.yasaklar, func(b pluginapi.BanEntryInfo) bool { return b.AuthorKey == anahtar }), y)
	a.s.gecmis = append([]pluginapi.HistoryEntryInfo{{Action: "ban", PeerID: anahtar, Reason: sebep, ModeratorKey: y.ModeratorKey,
		IsFounderMod: y.IsFounderMod, At: time.Now(), ExpiresAt: y.ExpiresAt, Permanent: kalici}}, a.s.gecmis...)
	return nil
}

func (a modArka) Unban(_ context.Context, anahtar string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.yasaklar = slices.DeleteFunc(a.s.yasaklar, func(b pluginapi.BanEntryInfo) bool { return b.AuthorKey == anahtar })
	a.s.gecmis = append([]pluginapi.HistoryEntryInfo{{Action: "unban", PeerID: anahtar, ModeratorKey: a.s.kimlik.PubKey,
		IsFounderMod: a.s.kimlik.Kurucu, At: time.Now()}}, a.s.gecmis...)
	return nil
}

func (a modArka) History() ([]pluginapi.HistoryEntryInfo, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return slices.Clone(a.s.gecmis), nil
}

func (a modArka) GrantedMods() ([]pluginapi.GrantedModInfo, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return slices.Clone(a.s.moderatorler), nil
}

func (a modArka) Grant(_ context.Context, anahtar string, sure time.Duration, kalici bool) (string, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	g := pluginapi.GrantedModInfo{Key: anahtar, GrantedAt: time.Now(), Permanent: kalici}
	if !kalici {
		g.ExpiresAt = g.GrantedAt.Add(sure)
	}
	a.s.moderatorler = append(slices.DeleteFunc(a.s.moderatorler, func(m pluginapi.GrantedModInfo) bool { return m.Key == anahtar }), g)
	return "", nil
}

func (a modArka) Revoke(_ context.Context, anahtar string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	for i := range a.s.moderatorler {
		if a.s.moderatorler[i].Key == anahtar {
			a.s.moderatorler[i].Revoked, a.s.moderatorler[i].RevokedAt = true, time.Now()
		}
	}
	return nil
}

// YasakEkle, sahte AND'a hazır bir yasak ekler (ör. Yönetici Paneli testleri).
func (s *Sahte) YasakEkle(y pluginapi.BanEntryInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.yasaklar = append(s.yasaklar, y)
}

// Yasaklar, şu anki yasaklardır.
func (s *Sahte) Yasaklar() []pluginapi.BanEntryInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.yasaklar)
}

// Moderatorler, atanmış moderatörlerdir (geri alınanlar Revoked ile).
func (s *Sahte) Moderatorler() []pluginapi.GrantedModInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.moderatorler)
}
