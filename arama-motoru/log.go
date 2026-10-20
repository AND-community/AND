package main

// log.go — bu programın çoğu hatayı (tek bir eşle senkronizasyon
// başarısız oldu, bir mesajın imzası geçersizdi vb.) kasıtlı olarak
// "en iyi çaba" mantığıyla sessizce yutması (_ = err), kullanıcı deneyimini
// bozmamak için doğru bir tasarım — ama bu, gerçek kullanımda "neden hiç
// sonuç yok" gibi sorunları teşhis etmeyi zorlaştırır. Bu dosya, TUI'yi
// hiç etkilemeyen (stdout'a değil bir dosyaya yazan), seviyeli ve boyut
// sınırlı bir tanılama günlüğü sağlar.

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	golog "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const gunlukDosyaAdi = "gunluk.log"

// maxGunlukBoyutu aşıldığında dosya .old'a taşınıp sıfırdan başlanır —
// uzun süre açık kalan bir işlemde günlüğün sınırsız büyümesini önler.
const maxGunlukBoyutu = 5 * 1024 * 1024

// gunlukcu, gunlukAyarla çağrılana kadar hiçbir şey yazmaz (io.Discard) —
// böylece testler ya da gunlukAyarla'nın başarısız olduğu durumlar, her
// çağrı noktasında nil kontrolü gerektirmez.
var gunlukcu = log.New(io.Discard, "", log.LstdFlags)

type gunlukSeviye int

const (
	seviyeHata gunlukSeviye = iota
	seviyeUyari
	seviyeBilgi
)

// aktifGunlukSeviyesi, hangi seviyedeki satırların GERÇEKTEN yazılacağını
// belirler — varsayılan (seviyeBilgi) her şeyi yazar, tıpkı bu alan hiç
// yokmuş gibi eski davranışla aynı. hata seviyesi bu filtreden bağımsız
// olarak HER ZAMAN yazılır (bkz. gunlukHata) — bir çalıştırmayı
// teşhis etmenin son çaresi budur, asla sessizce yutulmamalı.
var aktifGunlukSeviyesi = seviyeBilgi

// gunlukSeviyeAyarla, -log-level bayrağının değerini uygular. "" (bayrak
// verilmediğinde flag.String'in kendi varsayılanı) da geçerli kabul edilir
// ve varsayılan seviyeyi korur.
func gunlukSeviyeAyarla(s string) error {
	switch s {
	case "", "bilgi":
		aktifGunlukSeviyesi = seviyeBilgi
	case "uyari", "uyarı":
		aktifGunlukSeviyesi = seviyeUyari
	case "hata":
		aktifGunlukSeviyesi = seviyeHata
	default:
		return fmt.Errorf("geçersiz -log-level değeri %q (hata|uyari|bilgi olmalı)", s)
	}
	return nil
}

func gunlukBilgi(v ...any) {
	if aktifGunlukSeviyesi >= seviyeBilgi {
		gunlukcu.Println(append([]any{"[bilgi]"}, v...)...)
	}
}
func gunlukUyari(v ...any) {
	if aktifGunlukSeviyesi >= seviyeUyari {
		gunlukcu.Println(append([]any{"[uyarı]"}, v...)...)
	}
}
func gunlukHata(v ...any) { gunlukcu.Println(append([]any{"[hata]"}, v...)...) }

func gunlukBilgif(format string, v ...any) {
	if aktifGunlukSeviyesi >= seviyeBilgi {
		gunlukcu.Printf("[bilgi] "+format, v...)
	}
}

// donerGunluk, boyut sınırını aşınca kendini döndüren (eskisini .old'a
// taşıyıp sıfırdan başlayan) bir io.WriteCloser'dır.
type donerGunluk struct {
	mu    sync.Mutex
	yol   string
	dosya *os.File
}

func (g *donerGunluk) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.dosya.Write(p)
}

func (g *donerGunluk) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.dosya.Close()
}

// BoyutKontrolEt, dosya maxGunlukBoyutu'nu aşmışsa döndürür. Periyodik
// olarak (uzun süre açık kalan işlemlerde) çağrılmak üzere tasarlandı —
// bkz. main.go'daki çağrı noktası.
func (g *donerGunluk) BoyutKontrolEt() {
	g.mu.Lock()
	defer g.mu.Unlock()
	bilgi, err := g.dosya.Stat()
	if err != nil || bilgi.Size() < maxGunlukBoyutu {
		return
	}
	_ = g.dosya.Close()
	eski := g.yol + ".old"
	_ = os.Remove(eski)
	_ = os.Rename(g.yol, eski)
	if f, err := os.OpenFile(g.yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		g.dosya = f
	}
}

// gunlukAyarla, dataDir/gunluk.log dosyasını (ekleme kipinde) açıp
// gunlukcu'yu ona yönlendirir. Dönen *donerGunluk hem io.Closer'dır
// (program kapanırken kapatılmalı) hem de periyodik boyut kontrolü için
// BoyutKontrolEt metodunu sağlar.
func gunlukAyarla(dataDir string) (*donerGunluk, error) {
	yol := filepath.Join(dataDir, gunlukDosyaAdi)
	f, err := os.OpenFile(yol, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	g := &donerGunluk{yol: yol, dosya: f}
	gunlukcu = log.New(g, "", log.LstdFlags)
	kutuphaneGunlukleriniYonlendir(g)
	return g, nil
}

// kutuphaneGunlukleriniYonlendir, kullanılan kütüphanelerin terminale
// yazdığı günlükleri w'ye (gunluk.log ya da io.Discard) yönlendirir. Program
// tam ekran bir TUI'dir; terminale doğrudan yazılan tek bir satır bile ekranı
// kaydırıp çerçeveyi bozar. İki kaynak vardır:
//
//   - Standart log paketi: libp2p'nin mDNS'i (zeroconf), Windows'ta çoklu
//     yayına uygun olmayan her ağ arayüzü için (Hyper-V, WSL, VPN gibi sanal
//     adaptörler) "[WARN] mdns: Failed to set multicast interface: no such
//     interface" yazar — mDNS uygun arayüzlerde çalışmaya devam eder, satır
//     zararsızdır ama sürekli tekrarlanır.
//   - go-log (libp2p'nin kendi günlüğü): varsayılan olarak hata düzeyindeki
//     satırları stderr'e yazar.
func kutuphaneGunlukleriniYonlendir(w io.Writer) {
	log.SetOutput(&tekrarSuzucu{hedef: w})
	golog.SetPrimaryCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(w), zapcore.DebugLevel))
}

// mdnsArayuzUyarisi, zeroconf'un her uygunsuz ağ arayüzü için tekrarladığı
// zararsız uyarıdır (bkz. kutuphaneGunlukleriniYonlendir).
var mdnsArayuzUyarisi = []byte("mdns: Failed to set multicast interface")

// tekrarSuzucu, mDNS arayüz uyarısını günlüğe yalnızca bir kez yazar (aksi
// halde dosyayı saniyede onlarca özdeş satırla doldururdu); diğer her satırı
// olduğu gibi iletir.
type tekrarSuzucu struct {
	hedef       io.Writer
	mu          sync.Mutex
	mdnsYazildi bool
}

func (t *tekrarSuzucu) Write(p []byte) (int, error) {
	if bytes.Contains(p, mdnsArayuzUyarisi) {
		t.mu.Lock()
		tekrar := t.mdnsYazildi
		t.mdnsYazildi = true
		t.mu.Unlock()
		if tekrar {
			return len(p), nil
		}
	}
	return t.hedef.Write(p)
}
