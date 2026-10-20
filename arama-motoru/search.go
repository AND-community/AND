package main

// search.go — bu programın KENDİ protokolü: aynı programı çalıştıran
// eşler arasında doğrudan bir libp2p stream'i üzerinden arama sorgusu/
// yanıtı alışverişi. AND'ın DM kanalının aksine (~16 KB mesaj sınırı,
// kimliği yalnızca dolaylı olarak taşıyan bir "from" alanı), burada:
//   - Boyut sınırı yok (bir stream, DM'in aksine rastgele uzunlukta veri
//     taşıyabilir) — sonuçlar kısaltılmadan tam gövdeyle taşınabilir.
//   - Gönderenin kimliği, stream'in bağlı olduğu libp2p bağlantısından
//     (st.Conn().RemotePeer()) doğrudan ve sahtesi mümkün olmayan şekilde
//     bilinir — self-declared bir alana güvenmeye hiç gerek yok.
//
// ÇOK ATLAMALI YAYILMA: bir sorgu yalnızca doğrudan bağlı eşlere değil,
// TTL (Time-To-Live, "kaç atlama") sayısı kadar öteye de yayılır. Her
// düğüm, bir sorguyu aldığında (a) kendi yerel indeksinde arar VE (b) TTL
// hâlâ 1'den büyükse, sorguyu (TTL-1 ile) kendi doğrudan eşlerine —
// sorguyu ilk gönderen hariç — ileterek onların da yanıtlarını toplar ve
// hepsini TEK bir birleşik yanıt olarak geri döner. Böylece istemci
// yalnızca kendi doğrudan eşlerine değil, ağın birkaç atlama ötesine
// kadar aynı programı çalıştıran her düğüme ulaşabilir (klasik
// Gnutella-tarzı "flood & aggregate" deseni).
//
// Sonsuz döngü/gereksiz tekrar koruması: her sorgu, üretildiği düğümde
// rastgele bir SorguID alır ve bu ID, ağ genelinde AYNI kullanıcı
// araması için PAYLAŞILIR (bkz. yeniAramaIstegi). Bir düğüm aynı
// SorguID'yi ikinci kez gördüğünde (mesh topolojisinde birden fazla
// yoldan gelebilir) onu bir daha İLETMEZ — yalnızca kendi yerel
// sonuçlarıyla yanıt verir; bu da sorguOnbellek ile sağlanır.
//
// Kötüye kullanıma karşı savunma katmanları: hizSiniri (bir eşten gelen
// istek sıklığını sınırlar — hem doğrudan hem iletilen istekler için
// geçerlidir, çünkü iletilen bir istek de normal bir yeni stream'dir),
// sorguOnbellek (aynı sorgunun tekrar tekrar iletilmesini önler), TTL
// üst sınırı (bir düğüm, istemcinin bildirdiği TTL ne olursa olsun kendi
// aramaVarsayilanTTL sınırını aşamaz) ve connmgr'ın (network.go) zaten
// uyguladığı toplam bağlı eş sayısı sınırı (dolayısıyla bir düğümün tek
// bir sorgu için yapabileceği iletim sayısı da örtük olarak sınırlıdır).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"slices"
	"sort"
	"sync"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/lucian95511/and/protokol"
)

const aramaProtokolu = protokol.AramaProtokolu
const aramaZamanAsimi = 20 * time.Second
const maxAramaIstekBayt = 4 * 1024
const maxAramaYanitBayt = 4 * 1024 * 1024
const maxAgSonucSayisi = 50

const (
	// maxSorguRune, ağdan gelen bir sorgu metninin en fazla uzunluğudur —
	// çok uzun ya da çok terimli bir FTS sorgusu düğümü yorabilir.
	maxSorguRune = 256
	// maxEszamanliGelenSorgu, aynı anda işlenen gelen sorgu sayısının üst
	// sınırıdır: eş başına hız sınırı, çok sayıda FARKLI eşin birlikte
	// yüklenmesini durdurmaz.
	maxEszamanliGelenSorgu = 16
	// maxIletimHedefi, başka bir eşten gelen bir sorgunun tek atlamada
	// iletildiği en fazla eş sayısıdır — TTL ile birlikte bir sorgunun ağda
	// yol açabileceği toplam akış sayısını sınırlar (yükseltme saldırısına
	// karşı).
	maxIletimHedefi = 8
)

// gelenSorguSemafor, aynı anda işlenen gelen sorguları sayar (bkz.
// maxEszamanliGelenSorgu).
var gelenSorguSemafor = make(chan struct{}, maxEszamanliGelenSorgu)

