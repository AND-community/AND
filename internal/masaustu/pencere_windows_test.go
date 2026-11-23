//go:build windows && pencere

package masaustu

// Gerçek bir pencere açan testler; ekranda kısa bir süre pencere belirdiği
// için yalnızca istenince çalışır:
//
//	go test -tags pencere -run TestPencere -v ./internal/masaustu

import (
	"testing"
	"time"
)

var pSendMessageW = user32.NewProc("SendMessageW")

// TestPencere_KucultulmusEtkinlestirme: görev çubuğundan küçültülmüş pencereye
// tıklanınca Windows, pencere henüz küçükken WM_ACTIVATE gönderir. WebView2
// küçültülmüş hâlde odak isteğini "The parameter is incorrect" ile reddeder;
// kütüphanenin Focus'u bu hatada süreci sonlandırıyordu (os.Exit) ve AND
// kendiliğinden kapanıyordu. Süreç kapanırsa bu test de "exit status 1" ile
// biter.
func TestPencere_KucultulmusEtkinlestirme(t *testing.T) {
	pen, err := pencereAc("about:blank", t.TempDir(), [3]uint8{9, 9, 11})
	if err != nil {
		t.Skip("WebView2 penceresi açılamadı: ", err)
	}
	p := pen.(*winPencere)
	defer func() {
		p.kapat()
		<-p.kapandi()
	}()
	h := p.hwnd.Load()

	const swMinimize, swRestore = 6, 9
	pShowWindow.Call(h, swMinimize)
	time.Sleep(500 * time.Millisecond)
	// WA_CLICKACTIVE (2), yüksek kelimede "küçültülmüş" (1).
	pSendMessageW.Call(h, wmActivate, 0x00010002, 0)
	time.Sleep(time.Second)
	pShowWindow.Call(h, swRestore)
	pSendMessageW.Call(h, wmActivate, 2, 0)
	time.Sleep(time.Second)

	if p.hwnd.Load() == 0 {
		t.Fatal("pencere kapandı")
	}
}
