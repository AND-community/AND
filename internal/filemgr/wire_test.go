package filemgr

// wire_test.go — filemgr_test.go'daki (frame kodlama/durum kalıcılığı) YANINDA,
// gerçek wire protokolünü (SendFile → libp2p stream → handleStream) iki gerçek
// (loopback) libp2p düğümü arasında uçtan uca test eder. Özellikle P1.5
// düzeltmesinin (hiç onay-eklentisi dinlemiyorsa transfer sessizce kabul
// edilmek yerine reddedilir) doğrudan bir regresyon testi yoktu — bu dosya
// o boşluğu kapatır.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/network"

	"github.com/libp2p/go-libp2p/core/peer"
)

func newTestNode(t *testing.T) *network.Node {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	node, err := network.New(id, nil, "/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("network.New: %v", err)
	}
	t.Cleanup(func() { _ = node.Close() })
	return node
}

func connectedPair(t *testing.T) (*network.Node, *network.Node) {
	t.Helper()
	nodeA := newTestNode(t)
	nodeB := newTestNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addrInfoB := peer.AddrInfo{ID: nodeB.Host.ID(), Addrs: nodeB.Host.Addrs()}
	if err := nodeA.Host.Connect(ctx, addrInfoB); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return nodeA, nodeB
}

// TestHandleStream_NoConsentSubscriberRejectsTransfer, P1.5'in ("dinleyen
// eklenti yoksa dosya sessizce kabul edilmesin") doğrudan regresyon testi:
// alıcı tarafta hiç kimse SubscribeConsent() çağırmamışken gönderilen bir
// dosyanın ne diske yazıldığını, ne de bir .part/.state artığı bıraktığını
// doğrular.
func TestHandleStream_NoConsentSubscriberRejectsTransfer(t *testing.T) {
	nodeA, nodeB := connectedPair(t)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "gizli.txt")
	if err := os.WriteFile(srcPath, []byte("bu dosya asla diske yazilmamali"), 0o600); err != nil {
		t.Fatalf("kaynak dosya yazılamadı: %v", err)
	}

	saveDir := t.TempDir()
	brokerA := New(nodeA, t.TempDir())
	brokerB := New(nodeB, saveDir) // brokerB.consentSubs kasıtlı olarak boş bırakıldı

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := brokerA.SendFile(ctx, nodeB.Host.ID().String(), "alice", srcPath)
	if err == nil {
		t.Fatal("hiç onay dinleyicisi yokken SendFile başarılı olmamalıydı")
	}

	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf("saveDir okunamadı: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("saveDir boş olmalıydı, ama şunlar bulundu: %v", names)
	}

	brokerB.mu.Lock()
	pending := len(brokerB.consentPend)
	brokerB.mu.Unlock()
	if pending != 0 {
		t.Fatalf("reddedilen transfer bir consentPend girdisi bırakmış: %d", pending)
	}
}

// TestHandleStream_ConsentAcceptedDeliversFile, kontrol grubu: en az bir onay
// dinleyicisi VAR ve kabul ediyorsa transfer normal şekilde tamamlanmalı —
// üstteki reddetme testinin, aslında hiçbir zaman çalışmayan bozuk bir
// protokolü test etmediğini kanıtlar.
func TestHandleStream_ConsentAcceptedDeliversFile(t *testing.T) {
	nodeA, nodeB := connectedPair(t)

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "merhaba.txt")
	content := []byte("bu dosya kabul edilince yazilmali")
	if err := os.WriteFile(srcPath, content, 0o600); err != nil {
		t.Fatalf("kaynak dosya yazılamadı: %v", err)
	}

	saveDir := t.TempDir()
	brokerA := New(nodeA, t.TempDir())
	brokerB := New(nodeB, saveDir)

	consentCh := brokerB.SubscribeConsent()
	defer brokerB.UnsubscribeConsent(consentCh)
	fileCh := brokerB.Subscribe()
	defer brokerB.Unsubscribe(fileCh)

	go func() {
		select {
		case req := <-consentCh:
			brokerB.RespondConsent(req.TransferID, true)
		case <-time.After(5 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brokerA.SendFile(ctx, nodeB.Host.ID().String(), "alice", srcPath); err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	select {
	case msg := <-fileCh:
		got, err := os.ReadFile(msg.SavePath)
		if err != nil {
			t.Fatalf("teslim edilen dosya okunamadı: %v", err)
		}
		if string(got) != string(content) {
			t.Fatalf("içerik uyuşmuyor: got %q, want %q", got, content)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("dosya mesajı zaman aşımına uğradı")
	}
}
