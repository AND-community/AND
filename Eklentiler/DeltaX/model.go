package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
)

// ─── Ekranlar ──────────────────────────────────────────────────────────────

type ekran int

const (
	ekAnaMenu ekran = iota
	ekKimlik
	ekGrupListesi
	ekGrupOlustur
	ekGrupPaneli
	ekUyeDavet
	ekDavetler
	ekProjeOlustur
	ekProjePaneli
	ekDurum
	ekCommitMesaj
	ekGunluk
	ekDallar
	ekYeniDal
	ekDiffListesi
	ekDiff
	ekGonderSecim
	ekUzakDallar
	ekPullIste
	ekDosyaOnay
	ekOnay
	ekYoneticiSecim
	ekDosyaListesi
	ekYeniDosya
	ekDosyaDuzenle
	ekDisaAktar
	ekYardim
)

// kisayol, ? yardım kutusunda gösterilen tek bir klavye kısayolunu tanımlar.
type kisayol struct {
	tus      string
	aciklama string
}

// onayTipi, genel onay ekranının (ekOnay) hangi yıkıcı işlem için gösterildiğini belirtir.
type onayTipi int

const (
	onayGrupCikKapat onayTipi = iota
	onayDalDegistir
	onayBirlestir
)

// bekleyenDavet, kabul edilmemiş bir grup davetidir. store.go'daki
// davetleriKaydet/davetleriYukle ile diske de yazılır — eklenti kapatılıp
// yeniden açılsa (ya da AND yeniden başlasa) bile davet kaybolmaz.
type bekleyenDavet struct {
	GrupID       string       `json:"grup_id"`
	GonderenPeer string       `json:"gonderen_peer"`
	Payload      davetPayload `json:"payload"`
}

// ─── tea.Msg tipleri ─────────────────────────────────────────────────────────

type dmPollResultMsg struct {
	msgs []pluginapi.DMMsg
	err  error
}
type filePollResultMsg struct {
	msgs []pluginapi.FileMsg
	err  error
}
type consentPollResultMsg struct {
	reqs []pluginapi.FileConsentReq
	err  error
}
type peersResultMsg struct {
	peers []pluginapi.PeerInfo
	err   error
}
type basitSonuc struct {
	islem string
	err   error
}
type grupOlusturSonucu struct {
	grup Grup
	err  error
}
type kabulSonucu struct {
	grup Grup
	err  error
}
type projeOlusturSonucu struct {
	proje Proje
	err   error
}
type durumSonucu struct {
	durum DurumSonucu
	err   error
}
type commitSonucu struct {
	hash    string
	atlanan int
	err     error
}
type gunlukSonucu struct {
	girdiler []LogGirdisi
}
type dalDegistirSonucu struct {
	dal   string
	kirli bool
	err   error
}
type diffSonucu struct {
	satirlar []DiffSatiri
	err      error
}
type pushSonucu struct {
	hedefAd     string
	nesneSayisi int
	err         error
}
type birlestirSonucu struct {
	catismalar []string
	yeniCommit string
	uzakHash   string
	err        error
}
type grupCikKapatSonucu struct {
	kapatildi bool // true: kurucu grubu kapattı, false: üye gruptan çıktı
	agHatasi  error
	err       error
}
type dosyaListesiSonucu struct {
	liste []string
	err   error
}
type dosyaAcSonucu struct {
	rel    string
	icerik string
	err    error
}
type dosyaKaydetSonucu struct {
	err error
}
type dosyaSilSonucu struct {
	rel string
	err error
}
type disaAktarSonucu struct {
	yol string
	err error
}

// ─── Model ────────────────────────────────────────────────────────────────────

type model struct {
	client   *pluginapi.Client
	identity pluginapi.IdentityInfo
	dataDir  string

	ekran   ekran
	w, h    int
	notice  string
	isErr   bool
	pending bool

	menuIdx int

	gruplar        []Grup
	grupIdx        int
	aktifGrup      *Grup
	grupAdIn       textinput.Model
	grupAramaAktif bool
	grupAramaIn    textinput.Model

	uyeDavetIn textinput.Model

	bekleyenDavetler []bekleyenDavet
	davetIdx         int

	projeler   []Proje
	projeIdx   int
	aktifProje *Proje
	projeAdIn  textinput.Model

	projePanelIdx int

	durum        DurumSonucu
	durumSonraki ekran

	commitMsgIn textinput.Model

	gunluk           []LogGirdisi
	gunlukVP         viewport.Model
	gunlukAramaAktif bool
	gunlukAramaIn    textinput.Model

	yeniDalIn textinput.Model
	dalIdx    int
	dallar    []string

	diffDosyalar []string
	diffIdx      int
	diffVP       viewport.Model
	diffBaslik   string

	pushIdx   int
	pullIdx   int
	uyelerHar []Uye // aktif grubun kendisi hariç üyeleri (push/pull hedef listesi)

	yoneticiHar []Uye
	yoneticiIdx int

	cevrimici map[string]bool

	uzakAnahtarlari []string
	uzakIdx         int

	pendingConsent *pluginapi.FileConsentReq
	onayOnceki     ekran

	sonCatismalar []string

	dosyaListesi    []string
	dosyaIdx        int
	dosyaSilOnayi   string // 'd' ikinci kez basılana kadar silme onayı bekleyen dosya
	dosyaDuzenleYol string
	dosyaDuzenleTA  textarea.Model
	yeniDosyaIn     textinput.Model

	disaAktarIn textinput.Model

	spinner spinner.Model

	yardimGeriEkran ekran

	onayTip       onayTipi
	onayGeriEkran ekran
}

