package main

import (
	"os"
	"path/filepath"
	"testing"
)

func yeniTestProjesi(t *testing.T) (dataDir, projeID, calismaDir string) {
	t.Helper()
	dataDir = t.TempDir()
	projeID = "abc123"
	if err := os.MkdirAll(projeNesneDizini(dataDir, projeID), 0o700); err != nil {
		t.Fatal(err)
	}
	calismaDir = projeCalismaDizini(dataDir, projeID)
	if err := os.MkdirAll(calismaDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return
}

func yazDosya(t *testing.T, dir, rel, icerik string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(icerik), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNesneTekillestirme(t *testing.T) {
	dataDir, projeID, _ := yeniTestProjesi(t)
	h1, err := nesneYaz(dataDir, projeID, 'B', []byte("merhaba"))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := nesneYaz(dataDir, projeID, 'B', []byte("merhaba"))
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("aynı içerik farklı hash üretti: %s != %s", h1, h2)
	}
	tip, payload, err := nesneOku(dataDir, projeID, h1)
	if err != nil {
		t.Fatal(err)
	}
	if tip != 'B' || string(payload) != "merhaba" {
		t.Fatalf("okunan nesne beklenmedik: tip=%c payload=%q", tip, payload)
	}
}

func TestSnapshotVeCheckout(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.txt", "birinci")
	yazDosya(t, calismaDir, "alt/b.txt", "ikinci")

	treeHash, girdiler, atlanan, err := agacOlustur(dataDir, projeID, calismaDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlanan) != 0 {
		t.Fatalf("beklenmedik atlanan dosya: %v", atlanan)
	}
	if len(girdiler) != 2 {
		t.Fatalf("2 girdi bekleniyordu, %d geldi", len(girdiler))
	}

	commitHash, err := commitOlustur(dataDir, projeID, treeHash, nil, "test", "ilk kayıt")
	if err != nil {
		t.Fatal(err)
	}

	// Çalışma dizinini kirlet, sonra aynı ağaca geri dön.
	yazDosya(t, calismaDir, "a.txt", "degistirildi")
	if err := os.Remove(filepath.Join(calismaDir, "alt/b.txt")); err != nil {
		t.Fatal(err)
	}

	c, err := commitOku(dataDir, projeID, commitHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := dalaGec(dataDir, projeID, calismaDir, c.Agac); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(calismaDir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "birinci" {
		t.Fatalf("checkout sonrası içerik yanlış: %q", data)
	}
	data2, err := os.ReadFile(filepath.Join(calismaDir, "alt/b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "ikinci" {
		t.Fatalf("checkout sonrası silinen dosya geri gelmedi: %q", data2)
	}
}

func TestDurumHesapla(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.txt", "v1")
	treeHash, _, _, err := agacOlustur(dataDir, projeID, calismaDir)
	if err != nil {
		t.Fatal(err)
	}
	commitHash, err := commitOlustur(dataDir, projeID, treeHash, nil, "test", "ilk")
	if err != nil {
		t.Fatal(err)
	}

	yazDosya(t, calismaDir, "a.txt", "v2")   // değişen
	yazDosya(t, calismaDir, "b.txt", "yeni") // eklenen

	c, _ := commitOku(dataDir, projeID, commitHash)
	d, err := durumHesapla(dataDir, projeID, calismaDir, c.Agac)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Degisen) != 1 || d.Degisen[0] != "a.txt" {
		t.Fatalf("değişen listesi yanlış: %v", d.Degisen)
	}
	if len(d.Eklenen) != 1 || d.Eklenen[0] != "b.txt" {
		t.Fatalf("eklenen listesi yanlış: %v", d.Eklenen)
	}
	if d.temizMi() {
		t.Fatal("durum temiz olmamalıydı")
	}
}

func TestUcluBirlestirmeCakismasiz(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)

	// Temel commit: iki dosya.
	yazDosya(t, calismaDir, "a.txt", "ortak")
	yazDosya(t, calismaDir, "b.txt", "ortak-b")
	temelTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)
	temelHash, _ := commitOlustur(dataDir, projeID, temelTree, nil, "test", "temel")

	// Yerel dal: a.txt değişti.
	yazDosya(t, calismaDir, "a.txt", "yerel-degisiklik")
	yerelTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)
	yerelHash, _ := commitOlustur(dataDir, projeID, yerelTree, []string{temelHash}, "test", "yerel")

	// Uzak dal (temelden dallanmış): b.txt değişti, a.txt aynı kaldı.
	yazDosya(t, calismaDir, "a.txt", "ortak") // yerelden önceki haline döndür (temel ile aynı)
	yazDosya(t, calismaDir, "b.txt", "uzak-degisiklik")
	uzakTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)
	uzakHash, _ := commitOlustur(dataDir, projeID, uzakTree, []string{temelHash}, "test", "uzak")

	ata := ortakAta(dataDir, projeID, yerelHash, uzakHash)
	if ata != temelHash {
		t.Fatalf("ortak ata yanlış: beklenen %s, alınan %s", temelHash, ata)
	}

	// Çalışma dizinini yerel commit durumuna döndür, sonra birleştir.
	yc, _ := commitOku(dataDir, projeID, yerelHash)
	if err := dalaGec(dataDir, projeID, calismaDir, yc.Agac); err != nil {
		t.Fatal(err)
	}
	uc, _ := commitOku(dataDir, projeID, uzakHash)
	catismalar, err := birlestirmeUygula(dataDir, projeID, calismaDir, temelTree, yc.Agac, uc.Agac)
	if err != nil {
		t.Fatal(err)
	}
	if len(catismalar) != 0 {
		t.Fatalf("çakışma olmamalıydı: %v", catismalar)
	}
	aData, _ := os.ReadFile(filepath.Join(calismaDir, "a.txt"))
	bData, _ := os.ReadFile(filepath.Join(calismaDir, "b.txt"))
	if string(aData) != "yerel-degisiklik" {
		t.Fatalf("a.txt birleştirme sonrası yanlış: %q", aData)
	}
	if string(bData) != "uzak-degisiklik" {
		t.Fatalf("b.txt birleştirme sonrası yanlış: %q", bData)
	}
}

