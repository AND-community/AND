// Package githubverify kanıtlar ki bir AND kimliğinin arkasındaki kişi,
// iddia ettiği bir GitHub hesabına gerçekten sahip. Bunu GitHub'ın Device
// Authorization Flow'uyla (RFC 8628) yapar: kullanıcıya kısa bir kod
// gösterilir, kullanıcı bu kodu tarayıcıda github.com/login/device'ta
// onaylar, biz de arka planda onay gelene kadar yoklama yaparız ve son
// olarak GitHub'ın kendi API'sinden gerçek kullanıcı adını çekeriz.
//
// Bu, bir merkezi sunucu gerektirmez (Device Flow "public client" olarak
// tasarlanmıştır — bir client secret'a ihtiyaç duymaz) ama GitHub'a çıkış
// yapan ağ isteklerine ihtiyaç duyar; bu da projenin "sunucusuz P2P"
// tasarımına eklenen tek dışa bağımlılıktır.
package githubverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ClientID, Device Flow için kullanılan GitHub OAuth App'in Client ID'sidir.
// Device Flow public client'ları bir secret taşımaz (dolayısıyla burada
// korunması gereken gizli bir değer yok), ama bir Client ID olmadan akış
// hiç başlayamaz. Derleme sırasında ayarlanmalı:
//
//	go build -ldflags "-X github.com/lucian95511/and/internal/githubverify.ClientID=<id>" ./cmd/and
//
// (bkz. internal/moderation.FounderPubKeyHex ve internal/updater.GitHubRepo
// — aynı "-ldflags -X ile derleme zamanında ayarlanan var" deseni.)
var ClientID = ""

const (
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
	userAPIURL     = "https://api.github.com/user"
	userAgent      = "AND-community/1.0"
	scope          = "read:user"

	// maxResponseBytes: bu uç noktaların hepsi birkaç yüz baytlık JSON
	// döndürür — ondan çok daha büyük bir gövde ya beklenmeyen bir yanıt ya
	// da kötü niyetli bir ortadaki-adam; her iki durumda da io.LimitReader
	// belleği sınırsızca tüketmeyi engeller.
	maxResponseBytes = 1 << 16

	requestTimeout = 30 * time.Second

	defaultInterval = 5 * time.Second // GitHub'ın belgelenen varsayılan yoklama aralığı
)

// ErrNotConfigured, ClientID derleme zamanında ayarlanmadan bu paketin
// herhangi bir işlevi çağrıldığında döner.
var ErrNotConfigured = errors.New("githubverify: ClientID yapılandırılmamış (derleme sırasında -ldflags -X ile ayarlanmalı)")

// usernameRe, GitHub'ın belgelenen login karakter kümesini yansıtır:
// alfanumerik karakterler ve tek tek tire, başta/sonda tire olmadan, en
// fazla 39 karakter. Bu kontrolü hem burada hem de profile paketinde
// (uzaktan gelen bir Profile mesajındaki GitHubUser alanı için) ayrı ayrı
// uyguluyoruz — GitHub'ın kendi API'sinden geldiğini iddia eden bir değere
// bile kör güvenmiyoruz.
var usernameRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// ValidUsername, s'nin GitHub'ın kullanıcı adı biçimine uyup uymadığını
// bildirir. Boş dize geçerli sayılır (henüz bağlanmamış anlamına gelir).
func ValidUsername(s string) bool {
	return s == "" || usernameRe.MatchString(s)
}

// DeviceCode, RequestDeviceCode'un sonucudur: kullanıcıya gösterilecek kod
// ile PollForToken'ın ihtiyaç duyduğu dahili durumu taşır.
type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int // saniye
	Interval        int // saniye — bu aralıktan daha sık yoklama yapılmamalı
}

