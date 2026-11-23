package masaustu

// api.go — arayüzün çağırdığı uç noktalar. Metinler sunucuda etkin dile
// çevrilir (terminal arayüzüyle aynı çeviriler); ağdan gelen içerik hiçbir
// zaman HTML olarak değil, düz metin olarak gönderilir ve arayüz onu
// textContent ile yazar.

import (
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/bbcode"
	"github.com/lucian95511/and/internal/chatlog"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/lisans"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/internal/updater"
	"github.com/lucian95511/and/pluginui"
	"github.com/lucian95511/and/protokol"
)

func (u *Uygulama) rotalar() {
	m := u.sv.mux
	// Her aşamada
	m.HandleFunc("GET /api/baslangic", u.baslangic)
	m.HandleFunc("POST /api/dil", u.dilSec)
	m.HandleFunc("GET /api/olaylar", u.yayin.isle)
	m.HandleFunc("POST /api/kapat", u.kapatIstegi)
	m.HandleFunc("POST /api/disari", u.disariAc)
	// Giriş
	m.HandleFunc("POST /api/giris", u.girisAc)
	m.HandleFunc("POST /api/kayit", u.kayitOl)
	m.HandleFunc("GET /api/kayit/kod", u.kayitKodu)
	m.HandleFunc("POST /api/kayit/dogrula", u.kayitDogrula)
	m.HandleFunc("POST /api/kayit/geri", u.kayitGeri)
	m.HandleFunc("POST /api/ad", u.adVer)
	m.HandleFunc("POST /api/sifre-gucu", u.sifreGucu)
	m.HandleFunc("POST /api/geri-yukleme", u.geriYuklemeyeGec)
	m.HandleFunc("POST /api/geri-yukleme/kod", u.geriYuklemeKodu)
	m.HandleFunc("POST /api/geri-yukleme/kaydet", u.geriYukle)
	// Uygulama (çekirdek hazırken)
	m.HandleFunc("GET /api/oturum", u.cekirdekle(u.oturum))
	m.HandleFunc("GET /api/kurallar", u.kurallar)
	m.HandleFunc("POST /api/kurallar", u.kurallariKabulEt)
	m.HandleFunc("GET /api/forum", u.cekirdekle(u.forumOzeti))
	m.HandleFunc("GET /api/forum/konu", u.cekirdekle(u.konuGetir))
	m.HandleFunc("POST /api/forum/konu", u.cekirdekle(u.konuAc))
	m.HandleFunc("POST /api/forum/yanit", u.cekirdekle(u.yanitYaz))
	m.HandleFunc("GET /api/sohbet", u.cekirdekle(u.sohbetGecmisi))
	m.HandleFunc("POST /api/sohbet", u.cekirdekle(u.sohbetGonder))
	m.HandleFunc("GET /api/profil", u.cekirdekle(u.profilGetir))
	m.HandleFunc("POST /api/profil", u.cekirdekle(u.profilKaydet))
	m.HandleFunc("GET /api/temalar", u.temalar)
	m.HandleFunc("POST /api/tema", u.temaSec)
	m.HandleFunc("GET /api/tema/ozel", u.temaOzelOnizle)
	m.HandleFunc("POST /api/tema/ozel", u.temaOzel)
	m.HandleFunc("GET /api/eklentiler", u.cekirdekle(u.eklentiListesi))
	m.HandleFunc("POST /api/eklenti/etkin", u.cekirdekle(u.eklentiEtkin))
	m.HandleFunc("POST /api/eklenti/ac", u.cekirdekle(u.eklentiAc))
	m.HandleFunc("GET /api/ag", u.cekirdekle(u.agDurumu))
	m.HandleFunc("GET /api/hakkinda", u.hakkinda)
}

// cekirdekle, çekirdek henüz kurulmadıysa isteği 503 ile geri çevirir.
func (u *Uygulama) cekirdekle(f func(http.ResponseWriter, *http.Request, *cekirdek)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := u.cek.Load()
		if d == nil {
			hataYaz(w, http.StatusServiceUnavailable, dil.T("AND başlatılıyor…"))
			return
		}
		f(w, r, d)
	}
}

