//go:build windows

package masaustu

// pty_windows.go — eklentinin terminal arayüzünü AND penceresinin içinde
// göstermek için Windows'un sözde konsolu (ConPTY, Windows 10 1809 ve
// sonrası). Eklenti gerçek bir konsola bağlıymış gibi çalışır: ekranına
// yazdıkları VT dizileri olarak bir borudan bize gelir, arayüzdeki terminal
// (xterm.js) onları çizer; tuşlar öteki borudan eklentiye gider.

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var conptyVar = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find() == nil

type konPTY struct {
	giris *os.File // tuşlar → eklenti
	cikis *os.File // eklentinin ekranı → biz
	islem windows.Handle
	bitti chan struct{}
	kod   uint32

	mu      sync.Mutex
	hpc     windows.Handle
	kapandi bool // sözde konsol kapatıldı (hpc artık geçersiz)
}

func boyut(sutun, satir int) windows.Coord {
	return windows.Coord{X: int16(sinirla(sutun, 2, 1000)), Y: int16(sinirla(satir, 1, 1000))}
}

// ptyBaslat, cmd'yi sutun×satir boyutunda bir sözde konsolda başlatır.
func ptyBaslat(cmd *exec.Cmd, sutun, satir int) (pty, error) {
	if !conptyVar {
		return nil, errPTYYok
	}
	s, err := surecHazirla(cmd)
	if err != nil {
		return nil, err
	}
	var girisOku, girisYaz, cikisOku, cikisYaz windows.Handle
	if err := windows.CreatePipe(&girisOku, &girisYaz, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&cikisOku, &cikisYaz, nil, 0); err != nil {
		windows.CloseHandle(girisOku)
		windows.CloseHandle(girisYaz)
		return nil, err
	}
	var hpc windows.Handle
	err = windows.CreatePseudoConsole(boyut(sutun, satir), girisOku, cikisYaz, 0, &hpc)
	// Sözde konsol, boruların kendi uçlarını çoğalttı; bizdekiler kapatılır.
	// (Açık kalırsa eklenti çıkınca çıkış borusu EOF vermez.)
	windows.CloseHandle(girisOku)
	windows.CloseHandle(cikisYaz)
	if err != nil {
		windows.CloseHandle(girisYaz)
		windows.CloseHandle(cikisOku)
		return nil, err
	}
	p := &konPTY{
		hpc:   hpc,
		giris: os.NewFile(uintptr(girisYaz), "pty-giris"),
		cikis: os.NewFile(uintptr(cikisOku), "pty-cikis"),
		bitti: make(chan struct{}),
	}
	if err := p.baslat(s); err != nil {
		windows.ClosePseudoConsole(hpc)
		p.giris.Close()
		p.cikis.Close()
		return nil, err
	}
	go p.bekle()
	return p, nil
}

func (p *konPTY) baslat(s surec) error {
	nitelikler, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer nitelikler.Delete()
	// Nitelik, HPCON'un adresi değil kendisidir. Değer bellekten yeniden
	// yorumlanır (uintptr'den unsafe.Pointer'a doğrudan çevirmek vet'in
	// uyardığı bir kalıptır); HPCON, Go yığınının dışında bir adrestir.
	if err := nitelikler.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&p.hpc)), unsafe.Sizeof(p.hpc)); err != nil {
		return err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: nitelikler.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Standart tutamaçlar boş bırakılır: eklenti sözde konsolun kendi
	// girdi/çıktısını kullanır, AND'ın konsoluna ya da yönlendirmelerine
	// bağlanmaz.
	si.Flags = windows.STARTF_USESTDHANDLES
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(s.uygulama, s.satir, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, &s.ortam[0], s.dizin, &si.StartupInfo, &pi); err != nil {
		return err
	}
	windows.CloseHandle(pi.Thread)
	p.islem = pi.Process
	return nil
}

// bekle, eklenti çıkınca sözde konsolu kapatır: çıkış borusu kalan
// çıktıdan sonra EOF verir. ClosePseudoConsole, eski Windows sürümlerinde
// çıktı okunana kadar dönmeyebilir; okuma ayrı bir goroutine'de sürer.
func (p *konPTY) bekle() {
	windows.WaitForSingleObject(p.islem, windows.INFINITE)
	windows.GetExitCodeProcess(p.islem, &p.kod)
	p.konsoluKapat()
	close(p.bitti)
}

func (p *konPTY) konsoluKapat() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.kapandi {
		p.kapandi = true
		windows.ClosePseudoConsole(p.hpc)
	}
}

func (p *konPTY) Read(b []byte) (int, error) {
	n, err := p.cikis.Read(b)
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) {
		return n, errPTYBitti
	}
	return n, err
}

func (p *konPTY) Write(b []byte) (int, error) { return p.giris.Write(b) }

func (p *konPTY) Boyutla(sutun, satir int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.kapandi {
		return errPTYBitti
	}
	return windows.ResizePseudoConsole(p.hpc, boyut(sutun, satir))
}

func (p *konPTY) Durdur() error {
	select {
	case <-p.bitti:
		return nil
	default:
	}
	return windows.TerminateProcess(p.islem, 1)
}

func (p *konPTY) Bitti() <-chan struct{} { return p.bitti }

func (p *konPTY) CikisKodu() int { return int(p.kod) }

// Kapat, eklenti çıktıktan sonra tutamaçları bırakır.
func (p *konPTY) Kapat() error {
	<-p.bitti
	p.giris.Close()
	p.cikis.Close()
	return windows.CloseHandle(p.islem)
}
