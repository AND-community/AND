package masaustu

// giris.go — kimliğin açılması ya da ilk kez oluşturulması. Akış terminal
// arayüzününkiyle (internal/tui/login.go) aynıdır ve aynı denetimleri yapar:
//
//   - Kimlik dosyası varsa: şifre → (kimliğin adı yoksa) görünen ad.
//   - Yoksa: ad, şifre ve tekrarı → kurtarma kodu gösterilir → kullanıcı
//     kodu yeniden yazarak not ettiğini kanıtlar → kimlik şifreyle kaydedilir.
//   - Şifre unutulduysa ya da yeni bir cihazdaysa: kurtarma kodu → ad ve yeni
//     şifre; varolan kimlik dosyası silinmez, kenara alınır (bkz.
//     tui/geri_yukle.go).
//
// Şifre ve henüz kaydedilmemiş kimlik yalnızca bu süreçte, akış bitene dek
// tutulur.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/tui"
)

// Giriş aşamaları (arayüz bunlara göre ekran seçer).
const (
	asamaGiris    = "giris"     // şifreyle aç
	asamaKayit    = "kayit"     // yeni kimlik: ad ve şifre
	asamaKod      = "kod"       // yeni kimlik: kurtarma kodunu göster ve doğrulat
	asamaAd       = "ad"        // açılan kimliğin adı yok: ad iste
	asamaGeriKod  = "geri-kod"  // kurtarma koduyla geri yükleme: kod
	asamaGeriForm = "geri-form" // kurtarma koduyla geri yükleme: ad ve yeni şifre
	asamaBasliyor = "basliyor"  // kimlik açıldı, çekirdek kuruluyor
)

type giris struct {
	mu     sync.Mutex
	dosya  string
	asama  string
	kimlik *stdcrypto.Identity // açılan ya da üretilen, henüz teslim edilmemiş kimlik
	sifre  string
	kod    string // gösterilen kurtarma kodu
	sonuc  chan *stdcrypto.Identity

	// Geri yükleme: dönülecek önceki aşama, kimliğin bu cihazda son
	// açılanla karşılaştırması ve farklıysa uyarının gösterildiği.
	onceki   string
	eslesme  string
	uyarildi bool
}

func yeniGiris(dosya string) *giris {
	g := &giris{dosya: dosya, asama: asamaKayit, sonuc: make(chan *stdcrypto.Identity, 1)}
	if kimlikVar(dosya) {
		g.asama = asamaGiris
	}
	return g
}

// teslim, açılan kimliği bekleyen Giris'e verir; akışın gizli verileri silinir.
func (g *giris) teslim() {
	id := g.kimlik
	g.kimlik, g.sifre, g.kod = nil, "", ""
	g.asama = asamaBasliyor
	g.sonuc <- id
}

func (g *giris) durum() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.asama
}

func (u *Uygulama) girisAc(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Sifre string `json:"sifre"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaGiris {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	id, err := stdcrypto.LoadEncrypted(g.dosya, istek.Sifre)
	if err != nil {
		if errors.Is(err, stdcrypto.ErrWrongPassphrase) {
			hataYaz(w, http.StatusUnauthorized, dil.T("şifre yanlış, tekrar dene"))
			return
		}
		hataYaz(w, http.StatusInternalServerError, fmt.Errorf(dil.T("kimlik dosyası açılamadı: %w"), err).Error())
		return
	}
	g.kimlik, g.sifre = id, istek.Sifre
	if id.Name() == "" {
		g.asama = asamaAd
	} else {
		g.teslim()
	}
	jsonYaz(w, map[string]string{"asama": g.asama})
}

func (u *Uygulama) kayitOl(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Ad     string `json:"ad"`
		Sifre  string `json:"sifre"`
		Tekrar string `json:"tekrar"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaKayit {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	ad := temiz(istek.Ad)
	switch {
	case ad == "":
		hataYaz(w, http.StatusBadRequest, dil.T("görünen ad boş olamaz"))
		return
	case len([]rune(ad)) > 32:
		hataYaz(w, http.StatusBadRequest, dil.T("görünen ad en çok 32 karakter olabilir"))
		return
	case istek.Sifre == "":
		hataYaz(w, http.StatusBadRequest, dil.T("şifre boş olamaz"))
		return
	case len([]rune(istek.Sifre)) < 8:
		hataYaz(w, http.StatusBadRequest, dil.T("şifre en az 8 karakter olmalı"))
		return
	case istek.Sifre != istek.Tekrar:
		hataYaz(w, http.StatusBadRequest, dil.T("şifreler eşleşmiyor"))
		return
	}
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		hataYaz(w, http.StatusInternalServerError, err.Error())
		return
	}
	id.SetName(ad)
	g.kimlik, g.sifre, g.kod = id, istek.Sifre, id.RecoveryCode()
	g.asama = asamaKod
	jsonYaz(w, map[string]string{"asama": g.asama, "kod": g.kod})
}

