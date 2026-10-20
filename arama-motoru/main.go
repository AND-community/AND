// AND Arama Motoru — AND ekosistemindeki (P2P forum + sohbet) içeriği
// arayan, AND'dan TAMAMEN BAĞIMSIZ çalışan bir program. AND'ın kendisi
// açık olmasa bile, bu programın kendi kimliği ve kendi libp2p ağ
// bağlantısıyla AND ağına doğrudan bir eş (peer) olarak katılır — bkz.
// README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Version, -ldflags "-X main.Version=..." ile derleme zamanında
// ezilebilir (AND'ın kendi cmd/andmod'undaki aynı desen); ezilmezse bu
// varsayılan kullanılır. BuildCommit/BuildDate AYNI şekilde (ör. bir
// derleme/release betiğinden) doldurulabilir; boş bırakılırlarsa -version
// çıktısında hiç gösterilmezler (bkz. main()'in -version dalı) — bu proje
// bir git deposu İÇİNDE OLMAYABİLİR, bu yüzden derleme zamanında otomatik
// `git rev-parse` denemek yerine kasıtlı olarak İSTEĞE BAĞLI tutulur.
var (
	Version     = "v0.1.0"
	BuildCommit string
	BuildDate   string
)

const gunlukBoyutKontrolAraligi = 10 * time.Minute
const bilinenPeerKayitAraligi = 2 * time.Minute

func varsayilanVeriDizini(bayrakDegeri string) (string, error) {
	if bayrakDegeri != "" {
		return bayrakDegeri, nil
	}
	if v := os.Getenv("ARAMA_MOTORU_VERI_DIR"); v != "" {
		return v, nil
	}
	ev, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kullanıcı ev dizini bulunamadı: %w", err)
	}
	return filepath.Join(ev, ".and-arama-motoru"), nil
}

func olumculCik(gunlukMesaji string, err error) {
	fmt.Fprintln(os.Stderr, "AND Arama Motoru:", err)
	gunlukHata(gunlukMesaji+":", err)
	os.Exit(1)
}

// hataVeCik, olumculCik'in GÜNLÜK DOSYASI HENÜZ AÇILMAMIŞKEN (dataDir
// çözümü, -search/-site CLI dalları — bunlar gunlukAyarla'dan ÖNCE
// çalışır) kullanılan sadeleştirilmiş biçimidir: yalnızca stderr'e yazıp
// çıkar, gunlukHata'yı ÇAĞIRMAYA ÇALIŞMAZ (o an için anlamsız olurdu).
// Önceden bu tek satırlık kalıp (fmt.Fprintln + os.Exit(1)) altı ayrı
// yerde elle tekrarlanıyordu — tek bir yardımcıya toplanarak hem tutarlı
// bir mesaj biçimi garanti edilir hem de gelecekte biçim değişirse tek
// yerden güncellenir.
func hataVeCik(err error) {
	fmt.Fprintln(os.Stderr, "AND Arama Motoru:", err)
	os.Exit(1)
}

// bayrakHatasiCik, KULLANICI HATASI (geçersiz bayrak değeri) için çıkar —
// os.Exit(2), Unix kabuk araçlarında "hatalı kullanım" için yaygın
// kuraldır (bkz. örn. grep/git); olumculCik'in kullandığı 1'den (çalışma
// zamanı hatası) kasıtlı olarak farklıdır — bir betik ikisini ayırt
// edebilsin diye.
func bayrakHatasiCik(err error) {
	fmt.Fprintln(os.Stderr, "AND Arama Motoru:", err)
	os.Exit(2)
}

// recoverGoroutine, bir arka plan goroutine'inin en başında defer
// edilmek üzere tasarlanmıştır: içerik (forum/sohbet gossip'i, sync
// yanıtları) AĞDAKİ GÜVENİLMEYEN eşlerden geldiğinden, ayrıştırma/işleme
// mantığındaki beklenmeyen bir kenar durumu panikleyebilir — bunu
// yakalamadan bırakmak tek bir kötü mesajın TÜM programı çökertmesi
// anlamına gelirdi. isim, hangi goroutine olduğunu günlükte ayırt etmek
// içindir.
func recoverGoroutine(isim string) {
	if r := recover(); r != nil {
		gunlukHata(fmt.Sprintf("panik yakalandı (%s), goroutine sonlandırıldı:", isim), r)
	}
}