// aramaEsleri, bağlı eşlerden "ağda ara" protokolünü konuşanları döner
// (haric hariç). Bağlı eşlerin çoğu genel DHT düğümüdür; onlara akış açmak
// her seferinde "protokol desteklenmiyor" ile biterdi. Protokolleri henüz
// bilinmeyen (identify sürüyor) eşler, bilinmeyenleriDahilEt ise listeye
// girer — kullanıcının kendi aramasında yeni bağlanmış bir eş kaçmasın diye;
// başka bir eşin sorgusunu iletirken girmez.
func aramaEsleri(h libp2phost.Host, haric peer.ID, bilinmeyenleriDahilEt bool) []peer.ID {
	ps := h.Peerstore()
	var out []peer.ID
	for _, p := range h.Network().Peers() {
		if p == haric {
			continue
		}
		protos, err := ps.GetProtocols(p)
		switch {
		case err != nil || len(protos) == 0:
			if bilinmeyenleriDahilEt {
				out = append(out, p)
			}
		case slices.Contains(protos, protocol.ID(aramaProtokolu)):
			out = append(out, p)
		}
	}
	return out
}

// istegiDenetle, ağdaki güvenilmeyen bir eşten gelen isteği bu düğümün
// sınırlarına çeker: sorgu metni kısaltılır, TTL ve bitiş zamanı bu düğümün
// üst sınırlarına çekilir. Yazar anahtarı ya da içerik türü geçersizse istek
// reddedilir (false) — boşaltmak sorguyu genişletir, yükü artırırdı.
func istegiDenetle(istek aramaIstek, simdi time.Time) (aramaIstek, bool) {
	if istek.YazarKey != "" {
		if _, err := hex.DecodeString(istek.YazarKey); err != nil || len(istek.YazarKey) != 64 {
			return istek, false
		}
	}
	switch SonucTip(istek.Tip) {
	case "", TipKonu, TipYanit, TipSohbet, TipSite:
	default:
		return istek, false
	}
	if r := []rune(istek.Metin); len(r) > maxSorguRune {
		istek.Metin = string(r[:maxSorguRune])
	}
	if istek.SorguID == "" || len(istek.SorguID) > 64 {
		istek.SorguID = yeniSorguID()
	}
	if istek.TTL <= 0 {
		istek.TTL = 1
	} else if istek.TTL > aramaVarsayilanTTL {
		istek.TTL = aramaVarsayilanTTL
	}
	if enGec := simdi.Add(aramaZamanAsimi); istek.SonZaman.IsZero() || istek.SonZaman.After(enGec) {
		istek.SonZaman = enGec
	}
	return istek, true
}

const (
	hizSiniriPenceresi = time.Minute
	// İletilen (relay) istekler de bu sayaca dahil olduğundan (iki eş
	// arasındaki normal bir stream'den ayırt edilemezler), doğrudan
	// sorgulardan daha yüksek tutulur — bkz. dosyanın başındaki savunma
	// katmanları notu.
	hizSiniriMax = 60
)

// aramaVarsayilanTTL, bir sorgunun kaç atlama öteye kadar yayılacağını
// belirler — hem istemcinin başlangıç değeri hem de her düğümün,
// istemcinin bildirdiği değerden bağımsız olarak uyguladığı ÜST sınırdır
// (bkz. dosyanın başındaki doc yorumu). const DEĞİL: main.go'nun -ttl
// bayrağıyla (aramaTTLMin..aramaTTLMax sınırları içinde) ezilebilir —
// programın GERİ KALANI (host/ağ kurulumu) başlamadan, main() içinde tek
// seferlik atandığı için sonradan başka bir goroutine ile yarışmaz.
var aramaVarsayilanTTL = 3

const (
	aramaTTLMin = 1
	aramaTTLMax = 5
)

// ttlDenetle, -ttl bayrağının değerini doğrular.
func ttlDenetle(n int) error {
	if n < aramaTTLMin || n > aramaTTLMax {
		return fmt.Errorf("-ttl %d..%d arasında olmalı", aramaTTLMin, aramaTTLMax)
	}
	return nil
}

