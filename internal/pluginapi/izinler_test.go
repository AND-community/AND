package pluginapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouteTableIzinleri: her uç nokta bilinçli bir izin taşır. Yeni bir uç
// nokta izinsiz (boş) eklenirse ya da hassas bir uç nokta yanlışlıkla
// herkese açık bırakılırsa bu test başarısız olur.
func TestRouteTableIzinleri(t *testing.T) {
	s := NewServer(&mockID{}, &mockForum{}, nil, nil, nil, nil, nil, "", nil)
	gecerli := map[string]bool{IzinTemel: true}
	for _, b := range IzinBilgileri {
		gecerli[b.Ad] = true
	}
	beklenen := map[string]string{
		"/api/v1/dm/send":           IzinDM,
		"/api/v1/dm/poll":           IzinDM,
		"/api/v1/dm/history":        IzinDM,
		"/api/v1/file/send":         IzinDosya,
		"/api/v1/file/poll":         IzinDosya,
		"/api/v1/file/consent-poll": IzinDosya,
		"/api/v1/file/consent":      IzinDosya,
		"/api/v1/forum/delete":      IzinModerasyon,
		"/api/v1/mod/grant":         IzinModerasyon,
		"/api/v1/mod/revoke":        IzinModerasyon,
		"/api/v1/mod/unban":         IzinModerasyon,
		"/api/v1/chat/send":         IzinSohbet,
		"/api/v1/site/publish":      IzinSite,
		"/api/v1/theme":             IzinTema,
	}
	for _, r := range s.routeTable() {
		if !gecerli[r.izin] {
			t.Errorf("%s: geçersiz ya da boş izin %q", r.pattern, r.izin)
		}
		if r.okumaSerbest && r.izin == IzinTemel {
			t.Errorf("%s: okumaSerbest yalnızca izin isteyen bir route'ta anlamlı", r.pattern)
		}
		if want, ok := beklenen[r.pattern]; ok && r.izin != want {
			t.Errorf("%s: izin %q olmalı, %q", r.pattern, want, r.izin)
		}
	}
}

func TestBilinenIzinler(t *testing.T) {
	got := BilinenIzinler([]string{"theme", "dm", "yok", "dm", "file"})
	if strings.Join(got, ",") != "dm,file,theme" {
		t.Fatalf("tanınanlar tekrarsız ve IzinBilgileri sırasıyla gelmeli: %v", got)
	}
	if BilinenIzinler(nil) != nil {
		t.Fatal("boş istek boş dönmeli")
	}
}

// izinliIstemci, sunucudan adı verilen izinlerle bir eklenti anahtarı alıp o
// anahtarla konuşan bir istemci döndürür.
func izinliIstemci(t *testing.T, srv *Server, addr string, izinler ...string) (*Client, func()) {
	t.Helper()
	anahtar, geriAl, err := srv.EklentiAnahtari("deneme", izinler)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AND_API_ADDR", addr)
	t.Setenv("AND_API_TOKEN", anahtar)
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return c, geriAl
}

