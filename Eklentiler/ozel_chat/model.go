package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/ui"
)

// istemci, eklentinin kullandığı AND API çağrılarıdır (*pluginapi.Client);
// testlerde sahtesi kullanılır.
type istemci interface {
	DMHistory() ([]pluginapi.DMRecord, error)
	PollDM() ([]pluginapi.DMMsg, error)
	SendDM(peerID, message string) error
	SendFile(peerID, localPath string) error
	PollFile() ([]pluginapi.FileMsg, error)
	PollConsent() ([]pluginapi.FileConsentReq, error)
	RespondConsent(transferID string, accept bool) error
	Peers() ([]pluginapi.PeerInfo, error)
}

type odak int

const (
	odakListe odak = iota // konuşma listesi
	odakYazi              // seçili konuşmada mesaj yazma
	odakYeni              // yeni konuşma: eş seçimi
	odakDosya             // gönderilecek dosyanın yolu
)

const (
	maxMesajRune = 1000
	// onaySuresi, gelen bir dosya isteğinin ekranda bekleme süresidir — AND
	// isteği 30 sn sonra kendisi reddeder; biraz önce kapatılır.
	onaySuresi      = 28 * time.Second
	yenilemeAraligi = time.Second
	tekrarBekleme   = 2 * time.Second
	maxCanliKayit   = 500
	okunduDosyaAdi  = "ozel_chat_okundu.json"
)

// ─── Mesajlar ────────────────────────────────────────────────────────────────

type gecmisMsg struct {
	kayitlar []pluginapi.DMRecord
	err      error
}
type canliDMMsg struct {
	mesajlar []pluginapi.DMMsg
	err      error
}
type eslerMsg struct {
	esler []pluginapi.PeerInfo
	err   error
}
type onayMsg struct {
	istekler []pluginapi.FileConsentReq
	err      error
}
type onayYanitMsg struct {
	dosya string
	kabul bool
	err   error
}
type dosyaMsg struct {
	dosyalar []pluginapi.FileMsg
	err      error
}
type gonderildiMsg struct {
	peer, metin string
	err         error
}
type dosyaGonderildiMsg struct {
	peer, ad string
	boyut    int64
	err      error
}
type tikMsg time.Time
type yenidenDinleMsg string // "dm" | "onay" | "dosya"

// ─── Komutlar ────────────────────────────────────────────────────────────────

func gecmisCek(c istemci) tea.Cmd {
	return func() tea.Msg { k, err := c.DMHistory(); return gecmisMsg{k, err} }
}

func dmDinle(c istemci) tea.Cmd {
	return func() tea.Msg { m, err := c.PollDM(); return canliDMMsg{m, err} }
}

func eslerCek(c istemci) tea.Cmd {
	return func() tea.Msg { e, err := c.Peers(); return eslerMsg{e, err} }
}

func onayDinle(c istemci) tea.Cmd {
	return func() tea.Msg { r, err := c.PollConsent(); return onayMsg{r, err} }
}

func dosyaDinle(c istemci) tea.Cmd {
	return func() tea.Msg { f, err := c.PollFile(); return dosyaMsg{f, err} }
}

func tik() tea.Cmd {
	return tea.Tick(yenilemeAraligi, func(t time.Time) tea.Msg { return tikMsg(t) })
}

func yenidenDinle(ne string) tea.Cmd {
	return tea.Tick(tekrarBekleme, func(time.Time) tea.Msg { return yenidenDinleMsg(ne) })
}

// ─── Model ───────────────────────────────────────────────────────────────────

type mesaj struct {
	metin string
	zaman time.Time
	giden bool
	olay  bool // dosya alındı/gönderildi gibi yerel bir olay satırı
}

type konusma struct {
	peerID    string
	ad        string // eşin kendi beyan ettiği son ad (doğrulanmamış)
	mesajlar  []mesaj
	son       time.Time
	okunmamis int
}

type bekleyenOnay struct {
	pluginapi.FileConsentReq
	bitis time.Time
}

