package dmmgr

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/internal/pluginapi"
)

func testAnahtari(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	k, err := GecmisAnahtari(priv)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKalici_GecmisYenidenAcilincaGelirVeSifreli(t *testing.T) {
	yol := filepath.Join(t.TempDir(), "dm_gecmis.bin")
	anahtar := testAnahtari(t)

	b1 := New(newTestNode(t))
	if err := b1.KaliciAc(yol, anahtar); err != nil {
		t.Fatal(err)
	}
	b1.record(pluginapi.DMRecord{PeerID: "12D3KooWBob", Name: "bob", Text: "gizli selam", At: time.Now().UTC()})
	b1.MarkRead("12D3KooWBob", time.Now().UTC().Add(-time.Hour))

	ham, err := os.ReadFile(yol)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ham, []byte("gizli selam")) || bytes.Contains(ham, []byte("12D3KooWBob")) {
		t.Fatal("dosyada açık metin olmamalı")
	}
	if fi, _ := os.Stat(yol); fi != nil && fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("dosya yalnızca sahibine açık olmalı: %v", fi.Mode())
	}

	b2 := New(newTestNode(t))
	if err := b2.KaliciAc(yol, anahtar); err != nil {
		t.Fatal(err)
	}
	g := b2.History()
	if len(g) != 1 || g[0].Text != "gizli selam" || g[0].ID == "" {
		t.Fatalf("geçmiş yeniden açılınca gelmeli: %+v", g)
	}
	if b2.OkunmamisSayisi() != 1 {
		t.Fatalf("okundu işaretinden sonra gelen mesaj okunmamış sayılmalı: %d", b2.OkunmamisSayisi())
	}
}

func TestKalici_YanlisAnahtarlaUzerineYazilmaz(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, "dm_gecmis.bin")
	b1 := New(newTestNode(t))
	if err := b1.KaliciAc(yol, testAnahtari(t)); err != nil {
		t.Fatal(err)
	}
	b1.record(pluginapi.DMRecord{PeerID: "12D3KooWBob", Text: "eski", At: time.Now()})
	eski, _ := os.ReadFile(yol)

	b2 := New(newTestNode(t))
	if err := b2.KaliciAc(yol, testAnahtari(t)); err == nil {
		t.Fatal("başka anahtarla açılan dosya hata bildirmeli")
	}
	if len(b2.History()) != 0 {
		t.Fatal("çözülemeyen geçmiş yüklenmemeli")
	}
	kenar, err := os.ReadFile(yol + ".okunamadi")
	if err != nil || !bytes.Equal(kenar, eski) {
		t.Fatalf("çözülemeyen dosya silinmemeli, kenara alınmalı: %v", err)
	}
}

func TestGidenKutusu_AliciBaglanincaGonderilir(t *testing.T) {
	a, b := newTestNode(t), newTestNode(t)
	ba := New(a)
	bb := New(b)
	ch := bb.Subscribe()
	defer bb.Unsubscribe(ch)

	// B'nin adresi A'da yok: doğrudan gönderim başarısız olur, mesaj sıraya girer.
	if err := ba.SendDMQueued(context.Background(), b.Host.ID().String(), "ayse", "sonra oku"); err != nil {
		t.Fatalf("ulaşılamayan alıcıya mesaj hata değil sıraya alınmalı: %v", err)
	}
	g := ba.History()
	if len(g) != 1 || !g[0].Pending || !g[0].Outgoing {
		t.Fatalf("mesaj geçmişte bekliyor görünmeli: %+v", g)
	}

	// B bağlanınca giden kutusu boşalır.
	if err := a.Host.Connect(context.Background(), peer.AddrInfo{ID: b.Host.ID(), Addrs: b.Host.Addrs()}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m.Text != "sonra oku" || m.VerifiedPeerID != a.Host.ID().String() {
			t.Fatalf("B'ye ulaşan mesaj beklenmedik: %+v", m)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("alıcı bağlanınca bekleyen mesaj gönderilmeliydi")
	}
	son := time.Now().Add(5 * time.Second)
	for ba.History()[0].Pending {
		if time.Now().After(son) {
			t.Fatal("gönderilen mesaj artık bekliyor görünmemeli")
		}
		time.Sleep(20 * time.Millisecond)
	}
	ba.mu.Lock()
	kalan := len(ba.giden)
	ba.mu.Unlock()
	if kalan != 0 {
		t.Fatalf("giden kutusu boşalmalı, %d mesaj kaldı", kalan)
	}
}

func TestGidenKutusu_SuresiDolanGonderilemediOlur(t *testing.T) {
	b := New(newTestNode(t))
	b.giden = []bekleyenDM{{ID: "x1", PeerID: "12D3KooWBob", Text: "eski", At: time.Now().Add(-gidenOmru - time.Minute)}}
	b.history = []pluginapi.DMRecord{{ID: "x1", PeerID: "12D3KooWBob", Text: "eski", Outgoing: true, Pending: true}}
	b.suresiDolanlariAyikla(time.Now())
	if len(b.giden) != 0 || !b.history[0].Failed || b.history[0].Pending {
		t.Fatalf("süresi dolan mesaj gönderilemedi olarak işaretlenmeli: %+v", b.history[0])
	}
}

func TestGidenKutusu_DoluysaReddeder(t *testing.T) {
	b := New(newTestNode(t))
	for i := 0; i < maxGiden; i++ {
		b.giden = append(b.giden, bekleyenDM{ID: yeniID(), PeerID: "12D3KooWBob", At: time.Now()})
	}
	hedef := newTestNode(t).Host.ID().String()
	if err := b.SendDMQueued(context.Background(), hedef, "ben", "bir tane daha"); err == nil {
		t.Fatal("dolu giden kutusu yeni mesajı reddetmeli")
	}
}

func TestOkundu(t *testing.T) {
	b := New(newTestNode(t))
	t0 := time.Now().UTC()
	b.record(pluginapi.DMRecord{PeerID: "p1", Text: "a", At: t0})
	b.record(pluginapi.DMRecord{PeerID: "p1", Text: "b", At: t0.Add(time.Minute)})
	b.record(pluginapi.DMRecord{PeerID: "p2", Text: "c", At: t0})
	b.record(pluginapi.DMRecord{PeerID: "p2", Text: "giden", At: t0.Add(time.Hour), Outgoing: true})
	if n := b.OkunmamisSayisi(); n != 3 {
		t.Fatalf("giden mesajlar sayılmadan 3 okunmamış olmalı: %d", n)
	}
	b.MarkRead("p1", t0)
	b.MarkRead("p1", t0.Add(-time.Hour)) // daha eski zaman geri almaz
	if n := b.OkunmamisSayisi(); n != 2 {
		t.Fatalf("p1'in ilk mesajı okunmuş sayılmalı: %d", n)
	}
	if got := b.ReadMarks()["p1"]; !got.Equal(t0) {
		t.Fatalf("okundu zamanı geri gitmemeli: %v", got)
	}
}
