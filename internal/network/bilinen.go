package network

// bilinen.go — daha önce bağlanılmış AND düğümlerinin kalıcı listesi.
//
// AND eşlerini genel DHT'deki buluşma noktasından bulur; bu, açılışta
// onlarca saniye sürebilir ve DHT'ye ulaşılamayan bir ağda hiç çalışmaz.
// Bilinen eşler listesi sayesinde düğüm açılır açılmaz, daha önce
// konuştuğu AND düğümlerine doğrudan bağlanmayı dener. Listeye yalnızca AND
// protokolünü konuştuğu doğrulanmış eşler girer; başkasının söylediği
// (ör. eş değişimiyle öğrenilen) bir adres, o eşe gerçekten bağlanılmadan
// diske yazılmaz.

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/lucian95511/and/internal/dosya"
)

const (
	bilinenDosyaAdi = "bilinen_esler.json"
	bilinenSurumu   = 1

	// bilinenEnFazla, listede tutulan en fazla eştir; dolunca en uzun
	// süredir görülmeyenler çıkar.
	bilinenEnFazla = 256
	// bilinenAdresSiniri, bir eş için saklanan en fazla adrestir.
	bilinenAdresSiniri = 8
	// bilinenRoleAdresSiniri, bunlardan en fazla kaçının röle adresi
	// olabileceğidir (röle adresleri çabuk eskir).
	bilinenRoleAdresSiniri = 2
	// bilinenOmru, bu kadar süredir görülmeyen eşin listeden çıkarıldığı süredir.
	bilinenOmru = 30 * 24 * time.Hour
	// bilinenCokBasarisiz, art arda bu kadar bağlanma denemesi boşa çıkan
	// eşin adaylar arasında sona atıldığı sayıdır.
	bilinenCokBasarisiz = 3
)

type bilinenKayit struct {
	adresler  []ma.Multiaddr
	gorulme   time.Time
	basarisiz int
}

// BilinenEsler, bilinen AND düğümlerinin listesidir. Yöntemleri eşzamanlı
// çağrılabilir.
type BilinenEsler struct {
	yol   string // boşsa liste yalnızca bellektedir
	simdi func() time.Time

	mu    sync.Mutex
	esler map[peer.ID]*bilinenKayit
	kirli bool
}

type bilinenDosyaKaydi struct {
	ID        string    `json:"id"`
	Adresler  []string  `json:"adresler"`
	Gorulme   time.Time `json:"son_gorulme"`
	Basarisiz int       `json:"basarisiz,omitempty"`
}

type bilinenDosya struct {
	Surum int                 `json:"surum"`
	Esler []bilinenDosyaKaydi `json:"esler"`
}

// BilinenAc, yol'daki listeyi yükler; yol boşsa liste yalnızca bellekte
// tutulur. Dosya yoksa liste boştur. Okunamayan dosya kenara alınır (bir
// sonraki kayıt üzerine yazmasın) ve boş listeyle başlanır: liste yalnızca
// bir hızlandırmadır, kaybı keşfi durdurmaz.
func BilinenAc(yol string) *BilinenEsler {
	b := &BilinenEsler{yol: yol, simdi: time.Now, esler: map[peer.ID]*bilinenKayit{}}
	if yol == "" {
		return b
	}
	veri, err := os.ReadFile(yol)
	if err != nil {
		return b
	}
	var d bilinenDosya
	if err := json.Unmarshal(veri, &d); err != nil {
		_, _ = dosya.KenaraAl(yol, "bozuk")
		return b
	}
	simdi := b.simdi()
	for _, k := range d.Esler {
		pid, err := peer.Decode(k.ID)
		if err != nil || simdi.Sub(k.Gorulme) > bilinenOmru {
			continue
		}
		var adresler []ma.Multiaddr
		for _, s := range k.Adresler {
			if a, err := ma.NewMultiaddr(s); err == nil {
				adresler = append(adresler, a)
			}
		}
		adresler = adresleriSec(adresler)
		if len(adresler) == 0 {
			continue
		}
		// Gelecekteki bir zaman (saat geri alınmış, dosya elle değiştirilmiş)
		// eşi sonsuza dek listenin başında tutmasın.
		gorulme := k.Gorulme
		if gorulme.After(simdi) {
			gorulme = simdi
		}
		b.esler[pid] = &bilinenKayit{adresler: adresler, gorulme: gorulme, basarisiz: max(k.Basarisiz, 0)}
	}
	b.budaLocked()
	return b
}

