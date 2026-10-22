package pluginapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/lucian95511/and/dil"
)

// APIVersion, eklenti API'sinin sürümüdür. Bir eklentinin manifest'indeki
// min_api bundan büyükse AND onu başlatmaz: eklenti daha yeni bir AND ister.
//
//	1: ilk sürüm (bu sabit ve /api/v1/version yoktu)
//	2: izin modeli ve /api/v1/version
const APIVersion = 2

// Eklenti izinleri. Her uç nokta routeTable'da bunlardan birini taşır.
// IzinTemel herkese açık okumalardır (kimlik, eşler, forum ve site içeriği,
// genel sohbet, tema); her eklentide vardır ve onay istemez.
const (
	IzinTemel      = "temel"
	IzinDM         = "dm"
	IzinDosya      = "file"
	IzinSohbet     = "chat"
	IzinForum      = "forum"
	IzinSite       = "site"
	IzinModerasyon = "moderation"
	IzinTema       = "theme"
)

// IzinBilgisi, onay ekranında bir iznin kullanıcıya nasıl anlatılacağıdır.
// Baslik ve Aciklama Türkçe kaynak metinlerdir; ekranda dil.T ile çevrilir.
type IzinBilgisi struct {
	Ad       string
	Baslik   string
	Aciklama string
}

// IzinBilgileri, onay istenebilecek izinlerdir; sırası ekrandaki sıradır.
var IzinBilgileri = []IzinBilgisi{
	{IzinDM, dil.K("Özel mesajlar"), dil.K("Özel mesaj geçmişini okuyabilir ve senin adına özel mesaj gönderebilir.")},
	{IzinDosya, dil.K("Dosya aktarımı"), dil.K("Senin adına dosya gönderebilir, gelen dosya isteklerini görüp yanıtlayabilir.")},
	{IzinSohbet, dil.K("Genel sohbet"), dil.K("Genel sohbete senin adına mesaj yazabilir.")},
	{IzinForum, dil.K("Forum"), dil.K("Senin adına konu açabilir ve yanıt yazabilir.")},
	{IzinSite, dil.K("Kişisel site"), dil.K("Kişisel sitende sayfa yayınlayabilir ve silebilir.")},
	{IzinModerasyon, dil.K("Moderasyon"), dil.K("Kurucu ya da moderatörsen senin yetkinle konu silebilir, ban kaldırabilir, moderatör atayabilir.")},
	{IzinTema, dil.K("Tema"), dil.K("AND'ın renk temasını değiştirebilir.")},
}

// IzinBul, adı verilen iznin bilgisini döndürür.
func IzinBul(ad string) (IzinBilgisi, bool) {
	for _, b := range IzinBilgileri {
		if b.Ad == ad {
			return b, true
		}
	}
	return IzinBilgisi{}, false
}

// BilinenIzinler, istenen'deki tanınan izinleri tekrarsız ve IzinBilgileri
// sırasıyla döndürür. Tanınmayan adlar (yazım hatası ya da daha yeni bir
// AND'ın izni) atılır: hiçbir şeye yetki vermezler.
func BilinenIzinler(istenen []string) []string {
	var out []string
	for _, b := range IzinBilgileri {
		for _, ad := range istenen {
			if ad == b.Ad {
				out = append(out, b.Ad)
				break
			}
		}
	}
	return out
}

// yetki, bir API anahtarının neye erişebildiğidir.
type yetki struct {
	eklenti string // anahtarın verildiği eklenti; "" ise AND'ın kendisi ya da harici araç
	tam     bool   // her uç noktaya erişir
	izinler map[string]bool
}

func (y yetki) izinli(izin string) bool {
	return y.tam || izin == IzinTemel || y.izinler[izin]
}

type yetkiAnahtari struct{}

// anahtarOzeti, anahtar haritasının anahtarıdır. Ham anahtar yerine özeti
// tutmak, haritadaki aramanın zamanlamasının anahtar hakkında bilgi
// sızdırmasını önler: saldırganın tahmini yalnızca kendi tahmininin
// özetiyle karşılaştırılır.
func anahtarOzeti(anahtar string) [sha256.Size]byte {
	return sha256.Sum256([]byte(anahtar))
}

