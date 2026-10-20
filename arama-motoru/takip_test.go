package main

// takip_test.go — index.go'nun "Kayıtlı arama takibi" bölümünün
// (TakipEkle/TakipListele/TakipSil/TakipGoruldu/TakipToplamYeni/
// takipEslestir) ve model.go'nun bunu kullanan ekranlarının testleri.

import "testing"

func TestTakipEkle_BosSorguVeYazarReddedilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("", "", ""); err == nil {
		t.Fatal("hem sorgu hem yazar boşken bir hata bekleniyordu")
	}
}

func TestTakipEkle_AyniUclununTekrariNoOptur(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("golang", TipKonu, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.TakipEkle("golang", TipKonu, ""); err != nil {
		t.Fatalf("aynı üçlünün tekrar eklenmesi hata vermemeliydi (INSERT OR IGNORE): %v", err)
	}
	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 {
		t.Fatalf("tekrar edilen üçlü ikinci bir satır oluşturmamalıydı: %+v", liste)
	}
}

func TestTakipListele_EnYeniEklenenOnde(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("ilk", "", "yazar1"); err != nil {
		t.Fatal(err)
	}
	if err := d.TakipEkle("ikinci", "", "yazar2"); err != nil {
		t.Fatal(err)
	}
	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 2 || liste[0].Sorgu != "ikinci" || liste[1].Sorgu != "ilk" {
		t.Fatalf("en yeni eklenen (ikinci) önde olmalıydı: %+v", liste)
	}
}

func TestTakipSil_Kaldirir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("golang", "", ""); err != nil {
		t.Fatal(err)
	}
	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 {
		t.Fatalf("beklenen 1 kayıt, %d geldi", len(liste))
	}
	if err := d.TakipSil(liste[0].ID); err != nil {
		t.Fatal(err)
	}
	liste2, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste2) != 0 {
		t.Fatalf("silinen kayıt hâlâ listede: %+v", liste2)
	}
}

// TestTakipEslestir_SorguIleEslesenYeniIcerikSayaciArtirir, Ekle'nin
// (forum/chat/site ingest yolu) çağırdığı takipEslestir'in, kaydedilen
// sorguyla GERÇEKTEN eşleşen yeni bir kayıt geldiğinde yeni_eslesme
// sayacını artırdığını doğrular.
func TestTakipEslestir_SorguIleEslesenYeniIcerikSayaciArtirir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("kelebek", "", ""); err != nil {
		t.Fatal(err)
	}

	// Eşleşmeyen bir kayıt sayaci ARTIRMAMALI.
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "alakasız", Govde: "ilgisiz metin"}); err != nil {
		t.Fatal(err)
	}
	// Eşleşen bir kayıt sayaci artırmalı.
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "kelebek haberi", Govde: "g"}); err != nil {
		t.Fatal(err)
	}

	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || liste[0].YeniEslesme != 1 {
		t.Fatalf("tam olarak bir eşleşme bekleniyordu: %+v", liste)
	}

	if err := d.TakipGoruldu(liste[0].ID); err != nil {
		t.Fatal(err)
	}
	liste2, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if liste2[0].YeniEslesme != 0 {
		t.Fatalf("TakipGoruldu sonrası sayaç sıfırlanmalıydı: %+v", liste2)
	}
}

// TestTakipEslestir_TipVeYazarFiltresiUygulanir, boş sorgulu (yalnızca
// tür/yazar filtreli) bir takibin, filtreye uyan HER yeni kayıtla eşleştiğini
// ama uymayanla eşleşmediğini doğrular.
func TestTakipEslestir_TipVeYazarFiltresiUygulanir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("", TipSohbet, "anahtar-ali"); err != nil {
		t.Fatal(err)
	}

	// Yanlış yazar — eşleşmemeli.
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s1", Govde: "merhaba", YazarKey: "anahtar-baska"}); err != nil {
		t.Fatal(err)
	}
	// Yanlış tür (doğru yazar) — eşleşmemeli.
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "b", Govde: "g", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	// Doğru tür VE yazar — eşleşmeli.
	if err := d.Ekle(AramaSonucu{Tip: TipSohbet, KaynakID: "s2", Govde: "selam", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}

	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || liste[0].YeniEslesme != 1 {
		t.Fatalf("tam olarak bir eşleşme bekleniyordu (tip+yazar filtresiyle): %+v", liste)
	}
}

// TestTakipEslestir_EkleYaGuncelleGuncellemedeDeCalisir, site sayfası gibi
// DEĞİŞEBİLİR içerikte de (EkleYaGuncelle) takip eşleştirmesinin
// çalıştığını doğrular.
func TestTakipEslestir_EkleYaGuncelleGuncellemedeDeCalisir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("", TipSite, "anahtar-ali"); err != nil {
		t.Fatal(err)
	}
	if err := d.EkleYaGuncelle(AramaSonucu{Tip: TipSite, KaynakID: "anahtar-ali/hakkimda", Baslik: "Hakkımda", Govde: "g", YazarKey: "anahtar-ali"}); err != nil {
		t.Fatal(err)
	}
	liste, err := d.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || liste[0].YeniEslesme != 1 {
		t.Fatalf("EkleYaGuncelle de eşleşmeyi tetiklemeliydi: %+v", liste)
	}
}

