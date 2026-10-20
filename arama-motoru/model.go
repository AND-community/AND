package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

type ekran int

const (
	ekAnaMenu ekran = iota
	ekArama
	ekDetay
	ekDurum
	// ekKodlaGit, birinden alınan bir site paylaşım kodunu (bkz.
	// index.go'nun SiteSayfasiBul'u) girip doğrudan o sayfaya gitmek
	// içindir — forum/sohbette o yazarı tesadüfen bulmayı BEKLEMEDEN.
	ekKodlaGit
	// ekYardim, TÜM ekranlardaki kısayolları ve gelişmiş sorgu sözdizimini
	// (README'de belgelenen ama uygulama İÇİNDE hiçbir yerde
	// gösterilmeyen bilgi) TEK bir başvuru ekranında toplar — f1 ile HER
	// ekrandan erişilebilir (bkz. handleKey), esc ile GİRİLDİĞİ ekrana
	// geri döner (bkz. yardimOncesiEkran).
	ekYardim
	// ekTakip, "takip et" ile kaydedilen aramaların (bkz. index.go'nun
	// "Kayıtlı arama takibi" bölümü) listelendiği ekrandır — Ana Menü'den
	// girilir, her satır kaç YENİ eşleşme biriktiğini gösterir.
	ekTakip
)

type tickMsg struct{}
type durumMsg struct {
	peerSayisi   int
	indeksSayisi int
	// takipYeniSayisi, TÜM kayıtlı aramalardaki toplam okunmamış eşleşme
	// sayısıdır (bkz. index.go'nun TakipToplamYeni'si) — Ana Menü'deki
	// "Takip Edilenler" seçeneğinin yanında bir rozet olarak gösterilir.
	// Diğer iki alanla AYNI 2 saniyelik tick'te tazelenir; sorgu tek bir
	// COALESCE(SUM(...)) olduğundan (küçük bir tabloda) ek bir yük değildir.
	takipYeniSayisi int
}
type yerelAramaSonucuMsg struct {
	sorgu string
	sayfa AramaSayfasi
	// yazimOnerisi, sayfa.Sonuclar boşsa VE sorgu boş değilse (bkz.
	// yerelAramaCmd) doldurulan bir "şunu mu demek istediniz?" önerisidir —
	// boşsa öneri yok demektir (ctrl+d ile uygulanabilir, bkz. keyArama).
	yazimOnerisi string
	err          error
}
type agAramaSonucuMsg struct {
	sonuclar []AramaSonucu
	// hedefli, bu sonucun agdaAraCmd'nin (tüm eşlere yayılan) TÜM-eşler
	// taramasından mı yoksa agdaTekEsHedefleCmd'nin (bkz. keyDetay'ın "t"
	// tuşu) TEK bir eşi hedefleyen sorgusundan mı geldiğini ayırt eder —
	// yalnızca bildirim METNİNİ (bkz. updateInner) özelleştirmek için,
	// başka bir davranış farkı yoktur.
	hedefli bool
	err     error
}
type baglantiAcildiMsg struct {
	url string
	err error
}

// agSiteBulunduMsg, agdaSiteAraCmd'nin (bkz. o fonksiyonun doc yorumu —
// "Kodla Git ağa düşsün" ve Sonuç Detayı'ndaki iç site bağlantıları,
// ikisi de bunu kullanır) sonucunu taşır.
type agSiteBulunduMsg struct {
	kod       string
	s         AramaSonucu
	bulunduMu bool
	err       error
}

// takipListesiMsg, ekTakip ekranına girildiğinde bir kez (bkz.
// keyAnaMenu'nün "Takip Edilenler" dalı) doldurulan kayıtlı arama listesini
// taşır.
type takipListesiMsg struct {
	liste []KayitliArama
	err   error
}

// panoTemizleMsg, panoyaKopyalaDizisi'nin View() çıktısına gömüldüğü TEK
// kareden hemen sonra gelir ve m.panoKopyalaDizisi'ni temizler — bkz.
// pano.go'nun doc yorumu (aynı diziyi her karede tekrar göndermemek için).
type panoTemizleMsg struct{}

func panoTemizleCmd() tea.Cmd {
	return func() tea.Msg { return panoTemizleMsg{} }
}

// durumIstatistikMsg, Durum ekranına girildiğinde (bkz. keyAnaMenu'nun
// "Durum" dalı) BİR KEZ hesaplanan "arama konsolu" istatistiklerini taşır
// — mevcut 2 saniyelik tickMsg'de DEĞİL, çünkü depoAc'ın
// SetMaxOpenConns(1) sınırlamasıyla her tick'te ek GROUP BY sorguları
// koşturmak gereksiz bir yük olurdu.
type durumIstatistikMsg struct {
	tipDagilimi     map[SonucTip]int
	enAktifYazarlar []YazarIstatistigi
	gunlukTrend     []GunlukSayim
	// esGuven, bu SÜREÇ boyunca DOĞRUDAN sorgulanmış eşlerin geçerli/
	// geçersiz imzalı sonuç oranını özetler — bkz. search.go'nun
	// esGuvenDefteri'si. Diğer üç alanın AKSİNE bir DB sorgusu DEĞİLDİR
	// (bellek-içi), ama tutarlılık için AYNI tek-seferlik Cmd'de toplanır.
	esGuven []EsGuvenOzet
	err     error
}