// ─── Her aşamada ────────────────────────────────────────────────────────────

func (u *Uygulama) baslangic(w http.ResponseWriter, _ *http.Request) {
	asama := u.giris.durum()
	if asama == asamaBasliyor && u.cek.Load() != nil {
		asama = "uygulama"
	}
	diller := make([]map[string]string, len(dil.Diller))
	for i, d := range dil.Diller {
		diller[i] = map[string]string{"kod": d.Kod, "ad": d.Ad}
	}
	jsonYaz(w, map[string]any{
		"asama":    asama,
		"dil":      dil.Etkin(),
		"diller":   diller,
		"surum":    updater.Version,
		"metinler": arayuzMetinleri(),
		"tema":     temaRenkleri(tui.TemaOku(u.dir)),
	})
}

func (u *Uygulama) dilSec(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Kod string `json:"kod"`
	}
	if jsonOku(r, &istek) != nil || !dil.Ayarla(istek.Kod) {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if err := dil.Kaydet(u.dir, istek.Kod); err != nil {
		hataYaz(w, http.StatusInternalServerError, dil.T("Dil seçimi kaydedilemedi: ")+err.Error())
		return
	}
	jsonYaz(w, map[string]any{"dil": dil.Etkin(), "metinler": arayuzMetinleri()})
}

func (u *Uygulama) kapatIstegi(w http.ResponseWriter, _ *http.Request) {
	jsonYaz(w, map[string]bool{"tamam": true})
	go u.bitir()
}

