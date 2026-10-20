package main

// peers.go — son bilinen eş adreslerini diske kaydedip bir sonraki
// çalıştırmada doğrudan onlara da bağlanmayı dener. DHT/mDNS keşfi
// (network.go) tek başına yeterli olsa da, DHT rendezvous keşfi bir süre
// alabilir — daha önce başarıyla bağlandığımız eşleri doğrudan çevirmek,
// özellikle küçük bir AND topluluğunda, her açılışta bağlantı kurmayı
// gözle görülür şekilde hızlandırır.

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const bilinenPeerlerDosyaAdi = "bilinen_peerler.json"
const maxKayitliPeer = 200

type kayitliPeer struct {
	ID    string   `json:"id"`
	Addrs []string `json:"addrs"`
}

// bilinenPeerleriYukle, dataDir/bilinen_peerler.json içindeki eş
// adreslerini okur. Dosya yoksa ya da bozuksa (boş liste, hata değil)
// döner — bu salt bir hız optimizasyonu, DHT/mDNS keşfinin yerini tutmaz.
func bilinenPeerleriYukle(dataDir string) []peer.AddrInfo {
	yol := filepath.Join(dataDir, bilinenPeerlerDosyaAdi)
	data, err := os.ReadFile(yol)
	if err != nil {
		return nil
	}
	var kayitlar []kayitliPeer
	if json.Unmarshal(data, &kayitlar) != nil {
		return nil
	}
	var out []peer.AddrInfo
	for _, k := range kayitlar {
		pid, err := peer.Decode(k.ID)
		if err != nil {
			// Tek bir bozuk kayıt YÜZÜNDEN tüm dosyayı reddetmek gereksiz
			// olurdu (diğer kayıtlar hâlâ geçerli olabilir) — bu yüzden
			// yalnızca bu kayıt atlanır; ama sessizce değil, günlüğe
			// yazılır: aksi halde "neden bilinen eşlerin bir kısmı hiç
			// yüklenmiyor" gibi bir sorun teşhis edilemez hale gelirdi.
			gunlukUyari("bilinen_peerler.json: geçersiz peer ID atlandı:", k.ID, err)
			continue
		}
		var addrs []multiaddr.Multiaddr
		for _, a := range k.Addrs {
			ma, err := multiaddr.NewMultiaddr(a)
			if err != nil {
				gunlukUyari("bilinen_peerler.json:", pid, "için geçersiz adres atlandı:", a, err)
				continue
			}
			addrs = append(addrs, ma)
		}
		if len(addrs) == 0 {
			continue
		}
		out = append(out, peer.AddrInfo{ID: pid, Addrs: addrs})
	}
	return out
}

// bilinenPeerleriKaydet, şu an bağlı olduğumuz eşlerin adreslerini
// dataDir/bilinen_peerler.json'a yazar (bir sonraki açılışta hızlı
// yeniden bağlanmak için).
func bilinenPeerleriKaydet(dataDir string, h host.Host) error {
	var kayitlar []kayitliPeer
	for _, pid := range h.Network().Peers() {
		conns := h.Network().ConnsToPeer(pid)
		if len(conns) == 0 {
			continue
		}
		var addrs []string
		for _, c := range conns {
			addrs = append(addrs, c.RemoteMultiaddr().String())
		}
		if len(addrs) == 0 {
			continue
		}
		kayitlar = append(kayitlar, kayitliPeer{ID: pid.String(), Addrs: addrs})
		if len(kayitlar) >= maxKayitliPeer {
			break
		}
	}
	data, err := json.MarshalIndent(kayitlar, "", "  ")
	if err != nil {
		return err
	}
	return atomicYaz(filepath.Join(dataDir, bilinenPeerlerDosyaAdi), data, 0o600)
}