type model struct {
	ctx    context.Context
	host   libp2phost.Host
	depo   *Depo
	kimlik *Kimlik

	yeniKimlik bool
	// temaAdi, dataDir/tema.json'dan yüklenen özel temanın adıdır (bkz.
	// tema.go) — boşsa hiç özel tema yüklenmemiştir, view.go'nun sabit
	// varsayılan paleti kullanılıyordur ve ana menüde AYRICA gösterilmez.
	temaAdi string

	ekran ekran
	// yardimOncesiEkran, f1'e (bkz. ekYardim'in doc yorumu) hangi
	// ekrandayken basıldığını hatırlar — Yardım'dan esc ile çıkınca
	// (ör. yarım kalmış bir arama ya da kod girişini KAYBETMEDEN) oraya
	// geri döner.
	yardimOncesiEkran ekran
	w, h              int
	notice            string
	isErr             bool

	menuIdx int

	aramaIn textinput.Model
	// kodIn, ekKodlaGit ekranının paylaşım kodu (bkz. index.go'nun
	// SiteSayfasiBul'u) giriş kutusudur — aramaIn'den AYRI tutulur ki iki
	// ekran arasında geçince birbirlerinin metnini bozmasınlar.
	kodIn  textinput.Model
	filtre SonucTip // "" = tümü; bkz. sonrakiFiltre
	// yazarFiltre/yazarFiltreAd: doluysa yalnızca bu yazar anahtarına ait
	// sonuçlar aranır ("bu yazarın diğer içerikleri", bkz. keyDetay'daki
	// 'a' tuşu) — yazarFiltreAd yalnızca görüntüleme içindir (bkz. view.go).
	yazarFiltre   string
	yazarFiltreAd string
	yerelSonuclar []AramaSonucu
	agSonuclari   []AramaSonucu
	sonucIdx      int
	agAraniyor    bool
	// siralama, geçerli sonuç sıralama modudur (bkz. index.go'nun
	// SiralamaModu'su) — ctrl+r ile Alaka/Yeni arasında değiştirilir.
	siralama SiralamaModu

	// sayfaNo, 0'dan başlayan geçerli sonuç sayfasıdır (bkz. pgup/pgdown,
	// keyArama). sonrakiVarMi, en son yerelAramaSonucuMsg'in
	// AramaSayfasi.SonrakiVarMi'sinden gelir — pgdown'ın etkin olup
	// olmadığını belirler (bkz. view.go'nun sayfalama göstergesi).
	sayfaNo      int
	sonrakiVarMi bool
	// yazimOnerisi, son aramanın sıfır sonuç verdiği ve bir "şunu mu demek
	// istediniz?" adayı bulunduğu durumda doludur (bkz. yerelAramaCmd) —
	// ctrl+d ile uygulanabilir.
	yazimOnerisi string

	seciliSonuc *AramaSonucu
	baglantilar []string // seciliSonuc'un gövdesinde bulunan http(s) bağlantıları — bkz. links.go
	// siteBaglantilari, seciliSonuc'un gövdesinde geçen and-site:// iç
	// bağlantı kodlarıdır (bkz. links.go'nun siteBaglantilariBul'u) —
	// baglantilar'dan AYRI tutulur çünkü açılış eylemleri farklıdır (http
	// harici tarayıcıda, and-site uygulama İÇİNDE gezinerek).
	siteBaglantilari []string
	// qrGoster, Sonuç Detayı'nda bir site sayfasının paylaşım kodunun QR
	// kodunu (bkz. qr.go) gösterip göstermediğidir — 'k' ile açılıp
	// kapanır, her yeni seciliSonuc atamasında false'a döner (bkz.
	// keyArama/keyKodlaGit/agSiteBulunduMsg işleyicileri).
	qrGoster bool
	// panoKopyalaDizisi, doluysa BİR SONRAKİ View() çıktısına gömülüp
	// hemen ardından temizlenen bekleyen bir OSC 52 pano-kopyalama
	// dizisidir — bkz. pano.go'nun doc yorumu, keyDetay'ın "c" tuşu.
	panoKopyalaDizisi string
	// kodAraniyor, "Kodla Git" ekranının (ya da Sonuç Detayı'ndaki bir iç
	// site bağlantısının) yerelde bulunamayıp ağda arandığı ("Kodla Git
	// ağa düşsün", bkz. agdaSiteAraCmd) süre boyunca true'dur — spinner
	// göstermek için.
	kodAraniyor bool

	// takipListesi, ekTakip ekranına girildiğinde bir kez doldurulur (bkz.
	// takipListesiCmd) — index.go'nun TakipListele'siyle AYNI sırada.
	takipListesi []KayitliArama
	takipIdx     int

	peerSayisi      int
	indeksSayisi    int
	takipYeniSayisi int

	// Durum ekranının "arama konsolu" istatistik panelleri — yalnızca o
	// ekrana girildiğinde durumIstatistikCmd ile doldurulur (bkz.
	// durumIstatistikMsg'in doc yorumu).
	tipDagilimi     map[SonucTip]int
	enAktifYazarlar []YazarIstatistigi
	gunlukTrend     []GunlukSayim
	esGuven         []EsGuvenOzet

	spinner spinner.Model
}