func main() {
	bayrakSurum := flag.Bool("version", false, "sürüm bilgisini yazdır ve çık")
	bayrakVeriDir := flag.String("veri-dir", "", "veri dizini (varsayılan: $ARAMA_MOTORU_VERI_DIR ya da ~/.and-arama-motoru)")
	bayrakLogSeviye := flag.String("log-level", "bilgi", "günlük ayrıntı düzeyi: hata|uyari|bilgi")
	bayrakTTL := flag.Int("ttl", aramaVarsayilanTTL, fmt.Sprintf("ağda arama için varsayılan atlama (TTL) sayısı (%d-%d)", aramaTTLMin, aramaTTLMax))
	bayrakArama := flag.String("search", "", "yerel indekste ara, sonuçları yazdır ve çık (TUI/ağ AÇILMAZ — betiklenebilir hızlı arama)")
	bayrakTip := flag.String("tip", "", "-search ile birlikte: sonuçları türe göre sınırla (konu|yanit|sohbet|site)")
	bayrakYazar := flag.String("yazar", "", "yalnızca bu yazar anahtarına (hex) ait sonuçlar; -search boşsa bile tek başına o yazarın tüm içeriğini listeler")
	bayrakLimit := flag.Int("limit", 20, "-search ile birlikte: en fazla kaç sonuç")
	bayrakSayfa := flag.Int("sayfa", 1, "-search ile birlikte: sayfa numarası (1'den başlar)")
	bayrakSite := flag.String("site", "", "bir site paylaşım kodunu (yazarAnahtarı/slug) yerel indekste ara, sayfayı yazdır ve çık (TUI/ağ AÇILMAZ)")
	bayrakJSON := flag.Bool("json", false, "-search/-site ile birlikte: sonucu JSON olarak yazdır")
	bayrakProtokolKaydet := flag.Bool("protokol-kaydet", false, "and-site:// bağlantılarını bu programla açacak şekilde işletim sistemine kaydet (yalnızca Windows) ve çık")
	bayrakProtokolKaldir := flag.Bool("protokol-kaldir", false, "and-site:// bağlantı kaydını kaldır (yalnızca Windows) ve çık")

	// flag.Usage, PrintDefaults'un düz/alfabetik tek listesi yerine
	// seçenekleri ANLAMLI kategorilere ayırır (Genel/Arama/Site paylaşımı/
	// Ağ) — gerçek CLI araçlarının (git, docker, kubectl) yardım çıktısına
	// benzer. Her satır flag.Lookup ile TEK kaynaktan (yukarıdaki flag.X
	// çağrılarının kendi açıklamalarından) okunur — açıklama iki yerde
	// tekrarlanıp birbirinden SAPMA riski taşımaz.
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "AND Arama Motoru %s — AND ekosistemini (forum + sohbet + kişisel site) yerelde ve ağda arayan bağımsız bir P2P eşi.\n\n", Version)
		fmt.Fprintf(out, "Kullanım:\n  %s [seçenekler]\n\n", os.Args[0])

		yazdirGrup := func(baslik string, isimler ...string) {
			fmt.Fprintln(out, baslik)
			for _, isim := range isimler {
				f := flag.Lookup(isim)
				if f == nil {
					continue
				}
				varsayilan := ""
				if f.DefValue != "" && f.DefValue != "false" {
					varsayilan = fmt.Sprintf(" (varsayılan: %s)", f.DefValue)
				}
				fmt.Fprintf(out, "  -%-10s %s%s\n", f.Name, f.Usage, varsayilan)
			}
			fmt.Fprintln(out)
		}
		yazdirGrup("Genel:", "version", "veri-dir", "log-level")
		yazdirGrup("Arama (-search ile, TUI/ağ AÇILMAZ):", "search", "tip", "yazar", "limit", "sayfa", "json")
		yazdirGrup("Site paylaşımı (-site ile, TUI/ağ AÇILMAZ):", "site", "protokol-kaydet", "protokol-kaldir")
		yazdirGrup("Ağ:", "ttl")
	}
	flag.Parse()

	if *bayrakSurum {
		fmt.Println("AND Arama Motoru", Version)
		if BuildCommit != "" {
			fmt.Println("  commit: ", BuildCommit)
		}
		if BuildDate != "" {
			fmt.Println("  derleme:", BuildDate)
		}
		fmt.Printf("  go:      %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return
	}

	// -protokol-kaydet/-protokol-kaldir: and-site:// bağlantılarının
	// işletim sistemi tarafından bu programla açılıp açılmayacağını
	// belirler — bkz. protokol_windows.go'nun doc yorumu. Veri
	// dizini/günlük hiç gerekmediğinden en erken noktada, diğer her
	// şeyden önce işlenir.
	if *bayrakProtokolKaydet {
		exeYolu, err := protokolKaydet()
		if err != nil {
			hataVeCik(err)
		}
		fmt.Printf("and-site:// bağlantıları artık şu programla açılacak: %s\n", exeYolu)
		return
	}
	if *bayrakProtokolKaldir {
		if err := protokolKaldir(); err != nil {
			hataVeCik(err)
		}
		fmt.Println("and-site:// bağlantı kaydı kaldırıldı.")
		return
	}

	// İşletim sistemi, kayıtlı and-site:// bağlantısına tıklandığında bu
	// programı URI'yi TEK argüman olarak vererek çalıştırır (bkz.
	// protokolKaydet'in yazdığı "shell\open\command"). Bu, -site
	// bayrağıyla AYNI yerel sorguyu (TUI/ağ AÇILMAZ) çalıştırır — kod
	// yalnızca elde ediliş biçimi (bayrak vs. tıklanan link) farklıdır.
	// resolveSiteKod, -site AÇIKÇA verilmişse pozisyonel argümanı hiç
	// dikkate almaz (bkz. onun doc yorumu) — bu yüzden -site "x" -y (URI)
	// gibi anlamsız bir kombinasyon sessizce URI'yi yok sayar, "x"i kullanır.
	*bayrakSite = resolveSiteKod(*bayrakSite, flag.Arg(0))

	if err := gunlukSeviyeAyarla(*bayrakLogSeviye); err != nil {
		bayrakHatasiCik(err)
	}
	if err := ttlDenetle(*bayrakTTL); err != nil {
		bayrakHatasiCik(err)
	}
	aramaVarsayilanTTL = *bayrakTTL

	dataDir, err := varsayilanVeriDizini(*bayrakVeriDir)
	if err != nil {
		hataVeCik(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		hataVeCik(fmt.Errorf("veri dizini oluşturulamadı: %w", err))
	}

	// -site: bir paylaşım koduyla (bkz. index.go'nun SiteSayfasiBul'u)
	// doğrudan TEK bir site sayfası arayıp yazdır ve çık — TUI/ağ AÇILMAZ,
	// -search'ten AYRI bir dal (farklı bir sorgu şekli: metin arama değil,
	// tam kod eşleşmesi).
	if *bayrakSite != "" {
		depo, err := depoAc(dataDir)
		if err != nil {
			hataVeCik(err)
		}
		defer depo.Kapat() //nolint:errcheck
		if err := siteCLICalistir(depo, *bayrakSite, *bayrakJSON); err != nil {
			hataVeCik(err)
		}
		return
	}

	// -search (ya da tek başına -yazar): TUI'yi ve ağı hiç başlatmadan tek
	// seferlik, hızlı bir yerel arama yapıp çık — bkz. cli.go'nun doc
	// yorumu.
	if *bayrakArama != "" || *bayrakYazar != "" {
		tip, err := tipDenetle(*bayrakTip)
		if err != nil {
			bayrakHatasiCik(err)
		}
		depo, err := depoAc(dataDir)
		if err != nil {
			hataVeCik(err)
		}
		defer depo.Kapat() //nolint:errcheck
		if err := aramaCLICalistir(depo, *bayrakArama, tip, *bayrakYazar, *bayrakLimit, *bayrakSayfa, *bayrakJSON); err != nil {
			hataVeCik(err)
		}
		return
	}

	gunluk, gunlukErr := gunlukAyarla(dataDir)
	if gunlukErr == nil {
		defer gunluk.Close() //nolint:errcheck
	} else {
		// Günlük dosyası açılamasa da kütüphane günlükleri terminale
		// yazılıp tam ekran arayüzü bozmasın.
		kutuphaneGunlukleriniYonlendir(io.Discard)
	}
	gunlukBilgi("başlatılıyor —", Version, "— veri dizini:", dataDir)

	// Tema: önce kendi tema.json'umuz, o yoksa AND'da seçili tema (bkz.
	// tema.go'nun etkinTema'sı). İkisi de yoksa TemaAyarlari{} sıfır değerli
	// döner ve temaUygula hiçbir col*'ı değiştirmez; program view.go'nun sabit
	// varsayılan (AND'ın "Klasik" teması) paletiyle çalışmaya devam eder.
	tema, temaKaynagi := etkinTema(dataDir, andTemaYolu())
	temaUygula(tema)
	if temaKaynagi != "" {
		gunlukBilgi("tema uygulandı:", tema.TemaAdi, "— kaynak:", temaKaynagi)
	}

	kimlik, yeniKimlik, err := kimlikYukleYaOlustur(dataDir)
	if err != nil {
		olumculCik("kimlik hatası", err)
	}
	gunlukBilgif("kimlik hazır (yeni=%v): %s", yeniKimlik, kimlik.PubKeyHex())

	depo, err := depoAc(dataDir)
	if err != nil {
		olumculCik("depo hatası", err)
	}

	h, err := dugumOlustur(kimlik)
	if err != nil {
		_ = depo.Kapat()
		olumculCik("libp2p host hatası", err)
	}
	gunlukBilgi("libp2p host hazır, peer ID:", h.ID())

	ctx, cancel := context.WithCancel(context.Background())

	// Arama sunucusu, HERHANGİ bir eşe bağlanmadan önce kaydedilir: eşler
	// protokol listemizi ilk tanışmada (identify) öğrenir ve "ağda ara"
	// sorgularını yalnızca bu protokolü konuşan eşlere yönlendirir (bkz.
	// search.go'nun aramaEsleri'si) — sonradan kaydedilse, o ana kadar
	// bağlanan eşler bizi kısa bir süre arama motoru olarak görmezdi.
	hs := yeniHizSiniri()
	sc := yeniSorguOnbellek()
	aramaSunucusuKaydet(ctx, h, depo, hs, sc)

	// Bir önceki çalıştırmadan bilinen eşlere doğrudan bağlanmayı dene —
	// DHT/mDNS keşfinden daha hızlı — bkz. peers.go'nun doc yorumu.
	bilinenler := bilinenPeerleriYukle(dataDir)
	gunlukBilgif("%d bilinen eş dosyadan yüklendi", len(bilinenler))
	for _, pi := range bilinenler {
		go func(pi peer.AddrInfo) {
			defer recoverGoroutine("bilinen-peer-baglan")
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_ = h.Connect(cctx, pi)
		}(pi)
	}

	kesif, err := kesifBaslat(ctx, h)
	if err != nil {
		cancel()
		_ = h.Close()
		_ = depo.Kapat()
		olumculCik("keşif hatası", err)
	}
	gunlukBilgi("keşif (DHT+mDNS) başlatıldı")

	ps, err := pubsubBaslat(ctx, h)
	if err != nil {
		cancel()
		_ = kesif.Kapat()
		_ = h.Close()
		_ = depo.Kapat()
		olumculCik("pubsub hatası", err)
	}
	forumKonu, err := konuyaKatil(ps, h, forumTopicAdi)
	if err != nil {
		cancel()
		_ = kesif.Kapat()
		_ = h.Close()
		_ = depo.Kapat()
		olumculCik("forum konusu hatası", err)
	}
	chatKonu, err := konuyaKatil(ps, h, chatTopicAdi)
	if err != nil {
		cancel()
		_ = kesif.Kapat()
		_ = h.Close()
		_ = depo.Kapat()
		olumculCik("sohbet konusu hatası", err)
	}
	siteKonu, err := konuyaKatil(ps, h, siteTopicAdi)
	if err != nil {
		cancel()
		_ = kesif.Kapat()
		_ = h.Close()
		_ = depo.Kapat()
		olumculCik("site konusu hatası", err)
	}

	// arkaPlan, aşağıdaki tüm sürekli-çalışan goroutine'leri izler — kapanış
	// sırasında (bkz. dosyanın sonundaki temizlik bloğu) ctx iptal edildikten
	// sonra hepsinin GERÇEKTEN durmasını beklemek için gerekli. Bunu izlemeden
	// yalnızca cancel() çağırıp hemen ağ kaynaklarını kapatmak — önceki
	// sürümün hatası — hâlâ çalışan bir goroutine'in kapalı bir host'u
	// kullanmaya çalışmasına, ya da (daha kötüsü) periyodik bilinen-eş
	// kaydının kapanışta hiç çalışma fırsatı bulamadan sürecin bitmesine
	// yol açabilirdi.
	var arkaPlan sync.WaitGroup

	arkaPlan.Add(1)
	go func() {
		defer arkaPlan.Done()
		defer recoverGoroutine("forum-gossip")
		for data := range forumKonu.Mesajlar(ctx) {
			forumMesajIsle(depo, data)
		}
	}()
	arkaPlan.Add(1)
	go func() {
		defer arkaPlan.Done()
		defer recoverGoroutine("chat-gossip")
		for data := range chatKonu.Mesajlar(ctx) {
			chatMesajIsle(depo, data)
		}
	}()
	arkaPlan.Add(1)
	go func() {
		defer arkaPlan.Done()
		defer recoverGoroutine("site-gossip")
		for data := range siteKonu.Mesajlar(ctx) {
			siteMesajIsle(depo, data)
		}
	}()

	// Yeni bağlanan her eşle bir kez senkronize ol — böylece yalnızca
	// bundan sonra gelecek canlı gossip'i değil, o eşin zaten bildiği
	// geçmişi de yakalarız. Tek tek senkronizasyon goroutine'leri (aşağıda,
	// pid başına) kasıtlı olarak arkaPlan'a dahil değil — kendi 30sn'lik
	// zaman aşımlarıyla sınırlılar ve ctx iptal edilince zaten hızla
	// sonlanırlar; hepsini beklemek kapanışı gereksiz yavaşlatırdı.
	arkaPlan.Add(1)
	go func() {
		defer arkaPlan.Done()
		defer recoverGoroutine("peer-sync-dongusu")
		senkronize := map[peer.ID]bool{}
		for {
			select {
			case pid := <-kesif.PeerBaglandiCh():
				if senkronize[pid] {
					continue
				}
				senkronize[pid] = true
				gunlukBilgi("yeni eş bağlandı:", pid)
				go func(pid peer.ID) {
					defer recoverGoroutine("peer-sync")
					select {
					case <-ctx.Done():
						return
					case <-time.After(1 * time.Second):
					}
					if err := forumIleSenkronizeOl(ctx, h, pid, depo); err != nil {
						gunlukUyari("forum sync başarısız", pid, ":", err)
					}
					if err := chatIleSenkronizeOl(ctx, h, pid, depo); err != nil {
						gunlukUyari("chat sync başarısız", pid, ":", err)
					}
					if err := siteIleSenkronizeOl(ctx, h, pid, depo); err != nil {
						gunlukUyari("site sync başarısız", pid, ":", err)
					}
				}(pid)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Bilinen eş listesini ve günlük dosyasının boyutunu periyodik olarak
	// tazele — uzun süre açık kalan bir işlemde ikisi de sınırsız
	// büyümemeli/eskimemeli.
	arkaPlan.Add(1)
	go func() {
		defer arkaPlan.Done()
		defer recoverGoroutine("periyodik-bakim")
		peerTicker := time.NewTicker(bilinenPeerKayitAraligi)
		defer peerTicker.Stop()
		gunlukTicker := time.NewTicker(gunlukBoyutKontrolAraligi)
		defer gunlukTicker.Stop()
		for {
			select {
			case <-peerTicker.C:
				if err := bilinenPeerleriKaydet(dataDir, h); err != nil {
					gunlukHata("bilinen eşler kaydedilemedi:", err)
				}
			case <-gunlukTicker.C:
				if gunluk != nil {
					gunluk.BoyutKontrolEt()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		gunlukBilgi("kapatma sinyali alındı")
		cancel()
	}()

	m := yeniModel(ctx, h, depo, kimlik)
	m.yeniKimlik = yeniKimlik
	m.temaAdi = tema.TemaAdi
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, runErr := p.Run()

	// Temiz kapanış: önce arka plan goroutine'lerine dur sinyali (cancel),
	// sonra hepsinin fiilen durmasını bekle (arkaPlan.Wait — bu, periyodik
	// kayıt goroutine'inin son bir "bilinen eşler" yazması için de zaman
	// tanır), EN SON ağ/depo/günlük kaynaklarını kapat.
	cancel()
	arkaPlan.Wait()
	if err := bilinenPeerleriKaydet(dataDir, h); err != nil {
		gunlukHata("kapanışta bilinen eşler kaydedilemedi:", err)
	}
	_ = kesif.Kapat()
	_ = h.Close()
	_ = depo.Kapat()
	gunlukBilgi("normal şekilde kapatıldı")

	if runErr != nil {
		olumculCik("TUI çalıştırma hatası", runErr)
	}
}
