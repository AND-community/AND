package dmmgr

// kalici.go — DM geçmişinin diskte şifreli saklanması, giden kutusu ve
// konuşma başına okundu bilgisi.
//
// Geçmiş, giden kutusu ve okundu bilgisi tek bir dosyada, kimliğin özel
// anahtarından HKDF ile türetilen bir anahtarla AES-256-GCM şifreli durur.
// Kimlik dosyası zaten parolayla korunduğu için geçmiş de ancak AND'a giriş
// yapılabildiğinde okunabilir. KaliciAc çağrılmazsa her şey yalnızca
// bellektedir (eski davranış).

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/internal/pluginapi"
)

const (
	// gidenOmru, giden kutusundaki bir mesajın alıcının bağlanmasını en fazla
	// ne kadar bekleyeceğidir; sonra "gönderilemedi" olarak işaretlenir.
	gidenOmru = 7 * 24 * time.Hour
	// maxGiden, giden kutusundaki en fazla mesaj sayısıdır.
	maxGiden = 200
	// ilkDenemeSuresi, sıraya almadan önce doğrudan göndermenin en fazla ne
	// kadar deneneceğidir.
	ilkDenemeSuresi = 10 * time.Second

	dosyaSurumu = 1
	// ekVeri, hem anahtar türetmede hem şifrelemede (AAD) kullanılır; dosya
	// biçimi değişirse sürüm artar.
	ekVeri = "AND dm geçmişi v1"
)

// bekleyenDM, giden kutusunda alıcının bağlanmasını bekleyen bir mesajdır.
type bekleyenDM struct {
	ID     string    `json:"id"`
	PeerID string    `json:"peer_id"`
	Ad     string    `json:"ad"` // gönderilirken kullanılacak kendi görünen adın
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
}

type kayitDosyasi struct {
	Surum  int                  `json:"surum"`
	Gecmis []pluginapi.DMRecord `json:"gecmis"`
	Giden  []bekleyenDM         `json:"giden,omitempty"`
	Okundu map[string]time.Time `json:"okundu,omitempty"`
}

// GecmisAnahtari, kimliğin özel anahtarından DM geçmişinin şifreleme
// anahtarını türetir.
func GecmisAnahtari(priv ed25519.PrivateKey) ([]byte, error) {
	return hkdf.Key(sha256.New, priv.Seed(), nil, ekVeri, 32)
}

// KaliciAc, geçmişi, giden kutusunu ve okundu bilgisini yol'daki şifreli
// dosyaya bağlar: varsa yükler, bundan sonraki her değişiklikte yazar. Dosya
// bu anahtarla çözülemezse (başka bir kimliğin ya da bozuk dosya) üzerine
// yazılmaz, yanına ".okunamadi" uzantısıyla taşınır; geçmiş boş başlar ve
// dönen hata bunu bildirir (kalıcılık yine de açıktır).
func (b *Broker) KaliciAc(yol string, anahtar []byte) error {
	if len(anahtar) != 32 {
		return errors.New("dmmgr: geçmiş anahtarı 32 bayt olmalı")
	}
	var dosya kayitDosyasi
	var yukleHatasi error
	veri, err := os.ReadFile(yol)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("dmmgr: geçmiş okunamadı: %w", err)
	default:
		acik, err := coz(anahtar, veri)
		if err == nil {
			err = json.Unmarshal(acik, &dosya)
		}
		if err != nil {
			yukleHatasi = fmt.Errorf("dmmgr: önceki DM geçmişi çözülemedi, %s.okunamadi olarak kenara alındı: %w", yol, err)
			_ = os.Rename(yol, yol+".okunamadi")
			dosya = kayitDosyasi{}
		}
	}

	b.mu.Lock()
	b.kalici, b.anahtar = yol, anahtar
	b.history = append(dosya.Gecmis, b.history...)
	if over := len(b.history) - historyLimit; over > 0 {
		b.history = append(b.history[:0], b.history[over:]...)
	}
	b.giden = append(dosya.Giden, b.giden...)
	if b.okundu == nil {
		b.okundu = map[string]time.Time{}
	}
	for p, t := range dosya.Okundu {
		if t.After(b.okundu[p]) {
			b.okundu[p] = t
		}
	}
	b.mu.Unlock()

	b.suresiDolanlariAyikla(time.Now())
	b.kaydet()
	return yukleHatasi
}