func yeniModel(ctx context.Context, h libp2phost.Host, depo *Depo, kimlik *Kimlik) model {
	ti := textinput.New()
	ti.Placeholder = "Arama sorgusu…"
	ti.CharLimit = 200
	// Varsayılan "> " istemi yerine uygulamanın kendi arama simgesi — bkz.
	// view.go'nun ustBar/durum çubuğu gibi diğer "chrome" öğeleriyle aynı
	// görsel dile ait tek karakterlik bir ok.
	ti.Prompt = "› "

	ki := textinput.New()
	ki.Placeholder = "yazarAnahtarı/slug"
	ki.CharLimit = 200
	ki.Prompt = "› "

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	return model{
		ctx:     ctx,
		host:    h,
		depo:    depo,
		kimlik:  kimlik,
		ekran:   ekAnaMenu,
		aramaIn: ti,
		kodIn:   ki,
		spinner: sp,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, durumYenileCmd(m.host, m.depo), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func pendingCmd(m model, cmd tea.Cmd) tea.Cmd {
	return tea.Batch(m.spinner.Tick, cmd)
}

func durumYenileCmd(h libp2phost.Host, depo *Depo) tea.Cmd {
	return func() tea.Msg {
		n, _ := depo.SayimAl()
		takip, _ := depo.TakipToplamYeni()
		return durumMsg{peerSayisi: len(h.Network().Peers()), indeksSayisi: n, takipYeniSayisi: takip}
	}
}

// durumIstatistikCmd, Durum ekranına girildiğinde bir kez koşturulan
// "arama konsolu" istatistik sorgularını toplar — bkz. durumIstatistikMsg'in
// doc yorumu (neden tickMsg'de DEĞİL).
func durumIstatistikCmd(depo *Depo) tea.Cmd {
	return func() tea.Msg {
		tipDagilimi, err := depo.TipDagilimiAl()
		if err != nil {
			return durumIstatistikMsg{err: err}
		}
		yazarlar, err := depo.EnAktifYazarlarAl(5)
		if err != nil {
			return durumIstatistikMsg{err: err}
		}
		trend, err := depo.GunlukIcerikTrendiAl(14)
		if err != nil {
			return durumIstatistikMsg{err: err}
		}
		esGuven := esGuvenDefteriVarsayilan.enIyiListele(5)
		return durumIstatistikMsg{tipDagilimi: tipDagilimi, enAktifYazarlar: yazarlar, gunlukTrend: trend, esGuven: esGuven}
	}
}

// yerelAramaCmd, yerel indekste sayfaNo'ıncı sayfayı (0'dan başlar,
// aramaSayfaBoyu kadar) arar. Sonuç sıfırsa ve bir sorgu terimi verilmişse
// (offset==0'da — yüksek sayfalarda "sonuç yok" beklenen bir durumdur, o
// zaman yazım önerisi ANLAMSIZ olurdu) bir "şunu mu demek istediniz?"
// önerisi de üretilmeye çalışılır. siralama, index.go'nun SiralamaModu'su —
// bkz. keyArama'nın ctrl+r'si.
func yerelAramaCmd(depo *Depo, sorgu string, tip SonucTip, yazarKey string, siralama SiralamaModu, sayfaNo int) tea.Cmd {
	return func() tea.Msg {
		offset := sayfaNo * aramaSayfaBoyu
		sayfa, err := depo.AraGelismisSirali(sorgu, tip, yazarKey, siralama, aramaSayfaBoyu, offset)
		var oneri string
		if err == nil && offset == 0 && len(sayfa.Sonuclar) == 0 && strings.TrimSpace(sorgu) != "" {
			oneri, _ = depo.YazimOnerisiUret(sorgu)
		}
		return yerelAramaSonucuMsg{sorgu: sorgu, sayfa: sayfa, yazimOnerisi: oneri, err: err}
	}
}

// agdaAraCmd, sorguyu şu an bağlı olduğumuz TÜM eşlere eşzamanlı olarak
// yollar ve hepsinin yanıtını (ya da kendi zaman aşımını) bekleyip
// birleştirir. TÜM hedeflere AYNI (paylaşılan) SorguID'li istek gönderilir
// — bkz. search.go'nun yeniAramaIstegi doc yorumu — böylece bu düğümün N
// doğrudan eşi, ağın derinlerinde AYNI çok-atlamalı yayılma ağacına
// katılır, N tane bağımsız (ve gereksiz yere tekrar eden) ağaca değil.
// search.go'nun doc yorumundaki gibi bu artık DM tabanlı bir anket değil,
// doğrudan bir istek/yanıt RPC'sidir; her düğüm kendi TTL'i kadar öteye
// de sorar, dönen sonuçlar burada kaynak_id'ye göre tekilleştirilir.
func agdaAraCmd(ctx context.Context, h libp2phost.Host, sorgu string, tip SonucTip, yazarKey string) tea.Cmd {
	return func() tea.Msg {
		peers := aramaEsleri(h, "", true)
		if len(peers) == 0 {
			return agAramaSonucuMsg{err: fmt.Errorf("Arama Motoru çalıştıran bağlı eş yok")}
		}
		istek := yeniAramaIstegi(sorgu, tip, yazarKey)
		var mu sync.Mutex
		var toplam []AramaSonucu
		gorulen := map[string]bool{}
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go func(p peer.ID) {
				defer wg.Done()
				defer recoverGoroutine("agda-ara-fanout")
				sonuclar, err := aramaIstekGonderRaw(ctx, h, p, istek)
				if err != nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				for _, s := range sonuclar {
					if gorulen[s.KaynakID] {
						continue
					}
					gorulen[s.KaynakID] = true
					kaynakPeer := s.KaynakPeer
					if kaynakPeer == "" {
						kaynakPeer = p.String()
					}
					toplam = append(toplam, AramaSonucu{
						Tip: s.Tip, KaynakID: s.KaynakID, UstID: s.UstID, Kategori: s.Kategori,
						Baslik: s.Baslik, Govde: s.Govde, YazarAd: s.YazarAd, YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS,
						KaynakPeer: kaynakPeer,
					})
				}
			}(p)
		}
		wg.Wait()
		return agAramaSonucuMsg{sonuclar: toplam}
	}
}

// agdaTekEsHedefleCmd, agdaAraCmd'nin AKSİNE TÜM eşlere değil TEK bir
// hedefe (bkz. search.go'nun aramaTekEsIstegi'si, TTL=1) sorar — Sonuç
// Detayı'nda bir ağ sonucunun KaynakPeer'ını hedefleyerek "bu eşin
// GÜNCEL indeksinde başka ne var" diye kontrol etmek için (bkz. keyDetay'ın
// "t" tuşu). hedefStr'nin adresi bu düğümün peerstore'unda yoksa (ör. yalnızca
// çok atlamalı bir yayılma zincirinde DOLAYLI olarak görülmüş, hiç doğrudan
// bağlanılmamış bir eş) dial başarısız olabilir — bu, aramaIstekGonderRaw'ın
// normal "stream açılamadı" hatası olarak yüzeye çıkar, özel bir durum değildir.
func agdaTekEsHedefleCmd(ctx context.Context, h libp2phost.Host, hedefStr string, sorgu string, tip SonucTip, yazarKey string) tea.Cmd {
	return func() tea.Msg {
		sonuclar, err := aramaTekEsIstegi(ctx, h, hedefStr, sorgu, tip, yazarKey)
		if err != nil {
			return agAramaSonucuMsg{err: err, hedefli: true}
		}
		return agAramaSonucuMsg{sonuclar: sonuclar, hedefli: true}
	}
}

// agdaSiteAraCmd, bir site paylaşım kodunun ("Kod ağa düşsün" — bkz.
// keyKodlaGit'in doc yorumu ve keyDetay'ın "g" tuşu) YEREL indekste
// bulunamadığında ağda aranmasıdır. Sayfa kodunun kendisiyle (kaynak_id)
// ağda doğrudan arama YAPAN bir alan yoktur — arama protokolü (search.go)
// yalnızca serbest metin + tür + yazar filtresi taşır. Bunun yerine mevcut
// protokol OLDUĞU GİBİ, hiçbir değişiklik yapılmadan yeniden kullanılır:
// kod'un yazarKey'i çıkarılıp Metin="" + Tip=TipSite + YazarKey=yazarKey ile
// sorgulanır — bu, sunucu tarafında (aramaSunucusuKaydet) AraGelismis'in
// "anahtar kelimesiz ama yazar filtreli" dalına (yazarinTumIcerigi) düşer
// ve o yazarın TÜM site sayfalarını (en yeniden eskiye, en fazla
// maxAgSonucSayisi tanesini) döner; istemci bunlar arasından tam KaynakID
// eşleşmesini arar. Bulunan sayfa, aramaIstekGonderRaw'ın (search.go'nun
// TEK boğaz noktası) zaten imza doğrulamasından geçmiştir — bu yüzden
// güvenle YEREL indekse de kalıcılaştırılır (depo.EkleYaGuncelle), bir
// dahaki sefere ağa çıkmadan erişilebilir olsun diye.
func agdaSiteAraCmd(ctx context.Context, h libp2phost.Host, depo *Depo, kod string) tea.Cmd {
	return func() tea.Msg {
		yazarKey, _, ok := siteKaynakIDCoz(kod)
		if !ok {
			return agSiteBulunduMsg{kod: kod, err: fmt.Errorf("geçersiz kod biçimi")}
		}
		peers := aramaEsleri(h, "", true)
		if len(peers) == 0 {
			return agSiteBulunduMsg{kod: kod, err: fmt.Errorf("Arama Motoru çalıştıran bağlı eş yok")}
		}
		istek := yeniAramaIstegi("", TipSite, yazarKey)
		var mu sync.Mutex
		var bulunan *AramaSonucu
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go func(p peer.ID) {
				defer wg.Done()
				defer recoverGoroutine("agda-site-ara")
				sonuclar, err := aramaIstekGonderRaw(ctx, h, p, istek)
				if err != nil {
					return
				}
				for _, s := range sonuclar {
					if s.KaynakID != kod {
						continue
					}
					mu.Lock()
					if bulunan == nil {
						bulunan = &AramaSonucu{
							Tip: s.Tip, KaynakID: s.KaynakID, UstID: s.UstID, Kategori: s.Kategori,
							Baslik: s.Baslik, Govde: s.Govde, YazarAd: s.YazarAd, YazarKey: s.YazarKey,
							OlusturmaTS: s.OlusturmaTS, KaynakPeer: p.String(), Sig: s.Sig,
						}
					}
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
		if bulunan == nil {
			return agSiteBulunduMsg{kod: kod, bulunduMu: false}
		}
		_ = depo.EkleYaGuncelle(*bulunan)
		return agSiteBulunduMsg{kod: kod, s: *bulunan, bulunduMu: true}
	}
}

// takipListesiCmd, ekTakip ekranına girildiğinde kayıtlı aramaları yükler.
func takipListesiCmd(depo *Depo) tea.Cmd {
	return func() tea.Msg {
		liste, err := depo.TakipListele()
		return takipListesiMsg{liste: liste, err: err}
	}
}

func baglantiAcCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return baglantiAcildiMsg{url: url, err: baglantiAc(url)}
	}
}

// gorunenSonuclar, yerel ve ağ sonuçlarını KaynakID'ye göre tekilleştirip
// birleştirir — aynı içerik hem kendi yerel indeksimizde hem de ağdaki
// bir eşin yanıtında bulunabilir (ör. o eş de aynı gönderiyi zaten
// senkronize etmişse); yerel sonuçlar önceliklidir (zaten elimizde,
// ekstra bir "[Ağ: …]" etiketiyle karıştırmaya gerek yok).
func (m model) gorunenSonuclar() []AramaSonucu {
	out := make([]AramaSonucu, 0, len(m.yerelSonuclar)+len(m.agSonuclari))
	gorulen := map[string]bool{}
	for _, s := range m.yerelSonuclar {
		if gorulen[s.KaynakID] {
			continue
		}
		gorulen[s.KaynakID] = true
		out = append(out, s)
	}
	for _, s := range m.agSonuclari {
		if gorulen[s.KaynakID] {
			continue
		}
		gorulen[s.KaynakID] = true
		out = append(out, s)
	}
	return out
}

// sonrakiFiltre, "tab" ile döngüsel olarak gezilen içerik türü
// filtresidir: Tümü → Konu → Yanıt → Sohbet → Site → Tümü …
func sonrakiFiltre(t SonucTip) SonucTip {
	switch t {
	case "":
		return TipKonu
	case TipKonu:
		return TipYanit
	case TipYanit:
		return TipSohbet
	case TipSohbet:
		return TipSite
	default:
		return ""
	}
}

// Update, gerçek işi updateInner'a devreder ve onu bir panik kalkanıyla
// sarar: içerik ağdaki GÜVENİLMEYEN eşlerden gelir (bkz. dosyanın
// başındaki güvenlik ilkesi) ve beklenmeyen bir kenar durumu (ör. bozuk
// bir sonuç, indeks dışı bir erişim) Update içinde panikleyebilir —
// bunu yakalamadan bırakmak, TEK bir kötü mesajın tüm TUI'yi çökertip
// kullanıcının aramasını yarıda kesmesi anlamına gelirdi. Panik
// yakalanırsa günlüğe yazılır ve panikten ÖNCEKİ model değişmeden
// döndürülür — kullanıcı bir sonraki tuşuna basmaya devam edebilir.
func (m model) Update(msg tea.Msg) (rm tea.Model, rc tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			gunlukHata("panik yakalandı (Update), yoksayılıyor:", r)
			rm, rc = m, nil
		}
	}()
	return m.updateInner(msg)
}

