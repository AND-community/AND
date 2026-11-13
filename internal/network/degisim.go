package network

// degisim.go — eş değişimi (/and/pex/1.0.0).
//
// Bir AND düğümü, bağlı olduğu öteki AND düğümlerinin adres kayıtlarını
// isteyene verir. Böylece tek bir AND düğümüne ulaşan (bilinen eşler
// listesinden, yerel ağdan ya da DHT'den) ötekilerini DHT'ye gerek kalmadan
// bulur; ağ, DHT'ye ulaşılamadığında da kendi kendini toparlar.
//
// Verilen her kayıt, anlattığı düğümün KENDİ imzaladığı libp2p adres
// kaydıdır (identify'da alınan zarf, aynen). Yanıtlayan düğüm başkası adına
// adres uyduramaz: imzası tutmayan ya da imzalayanla anlattığı düğüm farklı
// olan kayıt atılır. Kötü niyetli bir düğümün yapabileceği tek şey, kendi
// ürettiği kimlikler için kayıt vermektir; bu da çevrilecek adres sayısıyla
// sınırlıdır (DHT'deki buluşma kaydı da aynı ölçüde güvenilirdir).
//
// Eski AND sürümleri protokolü konuşmaz; onlara hiç sorulmaz.

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/record"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/lucian95511/and/internal/guvenli"
	"github.com/lucian95511/and/protokol"
)

// EsDegisimProtokolu, eş değişimi akışının protokol kimliğidir.
const EsDegisimProtokolu = protocol.ID(protokol.EsDegisimProtokolu)

const (
	degisimSurumu = 1
	// degisimEnFazlaKayit, tek yanıtta verilen (ve kabul edilen) en fazla kayıttır.
	degisimEnFazlaKayit = 24
	// degisimKayitSiniri, tek bir imzalı kaydın en büyük boyudur (libp2p'nin
	// identify'da kabul ettiği sınır).
	degisimKayitSiniri = 8 << 10
	// degisimAdresSiniri, bir kayıttan kullanılan en fazla adrestir.
	degisimAdresSiniri = 16
	degisimIstekSiniri = 512
	// degisimYanitSiniri: 24 kayıt × 8 KiB, base64 ve JSON payıyla.
	degisimYanitSiniri = 512 << 10
	degisimZamanAsimi  = 15 * time.Second
	// degisimEnAzAralik, aynı eşin iki isteği arasında olması gereken süredir.
	degisimEnAzAralik = 20 * time.Second
	// degisimIzlenenEs, istek zamanı izlenen en fazla eştir.
	degisimIzlenenEs = 1024
)

type degisimIstegi struct {
	V int `json:"v"`
	// N, istenen en fazla kayıt sayısıdır (0: yanıtlayanın sınırı).
	N int `json:"n,omitempty"`
}

type degisimYaniti struct {
	V int `json:"v"`
	// Esler, her biri bir düğümün kendi imzaladığı adres kaydıdır (libp2p
	// record.Envelope, protobuf; JSON'da base64).
	Esler [][]byte `json:"esler"`
}

// degisimSiniri, eş başına istek sıklığını sınırlar.
type degisimSiniri struct {
	mu  sync.Mutex
	son map[peer.ID]time.Time
}

func (s *degisimSiniri) izinVer(p peer.ID, simdi time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.son == nil {
		s.son = map[peer.ID]time.Time{}
	}
	if t, ok := s.son[p]; ok && simdi.Sub(t) < degisimEnAzAralik {
		return false
	}
	if len(s.son) >= degisimIzlenenEs {
		for k, t := range s.son {
			if simdi.Sub(t) >= degisimEnAzAralik {
				delete(s.son, k)
			}
		}
		if len(s.son) >= degisimIzlenenEs {
			// Kısa sürede çok fazla farklı eş soruyor: yenilerini geri çevir.
			return false
		}
	}
	s.son[p] = simdi
	return true
}

// degisimIsle, bir eşin "bağlı olduğun AND düğümlerini ver" isteğini yanıtlar.
func (n *Node) degisimIsle(s libp2pnet.Stream) {
	defer s.Close()
	// Eşin gönderdiği veri işlenirken çıkan bir hata yalnızca bu akışı
	// kapatır; düğüm çökmez (bkz. guvenli).
	defer guvenli.Kurtar("eş değişimi akışı", func() { _ = s.Reset() })

	p := s.Conn().RemotePeer()
	_ = s.SetDeadline(time.Now().Add(degisimZamanAsimi))
	if !n.degisim.izinVer(p, time.Now()) {
		_ = s.Reset()
		return
	}
	var istek degisimIstegi
	if err := json.NewDecoder(io.LimitReader(s, degisimIstekSiniri)).Decode(&istek); err != nil {
		_ = s.Reset()
		return
	}
	adet := istek.N
	if adet <= 0 || adet > degisimEnFazlaKayit {
		adet = degisimEnFazlaKayit
	}
	yanit := degisimYaniti{V: degisimSurumu, Esler: n.Esler.imzaliKayitlar(p, adet)}
	if yanit.Esler == nil {
		yanit.Esler = [][]byte{}
	}
	if err := json.NewEncoder(s).Encode(yanit); err != nil {
		_ = s.Reset()
	}
}