type model struct {
	c       istemci
	ben     pluginapi.IdentityInfo
	dataDir string

	gecmis   []pluginapi.DMRecord
	canliMod bool // AND geçmiş tutmuyor: yalnızca canlı DM'ler (bkz. gecmisMsg)
	olaylar  map[string][]mesaj
	yeniler  map[string]time.Time // elle başlatılmış, henüz mesajı olmayan konuşmalar
	okundu   map[string]time.Time
	esler    []pluginapi.PeerInfo // bağlı AND düğümleri

	secili   string
	listeIdx int
	yeniIdx  int
	odak     odak
	kaydirma int // mesaj akışında yukarı kaydırılan satır sayısı

	yazi, yeniEs, dosyaYolu textinput.Model

	onaylar      []bekleyenOnay
	gonderiliyor int
	bildirim     string
	hata         bool

	w, h  int
	tikNo int
	simdi func() time.Time
}

func yeniModel(c istemci, ben pluginapi.IdentityInfo, dataDir string) model {
	yazi := textinput.New()
	yazi.Placeholder = "mesajını yaz…"
	yazi.CharLimit = maxMesajRune
	yeniEs := textinput.New()
	yeniEs.Placeholder = "Peer ID (12D3KooW…)"
	yeniEs.CharLimit = 80
	dosya := textinput.New()
	dosya.Placeholder = `C:\Users\…\dosya.pdf`
	dosya.CharLimit = 1024
	for _, in := range []*textinput.Model{&yazi, &yeniEs, &dosya} {
		ui.StyleInput(in)
	}
	return model{
		c: c, ben: ben, dataDir: dataDir,
		olaylar: map[string][]mesaj{}, yeniler: map[string]time.Time{},
		okundu: okunduYukle(dataDir),
		yazi:   yazi, yeniEs: yeniEs, dosyaYolu: dosya,
		simdi: time.Now,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, gecmisCek(m.c), eslerCek(m.c), onayDinle(m.c), dosyaDinle(m.c), tik())
}

// ─── Türetilen veri ──────────────────────────────────────────────────────────

// konusmalar, geçmişten, yerel olaylardan ve elle başlatılan konuşmalardan
// konuşma listesini kurar; en son hareket eden en üstte.
func (m model) konusmalar() []konusma {
	idx := map[string]*konusma{}
	var sira []*konusma
	al := func(p string) *konusma {
		k := idx[p]
		if k == nil {
			k = &konusma{peerID: p}
			idx[p] = k
			sira = append(sira, k)
		}
		return k
	}
	for _, r := range m.gecmis {
		k := al(r.PeerID)
		if !r.Outgoing && r.Name != "" {
			k.ad = pluginui.Safe(r.Name)
		}
		k.mesajlar = append(k.mesajlar, mesaj{metin: pluginui.Safe(r.Text), zaman: r.At, giden: r.Outgoing})
		if !r.Outgoing && r.At.After(m.okundu[r.PeerID]) {
			k.okunmamis++
		}
	}
	for p, olaylar := range m.olaylar {
		k := al(p)
		k.mesajlar = append(k.mesajlar, olaylar...)
	}
	for p, t := range m.yeniler {
		if k := al(p); k.son.Before(t) {
			k.son = t
		}
	}
	out := make([]konusma, 0, len(sira))
	for _, k := range sira {
		sort.SliceStable(k.mesajlar, func(i, j int) bool { return k.mesajlar[i].zaman.Before(k.mesajlar[j].zaman) })
		if n := len(k.mesajlar); n > 0 && k.mesajlar[n-1].zaman.After(k.son) {
			k.son = k.mesajlar[n-1].zaman
		}
		out = append(out, *k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].son.Equal(out[j].son) {
			return out[i].son.After(out[j].son)
		}
		return out[i].peerID < out[j].peerID
	})
	return out
}

// konusmaBul, peerID'nin konuşmasını döner.
func (m model) konusmaBul(peerID string) (konusma, bool) {
	for _, k := range m.konusmalar() {
		if k.peerID == peerID {
			return k, true
		}
	}
	return konusma{}, false
}