func (m model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(durumYenileCmd(m.host, m.depo), tickCmd())

	case durumMsg:
		m.peerSayisi = msg.peerSayisi
		m.indeksSayisi = msg.indeksSayisi
		m.takipYeniSayisi = msg.takipYeniSayisi
		return m, nil

	case yerelAramaSonucuMsg:
		if msg.sorgu != strings.TrimSpace(m.aramaIn.Value()) {
			return m, nil
		}
		if msg.err != nil {
			m.notice, m.isErr = "Arama hatası: "+msg.err.Error(), true
			return m, nil
		}
		m.yerelSonuclar = ekranIcinListe(msg.sayfa.Sonuclar) // ağdan gelmiş içerik — bkz. guvenlik.go
		m.sonrakiVarMi = msg.sayfa.SonrakiVarMi
		m.yazimOnerisi = msg.yazimOnerisi
		if m.sonucIdx >= len(m.gorunenSonuclar()) {
			m.sonucIdx = 0
		}
		return m, nil

	case durumIstatistikMsg:
		if msg.err != nil {
			m.notice, m.isErr = "İstatistikler alınamadı: "+msg.err.Error(), true
			return m, nil
		}
		m.tipDagilimi = msg.tipDagilimi
		m.enAktifYazarlar = msg.enAktifYazarlar
		for i := range m.enAktifYazarlar {
			m.enAktifYazarlar[i].YazarAd = guvenliMetin(m.enAktifYazarlar[i].YazarAd)
		}
		m.gunlukTrend = msg.gunlukTrend
		m.esGuven = msg.esGuven
		return m, nil

	case agAramaSonucuMsg:
		m.agAraniyor = false
		if msg.err != nil {
			m.notice, m.isErr = "Ağda arama başarısız: "+msg.err.Error(), true
			return m, nil
		}
		m.agSonuclari = ekranIcinListe(msg.sonuclar)
		switch {
		case len(m.agSonuclari) == 0 && msg.hedefli:
			m.notice, m.isErr = "Hedeflenen eşten ek sonuç gelmedi.", false
		case len(m.agSonuclari) == 0:
			m.notice, m.isErr = "Ağdan ek sonuç gelmedi.", false
		case msg.hedefli:
			m.notice, m.isErr = fmt.Sprintf("Hedeflenen eşten %d sonuç geldi.", len(m.agSonuclari)), false
		default:
			m.notice, m.isErr = fmt.Sprintf("Ağdan %d ek sonuç geldi.", len(m.agSonuclari)), false
		}
		return m, nil

	case agSiteBulunduMsg:
		m.kodAraniyor = false
		if msg.err != nil {
			m.notice, m.isErr = "Ağda arama başarısız: "+msg.err.Error(), true
			return m, nil
		}
		if !msg.bulunduMu {
			m.notice, m.isErr = "Bu kod ağda da bulunamadı.", true
			return m, nil
		}
		s := msg.s.ekranIcin()
		s = s.ekranIcin()
		m.seciliSonuc = &s
		m.baglantilar = baglantilariBul(s.Govde)
		m.siteBaglantilari = siteBaglantilariBul(s.Govde)
		m.qrGoster = false
		m.notice, m.isErr = "", false
		m.ekran = ekDetay
		return m, nil

	case takipListesiMsg:
		if msg.err != nil {
			m.notice, m.isErr = "Kayıtlı aramalar alınamadı: "+msg.err.Error(), true
			return m, nil
		}
		m.takipListesi = msg.liste
		if m.takipIdx >= len(m.takipListesi) {
			m.takipIdx = 0
		}
		return m, nil

	case baglantiAcildiMsg:
		if msg.err != nil {
			m.notice, m.isErr = "Bağlantı açılamadı: "+msg.err.Error(), true
		} else {
			m.notice, m.isErr = "Tarayıcıda açıldı: "+kirp(msg.url, 60), false
		}
		return m, nil

	case panoTemizleMsg:
		m.panoKopyalaDizisi = ""
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey, f1'i (bkz. ekYardim'in doc yorumu) TEK bir yerden, HER
// ekrandan (ekYardim'in kendisi HARİÇ — sonsuz döngü olmasın) evrensel
// olarak yakalar; bu sayede her ekranın kendi tuş işleyicisine ayrı ayrı
// eklemek yerine tek noktadan, kaçırılma riski olmadan yönetilir. f1
// (klavyede METİN ÜRETMEYEN bir fonksiyon tuşu) KASITLI seçildi: Arama/
// Kodla Git ekranlarındaki metin kutularına "?" gibi yazılabilir bir
// karakteri kısayol yapmak, o karakteri sorguya/koda YAZMAYI imkânsız
// kılardı.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "f1" && m.ekran != ekYardim {
		m.yardimOncesiEkran = m.ekran
		m.ekran = ekYardim
		return m, nil
	}
	switch m.ekran {
	case ekAnaMenu:
		return m.keyAnaMenu(msg)
	case ekArama:
		return m.keyArama(msg)
	case ekDetay:
		return m.keyDetay(msg)
	case ekDurum:
		return m.keyDurum(msg)
	case ekKodlaGit:
		return m.keyKodlaGit(msg)
	case ekYardim:
		return m.keyYardim(msg)
	case ekTakip:
		return m.keyTakip(msg)
	}
	return m, nil
}

