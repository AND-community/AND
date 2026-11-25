package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/protokol"
)

// sahteIstemci, AND'ın eklenti API'sinin bellekteki karşılığıdır.
type sahteIstemci struct {
	mu        sync.Mutex
	konular   []pluginapi.PostDetail
	yanitlar  []string // "konu:metin"
	acilan    []string // "kategori/başlık"
	silinen   []string
	profiller map[string]*pluginapi.ProfileInfo
	yanitHata error
	degisim   chan pluginapi.ForumChanges
}

func (c *sahteIstemci) ForumPosts() ([]pluginapi.PostSummary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []pluginapi.PostSummary
	for _, k := range c.konular {
		son := k.CreatedAt
		for _, r := range k.Replies {
			if r.CreatedAt.After(son) {
				son = r.CreatedAt
			}
		}
		out = append(out, pluginapi.PostSummary{ID: k.ID, Category: k.Category, Title: k.Title, AuthorName: k.AuthorName,
			AuthorKey: k.AuthorKey, CreatedAt: k.CreatedAt, LastActivity: son, ReplyCount: len(k.Replies)})
	}
	return out, nil
}

func (c *sahteIstemci) GetPost(id string) (*pluginapi.PostDetail, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.konular {
		if k.ID == id {
			k.Replies = append([]pluginapi.ReplyInfo(nil), k.Replies...)
			return &k, nil
		}
	}
	return nil, nil
}

func (c *sahteIstemci) CreatePost(kat, baslik, govde string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acilan = append(c.acilan, kat+"/"+baslik)
	return nil
}

func (c *sahteIstemci) CreateReply(konu, govde string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.yanitHata != nil {
		return c.yanitHata
	}
	c.yanitlar = append(c.yanitlar, konu+":"+govde)
	return nil
}

func (c *sahteIstemci) DeleteOwnPost(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.silinen = append(c.silinen, id)
	return nil
}

func (c *sahteIstemci) Profile(key string) (*pluginapi.ProfileInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.profiller[key]; p != nil {
		return p, nil
	}
	return nil, &pluginapi.APIError{Status: 404, Message: "yok"}
}

func (c *sahteIstemci) ForumChanges(since uint64) (pluginapi.ForumChanges, error) {
	if c.degisim == nil {
		return pluginapi.ForumChanges{}, errors.New("bağlantı yok")
	}
	return <-c.degisim, nil
}

var (
	ben     = pluginapi.IdentityInfo{Name: "umut", PubKey: strings.Repeat("aa", 32)}
	baskasi = strings.Repeat("bb", 32)
	t0      = time.Now().Add(-3 * time.Hour)
)

func ornekIstemci() *sahteIstemci {
	return &sahteIstemci{konular: []pluginapi.PostDetail{
		{ID: "k1", Category: "Genel", Title: "Eski konu", Body: "gövde", AuthorName: "ayse", AuthorKey: baskasi, CreatedAt: t0},
		{ID: "k2", Category: "Genel", Title: "Yeni konu", Body: "[code=go]x := 1[/code]", AuthorName: "umut", AuthorKey: ben.PubKey,
			CreatedAt: t0.Add(time.Hour), Replies: []pluginapi.ReplyInfo{{ID: "y1", AuthorName: "ayse", AuthorKey: baskasi, Body: "yanıt", CreatedAt: t0.Add(2 * time.Hour)}}},
		{ID: "k3", Category: "Yazılım", Title: "Başka kategori", Body: "b", AuthorName: "ayse", AuthorKey: baskasi, CreatedAt: t0},
	}, profiller: map[string]*pluginapi.ProfileInfo{baskasi: {Key: baskasi, Name: "ayse", PostCount: 2, Bio: "merhaba ben ayşe"}}}
}

func harfler(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	tusEnter = tea.KeyMsg{Type: tea.KeyEnter}
	tusEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	tusCtrlS = tea.KeyMsg{Type: tea.KeyCtrlS}
	tusCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
	tusAsagi = tea.KeyMsg{Type: tea.KeyDown}
)