func newModel(c *pluginapi.Client, id pluginapi.IdentityInfo, dataDir string) model {
	grupAdIn := textinput.New()
	grupAdIn.Placeholder = "Grup adı…"
	grupAdIn.CharLimit = 80

	grupAramaIn := textinput.New()
	grupAramaIn.Placeholder = "Ara (grup/kurucu adı)…"
	grupAramaIn.CharLimit = 100

	uyeDavetIn := textinput.New()
	uyeDavetIn.Placeholder = "Davet edilecek kişinin peer ID'si (12D3Koo…)"
	uyeDavetIn.CharLimit = 128

	projeAdIn := textinput.New()
	projeAdIn.Placeholder = "Proje adı…"
	projeAdIn.CharLimit = 80

	commitMsgIn := textinput.New()
	commitMsgIn.Placeholder = "Kayıt (commit) mesajı…"
	commitMsgIn.CharLimit = 200

	yeniDalIn := textinput.New()
	yeniDalIn.Placeholder = "Yeni dal adı…"
	yeniDalIn.CharLimit = 60

	yeniDosyaIn := textinput.New()
	yeniDosyaIn.Placeholder = "Göreli dosya yolu (ör. src/ana.go)…"
	yeniDosyaIn.CharLimit = 240

	gunlukAramaIn := textinput.New()
	gunlukAramaIn.Placeholder = "Ara (mesaj/yazar/hash)…"
	gunlukAramaIn.CharLimit = 100

	disaAktarIn := textinput.New()
	disaAktarIn.Placeholder = "Hedef klasör…"
	disaAktarIn.CharLimit = 300
	disaAktarIn.SetValue(varsayilanDisaAktarKlasoru())

	dosyaDuzenleTA := textarea.New()
	dosyaDuzenleTA.ShowLineNumbers = true

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))

	return model{
		client:           c,
		identity:         id,
		dataDir:          dataDir,
		ekran:            ekAnaMenu,
		grupAdIn:         grupAdIn,
		grupAramaIn:      grupAramaIn,
		uyeDavetIn:       uyeDavetIn,
		projeAdIn:        projeAdIn,
		commitMsgIn:      commitMsgIn,
		yeniDalIn:        yeniDalIn,
		yeniDosyaIn:      yeniDosyaIn,
		dosyaDuzenleTA:   dosyaDuzenleTA,
		gunlukAramaIn:    gunlukAramaIn,
		disaAktarIn:      disaAktarIn,
		cevrimici:        map[string]bool{},
		spinner:          sp,
		bekleyenDavetler: davetleriYukle(dataDir),
	}
}

func (m model) kendiUye() Uye {
	return Uye{Ad: m.identity.Name, PeerID: m.identity.PeerID, PubKey: m.identity.PubKey}
}

func (m model) calismaDir() string {
	return projeCalismaDizini(m.dataDir, m.aktifProje.ID)
}

// bildirGuncelle, arka plandan (zarfIsle, filePollResultMsg) gelen bir
// BİLGİLENDİRME mesajının, kullanıcının henüz görmediği bir HATA mesajının
// üzerine sessizce yazılmasını engeller — sürekli dönen DM/dosya yoklaması bir
// hatayı okumaya fırsat bulamadan değiştirmesin diye. Yeni mesaj kendisi bir
// hataysa (hata=true) her zaman gösterilir; iki hata arasında en yenisi kazanır.
func (m model) bildirGuncelle(mesaj string, hata bool) model {
	if m.isErr && !hata {
		return m
	}
	m.notice, m.isErr = mesaj, hata
	return m
}

// kisayollarIcin, verilen ekranda geçerli olan klavye kısayollarını döndürür
// — ? yardım kutusunun (bkz. Update/viewYardim) tek veri kaynağıdır. Yalnızca
// metin girişi OLMAYAN liste/menü ekranları kapsanır: bir ekran burada yoksa
// (nil döner) o ekranda ? sıradan bir tuş olarak metne yazılır/yok sayılır ve
// yardım kutusu hiç açılmaz.
func (m model) kisayollarIcin(e ekran) []kisayol {
	switch e {
	case ekAnaMenu:
		return []kisayol{{"↑/↓, j/k", "menüde gez"}, {"enter", "seç"}, {"q, esc", "çıkış"}}
	case ekKimlik:
		return []kisayol{{"esc", "geri"}}
	case ekGrupListesi:
		return []kisayol{{"↑/↓", "grup seç"}, {"enter", "grubu aç"}, {"/", "ara"}, {"r", "yenile"}, {"esc", "geri"}}
	case ekGrupPaneli:
		ks := []kisayol{{"↑/↓", "proje seç"}, {"enter", "projeyi aç"}, {"n", "yeni proje"}}
		if m.aktifGrup != nil {
			if m.aktifGrup.yetkiliMi(m.identity.PeerID) {
				ks = append(ks, kisayol{"i", "üye davet et"})
			}
			if m.aktifGrup.benKurucuMuyum(m.identity.PeerID) {
				ks = append(ks, kisayol{"y", "yönetici ata/kaldır"})
			}
			if m.aktifGrup.yetkiliMi(m.identity.PeerID) {
				ks = append(ks, kisayol{"x", "grubu kapat"})
			} else {
				ks = append(ks, kisayol{"x", "gruptan çık"})
			}
		}
		return append(ks, kisayol{"r", "yenile"}, kisayol{"esc", "geri"})
	case ekDavetler:
		return []kisayol{{"↑/↓", "davet seç"}, {"a", "kabul et"}, {"r", "reddet"}, {"esc", "geri"}}
	case ekProjePaneli:
		return []kisayol{{"↑/↓", "işlem seç"}, {"enter", "çalıştır"}, {"esc", "geri"}}
	case ekDurum:
		return []kisayol{{"esc", "geri"}}
	case ekGunluk:
		if m.gunlukAramaAktif {
			return nil
		}
		return []kisayol{{"↑/↓", "kaydır"}, {"/", "ara"}, {"esc", "geri"}}
	case ekDallar:
		return []kisayol{{"↑/↓", "dal seç"}, {"enter", "dala geç"}, {"n", "yeni dal"}, {"esc", "geri"}}
	case ekDiffListesi:
		return []kisayol{{"↑/↓", "dosya seç"}, {"enter", "farkı göster"}, {"esc", "geri"}}
	case ekDiff:
		return []kisayol{{"↑/↓", "kaydır"}, {"esc", "geri"}}
	case ekGonderSecim:
		return []kisayol{{"↑/↓", "üye seç"}, {"enter", "gönder"}, {"a", "çevrimiçi herkese gönder"}, {"esc", "geri"}}
	case ekUzakDallar:
		return []kisayol{{"↑/↓", "uç seç"}, {"enter", "birleştir"}, {"esc", "geri"}}
	case ekPullIste:
		return []kisayol{{"↑/↓", "üye seç"}, {"enter", "çek isteği gönder"}, {"esc", "geri"}}
	case ekYoneticiSecim:
		return []kisayol{{"↑/↓", "üye seç"}, {"enter", "yönetici yap/kaldır"}, {"esc", "geri"}}
	case ekDosyaListesi:
		return []kisayol{{"↑/↓", "dosya seç"}, {"enter", "düzenle"}, {"n", "yeni dosya"}, {"d", "sil (iki kez bas)"}, {"esc", "geri"}}
	}
	return nil
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		dmPollCmd(m.client),
		filePollCmd(m.client),
		consentPollCmd(m.client),
		peersPollCmd(m.client),
	)
}

