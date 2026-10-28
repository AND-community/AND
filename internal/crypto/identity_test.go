package crypto

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateIdentity_TwelveWords(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	words := strings.Fields(id.Mnemonic())
	if len(words) != 12 {
		t.Fatalf("expected 12-word mnemonic, got %d words: %q", len(words), id.Mnemonic())
	}

	if len(id.PrivateKey()) == 0 || len(id.PublicKey()) == 0 {
		t.Fatal("expected non-empty keypair")
	}
}

func TestGenerateIdentity_Unique(t *testing.T) {
	a, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	b, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	if a.Mnemonic() == b.Mnemonic() {
		t.Fatal("two generated identities produced the same mnemonic")
	}
}

func TestRestoreFromMnemonic_RoundTrip(t *testing.T) {
	original, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	restored, err := RestoreFromMnemonic(strings.Fields(original.Mnemonic()))
	if err != nil {
		t.Fatalf("RestoreFromMnemonic: %v", err)
	}

	if !restored.PublicKey().Equal(original.PublicKey()) {
		t.Fatal("restored identity has a different public key than the original")
	}
}

func TestRestoreFromMnemonic_InvalidMnemonic(t *testing.T) {
	_, err := RestoreFromMnemonic([]string{"not", "a", "valid", "mnemonic"})
	if err == nil {
		t.Fatal("expected an error for an invalid mnemonic, got nil")
	}
}

// TestRestoreFromRecovery_KodVeKelimeler: kayıtta gösterilen kod; küçük
// harf, tire ya da boşluk farkıyla da aynı kimliği verir. Eski sürümlerin
// gösterdiği 12 kelime de kabul edilir.
func TestRestoreFromRecovery_KodVeKelimeler(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	kod := id.RecoveryCode()
	girdiler := []string{
		kod,
		strings.ToLower(kod),
		strings.ReplaceAll(kod, "-", ""),
		"  " + strings.ReplaceAll(kod, "-", " ") + "\n",
		id.Mnemonic(),
		"  " + strings.ToUpper(strings.Join(strings.Fields(id.Mnemonic()), "   ")) + " ",
	}
	for _, g := range girdiler {
		geri, err := RestoreFromRecovery(g)
		if err != nil {
			t.Errorf("%q: %v", g, err)
			continue
		}
		if string(geri.PublicKey()) != string(id.PublicKey()) {
			t.Errorf("%q: başka bir kimlik üretildi", g)
		}
	}
}

// TestRestoreFromRecovery_EksikYaDaFazlaKod: kodun sağlama toplamı yoktur;
// eksik ya da fazla bir hane (bip39'un kabul ettiği 20/24 baytlık uzunluklar
// dahil) sessizce başka bir kimlik üretmek yerine reddedilir.
func TestRestoreFromRecovery_EksikYaDaFazlaKod(t *testing.T) {
	id, _ := GenerateIdentity()
	kod := strings.ReplaceAll(id.RecoveryCode(), "-", "")
	for _, g := range []string{
		"",
		kod[:31],
		kod + "0",
		kod + "00000000", // 20 bayt: bip39 için geçerli, AND kodu için değil
		"ZZ" + kod[2:],
		"bir iki üç",
		strings.Repeat("abandon ", 11) + "zoo", // 12 kelime, sağlama toplamı yanlış
	} {
		if _, err := RestoreFromRecovery(g); !errors.Is(err, ErrInvalidRecoveryCode) {
			t.Errorf("%q kabul edilmemeli (ErrInvalidRecoveryCode), hata: %v", g, err)
		}
	}
}
