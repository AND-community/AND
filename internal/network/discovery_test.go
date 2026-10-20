package network

// discovery_test.go — mdnsNoiseFilter'ı kapsar (bkz. muteNoisyMDNSLog'un doc
// yorumu): yalnızca zeroconf/v2'nin Windows'a özgü, bilinen gürültülü
// "Failed to set multicast interface" satırını düşürüp başka her şeyi
// olduğu gibi ilettiğini doğrular.

import (
	"bytes"
	"testing"
)

func TestMdnsNoiseFilter_DropsKnownNoisyLine(t *testing.T) {
	var out bytes.Buffer
	f := mdnsNoiseFilter{out: &out}

	line := []byte("2026/09/13 17:26:36 [WARN] mdns: Failed to set multicast interface: no such interface\n")
	n, err := f.Write(line)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(line) {
		t.Fatalf("Write döndürdüğü bayt sayısı = %d, want %d (çağıran, alttaki writer'a hiç yazılmasa bile tam yazıldığını sanmalı)", n, len(line))
	}
	if out.Len() != 0 {
		t.Fatalf("bilinen gürültülü satır alttaki writer'a ULAŞMAMALIYDI, got %q", out.String())
	}
}

func TestMdnsNoiseFilter_PassesOtherLinesThrough(t *testing.T) {
	var out bytes.Buffer
	f := mdnsNoiseFilter{out: &out}

	line := []byte("2026/09/13 17:26:36 [ERROR] something-else: gerçek bir hata\n")
	if _, err := f.Write(line); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.String() != string(line) {
		t.Fatalf("ilgisiz bir satır DEĞİŞTİRİLMEDEN geçmeliydi, got %q, want %q", out.String(), string(line))
	}
}

func TestMuteNoisyMDNSLog_IdempotentAcrossMultipleCalls(t *testing.T) {
	// sync.Once nedeniyle ikinci çağrı panik/hata üretmemeli — StartDiscovery
	// birden çok kez çağrılabilen bir test/çoklu-düğüm senaryosunda güvenli
	// olmalı.
	muteNoisyMDNSLog()
	muteNoisyMDNSLog()
}
