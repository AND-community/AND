package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	lp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/dil/diltest"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"
)

func testEsKimligi(t *testing.T) peer.ID {
	t.Helper()
	_, pub, err := lp2pcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// agDurumlari, Ağ sekmesinin çizdiği başlıca durumlardır.
func agDurumlari(t *testing.T) map[string]network.Durum {
	t.Helper()
	simdi := time.Now()
	esler := []network.EsDurumu{
		{EsBilgisi: network.EsBilgisi{ID: testEsKimligi(t), Surum: "AND/v0.2.0", Gecikme: 42 * time.Millisecond},
			Adres: "203.0.113.7:24817", Tasiyici: "quic", Giden: true},
		{EsBilgisi: network.EsBilgisi{ID: testEsKimligi(t), Surum: "AND/v0.1.0\x1b[31m", Gecikme: 300 * time.Microsecond},
			Adres: "192.168.1.20:31000", Tasiyici: "tcp", Yerel: true},
		{EsBilgisi: network.EsBilgisi{ID: testEsKimligi(t), Sinirli: true},
			Adres: "198.51.100.9:4001", Tasiyici: "role"},
	}
	kesif := network.KesifDurumu{Calisiyor: true, DHTEsleri: 136, SonDuyuru: simdi.Add(-3 * time.Minute),
		SonArama: simdi.Add(-time.Minute), SonBulunan: 2, SonDegisim: simdi.Add(-5 * time.Minute), DegisimdenGelen: 3}
	return map[string]network.Durum{
		"acik": {Erisim: network.ErisimAcik, Port: 24817, AcikAdresler: []string{"/ip4/203.0.113.7/tcp/24817", "/ip4/203.0.113.7/udp/24817/quic-v1"},
			BagliEs: 58, ANDEs: 2, SinirliEs: 1, BilinenEs: 12, Esler: esler, Kesif: kesif},
		"role": {Erisim: network.ErisimRoleIle, NAT: network.NATUygun, Port: 24817, Roleler: 2,
			BagliEs: 40, ANDEs: 1, BilinenEs: 1, Esler: esler[:1], Kesif: kesif},
		"kapali_simetrik": {Erisim: network.ErisimKapali, NAT: network.NATZor, BagliEs: 9,
			Kesif: network.KesifDurumu{Calisiyor: true, DuyuruHatasi: "failed to find any peer in table"}},
		"kapali_bilinmiyor": {Erisim: network.ErisimKapali, Kesif: network.KesifDurumu{Calisiyor: true, DHTEsleri: 0}},
		"baslangic":         {},
	}
}

// Ağ sekmesi her durumda ve her dilde terminale sığar, çevrilmemiş metin
// bırakmaz; eşin bildirdiği sürüm metnindeki kaçış dizisi ekrana ulaşmaz.
func TestAgSekmesiHerDilde(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	denetci := diltest.YeniDenetci(t, ozelAdlar)
	peerID := testEsKimligi(t).String()
	for _, d := range dil.Diller {
		dil.Ayarla(d.Kod)
		for ad, durum := range agDurumlari(t) {
			for _, w := range []int{76, 96, 116} {
				icerik := agDurumuYaz(w, peerID, durum)
				for i, s := range strings.Split(strings.TrimRight(icerik, "\n"), "\n") {
					if g := lipgloss.Width(s); g > w {
						t.Errorf("%s %s w=%d: satır %d genişliği %d: %q", d.Kod, ad, w, i, g, ansi.Strip(s))
					}
				}
				if strings.Contains(icerik, "\x1b[31m") {
					t.Errorf("%s %s: eşin sürüm metnindeki kaçış dizisi ekrana ulaştı", d.Kod, ad)
				}
				for _, k := range denetci.Kalintilar(d.Kod, ansi.Strip(icerik)) {
					t.Errorf("%s %s: %s", d.Kod, ad, k)
				}
			}
		}
	}
}

func TestAgSekmesiIcerik(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	dil.Ayarla(dil.Kaynak)
	durumlar := agDurumlari(t)
	icerik := func(ad string) string { return ansi.Strip(agDurumuYaz(116, "12D3KooWTest", durumlar[ad])) }

	acik := icerik("acik")
	for _, beklenen := range []string{"erişilebilir", "24817 (TCP ve UDP)", "/ip4/203.0.113.7/tcp/24817",
		"2 doğrudan", "1 röle üzerinden", "203.0.113.7:24817", "quic", "42 ms", "<1 ms", "yerel ağ", "sınırlı", "röle",
		"AND/v0.2.0", "136 düğüm", "son duyuru 3 dk önce", "2 AND düğümü bulundu", "bugüne dek 3 yeni düğüm öğrenildi",
		"12 AND düğümü", "58 eş bağlı"} {
		if !strings.Contains(acik, beklenen) {
			t.Errorf("açık durumda %q yok:\n%s", beklenen, acik)
		}
	}
	// Doğrudan erişilebilen düğüme port yönlendirmesi önerilmez; NAT satırı yoktur.
	if strings.Contains(acik, "modeminde") || strings.Contains(acik, "delik açmaya") {
		t.Error("erişilebilir düğümde NAT ve port yönlendirme önerisi olmamalı")
	}

	role := icerik("role")
	for _, beklenen := range []string{"röle üzerinden", "2 röle üzerinden ulaşılabilirsin", "delik açmaya uygun", "modeminde"} {
		if !strings.Contains(role, beklenen) {
			t.Errorf("röle durumunda %q yok:\n%s", beklenen, role)
		}
	}

	kapali := icerik("kapali_simetrik")
	for _, beklenen := range []string{"erişilemiyor", "delik açmaya uygun değil", "Simetrik NAT", "her açılışta rastgele",
		"henüz duyurulamadı", "henüz yapılmadı", "Henüz bağlı AND düğümü yok", "ulaşılamıyor"} {
		if !strings.Contains(kapali, beklenen) {
			t.Errorf("kapalı durumda %q yok:\n%s", beklenen, kapali)
		}
	}

	baslangic := icerik("baslangic")
	for _, beklenen := range []string{"belirleniyor", "Keşif çalışmıyor."} {
		if !strings.Contains(baslangic, beklenen) {
			t.Errorf("başlangıç durumunda %q yok:\n%s", beklenen, baslangic)
		}
	}

	// Çok eş varken liste kesilir.
	cok := durumlar["acik"]
	cok.Esler = nil
	for range agSekmesiEnFazlaEs + 5 {
		cok.Esler = append(cok.Esler, network.EsDurumu{EsBilgisi: network.EsBilgisi{ID: testEsKimligi(t)}, Tasiyici: "tcp"})
	}
	if ic := ansi.Strip(agDurumuYaz(116, "x", cok)); !strings.Contains(ic, "… ve 5 düğüm daha") {
		t.Errorf("uzun liste kesilmeli:\n%s", ic)
	}
}

// Gerçek bir düğümle Ayarlar › Ağ sekmesi açılır, kaydırılır ve tam ekranı
// kaplar.
func TestAyarlarAgSekmesi(t *testing.T) {
	t.Cleanup(func() { dil.Ayarla(dil.Kaynak) })
	dil.Ayarla(dil.Kaynak)
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })

	m := newAppModel(context.Background(), id, node, nil, "127.0.0.1:1", nil, nil, nil, t.TempDir(), nil, nil, nil)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = mm.(appModel)
	m.screen = screenSettings
	for range settingsTabSayisi - 1 {
		mm, _ = m.handleSettingsKey(tea.KeyMsg{Type: tea.KeyRight})
		m = mm.(appModel)
	}
	if m.settingsTab != 4 {
		t.Fatalf("son sekme Ağ olmalı; sekme %d", m.settingsTab)
	}
	ekran := m.View()
	duz := ansi.Strip(ekran)
	for _, beklenen := range []string{"Ağ", "Bu düğüm", kisaPeerID(node.Host.ID().String()), "AND düğümleri", "Keşif çalışmıyor.", "kaydır"} {
		if !strings.Contains(duz, beklenen) {
			t.Errorf("Ağ sekmesinde %q yok:\n%s", beklenen, duz)
		}
	}
	for i, s := range strings.Split(ekran, "\n") {
		if lipgloss.Width(s) != 100 {
			t.Fatalf("satır %d genişliği %d", i, lipgloss.Width(s))
		}
	}
	// Kaydırma tuşları sekmede kalır.
	for _, k := range []tea.KeyType{tea.KeyDown, tea.KeyPgDown, tea.KeyUp} {
		mm, _ = m.handleSettingsKey(tea.KeyMsg{Type: k})
		m = mm.(appModel)
	}
	if m.settingsTab != 4 || m.screen != screenSettings {
		t.Fatal("kaydırma sekmeden çıkarmamalı")
	}
}