// keyYardim, ekYardim ekranının tuş işleyicisidir — herhangi bir tuş
// (esc/q sınırlı değil, "f1'e tekrar basmak da kapatır" beklentisini de
// karşılamak için ayrıca f1 kontrolü) girildiği ekrana geri döner.
func (m model) keyYardim(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "f1":
		m.ekran = m.yardimOncesiEkran
		return m, nil
	}
	return m, nil
}

func (m model) menuSecenekleri() []string {
	return []string{"Ara", "Kodla Git", "Takip Edilenler", "Durum", "Çıkış"}
}

func (m model) keyAnaMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.menuSecenekleri()
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < len(opts)-1 {
			m.menuIdx++
		}
	case "enter":
		switch opts[m.menuIdx] {
		case "Çıkış":
			return m, tea.Quit
		case "Ara":
			m.ekran = ekArama
			m.notice = ""
			m.aramaIn.Focus()
			return m, nil
		case "Kodla Git":
			m.ekran = ekKodlaGit
			m.notice = ""
			m.kodIn.Focus()
			return m, nil
		case "Takip Edilenler":
			m.ekran = ekTakip
			m.notice = ""
			m.takipIdx = 0
			return m, takipListesiCmd(m.depo)
		case "Durum":
			m.ekran = ekDurum
			return m, durumIstatistikCmd(m.depo)
		}
	}
	return m, nil
}

