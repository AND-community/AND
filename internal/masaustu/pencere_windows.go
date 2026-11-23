//go:build windows

package masaustu

// pencere_windows.go — AND'ın Windows'taki penceresi: düz bir Win32 penceresi
// ve içine gömülü WebView2 (Windows 10/11 ile gelen Edge bileşeni). C
// derleyicisi gerekmez; WebView2 yükleyicisi saf Go'dur
// (github.com/wailsapp/go-webview2).
//
// WebView2 bir COM bileşenidir: kendi OS iş parçacığında, tek iş parçacıklı
// bir COM bölmesinde (STA) kurulur ve o iş parçacığının mesaj döngüsüyle
// çalışır. Pencere bu yüzden kendi kilitli iş parçacığında açılır; öteki
// goroutine'ler pencereye yalnızca PostMessage ile (kapat) dokunur.

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")
	shell  = windows.NewLazySystemDLL("shell32.dll")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pUpdateWindow                  = user32.NewProc("UpdateWindow")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pLoadImageW                    = user32.NewProc("LoadImageW")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	pGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	pIsIconic                      = user32.NewProc("IsIconic")
	pGetClientRect                 = user32.NewProc("GetClientRect")
	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pDwmSetWindowAttribute         = dwmapi.NewProc("DwmSetWindowAttribute")
	pShellExecuteW                 = shell.NewProc("ShellExecuteW")
)

const (
	wmDestroy       = 0x0002
	wmMove          = 0x0003
	wmSize          = 0x0005
	wmActivate      = 0x0006
	wmClose         = 0x0010
	wmGetMinMaxInfo = 0x0024
	wmDpiChanged    = 0x02E0
	sizeMinimized   = 1 // WM_SIZE wParam: SIZE_MINIMIZED

	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	swShowNormal       = 1
	smCxScreen         = 0
	smCyScreen         = 1
	imageIcon          = 1
	lrDefaultSize      = 0x0040
	lrShared           = 0x8000
	idcArrow           = 32512
	swpNoZOrder        = 0x0004
	swpNoActivate      = 0x0010

	dwmaUseImmersiveDarkMode = 20
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	dpiPerMonitorV2 = ^uintptr(3) // (HANDLE)-4

	// Pencerenin 96 DPI'daki boyutları.
	pencereGenislik  = 1280
	pencereYukseklik = 820
	enAzGenislik     = 860
	enAzYukseklik    = 560
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type nokta struct{ x, y int32 }

type mesaj struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      nokta
	private uint32
}

type dikdortgen struct{ sol, ust, sag, alt int32 }

type minMaxInfo struct {
	reserved, maxSize, maxPosition, minTrackSize, maxTrackSize nokta
}

// winPencere, açık pencerenin durumudur. Windows'ta aynı anda tek pencere
// vardır; pencere işlevi (wndproc) ona etkin değişkeninden ulaşır.
type winPencere struct {
	hwnd     atomic.Uintptr
	chromium *edge.Chromium
	hazir    atomic.Bool // chromium gömüldü; boyutlandırma artık güvenli
	c        chan struct{}
	once     sync.Once
}

var (
	etkin         *winPencere
	sinifKaydi    sync.Once
	sinifHatasi   error
	sinifAdi      = windows.StringToUTF16Ptr("ANDMasaustu")
	pencereIslevi = windows.NewCallback(wndproc)
)

func (p *winPencere) kapandi() <-chan struct{} { return p.c }

func (p *winPencere) kapat() {
	if h := p.hwnd.Load(); h != 0 {
		pPostMessageW.Call(h, wmClose, 0, 0)
	}
}

// pencereAc, pencereyi kendi iş parçacığında açar ve arayüzü (adres)
// yükler. WebView2 kurulu değilse hata döner; çağıran tarayıcıya geçer.
func pencereAc(adres, veriDizini string, zemin [3]uint8) (pencere, error) {
	if etkin != nil {
		return nil, errors.New("masaustu: pencere zaten açık")
	}
	// Gömme sırasında bir hata kütüphanede süreci sonlandırır; bu yüzden
	// WebView2'nin kurulu olduğu önceden denetlenir.
	if surum, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil || surum == "" {
		return nil, fmt.Errorf("WebView2 bulunamadı: %v", err)
	}
	p := &winPencere{c: make(chan struct{})}
	hazir := make(chan error, 1)
	go p.calistir(adres, veriDizini, zemin, hazir)
	if err := <-hazir; err != nil {
		return nil, err
	}
	return p, nil
}

