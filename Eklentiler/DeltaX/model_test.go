package main

import (
	"encoding/json"
	"testing"

	"github.com/lucian95511/and/internal/pluginapi"
)

func TestZarfIsleSahteKurucudanRosterGuncellemesiniReddeder(t *testing.T) {
	dataDir := t.TempDir()

	gercek := Uye{Ad: "gercekKurucu", PeerID: "peer-gercek"}
	g := Grup{ID: "grup1", Ad: "Orijinal Ad", Sir: mustSir(t), KurucuAd: gercek.Ad, KurucuPeer: gercek.PeerID, Uyeler: []Uye{gercek}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	// Grubun gerçek kurucusu olmayan biri, sahte bir Kurucu.PeerID ile grup
	// adını ve üye listesini değiştirmeye çalışıyor.
	saldirgan := Uye{Ad: "saldirgan", PeerID: "peer-saldirgan"}
	payload, _ := json.Marshal(davetPayload{
		GrupAd:   "Ele Geçirilmiş Ad",
		Sir:      g.Sir,
		Kurucu:   Uye{Ad: "sahte", PeerID: "peer-sahte"}, // gerçek kurucuyla eşleşmiyor
		Gonderen: Uye{Ad: "sahte", PeerID: "peer-sahte"}, // ne kurucu ne yönetici
		Uyeler:   []Uye{saldirgan},
	})

	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: "peer-kurban"}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mUyeGuncelle, GrupID: g.ID, Payload: payload})
	_ = m

	sonra, err := grupOku(dataDir, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sonra.Ad != "Orijinal Ad" {
		t.Fatalf("grup adı sahte kurucudan gelen mesajla değişmemeliydi, geldi: %q", sonra.Ad)
	}
	if len(sonra.Uyeler) != 1 || sonra.Uyeler[0].PeerID != gercek.PeerID {
		t.Fatalf("üye listesi sahte mesajla değişmemeliydi: %+v", sonra.Uyeler)
	}
}

func TestZarfIsleGercekKurucudanRosterGuncellemesiniKabulEder(t *testing.T) {
	dataDir := t.TempDir()

	gercek := Uye{Ad: "gercekKurucu", PeerID: "peer-gercek"}
	g := Grup{ID: "grup1", Ad: "Orijinal Ad", Sir: mustSir(t), KurucuAd: gercek.Ad, KurucuPeer: gercek.PeerID, Uyeler: []Uye{gercek}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	yeniUye := Uye{Ad: "yeniUye", PeerID: "peer-yeni"}
	payload, _ := json.Marshal(davetPayload{
		GrupAd:   "Güncellenmiş Ad",
		Sir:      g.Sir,
		Kurucu:   gercek,
		Gonderen: gercek, // gerçek kurucu gönderiyor
		Uyeler:   []Uye{gercek, yeniUye},
	})

	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: "peer-kurban"}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mUyeGuncelle, GrupID: g.ID, Payload: payload})
	_ = m

	sonra, err := grupOku(dataDir, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sonra.Ad != "Güncellenmiş Ad" {
		t.Fatalf("grup adı gerçek kurucudan gelen mesajla güncellenmeliydi, geldi: %q", sonra.Ad)
	}
	if len(sonra.Uyeler) != 2 {
		t.Fatalf("üye listesi 2 üye içermeliydi: %+v", sonra.Uyeler)
	}
}

func TestZarfIsleAyrilKurucuUyeyiRosterdenCikarir(t *testing.T) {
	dataDir := t.TempDir()

	kurucu := Uye{Ad: "kurucu", PeerID: "peer-kurucu"}
	ayrilan := Uye{Ad: "ayrilan", PeerID: "peer-ayrilan"}
	g := Grup{ID: "grup1", Ad: "Grup", Sir: mustSir(t), KurucuAd: kurucu.Ad, KurucuPeer: kurucu.PeerID, Uyeler: []Uye{kurucu, ayrilan}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(ayrilPayload{Uye: ayrilan})
	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: kurucu.PeerID}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mAyril, GrupID: g.ID, Payload: payload})
	_ = m

	sonra, err := grupOku(dataDir, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sonra.uyeMi(ayrilan.PeerID) {
		t.Fatalf("ayrilan üye hâlâ roster'da: %+v", sonra.Uyeler)
	}
	if !sonra.uyeMi(kurucu.PeerID) {
		t.Fatal("kurucu yanlışlıkla roster'dan çıkarılmış")
	}
}