func TestTakipToplamYeni_TumKayitlarinToplaminiVerir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	if err := d.TakipEkle("a", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.TakipEkle("b", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "a", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k2", Baslik: "b", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	toplam, err := d.TakipToplamYeni()
	if err != nil {
		t.Fatal(err)
	}
	if toplam != 2 {
		t.Fatalf("toplam=2 bekleniyordu, got %d", toplam)
	}
}

// ─── model.go: ctrl+t (Ara ekranı), ekTakip ekranı ─────────────────────────

func TestModel_CtrlTAramayiTakibeAlir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	m.aramaIn.SetValue("golang")

	next, _ := m.Update(keyMsg("ctrl+t"))
	m = next.(model)
	if m.isErr {
		t.Fatalf("başarılı takibe almada hata bildirimi olmamalıydı: %q", m.notice)
	}
	liste, err := m.depo.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 || liste[0].Sorgu != "golang" {
		t.Fatalf("takip listesine eklenmemiş: %+v", liste)
	}
}

func TestModel_CtrlRSiralamaModunuDegistirir(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	if m.siralama != SiralamaAlaka {
		t.Fatalf("varsayılan sıralama SiralamaAlaka olmalıydı, got %v", m.siralama)
	}
	next, _ := m.Update(keyMsg("ctrl+r"))
	m = next.(model)
	if m.siralama != SiralamaYeni {
		t.Fatalf("ctrl+r sonrası SiralamaYeni bekleniyordu, got %v", m.siralama)
	}
	next, _ = m.Update(keyMsg("ctrl+r"))
	m = next.(model)
	if m.siralama != SiralamaAlaka {
		t.Fatalf("ikinci ctrl+r sonrası SiralamaAlaka'ya dönmeliydi, got %v", m.siralama)
	}
}

func TestModel_TakipEkraniGezinmeVeSilme(t *testing.T) {
	m := testModel(t)
	if err := m.depo.TakipEkle("birinci", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.TakipEkle("ikinci", "", ""); err != nil {
		t.Fatal(err)
	}

	next, cmd := m.Update(takipListesiCmd(m.depo)())
	m = next.(model)
	_ = cmd
	if len(m.takipListesi) != 2 {
		t.Fatalf("2 kayıtlı arama bekleniyordu: %+v", m.takipListesi)
	}
	m.ekran = ekTakip

	// "ikinci" en yeni eklenen olduğundan listenin başında (idx 0).
	if m.takipListesi[m.takipIdx].Sorgu != "ikinci" {
		t.Fatalf("beklenen ilk satır 'ikinci': %+v", m.takipListesi)
	}
	next, _ = m.Update(keyMsg("d"))
	m = next.(model)
	if len(m.takipListesi) != 1 || m.takipListesi[0].Sorgu != "birinci" {
		t.Fatalf("silme sonrası yalnızca 'birinci' kalmalıydı: %+v", m.takipListesi)
	}
	liste, err := m.depo.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if len(liste) != 1 {
		t.Fatalf("silme veritabanına da yansımalıydı: %+v", liste)
	}
}

// TestModel_TakipEkraniEnterAramayaGecirirVeGoruldu, bir kayıtlı aramaya
// enter basmanın Ara ekranına geçip sorguyu doldurduğunu VE yeni_eslesme
// sayacını sıfırladığını doğrular.
func TestModel_TakipEkraniEnterAramayaGecirirVeGoruldu(t *testing.T) {
	m := testModel(t)
	if err := m.depo.TakipEkle("kelebek", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "k1", Baslik: "kelebek haberi", Govde: "g"}); err != nil {
		t.Fatal(err)
	}
	liste, err := m.depo.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if liste[0].YeniEslesme != 1 {
		t.Fatalf("eşleşme beklenıyordu: %+v", liste)
	}
	m.takipListesi = liste
	m.takipIdx = 0
	m.ekran = ekTakip

	next, _ := m.Update(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekArama || m.aramaIn.Value() != "kelebek" {
		t.Fatalf("Ara ekranına 'kelebek' sorgusuyla geçilmeliydi: ekran=%v sorgu=%q", m.ekran, m.aramaIn.Value())
	}
	liste2, err := m.depo.TakipListele()
	if err != nil {
		t.Fatal(err)
	}
	if liste2[0].YeniEslesme != 0 {
		t.Fatalf("enter sonrası TakipGoruldu çağrılmalıydı: %+v", liste2)
	}
}

func TestModel_AnaMenudenTakipEdilenlereGirer(t *testing.T) {
	m := testModel(t)
	if err := m.depo.TakipEkle("x", "", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		next, _ := m.Update(keyMsg("down"))
		m = next.(model)
	}
	if m.menuSecenekleri()[m.menuIdx] != "Takip Edilenler" {
		t.Fatalf("menüde 'Takip Edilenler' seçili olmalıydı, got %q", m.menuSecenekleri()[m.menuIdx])
	}
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(model)
	if m.ekran != ekTakip {
		t.Fatalf("ekTakip'e geçilmeliydi, got %v", m.ekran)
	}
	if cmd == nil {
		t.Fatal("Takip Edilenler'e girince takipListesiCmd tetiklenmeliydi")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if len(m.takipListesi) != 1 {
		t.Fatalf("1 kayıtlı arama bekleniyordu: %+v", m.takipListesi)
	}
	_ = m.View()
}