func (p *winPencere) calistir(adres, veriDizini string, zemin [3]uint8, hazir chan<- error) {
	runtime.LockOSThread() // iş parçacığı goroutine'le birlikte sonlanır
	defer p.once.Do(func() { close(p.c) })

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil &&
		err != syscall.Errno(windows.S_FALSE) { // S_FALSE: zaten başlatılmış
		hazir <- fmt.Errorf("COM: %w", err)
		return
	}
	defer windows.CoUninitialize()

	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(dpiPerMonitorV2)
	}
	hwnd, err := pencereYarat()
	if err != nil {
		hazir <- err
		return
	}
	p.hwnd.Store(hwnd)
	etkin = p

	ch := edge.NewChromium()
	ch.DataPath = veriDizini
	ch.SetErrorCallback(func(err error) { log.Printf("masaüstü: WebView2: %v", err) })
	ch.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		log.Printf("masaüstü: sayfa yüklendi")
	}
	ch.ProcessFailedCallback = func(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		tur, _ := args.GetProcessFailedKind()
		log.Printf("masaüstü: WebView2 süreci çöktü (tür %d)", tur)
	}
	if !ch.Embed(hwnd) {
		pDestroyWindow.Call(hwnd)
		hazir <- errors.New("WebView2 gömülemedi")
		return
	}
	p.chromium = ch
	if ayar, err := ch.GetSettings(); err == nil {
		_ = ayar.PutAreDevToolsEnabled(false)
		_ = ayar.PutIsStatusBarEnabled(false)
		_ = ayar.PutAreHostObjectsAllowed(false)
		_ = ayar.PutIsWebMessageEnabled(false)
	}
	ch.SetBackgroundColour(zemin[0], zemin[1], zemin[2], 255)
	p.hazir.Store(true)
	p.boyutla(hwnd)
	if err := ch.Show(); err != nil {
		log.Printf("masaüstü: WebView2 gösterilemedi: %v", err)
	}
	ch.Navigate(adres)

	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)
	pSetForegroundWindow.Call(hwnd)
	hazir <- nil

	var m mesaj
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0: WM_QUIT, -1: hata
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	p.hwnd.Store(0)
}

// Kütüphanenin Chromium.Resize ve Chromium.Focus işlevleri bir hata
// aldıklarında süreci sonlandırır (os.Exit). Pencere açıldıktan sonra
// onların yerine bu ikisi kullanılır: hata yalnızca günlüğe yazılır ve
// küçültülmüş pencerede (boyutu sıfırken) WebView2'ye hiç dokunulmaz.

func kucuk(hwnd uintptr) bool {
	r, _, _ := pIsIconic.Call(hwnd)
	return r != 0
}

// boyutla, WebView'ı pencerenin istemci alanına yerleştirir.
func (p *winPencere) boyutla(hwnd uintptr) {
	ctrl := p.chromium.GetController()
	if ctrl == nil || kucuk(hwnd) {
		return
	}
	var r edge.Rect
	if ok, _, _ := pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return
	}
	if err := ctrl.PutBounds(r); err != nil {
		log.Printf("masaüstü: WebView2 boyutlandırılamadı: %v", err)
	}
}

// odakla, klavye odağını WebView'a verir.
func (p *winPencere) odakla(hwnd uintptr) {
	ctrl := p.chromium.GetController()
	if ctrl == nil || kucuk(hwnd) {
		return
	}
	if err := ctrl.MoveFocus(edge.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC); err != nil {
		log.Printf("masaüstü: WebView2 odaklanamadı: %v", err)
	}
}

func olcekle(deger int32, dpi uintptr) int32 {
	if dpi == 0 {
		dpi = 96
	}
	return int32(int64(deger) * int64(dpi) / 96)
}

