package tui

// paylasilan.go — terminal arayüzünün masaüstü arayüzüyle (internal/masaustu)
// paylaştığı, ekrana bağlı olmayan parçalar: kurallar onayı, gizlilik ve
// kural maddeleri, kimlik özeti ve kayıtlı tema. İki arayüz de aynı dosyaları
// ve aynı çevrilmiş metinleri kullanır; biri değişince öteki geride kalmaz.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/lucian95511/and/dil"
	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/ui"
)

// KurallarSurumu, gizlilik özeti ile topluluk kurallarının güncel sürümüdür.
const KurallarSurumu = kurallarSurumu

// KurallarOnaylandi, güncel kuralların bu veri dizininde onaylanıp
// onaylanmadığıdır.
func KurallarOnaylandi(dataDir string) bool { return kurallarOnaylandi(dataDir) }

// KurallariOnayla, güncel kuralların şimdi onaylandığını kaydeder.
func KurallariOnayla(dataDir string) error { return kurallariOnayla(dataDir, time.Now()) }

// GizlilikMaddeleri, gizlilik özetinin etkin dildeki maddeleridir.
func GizlilikMaddeleri() []string { return cevrilmis(gizlilikMaddeleri) }

// KuralMaddeleri, topluluk kurallarının etkin dildeki maddeleridir.
func KuralMaddeleri() []string { return cevrilmis(kuralMaddeleri) }

func cevrilmis(kaynak []string) []string {
	out := make([]string, len(kaynak))
	for i, s := range kaynak {
		out[i] = dil.T(s)
	}
	return out
}

// SifreGucu, yeni bir şifrenin 0–4 arası gücü ve etkin dildeki etiketidir
// (giriş ekranındaki göstergenin aynısı).
func SifreGucu(s string) (int, string) { return sifreGucu(s) }

// KimlikOzetiYaz, bu cihazda açılan kimliğin genel anahtarını not eder (bkz.
// kimlikOzetiYaz); her iki arayüz de girişten sonra çağırır.
func KimlikOzetiYaz(dataDir string, id *stdcrypto.Identity) { kimlikOzetiYaz(dataDir, id) }

// TemaOku, veri dizinindeki kayıtlı temayı döndürür; kayıt yoksa ya da
// okunamıyorsa varsayılan tema (Klasik) döner.
func TemaOku(dataDir string) ui.Theme {
	if dataDir != "" {
		if veri, err := os.ReadFile(filepath.Join(dataDir, ThemeFile)); err == nil {
			var t ui.Theme
			if json.Unmarshal(veri, &t) == nil {
				return ui.Normalize(t)
			}
		}
	}
	return ui.DefaultTheme()
}

// NeZaman, bir zamanın "3 dk önce" biçimindeki göreli gösterimidir (forum ve
// ağ ekranlarındakiyle aynı).
func NeZaman(t time.Time) string { return forumNeZaman(t) }

// KimlikKarsilastir, kurtarma koduyla geri üretilen bir kimliği bu cihazda
// son açılan kimlikle karşılaştırır: "ayni" (ve o kimliğin adı), "farkli"
// ya da bu cihazda kayıt yoksa "bilinmiyor".
func KimlikKarsilastir(dataDir string, id *stdcrypto.Identity) (durum, ad string) {
	o := kimlikOzetiOku(dataDir)
	switch {
	case o == nil:
		return "bilinmiyor", ""
	case o.ayniMi(id):
		return "ayni", o.Ad
	}
	return "farkli", ""
}

// KisaKimlik, kimliğin Peer ID'sinin karşılaştırmaya yetecek kısaltmasıdır.
func KisaKimlik(id *stdcrypto.Identity) string { return kisaKimlik(id) }