func TestUcluBirlestirmeCakismali(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)

	yazDosya(t, calismaDir, "a.txt", "ortak")
	temelTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)

	yazDosya(t, calismaDir, "a.txt", "yerel-surum")
	yerelTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)

	yazDosya(t, calismaDir, "a.txt", "uzak-surum")
	uzakTree, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)

	catismalar, err := birlestirmeUygula(dataDir, projeID, calismaDir, temelTree, yerelTree, uzakTree)
	if err != nil {
		t.Fatal(err)
	}
	if len(catismalar) != 1 || catismalar[0] != "a.txt" {
		t.Fatalf("a.txt çakışması bekleniyordu: %v", catismalar)
	}
	data, _ := os.ReadFile(filepath.Join(calismaDir, "a.txt"))
	if !contains(string(data), "<<<<<<< YEREL") || !contains(string(data), ">>>>>>> UZAK") {
		t.Fatalf("çakışma işaretleri yazılmamış: %q", data)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestPaketSifrelemeVeUygulama(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.txt", "merhaba dunya")
	treeHash, _, _, _ := agacOlustur(dataDir, projeID, calismaDir)
	commitHash, err := commitOlustur(dataDir, projeID, treeHash, nil, "test", "ilk")
	if err != nil {
		t.Fatal(err)
	}

	paket, sayi, err := paketOlustur(dataDir, projeID, "ana", commitHash, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if sayi != 3 { // commit + tree + blob
		t.Fatalf("3 nesne bekleniyordu, %d geldi", sayi)
	}

	sir, err := sirUret()
	if err != nil {
		t.Fatal(err)
	}
	sifreli, err := sirliSifrele(sir, paket)
	if err != nil {
		t.Fatal(err)
	}

	// Yanlış anahtarla çözülemeyeceğini doğrula.
	yanlisSir, _ := sirUret()
	if _, err := sirliCoz(yanlisSir, sifreli); err == nil {
		t.Fatal("yanlış anahtarla çözme başarılı olmamalıydı")
	}

	acik, err := sirliCoz(sir, sifreli)
	if err != nil {
		t.Fatal(err)
	}

	// Taze bir proje/nesne deposuna uygula.
	dataDir2, projeID2, _ := yeniTestProjesi(t)
	manifest, uygulanan, err := paketUygula(dataDir2, projeID2, acik)
	if err != nil {
		t.Fatal(err)
	}
	if uygulanan != 3 {
		t.Fatalf("3 nesne uygulanmalıydı, %d uygulandı", uygulanan)
	}
	if manifest.Dal != "ana" || manifest.HeadHash != commitHash {
		t.Fatalf("manifest yanlış: %+v", manifest)
	}
	if !nesneVarMi(dataDir2, projeID2, commitHash) {
		t.Fatal("commit nesnesi hedef depoda yok")
	}
}

func TestSatirDiffCokUzunDosyaAtlanir(t *testing.T) {
	dataDir, projeID, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.txt", "v1")
	treeHash, _, _, err := agacOlustur(dataDir, projeID, calismaDir)
	if err != nil {
		t.Fatal(err)
	}
	commitHash, err := commitOlustur(dataDir, projeID, treeHash, nil, "test", "ilk")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := commitOku(dataDir, projeID, commitHash)

	// maxDiffSatir sınırının üstünde, çok sayıda kısa satır içeren bir dosya —
	// bayt boyutu küçük olsa da LCS tablosunun patlamaması için reddedilmeli.
	var coksatir string
	for i := 0; i <= maxDiffSatir+10; i++ {
		coksatir += "x\n"
	}
	yazDosya(t, calismaDir, "a.txt", coksatir)

	if _, err := dosyaFarkiHesapla(dataDir, projeID, calismaDir, c.Agac, "a.txt"); err == nil {
		t.Fatal("çok satırlı dosya için hata bekleniyordu, nil döndü")
	}
}

func TestIndexOfBulunamadi(t *testing.T) {
	if got := indexOf([]string{"a", "b"}, "c"); got != -1 {
		t.Fatalf("bulunamayan eleman için -1 bekleniyordu, %d geldi", got)
	}
	if got := indexOf([]string{"a", "b"}, "a"); got != 0 {
		t.Fatalf("indeks 0 bekleniyordu, %d geldi", got)
	}
}

func TestGunlukFiltrele(t *testing.T) {
	girdiler := []LogGirdisi{
		{Hash: "aaaa1111", Commit: Commit{Yazar: "ali", Mesaj: "ilk kayıt"}},
		{Hash: "bbbb2222", Commit: Commit{Yazar: "veli", Mesaj: "hata düzeltmesi"}},
		{Hash: "cccc3333", Commit: Commit{Yazar: "ali", Mesaj: "yeni özellik"}},
	}

	if got := gunlukFiltrele(girdiler, ""); len(got) != 3 {
		t.Fatalf("boş sorguda tüm girdiler dönmeliydi, %d döndü", len(got))
	}
	if got := gunlukFiltrele(girdiler, "HATA"); len(got) != 1 || got[0].Commit.Yazar != "veli" {
		t.Fatalf("mesaj araması yanlış: %+v", got)
	}
	if got := gunlukFiltrele(girdiler, "ali"); len(got) != 2 {
		t.Fatalf("yazar araması yanlış: %d sonuç", len(got))
	}
	if got := gunlukFiltrele(girdiler, "cccc"); len(got) != 1 || got[0].Hash != "cccc3333" {
		t.Fatalf("hash araması yanlış: %+v", got)
	}
	if got := gunlukFiltrele(girdiler, "bulunmaz"); len(got) != 0 {
		t.Fatalf("eşleşmeyen sorguda boş liste bekleniyordu, %d döndü", len(got))
	}
}

func TestPaketAyristir(t *testing.T) {
	ad := paketDosyaAdi("grup1", "proje1")
	g, p, ok := paketAyristir(ad)
	if !ok || g != "grup1" || p != "proje1" {
		t.Fatalf("paket dosya adı ayrıştırılamadı: %s -> %s %s %v", ad, g, p, ok)
	}
	if _, _, ok := paketAyristir("baska_bir_dosya.txt"); ok {
		t.Fatal("ilgisiz dosya adı proje ağı paketi olarak tanınmamalıydı")
	}
}