// ─── Async komutlar ───────────────────────────────────────────────────────────

func dmPollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		msgs, err := c.PollDM()
		return dmPollResultMsg{msgs: msgs, err: err}
	}
}

func dmPollCmdAfter(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		msgs, err := c.PollDM()
		return dmPollResultMsg{msgs: msgs, err: err}
	}
}

func filePollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		msgs, err := c.PollFile()
		return filePollResultMsg{msgs: msgs, err: err}
	}
}

func filePollCmdAfter(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		msgs, err := c.PollFile()
		return filePollResultMsg{msgs: msgs, err: err}
	}
}

func consentPollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		reqs, err := c.PollConsent()
		return consentPollResultMsg{reqs: reqs, err: err}
	}
}

func peersPollCmd(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		peers, err := c.Peers()
		return peersResultMsg{peers: peers, err: err}
	}
}

func peersPollCmdAfter(c *pluginapi.Client) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(5 * time.Second)
		peers, err := c.Peers()
		return peersResultMsg{peers: peers, err: err}
	}
}

func onayCmd(c *pluginapi.Client, transferID string, accept bool) tea.Cmd {
	return func() tea.Msg {
		return basitSonuc{islem: "onay", err: c.RespondConsent(transferID, accept)}
	}
}

func grupOlusturCmd(dataDir, ad string, kendi Uye) tea.Cmd {
	return func() tea.Msg {
		sir, err := sirUret()
		if err != nil {
			return grupOlusturSonucu{err: err}
		}
		g := Grup{
			ID: rastgeleID(), Ad: ad, Sir: sir,
			KurucuAd: kendi.Ad, KurucuPeer: kendi.PeerID,
			Uyeler: []Uye{kendi}, OlusturmaTS: time.Now().UTC(),
		}
		if err := grupKaydet(dataDir, g); err != nil {
			return grupOlusturSonucu{err: err}
		}
		return grupOlusturSonucu{grup: g}
	}
}

func davetGonderCmd(c *pluginapi.Client, hedefPeer string, g Grup, kendi Uye) tea.Cmd {
	return func() tea.Msg {
		return basitSonuc{islem: "davet", err: davetGonder(c, hedefPeer, g, kendi)}
	}
}

func kabulCmd(c *pluginapi.Client, dataDir string, davet bekleyenDavet, kendi Uye, kabul bool) tea.Cmd {
	return func() tea.Msg {
		if !kabul {
			return kabulSonucu{err: davetYanitGonder(c, davet.GonderenPeer, davet.GrupID, false, kendi)}
		}
		g := Grup{
			ID: davet.GrupID, Ad: davet.Payload.GrupAd, Sir: davet.Payload.Sir,
			KurucuAd: davet.Payload.Kurucu.Ad, KurucuPeer: davet.Payload.Kurucu.PeerID,
			Yoneticiler: davet.Payload.Yoneticiler,
			Uyeler:      davet.Payload.Uyeler, OlusturmaTS: time.Now().UTC(),
		}
		g.uyeEkleGuncelle(kendi)
		if err := grupKaydet(dataDir, g); err != nil {
			return kabulSonucu{err: err}
		}
		err := davetYanitGonder(c, davet.GonderenPeer, davet.GrupID, true, kendi)
		return kabulSonucu{grup: g, err: err}
	}
}

func rosterYayinCmd(c *pluginapi.Client, g Grup, kendi Uye) tea.Cmd {
	return func() tea.Msg {
		if hatalar := uyeGuncellemesiYayinla(c, g, kendi); len(hatalar) > 0 {
			return basitSonuc{islem: "roster", err: fmt.Errorf("%d üyeye ulaşılamadı", len(hatalar))}
		}
		return basitSonuc{islem: "roster"}
	}
}

