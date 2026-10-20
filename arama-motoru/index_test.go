package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedTime, farklı ama belirlenimci OlusturmaTS değerleri üretmek için
// kullanılır (ör. sıralama testlerinde) — gerçek saate bağımlı olmadan.
func fixedTime(saniyeOfset int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, saniyeOfset, 0, time.UTC)
}

func TestDepoAc_BosDataDirReddedilir(t *testing.T) {
	if _, err := depoAc(""); err == nil {
		t.Fatal("boş dataDir ile açılış hata vermeliydi")
	}
}

// TestDepoAc_WALModuEtkin, performans/eşzamanlılık için depoAc'ın gerçekten
// WAL günlük modunu uyguladığını doğrular (bkz. depoAc'ın doc yorumu) —
// varsayılan rollback-journal modunda bu sorgu "delete" döner.
func TestDepoAc_WALModuEtkin(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	var mod string
	if err := d.db.QueryRow("PRAGMA journal_mode").Scan(&mod); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mod, "wal") {
		t.Fatalf("journal_mode = %q, want \"wal\"", mod)
	}
}

// TestDepoAc_EskiSemaGocEder, sig sütunu eklenmeden ÖNCE oluşturulmuş bir
// arama_fts tablosuyla depoAc çağrıldığında migrateAddSigColumns'ın onu
// güvenle göç ettirdiğini (silip yeniden oluşturduğunu) doğrular. Bu
// programın yerel indeksi tamamen yeniden oluşturulabilir bir P2P ağ
// önbelleği olduğundan (bkz. migrateAddSigColumns'ın doc yorumu), eski
// (sig'siz — yani ağda güvenle yeniden paylaşılamayan) satırların
// KAYBOLMASI beklenen ve kabul edilebilir bir davranıştır; asıl kritik
// olan depoAc'ın HATA VERMEDEN yeni şemayla açılabilmesi ve ardından
// normal şekilde yeni kayıt kabul edebilmesidir.
func TestDepoAc_EskiSemaGocEder(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, veriDosyaAdi)

	eskiDB, err := sql.Open("sqlite", "file:"+yol)
	if err != nil {
		t.Fatal(err)
	}
	eskiStmts := []string{
		`CREATE TABLE bilinen (id TEXT PRIMARY KEY, tip TEXT NOT NULL, silindi INTEGER NOT NULL DEFAULT 0)`,
		`CREATE VIRTUAL TABLE arama_fts USING fts5(
			tip UNINDEXED, kaynak_id UNINDEXED, ust_id UNINDEXED, kategori, baslik, govde,
			yazar_ad, yazar_key UNINDEXED, olusturma_ts UNINDEXED,
			tokenize = 'unicode61 remove_diacritics 2'
		)`,
	}
	for _, s := range eskiStmts {
		if _, err := eskiDB.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := eskiDB.Exec(`INSERT INTO bilinen (id, tip, silindi) VALUES ('eski1', 'konu', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := eskiDB.Exec(`INSERT INTO arama_fts (tip, kaynak_id, ust_id, kategori, baslik, govde, yazar_ad, yazar_key, olusturma_ts)
		VALUES ('konu', 'eski1', '', 'genel', 'eski başlık', 'eski gövde', 'biri', 'anahtar', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := eskiDB.Close(); err != nil {
		t.Fatal(err)
	}

	d, err := depoAc(dir)
	if err != nil {
		t.Fatalf("eski şemalı veritabanı göç etmeden açılamadı: %v", err)
	}
	defer d.Kapat() //nolint:errcheck

	var sigVar string
	if err := d.db.QueryRow(`SELECT sig FROM arama_fts LIMIT 1`).Scan(&sigVar); err != nil && err != sql.ErrNoRows {
		t.Fatalf("göç sonrası sig sütunu sorgulanamadı: %v", err)
	}

	if biliniyor, _ := d.Biliniyor("eski1"); biliniyor {
		t.Fatal("göç, eski (sig'siz) kayıtları temizlemeliydi — bilinen tablosu da sıfırlanmalı")
	}
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("göç sonrası indeks boş olmalıydı, %d kayıt var", n)
	}

	// Yeni bir kayıt eklemek (artık sig sütunuyla) hâlâ normal çalışmalı.
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "yeni1", Govde: "yeni gövde", Sig: "abc"}); err != nil {
		t.Fatalf("göç sonrası yeni kayıt eklenemedi: %v", err)
	}
}

func TestEkleVeAra_BasitEslesme(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Kategori: "genel", Baslik: "Merhaba Dünya", Govde: "test gönderisi", YazarAd: "ali"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p2", Kategori: "genel", Baslik: "Alakasız", Govde: "farklı içerik", YazarAd: "veli"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("merhaba", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("beklenmeyen sonuçlar: %+v", sonuclar)
	}
}

func TestEkle_TekrarEklemeYokSayilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	s := AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "başlık", Govde: "gövde"}
	if err := d.Ekle(s); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(s); err != nil {
		t.Fatal(err)
	}
	n, err := d.SayimAl()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tekrar ekleme yeni kayıt oluşturmamalıydı, sayı=%d", n)
	}
}

func TestSil_TombstoneYenidenEklemeyiEngeller(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	s := AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "başlık", Govde: "gövde"}
	if err := d.Ekle(s); err != nil {
		t.Fatal(err)
	}
	if err := d.Sil("p1", TipKonu); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("silinen kayıt FTS'de kalmamalıydı, sayı=%d", n)
	}

	// Silinen bir ID daha sonra tekrar (ör. eski bir gossip replay'i ile)
	// eklenmeye çalışılırsa geri gelmemeli.
	if err := d.Ekle(s); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("tombstone'lanmış bir ID yeniden canlanmamalıydı, sayı=%d", n)
	}
	if biliniyor, silindi := d.Biliniyor("p1"); !biliniyor || !silindi {
		t.Fatalf("p1 bilinen+silindi olarak işaretli kalmalıydı: biliniyor=%v silindi=%v", biliniyor, silindi)
	}
}

func TestBilinenIDler_TipeGoreFiltrelenir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	_ = d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "x", Govde: "y"})
	_ = d.Ekle(AramaSonucu{Tip: TipYanit, KaynakID: "r1", Govde: "y"})
	_ = d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "m1", Govde: "y"})

	konular, err := d.BilinenIDler(TipKonu)
	if err != nil {
		t.Fatal(err)
	}
	if len(konular) != 1 || konular[0] != "p1" {
		t.Fatalf("beklenmeyen konu ID'leri: %v", konular)
	}
}

func TestAra_TurkceAksanDuyarsiz(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	_ = d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "Şifre Sıfırlama", Govde: "hesabınızın şifresi", YazarAd: "ali"})

	sonuclar, err := d.Ara("sifre", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("aksan-duyarsız eşleşme başarısız: %+v", sonuclar)
	}
}

func TestAra_OzelFTSKarakterleriGuvenli(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	_ = d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "normal", Govde: "normal gövde"})

	for _, sorgu := range []string{`"alıntı`, `test* OR (1=1)`, `-hariç`, `sütun:değer`, `""`} {
		if _, err := d.Ara(sorgu, 10); err != nil {
			t.Errorf("sorgu %q hata verdi: %v", sorgu, err)
		}
	}
}

func TestAra_YanitUstKonuBasliginiCozer(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Kategori: "teknik", Baslik: "Ana Konu Başlığı", Govde: "konu gövdesi"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipYanit, KaynakID: "r1", UstID: "p1", Govde: "aranan yanıt metni", YazarAd: "veli"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("aranan", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("1 sonuç bekleniyordu, %d geldi", len(sonuclar))
	}
	if sonuclar[0].Baslik != "Ana Konu Başlığı" || sonuclar[0].Kategori != "teknik" {
		t.Fatalf("yanıt, üst konunun başlığını/kategorisini çözmedi: %+v", sonuclar[0])
	}
}

func TestAra_UstKonuHenuzYoksaBaslikBosKalir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	// Yanıt, üst konu henüz senkronize olmadan geldi — gerçekçi bir
	// gossip sıralaması (bkz. konuBasligiVeKategori'nin doc yorumu).
	if err := d.Ekle(AramaSonucu{Tip: TipYanit, KaynakID: "r1", UstID: "p1", Govde: "aranan yanıt metni"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("aranan", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("1 sonuç bekleniyordu, %d geldi", len(sonuclar))
	}
	if sonuclar[0].Baslik != "" {
		t.Fatalf("henüz bilinmeyen bir üst konu için başlık uydurulmamalıydı: %q", sonuclar[0].Baslik)
	}
}

func TestAra_SnippetVeHighlightIsaretleyicileriIcerir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "başlık", Govde: "bu bir aranankelime içerir"}); err != nil {
		t.Fatal(err)
	}

	sonuclar, err := d.Ara("aranankelime", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 {
		t.Fatalf("1 sonuç bekleniyordu, %d geldi", len(sonuclar))
	}
	s := sonuclar[0]
	if !strings.Contains(s.Govde, SnippetOncesi+"aranankelime"+SnippetSonrasi) {
		t.Fatalf("Govde eşleşen terimi işaretlememiş: %q", s.Govde)
	}
	if !strings.Contains(s.Snippet, SnippetOncesi+"aranankelime"+SnippetSonrasi) {
		t.Fatalf("Snippet eşleşen terimi işaretlememiş: %q", s.Snippet)
	}
	if duzMetin(s.Govde) != "bu bir aranankelime içerir" {
		t.Fatalf("işaretleyiciler çıkarılınca orijinal metin bozulmuş: %q", duzMetin(s.Govde))
	}
}