// kayitKodu, kurtarma kodu ekranı yeniden yüklenirse kodu yeniden verir.
func (u *Uygulama) kayitKodu(w http.ResponseWriter, _ *http.Request) {
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaKod {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	jsonYaz(w, map[string]string{"kod": g.kod})
}

// kodNormal, kurtarma kodunu karşılaştırma için sadeleştirir (tire, boşluk
// ve büyük/küçük harf önemsizdir; bkz. tui/login.go).
func kodNormal(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func (u *Uygulama) kayitDogrula(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Kod string `json:"kod"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaKod {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	if kodNormal(istek.Kod) != kodNormal(g.kod) {
		hataYaz(w, http.StatusBadRequest, dil.T("yanlış kelime — kurtarma kodunu tekrar kontrol et"))
		return
	}
	if err := g.kimlik.SaveEncrypted(g.dosya, g.sifre); err != nil {
		hataYaz(w, http.StatusInternalServerError, fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err).Error())
		return
	}
	g.teslim()
	jsonYaz(w, map[string]string{"asama": g.asama})
}

// kayitGeri, kurtarma kodu ekranından forma döner; kod hiçbir yere
// kaydedilmediği için yeniden gönderilen formda yeni bir kimlik üretilir.
func (u *Uygulama) kayitGeri(w http.ResponseWriter, _ *http.Request) {
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama == asamaKod {
		g.kimlik, g.sifre, g.kod = nil, "", ""
		g.asama = asamaKayit
	}
	jsonYaz(w, map[string]string{"asama": g.asama})
}

func (u *Uygulama) adVer(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Ad   string `json:"ad"`
		Atla bool   `json:"atla"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	g := u.giris
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.asama != asamaAd {
		hataYaz(w, http.StatusConflict, "")
		return
	}
	if !istek.Atla {
		ad := temiz(istek.Ad)
		if ad == "" {
			hataYaz(w, http.StatusBadRequest, dil.T("görünen ad boş olamaz"))
			return
		}
		if len([]rune(ad)) > 32 {
			hataYaz(w, http.StatusBadRequest, dil.T("görünen ad en çok 32 karakter olabilir"))
			return
		}
		g.kimlik.SetName(ad)
		if err := g.kimlik.SaveEncrypted(g.dosya, g.sifre); err != nil {
			hataYaz(w, http.StatusInternalServerError, fmt.Errorf(dil.T("kimlik kaydedilemedi: %w"), err).Error())
			return
		}
	}
	g.teslim()
	jsonYaz(w, map[string]string{"asama": g.asama})
}

func (u *Uygulama) sifreGucu(w http.ResponseWriter, r *http.Request) {
	var istek struct {
		Sifre string `json:"sifre"`
	}
	if jsonOku(r, &istek) != nil {
		hataYaz(w, http.StatusBadRequest, "")
		return
	}
	puan, etiket := tui.SifreGucu(istek.Sifre)
	jsonYaz(w, map[string]any{"puan": puan, "etiket": etiket})
}
