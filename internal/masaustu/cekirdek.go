package masaustu

// cekirdek.go — girişten sonra kurulan AND çekirdeğinin arayüze bağlanması:
// canlı olaylar (yeni ileti, konu, yanıt, ağ durumu, tema, güncelleme).

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/chatlog"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/peersync"
	"github.com/lucian95511/and/internal/pluginmgr"
	"github.com/lucian95511/and/internal/profile"
	"github.com/lucian95511/and/internal/tui"
	"github.com/lucian95511/and/pluginapi"
	"github.com/lucian95511/and/ui"
)

// Cekirdek, cmd/and'ın kurduğu ve masaüstü arayüzünün kullandığı AND
// bileşenleridir; terminal arayüzüne (tui.Run) verilenlerin aynısı.
type Cekirdek struct {
	Kimlik     *stdcrypto.Identity
	Dugum      *network.Node
	Forum      *forum.Forum
	Sohbet     *chatlog.Store
	Profil     *profile.Store
	Eklentiler []pluginmgr.Plugin
	// APIAdresi, eklenti API'sinin adresidir; boşsa eklentiler açılamaz.
	APIAdresi string
	// EklentiAnahtari, bir eklentiye yalnızca onaylanan izinleri taşıyan
	// API anahtarı üretir (bkz. pluginapi.Server.EklentiAnahtari).
	EklentiAnahtari func(eklenti string, izinler []string) (anahtar string, geriAl func(), err error)
	Senkron         func() peersync.Status
	OkunmamisDM     func() int
	// Uyari, açılışta kullanıcıya gösterilecek bir uyarıdır (ör. veritabanı
	// onarıldı).
	Uyari  string
	Kurucu bool
	// Guncelleme, indirilen yeni sürümü; Tema, bir eklentinin (Pembiş)
	// uyguladığı temayı bildirir. İkisi de nil olabilir.
	Guncelleme <-chan tui.UpdateReadyMsg
	Tema       <-chan pluginapi.ThemeReq
	// Harici, VS Code bağlantısını (eklenti API'sinin harici dinleyicisi)
	// açıp kapatır; nil ise Ayarlar'da gösterilmez.
	Harici HariciDenetim
}

// cekirdek, Cekirdek'in arayüz tarafındaki çalışma durumudur.
type cekirdek struct {
	Cekirdek
	dir     string
	anahtar string // kendi genel anahtarımız (hex)
	peerID  string

	mu         sync.Mutex
	eklentiler []pluginmgr.Plugin // etkin/kapalı durumu Ayarlar'dan değişir
}

func yeniCekirdek(dir string, c Cekirdek) *cekirdek {
	d := &cekirdek{Cekirdek: c, dir: dir, eklentiler: append([]pluginmgr.Plugin(nil), c.Eklentiler...)}
	if c.Kimlik != nil {
		d.anahtar = hex.EncodeToString(c.Kimlik.PublicKey())
	}
	if c.Dugum != nil {
		d.peerID = c.Dugum.Host.ID().String()
	}
	return d
}

// rol, yazarın moderatör ya da kurucuysa etkin dildeki rol adıdır; sıradan
// üyeler için boştur.
func (d *cekirdek) rol(yazar string) string {
	if d.Profil == nil {
		return ""
	}
	r := d.Profil.RoleOf(yazar)
	if r == profile.RoleMember {
		return ""
	}
	return dil.T(r.Label())
}

// olaylariIlet, çekirdeğin olaylarını arayüze iletir; ctx bitince ya da
// pencere kapanınca durur.
func (u *Uygulama) olaylariIlet(ctx context.Context, d *cekirdek) {
	var (
		konular  <-chan *forum.Post
		yanitlar <-chan *forum.Reply
		iletiler <-chan *chatlog.Message
		gecmis   <-chan struct{}
	)
	if d.Forum != nil {
		konular, yanitlar = d.Forum.NewPosts(), d.Forum.NewReplies()
	}
	if d.Sohbet != nil {
		iletiler, gecmis = d.Sohbet.NewMessages(), d.Sohbet.HistoryChanged()
	}
	tik := time.NewTicker(5 * time.Second)
	defer tik.Stop()
	u.yayin.gonder("durum", u.durumOzeti(d))
	for {
		select {
		case <-ctx.Done():
			return
		case <-u.bitti:
			return
		case p, ok := <-konular:
			if !ok {
				konular = nil
				continue
			}
			u.yayin.gonder("forum", map[string]string{"tur": "konu", "konu": p.ID, "kategori": p.Category})
		case r, ok := <-yanitlar:
			if !ok {
				yanitlar = nil
				continue
			}
			u.yayin.gonder("forum", map[string]string{"tur": "yanit", "konu": r.PostID})
		case m, ok := <-iletiler:
			if !ok {
				iletiler = nil
				continue
			}
			u.yayin.gonder("sohbet", d.ileti(m))
		case _, ok := <-gecmis:
			if !ok {
				gecmis = nil
				continue
			}
			u.yayin.gonder("sohbet-gecmis", true)
		case g, ok := <-d.Guncelleme:
			if !ok {
				d.Guncelleme = nil
				continue
			}
			metin := dil.F("%s indirildi — çıkışta otomatik yenilenir", g.Version)
			if g.Eklentiler > 0 {
				metin = dil.F("%s ve %d eklenti indirildi — çıkışta otomatik yenilenir", g.Version, g.Eklentiler)
			}
			u.guncelleme.Store(&metin)
			u.yayin.gonder("durum", u.durumOzeti(d))
		case t, ok := <-d.Tema:
			if !ok {
				d.Tema = nil
				continue
			}
			tema := ui.Normalize(ui.Theme{ThemeName: t.ThemeName, Accent: t.Accent, SelBG: t.SelBG,
				SelFG: t.SelFG, Muted: t.Muted, Name: t.Name, Badge: t.Badge})
			tui.SaveTheme(u.dir, tema)
			u.yayin.gonder("tema", temaRenkleri(tema))
		case <-tik.C:
			u.yayin.gonder("durum", u.durumOzeti(d))
		}
	}
}

// durumOzeti, kenar çubuğunun ve üst çubuğun gösterdiği anlık durumdur.
func (u *Uygulama) durumOzeti(d *cekirdek) map[string]any {
	o := map[string]any{}
	if d.Dugum != nil {
		ag := d.Dugum.Durum()
		o["esler"] = ag.BagliEs
		o["andEsler"] = ag.ANDEs + ag.SinirliEs
		o["erisim"] = erisimEtiketi(ag.Erisim)
		o["erisimDurum"] = erisimDurumu(ag.Erisim)
	}
	if d.Senkron != nil {
		s := d.Senkron()
		if !s.LastSuccess.IsZero() {
			o["senkron"] = s.LastSuccess
		}
	}
	if d.OkunmamisDM != nil {
		o["okunmamisDM"] = d.OkunmamisDM()
	}
	if g := u.guncelleme.Load(); g != nil {
		o["guncelleme"] = *g
	}
	return o
}

func erisimEtiketi(e network.Erisilebilirlik) string {
	switch e {
	case network.ErisimAcik:
		return dil.T("erişilebilir")
	case network.ErisimRoleIle:
		return dil.T("röle üzerinden")
	case network.ErisimKapali:
		return dil.T("erişilemiyor")
	}
	return dil.T("belirleniyor")
}

func erisimDurumu(e network.Erisilebilirlik) string {
	switch e {
	case network.ErisimAcik, network.ErisimRoleIle:
		return "iyi"
	case network.ErisimKapali:
		return "uyari"
	}
	return "soluk"
}