// disariAc, içerikteki bir web bağlantısını (kullanıcı onayladıktan sonra)
// varsayılan tarayıcıda açar. Yalnızca http ve https adresleri açılır.
func (u *Uygulama) disariAc(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Adres string `json:"adres"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	a, err := url.Parse(temiz(istek.Adres))
	if err != nil || (a.Scheme != "http" && a.Scheme != "https") || a.Host == "" || a.User != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if err := tarayicidaAc(a.String()); err != nil {
		hataYaz(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// ─── Oturum ve kurallar ─────────────────────────────────────────────────────

func (u *Uygulama) oturum(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	ad := ""
	if d.Kimlik != nil {
		ad = d.Kimlik.Name()
	}
	jsonYaz(w, map[string]any{
		"ad":             ad,
		"anahtar":        d.anahtar,
		"peerID":         d.peerID,
		"rol":            d.rol(d.anahtar),
		"kurucu":         d.Kurucu,
		"surum":          updater.Version,
		"uyari":          d.Uyari,
		"kurallarOnayli": tui.KurallarOnaylandi(u.dir),
		"menu":           d.menuEklentileri(),
		"konuAcabilir":   d.konuAcabilir(),
	})
}

func (u *Uygulama) kurallar(w http.ResponseWriter, _ *http.Request) {
	jsonYaz(w, map[string]any{
		"surum":    tui.KurallarSurumu,
		"giris":    dil.T("AND'ı kullanmadan önce gizlilik özetini ve topluluk kurallarını oku. Kabul ettiğinde bu ekran, kurallar değişene kadar bir daha gösterilmez."),
		"gizlilik": tui.GizlilikMaddeleri(),
		"kurallar": tui.KuralMaddeleri(),
		"not":      dil.T("Bunları istediğin zaman ana menüdeki AND hakkında ekranından yeniden okuyabilirsin."),
		"onayli":   tui.KurallarOnaylandi(u.dir),
	})
}

func (u *Uygulama) kurallariKabulEt(w http.ResponseWriter, _ *http.Request) {
	if err := tui.KurallariOnayla(u.dir); err != nil {
		hataYaz(w, http.StatusInternalServerError, dil.T("Onay kaydedilemedi: ")+err.Error())
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// ─── Forum ──────────────────────────────────────────────────────────────────

type konuOzeti struct {
	ID          string    `json:"id"`
	Kategori    string    `json:"kategori"`
	KategoriAd  string    `json:"kategoriAd"`
	Baslik      string    `json:"baslik"`
	Yazar       string    `json:"yazar"`
	YazarKisa   string    `json:"yazarKisa"`
	Rol         string    `json:"rol,omitempty"`
	Zaman       time.Time `json:"zaman"`
	SonEtkinlik time.Time `json:"sonEtkinlik"`
	Yanit       int       `json:"yanit"`
	Kendi       bool      `json:"kendi,omitempty"`
}

func kisaAnahtar(a string) string {
	if len(a) <= 8 {
		return a
	}
	return a[:8]
}

func (d *cekirdek) konuOzeti(p *forum.Post) konuOzeti {
	yanitlar := d.Forum.Replies(p.ID)
	son := p.CreatedAt
	for _, r := range yanitlar {
		if r.CreatedAt.After(son) {
			son = r.CreatedAt
		}
	}
	return konuOzeti{
		ID: p.ID, Kategori: p.Category, KategoriAd: dil.T(p.Category),
		Baslik: p.Title, Yazar: p.AuthorName, YazarKisa: kisaAnahtar(p.AuthorKey), Rol: d.rol(p.AuthorKey),
		Zaman: p.CreatedAt, SonEtkinlik: son, Yanit: len(yanitlar), Kendi: p.AuthorKey == d.anahtar,
	}
}

// forumOzeti, kategorileri (konu sayılarıyla) ve seçilen kategorinin
// konularını son etkinliğe göre sıralı döner. Kategori boşsa bütün konular.
func (u *Uygulama) forumOzeti(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	if d.Forum == nil {
		hataYaz(w, http.StatusServiceUnavailable, "")
		return
	}
	sayilar := d.Forum.CategoryCounts()
	kategoriler := make([]map[string]any, 0, len(protokol.Kategoriler))
	for _, k := range protokol.Kategoriler {
		kategoriler = append(kategoriler, map[string]any{"ad": k, "etiket": dil.T(k), "sayi": sayilar[k]})
	}
	secili := r.URL.Query().Get("kategori")
	var konular []*forum.Post
	if secili != "" && protokol.GecerliKategori(secili) {
		konular = d.Forum.PostsByCategory(secili)
	} else {
		konular = d.Forum.AllInMemoryPosts()
	}
	ozetler := make([]konuOzeti, 0, len(konular))
	for _, p := range konular {
		ozetler = append(ozetler, d.konuOzeti(p))
	}
	sort.Slice(ozetler, func(i, j int) bool { return ozetler[i].SonEtkinlik.After(ozetler[j].SonEtkinlik) })
	jsonYaz(w, map[string]any{
		"kategoriler":  kategoriler,
		"toplam":       d.Forum.TotalPosts(),
		"konular":      ozetler,
		"konuAcabilir": d.konuAcabilir(),
	})
}

type yanitBilgisi struct {
	ID        string         `json:"id"`
	Yazar     string         `json:"yazar"`
	YazarKisa string         `json:"yazarKisa"`
	Rol       string         `json:"rol,omitempty"`
	Zaman     time.Time      `json:"zaman"`
	Govde     []bbcode.Parca `json:"govde"`
	Kendi     bool           `json:"kendi,omitempty"`
}

func (u *Uygulama) konuGetir(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	if d.Forum == nil {
		hataYaz(w, http.StatusServiceUnavailable, "")
		return
	}
	p := d.Forum.PostByID(r.URL.Query().Get("id"))
	if p == nil {
		hataYaz(w, http.StatusNotFound, dil.T("Konu bulunamadı; silinmiş ya da süresi dolmuş olabilir."))
		return
	}
	yanitlar := d.Forum.Replies(p.ID)
	sort.SliceStable(yanitlar, func(i, j int) bool { return yanitlar[i].CreatedAt.Before(yanitlar[j].CreatedAt) })
	liste := make([]yanitBilgisi, len(yanitlar))
	for i, y := range yanitlar {
		liste[i] = yanitBilgisi{
			ID: y.ID, Yazar: y.AuthorName, YazarKisa: kisaAnahtar(y.AuthorKey), Rol: d.rol(y.AuthorKey),
			Zaman: y.CreatedAt, Govde: bbcode.Parcala(y.Body), Kendi: y.AuthorKey == d.anahtar,
		}
	}
	jsonYaz(w, map[string]any{
		"konu":     d.konuOzeti(p),
		"govde":    bbcode.Parcala(p.Body),
		"yanitlar": liste,
		"sinir":    forum.MaxBodyBytes,
	})
}

// icerikDenetle, bir konu ya da yanıt metnini terminal arayüzünün ve Konu
// Aç eklentisinin yaptığı denetimlerle doğrular; sorun yoksa boş döner.
func icerikDenetle(icerik string, bosHata string) string {
	switch {
	case icerik == "":
		return bosHata
	case !bbcode.Dengeli(icerik):
		return dil.T("Kapanmamış [code] etiketi var, kontrol edin")
	case len(icerik) > forum.MaxBodyBytes:
		return dil.F("Yanıt çok uzun (%d/%d bayt)", len(icerik), forum.MaxBodyBytes)
	}
	return ""
}

const enUzunBaslik = 100 // Konu Aç'ınkiyle aynı (karakter)

func (u *Uygulama) konuAc(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Kategori string `json:"kategori"`
		Baslik   string `json:"baslik"`
		Metin    string `json:"metin"`
	}
	if jsonOku(r, &istek) != nil || d.Forum == nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if !d.konuAcabilir() {
		hataYaz(w, http.StatusForbidden, dil.T("Yeni konu açmak için Konu Aç eklentisini Ayarlar › Eklentiler'den etkinleştir."))
		return
	}
	baslik, metin := temiz(istek.Baslik), temiz(istek.Metin)
	switch {
	case !protokol.GecerliKategori(istek.Kategori):
		hataYaz(w, http.StatusBadRequest, dil.T("Bir kategori seç."))
		return
	case baslik == "":
		hataYaz(w, http.StatusBadRequest, dil.T("Başlık boş olamaz"))
		return
	case len([]rune(baslik)) > enUzunBaslik:
		hataYaz(w, http.StatusBadRequest, dil.F("Başlık en çok %d karakter olabilir.", enUzunBaslik))
		return
	}
	if h := icerikDenetle(metin, dil.T("İçerik boş olamaz")); h != "" {
		hataYaz(w, http.StatusBadRequest, h)
		return
	}
	p, err := d.Forum.CreatePost(r.Context(), istek.Kategori, baslik, metin)
	if err != nil {
		hataYaz(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonYaz(w, map[string]string{"id": p.ID})
}

func (u *Uygulama) yanitYaz(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Konu  string `json:"konu"`
		Metin string `json:"metin"`
	}
	if jsonOku(r, &istek) != nil || d.Forum == nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if d.Forum.PostByID(istek.Konu) == nil {
		hataYaz(w, http.StatusNotFound, dil.T("Konu bulunamadı; silinmiş ya da süresi dolmuş olabilir."))
		return
	}
	metin := temiz(istek.Metin)
	if h := icerikDenetle(metin, dil.T("Yanıt boş olamaz")); h != "" {
		hataYaz(w, http.StatusBadRequest, h)
		return
	}
	if _, err := d.Forum.CreateReply(r.Context(), istek.Konu, metin); err != nil {
		// Yanıt yerelde saklandı; yalnızca yayımlanamadı (ör. henüz eş yok).
		hataYaz(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// konuAcabilir, terminal arayüzündeki kuralın aynısıdır: yeni konu açmak
// Konu Aç eklentisinin işidir ve o etkinken mümkündür.
func (d *cekirdek) konuAcabilir() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, p := range d.eklentiler {
		if p.Name() == "konu_ac" && p.Enabled {
			return true
		}
	}
	return false
}

// ─── Sohbet ─────────────────────────────────────────────────────────────────

type iletiBilgisi struct {
	ID        string    `json:"id,omitempty"`
	Yazar     string    `json:"yazar"`
	YazarKisa string    `json:"yazarKisa,omitempty"`
	Rol       string    `json:"rol,omitempty"`
	Metin     string    `json:"metin"`
	Zaman     time.Time `json:"zaman"`
	Kendi     bool      `json:"kendi,omitempty"`
	Imzasiz   bool      `json:"imzasiz,omitempty"`
}

func (d *cekirdek) ileti(m *chatlog.Message) iletiBilgisi {
	return iletiBilgisi{
		ID: m.ID, Yazar: m.AuthorName, YazarKisa: kisaAnahtar(m.AuthorKey), Rol: d.rol(m.AuthorKey),
		Metin: m.Text, Zaman: m.SentAt, Kendi: m.AuthorKey != "" && m.AuthorKey == d.anahtar, Imzasiz: !m.Signed(),
	}
}

func (u *Uygulama) sohbetGecmisi(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	var liste []iletiBilgisi
	if d.Sohbet != nil {
		for _, m := range d.Sohbet.All() {
			liste = append(liste, d.ileti(m))
		}
	}
	jsonYaz(w, map[string]any{"iletiler": liste, "sinir": 500})
}

func (u *Uygulama) sohbetGonder(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Metin string `json:"metin"`
	}
	if jsonOku(r, &istek) != nil || d.Sohbet == nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	metin := temiz(istek.Metin)
	if metin == "" {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	if len([]rune(metin)) > 500 {
		hataYaz(w, http.StatusBadRequest, dil.F("İleti en çok %d karakter olabilir.", 500))
		return
	}
	m, err := d.Sohbet.Send(r.Context(), metin)
	if err != nil {
		hataYaz(w, http.StatusBadGateway, dil.T("Gönderilemedi: ")+err.Error())
		return
	}
	jsonYaz(w, d.ileti(m))
}

// ─── Profil ─────────────────────────────────────────────────────────────────

func (u *Uygulama) profilGetir(w http.ResponseWriter, _ *http.Request, d *cekirdek) {
	o := map[string]any{
		"ad": d.Kimlik.Name(), "anahtar": d.anahtar, "peerID": d.peerID, "rol": d.rol(d.anahtar),
		"sinir": map[string]int{"bio": profile.MaxBioBytes, "durum": profile.MaxStatusBytes,
			"baglanti": profile.MaxLinkBytes, "etiket": profile.MaxTags},
	}
	kategoriler := make([]map[string]string, len(protokol.Kategoriler))
	for i, k := range protokol.Kategoriler {
		kategoriler[i] = map[string]string{"ad": k, "etiket": dil.T(k)}
	}
	o["kategoriler"] = kategoriler
	if d.Profil != nil {
		if p := d.Profil.Get(d.anahtar); p != nil {
			o["bio"], o["durum"], o["baglanti"], o["etiketler"], o["github"] = p.Bio, p.Status, p.Link, p.Tags, p.GitHubUser
		}
	}
	if d.Forum != nil {
		konu, yanit, katilim := d.Forum.AuthorStats(d.anahtar)
		o["konuSayisi"], o["yanitSayisi"] = konu, yanit
		if !katilim.IsZero() {
			o["katilim"] = katilim
		}
	}
	jsonYaz(w, o)
}

func (u *Uygulama) profilKaydet(w http.ResponseWriter, r *http.Request, d *cekirdek) {
	var istek struct {
		Bio       string   `json:"bio"`
		Durum     string   `json:"durum"`
		Baglanti  string   `json:"baglanti"`
		Etiketler []string `json:"etiketler"`
	}
	if jsonOku(r, &istek) != nil || d.Profil == nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	var etiketler []string
	for _, k := range protokol.Kategoriler {
		if slices.Contains(istek.Etiketler, k) {
			etiketler = append(etiketler, k)
		}
	}
	if len(etiketler) > profile.MaxTags {
		etiketler = etiketler[:profile.MaxTags]
	}
	github := ""
	if p := d.Profil.Get(d.anahtar); p != nil {
		github = p.GitHubUser // ayrı bir akışla doğrulanır; burada korunur
	}
	err := d.Profil.PublishOwn(r.Context(), profile.OwnUpdate{
		Bio: temiz(istek.Bio), Status: temiz(istek.Durum), Link: temiz(istek.Baglanti), Tags: etiketler, GitHubUser: github,
	})
	if err != nil {
		hataYaz(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonYaz(w, map[string]bool{"tamam": true})
}

// ─── Hakkında ───────────────────────────────────────────────────────────────

func (u *Uygulama) hakkinda(w http.ResponseWriter, r *http.Request) {
	bilesen := len(lisans.Moduller())
	switch r.URL.Query().Get("sekme") {
	case "lisans":
		jsonYaz(w, map[string]any{
			"giris": dil.T("GNU Affero Genel Kamu Lisansı, sürüm 3. Aşağıdaki resmî İngilizce metin bağlayıcıdır."),
			"metin": lisans.Metin(),
		})
	case "ucuncu":
		jsonYaz(w, map[string]any{
			"giris": dil.F("AND %d açık kaynak bileşen içerir; her biri kendi lisansıyla dağıtılır. Aşağıdaki özgün İngilizce bildirim her bileşenin lisans metnini içerir.", bilesen),
			"metin": lisans.UcuncuTaraf(),
		})
	default:
		jsonYaz(w, map[string]any{
			"bilgiler": [][2]string{
				{dil.T("Sürüm"), updater.Version},
				{dil.T("Derleme"), runtime.Version() + " · " + runtime.GOOS + "/" + runtime.GOARCH},
				{dil.T("Lisans"), lisans.Ad},
				{dil.T("Kaynak kodu"), lisans.KaynakKodu},
				{dil.T("Veri dizini"), u.dir},
			},
			"yasal": []string{
				dil.T("Telif hakkı (C) 2026 AND katkıcıları"),
				dil.T("Bu program özgür yazılımdır: GNU AGPL-3.0 koşullarıyla değiştirip yeniden dağıtabilirsin.") + " " +
					dil.T("Program yararlı olması umuduyla dağıtılır."),
				dil.T("Yasaların izin verdiği ölçüde HİÇBİR GARANTİSİ YOKTUR.") + " " +
					dil.T("Satılabilirlik ya da belirli bir amaca uygunluk garantisi de verilmez; ayrıntılar Lisans sekmesindedir."),
				dil.T("AND'ı değiştirip ağ üzerinden başkalarının kullanımına sunarsan, lisansın 13. maddesi gereği değiştirdiğin sürümün kaynak kodunu da onlara sunmalısın."),
			},
			"ucuncu":        dil.F("AND %d açık kaynak bileşen içerir; her biri kendi lisansıyla dağıtılır. Lisans metinleri Üçüncü taraf sekmesinde ve and --ucuncu-taraf çıktısındadır.", bilesen),
			"guvenlik":      dil.T("Güvenlik açıklarını herkese açık bir issue yerine SECURITY.md'deki adrese bildir."),
			"gizlilikGiris": dil.T("AND'ın hangi verileri nerede tuttuğu ve kimlerin görebileceği:"),
			"gizlilik":      tui.GizlilikMaddeleri(),
			"gizlilikNot": dil.F("Bu cihazdaki verilerin %s dizinindedir; AND'ı kaldırırken bu dizini silerek hepsini silebilirsin. Başka eşlere yayılmış içerik bundan etkilenmez.", u.dir) +
				" " + dil.T("Tamamı: depodaki GIZLILIK.md."),
			"kurallarGiris": dil.T("AND'ı kullanırken şu kurallara uyarsın:"),
			"kurallar":      tui.KuralMaddeleri(),
			"kurallarNot":   dil.F("Kuralların sürümü: %d. Tamamı: depodaki KULLANIM_KOSULLARI.md.", tui.KurallarSurumu),
		})
	}
}

// guvenliMetin, eşlerin kendi beyan ettiği kısa bir metni (ör. sürüm)
// kontrol karakterlerinden temizler.
func guvenliMetin(s string) string { return strings.TrimSpace(pluginui.Safe(s)) }