// grupTerkEtCmd, gruptan ayrılma ya da (kurucu/yönetici durumunda) grubu
// tamamen kapatma işlemini yürütür: önce ilgili taraflar ağ üzerinden
// bilgilendirilir (en iyi çaba — ulaşılamayanlar agHatasi olarak
// raporlanır), ardından grubun yerel kaydı silinir. Grubun proje verileri
// (nesneler, çalışma klasörü) kasıtlı olarak diskte bırakılır.
func grupTerkEtCmd(c *pluginapi.Client, dataDir string, g Grup, kendi Uye) tea.Cmd {
	return func() tea.Msg {
		kapatildi := g.yetkiliMi(kendi.PeerID)
		var agHatasi error
		if kapatildi {
			if hatalar := grupKapatBildir(c, g, kendi); len(hatalar) > 0 {
				agHatasi = fmt.Errorf("%d üyeye ulaşılamadı", len(hatalar))
			}
		} else if hatalar := ayrilBildir(c, g, kendi); len(hatalar) > 0 {
			agHatasi = fmt.Errorf("%d yetkiliye ulaşılamadı", len(hatalar))
		}
		if err := grupSil(dataDir, g.ID); err != nil {
			return grupCikKapatSonucu{kapatildi: kapatildi, err: err}
		}
		return grupCikKapatSonucu{kapatildi: kapatildi, agHatasi: agHatasi}
	}
}

func projeOlusturCmd(dataDir, grupID, ad string) tea.Cmd {
	return func() tea.Msg {
		p, err := projeOlustur(dataDir, grupID, ad)
		return projeOlusturSonucu{proje: p, err: err}
	}
}

func durumCmd(dataDir, projeID, calismaDir, head string) tea.Cmd {
	return func() tea.Msg {
		headTree := headTreeOf(dataDir, projeID, head)
		d, err := durumHesapla(dataDir, projeID, calismaDir, headTree)
		return durumSonucu{durum: d, err: err}
	}
}

func headTreeOf(dataDir, projeID, head string) string {
	if head == "" {
		return ""
	}
	c, err := commitOku(dataDir, projeID, head)
	if err != nil {
		return ""
	}
	return c.Agac
}

func commitCmd(dataDir, projeID, calismaDir, head, yazar, mesaj string) tea.Cmd {
	return func() tea.Msg {
		yeniTree, _, atlanan, err := agacOlustur(dataDir, projeID, calismaDir)
		if err != nil {
			return commitSonucu{err: err}
		}
		var ebeveynler []string
		if head != "" {
			ebeveynler = []string{head}
		}
		if ikinci, birlesmeVar := birlesmeBekleniyorMu(dataDir, projeID); birlesmeVar {
			ebeveynler = append(ebeveynler, ikinci)
		} else if head != "" {
			if hc, err := commitOku(dataDir, projeID, head); err == nil && hc.Agac == yeniTree {
				return commitSonucu{err: fmt.Errorf("kaydedilecek değişiklik yok")}
			}
		}
		hash, err := commitOlustur(dataDir, projeID, yeniTree, ebeveynler, yazar, mesaj)
		if err != nil {
			return commitSonucu{err: err}
		}
		birlesmeTemizle(dataDir, projeID)
		return commitSonucu{hash: hash, atlanan: len(atlanan)}
	}
}

func gunlukCmd(dataDir, projeID, head string) tea.Cmd {
	return func() tea.Msg {
		return gunlukSonucu{girdiler: gunlukAl(dataDir, projeID, head, 200)}
	}
}

func dalDegistirCmd(dataDir, projeID, calismaDir, mevcutHead, hedefDal, hedefHead string) tea.Cmd {
	return func() tea.Msg {
		mevcutTree := headTreeOf(dataDir, projeID, mevcutHead)
		durum, err := durumHesapla(dataDir, projeID, calismaDir, mevcutTree)
		if err != nil {
			return dalDegistirSonucu{err: err}
		}
		if !durum.temizMi() {
			return dalDegistirSonucu{kirli: true}
		}
		hedefTree := headTreeOf(dataDir, projeID, hedefHead)
		if err := dalaGec(dataDir, projeID, calismaDir, hedefTree); err != nil {
			return dalDegistirSonucu{err: err}
		}
		return dalDegistirSonucu{dal: hedefDal}
	}
}

func diffCmd(dataDir, projeID, calismaDir, head, yol string) tea.Cmd {
	return func() tea.Msg {
		headTree := headTreeOf(dataDir, projeID, head)
		satirlar, err := dosyaFarkiHesapla(dataDir, projeID, calismaDir, headTree, yol)
		return diffSonucu{satirlar: satirlar, err: err}
	}
}

func dosyaListesiCmd(calismaDir string) tea.Cmd {
	return func() tea.Msg {
		liste, err := dosyaListesiOku(calismaDir)
		return dosyaListesiSonucu{liste: liste, err: err}
	}
}

func dosyaAcCmd(calismaDir, rel string) tea.Cmd {
	return func() tea.Msg {
		icerik, err := dosyaIcerigiOku(calismaDir, rel)
		return dosyaAcSonucu{rel: rel, icerik: icerik, err: err}
	}
}

func dosyaKaydetCmd(calismaDir, rel, icerik string) tea.Cmd {
	return func() tea.Msg {
		return dosyaKaydetSonucu{err: dosyaIcerigiYaz(calismaDir, rel, icerik)}
	}
}

func dosyaSilCmd(calismaDir, rel string) tea.Cmd {
	return func() tea.Msg {
		return dosyaSilSonucu{rel: rel, err: dosyaSil(calismaDir, rel)}
	}
}

func disaAktarCmd(dataDir, projeID, projeAd, hedefKlasor string) tea.Cmd {
	return func() tea.Msg {
		yol, err := projeDisaAktar(dataDir, projeID, projeAd, hedefKlasor)
		return disaAktarSonucu{yol: yol, err: err}
	}
}