func pencereYarat() (uintptr, error) {
	var ornek windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &ornek); err != nil {
		return 0, err
	}
	sinifKaydi.Do(func() {
		// AND'ın simgesi (kaynak kimliği 1; bkz. cmd/and/winres) yoksa
		// Windows'un varsayılan uygulama simgesi kullanılır.
		simge, _, _ := pLoadImageW.Call(uintptr(ornek), 1, imageIcon, 0, 0, lrDefaultSize|lrShared)
		imlec, _, _ := pLoadCursorW.Call(0, idcArrow)
		wc := wndClassEx{
			lpfnWndProc:   pencereIslevi,
			hInstance:     ornek,
			hIcon:         windows.Handle(simge),
			hIconSm:       windows.Handle(simge),
			hCursor:       windows.Handle(imlec),
			lpszClassName: sinifAdi,
		}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			sinifHatasi = fmt.Errorf("RegisterClassExW: %w", err)
		}
	})
	if sinifHatasi != nil {
		return 0, sinifHatasi
	}

	var dpi uintptr = 96
	if pGetDpiForSystem.Find() == nil {
		dpi, _, _ = pGetDpiForSystem.Call()
	}
	g, y := olcekle(pencereGenislik, dpi), olcekle(pencereYukseklik, dpi)
	ekranG, _, _ := pGetSystemMetrics.Call(smCxScreen)
	ekranY, _, _ := pGetSystemMetrics.Call(smCyScreen)
	g = min(g, int32(ekranG)*9/10)
	y = min(y, int32(ekranY)*9/10)
	x, ust := (int32(ekranG)-g)/2, (int32(ekranY)-y)/2

	baslik := windows.StringToUTF16Ptr("AND")
	hwnd, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(sinifAdi)), uintptr(unsafe.Pointer(baslik)),
		wsOverlappedWindow, uintptr(x), uintptr(ust), uintptr(g), uintptr(y), 0, 0, uintptr(ornek), 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", err)
	}
	// Windows 11'de başlık çubuğu arayüzün koyu zeminine uyar (eski
	// sürümlerde yok sayılır).
	if pDwmSetWindowAttribute.Find() == nil {
		koyu := int32(1)
		pDwmSetWindowAttribute.Call(hwnd, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&koyu)), unsafe.Sizeof(koyu))
	}
	return hwnd, nil
}

func wndproc(hwnd, msg, wp, lp uintptr) uintptr {
	p := etkin
	switch msg {
	case wmSize:
		if p != nil && p.hazir.Load() {
			p.boyutla(hwnd)
			// Görev çubuğundan geri açılırken odak, pencere küçükken
			// verilemedi (bkz. wmActivate); pencere büyüyünce verilir.
			if wp != sizeMinimized {
				if on, _, _ := pGetForegroundWindow.Call(); on == hwnd {
					p.odakla(hwnd)
				}
			}
		}
	case wmMove:
		if p != nil && p.hazir.Load() {
			_ = p.chromium.NotifyParentWindowPositionChanged()
		}
	case wmActivate:
		// Yüksek kelime, pencerenin küçültülmüş olduğunu söyler: görev
		// çubuğundan tıklanınca Windows pencereyi küçükken etkinleştirir.
		if p != nil && p.hazir.Load() && wp&0xFFFF != 0 && wp>>16&0xFFFF == 0 {
			p.odakla(hwnd)
		}
	case wmGetMinMaxInfo:
		var dpi uintptr = 96
		if pGetDpiForWindow.Find() == nil {
			dpi, _, _ = pGetDpiForWindow.Call(hwnd)
		}
		mmi := *(**minMaxInfo)(unsafe.Pointer(&lp)) // lp, Windows'un verdiği MINMAXINFO adresi
		mmi.minTrackSize = nokta{olcekle(enAzGenislik, dpi), olcekle(enAzYukseklik, dpi)}
		return 0
	case wmDpiChanged:
		r := *(**dikdortgen)(unsafe.Pointer(&lp)) // önerilen yeni pencere dikdörtgeni
		pSetWindowPos.Call(hwnd, 0, uintptr(r.sol), uintptr(r.ust), uintptr(r.sag-r.sol), uintptr(r.alt-r.ust),
			swpNoZOrder|swpNoActivate)
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// tarayicidaAc, adresi varsayılan tarayıcıda açar.
func tarayicidaAc(adres string) error {
	islem := windows.StringToUTF16Ptr("open")
	hedef, err := windows.UTF16PtrFromString(adres)
	if err != nil {
		return err
	}
	r, _, _ := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(islem)), uintptr(unsafe.Pointer(hedef)), 0, 0, swShowNormal)
	if r <= 32 {
		return fmt.Errorf("ShellExecuteW: %d", r)
	}
	return nil
}
