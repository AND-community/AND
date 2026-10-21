package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/protokol"
)

const (
	alice = "12D3KooWAzizeAbCdEfGhJkLmNpQrStUvWxYz123456789abcde"
	bob   = "12D3KooWBobAbCdEfGhJkLmNpQrStUvWxYz123456789abcdefg"
	ben   = "12D3KooWBenAbCdEfGhJkLmNpQrStUvWxYz123456789abcdefg"
)

type sahteIstemci struct {
	mu         sync.Mutex
	gecmis     []pluginapi.DMRecord
	gecmisErr  error
	sendErr    error
	gonderilen []string
	dosyalar   []string
	yanitlar   []string
	esler      []pluginapi.PeerInfo
	// sirali: SendDMQueued ile gönderilenler; okunduYok: okundu bilgisi
	// tutmayan eski bir AND; okundu: çekirdeğe bildirilen işaretler.
	sirali    []string
	okunduYok bool
	okundu    map[string]time.Time
}

func (s *sahteIstemci) SendDMQueued(peerID, message string) error {
	if err := s.SendDM(peerID, message); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sirali = append(s.sirali, peerID+":"+message)
	return nil
}
func (s *sahteIstemci) DMReadMarks() (map[string]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.okunduYok {
		return nil, &pluginapi.APIError{Status: 404}
	}
	out := map[string]time.Time{}
	for p, t := range s.okundu {
		out[p] = t
	}
	return out, nil
}
func (s *sahteIstemci) MarkDMRead(peerID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.okundu == nil {
		s.okundu = map[string]time.Time{}
	}
	s.okundu[peerID] = at
	return nil
}

func (s *sahteIstemci) DMHistory() ([]pluginapi.DMRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pluginapi.DMRecord(nil), s.gecmis...), s.gecmisErr
}
func (s *sahteIstemci) PollDM() ([]pluginapi.DMMsg, error) { return nil, nil }
func (s *sahteIstemci) SendDM(peerID, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	s.gonderilen = append(s.gonderilen, peerID+":"+message)
	s.gecmis = append(s.gecmis, pluginapi.DMRecord{PeerID: peerID, Text: message, At: time.Now(), Outgoing: true})
	return nil
}
func (s *sahteIstemci) SendFile(peerID, localPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dosyalar = append(s.dosyalar, peerID+":"+filepath.Base(localPath))
	return nil
}
func (s *sahteIstemci) PollFile() ([]pluginapi.FileMsg, error)           { return nil, nil }
func (s *sahteIstemci) PollConsent() ([]pluginapi.FileConsentReq, error) { return nil, nil }
func (s *sahteIstemci) RespondConsent(id string, kabul bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if kabul {
		s.yanitlar = append(s.yanitlar, id+":kabul")
	} else {
		s.yanitlar = append(s.yanitlar, id+":ret")
	}
	return nil
}
func (s *sahteIstemci) Peers() ([]pluginapi.PeerInfo, error) { return s.esler, nil }

func testModeli(t *testing.T, c *sahteIstemci) model {
	t.Helper()
	m := yeniModel(c, pluginapi.IdentityInfo{Name: "ben", PeerID: ben}, t.TempDir())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	return mm.(model)
}

func tusa(t *testing.T, m model, k tea.KeyMsg) (model, tea.Cmd) {
	t.Helper()
	mm, cmd := m.Update(k)
	return mm.(model), cmd
}

