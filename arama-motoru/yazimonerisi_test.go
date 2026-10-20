package main

import "testing"

// ─── terimNormallestir / duzenlemeMesafesi (saf fonksiyonlar) ──────────────

// Beklenen çıktıda 'ı' (noktasız) BİLEREK korunur — bkz. aşağıdaki
// TestTerimNormallestir_NoktasizIKatlanmaz'ın doc yorumu.
func TestTerimNormallestir_TurkceAksanlarKatlanir(t *testing.T) {
	got := terimNormallestir("İstanbul Çılgın Öğütür Üşür Şıklık")
	want := "istanbul cılgın ogutur usur sıklık"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestTerimNormallestir_NoktasizIKatlanmaz, unicode61 remove_diacritics 2
// tokenizer'ının AMPİRİK OLARAK doğrulanmış davranışını yansıtır: noktasız
// 'ı' KATLANMAZ (ç/ğ/ö/ü/ş'nin aksine) — terimNormallestir bunu YANLIŞ
// katlarsa, YazimOnerisiUret indeksteki gerçek terimlerle asla eşleşmez.
// Girdi BİLEREK zaten küçük harfli dotless 'ı' ile yazılır — ASCII büyük
// 'I' kullanmak bunu test ETMEZ: Go'nun strings.ToLower'ı Türkçe-duyarlı
// olmadığından ASCII 'I'yı HER ZAMAN noktalı 'i'ye çevirir (bu, tamamen
// ayrı ve beklenen bir Unicode varsayılan davranışı, remove_diacritics'in
// konusu değil).
func TestTerimNormallestir_NoktasizIKatlanmaz(t *testing.T) {
	got := terimNormallestir("kırmızı")
	want := "kırmızı"
	if got != want {
		t.Fatalf("got %q, want %q — noktasız ı katlanmamalı", got, want)
	}
}

func TestDuzenlemeMesafesi_TemelDurumlar(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"kelime", "kelime", 0},
		{"kelime", "kelim", 1},     // sonda bir harf eksik
		{"kelime", "kelimeler", 3}, // sona ekleme
		{"", "abc", 3},
		{"abc", "", 3},
		{"kitap", "kutap", 1}, // tek karakter değişimi
	}
	for _, c := range cases {
		if got := duzenlemeMesafesi(c.a, c.b); got != c.want {
			t.Errorf("duzenlemeMesafesi(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestDuzenlemeMesafesi_CokBaytliKarakterlerRuneBazliSayilir, Türkçe çok
// baytlı bir karakterin (ör. 'ş', UTF-8'de 2 bayt) TEK bir düzenleme adımı
// saydığını doğrular — byte bazlı bir uygulama bunu yanlış (2 adım gibi)
// sayardı.
func TestDuzenlemeMesafesi_CokBaytliKarakterlerRuneBazliSayilir(t *testing.T) {
	if got := duzenlemeMesafesi("şeker", "seker"); got != 1 {
		t.Fatalf("'ş'→'s' tek bir rune değişimi olmalıydı, got %d", got)
	}
}

// ─── YazimOnerisiUret — uçtan uca ───────────────────────────────────────────

func TestYazimOnerisiUret_TekHarfHatasiniDuzeltir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek bahçesi", Govde: "kelebekler hakkında bir yazı"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("kelebk", 10) // sıfır sonuç bekleniyor: "kelebk" indekste yok
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 0 {
		t.Fatalf("test öncülü bozuk: \"kelebk\" hiçbir şeyle eşleşmemeliydi, %d sonuç geldi", len(sonuclar))
	}

	oneri, bulunduMu := d.YazimOnerisiUret("kelebk")
	if !bulunduMu {
		t.Fatal("bir öneri bulunması bekleniyordu")
	}
	if oneri != "kelebek" {
		t.Fatalf("got oneri=%q, want \"kelebek\"", oneri)
	}
}

// TestYazimOnerisiUret_BilinenTerimIcinOneriUretmez, sorgu zaten indekste
// bilinen bir terimin ÖN EKİYLE eşleştiğinde (yani arama zaten sonuç
// verecekken) yazım önerisinin GEREKSİZ yere tetiklenmediğini doğrular.
func TestYazimOnerisiUret_BilinenTerimIcinOneriUretmez(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek bahçesi", Govde: "gövde"}); err != nil {
		t.Fatal(err)
	}

	if _, bulunduMu := d.YazimOnerisiUret("kelebek"); bulunduMu {
		t.Fatal("zaten bilinen bir terim için öneri üretilmemeliydi")
	}
	// Prefix de aynı şekilde: "kele" indeksteki "kelebek"in bir ön eki.
	if _, bulunduMu := d.YazimOnerisiUret("kele"); bulunduMu {
		t.Fatal("bilinen bir terimin ön eki için öneri üretilmemeliydi")
	}
}

// TestYazimOnerisiUret_BosIndeksdeOneriYok, hiç içerik indekslenmemişken
// (terim_sozlugu boş) panik atmadan/hata vermeden "öneri yok" döndüğünü
// doğrular.
func TestYazimOnerisiUret_BosIndeksdeOneriYok(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if _, bulunduMu := d.YazimOnerisiUret("herhangi"); bulunduMu {
		t.Fatal("boş indekste öneri bulunmamalıydı")
	}
}

// TestYazimOnerisiUret_CokUzakKelimeIcinOneriYok, düzenleme mesafesi
// eşiğin çok üzerinde olan tamamen alakasız bir kelime için öneri
// üretilMEdiğini doğrular — her sıfır-sonuçlu arama rastgele bir terim
// önermemeli.
func TestYazimOnerisiUret_CokUzakKelimeIcinOneriYok(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek", Govde: "gövde"}); err != nil {
		t.Fatal(err)
	}

	if _, bulunduMu := d.YazimOnerisiUret("otomobilendüstrisi"); bulunduMu {
		t.Fatal("alakasız/çok uzak bir kelime için öneri üretilmemeliydi")
	}
}
