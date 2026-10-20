package main

// network2_test.go — network.go'nun DHT/mDNS'in gerçek internet/LAN
// keşfine bağımlı OLMAYAN kısımlarının testleri (bkz. network_test.go'nun
// doc yorumu: kesifBaslat kasıtlı olarak burada test edilmiyor — gerçek
// bootstrap eşlerine bağlanmaya çalışırdı, testleri yavaş/kararsız yapardı).

import (
	"context"
	"testing"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestDugumOlustur_GecerliKimlikleHostOlusturur(t *testing.T) {
	k, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h, err := dugumOlustur(k)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close() //nolint:errcheck

	if h.ID().String() == "" {
		t.Fatal("host'un bir peer ID'si olmalıydı")
	}
	// Aynı kimlikten türetilen host, kimliğin kendi public key'iyle
	// eşleşen bir peer ID'ye sahip olmalı — libp2pOzelAnahtar'ın Kimlik'i
	// DOĞRU aktardığının dolaylı doğrulaması.
	priv, err := libp2pOzelAnahtar(k)
	if err != nil {
		t.Fatal(err)
	}
	wantID, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		t.Fatal(err)
	}
	if h.ID() != wantID {
		t.Fatalf("peer ID kimlikten türetilmemiş: got %s, want %s", h.ID(), wantID)
	}
}

func TestBaglanEgerYeniyse_KendineBaglanmayaCalismaz(t *testing.T) {
	h := testHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// pi.ID == h.ID() dalı erken dönmeli; Connect hiç çağrılmamalı (h zaten
	// kendine bağlı değil, çağrılsaydı da no-op olurdu ama asıl doğrulanan
	// şey panik atmadan/bloklanmadan hemen dönmesi).
	baglanEgerYeniyse(ctx, h, peer.AddrInfo{ID: h.ID()})
}

func TestBaglanEgerYeniyse_ZatenBagliysaTekrarDenemez(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	if h1.Network().Connectedness(h2.ID()) != libp2pnet.Connected {
		t.Fatal("test öncülü bozuk: h1-h2 bağlı olmalıydı")
	}
	// Zaten bağlıyken, kasıtlı olarak YANLIŞ (boş) adreslerle çağırmak
	// bunun gerçekten "zaten bağlı, hiçbir şey yapma" dalına girdiğini
	// kanıtlar — adresler doğru olsaydı bile bir Connect denemesi hatasız
	// dönerdi, ama BOŞ adreslerle bir bağlantı denemesi hataya yol açardı;
	// panik atmadan/hata olmadan dönmesi "zaten bağlı" kısayolunun
	// çalıştığını gösterir.
	baglanEgerYeniyse(ctx, h1, peer.AddrInfo{ID: h2.ID()})

	if h1.Network().Connectedness(h2.ID()) != libp2pnet.Connected {
		t.Fatal("bağlantı durumu değişmemeliydi")
	}
}

// TestPubsubBaslatVeKonuyaKatil_MesajGetirirVeKendiMesajiniFiltreler,
// iki gerçek (yerel) host'u pubsub ile aynı konuya bağlayıp bir mesajın
// gönderenden alıcıya ulaştığını VE gönderenin kendi mesajını (Mesajlar'ın
// GetFrom() == self.ID() kontrolü) görmediğini doğrular.
func TestPubsubBaslatVeKonuyaKatil_MesajGetirirVeKendiMesajiniFiltreler(t *testing.T) {
	h1, h2, ctx := ikiBagliHost(t)

	ps1, err := pubsubBaslat(ctx, h1)
	if err != nil {
		t.Fatal(err)
	}
	ps2, err := pubsubBaslat(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}

	const konuAdi = "test-konusu"
	k1, err := konuyaKatil(ps1, h1, konuAdi)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := konuyaKatil(ps2, h2, konuAdi)
	if err != nil {
		t.Fatal(err)
	}

	// GossipSub mesh'inin kurulması (peer keşfi + grafting) anlık değildir
	// — ilk yayının kaybolmaması için kısa bir yerleşme süresi bekleniyor.
	time.Sleep(500 * time.Millisecond)

	mesajlar2 := k2.Mesajlar(ctx)
	if err := k1.topic.Publish(ctx, []byte("merhaba")); err != nil {
		t.Fatal(err)
	}

	select {
	case data := <-mesajlar2:
		if string(data) != "merhaba" {
			t.Fatalf("beklenmeyen mesaj: %q", data)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mesaj zaman aşımına uğradı")
	}

	// k1 kendi yayınladığı mesajı GÖRMEMELİ (GetFrom() == self.ID() filtresi).
	mesajlar1 := k1.Mesajlar(ctx)
	select {
	case data := <-mesajlar1:
		t.Fatalf("gönderen kendi mesajını görmemeliydi: %q", data)
	case <-time.After(500 * time.Millisecond):
		// beklenen: hiçbir şey gelmedi.
	}
}
