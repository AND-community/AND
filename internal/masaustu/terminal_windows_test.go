//go:build windows

package masaustu

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPTYCocuk, sözde terminal testlerinin çocuk sürecidir: "hazır" yazar,
// bir satır okur, onu geri yazar ve 3 koduyla çıkar. "bekle" verilirse
// hiç çıkmaz.
func TestPTYCocuk(t *testing.T) {
	if os.Getenv("AND_PTY_COCUK") == "" {
		t.Skip("yalnızca sözde terminal testlerinin çocuğu olarak çalışır")
	}
	fmt.Println("hazır")
	if os.Getenv("AND_PTY_COCUK") == "bekle" {
		select {}
	}
	satir, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Printf("yankı: %s\n", strings.TrimSpace(satir))
	os.Exit(3)
}

func cocuk(tur string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestPTYCocuk$")
	cmd.Env = append(os.Environ(), "AND_PTY_COCUK="+tur)
	return cmd
}

// cikti, bir sözde terminalin çıktısını arka planda toplar.
type cikti struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *cikti) topla(p pty) {
	tampon := make([]byte, 4096)
	for {
		n, err := p.Read(tampon)
		c.mu.Lock()
		c.b.Write(tampon[:n])
		c.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (c *cikti) bekle(t *testing.T, aranan string) {
	t.Helper()
	son := time.Now().Add(15 * time.Second)
	for time.Now().Before(son) {
		c.mu.Lock()
		s := c.b.String()
		c.mu.Unlock()
		if strings.Contains(s, aranan) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.Fatalf("%q gelmedi; çıktı: %q", aranan, c.b.String())
}

func TestPTY_GirdiCiktiVeCikisKodu(t *testing.T) {
	p, err := ptyBaslat(cocuk("yanki"), 80, 25)
	if err != nil {
		t.Fatal(err)
	}
	var c cikti
	go c.topla(p)
	c.bekle(t, "hazır")
	if err := p.Boyutla(100, 30); err != nil {
		t.Errorf("boyutlama: %v", err)
	}
	if _, err := p.Write([]byte("merhaba dünya\r")); err != nil {
		t.Fatal(err)
	}
	c.bekle(t, "yankı: merhaba dünya")
	select {
	case <-p.Bitti():
	case <-time.After(15 * time.Second):
		t.Fatal("çocuk çıkmadı")
	}
	if k := p.CikisKodu(); k != 3 {
		t.Errorf("çıkış kodu = %d, istenen 3", k)
	}
	if err := p.Kapat(); err != nil {
		t.Error(err)
	}
	if err := p.Boyutla(80, 25); err == nil {
		t.Error("kapanmış sözde terminal boyutlanabildi")
	}
}

func TestPTY_Durdur(t *testing.T) {
	p, err := ptyBaslat(cocuk("bekle"), 80, 25)
	if err != nil {
		t.Fatal(err)
	}
	var c cikti
	go c.topla(p)
	c.bekle(t, "hazır")
	if err := p.Durdur(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Bitti():
	case <-time.After(15 * time.Second):
		t.Fatal("durdurulan çocuk çıkmadı")
	}
	_ = p.Kapat()
}

// Uçtan uca: terminal açılır, çıktısı geçmişten okunur, tuşlar yazılır,
// eklenti çıkınca terminal "bitti" olur ve API anahtarı geri alınır.
func TestTerminal_Uclar(t *testing.T) {
	d := yeniDeneme(t)
	geriAlindi := make(chan struct{})
	tm, err := d.u.term.baslat("deneme", "Deneme", cocuk("yanki"), 80, 25, func() { close(geriAlindi) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.u.term.hepsiniDurdur)
	gecmis := func() map[string]any {
		return jsonCoz(t, d.al("/api/terminal/gecmis?id="+tm.id+"&ofs=0"))
	}
	metni := func(g map[string]any) string {
		b, _ := base64.StdEncoding.DecodeString(g["veri"].(string))
		return string(b)
	}
	bekle := func(kosul func(map[string]any) bool) map[string]any {
		t.Helper()
		son := time.Now().Add(15 * time.Second)
		for {
			g := gecmis()
			if kosul(g) {
				return g
			}
			if time.Now().After(son) {
				t.Fatalf("beklenen durum gelmedi: %q bitti=%v", metni(g), g["bitti"])
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
	bekle(func(g map[string]any) bool { return strings.Contains(metni(g), "hazır") })

	l := jsonCoz(t, d.al("/api/terminal/liste"))["terminaller"].([]any)
	if len(l) != 1 || l[0].(map[string]any)["etiket"] != "Deneme" {
		t.Fatalf("liste = %v", l)
	}
	if w := d.gonder("/api/terminal/boyut", map[string]any{"id": tm.id, "sutun": 90, "satir": 28, "yenile": true}); w.Code != http.StatusOK {
		t.Errorf("boyut = %d", w.Code)
	}
	if w := d.gonder("/api/terminal/yaz", map[string]any{"id": "yok", "veri": "x"}); w.Code != http.StatusNotFound {
		t.Errorf("olmayan terminale yazma = %d", w.Code)
	}
	if w := d.gonder("/api/terminal/yaz", map[string]any{"id": tm.id, "veri": "selam\r"}); w.Code != http.StatusOK {
		t.Fatalf("yaz = %d", w.Code)
	}
	g := bekle(func(g map[string]any) bool { return g["bitti"] == true })
	if !strings.Contains(metni(g), "yankı: selam") || g["kod"] != float64(3) {
		t.Errorf("çıktı %q, kod %v", metni(g), g["kod"])
	}
	select {
	case <-geriAlindi:
	case <-time.After(5 * time.Second):
		t.Error("eklentinin API anahtarı geri alınmadı")
	}
	// Kaldığı yerden okuma: son'dan sonrası boş, kesik değil.
	son := int64(g["son"].(float64))
	g2 := jsonCoz(t, d.al(fmt.Sprintf("/api/terminal/gecmis?id=%s&ofs=%d", tm.id, son)))
	if g2["veri"] != "" || g2["kesik"] != false {
		t.Errorf("sondan okuma = %v", g2)
	}
	if w := d.gonder("/api/terminal/kapat", map[string]any{"id": tm.id}); w.Code != http.StatusOK {
		t.Errorf("kapat = %d", w.Code)
	}
	if l := jsonCoz(t, d.al("/api/terminal/liste"))["terminaller"].([]any); len(l) != 0 {
		t.Errorf("kapatılan terminal listede: %v", l)
	}
}
