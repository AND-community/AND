package filemgr

import (
	"bytes"
	"testing"
	"time"
)

// FuzzCerceveler: eşin gönderdiği dosya aktarımı çerçeveleri (INIT, CHUNK,
// RESUME, ACK) ne olursa olsun okuyucular panik yapmaz ve sınırların
// üstünde bellek ayırmaz. Uzun deneme:
//
//	go test ./internal/filemgr -run '^$' -fuzz FuzzCerceveler -fuzztime 30s
func FuzzCerceveler(f *testing.F) {
	var init bytes.Buffer
	_ = writeINIT(&init, "0123456789abcdef0123456789abcdef", "rapor.pdf", 1024, uint32(chunkBytes), 1, time.Now())
	f.Add(init.Bytes())
	var parca bytes.Buffer
	_ = writeCHUNK(&parca, 0, []byte("veri"))
	f.Add(parca.Bytes()[1:]) // readCHUNKBody tür baytından sonrasını okur
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, veri []byte) {
		_, ad, boyut, _, _, _, err := readINIT(bytes.NewReader(veri))
		if err == nil && (len(ad) > 512 || boyut <= 0) {
			t.Fatalf("readINIT sınır dışı değeri kabul etti: ad %d bayt, boyut %d", len(ad), boyut)
		}
		if _, data, err := readCHUNKBody(bytes.NewReader(veri)); err == nil && len(data) > chunkBytes*2 {
			t.Fatalf("readCHUNKBody %d baytlık parça kabul etti", len(data))
		}
		_, _ = readRESUME(bytes.NewReader(veri))
		_, _ = readACK(bytes.NewReader(veri))
	})
}
