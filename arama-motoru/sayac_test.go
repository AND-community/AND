package main

// sayac_test.go — SayimAl'ın artık bir SQL sorgusu değil, Depo.sayac'ın
// (bkz. index.go'nun doc yorumu) bellek-içi bir atomic sayacı olmasının
// getirdiği YENİ davranışların testleri: EkleYaGuncelle'nin "güncelleme"
// dalının sayacı DEĞİŞTİRMEMESİ, var olmayan bir ID'yi silmenin sayacı
// eksiye DÜŞÜRMEMESİ, eşzamanlı Ekle çağrılarının kaybolmadan doğru
// sayılması, ve depoAc'ın var olan bir veritabanını AÇARKEN sayacı doğru
// tohumlaması. Ekle/Sil'in TEMEL sayım davranışı zaten index_test.go'da
// (TestEkle_TekrarEklemeYokSayilir, TestSil_TombstoneYenidenEklemeyiEngeller)
// dolaylı olarak kapsanıyor.

import (
	"sync"
	"testing"
	"time"
)

func TestSayimAl_YeniDepoSifirlaBaslar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if n, err := d.SayimAl(); err != nil || n != 0 {
		t.Fatalf("yeni depo sayım=0 err=nil bekleniyordu, got n=%d err=%v", n, err)
	}
}

// TestEkleYaGuncelle_GuncellemeSayaciDegistirmez, aynı KaynakID'nin DAHA
// YENİ bir sürümle yeniden yayınlanmasının (delete+insert, net sıfır)
// sayacı ARTIRMADIĞINI doğrular — yalnızca gerçekten YENİ bir kaynak_id
// sayacı artırmalı.
func TestEkleYaGuncelle_GuncellemeSayaciDegistirmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	kod := siteKaynakID("anahtar-ali", "hakkimda")
	eski := fixedTime(1)
	yeni := fixedTime(2)

	if err := d.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "v1", Govde: "g", OlusturmaTS: eski}); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("ilk yayından sonra sayım=1 bekleniyordu, got %d", n)
	}

	if err := d.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "v2", Govde: "g2", OlusturmaTS: yeni}); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("yeniden yayından (güncelleme) sonra sayım HÂLÂ 1 olmalıydı, got %d", n)
	}

	// Eski (replay) bir sürüm sessizce yok sayılır — bu da sayacı
	// değiştirmemeli.
	if err := d.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: kod, Baslik: "eski-replay", Govde: "g", OlusturmaTS: eski}); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("eski bir replay'den sonra sayım HÂLÂ 1 olmalıydı, got %d", n)
	}
}

// TestSil_VarOlmayanKaynakSayaciEksiyeDusurmez, hiç var olmamış (ya da
// zaten silinmiş) bir kaynak_id'yi tekrar silmenin sayacı EKSİYE
// düşürmediğini doğrular (bkz. Sil'in RowsAffected() kontrolü).
func TestSil_VarOlmayanKaynakSayaciEksiyeDusurmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Sil("hic-var-olmamis-id", TipKonu); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("var olmayan bir ID'yi silmek sayacı değiştirmemeliydi, got %d", n)
	}

	// Zaten silinmiş bir ID'yi İKİNCİ KEZ silmek de eksiye düşürmemeli.
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Sil("p1", TipKonu); err != nil {
		t.Fatal(err)
	}
	if err := d.Sil("p1", TipKonu); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("aynı ID'yi iki kez silmek sayacı 0'ın altına düşürmemeliydi, got %d", n)
	}
}

// TestSayac_EszamanliEklemelerKaybolmaz, atomic.Int64 kullanımının GERÇEK
// gerekçesini doğrular: birden fazla goroutine'den (forum/chat/site
// gossip işleyicileri, sync goroutine'leri gerçek hayatta AYNI ANDA
// çalışır) eşzamanlı Ekle çağrıldığında sayacın hiçbir artışı
// KAYBETMEDİĞİNİ doğrular.
func TestSayac_EszamanliEklemelerKaybolmaz(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	const goroutineSayisi = 20
	var wg sync.WaitGroup
	hatalar := make(chan error, goroutineSayisi)
	for i := 0; i < goroutineSayisi; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "p" + string(rune('a'+i))
			if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: id, Baslik: "b", Govde: "g", OlusturmaTS: time.Now()}); err != nil {
				hatalar <- err
			}
		}(i)
	}
	wg.Wait()
	close(hatalar)
	for err := range hatalar {
		t.Fatal(err)
	}

	if n, _ := d.SayimAl(); n != goroutineSayisi {
		t.Fatalf("eşzamanlı %d ekleme sonrası sayım=%d bekleniyordu, got %d", goroutineSayisi, goroutineSayisi, n)
	}
	// Gerçek DB durumuyla da tutarlı olmalı (sayacın DB'den SAPMADIĞININ
	// kanıtı).
	gercekSayi, err := d.BilinenIDler(TipKonu)
	if err != nil {
		t.Fatal(err)
	}
	if len(gercekSayi) != goroutineSayisi {
		t.Fatalf("bilinen ID listesi de %d olmalıydı, got %d", goroutineSayisi, len(gercekSayi))
	}
}

// TestDepoAc_VarOlanVeriyleSayaciDogruTohumlar, var olan (önceki bir
// çalıştırmadan kalma) bir veritabanı dosyasını YENİDEN açmanın sayacı
// SIFIRDAN DEĞİL, dosyadaki gerçek satır sayısından tohumladığını
// doğrular — depoAc'ın açılış COUNT(*)'unun doğruluğu.
func TestDepoAc_VarOlanVeriyleSayaciDogruTohumlar(t *testing.T) {
	dataDir := t.TempDir()

	d1, err := depoAc(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d1.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d1.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d1.Kapat(); err != nil {
		t.Fatal(err)
	}

	d2, err := depoAc(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Kapat() //nolint:errcheck
	if n, err := d2.SayimAl(); err != nil || n != 2 {
		t.Fatalf("yeniden açılan depo mevcut 2 kaydı yansıtmalıydı, got n=%d err=%v", n, err)
	}

	// Yeniden açıldıktan SONRA da normal şekilde artmaya devam etmeli.
	if err := d2.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p2", Baslik: "b2", Govde: "g2"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := d2.SayimAl(); n != 3 {
		t.Fatalf("yeniden açılan depoda ekleme sonrası sayım=3 bekleniyordu, got %d", n)
	}
}
