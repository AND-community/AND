package githubverify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withMockEndpoints redirects deviceCodeURL/accessTokenURL/userAPIURL (via
// http.DefaultTransport) to ts for the duration of the test, and restores
// the real URLs on cleanup — RequestDeviceCode/PollForToken/FetchUsername
// call the package-level const URLs directly, so we swap in a RoundTripper
// that rewrites the scheme+host to the test server's instead of threading a
// baseURL parameter through every exported function just for tests.
func withMockEndpoints(t *testing.T, ts *httptest.Server) {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = rewriteTransport{to: ts.URL, base: orig}
	t.Cleanup(func() { http.DefaultTransport = orig })
}

type rewriteTransport struct {
	to   string
	base http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	target, err := http.NewRequest(req.Method, rt.to+u.Path, req.Body)
	if err != nil {
		return nil, err
	}
	target.Header = req.Header
	target = target.WithContext(req.Context())
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(target)
}

func setClientID(t *testing.T, id string) {
	t.Helper()
	old := ClientID
	ClientID = id
	t.Cleanup(func() { ClientID = old })
}

// ─── RequestDeviceCode ───────────────────────────────────────────────────────

func TestRequestDeviceCode_NotConfigured(t *testing.T) {
	setClientID(t, "")
	if _, err := RequestDeviceCode(context.Background()); err != ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRequestDeviceCode_Success(t *testing.T) {
	setClientID(t, "test-client-id")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/device/code" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("client_id") != "test-client-id" {
			t.Errorf("client_id = %q", r.FormValue("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "devcode123",
			"user_code":        "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         5,
		})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	dc, err := RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	if dc.UserCode != "ABCD-1234" || dc.DeviceCode != "devcode123" {
		t.Errorf("dc = %+v", dc)
	}
	if dc.Interval != 5 || dc.ExpiresIn != 900 {
		t.Errorf("dc interval/expiry = %d/%d", dc.Interval, dc.ExpiresIn)
	}
}

func TestRequestDeviceCode_HTTPErrorRejected(t *testing.T) {
	setClientID(t, "test-client-id")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	if _, err := RequestDeviceCode(context.Background()); err == nil {
		t.Fatal("http 500 döndüren bir sunucu hata döndürmeliydi")
	}
}

func TestRequestDeviceCode_MissingFieldsRejected(t *testing.T) {
	setClientID(t, "test-client-id")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"device_code": ""})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	if _, err := RequestDeviceCode(context.Background()); err == nil {
		t.Fatal("eksik device_code/user_code alanları olan bir yanıt reddedilmeliydi")
	}
}

// ─── PollForToken ───────────────────────────────────────────────────────────

func TestPollForToken_SucceedsAfterPending(t *testing.T) {
	setClientID(t, "test-client-id")
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-abc"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	dc := &DeviceCode{DeviceCode: "dc1", Interval: 0, ExpiresIn: 60}
	// interval 0 → PollForToken defaultInterval'a düşer; testin makul sürede
	// bitmesi için interval'i doğrudan çok küçük bir değere zorluyoruz.
	dc.Interval = 1

	start := time.Now()
	tok, err := PollForToken(context.Background(), dc)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok != "tok-abc" {
		t.Errorf("token = %q", tok)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("beklenenden çok uzun sürdü: %v", elapsed)
	}
	if calls < 3 {
		t.Errorf("calls = %d, want >= 3 (pending x2 + success)", calls)
	}
}

func TestPollForToken_AccessDeniedStopsImmediately(t *testing.T) {
	setClientID(t, "test-client-id")
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "access_denied"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	dc := &DeviceCode{DeviceCode: "dc1", Interval: 1, ExpiresIn: 60}
	_, err := PollForToken(context.Background(), dc)
	if err == nil {
		t.Fatal("access_denied bir hata döndürmeliydi")
	}
	if calls != 1 {
		t.Errorf("access_denied sonrası yoklamaya devam edilmemeliydi, calls = %d", calls)
	}
}

func TestPollForToken_ContextCancelStopsPolling(t *testing.T) {
	setClientID(t, "test-client-id")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	ctx, cancel := context.WithCancel(context.Background())
	dc := &DeviceCode{DeviceCode: "dc1", Interval: 1, ExpiresIn: 60}

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := PollForToken(ctx, dc)
	if err == nil {
		t.Fatal("iptal edilen bir bağlam hata döndürmeliydi")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("iptalden sonra hemen dönmeliydi, %v sürdü", elapsed)
	}
}

func TestPollForToken_ExpiredDeadlineStopsPolling(t *testing.T) {
	setClientID(t, "test-client-id")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	dc := &DeviceCode{DeviceCode: "dc1", Interval: 1, ExpiresIn: 1} // 1 saniye içinde dolar
	_, err := PollForToken(context.Background(), dc)
	if err == nil {
		t.Fatal("süresi dolan bir kod hata döndürmeliydi")
	}
}

// ─── FetchUsername ──────────────────────────────────────────────────────────

func TestFetchUsername_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok-abc" {
			t.Errorf("Authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	name, err := FetchUsername(context.Background(), "tok-abc")
	if err != nil {
		t.Fatalf("FetchUsername: %v", err)
	}
	if name != "octocat" {
		t.Errorf("name = %q", name)
	}
}

func TestFetchUsername_InvalidLoginShapeRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Kontrol karakterleri / ANSI kaçış dizileri içeren bir "login" —
		// GitHub'ın API'si asla böyle bir şey döndürmemeli, ama gelen yanıta
		// kör güvenmemek gerekir (bkz. paket belgesi).
		json.NewEncoder(w).Encode(map[string]any{"login": "oct\x1b[31mocat"})
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	if _, err := FetchUsername(context.Background(), "tok-abc"); err == nil {
		t.Fatal("GitHub kullanıcı adı biçimine uymayan bir login reddedilmeliydi")
	}
}

func TestFetchUsername_EmptyTokenRejected(t *testing.T) {
	if _, err := FetchUsername(context.Background(), ""); err == nil {
		t.Fatal("boş erişim jetonu reddedilmeliydi")
	}
}

func TestFetchUsername_HTTPErrorRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	withMockEndpoints(t, ts)

	if _, err := FetchUsername(context.Background(), "tok-abc"); err == nil {
		t.Fatal("http 401 döndüren bir sunucu hata döndürmeliydi")
	}
}

// ─── ValidUsername ──────────────────────────────────────────────────────────

func TestValidUsername(t *testing.T) {
	cases := map[string]bool{
		"":                      true, // henüz bağlanmamış
		"octocat":               true,
		"oct-o-cat":             true,
		"a":                     true,
		strings.Repeat("a", 39): true,
		strings.Repeat("a", 40): false, // çok uzun
		"-octocat":              false, // baştan tire
		"octocat-":              false, // sondan tire
		"oct ocat":              false, // boşluk
		"oct\x1b[0mocat":        false, // ANSI kaçış
	}
	for in, want := range cases {
		if got := ValidUsername(in); got != want {
			t.Errorf("ValidUsername(%q) = %v, want %v", in, got, want)
		}
	}
}