func pushCmd(c *pluginapi.Client, dataDir string, g Grup, p Proje, hedefUye Uye, bilinenBaslangic string) tea.Cmd {
	return func() tea.Msg {
		n, err := pushYap(c, dataDir, g, p, hedefUye, bilinenBaslangic)
		return pushSonucu{hedefAd: hedefUye.Ad, nesneSayisi: n, err: err}
	}
}

func pullIsteCmd(c *pluginapi.Client, hedefUye Uye, g Grup, p Proje, kendi Uye) tea.Cmd {
	return func() tea.Msg {
		return basitSonuc{islem: "pull_iste", err: pullIste(c, hedefUye, g, p, kendi)}
	}
}

func birlestirCmd(dataDir, projeID, calismaDir, yerelHead, uzakHead, yazar string) tea.Cmd {
	return func() tea.Msg {
		yerelCommit, err := commitOku(dataDir, projeID, yerelHead)
		if err != nil {
			return birlestirSonucu{err: err}
		}
		durum, err := durumHesapla(dataDir, projeID, calismaDir, yerelCommit.Agac)
		if err != nil {
			return birlestirSonucu{err: err}
		}
		if !durum.temizMi() {
			return birlestirSonucu{err: fmt.Errorf("önce mevcut değişiklikleri kaydet")}
		}

		uzakCommit, err := commitOku(dataDir, projeID, uzakHead)
		if err != nil {
			return birlestirSonucu{err: fmt.Errorf("uzak commit bulunamadı — önce Gönder/Çek ile nesneleri al: %w", err)}
		}

		temel := ortakAta(dataDir, projeID, yerelHead, uzakHead)
		if temel == uzakHead {
			return birlestirSonucu{err: fmt.Errorf("zaten güncel")}
		}
		if temel == yerelHead {
			if err := dalaGec(dataDir, projeID, calismaDir, uzakCommit.Agac); err != nil {
				return birlestirSonucu{err: err}
			}
			return birlestirSonucu{yeniCommit: uzakHead}
		}

		temelTree := headTreeOf(dataDir, projeID, temel)
		catismalar, err := birlestirmeUygula(dataDir, projeID, calismaDir, temelTree, yerelCommit.Agac, uzakCommit.Agac)
		if err != nil {
			return birlestirSonucu{err: err}
		}
		if len(catismalar) > 0 {
			_ = birlesmeIsaretle(dataDir, projeID, uzakHead)
			return birlestirSonucu{catismalar: catismalar, uzakHash: uzakHead}
		}
		yeniTree, _, _, err := agacOlustur(dataDir, projeID, calismaDir)
		if err != nil {
			return birlestirSonucu{err: err}
		}
		yeniHash, err := commitOlustur(dataDir, projeID, yeniTree, []string{yerelHead, uzakHead}, yazar, "Birleştirme: "+kisaHash(uzakHead))
		if err != nil {
			return birlestirSonucu{err: err}
		}
		return birlestirSonucu{yeniCommit: yeniHash}
	}
}

// ─── DM protokol zarfı işleme ───────────────────────────────────────────────