func harf(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestKonusmalar_GruplarSiralarVeOkunmamisSayar(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	m := testModeli(t, &sahteIstemci{})
	m.gecmis = []pluginapi.DMRecord{
		{PeerID: alice, Name: "alice", Text: "selam", At: t0},
		{PeerID: bob, Name: "bob", Text: "naber", At: t0.Add(time.Minute)},
		{PeerID: alice, Text: "merhaba", At: t0.Add(2 * time.Minute), Outgoing: true},
		{PeerID: alice, Name: "alice", Text: "nasılsın?", At: t0.Add(3 * time.Minute)},
	}
	m.okundu[alice] = t0 // ilk mesajı okumuş
	liste := m.konusmalar()
	if len(liste) != 2 || liste[0].peerID != alice || liste[1].peerID != bob {
		t.Fatalf("en son hareket eden en üstte olmalı: %+v", liste)
	}
	if liste[0].okunmamis != 1 || liste[1].okunmamis != 1 {
		t.Fatalf("okunmamış sayıları yanlış: alice=%d bob=%d", liste[0].okunmamis, liste[1].okunmamis)
	}
	if liste[0].ad != "alice" || len(liste[0].mesajlar) != 3 {
		t.Fatalf("alice konuşması beklenmedik: %+v", liste[0])
	}
}

func TestMakineMesajlariGosterilmez(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	makine := protokol.DeltaXOnEki + "1|eyJ0aXAiOiJ1eWVfZ3VuY2VsbGUifQ=="
	c := &sahteIstemci{gecmis: []pluginapi.DMRecord{
		{PeerID: alice, Name: "alice", Text: "selam", At: t0},
		{PeerID: alice, Name: "alice", Text: makine, At: t0.Add(time.Minute)},
		{PeerID: bob, Name: "bob", Text: makine, At: t0.Add(2 * time.Minute)},
	}}
	m := testModeli(t, c)
	mm, _ := m.Update(gecmisMsg{kayitlar: c.gecmis})
	m = mm.(model)
	liste := m.konusmalar()
	if len(liste) != 1 || liste[0].peerID != alice || len(liste[0].mesajlar) != 1 || liste[0].okunmamis != 1 {
		t.Fatalf("DeltaX'in makine mesajları konuşma olarak görünmemeli: %+v", liste)
	}

	// Geçmişi olmayan eski bir AND'da canlı gelenler de süzülür.
	mm, _ = m.Update(canliDMMsg{mesajlar: []pluginapi.DMMsg{{VerifiedPeerID: bob, From: "bob", Text: makine}}})
	if liste := mm.(model).konusmalar(); len(liste) != 1 {
		t.Fatalf("canlı gelen makine mesajı da gösterilmemeli: %+v", liste)
	}
}

func TestYazi_EnterGonderirHatadaMetinGeriGelir(t *testing.T) {
	c := &sahteIstemci{}
	m := testModeli(t, c).konusmaAc(alice)
	m.yazi.SetValue("  selam alice  ")
	m, cmd := tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.yazi.Value() != "" || m.gonderiliyor != 1 || cmd == nil {
		t.Fatalf("enter mesajı göndermeli ve kutuyu boşaltmalı (gonderiliyor=%d)", m.gonderiliyor)
	}
	mm, sonraki := m.Update(cmd())
	m = mm.(model)
	if len(c.gonderilen) != 1 || c.gonderilen[0] != alice+":selam alice" || m.gonderiliyor != 0 || sonraki == nil {
		t.Fatalf("SendDM kırpılmış metinle çağrılmalı ve geçmiş yenilenmeli: %v", c.gonderilen)
	}

	c.sendErr = errors.New("peer erişilemiyor")
	m.yazi.SetValue("ikinci")
	m, cmd = tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	mm, _ = m.Update(cmd())
	m = mm.(model)
	if !m.hata || !strings.Contains(m.bildirim, "peer erişilemiyor") || m.yazi.Value() != "ikinci" {
		t.Fatalf("gönderilemeyen metin kutuya geri gelmeli: bildirim=%q yazi=%q", m.bildirim, m.yazi.Value())
	}
}

// TestOnay_EnterKabulEtmezYEder: bir dosya isteği, kullanıcı tam o anda
// enter ile mesaj gönderirken ekrana düşerse yanlışlıkla kabul edilmemeli.
func TestOnay_EnterKabulEtmezYEder(t *testing.T) {
	c := &sahteIstemci{}
	m := testModeli(t, c).konusmaAc(alice)
	mm, _ := m.Update(onayMsg{istekler: []pluginapi.FileConsentReq{{TransferID: "t1", SenderID: alice, Filename: "rapor.pdf", Size: 2048}}})
	m = mm.(model)
	if len(m.onaylar) != 1 {
		t.Fatal("istek bekleyen onaylara eklenmeli")
	}
	m, cmd := tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || len(m.onaylar) != 1 || len(c.yanitlar) != 0 {
		t.Fatal("enter dosya isteğini kabul etmemeli")
	}
	m, cmd = tusa(t, m, harf("y"))
	if cmd == nil || len(m.onaylar) != 0 {
		t.Fatal("y isteği yanıtlamalı")
	}
	cmd()
	if len(c.yanitlar) != 1 || c.yanitlar[0] != "t1:kabul" {
		t.Fatalf("RespondConsent kabul ile çağrılmalı: %v", c.yanitlar)
	}
}

func TestOnay_SuresiDolunca(t *testing.T) {
	m := testModeli(t, &sahteIstemci{})
	simdi := time.Now()
	m.simdi = func() time.Time { return simdi }
	mm, _ := m.Update(onayMsg{istekler: []pluginapi.FileConsentReq{{TransferID: "t1", SenderID: bob, Filename: "a.zip"}}})
	m = mm.(model)
	simdi = simdi.Add(onaySuresi + time.Second)
	mm, _ = m.Update(tikMsg(simdi))
	m = mm.(model)
	if len(m.onaylar) != 0 || !strings.Contains(m.bildirim, "süresi doldu") {
		t.Fatalf("süresi dolan istek düşmeli: onaylar=%d bildirim=%q", len(m.onaylar), m.bildirim)
	}
}

func TestEskiAND_GecmisYoksaCanliModa(t *testing.T) {
	m := testModeli(t, &sahteIstemci{})
	mm, cmd := m.Update(gecmisMsg{err: &pluginapi.APIError{Status: 404}})
	m = mm.(model)
	if !m.canliMod || cmd == nil {
		t.Fatal("404'te canlı DM dinlemeye geçilmeli")
	}
	mm, _ = m.Update(canliDMMsg{mesajlar: []pluginapi.DMMsg{{From: "alice (12D3KooWAlic…)", VerifiedPeerID: alice, Text: "canlı"}}})
	m = mm.(model)
	liste := m.konusmalar()
	if len(liste) != 1 || liste[0].ad != "alice" || liste[0].mesajlar[0].metin != "canlı" {
		t.Fatalf("canlı DM konuşmaya eklenmeli: %+v", liste)
	}
	// Geçici (404 olmayan) hata canlı moda geçirmez.
	m2 := testModeli(t, &sahteIstemci{})
	mm, _ = m2.Update(gecmisMsg{err: errors.New("bağlantı reddedildi")})
	if mm.(model).canliMod {
		t.Fatal("geçici hata canlı moda geçirmemeli")
	}
}

func TestOkundu_KonusmaAcilincaKaydedilir(t *testing.T) {
	dir := t.TempDir()
	c := &sahteIstemci{gecmis: []pluginapi.DMRecord{{PeerID: alice, Name: "alice", Text: "selam", At: time.Now().Add(-time.Minute)}}}
	m := yeniModel(c, pluginapi.IdentityInfo{PeerID: ben}, dir)
	mm, _ := m.Update(gecmisMsg{kayitlar: c.gecmis})
	m = mm.(model)
	if m.konusmalar()[0].okunmamis != 1 {
		t.Fatal("açılmamış konuşmadaki mesaj okunmamış sayılmalı")
	}
	m = m.konusmaAc(alice)
	yeni := yeniModel(c, pluginapi.IdentityInfo{PeerID: ben}, dir)
	yeni.gecmis = c.gecmis
	if yeni.konusmalar()[0].okunmamis != 0 {
		t.Fatal("okundu bilgisi diske yazılıp yeniden açılışta yüklenmeli")
	}
}

func TestYeniKonusma_YalnizcaANDDugumleriVeDogrulama(t *testing.T) {
	c := &sahteIstemci{esler: []pluginapi.PeerInfo{{PeerID: bob, AND: true}, {PeerID: "12D3KooWDhtDugumuAbCdEfGhJkLmNpQrStUvWxYz12345678"}, {PeerID: ben, AND: true}}}
	m := testModeli(t, c)
	mm, _ := m.Update(eslerMsg{esler: c.esler})
	m = mm.(model)
	if len(m.esler) != 1 || m.esler[0].PeerID != bob {
		t.Fatalf("yalnızca kendimiz dışındaki AND düğümleri listelenmeli: %+v", m.esler)
	}
	m, _ = tusa(t, m, harf("n"))
	if m.odak != odakYeni {
		t.Fatal("n yeni konuşma formunu açmalı")
	}
	m.yeniEs.SetValue("bu-gecersiz")
	m, _ = tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.hata || m.odak != odakYeni {
		t.Fatal("geçersiz Peer ID reddedilmeli")
	}
	m.yeniEs.SetValue("")
	m, _ = tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.odak != odakYazi || m.secili != bob {
		t.Fatalf("boş girişte listeden seçili düğüm açılmalı: odak=%v secili=%q", m.odak, m.secili)
	}
	if len(m.konusmalar()) != 1 {
		t.Fatal("elle başlatılan konuşma listede görünmeli")
	}
}

func TestDosya_GonderimVeDogrulama(t *testing.T) {
	c := &sahteIstemci{}
	m := testModeli(t, c).konusmaAc(alice)
	m, _ = tusa(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if m.odak != odakDosya {
		t.Fatal("ctrl+f dosya yolu kutusunu açmalı")
	}
	m.dosyaYolu.SetValue(filepath.Join(t.TempDir(), "yok.txt"))
	m, cmd := tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !m.hata {
		t.Fatal("olmayan dosya gönderilmemeli")
	}
	yol := filepath.Join(t.TempDir(), "rapor.txt")
	if err := os.WriteFile(yol, []byte("içerik"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.dosyaYolu.SetValue(`"` + yol + `"`) // sürükle-bırak tırnakları
	m, cmd = tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.odak != odakYazi {
		t.Fatal("geçerli dosya gönderilmeli ve yazmaya dönülmeli")
	}
	mm, _ := m.Update(cmd())
	m = mm.(model)
	if len(c.dosyalar) != 1 || c.dosyalar[0] != alice+":rapor.txt" {
		t.Fatalf("SendFile çağrılmalı: %v", c.dosyalar)
	}
	k, _ := m.konusmaBul(alice)
	if len(k.mesajlar) != 1 || !k.mesajlar[0].olay || !strings.Contains(k.mesajlar[0].metin, "rapor.txt") {
		t.Fatalf("gönderim konuşmaya bir olay olarak eklenmeli: %+v", k.mesajlar)
	}
}

func TestYardimcilar(t *testing.T) {
	for id, want := range map[string]bool{alice: true, "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG": true, "12D3KooW0OIl": false, "bob": false, alice + "!": false} {
		if gecerliPeerID(id) != want {
			t.Errorf("gecerliPeerID(%q) = %v, want %v", id, !want, want)
		}
	}
	if got := dosyaYoluTemizle(`  'C:\a b\c.txt'  `); got != `C:\a b\c.txt` {
		t.Errorf("dosyaYoluTemizle tırnakları atmalı: %q", got)
	}
	if got := adParcasi("alice (12D3KooWAlic…)"); got != "alice" {
		t.Errorf("adParcasi: %q", got)
	}
	if got := adParcasi(alice); got != "" {
		t.Errorf("yalnızca Peer ID ise ad boş olmalı: %q", got)
	}
}

// TestView_EkraniKaplar: her durumda görünüm terminali tam kaplar.
func TestView_EkraniKaplar(t *testing.T) {
	c := &sahteIstemci{}
	durumlar := map[string]func(model) model{
		"boş": func(m model) model { return m },
		"konuşma": func(m model) model {
			m.gecmis = []pluginapi.DMRecord{
				{PeerID: alice, Name: "alice", Text: strings.Repeat("uzun bir mesaj ", 30), At: time.Now()},
				{PeerID: alice, Text: "kısa yanıt", At: time.Now(), Outgoing: true},
			}
			return m.konusmaAc(alice)
		},
		"yeni": func(m model) model {
			m.esler = []pluginapi.PeerInfo{{PeerID: bob, AND: true}}
			mm, _ := m.yeniKonusmaAc()
			return mm.(model)
		},
		"onay": func(m model) model {
			m.onaylar = []bekleyenOnay{{FileConsentReq: pluginapi.FileConsentReq{TransferID: "t", SenderID: bob, Filename: "a.zip", Size: 10}, bitis: time.Now().Add(time.Minute)}}
			return m
		},
	}
	for _, sz := range [][2]int{{213, 50}, {100, 30}, {70, 24}} {
		for ad, kur := range durumlar {
			m := yeniModel(c, pluginapi.IdentityInfo{PeerID: ben}, t.TempDir())
			mm, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			m = kur(mm.(model))
			lines := strings.Split(m.View(), "\n")
			if len(lines) != sz[1] {
				t.Fatalf("%s %dx%d: %d satır", ad, sz[0], sz[1], len(lines))
			}
			for i, l := range lines {
				if lipgloss.Width(l) != sz[0] {
					t.Fatalf("%s %dx%d: satır %d genişliği %d: %q", ad, sz[0], sz[1], i, lipgloss.Width(l), ansi.Strip(l))
				}
			}
		}
	}
}

func TestGonderim_GidenKutusuylaYapilir(t *testing.T) {
	c := &sahteIstemci{}
	m := testModeli(t, c).konusmaAc(alice)
	m.yazi.SetValue("çevrimdışıyken de gitsin")
	_, cmd := tusa(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	if len(c.sirali) != 1 {
		t.Fatalf("mesaj giden kutusunu destekleyen yoldan gönderilmeli: %v", c.sirali)
	}
}

func TestOkundu_CekirdekleEsitlenir(t *testing.T) {
	t0 := time.Now().Add(-time.Hour).UTC()
	c := &sahteIstemci{
		gecmis: []pluginapi.DMRecord{{PeerID: alice, Name: "aziz", Text: "selam", At: t0.Add(time.Minute)}},
		okundu: map[string]time.Time{bob: t0},
	}
	m := testModeli(t, c)
	m.okundu[alice] = t0 // yalnızca eski yerel dosyada olan işaret
	mm, _ := m.Update(gecmisMsg{kayitlar: c.gecmis})
	m = mm.(model)
	mm, _ = m.Update(okunduMsg{isaretler: map[string]time.Time{bob: t0}})
	m = mm.(model)
	if !m.cekirdekOkundu || !m.okundu[bob].Equal(t0) {
		t.Fatal("çekirdeğin okundu bilgisi alınmalı")
	}
	m = m.konusmaAc(alice)
	mm, cmd := m.Update(tikMsg(time.Now()))
	m = mm.(model)
	for _, alt := range cmd().(tea.BatchMsg) {
		if alt != nil {
			alt()
		}
	}
	if !c.okundu[alice].Equal(t0.Add(time.Minute)) {
		t.Fatalf("açılan konuşma çekirdekte okundu işaretlenmeli (ana menü rozeti için): %v", c.okundu)
	}
	if len(m.bildirilecek) != 0 {
		t.Fatal("bildirilen işaretler temizlenmeli")
	}
}

func TestOkundu_EskiANDdaYalnizcaYerel(t *testing.T) {
	c := &sahteIstemci{okunduYok: true, gecmis: []pluginapi.DMRecord{{PeerID: alice, Text: "selam", At: time.Now()}}}
	m := testModeli(t, c)
	o, err := c.DMReadMarks()
	mm, _ := m.Update(okunduMsg{isaretler: o, err: err})
	m = mm.(model)
	mm, _ = m.Update(gecmisMsg{kayitlar: c.gecmis})
	m = mm.(model).konusmaAc(alice)
	if m.cekirdekOkundu || len(m.bildirilecek) != 0 || len(c.okundu) != 0 {
		t.Fatal("okundu ucu olmayan AND'a işaret gönderilmemeli")
	}
}

func TestAkis_BekleyenVeGonderilemeyenGosterilir(t *testing.T) {
	m := testModeli(t, &sahteIstemci{})
	m.gecmis = []pluginapi.DMRecord{
		{PeerID: alice, Text: "bekleyen mesaj", At: time.Now(), Outgoing: true, Pending: true},
		{PeerID: alice, Text: "giden olmayan", At: time.Now(), Outgoing: true, Failed: true},
	}
	m = m.konusmaAc(alice)
	ekran := ansi.Strip(m.View())
	if !strings.Contains(ekran, "bekliyor · alıcı bağlanınca gidecek") || !strings.Contains(ekran, "gönderilemedi") {
		t.Fatalf("bekleyen ve gönderilemeyen mesaj işaretlenmeli:\n%s", ekran)
	}
}