// Gor, p'nin şu an bağlı bir AND düğümü olduğunu ve adreslerini kaydeder.
// Yeni adresler başa geçer; eskilerden yer kalanlar korunur (dünkü adres
// yarın yine geçerli olabilir).
func (b *BilinenEsler) Gor(p peer.ID, adresler []ma.Multiaddr) {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := b.esler[p]
	if k == nil {
		k = &bilinenKayit{}
		b.esler[p] = k
	}
	k.adresler = adresleriSec(append(slices.Clone(adresler), k.adresler...))
	k.gorulme, k.basarisiz, b.kirli = b.simdi(), 0, true
	if len(k.adresler) == 0 {
		// Adresi bilinmeyen eşe yeniden bağlanılamaz.
		delete(b.esler, p)
		return
	}
	b.budaLocked()
}

// Goruldu, listedeki p'nin son görülme zamanını şimdi yapar (bağlantı
// koparken ve kapanışta); p listede değilse bir şey yapmaz.
func (b *BilinenEsler) Goruldu(p peer.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if k := b.esler[p]; k != nil {
		k.gorulme, b.kirli = b.simdi(), true
	}
}

// Basarisiz, p'ye kayıtlı adreslerinden bağlanılamadığını kaydeder.
func (b *BilinenEsler) Basarisiz(p peer.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if k := b.esler[p]; k != nil {
		k.basarisiz++
		b.kirli = true
	}
}

// Adresler, p'nin kayıtlı adresleridir (yoksa nil).
func (b *BilinenEsler) Adresler(p peer.ID) []ma.Multiaddr {
	b.mu.Lock()
	defer b.mu.Unlock()
	if k := b.esler[p]; k != nil {
		return slices.Clone(k.adresler)
	}
	return nil
}

// Say, listedeki eş sayısıdır.
func (b *BilinenEsler) Say() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.esler)
}

// Adaylar, bağlanmayı denemeye değer en fazla n eşi verir: en son
// görülenler önce, art arda bağlanılamayanlar sonda. atla true dönen eşler
// (ör. zaten bağlı olanlar) atlanır; atla nil olabilir.
func (b *BilinenEsler) Adaylar(n int, atla func(peer.ID) bool) []peer.AddrInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.budaLocked()
	type aday struct {
		id peer.ID
		k  *bilinenKayit
	}
	adaylar := make([]aday, 0, len(b.esler))
	for id, k := range b.esler {
		if atla != nil && atla(id) {
			continue
		}
		adaylar = append(adaylar, aday{id, k})
	}
	sort.Slice(adaylar, func(i, j int) bool {
		x, y := adaylar[i], adaylar[j]
		if xk, yk := x.k.basarisiz >= bilinenCokBasarisiz, y.k.basarisiz >= bilinenCokBasarisiz; xk != yk {
			return yk
		}
		if !x.k.gorulme.Equal(y.k.gorulme) {
			return x.k.gorulme.After(y.k.gorulme)
		}
		return x.id < y.id
	})
	out := make([]peer.AddrInfo, 0, min(n, len(adaylar)))
	for _, a := range adaylar[:min(n, len(adaylar))] {
		out = append(out, peer.AddrInfo{ID: a.id, Addrs: slices.Clone(a.k.adresler)})
	}
	return out
}

// Kaydet, liste değiştiyse diske yazar.
func (b *BilinenEsler) Kaydet() error {
	b.mu.Lock()
	if b.yol == "" || !b.kirli {
		b.mu.Unlock()
		return nil
	}
	b.budaLocked()
	d := bilinenDosya{Surum: bilinenSurumu, Esler: make([]bilinenDosyaKaydi, 0, len(b.esler))}
	for id, k := range b.esler {
		kayit := bilinenDosyaKaydi{ID: id.String(), Gorulme: k.gorulme.UTC().Truncate(time.Second), Basarisiz: k.basarisiz}
		for _, a := range k.adresler {
			kayit.Adresler = append(kayit.Adresler, a.String())
		}
		d.Esler = append(d.Esler, kayit)
	}
	b.kirli = false
	b.mu.Unlock()

	sort.Slice(d.Esler, func(i, j int) bool {
		if !d.Esler[i].Gorulme.Equal(d.Esler[j].Gorulme) {
			return d.Esler[i].Gorulme.After(d.Esler[j].Gorulme)
		}
		return d.Esler[i].ID < d.Esler[j].ID
	})
	veri, err := json.MarshalIndent(d, "", "  ")
	if err == nil {
		err = dosya.AtomikYaz(b.yol, append(veri, '\n'), 0o600)
	}
	if err != nil {
		b.mu.Lock()
		b.kirli = true // bir sonraki turda yeniden denensin
		b.mu.Unlock()
	}
	return err
}