// kaydet, durumu dosyaya yazar. kayitMu, anlık görüntünün alınmasını da
// kapsar: önce alınan eski bir görüntü sonra alınan yenisinin üzerine
// yazamaz.
func (b *Broker) kaydet() {
	b.kayitMu.Lock()
	defer b.kayitMu.Unlock()

	b.mu.Lock()
	if b.kalici == "" {
		b.mu.Unlock()
		return
	}
	dosya := kayitDosyasi{
		Surum:  dosyaSurumu,
		Gecmis: append([]pluginapi.DMRecord(nil), b.history...),
		Giden:  append([]bekleyenDM(nil), b.giden...),
		Okundu: maps.Clone(b.okundu),
	}
	yol, anahtar := b.kalici, b.anahtar
	b.mu.Unlock()

	acik, err := json.Marshal(dosya)
	if err != nil {
		log.Printf("dmmgr: geçmiş kodlanamadı: %v", err)
		return
	}
	kapali, err := sifrele(anahtar, acik)
	if err != nil {
		log.Printf("dmmgr: geçmiş şifrelenemedi: %v", err)
		return
	}
	gecici := yol + ".yeni"
	if err := os.WriteFile(gecici, kapali, 0o600); err != nil {
		log.Printf("dmmgr: geçmiş yazılamadı: %v", err)
		return
	}
	if err := os.Rename(gecici, yol); err != nil {
		os.Remove(gecici)
		log.Printf("dmmgr: geçmiş yazılamadı: %v", err)
	}
}

func gcm(anahtar []byte) (cipher.AEAD, error) {
	blok, err := aes.NewCipher(anahtar)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blok)
}

func sifrele(anahtar, acik []byte) ([]byte, error) {
	a, err := gcm(anahtar)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, acik, []byte(ekVeri)), nil
}

func coz(anahtar, kapali []byte) ([]byte, error) {
	a, err := gcm(anahtar)
	if err != nil {
		return nil, err
	}
	if len(kapali) < a.NonceSize() {
		return nil, errors.New("dosya çok kısa")
	}
	return a.Open(nil, kapali[:a.NonceSize()], kapali[a.NonceSize():], []byte(ekVeri))
}

func yeniID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SendDMQueued, mesajı hemen göndermeyi dener; alıcıya ulaşılamazsa giden
// kutusuna alır (pluginapi.DMQueueBackend). Mesaj geçmişte Pending olarak
// görünür ve alıcı bağlandığında gönderilir.
func (b *Broker) SendDMQueued(ctx context.Context, peerIDStr, senderName, message string) error {
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("geçersiz peer ID: %w", err)
	}
	if strings.TrimSpace(message) == "" {
		return nil
	}
	denemeCtx, cancel := context.WithTimeout(ctx, ilkDenemeSuresi)
	err = b.SendDM(denemeCtx, peerIDStr, senderName, message)
	cancel()
	if err == nil {
		return nil
	}

	b.mu.Lock()
	if len(b.giden) >= maxGiden {
		b.mu.Unlock()
		return fmt.Errorf("giden kutusu dolu (%d mesaj); alıcılar bağlandıkça boşalır", maxGiden)
	}
	id, now := yeniID(), time.Now().UTC()
	b.giden = append(b.giden, bekleyenDM{ID: id, PeerID: pid.String(), Ad: senderName, Text: message, At: now})
	b.mu.Unlock()
	b.record(pluginapi.DMRecord{ID: id, PeerID: pid.String(), Text: message, At: now, Outgoing: true, Pending: true})
	return nil
}