func yeniAnahtar() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (s *Server) yetkiKaydet(anahtar string, y yetki) {
	s.yetkiMu.Lock()
	defer s.yetkiMu.Unlock()
	if s.yetkiler == nil {
		s.yetkiler = map[[sha256.Size]byte]yetki{}
	}
	s.yetkiler[anahtarOzeti(anahtar)] = y
}

func (s *Server) yetkiBul(anahtar string) (yetki, bool) {
	if anahtar == "" {
		return yetki{}, false
	}
	s.yetkiMu.RLock()
	defer s.yetkiMu.RUnlock()
	y, ok := s.yetkiler[anahtarOzeti(anahtar)]
	return y, ok
}

// EklentiAnahtari, adı verilen eklenti için yalnızca izinler'i (tanınanları)
// taşıyan yeni bir API anahtarı üretir. AND her eklenti başlatışında yeni bir
// anahtar alır ve eklenti kapanınca geriAl'ı çağırır; anahtar o andan sonra
// geçersizdir. geriAl birden çok kez çağrılabilir.
func (s *Server) EklentiAnahtari(eklenti string, izinler []string) (anahtar string, geriAl func(), err error) {
	anahtar, err = yeniAnahtar()
	if err != nil {
		return "", nil, fmt.Errorf("pluginapi: eklenti anahtarı üretilemedi: %w", err)
	}
	y := yetki{eklenti: eklenti, izinler: map[string]bool{}}
	for _, iz := range BilinenIzinler(izinler) {
		y.izinler[iz] = true
	}
	s.yetkiKaydet(anahtar, y)
	oz := anahtarOzeti(anahtar)
	var once sync.Once
	return anahtar, func() {
		once.Do(func() {
			s.yetkiMu.Lock()
			delete(s.yetkiler, oz)
			s.yetkiMu.Unlock()
		})
	}, nil
}

// tamYetkiyle, isteğe tam yetki ekler. Yalnızca erişimi işletim sisteminin
// zaten "bu kullanıcı" ile sınırladığı harici dinleyicide kullanılır (bkz.
// enableExternalAPI): oradaki güvenilir araç (VS Code eklentisi) AND'ın kendi
// arayüzüyle eşdeğer erişime sahiptir.
func tamYetkiyle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), yetkiAnahtari{}, yetki{tam: true})))
	})
}

// izinDenetle, bir uç noktanın izin denetimidir. İstekte yetki yoksa
// (kimlik doğrulamadan geçmemiş bir istek) erişim reddedilir; varsayılan
// kapalıdır. okumaSerbest ise GET istekleri izin istemez: aynı yolda
// herkese açık okuma ve izin isteyen yazma birlikte durur (forum/post,
// theme).
func izinDenetle(izin string, okumaSerbest bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		y, ok := r.Context().Value(yetkiAnahtari{}).(yetki)
		if !ok {
			writeForbidden(w, "yetkisiz erişim")
			return
		}
		if (okumaSerbest && r.Method == http.MethodGet) || y.izinli(izin) {
			h(w, r)
			return
		}
		mesaj := fmt.Sprintf("bu eklentinin %q izni yok", izin)
		if b, ok := IzinBul(izin); ok {
			mesaj = fmt.Sprintf("bu eklentinin %q izni yok (%s); AND'da izin verilmeden bu işlem yapılamaz", izin, b.Baslik)
		}
		writeForbidden(w, mesaj)
	}
}

func writeForbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(errResp{Error: msg}) //nolint:errcheck
}

// VersionInfo, GET /api/v1/version yanıtıdır.
type VersionInfo struct {
	API int `json:"api"`
}

func (s *Server) getVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, VersionInfo{API: APIVersion})
}

// APIVersion, bağlı AND'ın eklenti API sürümünü döndürür. Sürüm ucu olmayan
// eski bir AND için 1 döner.
func (c *Client) APIVersion() (int, error) {
	var v VersionInfo
	if err := c.get("/api/v1/version", &v); err != nil {
		if IsNotFound(err) {
			return 1, nil
		}
		return 0, err
	}
	return v.API, nil
}
