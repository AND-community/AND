package masaustu

// sunucu.go — masaüstü arayüzünü sunan yerel HTTP sunucusu.
//
// Sunucu yalnızca 127.0.0.1'de, rastgele bir portta dinler. Arayüzü açan
// pencere ya da tarayıcı, adresteki tek kullanımlık açılış koduyla bir oturum
// çerezi alır; çerezi olmayan hiçbir istek (sayfanın kendisi dahil) yanıt
// almaz. Aynı bilgisayardaki başka bir programın ya da bir web sitesinin
// arayüze erişmemesi için:
//
//   - Host başlığı tam olarak 127.0.0.1:<port> olmalı (DNS yeniden bağlama).
//   - Durum değiştiren istekler X-AND başlığını taşımalı; başka bir kökenden
//     gelen bir sayfa bu başlığı ön uçuş (CORS) onayı olmadan gönderemez,
//     sunucu da hiçbir kökene onay vermez. Origin ya da Sec-Fetch-Site başka
//     bir site gösteriyorsa istek reddedilir.
//   - Çerez HttpOnly ve SameSite=Strict'tir; sayfalar sıkı bir içerik
//     güvenliği politikasıyla (CSP) sunulur: satır içi betik yok, yalnızca
//     bu sunucudan betik, stil ve bağlantı.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lucian95511/and/arayuz"
)

//go:embed web
var webDosyalari embed.FS

const (
	cerezAdi     = "and_oturum"
	istekBasligi = "X-AND"
	// enBuyukGovde, bir API isteğinin gövdesinin üst sınırıdır; en uzun
	// içerik (8 KB'lık bir forum konusu) JSON'la birlikte rahatça sığar.
	enBuyukGovde = 64 << 10
)

const icerikPolitikasi = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

type sunucu struct {
	adres string // 127.0.0.1:<port>
	ln    net.Listener
	srv   *http.Server
	mux   *http.ServeMux

	mu         sync.Mutex
	acilisKodu string // ilk kullanımda silinir
	oturum     string
}

func rastgele() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand Go 1.24'ten beri hata döndürmez
	}
	return hex.EncodeToString(b)
}

func yeniSunucu() (*sunucu, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &sunucu{
		adres:      ln.Addr().String(),
		ln:         ln,
		mux:        http.NewServeMux(),
		acilisKodu: rastgele(),
		oturum:     rastgele(),
	}
	statik, err := fs.Sub(webDosyalari, "web")
	if err != nil {
		ln.Close()
		return nil, err
	}
	s.mux.Handle("GET /", http.FileServerFS(statik))
	s.srv = &http.Server{
		Handler:           s.koru(s.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// acilisAdresi, arayüzü açan adrestir; içindeki kod yalnızca bir kez geçerlidir.
func (s *sunucu) acilisAdresi() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return "http://" + s.adres + "/ac?kod=" + s.acilisKodu
}

func (s *sunucu) baslat() {
	go func() {
		if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Dinleyici kapandıysa arayüz zaten kapanıyordur.
			_ = err
		}
	}()
}

func (s *sunucu) kapat() {
	ctx, iptal := context.WithTimeout(context.Background(), 2*time.Second)
	defer iptal()
	_ = s.srv.Shutdown(ctx)
}

func esit(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// koru, her isteği yukarıdaki kurallara göre denetler.
func (s *sunucu) koru(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.adres {
			http.Error(w, "", http.StatusMisdirectedRequest)
			return
		}
		b := w.Header()
		b.Set("Content-Security-Policy", icerikPolitikasi)
		b.Set("X-Content-Type-Options", "nosniff")
		b.Set("Referrer-Policy", "no-referrer")
		b.Set("Cross-Origin-Opener-Policy", "same-origin")
		b.Set("Cross-Origin-Resource-Policy", "same-origin")
		b.Set("X-Frame-Options", "DENY")
		b.Set("Cache-Control", "no-store")

		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			http.Error(w, "", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/ac" && r.Method == http.MethodGet {
			s.ac(w, r)
			return
		}
		c, err := r.Cookie(cerezAdi)
		if err != nil || !esit(c.Value, s.oturum) {
			http.Error(w, "", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get(istekBasligi) != "1" {
				http.Error(w, "", http.StatusForbidden)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.adres {
				http.Error(w, "", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, govdeSiniri(r.URL.Path))
		}
		h.ServeHTTP(w, r)
	})
}

// ac, tek kullanımlık açılış kodunu oturum çerezine çevirir ve arayüze yönlendirir.
func (s *sunucu) ac(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	gecerli := esit(r.URL.Query().Get("kod"), s.acilisKodu)
	if gecerli {
		s.acilisKodu = ""
	}
	s.mu.Unlock()
	if !gecerli {
		http.Error(w, "", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cerezAdi, Value: s.oturum, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ─── JSON yardımcıları ──────────────────────────────────────────────────────

func jsonYaz(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// hataYaz, arayüzün kullanıcıya göstereceği (etkin dilde) bir hata döner.
func hataYaz(w http.ResponseWriter, kod int, mesaj string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(kod)
	_ = json.NewEncoder(w).Encode(map[string]string{"hata": mesaj})
}

func jsonOku(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func temiz(s string) string { return strings.TrimSpace(s) }

// govdeSiniri, bir isteğin gövdesinin üst sınırıdır. Arayüz kipindeki
// eklentilerin olayları (ör. bir dosya düzenleyicinin içeriği) protokolün satır
// sınırına dek büyüyebilir; geri kalan her istek enBuyukGovde'yle sınırlıdır.
func govdeSiniri(yol string) int64 {
	if yol == "/api/arayuz/olay" {
		return arayuz.SatirSiniri
	}
	return enBuyukGovde
}