func (m model) zarfIsle(z kontrolZarfi) (model, tea.Cmd) {
	switch z.Tip {
	case mDavet, mUyeGuncelle:
		var dp davetPayload
		if json.Unmarshal(z.Payload, &dp) != nil {
			return m, nil
		}
		if g, err := grupOku(m.dataDir, z.GrupID); err == nil {
			// Zaten bildiğimiz bir grup için gelen güncelleme, yalnızca zaten
			// güvendiğimiz bir yetkilinin (kurucu ya da yönetici) peer
			// ID'siyle eşleşiyorsa kabul edilir. DM API'si gönderenin gerçek
			// peer ID'sini doğrulamaz (yalnızca kendi beyan ettiği bir
			// görünen ad taşır) — bu kontrol olmadan gruba hiç üye olmayan
			// biri bile sahte bir üye listesi/isim göndererek rostere el
			// koyabilir.
			if !g.yetkiliMi(dp.Gonderen.PeerID) {
				return m, nil
			}
			g.Uyeler = dp.Uyeler
			g.Yoneticiler = dp.Yoneticiler
			g.Ad = dp.GrupAd
			_ = grupKaydet(m.dataDir, g)
			if m.aktifGrup != nil && m.aktifGrup.ID == g.ID {
				*m.aktifGrup = g
			}
			m = m.bildirGuncelle("\""+safe(dp.GrupAd)+"\" grubunun üye listesi güncellendi.", false)
			return m, nil
		}
		if z.Tip == mUyeGuncelle {
			return m, nil
		}
		for _, d := range m.bekleyenDavetler {
			if d.GrupID == z.GrupID {
				return m, nil
			}
		}
		m.bekleyenDavetler = append(m.bekleyenDavetler, bekleyenDavet{GrupID: z.GrupID, GonderenPeer: dp.Gonderen.PeerID, Payload: dp})
		_ = davetleriKaydet(m.dataDir, m.bekleyenDavetler)
		m = m.bildirGuncelle("Yeni grup daveti: "+safe(dp.GrupAd)+" (davet eden: "+safe(dp.Gonderen.Ad)+")", false)
		return m, nil

	case mDavetYanit:
		var yp davetYanitPayload
		if json.Unmarshal(z.Payload, &yp) != nil {
			return m, nil
		}
		g, err := grupOku(m.dataDir, z.GrupID)
		if err != nil || !g.yetkiliMi(m.identity.PeerID) || !yp.Kabul {
			return m, nil
		}
		g.uyeEkleGuncelle(yp.Uye)
		_ = grupKaydet(m.dataDir, g)
		if m.aktifGrup != nil && m.aktifGrup.ID == g.ID {
			*m.aktifGrup = g
		}
		m = m.bildirGuncelle(safe(yp.Uye.Ad)+" \""+safe(g.Ad)+"\" grubuna katıldı.", false)
		return m, rosterYayinCmd(m.client, g, m.kendiUye())

	case mPushBildir:
		var bp pushBildirPayload
		if json.Unmarshal(z.Payload, &bp) == nil {
			m = m.bildirGuncelle(fmt.Sprintf("Bir üye değişiklik gönderiyor (%s, %d nesne) — kabul edilince Uzak Dallar'da görünecek.", safe(bp.Dal), bp.NesneSayisi), false)
		}
		return m, nil

	case mPullIstek:
		var pp pullIstekPayload
		if json.Unmarshal(z.Payload, &pp) != nil {
			return m, nil
		}
		g, gerr := grupOku(m.dataDir, z.GrupID)
		p, perr := projeOku(m.dataDir, z.ProjeID)
		if gerr != nil || perr != nil || !g.uyeMi(pp.Isteyen.PeerID) {
			return m, nil
		}
		return m, pushCmd(m.client, m.dataDir, g, p, pp.Isteyen, pp.BilinenHash)

	case mAyril:
		var ap ayrilPayload
		if json.Unmarshal(z.Payload, &ap) != nil {
			return m, nil
		}
		g, err := grupOku(m.dataDir, z.GrupID)
		if err != nil || !g.yetkiliMi(m.identity.PeerID) || !g.uyeMi(ap.Uye.PeerID) {
			return m, nil
		}
		g.uyeCikar(ap.Uye.PeerID)
		g.yoneticiDurumunuDegistir(ap.Uye, false) // ayrılan bir yöneticiyse listeden de düşür
		_ = grupKaydet(m.dataDir, g)
		if m.aktifGrup != nil && m.aktifGrup.ID == g.ID {
			*m.aktifGrup = g
		}
		m = m.bildirGuncelle(safe(ap.Uye.Ad)+" \""+safe(g.Ad)+"\" grubundan ayrıldı.", false)
		return m, rosterYayinCmd(m.client, g, m.kendiUye())

	case mGrupKapat:
		var kp grupKapatPayload
		if json.Unmarshal(z.Payload, &kp) != nil {
			return m, nil
		}
		g, err := grupOku(m.dataDir, z.GrupID)
		if err != nil || !g.yetkiliMi(kp.Gonderen.PeerID) {
			return m, nil
		}
		_ = grupSil(m.dataDir, g.ID)
		if m.aktifGrup != nil && m.aktifGrup.ID == g.ID {
			m.aktifGrup = nil
			m.ekran = ekGrupListesi
		}
		m.gruplar = gruplariListele(m.dataDir)
		m.notice = "\"" + safe(g.Ad) + "\" grubu kurucusu tarafından kapatıldı."
		m.isErr = true
		return m, nil
	}
	return m, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.gunlukVP.Width = msg.Width - 4
		m.gunlukVP.Height = msg.Height - 8
		m.diffVP.Width = msg.Width - 4
		m.diffVP.Height = msg.Height - 8
		m.dosyaDuzenleTA.SetWidth(msg.Width - 4)
		m.dosyaDuzenleTA.SetHeight(msg.Height - 8)
		return m, nil

	case spinner.TickMsg:
		if !m.pending {
			return m, nil // işlem bitti — tik zincirini burada durdur, boşuna dönmeye devam etmesin
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case dmPollResultMsg:
		if msg.err != nil {
			return m, dmPollCmdAfter(m.client)
		}
		var cmds []tea.Cmd
		for _, dm := range msg.msgs {
			if z, ok := zarfCoz(dm.Text); ok {
				var cmd tea.Cmd
				m, cmd = m.zarfIsle(z)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		cmds = append(cmds, dmPollCmd(m.client))
		return m, tea.Batch(cmds...)

	case filePollResultMsg:
		if msg.err != nil {
			return m, filePollCmdAfter(m.client)
		}
		for _, fm := range msg.msgs {
			if !strings.HasPrefix(filepath.Base(fm.Filename), "projeag-") {
				continue
			}
			_, projeID, manifest, sayisi, err := pushPaketiIsle(m.dataDir, fm.SavePath, fm.From)
			_ = os.Remove(fm.SavePath)
			if err != nil {
				m = m.bildirGuncelle("Paket işlenemedi: "+describeErr(err), true)
				continue
			}
			if p, perr := projeOku(m.dataDir, projeID); perr == nil {
				if p.Uzaklar == nil {
					p.Uzaklar = map[string]string{}
				}
				if manifest.HeadHash != "" {
					p.Uzaklar[fm.From+"/"+manifest.Dal] = manifest.HeadHash
				}
				_ = projeKaydet(m.dataDir, p)
				if m.aktifProje != nil && m.aktifProje.ID == projeID {
					*m.aktifProje = p
				}
			}
			m = m.bildirGuncelle(fmt.Sprintf("%d yeni nesne alındı (%s) — Uzak Dallar'dan birleştirebilirsin.", sayisi, safe(manifest.Dal)), false)
		}
		return m, filePollCmd(m.client)

	case consentPollResultMsg:
		if msg.err != nil || len(msg.reqs) == 0 {
			return m, consentPollCmd(m.client)
		}
		req := msg.reqs[0]
		if strings.HasPrefix(req.Filename, "projeag-") && paketGonderenGuvenilirMi(m.dataDir, req.Filename, req.SenderID) {
			return m, tea.Batch(onayCmd(m.client, req.TransferID, true), consentPollCmd(m.client))
		}
		m.pendingConsent = &req
		m.onayOnceki = m.ekran
		m.ekran = ekDosyaOnay
		return m, nil

	case peersResultMsg:
		if msg.err == nil {
			nc := make(map[string]bool, len(msg.peers))
			for _, p := range msg.peers {
				nc[p.PeerID] = true
			}
			m.cevrimici = nc
		}
		return m, peersPollCmdAfter(m.client)

	case basitSonuc:
		m.pending = false
		switch msg.islem {
		case "davet":
			if msg.err != nil {
				m.notice, m.isErr = "Davet gönderilemedi: "+describeErr(msg.err), true
			} else {
				m.notice, m.isErr = "Davet gönderildi.", false
			}
		case "pull_iste":
			if msg.err != nil {
				m.notice, m.isErr = "Çek isteği gönderilemedi: "+describeErr(msg.err), true
			} else {
				m.notice, m.isErr = "Çek isteği gönderildi — karşı taraf çevrimiçiyse kısa süre içinde değişiklikler gelecek.", false
			}
		case "roster":
			if msg.err != nil {
				m.notice, m.isErr = "Üye listesi güncellemesi bazı üyelere ulaşmadı.", true
			}
		}
		return m, nil

	case grupOlusturSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Grup oluşturulamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.notice, m.isErr = "\""+msg.grup.Ad+"\" grubu oluşturuldu.", false
		m.gruplar = gruplariListele(m.dataDir)
		m.ekran = ekGrupListesi
		return m, nil

	case kabulSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "İşlem hatası: "+describeErr(msg.err), true
			return m, nil
		}
		if msg.grup.ID != "" {
			m.notice, m.isErr = "\""+msg.grup.Ad+"\" grubuna katıldın.", false
			m.gruplar = gruplariListele(m.dataDir)
		} else {
			m.notice, m.isErr = "Davet reddedildi.", false
		}
		return m, nil

	case projeOlusturSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Proje oluşturulamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.notice, m.isErr = "\""+msg.proje.Ad+"\" projesi oluşturuldu.", false
		m.projeler = projeleriListele(m.dataDir, m.aktifGrup.ID)
		m.ekran = ekGrupPaneli
		return m, nil

	case durumSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Durum hesaplanamadı: "+describeErr(msg.err), true
			m.ekran = ekProjePaneli
			return m, nil
		}
		m.durum = msg.durum
		if m.durumSonraki == ekDiffListesi {
			var dosyalar []string
			dosyalar = append(dosyalar, msg.durum.Degisen...)
			dosyalar = append(dosyalar, msg.durum.Eklenen...)
			sort.Strings(dosyalar)
			m.diffDosyalar = dosyalar
			m.diffIdx = 0
			m.ekran = ekDiffListesi
			return m, nil
		}
		m.ekran = ekDurum
		return m, nil

	case commitSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Kayıt oluşturulamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.aktifProje.Dallar[m.aktifProje.AktifDal] = msg.hash
		_ = projeKaydet(m.dataDir, *m.aktifProje)
		if msg.atlanan > 0 {
			m.notice = fmt.Sprintf("Kayıt oluşturuldu: %s (%d büyük dosya atlandı)", kisaHash(msg.hash), msg.atlanan)
		} else {
			m.notice = "Kayıt oluşturuldu: " + kisaHash(msg.hash)
		}
		m.isErr = false
		m.commitMsgIn.SetValue("")
		m.ekran = ekProjePaneli
		return m, nil

	case gunlukSonucu:
		m.pending = false
		m.gunluk = msg.girdiler
		m.gunlukAramaAktif = false
		m.gunlukAramaIn.SetValue("")
		m.gunlukAramaIn.Blur()
		m.gunlukVP.SetContent(gunlukMetni(msg.girdiler))
		m.gunlukVP.GotoTop()
		m.ekran = ekGunluk
		return m, nil

	case dalDegistirSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Dal değiştirilemedi: "+describeErr(msg.err), true
			return m, nil
		}
		if msg.kirli {
			m.notice, m.isErr = "Önce mevcut değişiklikleri kaydet (kaydedilmemiş dosyalar var).", true
			return m, nil
		}
		m.aktifProje.AktifDal = msg.dal
		_ = projeKaydet(m.dataDir, *m.aktifProje)
		m.notice, m.isErr = "Dal değiştirildi: "+msg.dal, false
		m.ekran = ekProjePaneli
		return m, nil

	case diffSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = describeErr(msg.err), true
			m.ekran = ekDiffListesi
			return m, nil
		}
		m.diffVP.SetContent(diffMetni(msg.satirlar))
		m.diffVP.GotoTop()
		m.ekran = ekDiff
		return m, nil

	case pushSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = safe(msg.hedefAd)+": gönderilemedi — "+describeErr(msg.err), true
		} else {
			m.notice, m.isErr = fmt.Sprintf("%s kullanıcısına %d nesne gönderildi.", safe(msg.hedefAd), msg.nesneSayisi), false
			if m.aktifProje != nil {
				p, err := projeOku(m.dataDir, m.aktifProje.ID)
				if err == nil {
					*m.aktifProje = p
				}
			}
		}
		return m, nil

	case birlestirSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Birleştirme hatası: "+describeErr(msg.err), true
			return m, nil
		}
		if len(msg.catismalar) > 0 {
			m.sonCatismalar = msg.catismalar
			m.notice = fmt.Sprintf("%d dosyada çakışma var — dosyaları elle düzenleyip Kaydet'e bas.", len(msg.catismalar))
			m.isErr = true
			m.ekran = ekProjePaneli
			return m, nil
		}
		m.aktifProje.Dallar[m.aktifProje.AktifDal] = msg.yeniCommit
		_ = projeKaydet(m.dataDir, *m.aktifProje)
		m.notice, m.isErr = "Birleştirme tamamlandı: "+kisaHash(msg.yeniCommit), false
		m.ekran = ekProjePaneli
		return m, nil

	case grupCikKapatSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "İşlem başarısız: "+describeErr(msg.err), true
			return m, nil
		}
		m.aktifGrup = nil
		m.aktifProje = nil
		m.gruplar = gruplariListele(m.dataDir)
		if m.grupIdx >= len(m.gruplar) {
			m.grupIdx = max0(len(m.gruplar) - 1)
		}
		m.ekran = ekGrupListesi
		if msg.kapatildi {
			m.notice = "Grup kapatıldı."
		} else {
			m.notice = "Gruptan çıkıldı."
		}
		if msg.agHatasi != nil {
			m.notice += " (uyarı: " + describeErr(msg.agHatasi) + ")"
		}
		m.isErr = false
		return m, nil

	case dosyaListesiSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Dosya listesi alınamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.dosyaListesi = msg.liste
		if m.dosyaIdx >= len(m.dosyaListesi) {
			m.dosyaIdx = max0(len(m.dosyaListesi) - 1)
		}
		m.dosyaSilOnayi = ""
		m.ekran = ekDosyaListesi
		return m, nil

	case dosyaAcSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Dosya açılamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.dosyaDuzenleYol = msg.rel
		m.dosyaDuzenleTA.SetValue(msg.icerik)
		m.dosyaDuzenleTA.Focus()
		m.ekran = ekDosyaDuzenle
		return m, nil

	case dosyaKaydetSonucu:
		if msg.err != nil {
			m.notice, m.isErr = "Kaydedilemedi: "+describeErr(msg.err), true
			return m, nil
		}
		m.notice, m.isErr = "Kaydedildi: "+m.dosyaDuzenleYol, false
		return m, nil

	case dosyaSilSonucu:
		if msg.err != nil {
			m.notice, m.isErr = "Silinemedi: "+describeErr(msg.err), true
			return m, nil
		}
		m.notice, m.isErr = "Silindi: "+msg.rel, false
		return m, dosyaListesiCmd(m.calismaDir())

	case disaAktarSonucu:
		m.pending = false
		if msg.err != nil {
			m.notice, m.isErr = "Dışa aktarılamadı: "+describeErr(msg.err), true
			return m, nil
		}
		m.notice, m.isErr = "Dışa aktarıldı: "+msg.yol, false
		m.ekran = ekProjePaneli
		return m, nil

	case tea.KeyMsg:
		if m.ekran == ekYardim {
			m.ekran = m.yardimGeriEkran
			return m, nil
		}
		if msg.String() == "?" && len(m.kisayollarIcin(m.ekran)) > 0 {
			m.yardimGeriEkran = m.ekran
			m.ekran = ekYardim
			return m, nil
		}
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.ekran {
	case ekGrupOlustur:
		m.grupAdIn, cmd = m.grupAdIn.Update(msg)
	case ekGrupListesi:
		if m.grupAramaAktif {
			m.grupAramaIn, cmd = m.grupAramaIn.Update(msg)
		}
	case ekUyeDavet:
		m.uyeDavetIn, cmd = m.uyeDavetIn.Update(msg)
	case ekProjeOlustur:
		m.projeAdIn, cmd = m.projeAdIn.Update(msg)
	case ekCommitMesaj:
		m.commitMsgIn, cmd = m.commitMsgIn.Update(msg)
	case ekYeniDal:
		m.yeniDalIn, cmd = m.yeniDalIn.Update(msg)
	case ekGunluk:
		if m.gunlukAramaAktif {
			m.gunlukAramaIn, cmd = m.gunlukAramaIn.Update(msg)
		} else {
			m.gunlukVP, cmd = m.gunlukVP.Update(msg)
		}
	case ekDiff:
		m.diffVP, cmd = m.diffVP.Update(msg)
	case ekYeniDosya:
		m.yeniDosyaIn, cmd = m.yeniDosyaIn.Update(msg)
	case ekDosyaDuzenle:
		m.dosyaDuzenleTA, cmd = m.dosyaDuzenleTA.Update(msg)
	case ekDisaAktar:
		m.disaAktarIn, cmd = m.disaAktarIn.Update(msg)
	}
	return m, cmd
}

