package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/pluginapi"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/pluginui"
	"github.com/lucian95511/and/internal/ui"
)

// EklentiAnahtari, eklentiye yalnızca onaylanan izinleri taşıyan bir API
// anahtarı verir; dönen geriAl eklenti kapanınca çağrılır. main, eklenti API
// sunucusunun EklentiAnahtari'nı buraya bağlar. Atanmamışsa eklenti boş
// anahtarla başlar ve hiçbir uca erişemez: tam yetkili ana anahtar hiçbir
// zaman eklentiye verilmez.
var EklentiAnahtari func(eklenti string, izinler []string) (anahtar string, geriAl func(), err error)

// izinIstegi, kullanıcının onayını bekleyen bir eklenti başlatmasıdır. hata
// doluysa eklenti hiç başlatılamaz (ör. daha yeni bir AND istiyor) ve ekran
// yalnızca nedenini söyler.
type izinIstegi struct {
	eklenti int
	izinler []string
	env     []string
	onceki  appScreen
	hata    string
}

func (m appModel) eklentiIndeksi(ad string) int {
	for i := range m.plugins {
		if m.plugins[i].Name() == ad {
			return i
		}
	}
	return -1
}

// eklentiAc, eklenti başlatmalarının tek giriş noktasıdır: önce API
// uyumluluğu, sonra izin onayı denetlenir; ikisi de tamamsa eklenti çalışır.
func (m appModel) eklentiAc(i int, env []string) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.plugins) || !m.plugins[i].Enabled {
		return m, nil
	}
	pl := m.plugins[i]
	if err := pluginmgr.Uyumlu(pl); err != nil {
		m.izinIstegi = &izinIstegi{eklenti: i, onceki: m.screen, hata: err.Error()}
		m.screen = screenIzin
		return m, nil
	}
	istenen := pl.Manifest.Permissions
	if !pluginmgr.IzinOnayli(m.dataDir, pl.Name(), istenen) {
		m.izinIstegi = &izinIstegi{eklenti: i, izinler: istenen, env: env, onceki: m.screen}
		m.screen = screenIzin
		return m, nil
	}
	return m.eklentiCalistir(i, env)
}

// eklentiCalistir, eklentiyi yalnızca manifest'inde istediği (ve kullanıcının
// onayladığı) izinleri taşıyan, kapanınca geçersizleşen bir anahtarla başlatır.
func (m appModel) eklentiCalistir(i int, env []string) (tea.Model, tea.Cmd) {
	pl := m.plugins[i]
	anahtar, geriAl := "", func() {}
	if EklentiAnahtari != nil {
		if a, g, err := EklentiAnahtari(pl.Name(), pl.Manifest.Permissions); err == nil {
			anahtar, geriAl = a, g
		}
	}
	cmd := pl.Launch(m.apiAddr, anahtar, m.dataDir, env...)
	ad := pl.Name()
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		geriAl()
		return pluginExitMsg{name: ad, err: err}
	})
}

func (m appModel) handleIzinKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ist := m.izinIstegi
	if ist == nil {
		m.screen = screenMenu
		return m, nil
	}
	geri := func() (tea.Model, tea.Cmd) {
		m.screen = ist.onceki
		m.izinIstegi = nil
		if m.screen == screenSettings {
			m.syncSettingsVP()
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "h", "n", "q":
		return geri()
	case "e", "y":
		if ist.hata != "" {
			return geri()
		}
		pl := m.plugins[ist.eklenti]
		if err := pluginmgr.IzinOnayla(m.dataDir, pl.Name(), ist.izinler); err != nil {
			ist.hata = dil.T("İzinler kaydedilemedi: ") + err.Error()
			return m, nil
		}
		m.screen = ist.onceki
		m.izinIstegi = nil
		return m.eklentiCalistir(ist.eklenti, ist.env)
	}
	return m, nil
}

func (m appModel) viewIzin() string {
	iw := ui.ContentWidth(m.width)
	ist := m.izinIstegi
	if ist == nil || ist.eklenti >= len(m.plugins) {
		return ui.Page(m.width, m.height, "", "")
	}
	pl := m.plugins[ist.eklenti]
	ad := pl.Label()
	if ad == "" {
		ad = pl.Name()
	}
	ad = dil.T(pluginui.Safe(ad))
	olcu := ui.Measure(iw)

	var b strings.Builder
	b.WriteString(ui.Header(iw, []string{dil.T("Eklentiler"), ad}, ui.S.Muted.Render(eklentiKaynagi(pl))) + "\n\n")
	if ist.hata != "" {
		b.WriteString(ui.Notice(ui.KindError, ist.hata) + "\n")
		f := ui.Divider(iw) + "\n" + ui.KeyHints(iw, ui.H("esc", dil.T("geri")))
		return ui.Page(m.width, m.height, b.String(), f)
	}

	b.WriteString(ui.S.Title.Render(dil.F("%s şu izinleri istiyor", ad)) + "\n\n")
	for _, iz := range ist.izinler {
		bilgi, ok := pluginapi.IzinBul(iz)
		if !ok {
			continue
		}
		b.WriteString("  " + ui.S.Accent.Render("●") + " " + ui.S.Bold.Render(dil.T(bilgi.Baslik)) + "\n")
		for _, satir := range strings.Split(ui.Wrap(dil.T(bilgi.Aciklama), olcu-4), "\n") {
			b.WriteString("    " + ui.S.Muted.Render(satir) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(ui.S.Muted.Render(ui.Wrap(dil.T("Her eklenti, herkese açık içeriği (forum, siteler, genel sohbet, kimliğin) izinsiz okuyabilir. ")+
		dil.T("Bu izinleri yalnızca güvendiğin eklentilere ver; Ayarlar › Eklentiler'den istediğin zaman geri alabilirsin."), olcu)) + "\n")

	f := ui.Divider(iw) + "\n" + ui.KeyHints(iw, ui.H("e", dil.T("izin ver ve aç")), ui.H("esc", dil.T("vazgeç")))
	return ui.Page(m.width, m.height, b.String(), f)
}

// eklentiKaynagi, eklentinin yazarı ve sürümüdür (başlıkta gösterilir).
func eklentiKaynagi(pl pluginmgr.Plugin) string {
	var parca []string
	if pl.Manifest.Author != "" {
		parca = append(parca, pluginui.Safe(pl.Manifest.Author))
	}
	if pl.Manifest.Version != "" {
		parca = append(parca, "v"+pluginui.Safe(pl.Manifest.Version))
	}
	return strings.Join(parca, "  ")
}

// izinOzeti, Ayarlar › Eklentiler'de seçili eklentinin izin satırıdır.
func (m appModel) izinOzeti(pl pluginmgr.Plugin) string {
	if len(pl.Manifest.Permissions) == 0 {
		return dil.T("İzin istemiyor")
	}
	var adlar []string
	for _, iz := range pl.Manifest.Permissions {
		if b, ok := pluginapi.IzinBul(iz); ok {
			adlar = append(adlar, dil.T(b.Baslik))
		}
	}
	durum := dil.T("ilk açılışta sorulacak")
	if pluginmgr.IzinOnayli(m.dataDir, pl.Name(), pl.Manifest.Permissions) {
		durum = dil.T("verildi (r ile geri al)")
	}
	return dil.F("İzinler: %s — %s", strings.Join(adlar, ", "), durum)
}
