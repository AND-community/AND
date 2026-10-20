package hizsiniri

import (
	"fmt"
	"testing"
	"time"
)

var simdi = time.Date(2026, 10, 20, 15, 30, 0, 0, time.UTC)

func TestGecmis_YazarSiniriYazildigiSaateGore(t *testing.T) {
	var g Gecmis
	saat := simdi.Add(-48 * time.Hour)
	kabul := 0
	for i := 0; i < 15; i++ {
		if g.Izin("yazar", saat.Add(time.Duration(i)*time.Minute), simdi, 10, 1000, 5*24*time.Hour) {
			kabul++
		}
	}
	if kabul != 10 {
		t.Fatalf("aynı saatte yazılmış 15 içerikten 10'u kabul edilmeli: %d", kabul)
	}
	if !g.Izin("yazar", saat.Add(-time.Hour), simdi, 10, 1000, 5*24*time.Hour) {
		t.Fatal("başka bir saatte yazılmış içerik ayrı sayılmalı")
	}
	if !g.Izin("baska", saat, simdi, 10, 1000, 5*24*time.Hour) {
		t.Fatal("başka yazarın sayımı ayrı olmalı")
	}
}

func TestGecmis_ToplamButceSaatlik(t *testing.T) {
	var g Gecmis
	for i := 0; i < 5; i++ {
		if !g.Izin(fmt.Sprint("y", i), simdi, simdi, 10, 5, time.Hour) {
			t.Fatalf("%d. içerik bütçe içinde", i)
		}
	}
	if g.Izin("y-fazla", simdi, simdi, 10, 5, time.Hour) {
		t.Fatal("saatlik toplam bütçe aşılınca reddedilmeli")
	}
	if !g.Izin("y-sonra", simdi, simdi.Add(time.Hour+time.Second), 10, 5, time.Hour) {
		t.Fatal("bir saat sonra bütçe yenilenmeli")
	}
}

func TestGecmis_EskiSaatlerUnutulur(t *testing.T) {
	var g Gecmis
	eski := simdi.Add(-10 * 24 * time.Hour)
	g.Izin("yazar", eski, simdi.Add(-10*24*time.Hour), 10, 1000, 24*time.Hour)
	g.Izin("yazar", simdi, simdi, 10, 1000, 24*time.Hour)
	if n := len(g.yazarSaat["yazar"]); n != 1 {
		t.Fatalf("saklama süresinden eski saat sayımları silinmeli, %d saat kaldı", n)
	}
}
