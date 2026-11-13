package dmmgr

// role_test.go — özel mesajın, iki düğüm birbirine yalnızca bir röle
// üzerinden (süresi ve veri miktarı sınırlı bir bağlantıyla) ulaşabildiğinde
// ve alıcı o an bağlı değilken de gönderilebildiğini sınar.

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"
)

// testRolesi, loopback'te çalışan ve varsayılan (sınırlı) röle hizmetini
// sunan bir düğümdür; adresini libp2p'nin açık adres saydığı bir alan adıyla
// duyurur (libp2p'nin kendi testlerindeki yöntem).
func testRolesi(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.DisableRelay(),
		libp2p.EnableRelayService(),
		libp2p.ForceReachabilityPublic(),
		libp2p.AddrsFactory(func(adr []ma.Multiaddr) []ma.Multiaddr {
			out := make([]ma.Multiaddr, 0, len(adr))
			for _, a := range adr {
				if s := a.String(); strings.HasPrefix(s, "/ip4/127.0.0.1/") {
					a = ma.StringCast("/dns/libp2p.internal" + strings.TrimPrefix(s, "/ip4/127.0.0.1"))
				}
				out = append(out, a)
			}
			return out
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// roleArkasindakiDugum, NAT arkasındaymış gibi davranan ve role'den yer
// ayırtmış bir düğüm kurar.
func roleArkasindakiDugum(t *testing.T, role host.Host) *network.Node {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	roleBilgisi := peer.AddrInfo{ID: role.ID(), Addrs: role.Network().ListenAddresses()}
	n, err := network.Yeni(id, network.Secenekler{
		DinlemeAdresleri: []string{"/ip4/127.0.0.1/tcp/0"},
		Libp2p: []libp2p.Option{
			libp2p.ForceReachabilityPrivate(),
			libp2p.EnableAutoRelayWithStaticRelays([]peer.AddrInfo{roleBilgisi}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	bitis := time.Now().Add(15 * time.Second)
	for !slices.ContainsFunc(n.Host.Addrs(), func(a ma.Multiaddr) bool { return strings.Contains(a.String(), "p2p-circuit") }) {
		if time.Now().After(bitis) {
			t.Fatal("röleden yer ayırtılamadı")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return n
}

// roleUzerindenBagla, a'yı b'ye yalnızca röle üzerinden bağlar.
func roleUzerindenBagla(t *testing.T, a, b *network.Node, role host.Host) {
	t.Helper()
	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	roleYerel := role.Network().ListenAddresses()[0]
	if err := a.Host.Connect(ctx, peer.AddrInfo{ID: role.ID(), Addrs: []ma.Multiaddr{roleYerel}}); err != nil {
		t.Fatal(err)
	}
	devre := roleYerel.Encapsulate(ma.StringCast("/p2p/" + role.ID().String() + "/p2p-circuit"))
	if err := a.Host.Connect(ctx, peer.AddrInfo{ID: b.Host.ID(), Addrs: []ma.Multiaddr{devre}}); err != nil {
		t.Fatalf("röle üzerinden bağlanılamadı: %v", err)
	}
	if c := a.Host.Network().Connectedness(b.Host.ID()); c != libp2pnet.Limited {
		t.Fatalf("bağlantı türü %v; röle üzerinden sınırlı olmalı", c)
	}
}

// Delik açma başarısız olduğunda (ör. simetrik NAT) iki düğüm arasında yalnızca
// röle üzerinden sınırlı bir bağlantı kalır. Özel mesaj küçüktür ve uçtan uca
// şifrelidir: o bağlantıdan da gönderilebilmelidir. Eskiden "limited
// connection" hatasıyla gönderilemiyordu.
func TestSendDM_RoleUzerinden(t *testing.T) {
	role := testRolesi(t)
	nodeB := roleArkasindakiDugum(t, role)
	nodeA := newTestNode(t)
	brokerA, brokerB := New(nodeA), New(nodeB)
	roleUzerindenBagla(t, nodeA, nodeB, role)

	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)
	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Ayşe", "röleden merhaba"); err != nil {
		t.Fatalf("röle üzerinden özel mesaj gönderilemedi: %v", err)
	}
	select {
	case msg := <-ch:
		if msg.Text != "röleden merhaba" || msg.VerifiedPeerID != nodeA.Host.ID().String() {
			t.Fatalf("gelen mesaj: %+v", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mesaj ulaşmadı")
	}

	// Yanıt da aynı sınırlı bağlantıdan döner (B'nin A'ya başka yolu yok).
	chA := brokerA.Subscribe()
	defer brokerA.Unsubscribe(chA)
	if err := brokerB.SendDM(ctx, nodeA.Host.ID().String(), "Mert", "aldım"); err != nil {
		t.Fatalf("röle üzerinden yanıt gönderilemedi: %v", err)
	}
	select {
	case msg := <-chA:
		if msg.Text != "aldım" {
			t.Fatalf("gelen yanıt: %+v", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("yanıt ulaşmadı")
	}
}

// Giden kutusunda bekleyen mesaj, alıcıya yalnızca röle üzerinden
// ulaşılabildiğinde de gönderilir.
func TestGidenKutusu_RoleUzerindenBosalir(t *testing.T) {
	role := testRolesi(t)
	nodeB := roleArkasindakiDugum(t, role)
	nodeA := newTestNode(t)
	brokerA, brokerB := New(nodeA), New(nodeB)

	ctx, iptal := context.WithTimeout(context.Background(), 20*time.Second)
	defer iptal()
	// B'nin adresi bilinmiyor: mesaj giden kutusuna alınır.
	if err := brokerA.SendDMQueued(ctx, nodeB.Host.ID().String(), "Ayşe", "sonra oku"); err != nil {
		t.Fatal(err)
	}
	if h := brokerA.History(); len(h) != 1 || !h[0].Pending {
		t.Fatalf("mesaj giden kutusunda beklemeli: %+v", h)
	}

	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)
	roleUzerindenBagla(t, nodeA, nodeB, role)
	select {
	case msg := <-ch:
		if msg.Text != "sonra oku" {
			t.Fatalf("gelen mesaj: %+v", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bekleyen mesaj röle bağlantısı kurulunca gönderilmedi")
	}
}

// Alıcı o an bağlı değilse ama daha önce bağlanılmış bir AND düğümüyse,
// bilinen eşler listesindeki adresinden çevrilir. Eskiden bağlantı koptuktan
// bir süre sonra (adresler unutulunca) mesaj "peer'a bağlanılamadı" ile
// reddediliyordu.
func TestSendDM_BilinenAdrestenCevirir(t *testing.T) {
	nodeA, nodeB := newTestNode(t), newTestNode(t)
	brokerA, brokerB := New(nodeA), New(nodeB)
	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)

	ctx, iptal := context.WithTimeout(context.Background(), 10*time.Second)
	defer iptal()
	// Adresi hiçbir yerde yokken gönderilemez.
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Ayşe", "merhaba"); err == nil {
		t.Fatal("adresi bilinmeyen eşe gönderim hata vermeli")
	}
	nodeA.Bilinen.Gor(nodeB.Host.ID(), nodeB.Host.Addrs())
	if err := brokerA.SendDM(ctx, nodeB.Host.ID().String(), "Ayşe", "merhaba"); err != nil {
		t.Fatalf("bilinen adresten gönderilemedi: %v", err)
	}
	select {
	case msg := <-ch:
		if msg.Text != "merhaba" {
			t.Fatalf("gelen mesaj: %+v", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mesaj ulaşmadı")
	}
}

// Giden kutusunda bekleyen mesaj için alıcı aranır: bağlanmasını beklemek
// yerine kayıtlı adresinden çevrilir.
func TestGidenKutusu_AliciAranir(t *testing.T) {
	nodeA, nodeB := newTestNode(t), newTestNode(t)
	brokerA, brokerB := New(nodeA), New(nodeB)
	ch := brokerB.Subscribe()
	defer brokerB.Unsubscribe(ch)

	ctx, iptal := context.WithTimeout(context.Background(), 20*time.Second)
	defer iptal()
	if err := brokerA.SendDMQueued(ctx, nodeB.Host.ID().String(), "Ayşe", "bekleyen"); err != nil {
		t.Fatal(err)
	}
	// Alıcının adresi bilinmezken deneme sonuçsuz kalır, mesaj kutuda durur.
	if n := brokerA.BekleyenleriDene(8); n != 1 {
		t.Fatalf("%d alıcı denendi; 1 beklenirdi", n)
	}
	select {
	case msg := <-ch:
		t.Fatalf("adres bilinmeden mesaj gitti: %+v", msg)
	case <-time.After(300 * time.Millisecond):
	}
	if h := brokerA.History(); len(h) != 1 || !h[0].Pending {
		t.Fatalf("mesaj kutuda beklemeli: %+v", h)
	}

	// Alıcı bilinen eşler listesinde: bir sonraki turda çevrilir ve mesaj gider.
	nodeA.Bilinen.Gor(nodeB.Host.ID(), nodeB.Host.Addrs())
	bitis := time.Now().Add(10 * time.Second)
	for {
		brokerA.BekleyenleriDene(8)
		select {
		case msg := <-ch:
			if msg.Text != "bekleyen" {
				t.Fatalf("gelen mesaj: %+v", msg)
			}
			if brokerA.BekleyenleriDene(8) != 0 {
				// Gönderim kaydı güncellenene kadar kısa bir an sürebilir.
				time.Sleep(200 * time.Millisecond)
			}
			return
		case <-time.After(300 * time.Millisecond):
		}
		if time.Now().After(bitis) {
			t.Fatal("bekleyen mesaj alıcı aranarak gönderilmedi")
		}
	}
}