// budaLocked, süresi dolan eşleri ve sınırı aşan en eski eşleri çıkarır.
// b.mu tutulurken çağrılır.
func (b *BilinenEsler) budaLocked() {
	simdi := b.simdi()
	for id, k := range b.esler {
		if simdi.Sub(k.gorulme) > bilinenOmru {
			delete(b.esler, id)
			b.kirli = true
		}
	}
	if len(b.esler) <= bilinenEnFazla {
		return
	}
	idler := make([]peer.ID, 0, len(b.esler))
	for id := range b.esler {
		idler = append(idler, id)
	}
	sort.Slice(idler, func(i, j int) bool {
		x, y := b.esler[idler[i]], b.esler[idler[j]]
		if !x.gorulme.Equal(y.gorulme) {
			return x.gorulme.After(y.gorulme)
		}
		return idler[i] < idler[j]
	})
	for _, id := range idler[bilinenEnFazla:] {
		delete(b.esler, id)
	}
	b.kirli = true
}

// adresTuru, bir adresin yeniden bağlanmadaki değeridir.
type adresTuru int

const (
	adresKullanilmaz adresTuru = iota // bağlantı-yerel, belirsiz, taşıyıcısız
	adresAcik                         // internetten erişilebilir, doğrudan
	adresYerel                        // özel ağ (ev, okul, iş yeri)
	adresLoopback                     // aynı makine (127.0.0.1, ::1)
	adresRole                         // bir röle üzerinden (/p2p-circuit)
)

// adresinTuru, a'yı sınıflandırır. Yalnızca IP ya da alan adıyla başlayan
// adresler kullanılır: röle adresinde bu, rölenin kendi adresidir.
func adresinTuru(a ma.Multiaddr) adresTuru {
	if a == nil || len(a.Bytes()) == 0 {
		return adresKullanilmaz
	}
	role := false
	ma.ForEach(a, func(c ma.Component) bool {
		if c.Protocol().Code == ma.P_CIRCUIT {
			role = true
			return false
		}
		return true
	})
	ip, err := manet.ToIP(a)
	if err != nil {
		// IP değil: alan adıysa (dns, dns4, dns6) açık adres sayılır.
		if !manet.IsPublicAddr(a) {
			return adresKullanilmaz
		}
	} else if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return adresKullanilmaz
	} else if ip.IsLoopback() {
		if role {
			return adresKullanilmaz
		}
		return adresLoopback
	}
	switch {
	case role:
		// Röle özel bir adresteyse (yerel ağdaki bir röle) dışarıdan işe yaramaz.
		if !manet.IsPublicAddr(a) {
			return adresKullanilmaz
		}
		return adresRole
	case manet.IsPublicAddr(a):
		return adresAcik
	case manet.IsPrivateAddr(a):
		return adresYerel
	default:
		// Yönlendirilemeyen aralıklar (belgeleme adresleri vb.).
		return adresKullanilmaz
	}
}

// adresleriSec, saklanacak adresleri seçer: kullanılmayanlar ve tekrarlar
// atılır; açık adresler önce, yerel ağ adresleri sonra, röle adresleri en
// sonda; her türün içinde verilen sıra korunur. Loopback adresleri de
// saklanır: identify onları yalnızca aynı makinedeki eşler için kabul eder.
func adresleriSec(adresler []ma.Multiaddr) []ma.Multiaddr {
	var acik, yerel, role []ma.Multiaddr
	gorulen := map[string]bool{}
	for _, a := range adresler {
		tur := adresinTuru(a)
		if tur == adresKullanilmaz || gorulen[string(a.Bytes())] {
			continue
		}
		gorulen[string(a.Bytes())] = true
		switch tur {
		case adresAcik:
			acik = append(acik, a)
		case adresYerel, adresLoopback:
			yerel = append(yerel, a)
		case adresRole:
			if len(role) < bilinenRoleAdresSiniri {
				role = append(role, a)
			}
		}
	}
	// Röle adresi, başka adresi olmayan (NAT arkasındaki) bir eşe ulaşmanın
	// tek yolu olabilir: sınır dolsa da ona yer bırakılır.
	dogrudan := append(acik, yerel...)
	if kalan := bilinenAdresSiniri - len(role); len(dogrudan) > kalan {
		dogrudan = dogrudan[:kalan]
	}
	return append(dogrudan, role...)
}

// saklanacakAdresler, bağlı p eşi için bilinen eşler listesine yazılacak
// adreslerdir: önce gerçekten işe yaramış olanlar (bizim çevirip
// bağlandığımız adresler; eş bunları kendisi duyurmuyor olabilir, ör. elle
// port yönlendirilmiş bir modemin arkasında), sonra eşin kendi bildirdikleri.
func saklanacakAdresler(h host.Host, p peer.ID) []ma.Multiaddr {
	var adresler []ma.Multiaddr
	for _, c := range h.Network().ConnsToPeer(p) {
		if c.Stat().Direction == libp2pnet.DirOutbound {
			adresler = append(adresler, c.RemoteMultiaddr())
		}
	}
	return append(adresler, h.Peerstore().Addrs(p)...)
}
