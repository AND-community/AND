package chatlog

import (
	"encoding/json"
	"testing"

	"github.com/lucian95511/and/protokol/altin"
)

// TestAltinOrnek, sohbet iletisinin ağdaki biçimini protokol/testdata'daki
// örneğe sabitler; arama motoru aynı örneği kendi koduyla doğrular.
func TestAltinOrnek(t *testing.T) {
	m := &Message{
		AuthorKey:  altin.AnahtarHex(),
		AuthorName: "altın",
		Text:       "Merhaba <dünya> & herkes",
		SentAt:     altin.Zaman,
	}
	m.ID = deriveID(m)
	m.Sig = altin.Imzala(payload(m))
	data, _ := json.Marshal(m)
	altin.Karsilastir(t, "sohbet_mesaji.json", data)

	var geri Message
	if err := json.Unmarshal(altin.Oku(t, "sohbet_mesaji.json"), &geri); err != nil || !verify(&geri) || deriveID(&geri) != geri.ID {
		t.Fatalf("kayıtlı sohbet örneği doğrulanmalı: %v", err)
	}
}