// adOf, bir eşin bilinen adıdır; yoksa kısaltılmış Peer ID'si.
func (m model) adOf(peerID string) string {
	for i := len(m.gecmis) - 1; i >= 0; i-- {
		if r := m.gecmis[i]; r.PeerID == peerID && !r.Outgoing && r.Name != "" {
			return pluginui.Safe(r.Name)
		}
	}
	return kisaID(peerID)
}

// okunduIsaretle, seçili konuşma açıkken gelen mesajlarını okundu sayar ve
// bunu diske yazar — Özel Chat yeniden açıldığında yalnızca gerçekten
// görülmemiş mesajlar "yeni" görünsün.
func (m *model) okunduIsaretle() {
	if m.odak != odakYazi || m.secili == "" {
		return
	}
	var son time.Time
	for _, r := range m.gecmis {
		if r.PeerID == m.secili && !r.Outgoing && r.At.After(son) {
			son = r.At
		}
	}
	if son.After(m.okundu[m.secili]) {
		m.okundu[m.secili] = son
		okunduKaydet(m.dataDir, m.okundu)
	}
}

// ─── Update ──────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// Kutu: istem ("❯ ") + metin + imleç; sağ bölmeye taşmadan sığmalı.
		g := max(min(sagW(icW(m.w)), ui.Measure(icW(m.w)))-4, 10)
		m.yazi.Width, m.yeniEs.Width, m.dosyaYolu.Width = g, g, g
		return m, nil

	case tea.KeyMsg:
		return m.tus(msg)

	case gecmisMsg:
		if msg.err != nil {
			if pluginapi.IsNotFound(msg.err) && !m.canliMod {
				m.canliMod = true
				m.bildirim, m.hata = "Bu AND sürümü mesaj geçmişi tutmuyor; yalnızca bu ekran açıkken gelen mesajlar görünür.", false
				return m, dmDinle(m.c)
			}
			return m, nil // geçici hata: bir sonraki tikte yeniden denenir
		}
		m.gecmis = msg.kayitlar
		m.okunduIsaretle()
		return m, nil

	case canliDMMsg:
		if msg.err != nil {
			return m, yenidenDinle("dm")
		}
		for _, d := range msg.mesajlar {
			m.canliEkle(pluginapi.DMRecord{PeerID: d.VerifiedPeerID, Name: adParcasi(d.From), Text: d.Text, At: m.simdi()})
		}
		m.okunduIsaretle()
		return m, dmDinle(m.c)

	case tikMsg:
		m.onaylariTemizle()
		m.tikNo++
		cmds := []tea.Cmd{tik()}
		if !m.canliMod {
			cmds = append(cmds, gecmisCek(m.c))
		}
		if m.tikNo%10 == 0 {
			cmds = append(cmds, eslerCek(m.c))
		}
		return m, tea.Batch(cmds...)

	case eslerMsg:
		if msg.err == nil {
			m.esler = m.esler[:0]
			for _, e := range msg.esler {
				if e.AND && e.PeerID != m.ben.PeerID {
					m.esler = append(m.esler, e)
				}
			}
			sort.Slice(m.esler, func(i, j int) bool { return m.esler[i].PeerID < m.esler[j].PeerID })
		}
		return m, nil

	case onayMsg:
		if msg.err != nil {
			return m, yenidenDinle("onay")
		}
		for _, r := range msg.istekler {
			r.Filename = pluginui.Safe(r.Filename)
			m.onaylar = append(m.onaylar, bekleyenOnay{FileConsentReq: r, bitis: m.simdi().Add(onaySuresi)})
		}
		return m, onayDinle(m.c)

	case onayYanitMsg:
		switch {
		case msg.err != nil:
			m.bildirim, m.hata = "Yanıt iletilemedi: "+msg.err.Error(), true
		case msg.kabul:
			m.bildirim, m.hata = msg.dosya+" kabul edildi, alınıyor…", false
		default:
			m.bildirim, m.hata = msg.dosya+" reddedildi.", false
		}
		return m, nil

	case dosyaMsg:
		if msg.err != nil {
			return m, yenidenDinle("dosya")
		}
		for _, f := range msg.dosyalar {
			ad := pluginui.Safe(f.Filename)
			m.olayEkle(f.From, fmt.Sprintf("↓ dosya alındı: %s (%s)", ad, boyutYaz(f.Size)))
			m.bildirim, m.hata = fmt.Sprintf("%s kaydedildi: %s", ad, pluginui.Safe(f.SavePath)), false
		}
		return m, dosyaDinle(m.c)

	case gonderildiMsg:
		m.gonderiliyor--
		if msg.err != nil {
			m.bildirim, m.hata = "Gönderilemedi: "+msg.err.Error(), true
			if m.yazi.Value() == "" {
				m.yazi.SetValue(msg.metin) // yazılan kaybolmasın
			}
			return m, nil
		}
		if m.canliMod {
			m.canliEkle(pluginapi.DMRecord{PeerID: msg.peer, Text: msg.metin, At: m.simdi(), Outgoing: true})
			return m, nil
		}
		return m, gecmisCek(m.c)

	case dosyaGonderildiMsg:
		m.gonderiliyor--
		if msg.err != nil {
			m.bildirim, m.hata = msg.ad+" gönderilemedi: "+msg.err.Error(), true
			return m, nil
		}
		m.olayEkle(msg.peer, fmt.Sprintf("↑ dosya gönderildi: %s (%s)", msg.ad, boyutYaz(msg.boyut)))
		m.bildirim, m.hata = msg.ad+" gönderildi.", false
		return m, nil

	case yenidenDinleMsg:
		switch msg {
		case "dm":
			return m, dmDinle(m.c)
		case "onay":
			return m, onayDinle(m.c)
		case "dosya":
			return m, dosyaDinle(m.c)
		}
	}
	return m.girdiyeIlet(msg)
}