func TestAra_BosSorgu(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	_ = d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "x", Govde: "y"})

	sonuclar, err := d.Ara("   ", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 0 {
		t.Fatalf("boş sorgu boş sonuç vermeliydi, %d geldi", len(sonuclar))
	}
}

func TestAraFiltreli_TipeGoreSinirlar(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "ortak kelime konu", Govde: "gövde"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipYanit, KaynakID: "y1", UstID: "k1", Govde: "ortak kelime yanıt"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "ortak kelime sohbet"}); err != nil {
		t.Fatal(err)
	}

	tumu, err := d.AraFiltreli("ortak", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tumu) != 3 {
		t.Fatalf("filtresiz aramada 3 sonuç bekleniyordu, %d geldi", len(tumu))
	}

	yalnizSohbet, err := d.AraFiltreli("ortak", TipSohbet, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(yalnizSohbet) != 1 || yalnizSohbet[0].KaynakID != "s1" {
		t.Fatalf("yalnızca sohbet türü beklenirdi: %+v", yalnizSohbet)
	}

	yalnizKonu, err := d.AraFiltreli("ortak", TipKonu, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(yalnizKonu) != 1 || yalnizKonu[0].KaynakID != "k1" {
		t.Fatalf("yalnızca konu türü beklenirdi: %+v", yalnizKonu)
	}
}

func TestAraGelismis_YazarKeyeGoreFiltreler(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "ortak konu", Govde: "gövde", YazarAd: "ali", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "ortak konu 2", Govde: "gövde 2", YazarAd: "veli", YazarKey: "anahtar-veli"}); err != nil {
		t.Fatal(err)
	}

	sayfa, err := d.AraGelismis("ortak", "", "anahtar-ali", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	sonuclar := sayfa.Sonuclar
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "k1" {
		t.Fatalf("yalnızca anahtar-ali'nin sonucu beklenirdi: %+v", sonuclar)
	}
}

// TestAraGelismis_BosSorguYazarKeyDoluysaTumIcerigiListeler,
// "yazarın diğer içerikleri" özelliğinin (bkz. model.go'nun 'a' tuşu) bir
// anahtar kelime GEREKTİRMEDEN çalıştığını doğrular — gerçek arama
// motorlarındaki "yazar sayfası" davranışı.
func TestAraGelismis_BosSorguYazarKeyDoluysaTumIcerigiListeler(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "birinci", Govde: "gövde 1", YazarKey: "anahtar-ali", OlusturmaTS: fixedTime(1)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "sohbet mesajı", YazarKey: "anahtar-ali", OlusturmaTS: fixedTime(2)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "başkasının konusu", Govde: "gövde 2", YazarKey: "anahtar-veli", OlusturmaTS: fixedTime(3)}); err != nil {
		t.Fatal(err)
	}

	sayfa, err := d.AraGelismis("", "", "anahtar-ali", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	sonuclar := sayfa.Sonuclar
	if len(sonuclar) != 2 {
		t.Fatalf("anahtar-ali'nin 2 içeriği bekleniyordu, %d geldi: %+v", len(sonuclar), sonuclar)
	}
	// En yeniden eskiye sıralı olmalı: s1 (t=2) k1'den (t=1) önce.
	if sonuclar[0].KaynakID != "s1" || sonuclar[1].KaynakID != "k1" {
		t.Fatalf("en yeniden eskiye sıralı bekleniyordu, got %+v", sonuclar)
	}

	// tip filtresiyle birleşimi de doğrula.
	yalnizKonuSayfa, err := d.AraGelismis("", TipKonu, "anahtar-ali", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	yalnizKonu := yalnizKonuSayfa.Sonuclar
	if len(yalnizKonu) != 1 || yalnizKonu[0].KaynakID != "k1" {
		t.Fatalf("yalnızca konu türü beklenirdi: %+v", yalnizKonu)
	}
}

func TestAraGelismis_BosSorguVeBosYazarKeyBosSonucVerir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck
	_ = d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "x", Govde: "y"})

	sayfa, err := d.AraGelismis("", "", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sayfa.Sonuclar) != 0 {
		t.Fatalf("ne sorgu ne yazar filtresi varken boş sonuç bekleniyordu, %d geldi", len(sayfa.Sonuclar))
	}
}