// keyKodlaGit, ekKodlaGit ekranının tuş işleyicisidir: kullanıcı bir kod
// yazıp enter'a basınca iki farklı şeyi kabul eder:
//
//   - "yazarAnahtarı/slug" (bir "/" içerir): TEK bir site sayfasının
//     paylaşım kodu (bkz. index.go'nun SiteSayfasiBul'u) — önce YEREL
//     indekste aranır; bulunursa doğrudan Detay ekranına geçer. Yerelde
//     yoksa VE bağlı eş varsa, "Kodla Git ağa düşsün" — otomatik olarak
//     agdaSiteAraCmd ile ağda aranır (bkz. onun doc yorumu): henüz bu
//     düğüme senkronize OLMAMIŞ ama ağda hâlâ mevcut bir sayfa bu şekilde
//     de bulunabilir.
//   - yalnızca "yazarAnahtarı" ("/" İÇERMEZ): TEK bir sayfa kodu değil —
//     o yazarın TÜM site sayfalarını (bir "site haritası") Ara ekranında,
//     TipSite filtresiyle listeler (index.go'nun yazarinTumIcerigi'ni
//     YENİDEN KULLANIR, yeni bir sorgu şekli İCAT ETMEZ). Liste
//     yüklendikten sonra kullanıcı normal Ara ekranındaki ctrl+n ile aynı
//     yazarın ağdaki diğer sayfalarını da arayabilir — ayrı bir "ağda
//     sitemap ara" akışına gerek kalmaz.
func (m model) keyKodlaGit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.kodIn.Blur()
		m.kodIn.SetValue("")
		m.notice = ""
		m.ekran = ekAnaMenu
		return m, nil
	case "enter":
		kod := strings.TrimSpace(m.kodIn.Value())
		if kod == "" {
			return m, nil
		}
		if _, _, ok := siteKaynakIDCoz(kod); !ok {
			// "/" yok — tek sayfa kodu değil, bir yazar anahtarı: site
			// haritası olarak Ara ekranında listele.
			m.yazarFiltre = kod
			m.yazarFiltreAd = ""
			m.filtre = TipSite
			m.aramaIn.SetValue("")
			m.agSonuclari = nil
			m.agAraniyor = false
			m.sonucIdx = 0
			m.sayfaNo = 0
			m.kodIn.Blur()
			m.kodIn.SetValue("")
			m.notice = ""
			m.ekran = ekArama
			m.aramaIn.Focus()
			return m, yerelAramaCmd(m.depo, "", TipSite, kod, m.siralama, m.sayfaNo)
		}
		s, bulunduMu, err := m.depo.SiteSayfasiBul(kod)
		if err != nil {
			m.notice, m.isErr = "Arama hatası: "+err.Error(), true
			return m, nil
		}
		if !bulunduMu {
			if len(m.host.Network().Peers()) == 0 {
				m.notice, m.isErr = "Bu kod yerel indekste bulunamadı — sayfa henüz senkronize olmamış olabilir.", true
				return m, nil
			}
			m.kodAraniyor = true
			m.notice, m.isErr = "Yerelde yok, ağda aranıyor…", false
			return m, pendingCmd(m, agdaSiteAraCmd(m.ctx, m.host, m.depo, kod))
		}
		m.seciliSonuc = &s
		m.baglantilar = baglantilariBul(s.Govde)
		m.siteBaglantilari = siteBaglantilariBul(s.Govde)
		m.qrGoster = false
		m.kodIn.Blur()
		m.kodIn.SetValue("")
		m.notice = ""
		m.ekran = ekDetay
		return m, nil
	}
	var cmd tea.Cmd
	m.kodIn, cmd = m.kodIn.Update(msg)
	return m, cmd
}

