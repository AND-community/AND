package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKimlikYukleYaOlustur_YeniKimlikUretirVeKaydeder(t *testing.T) {
	dataDir := t.TempDir()

	k1, yeni, err := kimlikYukleYaOlustur(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !yeni {
		t.Fatal("ilk çalıştırmada yeni:true bekleniyordu")
	}
	if len(k1.PublicKey()) == 0 {
		t.Fatal("public key boş")
	}

	k2, yeni2, err := kimlikYukleYaOlustur(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if yeni2 {
		t.Fatal("ikinci çalıştırmada yeni:false bekleniyordu (dosyadan yüklendi)")
	}
	if k1.PubKeyHex() != k2.PubKeyHex() {
		t.Fatalf("aynı dataDir farklı kimlik verdi: %s != %s", k1.PubKeyHex(), k2.PubKeyHex())
	}
}

func TestKimlikYukleYaOlustur_FarkliDataDirFarkliKimlik(t *testing.T) {
	k1, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	k2, _, err := kimlikYukleYaOlustur(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if k1.PubKeyHex() == k2.PubKeyHex() {
		t.Fatal("farklı dataDir'ler aynı kimliği üretmemeliydi")
	}
}

func TestKimlikYukleYaOlustur_BozukDosyaHataVerir(t *testing.T) {
	dataDir := t.TempDir()
	if _, _, err := kimlikYukleYaOlustur(dataDir); err != nil {
		t.Fatal(err)
	}
	// kimlik.json'ı bozuk bir mnemonic ile ez.
	yol := filepath.Join(dataDir, kimlikDosyaAdi)
	if err := os.WriteFile(yol, []byte(`{"mnemonic":"gecersiz kelimeler burada"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := kimlikYukleYaOlustur(dataDir); err == nil {
		t.Fatal("bozuk mnemonic içeren dosya hata vermeliydi")
	}
}

// TestKimlikYukleYaOlustur_OkumaHatasiNotExistDisindaysaHataVerir,
// kimlik.json'ın "yok" DIŞINDA bir nedenle okunamaması (burada: bir dosya
// yerine bir DİZİN olarak var olması) durumunda da hatanın doğru
// yayıldığını doğrular — bkz. denetim (bu dal önceden yalnızca "dosya hiç
// yok" durumuyla karıştırılıyor, os.IsNotExist(err)==false yolu hiç
// tetiklenmemişti).
func TestKimlikYukleYaOlustur_OkumaHatasiNotExistDisindaysaHataVerir(t *testing.T) {
	dataDir := t.TempDir()
	yol := filepath.Join(dataDir, kimlikDosyaAdi)
	if err := os.Mkdir(yol, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := kimlikYukleYaOlustur(dataDir); err == nil {
		t.Fatal("kimlik.json bir dizinken hata bekleniyordu")
	}
}
