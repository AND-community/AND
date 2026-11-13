//go:build canli

package network

// canli_test.go — GERÇEK ağa çıkan, elle çalıştırılan sınama; CI'da ve
// "go test ./..." içinde çalışmaz (canli derleme etiketi ister):
//
//	go test -tags canli -run TestCanli -v -count=1 -timeout 400s ./internal/network
//
// Geçici bir kimlik ve veri diziniyle genel libp2p ağına bağlanır, AND'ın
// keşfini (bilinen eşler, buluşma noktasında arama, eş değişimi, röle
// adayları) çalıştırır ve bu makinenin ağ durumunu yazar: erişilebilirlik,
// NAT türü, yer ayırtılan röleler, bulunan AND düğümleri. Buluşma noktasına
// DUYURU YAPMAZ: gerçek AND kullanıcıları bu geçici düğümü bulmaz.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
)

// yalnizArayan, buluşma noktasında arar ama duyuru yapmaz.
type yalnizArayan struct{ r *drouting.RoutingDiscovery }

func (yalnizArayan) Advertise(context.Context, string, ...discovery.Option) (time.Duration, error) {
	return 0, errors.New("canlı sınama duyuru yapmaz")
}

func (y yalnizArayan) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	return y.r.FindPeers(ctx, ns, opts...)
}

func TestCanli_Kesif(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	node, err := Yeni(id, Secenekler{VeriDizini: t.TempDir(), Surum: "canli-sinama"})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	ctx, iptal := context.WithTimeout(context.Background(), 240*time.Second)
	defer iptal()

	kdht, err := dual.New(ctx, node.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer kdht.Close()
	connectToBootstrapPeers(ctx, node.Host, nil)
	if err := kdht.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	routing := drouting.NewRoutingDiscovery(kdht)

	d := yeniKesif(node, yalnizArayan{routing}, varsayilanSureler)
	d.dht, d.routing = kdht, routing
	d.dhtHazir = func() bool { return kdht.WANActive() || kdht.LAN.RoutingTable().Size() > 0 }
	d.dhtEsleri = func() int { return kdht.WAN.RoutingTable().Size() }
	d.adresCoz = kdht.FindPeer
	node.SetRelaySource(d.roleAdaylari)
	d.baslat(ctx)

	t0 := time.Now()
	ilkRole := time.Duration(0)
	erisimAdi := map[Erisilebilirlik]string{ErisimBilinmiyor: "bilinmiyor", ErisimAcik: "açık", ErisimRoleIle: "röle ile", ErisimKapali: "kapalı"}
	natAdi := map[NATDurumu]string{NATBilinmiyor: "bilinmiyor", NATUygun: "uygun", NATZor: "zor"}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
		}
		s := node.Durum()
		if s.Roleler > 0 && ilkRole == 0 {
			ilkRole = time.Since(t0)
		}
		t.Logf("%5.0fs port %d | erişim %s, NAT %s, röle %d | eş %d (röle sunan aday %d), AND %d+%d | DHT %d, son arama: %d bulundu",
			time.Since(t0).Seconds(), s.Port, erisimAdi[s.Erisim], natAdi[s.NAT], s.Roleler,
			s.BagliEs, len(bagliRoleler(node.Host, 100)), s.ANDEs, s.SinirliEs, s.Kesif.DHTEsleri, s.Kesif.SonBulunan)
	}
	if ilkRole > 0 {
		t.Logf("SONUÇ: röleden yer ayırtıldı (%.0f sn sonra)", ilkRole.Seconds())
	} else {
		t.Logf("SONUÇ: röleden yer AYIRTILAMADI")
	}
	for _, a := range node.Host.Addrs() {
		if adresinTuru(a) == adresRole {
			t.Logf("röle adresi: %s", a)
		}
	}
}