// calistir, komutu (toplu komutlar dahil) kısa bir süre içinde dönen
// mesajlarıyla birlikte çalıştırır; bekleyen yoklamalar bırakılır.
func calistir(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	c := make(chan tea.Msg, 1)
	go func() { c <- cmd() }()
	select {
	case msg := <-c:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, alt := range b {
				out = append(out, calistir(alt)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// gonder, mesajı modele verir ve dönen komutların hemen dönen mesajlarını da
// (tur kez) işler.
func gonder(m model, msg tea.Msg, tur int) model {
	yeni, cmd := m.Update(msg)
	m = yeni.(model)
	for range tur {
		msgs := calistir(cmd)
		if len(msgs) == 0 {
			return m
		}
		var cmds []tea.Cmd
		for _, ms := range msgs {
			if _, ok := ms.(tea.QuitMsg); ok {
				continue
			}
			y, c := m.Update(ms)
			m = y.(model)
			cmds = append(cmds, c)
		}
		cmd = tea.Batch(cmds...)
	}
	return m
}

func basla(m model, tuslar ...tea.KeyMsg) model {
	for _, k := range tuslar {
		m = gonder(m, k, 3)
	}
	return m
}

func testModeli(t *testing.T, c *sahteIstemci, dataDir string) model {
	t.Helper()
	m := newModel(c, dataDir, ben, pluginapi.RoleInfo{})
	m = gonder(m, tea.WindowSizeMsg{Width: 100, Height: 30}, 0)
	m.katIdx = slices.Index(protokol.Kategoriler, "Genel")
	return gonder(m, konularYukle(c)(), 0)
}

// konuya, Genel kategorisinin ilk (en son etkinliği olan) konusunu açar.
func konuya(t *testing.T, m model) model {
	t.Helper()
	m = basla(m, tusEnter, tusEnter)
	if m.ekran != ekKonu || m.aktif == nil {
		t.Fatalf("konu açılmadı: ekran %v", m.ekran)
	}
	return m
}

func TestKonular_KategoriyeGoreVeSonEtkinligeGore(t *testing.T) {
	m := testModeli(t, ornekIstemci(), t.TempDir())
	if m.ekran != ekKategoriler {
		t.Fatalf("ilk ekran kategoriler olmalı: %v", m.ekran)
	}
	m = basla(m, tusEnter)
	if m.ekran != ekKonular || m.kategori != "Genel" {
		t.Fatalf("Genel'e girilmeli: %v %q", m.ekran, m.kategori)
	}
	k := m.kategoriKonulari()
	if len(k) != 2 || k[0].ID != "k2" || k[1].ID != "k1" {
		t.Fatalf("son etkinliğe göre sıralı olmalı: %+v", k)
	}
}

func TestKonu_AcincaYanitlarGorunur(t *testing.T) {
	m := konuya(t, testModeli(t, ornekIstemci(), t.TempDir()))
	if m.aktif.ID != "k2" || len(m.aktif.Replies) != 1 {
		t.Fatalf("aktif konu: %+v", m.aktif)
	}
	if duz := ansi.Strip(m.View()); !strings.Contains(duz, "yanıt") || !strings.Contains(duz, "Yeni konu") {
		t.Fatalf("konu görünümü:\n%s", duz)
	}
}

func TestYanitTaslagi_EscVeYenidenYanitlaKorunur(t *testing.T) {
	m := konuya(t, testModeli(t, ornekIstemci(), t.TempDir()))
	m = basla(m, harfler("r"), harfler("uzun bir yanıt"), tusEsc)
	if m.ekran != ekKonu {
		t.Fatalf("esc konuya dönmeli: %v", m.ekran)
	}
	m = basla(m, harfler("r"))
	if got := m.yanitAlan.Value(); got != "uzun bir yanıt" {
		t.Fatalf("taslak geri gelmeli: %q", got)
	}
}

func TestYanitTaslagi_YenidenBaslatmadaGeriGelir(t *testing.T) {
	for ad, cikis := range map[string]tea.KeyMsg{"esc": tusEsc, "ctrl+c": tusCtrlC} {
		t.Run(ad, func(t *testing.T) {
			dir := t.TempDir()
			m := konuya(t, testModeli(t, ornekIstemci(), dir))
			basla(m, harfler("r"), harfler("kaybolmasın"), cikis)
			m2 := konuya(t, testModeli(t, ornekIstemci(), dir))
			if m2 = basla(m2, harfler("r")); m2.yanitAlan.Value() != "kaybolmasın" {
				t.Fatalf("taslak geri gelmeli: %q", m2.yanitAlan.Value())
			}
		})
	}
}

func TestYanitTaslagi_CokUzunHatasindanSonraDuzenlenebilir(t *testing.T) {
	m := konuya(t, testModeli(t, ornekIstemci(), t.TempDir()))
	m = basla(m, harfler("r"))
	m.yanitAlan.SetValue(strings.Repeat("€", 3000)) // 9000 bayt > 8192
	m = basla(m, tusCtrlS)
	if m.errMsg == "" || m.gonderiliyor || !m.yanitAlan.Focused() {
		t.Fatalf("hata gösterilmeli ve kutu düzenlenebilir kalmalı: %q %v", m.errMsg, m.gonderiliyor)
	}
	m = basla(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if n := utf8.RuneCountInString(m.yanitAlan.Value()); n != 2999 {
		t.Fatalf("silme tuşu metni kısaltmalı: %d", n)
	}
}

func TestYanit_GonderilinceTaslakSilinir(t *testing.T) {
	c := ornekIstemci()
	dir := t.TempDir()
	m := konuya(t, testModeli(t, c, dir))
	c.yanitHata = errors.New("yayınlanamadı")
	m = basla(m, harfler("r"), harfler("gidecek"), tusCtrlS)
	if m.ekran != ekYanit || m.errMsg == "" || m.yanitAlan.Value() != "gidecek" {
		t.Fatalf("başarısız gönderim kutuyu korumalı: %v %q %q", m.ekran, m.errMsg, m.yanitAlan.Value())
	}
	c.yanitHata = nil
	m = basla(m, tusCtrlS)
	if len(c.yanitlar) != 1 || c.yanitlar[0] != "k2:gidecek" || m.ekran != ekKonu {
		t.Fatalf("yanıt gitmeli: %v ekran %v", c.yanitlar, m.ekran)
	}
	m2 := konuya(t, testModeli(t, c, dir))
	if m2 = basla(m2, harfler("r")); m2.yanitAlan.Value() != "" {
		t.Fatalf("gönderilen yanıtın taslağı silinmeli: %q", m2.yanitAlan.Value())
	}
}

func TestCikistaTaslaklarKaydedilir(t *testing.T) {
	dir := t.TempDir()
	m := konuya(t, testModeli(t, ornekIstemci(), dir))
	m = basla(m, harfler("r"), harfler("a"))
	m.taslaklar.sonKayit = time.Now()
	m = basla(m, harfler("bc"))
	cikistaTaslaklariKaydet(m)
	m2 := konuya(t, testModeli(t, ornekIstemci(), dir))
	if m2 = basla(m2, harfler("r")); m2.yanitAlan.Value() != "abc" {
		t.Fatalf("çıkışta son hâl yazılmalı: %q", m2.yanitAlan.Value())
	}
}

func TestTaslak_BozukDosyaCokmez(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "yanit_taslaklari.json"), []byte("{bozuk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "taslaklar_Genel.json"), []byte("çöp"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := konuya(t, testModeli(t, ornekIstemci(), dir))
	if m = basla(m, harfler("r")); m.yanitAlan.Value() != "" {
		t.Fatalf("bozuk dosyada taslak boş olmalı: %q", m.yanitAlan.Value())
	}
	m = basla(m, tusEsc, tusEsc, harfler("n"))
	if m.ekran != ekYeniKonu {
		t.Fatalf("yeni konu açılmalı: %v", m.ekran)
	}
	_ = m.View()
}

func TestDegisim_YeniKonuListeyiGunceller(t *testing.T) {
	c := ornekIstemci()
	m := testModeli(t, c, t.TempDir())
	c.mu.Lock()
	c.konular = append(c.konular, pluginapi.PostDetail{ID: "k4", Category: "Genel", Title: "Taze", AuthorName: "ece", CreatedAt: time.Now()})
	c.mu.Unlock()
	m = gonder(m, degisimMsg{d: pluginapi.ForumChanges{Surum: 1, Konular: []string{"k4"}}}, 2)
	if m.surum != 1 {
		t.Fatalf("sürüm güncellenmeli: %d", m.surum)
	}
	m = basla(m, tusEnter)
	if k := m.kategoriKonulari(); len(k) != 3 || k[0].ID != "k4" {
		t.Fatalf("yeni konu listede ve en üstte olmalı: %+v", k)
	}
}

func TestDegisim_HataSonrasiYenidenDener(t *testing.T) {
	m := testModeli(t, ornekIstemci(), t.TempDir())
	_, cmd := m.Update(degisimMsg{err: errors.New("koptu")})
	if cmd == nil {
		t.Fatal("geçici hatadan sonra yeniden denenmeli")
	}
	yeni, cmd := m.Update(degisimMsg{err: &pluginapi.APIError{Status: 404}})
	if cmd != nil || !yeni.(model).degisimYok {
		t.Fatal("eski AND'da (uç yok) yoklama durmalı")
	}
}

func TestKendiKonusunuSil(t *testing.T) {
	c := ornekIstemci()
	m := konuya(t, testModeli(t, c, t.TempDir())) // k2 benim
	m = basla(m, harfler("x"))
	if !m.silOnay {
		t.Fatal("kendi konusunda x onay istemeli")
	}
	m = basla(m, harfler("y"))
	if len(c.silinen) != 1 || c.silinen[0] != "k2" || m.ekran != ekKonular {
		t.Fatalf("silinmeli ve listeye dönülmeli: %v %v", c.silinen, m.ekran)
	}
	m = basla(m, tusAsagi, tusEnter, harfler("x")) // k1 başkasının
	if m.silOnay {
		t.Fatal("başkasının konusunda x bir şey yapmamalı")
	}
}

func TestProfil_YasakTusuYok_YetkiliyeIpucu(t *testing.T) {
	c := ornekIstemci()
	m := newModel(c, t.TempDir(), ben, pluginapi.RoleInfo{IsModerator: true})
	m = gonder(m, tea.WindowSizeMsg{Width: 100, Height: 30}, 0)
	m.katIdx = slices.Index(protokol.Kategoriler, "Genel")
	m = gonder(m, konularYukle(c)(), 0)
	m = basla(m, tusEnter, tusAsagi, harfler("p"))
	if m.ekran != ekProfil || m.profil == nil || m.profil.Name != "ayse" {
		t.Fatalf("profil açılmalı: %v %+v", m.ekran, m.profil)
	}
	duz := ansi.Strip(m.View())
	if !strings.Contains(duz, "merhaba ben ayşe") || !strings.Contains(duz, "Yönetici Paneli") {
		t.Fatalf("profil ve yetkili ipucu görünmeli:\n%s", duz)
	}
	if m = basla(m, harfler("b")); m.ekran != ekProfil {
		t.Fatal("b bir şey yapmamalı")
	}
}

func TestYeniKonu_KategoriTaslaklari(t *testing.T) {
	c := ornekIstemci()
	dir := t.TempDir()
	m := testModeli(t, c, dir)
	m = basla(m, tusEnter, harfler("n"))
	if m.ekran != ekYeniKonu {
		t.Fatalf("yeni konu ekranı: %v", m.ekran)
	}
	m = basla(m, harfler("Taslak başlık"), tusEsc)
	if m.ekran != ekKonular {
		t.Fatalf("esc foruma dönmeli: %v", m.ekran)
	}
	if ts := taslakOku(dir, "Genel"); len(ts) != 1 || ts[0].Baslik != "Taslak başlık" {
		t.Fatalf("taslak Genel'e saklanmalı: %+v", ts)
	}
	m = gonder(m, konuAcildiMsg{baslik: "Taslak başlık"}, 1)
	if !strings.Contains(m.okMsg, "Taslak başlık") {
		t.Fatalf("bildirim: %q", m.okMsg)
	}
}

func TestManifest_WellFormed(t *testing.T) {
	if manifest.Name != "forum" || manifest.Label == "" || manifest.UI != 1 || len(manifest.Permissions) != 1 ||
		manifest.Permissions[0] != pluginapi.IzinForum {
		t.Fatalf("manifest: %+v", manifest)
	}
}

var _ = fmt.Sprintf