const (

	// aramaIletMarji, bir düğümün "sorguyu ilet, yanıtları topla, KENDİ
	// yanıtımı geri gönder" için kendine bıraktığı süre payıdır — kalan
	// bütçenin tamamını iletime harcarsa kendi yanıtını göndermeye vakit
	// kalmaz.
	aramaIletMarji = 500 * time.Millisecond
	// aramaSorguOnbellekSuresi, bir SorguID'nin "daha önce görüldü"
	// olarak hatırlandığı süredir — sorgunun kendi zaman aşımından
	// (aramaZamanAsimi) belirgin şekilde uzun olmalı ki geç gelen bir
	// tekrar hâlâ doğru şekilde süzülsün.
	aramaSorguOnbellekSuresi = 2 * time.Minute
)

type aramaIstek struct {
	Metin string `json:"metin"`
	// Tip boşsa tüm türler aranır; doluysa SonucTip değerlerinden biri
	// ("konu"/"yanit"/"sohbet") olmalıdır — bkz. index.go'nun
	// AraFiltreli'si.
	Tip string `json:"tip,omitempty"`
	// YazarKey boşsa tüm yazarlar aranır; doluysa yalnızca o yazar
	// anahtarına ait sonuçlar döner — bkz. index.go'nun AraGelismis'i.
	YazarKey string `json:"yazar_key,omitempty"`
	// SorguID, ağ genelinde aynı kullanıcı araması için PAYLAŞILAN
	// rastgele bir tanımlayıcıdır — çok atlamalı yayılmada döngü/tekrar
	// önleme için kullanılır (bkz. sorguOnbellek).
	SorguID string `json:"sorgu_id"`
	// TTL, sorgunun kaç atlama daha öteye iletilebileceğidir; 1 = yalnızca
	// bu düğüm yanıtlasın, artık ileri iletme.
	TTL int `json:"ttl"`
	// SonZaman, sorgunun tüm ağ genelinde geçerli olduğu MUTLAK bitiş
	// zamanıdır — her atlama kendi payını bundan hesaplar (bkz.
	// iletVeTopla), göreli bir süre değil ki atlama sayısı arttıkça
	// toplam gecikme katlanarak büyümesin.
	SonZaman time.Time `json:"son_zaman"`
}

type agSonucu struct {
	Tip         SonucTip  `json:"tip"`
	KaynakID    string    `json:"kaynak_id"`
	UstID       string    `json:"ust_id,omitempty"`
	Kategori    string    `json:"kategori,omitempty"`
	Baslik      string    `json:"baslik,omitempty"`
	Govde       string    `json:"govde"`
	YazarAd     string    `json:"yazar_ad"`
	YazarKey    string    `json:"yazar_key,omitempty"`
	OlusturmaTS time.Time `json:"olusturma_ts"`
	// KaynakPeer, bu sonucu KENDİ yerel indeksinde bulan düğümün peer
	// ID'sidir — sonucu üreten düğüm tarafından etiketlenir ve ondan
	// sonraki her ara-düğüm tarafından DEĞİŞTİRİLMEDEN taşınır; böylece
	// çok atlamalı bir yanıtta bile sonucun GERÇEK kaynağı (yalnızca en
	// yakın atlama değil) bilinir.
	//
	// ÖNEMLİ SINIR: bu alan, taşıyan ara-düğümler tarafından İSTENİRSE
	// değiştirilebilir (imzalı değildir) — yalnızca bir görüntüleme
	// ipucudur, güvenlik kararı için KULLANILMAMALIDIR. Gerçek güvenlik
	// sınırı Sig/ImzaGovdesi'nin aşağıda doğrulanmasıdır: içerik kimin
	// yazarı olduğunu iddia ediyorsa (YazarKey) o yazarın GERÇEKTEN
	// imzaladığını kanıtlar; hangi düğümden geçtiğini değil.
	KaynakPeer string `json:"kaynak_peer,omitempty"`

	// Sig, AramaSonucu.Sig'in bire bir karşılığıdır — bkz. oradaki doc
	// yorumu. Boşsa (ya da dogrulaAgSonucu'yu geçmezse) bu tip GÜVENİLMEYEN
	// bir eşten gelen serbest biçimli veridir; HİÇBİR ÇAĞRI SİTESİ
	// doğrulamadan bu sonucu kullanmamalıdır.
	Sig string `json:"sig,omitempty"`
}