func gunlukMetni(girdiler []LogGirdisi) string {
	var sb strings.Builder
	for _, g := range girdiler {
		birlesme := ""
		if len(g.Commit.Ebeveynler) > 1 {
			birlesme = stMod.Render(" [birleştirme]")
		}
		sb.WriteString(stBold.Render(kisaHash(g.Hash)) + "  " +
			stDim.Render(g.Commit.TS.Local().Format("2006-01-02 15:04")) + "  " +
			g.Commit.Yazar + birlesme + "\n")
		sb.WriteString("  " + safe(g.Commit.Mesaj) + "\n\n")
	}
	if len(girdiler) == 0 {
		sb.WriteString(stMuted.Render("Henüz kayıt yok.\n"))
	}
	return sb.String()
}

func diffMetni(satirlar []DiffSatiri) string {
	var sb strings.Builder
	for _, s := range satirlar {
		switch s.Tip {
		case '+':
			sb.WriteString(stAdd.Render("+ "+safe(s.Metin)) + "\n")
		case '-':
			sb.WriteString(stDel.Render("- "+safe(s.Metin)) + "\n")
		default:
			sb.WriteString(stDim.Render("  "+safe(s.Metin)) + "\n")
		}
	}
	if len(satirlar) == 0 {
		sb.WriteString(stMuted.Render("Fark yok.\n"))
	}
	return sb.String()
}