func (m model) keyArama(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.aramaIn.Blur()
		m.ekran = ekAnaMenu
		return m, nil
	case "up":
		if m.sonucIdx > 0 {
			m.sonucIdx--
		}
		return m, nil
	case "down":
		if m.sonucIdx < len(m.gorunenSonuclar())-1 {
			m.sonucIdx++
		}
		return m, nil
	case "tab":
		m.filtre = sonrakiFiltre(m.filtre)
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "ctrl+y":
		if m.yazarFiltre == "" {
			return m, nil
		}
		m.yazarFiltre = ""
		m.yazarFiltreAd = ""
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "ctrl+r":
		// Alaka ⇄ Yeni sıralama modu geçişi — bkz. index.go'nun
		// SiralamaModu'su.
		if m.siralama == SiralamaAlaka {
			m.siralama = SiralamaYeni
		} else {
			m.siralama = SiralamaAlaka
		}
		m.sonucIdx = 0
		m.sayfaNo = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "ctrl+t":
		// Geçerli sorgu/tür/yazar kombinasyonunu kayıtlı aramalara ekle —
		// bkz. index.go'nun TakipEkle'si. Boş sorgu VE boş yazar filtresi
		// ("tüm ağı takip et") TakipEkle tarafından reddedilir.
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		if err := m.depo.TakipEkle(sorgu, m.filtre, m.yazarFiltre); err != nil {
			m.notice, m.isErr = "Takibe alınamadı: "+err.Error(), true
		} else {
			m.notice, m.isErr = "Bu arama takibe alındı — Ana Menü'deki \"Takip Edilenler\"den izleyebilirsin.", false
		}
		return m, nil
	case "pgdown":
		if !m.sonrakiVarMi {
			return m, nil
		}
		m.sayfaNo++
		m.sonucIdx = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "pgup":
		if m.sayfaNo == 0 {
			return m, nil
		}
		m.sayfaNo--
		m.sonucIdx = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "ctrl+d":
		// Yalnızca bir yazım önerisi varken devreye girer — textinput'un
		// varsayılan "ileri sil" davranışını (bkz. bubbles/textinput'un
		// öntanımlı DeleteCharacterForward bağı) KASITLI olarak yalnızca bu
		// durumda geçersiz kılıyoruz; öneri yoksa `break` ile switch'ten
		// çıkıp aşağıdaki genel textinput işleyicisine düşer.
		if m.yazimOnerisi == "" {
			break
		}
		m.aramaIn.SetValue(m.yazimOnerisi)
		m.aramaIn.CursorEnd()
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		return m, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "enter":
		liste := m.gorunenSonuclar()
		if m.sonucIdx < len(liste) {
			s := liste[m.sonucIdx]
			m.seciliSonuc = &s
			m.baglantilar = baglantilariBul(s.Govde)
			m.siteBaglantilari = siteBaglantilariBul(s.Govde)
			m.qrGoster = false
			m.ekran = ekDetay
		}
		return m, nil
	case "ctrl+n":
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		if (sorgu == "" && m.yazarFiltre == "") || m.agAraniyor {
			return m, nil
		}
		m.agAraniyor = true
		m.agSonuclari = nil
		m.notice, m.isErr = "Ağda aranıyor…", false
		return m, pendingCmd(m, agdaAraCmd(m.ctx, m.host, sorgu, m.filtre, m.yazarFiltre))
	}

	onceki := m.aramaIn.Value()
	var cmd tea.Cmd
	m.aramaIn, cmd = m.aramaIn.Update(msg)
	if m.aramaIn.Value() != onceki {
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
	}
	sorgu := strings.TrimSpace(m.aramaIn.Value())
	return m, tea.Batch(cmd, yerelAramaCmd(m.depo, sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo))
}