// dogrulaAgSonucu, GÜVENİLMEYEN bir eşten (doğrudan yanıt veren ya da
// yalnızca ileten) gelen bir agSonucu'nun içeriğinin gerçekten iddia ettiği
// YazarKey tarafından imzalandığını doğrular.
//
// Bu, arama sonuçlarının ağdaki bir düğüm tarafından tamamen UYDURULUP (sahte
// başlık/gövde/yazar ile) ya da GERÇEK bir imzalı içeriğin Sig'i korunup
// Baslik/Govde'si değiştirilerek (bir yazarın adına sahte içerik enjekte
// etme) meşru görünümlü bir sonuç olarak sunulmasını engelleyen TEK
// savunmadır. Bunun için s'nin KENDİ alanlarından (KaynakID/UstID/Kategori/
// Baslik/Govde/YazarKey/OlusturmaTS) — Sig HARİÇ hepsi doğrulanana kadar
// güvenilmeyen veri — verifyPost/verifyReply/verifyChat/verifySitePage'in
// beklediği TAM olarak aynı struct'ı yeniden kurar ve AYNI (postPayload/
// replyPayload/chatPayload/sitePagePayload) fonksiyonlarla AYNI imzalı
// payload'ı üretip üretmediğini bu fonksiyonlar aracılığıyla doğrular —
// Baslik/Govde/YazarKey/OlusturmaTS'in HERHANGİ biri değişirse imza artık
// tutmaz.
//
// İki ince nokta yeniden kurmanın orijinal imzalı baytlarla BİREBİR
// eşleşmesini sağlar:
//   - Govde, s.Govde ÜZERİNDE değil duzMetin(s.Govde) üzerinde kurulur —
//     AraGelismis'in yerel sonuçları highlight() ile SnippetOncesi/
//     SnippetSonrasi işaretleyicileriyle döndürdüğü ve bu HALİYLE agSonucu'na
//     kopyalandığı unutulmamalı (bkz. aramaSunucusuKaydet); duzMetin bu
//     işaretleyicileri kaldırıp diğer her baytı OLDUĞU GİBİ bıraktığından,
//     eşleşen bir terim olsun ya da olmasın orijinal gövdeyi bire bir geri
//     verir.
//   - OlusturmaTS, index.go'nun depoAc'ta göç ettirdiği sig sütunuyla
//     BİRLİKTE eklenen nanosaniye hassasiyetli saklama biçimiyle (bkz.
//     migrateAddSigColumns) tam olarak korunur — AND'ın kendisi CreatedAt'i
//     her zaman time.Now().UTC() ile ürettiğinden (bkz. internal/forum/
//     forum.go), saniye hassasiyetine ya da farklı bir saat dilimine
//     yuvarlama YAŞANMAZ; JSON'un time.Time kodlaması da (agSonucu.
//     OlusturmaTS) zaten nanosaniye hassasiyetlidir.
func dogrulaAgSonucu(s agSonucu) bool {
	if s.Sig == "" || s.YazarKey == "" || s.KaynakID == "" {
		return false
	}
	govde := duzMetin(s.Govde)
	switch s.Tip {
	case TipKonu:
		p := &Post{
			ID: s.KaynakID, Category: s.Kategori, AuthorKey: s.YazarKey,
			Title: s.Baslik, Body: govde, CreatedAt: s.OlusturmaTS, Sig: s.Sig,
		}
		return verifyPost(p)
	case TipYanit:
		r := &Reply{
			ID: s.KaynakID, PostID: s.UstID, AuthorKey: s.YazarKey,
			Body: govde, CreatedAt: s.OlusturmaTS, Sig: s.Sig,
		}
		return verifyReply(r)
	case TipSohbet:
		m := &ChatMessage{
			ID: s.KaynakID, AuthorKey: s.YazarKey, Text: govde, SentAt: s.OlusturmaTS, Sig: s.Sig,
		}
		return verifyChat(m)
	case TipSite:
		_, slug, ok := siteKaynakIDCoz(s.KaynakID)
		if !ok {
			return false
		}
		p := &SitePage{
			AuthorKey: s.YazarKey, Slug: slug, Title: s.Baslik, Body: govde, UpdatedAt: s.OlusturmaTS, Sig: s.Sig,
		}
		return verifySitePage(p)
	default:
		return false
	}
}

type aramaYanit struct {
	Sonuclar []agSonucu `json:"sonuclar"`
}