// girdiyeIlet, mesajı odaktaki giriş kutusuna iletir (imleç yanıp sönmesi vb.).
func (m model) girdiyeIlet(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.odak {
	case odakYazi:
		m.yazi, cmd = m.yazi.Update(msg)
	case odakYeni:
		m.yeniEs, cmd = m.yeniEs.Update(msg)
	case odakDosya:
		m.dosyaYolu, cmd = m.dosyaYolu.Update(msg)
	}
	return m, cmd
}

func (m *model) canliEkle(r pluginapi.DMRecord) {
	m.gecmis = append(m.gecmis, r)
	if over := len(m.gecmis) - maxCanliKayit; over > 0 {
		m.gecmis = m.gecmis[over:]
	}
}

func (m *model) olayEkle(peerID, metin string) {
	m.olaylar[peerID] = append(m.olaylar[peerID], mesaj{metin: metin, zaman: m.simdi(), olay: true})
}

// onaylariTemizle, süresi dolmuş dosya isteklerini düşürür (AND onları
// zaten reddetmiştir).
func (m *model) onaylariTemizle() {
	simdi := m.simdi()
	kalan := m.onaylar[:0]
	for _, o := range m.onaylar {
		if simdi.Before(o.bitis) {
			kalan = append(kalan, o)
		} else {
			m.bildirim, m.hata = o.Filename+" isteğinin süresi doldu, reddedildi.", false
		}
	}
	m.onaylar = kalan
}

// ─── Tuşlar ──────────────────────────────────────────────────────────────────

func (m model) tus(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Bekleyen bir dosya isteği her şeyden önce yanıtlanır. Kabul YALNIZCA
	// "y"/"e" ile: enter kabul etmez — istek, kullanıcı bir mesajı enter ile
	// gönderdiği anda ekrana düşerse dosya yanlışlıkla kabul edilmesin.
	if len(m.onaylar) > 0 {
		switch k.String() {
		case "y", "e":
			return m.onayYanitla(true)
		case "n", "h", "esc":
			return m.onayYanitla(false)
		}
		return m, nil
	}
	switch m.odak {
	case odakYazi:
		return m.tusYazi(k)
	case odakYeni:
		return m.tusYeni(k)
	case odakDosya:
		return m.tusDosya(k)
	default:
		return m.tusListe(k)
	}
}