func (m model) keyDetay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekArama
		m.aramaIn.Focus()
		return m, nil
	case "o":
		if len(m.baglantilar) > 0 {
			return m, baglantiAcCmd(m.baglantilar[0])
		}
		return m, nil
	case "a":
		if m.seciliSonuc == nil || m.seciliSonuc.YazarKey == "" {
			return m, nil
		}
		m.yazarFiltre = m.seciliSonuc.YazarKey
		m.yazarFiltreAd = m.seciliSonuc.YazarAd
		m.aramaIn.SetValue("")
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
		m.ekran = ekArama
		m.aramaIn.Focus()
		return m, yerelAramaCmd(m.depo, "", m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	case "t":
		// Hedefli P2P sorgu: bu sonucun geldiği eşi (KaynakPeer) DOĞRUDAN
		// hedefleyip geçerli sorgu/tür/yazar bağlamıyla tekrar sor — bkz.
		// agdaTekEsHedefleCmd'nin doc yorumu. Yalnızca bir AĞ sonucunda
		// anlamlıdır (KaynakPeer boşsa bu zaten yerel bir kayıttır).
		if m.seciliSonuc == nil || m.seciliSonuc.KaynakPeer == "" || m.agAraniyor {
			return m, nil
		}
		hedef := m.seciliSonuc.KaynakPeer
		sorgu := strings.TrimSpace(m.aramaIn.Value())
		m.ekran = ekArama
		m.agAraniyor = true
		m.agSonuclari = nil
		m.notice, m.isErr = "Eş hedefleniyor: "+kisaPeer(hedef)+"…", false
		return m, pendingCmd(m, agdaTekEsHedefleCmd(m.ctx, m.host, hedef, sorgu, m.filtre, m.yazarFiltre))
	case "g":
		// Sayfa içindeki İLK and-site:// iç bağlantısına git (bkz.
		// links.go'nun siteBaglantilariBul'u) — önce yerelde, yoksa (bağlı
		// eş varsa) ağda dener, "Kodla Git ağa düşsün" ile AYNI mekanizma.
		if len(m.siteBaglantilari) == 0 || m.kodAraniyor {
			return m, nil
		}
		kod := m.siteBaglantilari[0]
		if s, bulunduMu, err := m.depo.SiteSayfasiBul(kod); err == nil && bulunduMu {
			s = s.ekranIcin()
			m.seciliSonuc = &s
			m.baglantilar = baglantilariBul(s.Govde)
			m.siteBaglantilari = siteBaglantilariBul(s.Govde)
			m.qrGoster = false
			m.notice = ""
			return m, nil
		}
		if len(m.host.Network().Peers()) == 0 {
			m.notice, m.isErr = "Bağlantılı sayfa yerelde yok; ağda aramak için bağlı eş gerekiyor.", true
			return m, nil
		}
		m.kodAraniyor = true
		m.notice, m.isErr = "Bağlantılı sayfa ağda aranıyor…", false
		return m, pendingCmd(m, agdaSiteAraCmd(m.ctx, m.host, m.depo, kod))
	case "k":
		// Site paylaşım kodunun QR kodunu göster/gizle (bkz. qr.go).
		if m.seciliSonuc == nil || m.seciliSonuc.Tip != TipSite {
			return m, nil
		}
		m.qrGoster = !m.qrGoster
		return m, nil
	case "c":
		// Yalnızca site sayfalarında anlamlıdır — KaynakID kendisi zaten
		// paylaşım kodudur (bkz. index.go'nun SiteSayfasiBul'u, view.go'nun
		// viewDetay'ının "Kod" satırı, pano.go'nun doc yorumu).
		if m.seciliSonuc == nil || m.seciliSonuc.Tip != TipSite {
			return m, nil
		}
		m.panoKopyalaDizisi = panoyaKopyalaDizisi(m.seciliSonuc.KaynakID)
		m.notice, m.isErr = "Paylaşım kodu panoya kopyalandı.", false
		return m, panoTemizleCmd()
	}
	// Tek haneli bir rakam (1-9): o sıradaki bağlantıyı aç — bkz.
	// view.go'nun bağlantıları numaralandırarak listelemesi.
	if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0] - '1')
		if idx < len(m.baglantilar) {
			return m, baglantiAcCmd(m.baglantilar[idx])
		}
	}
	return m, nil
}

func (m model) keyDurum(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
	}
	return m, nil
}

// keyTakip, ekTakip ekranının tuş işleyicisidir: kayıtlı aramalar arasında
// gezinme, birini yükleyip Ara ekranına geçme (ve okunmuş sayma), ya da
// silme.
func (m model) keyTakip(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.ekran = ekAnaMenu
		return m, nil
	case "up", "k":
		if m.takipIdx > 0 {
			m.takipIdx--
		}
		return m, nil
	case "down", "j":
		if m.takipIdx < len(m.takipListesi)-1 {
			m.takipIdx++
		}
		return m, nil
	case "d":
		if m.takipIdx >= len(m.takipListesi) {
			return m, nil
		}
		secili := m.takipListesi[m.takipIdx]
		if err := m.depo.TakipSil(secili.ID); err != nil {
			m.notice, m.isErr = "Silinemedi: "+err.Error(), true
			return m, nil
		}
		m.takipListesi = append(append([]KayitliArama{}, m.takipListesi[:m.takipIdx]...), m.takipListesi[m.takipIdx+1:]...)
		if m.takipIdx >= len(m.takipListesi) && m.takipIdx > 0 {
			m.takipIdx--
		}
		m.notice, m.isErr = "Kayıtlı arama silindi.", false
		return m, nil
	case "enter":
		if m.takipIdx >= len(m.takipListesi) {
			return m, nil
		}
		secili := m.takipListesi[m.takipIdx]
		_ = m.depo.TakipGoruldu(secili.ID)
		m.aramaIn.SetValue(secili.Sorgu)
		m.aramaIn.CursorEnd()
		m.filtre = secili.Tip
		m.yazarFiltre = secili.YazarKey
		m.yazarFiltreAd = ""
		m.agSonuclari = nil
		m.agAraniyor = false
		m.sonucIdx = 0
		m.sayfaNo = 0
		m.notice = ""
		m.ekran = ekArama
		m.aramaIn.Focus()
		return m, yerelAramaCmd(m.depo, secili.Sorgu, m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)
	}
	return m, nil
}
