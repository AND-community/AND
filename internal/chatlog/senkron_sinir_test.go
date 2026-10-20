package chatlog

import (
	"fmt"
	"testing"
	"time"
)

// TestSenkron_GecmisCanliSohbetiEngellemez: senkronla gelen geçmiş mesajlar
// canlı sayaçlara sayılmaz. Önceden saatlik ağ bütçesini (rlGlobalMax) geçmiş
// doldurduğunda canlı sohbet bir saat reddediliyordu.
func TestSenkron_GecmisCanliSohbetiEngellemez(t *testing.T) {
	noRLCooldown(t)
	eski := rlGlobalMax
	rlGlobalMax = 5
	t.Cleanup(func() { rlGlobalMax = eski })

	s, _ := newTestStore(t)
	var resp syncResponse
	for i := 0; i < 20; i++ {
		pub, priv := genKey(t)
		resp.Messages = append(resp.Messages, signedMessage(priv, pub, fmt.Sprintf("eski mesaj %d", i), time.Now().Add(-time.Duration(i+1)*time.Minute)))
	}
	s.senkronYanitiUygula(resp)
	if n := len(s.All()); n != 20 {
		t.Fatalf("geçmişin tamamı gelmeli (canlı bütçe 5 olsa da), %d geldi", n)
	}

	pub, priv := genKey(t)
	s.handleMessage(mustJSON(t, signedMessage(priv, pub, "şimdi yazıldı", time.Now())))
	if n := len(s.All()); n != 21 {
		t.Fatal("senkrondan sonra canlı mesaj kabul edilmeli")
	}
}