func (m model) onayYanitla(kabul bool) (tea.Model, tea.Cmd) {
	o := m.onaylar[0]
	m.onaylar = m.onaylar[1:]
	c := m.c
	return m, func() tea.Msg {
		return onayYanitMsg{dosya: o.Filename, kabul: kabul, err: c.RespondConsent(o.TransferID, kabul)}
	}
}

func (m model) tusListe(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	liste := m.konusmalar()
	switch k.String() {
	case "esc", "q":
		return m, tea.Quit
	case "up", "k":
		m.listeIdx = max(m.listeIdx-1, 0)
	case "down", "j":
		m.listeIdx = min(m.listeIdx+1, max(len(liste)-1, 0))
	case "n", "ctrl+n":
		return m.yeniKonusmaAc()
	case "enter", "right", "tab":
		if len(liste) == 0 {
			return m.yeniKonusmaAc()
		}
		m.listeIdx = min(m.listeIdx, len(liste)-1)
		return m.konusmaAc(liste[m.listeIdx].peerID), textinput.Blink
	}
	return m, nil
}

func (m model) konusmaAc(peerID string) model {
	m.secili = peerID
	m.odak = odakYazi
	m.kaydirma = 0
	m.yeniEs.Blur()
	m.dosyaYolu.Blur()
	m.yazi.Focus()
	m.okunduIsaretle()
	for i, k := range m.konusmalar() {
		if k.peerID == peerID {
			m.listeIdx = i
		}
	}
	return m
}

func (m model) yeniKonusmaAc() (tea.Model, tea.Cmd) {
	m.odak = odakYeni
	m.yeniIdx = 0
	m.yeniEs.SetValue("")
	m.yazi.Blur()
	m.yeniEs.Focus()
	return m, tea.Batch(textinput.Blink, eslerCek(m.c))
}

func (m model) tusYazi(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "tab":
		m.odak = odakListe
		m.yazi.Blur()
		return m, nil
	case "ctrl+n":
		return m.yeniKonusmaAc()
	case "ctrl+f":
		m.odak = odakDosya
		m.yazi.Blur()
		m.dosyaYolu.Focus()
		return m, textinput.Blink
	case "pgup":
		m.kaydirma += 5
		return m, nil
	case "pgdown":
		m.kaydirma = max(m.kaydirma-5, 0)
		return m, nil
	case "enter":
		metin := strings.TrimSpace(m.yazi.Value())
		if metin == "" || m.secili == "" {
			return m, nil
		}
		m.yazi.SetValue("")
		m.kaydirma = 0
		m.gonderiliyor++
		c, peer := m.c, m.secili
		return m, func() tea.Msg { return gonderildiMsg{peer: peer, metin: metin, err: c.SendDM(peer, metin)} }
	}
	var cmd tea.Cmd
	m.yazi, cmd = m.yazi.Update(k)
	return m, cmd
}

func (m model) tusYeni(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.yeniEs.Blur()
		if m.secili != "" {
			m.odak = odakYazi
			m.yazi.Focus()
		} else {
			m.odak = odakListe
		}
		return m, nil
	case "up":
		m.yeniIdx = max(m.yeniIdx-1, 0)
		return m, nil
	case "down":
		m.yeniIdx = min(m.yeniIdx+1, max(len(m.esler)-1, 0))
		return m, nil
	case "enter":
		hedef := strings.TrimSpace(m.yeniEs.Value())
		if hedef == "" && len(m.esler) > 0 {
			hedef = m.esler[min(m.yeniIdx, len(m.esler)-1)].PeerID
		}
		if !gecerliPeerID(hedef) {
			m.bildirim, m.hata = "Geçerli bir Peer ID gir ya da listeden bir eş seç.", true
			return m, nil
		}
		if hedef == m.ben.PeerID {
			m.bildirim, m.hata = "Kendine mesaj gönderemezsin.", true
			return m, nil
		}
		if _, vardi := m.konusmaBul(hedef); !vardi {
			m.yeniler[hedef] = m.simdi()
		}
		m.bildirim = ""
		return m.konusmaAc(hedef), textinput.Blink
	}
	var cmd tea.Cmd
	m.yeniEs, cmd = m.yeniEs.Update(k)
	return m, cmd
}