// RequestDeviceCode, GitHub'ın Device Authorization Flow'unu (RFC 8628 §3.2)
// başlatır. Dönen DeviceCode.UserCode kullanıcıya gösterilmeli,
// VerificationURI'ye gidip onaylaması istenmeli; ardından PollForToken'a
// geçilmeli.
func RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	if ClientID == "" {
		return nil, ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	form := url.Values{"client_id": {ClientID}, "scope": {scope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("githubverify: cihaz kodu isteği: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("githubverify: cihaz kodu isteği http %d", resp.StatusCode)
	}

	var out struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return nil, fmt.Errorf("githubverify: cihaz kodu yanıtı çözümlenemedi: %w", err)
	}
	if out.DeviceCode == "" || out.UserCode == "" || out.VerificationURI == "" {
		return nil, errors.New("githubverify: eksik cihaz kodu yanıtı")
	}
	if out.Interval <= 0 {
		out.Interval = int(defaultInterval.Seconds())
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 900 // GitHub'ın belgelenen varsayılanı (15 dk)
	}
	return &DeviceCode{
		DeviceCode:      out.DeviceCode,
		UserCode:        out.UserCode,
		VerificationURI: out.VerificationURI,
		ExpiresIn:       out.ExpiresIn,
		Interval:        out.Interval,
	}, nil
}

var errSlowDown = errors.New("githubverify: slow_down")

// PollForToken, kullanıcı verification_uri'de user_code'u onaylayana kadar
// dc.Interval aralıklarla GitHub'ın token uç noktasını yoklar
// (RFC 8628 §3.4-3.5). ctx iptal edilirse ya da GitHub'ın bildirdiği
// ExpiresIn süresi dolarsa hata döner — sonsuza kadar yoklamaz.
func PollForToken(ctx context.Context, dc *DeviceCode) (string, error) {
	if ClientID == "" {
		return "", ErrNotConfigured
	}
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return "", errors.New("githubverify: kodun süresi doldu, tekrar deneyin")
		}

		token, retry, err := requestToken(ctx, dc.DeviceCode)
		if !retry {
			return token, err
		}
		if errors.Is(err, errSlowDown) {
			interval += 5 * time.Second
		}
	}
}

// requestToken tek bir yoklama isteği atar. retry=true, çağıranın
// döngüsüne devam etmesi (authorization_pending/slow_down/geçici ağ hatası)
// gerektiğini; retry=false ise bunun kesin sonuç (başarı ya da kalıcı
// başarısızlık) olduğunu bildirir.
func requestToken(ctx context.Context, deviceCode string) (token string, retry bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	form := url.Values{
		"client_id":   {ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", true, err // geçici ağ hatası — yoklamaya devam et
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", true, fmt.Errorf("githubverify: token isteği http %d", resp.StatusCode)
	}

	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", true, err
	}
	switch out.Error {
	case "":
		if out.AccessToken == "" {
			return "", true, errors.New("githubverify: boş erişim jetonu")
		}
		return out.AccessToken, false, nil
	case "authorization_pending":
		return "", true, nil
	case "slow_down":
		return "", true, errSlowDown
	case "expired_token":
		return "", false, errors.New("githubverify: kodun süresi doldu, tekrar deneyin")
	case "access_denied":
		return "", false, errors.New("githubverify: yetkilendirme reddedildi")
	default:
		return "", false, fmt.Errorf("githubverify: %s", out.Error)
	}
}

// FetchUsername, accessToken sahibinin GitHub kullanıcı adını döndürür.
// accessToken burada YALNIZCA bu tek istek için kullanılır — çağıran onu
// hiçbir yerde saklamamalı; ispatlanması gereken tek şey kullanıcı adı,
// jetonun kendisi değil (bkz. paket belgesi).
func FetchUsername(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("githubverify: boş erişim jetonu")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("githubverify: kullanıcı bilgisi isteği: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("githubverify: kullanıcı bilgisi http %d", resp.StatusCode)
	}

	var out struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("githubverify: kullanıcı yanıtı çözümlenemedi: %w", err)
	}
	if !ValidUsername(out.Login) || out.Login == "" {
		return "", errors.New("githubverify: beklenmeyen kullanıcı adı biçimi")
	}
	return out.Login, nil
}
