package pluginapi

// harici.go — VS Code gibi güvenilir harici araçların bağlandığı ikinci
// dinleyicinin (bkz. enableExternalAPI, externalpipe_*.go) AND'ın
// Ayarlar'ından açılıp kapatılması.
//
// AND artık çoğunlukla çift tıklamayla masaüstü uygulaması olarak açılıyor;
// AND_ENABLE_EXTERNAL_API ortam değişkenini ayarlamak bu kullanıcı için
// pratikte mümkün değil. Bu yüzden aynı dinleyici veri dizinindeki bir
// ayarla da açılır ve AND çalışırken açılıp kapatılabilir. Erişim kuralı
// değişmez: dinleyiciye yine yalnızca aynı işletim sistemi kullanıcısı
// bağlanabilir.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// HariciAyarDosyasi, harici bağlantı ayarının veri dizinindeki dosyasıdır.
const HariciAyarDosyasi = "harici_baglanti.json"

var (
	// ErrHariciOrtamda: bağlantı AND_ENABLE_EXTERNAL_API ortam değişkeniyle
	// açık; değişken her açılışta yeniden açacağı için ayardan kapatılamaz.
	ErrHariciOrtamda = errors.New("harici bağlantı AND_ENABLE_EXTERNAL_API ortam değişkeniyle açık")
	// ErrHariciKullanilamaz: bu sunucuda harici dinleyici yok (veri dizini
	// yok, HariciKapat çağrılmış ya da AND kapanıyor).
	ErrHariciKullanilamaz = errors.New("bu AND'da harici bağlantı kullanılamaz")
)

type hariciAyar struct {
	Acik bool `json:"acik"`
}

// HariciDurum, harici bağlantının o anki durumudur. Acik, dinleyicinin
// gerçekten çalıştığını; Ortamdan, AND_ENABLE_EXTERNAL_API=1 olduğunu söyler.
type HariciDurum struct {
	Acik     bool `json:"acik"`
	Ortamdan bool `json:"ortamdan"`
}

// HariciAyari, veri dizinindeki ayarın bağlantıyı açık tutup tutmadığını
// söyler. Dosya yoksa, okunamıyorsa ya da bozuksa kapalıdır.
func HariciAyari(dataDir string) bool {
	if dataDir == "" {
		return false
	}
	veri, err := os.ReadFile(filepath.Join(dataDir, HariciAyarDosyasi))
	if err != nil {
		return false
	}
	var a hariciAyar
	if json.Unmarshal(veri, &a) != nil {
		return false
	}
	return a.Acik
}

// hariciAyariKaydet, ayarı atomik olarak (geçici dosya + yeniden adlandırma)
// ve yalnızca sahibinin okuyabileceği izinlerle yazar.
func hariciAyariKaydet(dataDir string, acik bool) error {
	veri, err := json.MarshalIndent(hariciAyar{Acik: acik}, "", "  ")
	if err != nil {
		return err
	}
	gecici, err := os.CreateTemp(dataDir, HariciAyarDosyasi+".*.tmp")
	if err != nil {
		return err
	}
	geciciAd := gecici.Name()
	_, yazErr := gecici.Write(append(veri, '\n'))
	if yazErr == nil {
		yazErr = gecici.Chmod(0o600)
	}
	if yazErr == nil {
		yazErr = gecici.Sync()
	}
	if kapatErr := gecici.Close(); yazErr == nil {
		yazErr = kapatErr
	}
	if yazErr == nil {
		yazErr = os.Rename(geciciAd, filepath.Join(dataDir, HariciAyarDosyasi))
	}
	if yazErr != nil {
		_ = os.Remove(geciciAd)
	}
	return yazErr
}

// HariciDurumu, harici bağlantının o anki durumunu verir.
func (s *Server) HariciDurumu() HariciDurum {
	s.hariciMu.Lock()
	defer s.hariciMu.Unlock()
	return HariciDurum{Acik: s.externalSrv != nil, Ortamdan: enableExternalAPI()}
}

// HariciAyarla, harici bağlantıyı açar ya da kapatır ve seçimi veri
// dizinine kaydeder; değişiklik AND'ı yeniden başlatmadan hemen geçerli olur.
// Ortam değişkeniyle açık bağlantı kapatılamaz (ErrHariciOrtamda). Kayıt ya
// da dinleyici başarısız olursa hiçbir şey değişmez.
func (s *Server) HariciAyarla(acik bool) error {
	if s.hariciKapali || s.dataDir == "" {
		return ErrHariciKullanilamaz
	}
	if !acik && enableExternalAPI() {
		return ErrHariciOrtamda
	}
	s.hariciMu.Lock()
	defer s.hariciMu.Unlock()
	if s.hariciBitti {
		return ErrHariciKullanilamaz
	}
	if !acik {
		if err := hariciAyariKaydet(s.dataDir, false); err != nil {
			return err
		}
		s.hariciDurdurKilitli()
		return nil
	}
	yeniAcildi := false
	if s.externalSrv == nil {
		if err := s.hariciBaslatKilitli(); err != nil {
			return err
		}
		yeniAcildi = true
	}
	if err := hariciAyariKaydet(s.dataDir, true); err != nil {
		if yeniAcildi {
			s.hariciDurdurKilitli()
		}
		return err
	}
	return nil
}

// hariciBaslatKilitli, harici dinleyiciyi açar. hariciMu tutulurken çağrılır.
func (s *Server) hariciBaslatKilitli() error {
	extLn, err := listenExternal(s.dataDir)
	if err != nil {
		return fmt.Errorf("pluginapi: harici bağlantı dinleyicisi başlatılamadı: %w", err)
	}
	s.externalLn = extLn
	// authMiddleware YOK: bu dinleyiciye erişim zaten OS tarafından
	// (yalnızca bu kullanıcı) kısıtlanmış durumda — bkz. enableExternalAPI.
	// buildMux(true), yalnızca routeTable()'da allowExternal:true işaretli
	// route'ları içerir; buradaki araç tam yetkilidir.
	s.externalSrv = newHTTPServer(tamYetkiyle(s.buildMux(true)))
	go s.externalSrv.Serve(extLn) //nolint:errcheck
	return nil
}

// hariciDurdurKilitli, harici dinleyiciyi ve açık bağlantılarını kapatır.
// hariciMu tutulurken çağrılır.
func (s *Server) hariciDurdurKilitli() {
	if s.externalSrv == nil {
		return
	}
	s.externalSrv.Close() //nolint:errcheck
	// Serve dinleyiciyi henüz kaydetmeden Close çağrılmış olabilir; dinleyici
	// burada da kapatılır (ikinci kapatmanın hatası önemsizdir).
	_ = s.externalLn.Close()
	cleanupExternal(s.dataDir)
	s.externalSrv = nil
	s.externalLn = nil
}