func (m model) tusDosya(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.odak = odakYazi
		m.dosyaYolu.Blur()
		m.yazi.Focus()
		return m, nil
	case "enter":
		yol := dosyaYoluTemizle(m.dosyaYolu.Value())
		bilgi, err := os.Stat(yol)
		switch {
		case yol == "":
			return m, nil
		case err != nil:
			m.bildirim, m.hata = "Dosya bulunamadı: "+yol, true
			return m, nil
		case bilgi.IsDir():
			m.bildirim, m.hata = "Bir klasör değil, dosya seç.", true
			return m, nil
		case bilgi.Size() > pluginapi.MaxFileBytes:
			m.bildirim, m.hata = fmt.Sprintf("Dosya çok büyük (%s, en fazla %s).", boyutYaz(bilgi.Size()), boyutYaz(pluginapi.MaxFileBytes)), true
			return m, nil
		}
		m.dosyaYolu.SetValue("")
		m.odak = odakYazi
		m.dosyaYolu.Blur()
		m.yazi.Focus()
		m.gonderiliyor++
		m.bildirim, m.hata = "", false
		c, peer, ad, boyut := m.c, m.secili, filepath.Base(yol), bilgi.Size()
		return m, func() tea.Msg {
			return dosyaGonderildiMsg{peer: peer, ad: ad, boyut: boyut, err: c.SendFile(peer, yol)}
		}
	}
	var cmd tea.Cmd
	m.dosyaYolu, cmd = m.dosyaYolu.Update(k)
	return m, cmd
}

// ─── Yardımcılar ─────────────────────────────────────────────────────────────

// gecerliPeerID, s'nin bir libp2p Peer ID'sine benzeyip benzemediğidir
// (base58, Ed25519 için "12D3KooW…", eski RSA için "Qm…") — yazım hatalarını
// AND'a sormadan yakalamak için; asıl doğrulamayı AND yapar.
func gecerliPeerID(s string) bool {
	if len(s) < 46 || len(s) > 64 || !(strings.HasPrefix(s, "12D3KooW") || strings.HasPrefix(s, "Qm")) {
		return false
	}
	const base58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, r := range s {
		if !strings.ContainsRune(base58, r) {
			return false
		}
	}
	return true
}

// dosyaYoluTemizle, kullanıcının yazdığı ya da sürükleyip bıraktığı yolu
// düzeltir: çevreleyen tırnaklar atılır, "~" ev dizinine açılır.
func dosyaYoluTemizle(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		s = s[1 : len(s)-1]
	}
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		if ev, err := os.UserHomeDir(); err == nil {
			s = filepath.Join(ev, s[1:])
		}
	}
	return s
}

// adParcasi, dmmgr'nin "ad (12D3KooWAbCd…)" biçimindeki görünen adından
// adı ayırır (eski AND'ın canlı DM'lerinde geçmiş kaydı yoktur).
func adParcasi(from string) string {
	if i := strings.LastIndex(from, " ("); i > 0 && strings.HasSuffix(from, "…)") {
		return from[:i]
	}
	if strings.HasPrefix(from, "12D3KooW") || strings.HasPrefix(from, "Qm") {
		return ""
	}
	return from
}

// kisaID, "12D3KooWAb…XyZ012" biçiminde kısaltılmış Peer ID'dir.
func kisaID(p string) string {
	if len(p) <= 18 {
		return p
	}
	return p[:10] + "…" + p[len(p)-6:]
}

// boyutYaz, bayt sayısını okunur biçimde yazar.
func boyutYaz(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}

// okunduYukle, konuşma başına son okunan mesajın zamanını yükler.
func okunduYukle(dataDir string) map[string]time.Time {
	out := map[string]time.Time{}
	if dataDir == "" {
		return out
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, okunduDosyaAdi)); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func okunduKaydet(dataDir string, okundu map[string]time.Time) {
	if dataDir == "" {
		return
	}
	if data, err := json.Marshal(okundu); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, okunduDosyaAdi), data, 0o600)
	}
}