func durumKodu(err error) int {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

func TestEklentiAnahtari_IzinleriUygular(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dm := &historyDM{kayitlar: []DMRecord{{PeerID: "12D3KooWBob", Text: "gizli"}}}
	f := &mockForum{}
	srv := NewServer(&mockID{name: "ben", peerID: "12D3KooWBen"}, f, nil, dm, nil, nil, nil, "", nil)
	addr, _, err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// İzinsiz eklenti: herkese açık okumalar çalışır, özel mesajlar ve konu
	// açma 403 döner.
	c, _ := izinliIstemci(t, srv, addr)
	if _, err := c.Identity(); err != nil {
		t.Fatalf("kimlik herkese açık olmalı: %v", err)
	}
	if v, err := c.APIVersion(); err != nil || v != APIVersion {
		t.Fatalf("sürüm: %d %v", v, err)
	}
	if _, err := c.GetPost(strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("forum okuma herkese açık olmalı: %v", err)
	}
	if _, err := c.DMHistory(); durumKodu(err) != http.StatusForbidden {
		t.Fatalf("dm izni olmadan geçmiş okunmamalı, hata: %v", err)
	} else if !strings.Contains(err.Error(), "dm") {
		t.Fatalf("hata hangi iznin eksik olduğunu söylemeli: %v", err)
	}
	if err := c.CreatePost("Go", "başlık", "gövde"); durumKodu(err) != http.StatusForbidden {
		t.Fatalf("forum izni olmadan konu açılmamalı: %v", err)
	}
	if len(f.created) != 0 {
		t.Fatal("reddedilen istek arka uca ulaşmamalı")
	}

	// dm ve forum izinli eklenti.
	c2, geriAl := izinliIstemci(t, srv, addr, IzinDM, IzinForum)
	if got, err := c2.DMHistory(); err != nil || len(got) != 1 {
		t.Fatalf("dm izniyle geçmiş okunmalı: %v %v", got, err)
	}
	if err := c2.CreatePost("Go", "başlık", "gövde"); err != nil || len(f.created) != 1 {
		t.Fatalf("forum izniyle konu açılmalı: %v", err)
	}
	if err := c2.DeletePost(strings.Repeat("ab", 32)); durumKodu(err) != http.StatusForbidden {
		t.Fatalf("moderasyon izni olmadan konu silinmemeli: %v", err)
	}

	// Eklenti kapanınca anahtar geçersizleşir.
	geriAl()
	geriAl() // ikinci çağrı zararsız
	if _, err := c2.Identity(); durumKodu(err) != http.StatusUnauthorized {
		t.Fatalf("geri alınan anahtar reddedilmeli: %v", err)
	}
}

func TestAnaAnahtarTamYetkili(t *testing.T) {
	dm := &historyDM{kayitlar: []DMRecord{{PeerID: "12D3KooWBob", Text: "x"}}}
	c := startTestServer(t, &mockID{}, &mockForum{}, dm)
	if got, err := c.DMHistory(); err != nil || len(got) != 1 {
		t.Fatalf("AND'ın kendi anahtarı her uca erişmeli: %v %v", got, err)
	}
}

func TestIzinDenetle_YetkisizIstekReddedilir(t *testing.T) {
	cagrildi := false
	h := izinDenetle(IzinTemel, false, func(http.ResponseWriter, *http.Request) { cagrildi = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusForbidden || cagrildi {
		t.Fatalf("yetki taşımayan istek (kimlik doğrulamadan geçmemiş) reddedilmeli: %d", rec.Code)
	}
}

func TestIzinDenetle_OkumaSerbest(t *testing.T) {
	cagri := 0
	h := tamYetkisiz(izinDenetle(IzinTema, true, func(http.ResponseWriter, *http.Request) { cagri++ }))
	for _, tc := range []struct {
		method string
		kod    int
	}{{http.MethodGet, http.StatusOK}, {http.MethodPost, http.StatusForbidden}} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, "/api/v1/theme", nil))
		if rec.Code != tc.kod {
			t.Errorf("%s: %d bekleniyordu, %d", tc.method, tc.kod, rec.Code)
		}
	}
	if cagri != 1 {
		t.Fatalf("yalnızca GET işleyiciye ulaşmalı, %d çağrı", cagri)
	}
}

// tamYetkisiz, isteğe izinsiz bir eklentinin yetkisini ekler.
func tamYetkisiz(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), yetkiAnahtari{}, yetki{eklenti: "x", izinler: map[string]bool{}})))
	})
}

func TestAPIVersion_EskiANDBirDoner(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	t.Setenv("AND_API_ADDR", strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("AND_API_TOKEN", "x")
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := c.APIVersion(); err != nil || v != 1 {
		t.Fatalf("sürüm ucu olmayan AND 1 sayılmalı: %d %v", v, err)
	}
}