func TestZarfIsleGrupKapatSahteKurucudanKabulEtmez(t *testing.T) {
	dataDir := t.TempDir()

	kurucu := Uye{Ad: "kurucu", PeerID: "peer-kurucu"}
	g := Grup{ID: "grup1", Ad: "Grup", Sir: mustSir(t), KurucuAd: kurucu.Ad, KurucuPeer: kurucu.PeerID, Uyeler: []Uye{kurucu}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(grupKapatPayload{Gonderen: Uye{PeerID: "peer-sahte"}})
	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: "peer-uye"}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mGrupKapat, GrupID: g.ID, Payload: payload})
	_ = m

	if _, err := grupOku(dataDir, g.ID); err != nil {
		t.Fatalf("sahte kurucudan gelen kapatma isteğiyle grup silinmemeliydi: %v", err)
	}
}

func TestZarfIsleGrupKapatGercekKurucudanGrubuSiler(t *testing.T) {
	dataDir := t.TempDir()

	kurucu := Uye{Ad: "kurucu", PeerID: "peer-kurucu"}
	g := Grup{ID: "grup1", Ad: "Grup", Sir: mustSir(t), KurucuAd: kurucu.Ad, KurucuPeer: kurucu.PeerID, Uyeler: []Uye{kurucu}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(grupKapatPayload{Gonderen: kurucu})
	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: "peer-uye"}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mGrupKapat, GrupID: g.ID, Payload: payload})
	_ = m

	if _, err := grupOku(dataDir, g.ID); err == nil {
		t.Fatal("gerçek kurucudan gelen kapatma isteğiyle grup silinmeliydi")
	}
}

func TestZarfIsleGrupKapatYoneticidenDeKabulEder(t *testing.T) {
	dataDir := t.TempDir()

	kurucu := Uye{Ad: "kurucu", PeerID: "peer-kurucu"}
	yonetici := Uye{Ad: "yonetici", PeerID: "peer-yonetici"}
	g := Grup{ID: "grup1", Ad: "Grup", Sir: mustSir(t), KurucuAd: kurucu.Ad, KurucuPeer: kurucu.PeerID, Uyeler: []Uye{kurucu, yonetici}, Yoneticiler: []Uye{yonetici}}
	if err := grupKaydet(dataDir, g); err != nil {
		t.Fatal(err)
	}

	// Kurucu ulaşılamaz durumda olsa bile, kurucunun daha önce atadığı
	// bir yönetici grubu kapatabilmeli (yedek kurucu senaryosu).
	payload, _ := json.Marshal(grupKapatPayload{Gonderen: yonetici})
	m := model{dataDir: dataDir, identity: pluginapi.IdentityInfo{PeerID: "peer-uye"}}
	m, _ = m.zarfIsle(kontrolZarfi{Tip: mGrupKapat, GrupID: g.ID, Payload: payload})
	_ = m

	if _, err := grupOku(dataDir, g.ID); err == nil {
		t.Fatal("yöneticiden gelen kapatma isteğiyle grup silinmeliydi")
	}
}

func TestYetkiliMiKurucuVeYoneticiyiKapsar(t *testing.T) {
	kurucu := Uye{Ad: "kurucu", PeerID: "peer-kurucu"}
	yonetici := Uye{Ad: "yonetici", PeerID: "peer-yonetici"}
	digeri := Uye{Ad: "digeri", PeerID: "peer-digeri"}
	g := Grup{KurucuPeer: kurucu.PeerID, Yoneticiler: []Uye{yonetici}}

	if !g.yetkiliMi(kurucu.PeerID) {
		t.Error("kurucu yetkili sayılmalı")
	}
	if !g.yetkiliMi(yonetici.PeerID) {
		t.Error("yönetici yetkili sayılmalı")
	}
	if g.yetkiliMi(digeri.PeerID) {
		t.Error("sıradan üye yetkili sayılmamalı")
	}
}