// baglandi, bir eş bağlandığında çağrılır: ona giden kutusunda mesaj varsa
// arka planda gönderilir. Aynı eşe aynı anda tek gönderim yapılır.
func (b *Broker) baglandi(pid peer.ID) {
	b.mu.Lock()
	if b.gonderiliyor[pid] || !b.bekleyenVarLocked(pid) {
		b.mu.Unlock()
		return
	}
	if b.gonderiliyor == nil {
		b.gonderiliyor = map[peer.ID]bool{}
	}
	b.gonderiliyor[pid] = true
	b.mu.Unlock()

	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.gonderiliyor, pid)
			b.mu.Unlock()
		}()
		b.kuyruguGonder(pid)
	}()
}

func (b *Broker) bekleyenVarLocked(pid peer.ID) bool {
	for _, d := range b.giden {
		if d.PeerID == pid.String() {
			return true
		}
	}
	return false
}

// kuyruguGonder, pid'e bekleyen mesajları sırayla gönderir; biri
// gönderilemezse kalanlar bir sonraki bağlantıya kalır.
func (b *Broker) kuyruguGonder(pid peer.ID) {
	b.suresiDolanlariAyikla(time.Now())
	for {
		b.mu.Lock()
		var d *bekleyenDM
		for i := range b.giden {
			if b.giden[i].PeerID == pid.String() {
				kopya := b.giden[i]
				d = &kopya
				break
			}
		}
		b.mu.Unlock()
		if d == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := b.gonder(ctx, pid, d.Ad, d.Text, d.At)
		cancel()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.gidenCikarLocked(d.ID)
		b.kayitGuncelleLocked(d.ID, func(r *pluginapi.DMRecord) { r.Pending = false })
		b.mu.Unlock()
		b.kaydet()
	}
}

// suresiDolanlariAyikla, gidenOmru'nu aşan bekleyen mesajları giden
// kutusundan çıkarır ve geçmişte "gönderilemedi" olarak işaretler.
func (b *Broker) suresiDolanlariAyikla(now time.Time) {
	b.mu.Lock()
	degisti := false
	kalan := b.giden[:0]
	for _, d := range b.giden {
		if now.Sub(d.At) > gidenOmru {
			b.kayitGuncelleLocked(d.ID, func(r *pluginapi.DMRecord) { r.Pending, r.Failed = false, true })
			degisti = true
			continue
		}
		kalan = append(kalan, d)
	}
	b.giden = kalan
	b.mu.Unlock()
	if degisti {
		b.kaydet()
	}
}

func (b *Broker) gidenCikarLocked(id string) {
	for i, d := range b.giden {
		if d.ID == id {
			b.giden = append(b.giden[:i], b.giden[i+1:]...)
			return
		}
	}
}

func (b *Broker) kayitGuncelleLocked(id string, f func(*pluginapi.DMRecord)) {
	for i := range b.history {
		if b.history[i].ID == id {
			f(&b.history[i])
			return
		}
	}
}

// MarkRead, peerID ile konuşmanın at'a kadar okunduğunu kaydeder
// (pluginapi.DMReadBackend). Daha eski bir zaman bilgiyi geri almaz.
func (b *Broker) MarkRead(peerID string, at time.Time) {
	b.mu.Lock()
	if b.okundu == nil {
		b.okundu = map[string]time.Time{}
	}
	if !at.After(b.okundu[peerID]) {
		b.mu.Unlock()
		return
	}
	b.okundu[peerID] = at.UTC()
	b.mu.Unlock()
	b.kaydet()
}

// ReadMarks, konuşma başına okundu zamanlarının bir kopyasıdır.
func (b *Broker) ReadMarks() map[string]time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return maps.Clone(b.okundu)
}

// OkunmamisSayisi, okundu işaretinden sonra gelen mesaj sayısıdır (ana
// menüdeki rozet).
func (b *Broker) OkunmamisSayisi() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, r := range b.history {
		if !r.Outgoing && r.At.After(b.okundu[r.PeerID]) {
			n++
		}
	}
	return n
}