// EsleriIste, p'den bağlı olduğu öteki AND düğümlerini ister ve imzası
// doğrulanmış, çevrilebilir adresi olan kayıtları döndürür. Dönen eşlere
// bağlanılıp bağlanılmayacağına çağıran karar verir; hiçbiri AND düğümü
// olduğu doğrulanmadan bilinen eşler listesine girmez.
func (n *Node) EsleriIste(ctx context.Context, p peer.ID) ([]peer.AddrInfo, error) {
	ctx, iptal := context.WithTimeout(ctx, degisimZamanAsimi)
	defer iptal()
	s, err := n.Host.NewStream(ctx, p, EsDegisimProtokolu)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(degisimZamanAsimi))
	if err := json.NewEncoder(s).Encode(degisimIstegi{V: degisimSurumu, N: degisimEnFazlaKayit}); err != nil {
		_ = s.Reset()
		return nil, err
	}
	_ = s.CloseWrite()
	var yanit degisimYaniti
	if err := json.NewDecoder(io.LimitReader(s, degisimYanitSiniri)).Decode(&yanit); err != nil {
		_ = s.Reset()
		return nil, err
	}
	return kayitlariCoz(yanit.Esler, n.Host.ID(), baglantiGuveni(n.Host, p)), nil
}

// adresGuveni, bir eşten öğrenilen adreslerden hangilerine çevrileceğidir.
type adresGuveni int

const (
	// yalnizAcik: yalnızca internetten erişilebilir adresler ve röle
	// adresleri. İnternetteki bir eş, bu düğümü kendi yerel ağındaki
	// adresleri çevirmeye yönlendiremez.
	yalnizAcik adresGuveni = iota
	// yerelDahil: özel ağ adresleri de. Kayıtları veren eşle zaten özel bir
	// ağ üzerinden konuşuluyorsa (aynı ev, okul ya da iş yeri ağı) onun
	// komşularının özel adresleri de erişilebilir olabilir.
	yerelDahil
	// loopbackDahil: aynı makinedeki adresler de (kayıtları veren eş bu
	// makinedeyse).
	loopbackDahil
)

// baglantiGuveni, p ile kurulu bağlantının türüne göre ondan öğrenilen
// adreslere ne kadar güvenileceğini verir.
func baglantiGuveni(h host.Host, p peer.ID) adresGuveni {
	guven := yalnizAcik
	for _, c := range h.Network().ConnsToPeer(p) {
		if c.Stat().Limited {
			continue
		}
		switch adresinTuru(c.RemoteMultiaddr()) {
		case adresLoopback:
			return loopbackDahil
		case adresYerel:
			guven = yerelDahil
		}
	}
	return guven
}

// kayitlariCoz, bir eş değişimi yanıtındaki imzalı kayıtları doğrular ve
// çevrilebilir adresleriyle döndürür. Bozuk, imzası tutmayan, kendimize ait
// ya da kullanılabilir adresi kalmayan kayıtlar atılır; aynı düğüm bir kez
// sayılır.
func kayitlariCoz(kayitlar [][]byte, kendim peer.ID, guven adresGuveni) []peer.AddrInfo {
	var out []peer.AddrInfo
	gorulen := map[peer.ID]bool{kendim: true}
	for i, b := range kayitlar {
		if i >= degisimEnFazlaKayit {
			break
		}
		ai, ok := kayitCoz(b)
		if !ok || gorulen[ai.ID] {
			continue
		}
		gorulen[ai.ID] = true
		ai.Addrs = guvenilenAdresler(ai.Addrs, guven)
		if len(ai.Addrs) > 0 {
			out = append(out, ai)
		}
	}
	return out
}

// kayitCoz, imzalı bir libp2p adres kaydını doğrular: imza geçerli olmalı
// ve kaydı imzalayan anahtar, kaydın anlattığı düğümün anahtarı olmalıdır.
func kayitCoz(b []byte) (peer.AddrInfo, bool) {
	if len(b) == 0 || len(b) > degisimKayitSiniri {
		return peer.AddrInfo{}, false
	}
	zarf, kayit, err := record.ConsumeEnvelope(b, peer.PeerRecordEnvelopeDomain)
	if err != nil || zarf.PublicKey == nil {
		return peer.AddrInfo{}, false
	}
	pr, ok := kayit.(*peer.PeerRecord)
	if !ok {
		return peer.AddrInfo{}, false
	}
	imzalayan, err := peer.IDFromPublicKey(zarf.PublicKey)
	if err != nil || imzalayan != pr.PeerID {
		return peer.AddrInfo{}, false
	}
	return peer.AddrInfo{ID: pr.PeerID, Addrs: pr.Addrs}, true
}

// guvenilenAdresler, adreslerden güven düzeyinin izin verdiklerini seçer.
func guvenilenAdresler(adresler []ma.Multiaddr, guven adresGuveni) []ma.Multiaddr {
	var out []ma.Multiaddr
	for _, a := range adresler {
		switch adresinTuru(a) {
		case adresAcik, adresRole:
		case adresYerel:
			if guven < yerelDahil {
				continue
			}
		case adresLoopback:
			if guven < loopbackDahil {
				continue
			}
		default:
			continue
		}
		out = append(out, a)
		if len(out) >= degisimAdresSiniri {
			break
		}
	}
	return out
}