// yeniSorguID, çok atlamalı yayılmada döngü/tekrar önleme için kullanılan
// rastgele bir tanımlayıcı üretir — güvenlik amaçlı değildir (imza değil,
// yalnızca bir de-duplication anahtarı), bu yüzden crypto/rand yalnızca
// pratik bir rastgelelik kaynağı olarak kullanılıyor.
func yeniSorguID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// yeniAramaIstegi, YENİ bir kullanıcı araması için paylaşılacak isteği
// üretir. Aynı kullanıcı araması BİRDEN FAZLA doğrudan eşe gönderilecekse
// (bkz. model.go'nun agdaAraCmd'si), bu fonksiyon BİR KEZ çağrılmalı ve
// dönen istek her hedefe (aramaIstekGonderRaw ile) AYNEN gönderilmelidir
// — her hedef için ayrı bir istek (dolayısıyla ayrı bir SorguID) üretmek,
// ağın derinlerinde aynı aramanın gereksiz yere birden çok kez yayılıp
// işlenmesine yol açar.
func yeniAramaIstegi(metin string, tip SonucTip, yazarKey string) aramaIstek {
	return aramaIstek{
		Metin:    metin,
		Tip:      string(tip),
		YazarKey: yazarKey,
		SorguID:  yeniSorguID(),
		TTL:      aramaVarsayilanTTL,
		SonZaman: time.Now().Add(aramaZamanAsimi),
	}
}

// sorguOnbellek, bir SorguID'nin bu düğümde daha önce işlenip
// iletilmediğini takip eder — mesh topolojide bir sorgu birden fazla
// yoldan aynı düğüme ulaşabilir; ikinci ve sonraki gelişlerinde düğüm
// yalnızca kendi yerel sonuçlarıyla yanıt verir, bir daha İLETMEZ (aksi
// halde sorgu sayısı atlama başına katlanarak artardı).
type sorguOnbellek struct {
	mu      sync.Mutex
	gorulen map[string]time.Time
}

func yeniSorguOnbellek() *sorguOnbellek { return &sorguOnbellek{gorulen: map[string]time.Time{}} }

// gorulduMu, id daha önce görülmüşse true döner; görülmemişse onu
// "şimdi görüldü" olarak işaretleyip false döner. Çağrısı sırasında,
// süresi geçmiş eski girdileri de fırsatçı olarak temizler — ayrı bir
// arka plan goroutine'ine gerek kalmadan haritanın sınırsız büyümesini
// önler (sorgu hacmi zaten hizSiniri ile sınırlı olduğundan bu taraması
// ucuzdur).
func (c *sorguOnbellek) gorulduMu(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, t := range c.gorulen {
		if now.Sub(t) > aramaSorguOnbellekSuresi {
			delete(c.gorulen, k)
		}
	}
	if _, ok := c.gorulen[id]; ok {
		return true
	}
	c.gorulen[id] = now
	return false
}

// hizSiniri, bir eşten (libp2p'nin doğruladığı peer ID'sine göre) gelen
// sorgu isteklerini sınırlar — sürekli sorgu göndererek bu düğümü meşgul
// etmeye çalışan bir eşe karşı basit bir savunma.
type hizSiniri struct {
	mu     sync.Mutex
	gecmis map[peer.ID][]time.Time
}

func yeniHizSiniri() *hizSiniri { return &hizSiniri{gecmis: map[peer.ID][]time.Time{}} }

func (h *hizSiniri) izinVer(p peer.ID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-hizSiniriPenceresi)
	eski := h.gecmis[p]
	temiz := eski[:0]
	for _, t := range eski {
		if t.After(cutoff) {
			temiz = append(temiz, t)
		}
	}
	if len(temiz) >= hizSiniriMax {
		h.gecmis[p] = temiz
		return false
	}
	h.gecmis[p] = append(temiz, now)
	return true
}

