package tui

// chrome.go — ekranların ortak iskeleti: içerik genişliği, kısa kimlik/eş
// durumu metinleri. Görsel bileşenlerin kendisi internal/ui'dedir; burası
// yalnızca TUI'ye özgü ince bir katman.

import (
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/chatlog"
	"github.com/lucian95511/and/internal/forum"
	"github.com/lucian95511/and/internal/peersync"
	"github.com/lucian95511/and/internal/site"
	"github.com/lucian95511/and/internal/ui"
)

// ANDSyncProtocols, bir eşin AND düğümü olduğunu gösteren geçmiş senkronu
// protokolleridir — bağlı eşlerin çoğu AND değil, genel DHT düğümleridir.
func ANDSyncProtocols() []protocol.ID {
	return []protocol.ID{forum.SyncProtocol, chatlog.SyncProtocol, site.SyncProtocol}
}

// SyncStatus, cmd/and'daki geçmiş senkronizasyonu zamanlayıcısının özetini
// verir; ayarlanmamışsa (testler) nil'dir.
var SyncStatus func() peersync.Status

// OkunmamisDM, okunmamış özel mesaj sayısını verir (ana menüdeki rozet);
// ayarlanmamışsa (testler) nil'dir.
var OkunmamisDM func() int

// BaslangicUyarisi, açılışta kullanıcının bilmesi gereken bir durumdur
// (ör. bozuk bulunup onarılan veritabanı); ana ekranın altında gösterilir.
// cmd/and, Run'dan önce ayarlar.
var BaslangicUyarisi string

func okunmamisDM() int {
	if OkunmamisDM == nil {
		return 0
	}
	return OkunmamisDM()
}

// andEsSayisi, bağlı eşlerden AND protokolünü konuştuğu bilinenlerin sayısıdır.
func (m appModel) andEsSayisi() int {
	if m.node == nil {
		return 0
	}
	ps, protos, n := m.node.Host.Peerstore(), ANDSyncProtocols(), 0
	for _, p := range m.node.Host.Network().Peers() {
		if peersync.ProtocolSupport(ps, p, protos...) == peersync.SupportYes {
			n++
		}
	}
	return n
}

// icGenislik, terminal genişliği w iken tam ekran çerçevenin içerik
// genişliğidir (bkz. ui.Page). w henüz bilinmiyorsa (ilk WindowSizeMsg'den
// önce, testlerde) makul bir varsayılan döner. Uzun düz yazı bu genişlikte
// değil ui.Measure ile sınırlanmış okuma genişliğinde sarılır.
func icGenislik(w int) int {
	if w <= 0 {
		return 72
	}
	return ui.ContentWidth(w)
}

// kisaPeerID, uzun bir libp2p Peer ID'sini "12D3KooWAb…XyZ0123456" biçiminde
// kısaltır.
func kisaPeerID(pid string) string {
	r := []rune(pid)
	if len(r) <= 22 {
		return pid
	}
	return string(r[:10]) + "…" + string(r[len(r)-10:])
}

// esDurumu, bağlı eş sayısını durum noktasıyla gösterir. Bağlanmaya çalışmak
// bir HATA değildir — bu yüzden kırmızı değil uyarı renginde.
func esDurumu(n int) string {
	if n == 0 {
		return ui.S.Warning.Render(dil.T("◌ ağa bağlanıyor…"))
	}
	return ui.Dot(true) + " " + dil.F("%d eş bağlı", n)
}
