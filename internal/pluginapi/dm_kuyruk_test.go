package pluginapi

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// kuyrukluDM, giden kutusu ve okundu bilgisi olan bir DM arka ucudur.
type kuyrukluDM struct {
	mockDM
	siradaki []string
	okundu   map[string]time.Time
}

func (k *kuyrukluDM) SendDMQueued(_ context.Context, peerID, _, message string) error {
	k.siradaki = append(k.siradaki, peerID+":"+message)
	return nil
}
func (k *kuyrukluDM) MarkRead(peerID string, at time.Time) {
	if k.okundu == nil {
		k.okundu = map[string]time.Time{}
	}
	k.okundu[peerID] = at
}
func (k *kuyrukluDM) ReadMarks() map[string]time.Time { return k.okundu }

const gecerliPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

func TestDMSirali_KuyrukArkaUcunaGider(t *testing.T) {
	dm := &kuyrukluDM{}
	c := startTestServer(t, &mockID{name: "ben"}, &mockForum{}, dm)
	if err := c.SendDMQueued(gecerliPeer, "sonra"); err != nil {
		t.Fatal(err)
	}
	if len(dm.siradaki) != 1 || dm.siradaki[0] != gecerliPeer+":sonra" {
		t.Fatalf("queue_if_offline isteği kuyruk arka ucuna gitmeli: %v", dm.siradaki)
	}
	if err := c.SendDM(gecerliPeer, "şimdi"); err != nil {
		t.Fatal(err)
	}
	if len(dm.siradaki) != 1 {
		t.Fatal("normal gönderim kuyruğa girmemeli")
	}
}

func TestDMOkundu(t *testing.T) {
	dm := &kuyrukluDM{}
	c := startTestServer(t, &mockID{}, &mockForum{}, dm)
	an := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if err := c.MarkDMRead(gecerliPeer, an); err != nil {
		t.Fatal(err)
	}
	marks, err := c.DMReadMarks()
	if err != nil || !marks[gecerliPeer].Equal(an) {
		t.Fatalf("okundu bilgisi okunabilmeli: %v %v", marks, err)
	}
	var ae *APIError
	if err := c.MarkDMRead("bozuk id", an); err == nil {
		t.Fatal("geçersiz Peer ID reddedilmeli")
	} else if !asAPIError(err, &ae) || ae.Status != http.StatusBadRequest {
		t.Fatalf("400 beklenirdi: %v", err)
	}
}

func TestDMOkundu_EskiArkaUc404(t *testing.T) {
	c := startTestServer(t, &mockID{}, &mockForum{}, &mockDM{})
	if _, err := c.DMReadMarks(); !IsNotFound(err) {
		t.Fatalf("okundu bilgisi tutmayan arka uç 404 dönmeli: %v", err)
	}
}

func asAPIError(err error, hedef **APIError) bool {
	ae, ok := err.(*APIError)
	if ok {
		*hedef = ae
	}
	return ok
}