// agSonuclariniTekillestir, kaynak_id'ye göre tekrarları eler (aynı
// içerik hem kendi yerel aramamızda hem de iletilen bir yanıtta —ya da
// birden fazla iletilen yanıtta— görünebilir) ve en fazla limit tanesini
// döner.
func agSonuclariniTekillestir(sonuclar []agSonucu, limit int) []agSonucu {
	out := make([]agSonucu, 0, len(sonuclar))
	gorulen := map[string]bool{}
	for _, s := range sonuclar {
		if gorulen[s.KaynakID] {
			continue
		}
		gorulen[s.KaynakID] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// aramaSunucusuKaydet, gelen arama isteklerini yerel depoda arayıp
// yanıtlayan VE (TTL izin veriyorsa) sorguyu diğer doğrudan eşlere
// ileterek onların yanıtlarını da toplayan stream handler'ı host'a
// bağlar. ctx, iletim için açılan alt-isteklerin düğüm kapanışında
// (ctx iptal edildiğinde) hemen sonlanmasını sağlar.
func aramaSunucusuKaydet(ctx context.Context, h libp2phost.Host, depo *Depo, hs *hizSiniri, sc *sorguOnbellek) {
	h.SetStreamHandler(aramaProtokolu, func(st libp2pnet.Stream) {
		// libp2p bu handler'ı kendi goroutine'inde çalıştırır — burada
		// yakalanmayan bir panik TÜM PROGRAMI çökertir. İstek, ağdaki
		// GÜVENİLMEYEN bir eşten gelen serbest biçimli JSON'dur; bu
		// yüzden ayrıştırma/işleme mantığındaki beklenmeyen bir kenar
		// durumuna karşı bu kalkan şart.
		defer recoverGoroutine("arama-sunucusu")
		defer st.Close()
		st.SetDeadline(time.Now().Add(aramaZamanAsimi)) //nolint:errcheck

		gonderen := st.Conn().RemotePeer()
		if !hs.izinVer(gonderen) {
			gunlukUyari("eş hız sınırına takıldı, sorgu yanıtlanmadı:", gonderen)
			return
		}
		select {
		case gelenSorguSemafor <- struct{}{}:
			defer func() { <-gelenSorguSemafor }()
		default:
			gunlukUyari("aynı anda çok fazla gelen sorgu, yanıtlanmadı:", gonderen)
			return
		}

		var istek aramaIstek
		if err := json.NewDecoder(io.LimitReader(st, maxAramaIstekBayt)).Decode(&istek); err != nil {
			return
		}
		istek, gecerli := istegiDenetle(istek, time.Now())
		if !gecerli {
			gunlukUyari("geçersiz arama isteği reddedildi:", gonderen)
			return
		}

		yerelSayfa, err := depo.AraGelismis(istek.Metin, SonucTip(istek.Tip), istek.YazarKey, maxAgSonucSayisi, 0)
		if err != nil {
			gunlukHata("gelen sorgu aranamadı:", gonderen, err)
			return
		}
		yerel := yerelSayfa.Sonuclar
		toplam := make([]agSonucu, 0, len(yerel))
		for _, s := range yerel {
			toplam = append(toplam, agSonucu{
				Tip: s.Tip, KaynakID: s.KaynakID, UstID: s.UstID, Kategori: s.Kategori,
				Baslik: s.Baslik, Govde: s.Govde, YazarAd: s.YazarAd, YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS,
				KaynakPeer: h.ID().String(),
				Sig:        s.Sig,
			})
		}

		if istek.TTL > 1 && !sc.gorulduMu(istek.SorguID) {
			kalan := time.Until(istek.SonZaman) - aramaIletMarji
			if kalan > time.Second {
				toplam = append(toplam, iletVeTopla(ctx, h, gonderen, istek, kalan)...)
			}
		}

		_ = json.NewEncoder(st).Encode(aramaYanit{Sonuclar: agSonuclariniTekillestir(toplam, maxAgSonucSayisi)})
	})
}

// iletVeTopla, istek'i (TTL bir azaltılmış olarak) haric dışındaki tüm
// doğrudan eşlere eşzamanlı olarak iletir ve yanıtlarını birleştirip
// döner. butce, bu işlem için ayrılan (SonZaman'dan hesaplanmış) süredir.
func iletVeTopla(ctx context.Context, h libp2phost.Host, haric peer.ID, istek aramaIstek, butce time.Duration) []agSonucu {
	hedefler := aramaEsleri(h, haric, false)
	if len(hedefler) == 0 {
		return nil
	}
	if len(hedefler) > maxIletimHedefi {
		mrand.Shuffle(len(hedefler), func(i, j int) { hedefler[i], hedefler[j] = hedefler[j], hedefler[i] })
		hedefler = hedefler[:maxIletimHedefi]
	}

	fctx, cancel := context.WithTimeout(ctx, butce)
	defer cancel()

	ileriIstek := istek
	ileriIstek.TTL--

	var mu sync.Mutex
	var toplam []agSonucu
	var wg sync.WaitGroup
	for _, p := range hedefler {
		wg.Add(1)
		go func(p peer.ID) {
			defer wg.Done()
			defer recoverGoroutine("arama-iletim")
			sonuclar, err := aramaIstekGonderRaw(fctx, h, p, ileriIstek)
			if err != nil {
				return
			}
			mu.Lock()
			toplam = append(toplam, sonuclar...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return toplam
}

// aramaIstekleGonder, sorguyu TEK bir eşe, YENİ (paylaşılmamış) bir
// SorguID ile gönderir ve yanıtını döner. Aynı kullanıcı aramasını
// birden fazla hedefe göndermek için bunun yerine yeniAramaIstegi +
// aramaIstekGonderRaw kullanılmalı — bkz. yeniAramaIstegi'nin doc yorumu.
func aramaIstekleGonder(ctx context.Context, h libp2phost.Host, hedef peer.ID, metin string, tip SonucTip, yazarKey string) ([]agSonucu, error) {
	return aramaIstekGonderRaw(ctx, h, hedef, yeniAramaIstegi(metin, tip, yazarKey))
}

// aramaIstekGonderRaw, önceden oluşturulmuş bir istek'i hedef'e gönderir.
func aramaIstekGonderRaw(ctx context.Context, h libp2phost.Host, hedef peer.ID, istek aramaIstek) ([]agSonucu, error) {
	ctx, cancel := context.WithTimeout(ctx, aramaZamanAsimi)
	defer cancel()

	st, err := h.NewStream(ctx, hedef, aramaProtokolu)
	if err != nil {
		return nil, fmt.Errorf("arama isteği: stream açılamadı: %w", err)
	}
	defer st.Close()
	if dl, ok := ctx.Deadline(); ok {
		st.SetDeadline(dl) //nolint:errcheck
	}

	if err := json.NewEncoder(st).Encode(istek); err != nil {
		return nil, fmt.Errorf("arama isteği gönderilemedi: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var yanit aramaYanit
	if err := json.NewDecoder(io.LimitReader(st, maxAramaYanitBayt)).Decode(&yanit); err != nil {
		return nil, fmt.Errorf("arama yanıtı okunamadı: %w", err)
	}

	// hedef GÜVENİLMEYEN bir eştir (doğrudan yanıtlıyor OLABİLİR ya da
	// yalnızca kendi eşlerinden topladığı yanıtları ileten kötü niyetli bir
	// ara-düğüm de olabilir) — bu yüzden her sonucun imzası, aggregate
	// edilmeden ya da UI'ye ulaşmadan ÖNCE, tam olarak burada (tüm çağıranlar
	// için TEK bir boğaz noktasında: hem üst düzey kullanıcı sorguları hem
	// de iletVeTopla'nın iç içe geçmiş relay çağrıları buradan geçer)
	// doğrulanır. Doğrulamayı geçemeyen bir sonuç sessizce elenir.
	gecerli, gecersiz := 0, 0
	dogrulanmis := yanit.Sonuclar[:0]
	for _, s := range yanit.Sonuclar {
		if dogrulaAgSonucu(s) {
			dogrulanmis = append(dogrulanmis, s)
			gecerli++
		} else {
			gecersiz++
		}
	}
	// esGuvenDefteriKaydet, bu düğümün DOĞRUDAN sorguladığı hedef'in ne
	// sıklıkla geçerli/geçersiz imzalı sonuç döndürdüğünü izler — bkz.
	// esGuvenDefteri'nin doc yorumu (Durum ekranının "eş güven göstergesi").
	esGuvenDefteriVarsayilan.kaydet(hedef, gecerli, gecersiz)
	return dogrulanmis, nil
}

// ─── Hedefli P2P sorgu ───────────────────────────────────────────────────────
//
// agdaAraCmd (bkz. model.go) sorguyu TÜM bağlı eşlere birden yayar
// (flood & aggregate). Bazen kullanıcı belirli BİR eşin — ör. bir ağ
// sonucunun KaynakPeer'ının — indeksini DOĞRUDAN kontrol etmek ister
// ("bu eşte başka ne var, güncellendi mi") — aramaTekEsIstegi bunu TTL=1
// (yalnızca o eş yanıtlasın, ileri iletme yok) ile tek bir hedefe gönderir.
// Ayrı bir protokol GEREKTİRMEZ: mevcut aramaProtokolu/aramaIstekGonderRaw
// aynen kullanılır, yalnızca hedef kümesi (tüm eşler yerine tek bir eş) ve
// TTL farklıdır.
func aramaTekEsIstegi(ctx context.Context, h libp2phost.Host, hedefStr string, sorgu string, tip SonucTip, yazarKey string) ([]AramaSonucu, error) {
	hedef, err := peer.Decode(hedefStr)
	if err != nil {
		return nil, fmt.Errorf("geçersiz eş kimliği: %w", err)
	}
	istek := yeniAramaIstegi(sorgu, tip, yazarKey)
	istek.TTL = 1
	sonuclarAg, err := aramaIstekGonderRaw(ctx, h, hedef, istek)
	if err != nil {
		return nil, err
	}
	out := make([]AramaSonucu, 0, len(sonuclarAg))
	for _, s := range sonuclarAg {
		out = append(out, AramaSonucu{
			Tip: s.Tip, KaynakID: s.KaynakID, UstID: s.UstID, Kategori: s.Kategori,
			Baslik: s.Baslik, Govde: s.Govde, YazarAd: s.YazarAd, YazarKey: s.YazarKey, OlusturmaTS: s.OlusturmaTS,
			KaynakPeer: hedefStr,
		})
	}
	return out, nil
}

// ─── Eş güven göstergesi ─────────────────────────────────────────────────────
//
// "Basit bir gösterge" — ağdaki hangi eşlerin (bu düğümün DOĞRUDAN
// sorguladığı, yani yukarıdaki tek boğaz noktasından geçen) tutarlı şekilde
// geçerli imzalı sonuç döndürdüğünü, hangilerinin sahte/kurcalanmış içerik
// denediğini izler. Bu bir ENGELLEME mekanizması DEĞİLDİR (dogrulaAgSonucu
// zaten her sonucu bağımsız olarak doğruluyor ve geçersizleri eliyor —
// güvenlik ORADA sağlanır) — yalnızca kullanıcının Durum ekranında "hangi
// eşler şüpheli davranıyor" diye GÖRSEL bir ipucu edinmesi içindir.
//
// Süreç ömürlü, BELLEK-İÇİ bir defterdir (hizSiniri/sorguOnbellek ile AYNI
// desen: bir struct + kendi mutex'i, testler kendi örneğini oluşturabilir)
// — diske YAZILMAZ, her yeniden başlatmada sıfırlanır; bu kasıtlıdır,
// çünkü eş kimlikleri (peer.ID) kalıcı bir itibar sistemi kurmak için
// yeterince kararlı bir kimlik DEĞİLDİR (bir düğüm kimliğini değiştirip
// yeniden katılabilir) — bkz. identity.go'nun BIP39 kimlik modelinin peer
// kimliğinden AYRI olması.
type esGuvenDefteri struct {
	mu       sync.Mutex
	sayaclar map[peer.ID]*esGuvenSayaci
}

type esGuvenSayaci struct {
	Gecerli  int
	Gecersiz int
}

func yeniEsGuvenDefteri() *esGuvenDefteri {
	return &esGuvenDefteri{sayaclar: map[peer.ID]*esGuvenSayaci{}}
}

// esGuvenDefteriVarsayilan, aramaIstekGonderRaw'ın (bkz. yukarıdaki TEK
// boğaz noktası) kaydettiği süreç genelindeki varsayılan defterdir —
// hizSiniri/sorguOnbellek'in AKSİNE main()'de örneklenip elle taşınmaz,
// çünkü aramaIstekGonderRaw hem kullanıcı sorgularından (model.go) hem
// sunucu-tarafı relay'den (iletVeTopla) çağrılır ve ikisinin de AYNI
// gözlemi paylaşması istenir — bu tek amaç için ekstra bir parametre
// zincirlemek yerine tek bir paylaşılan örnek yeterlidir.
var esGuvenDefteriVarsayilan = yeniEsGuvenDefteri()

func (e *esGuvenDefteri) kaydet(p peer.ID, gecerli, gecersiz int) {
	if gecerli == 0 && gecersiz == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.sayaclar[p]
	if c == nil {
		c = &esGuvenSayaci{}
		e.sayaclar[p] = c
	}
	c.Gecerli += gecerli
	c.Gecersiz += gecersiz
}

// EsGuvenOzet, enIyiListele'nin bir satırıdır.
type EsGuvenOzet struct {
	Peer     peer.ID
	Gecerli  int
	Gecersiz int
}

// enIyiListele, en çok etkileşimde bulunulan (gecerli+gecersiz toplamı en
// yüksek) en fazla limit eşi döner — Durum ekranının "eş güven göstergesi"
// paneli için (bkz. model.go'nun durumIstatistikCmd'si).
func (e *esGuvenDefteri) enIyiListele(limit int) []EsGuvenOzet {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]EsGuvenOzet, 0, len(e.sayaclar))
	for p, c := range e.sayaclar {
		out = append(out, EsGuvenOzet{Peer: p, Gecerli: c.Gecerli, Gecersiz: c.Gecersiz})
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].Gecerli+out[i].Gecersiz, out[j].Gecerli+out[j].Gecersiz
		if ti != tj {
			return ti > tj
		}
		return out[i].Peer < out[j].Peer
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
