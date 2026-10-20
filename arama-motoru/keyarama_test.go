package main

// keyarama_test.go — keyArama'nın sayfalama (pgup/pgdown) ve yazım önerisi
// uygulama (ctrl+d) tuş işleyicilerinin MODEL seviyesinde testleri. Altta
// yatan veri katmanı (index.go'nun AraGelismis'i, yazimonerisi.go'nun
// YazimOnerisiUret'i) zaten ayrı test edilmiş; buradaki testler TUI'nin bu
// katmanları DOĞRU tetiklediğini doğrular — bu, gelismis_test.go/
// yazimonerisi_test.go'nun kapsamadığı bir boşluktu (bkz. denetim).

import (
	"testing"
	"time"
)

func modelSayfalamaIcinDoldur(t *testing.T, m model, adet int) {
	t.Helper()
	for i := 0; i < adet; i++ {
		id := "p" + string(rune('a'+i))
		if err := m.depo.Ekle(AramaSonucu{
			Tip: TipKonu, KaynakID: id, Baslik: "ortak konu", Govde: "gövde metni",
			YazarAd: "a", OlusturmaTS: fixedTime(i),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModel_KeyArama_PgDownSonrakiVarMiFalseKenNoOp(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	m.sonrakiVarMi = false
	m.sayfaNo = 0

	next, cmd := m.Update(keyMsg("pgdown"))
	m2 := next.(model)
	if m2.sayfaNo != 0 {
		t.Fatalf("sonrakiVarMi=false iken sayfaNo değişmemeliydi, got %d", m2.sayfaNo)
	}
	if cmd != nil {
		t.Fatal("sonrakiVarMi=false iken bir Cmd üretilmemeliydi")
	}
}

func TestModel_KeyArama_PgUpSayfaSifirdayKenNoOp(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	m.sayfaNo = 0

	next, cmd := m.Update(keyMsg("pgup"))
	m2 := next.(model)
	if m2.sayfaNo != 0 {
		t.Fatalf("sayfaNo 0'dan aşağı inmemeliydi, got %d", m2.sayfaNo)
	}
	if cmd != nil {
		t.Fatal("sayfaNo=0 iken pgup için bir Cmd üretilmemeliydi")
	}
}

// TestModel_KeyArama_PgDownPgUpGercekVeriyleSayfaDegistirir, 25 kayıtlık
// gerçek bir indeks üzerinde pgdown'ın ikinci sayfaya geçtiğini, pgup'ın
// birinciye döndüğünü — Cmd'nin GERÇEKTEN çalıştırılıp sonucun modele
// yansıtıldığını — doğrular.
func TestModel_KeyArama_PgDownPgUpGercekVeriyleSayfaDegistirir(t *testing.T) {
	m := testModel(t)
	modelSayfalamaIcinDoldur(t, m, 25)
	m.ekran = ekArama
	m.aramaIn.SetValue("ortak")

	// İlk sayfayı yükle (normalde bir tuş vuruşuyla tetiklenir; burada
	// doğrudan cmd'yi çalıştırıp mesajı işleyerek simüle ediyoruz).
	ilkSayfaMsg := yerelAramaCmd(m.depo, "ortak", m.filtre, m.yazarFiltre, m.siralama, m.sayfaNo)()
	next, _ := m.Update(ilkSayfaMsg)
	m = next.(model)
	if len(m.yerelSonuclar) != aramaSayfaBoyu || !m.sonrakiVarMi {
		t.Fatalf("ilk sayfa tam dolu (%d) ve sonrakiVarMi=true olmalıydı, got %d sonrakiVarMi=%v", aramaSayfaBoyu, len(m.yerelSonuclar), m.sonrakiVarMi)
	}
	ilkSayfaIlkID := m.yerelSonuclar[0].KaynakID

	next, cmd := m.Update(keyMsg("pgdown"))
	m = next.(model)
	if m.sayfaNo != 1 {
		t.Fatalf("pgdown sonrası sayfaNo=1 bekleniyordu, got %d", m.sayfaNo)
	}
	if cmd == nil {
		t.Fatal("pgdown bir Cmd üretmeliydi")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if len(m.yerelSonuclar) != 5 || m.sonrakiVarMi {
		t.Fatalf("ikinci (son) sayfada 5 sonuç ve sonrakiVarMi=false bekleniyordu, got %d sonrakiVarMi=%v", len(m.yerelSonuclar), m.sonrakiVarMi)
	}
	if m.yerelSonuclar[0].KaynakID == ilkSayfaIlkID {
		t.Fatal("ikinci sayfa birinciyle aynı sonuçları göstermemeliydi")
	}

	next, cmd = m.Update(keyMsg("pgup"))
	m = next.(model)
	if m.sayfaNo != 0 {
		t.Fatalf("pgup sonrası sayfaNo=0 bekleniyordu, got %d", m.sayfaNo)
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if m.yerelSonuclar[0].KaynakID != ilkSayfaIlkID {
		t.Fatal("pgup sonrası ilk sayfaya dönülmeliydi")
	}
}

// ─── ctrl+d: yazım önerisini uygula ─────────────────────────────────────────

func TestModel_KeyArama_CtrlDOneriYokkenTextinputaDuser(t *testing.T) {
	m := testModel(t)
	m.ekran = ekArama
	m.aramaIn.Focus()
	m.aramaIn.SetValue("merhaba")
	m.aramaIn.CursorEnd()
	m.yazimOnerisi = ""

	next, _ := m.Update(keyMsg("ctrl+d"))
	m2 := next.(model)
	// Öneri olmadığından özel dal devreye girmemeli — textinput'un
	// varsayılan "ileri sil" davranışı çalışır (imleç sonda olduğundan bu
	// durumda pratikte bir şey silmez), ama KRİTİK olan: aramaIn'in değeri
	// bir "öneri" ile DEĞİŞTİRİLMEMİŞ olmalı.
	if m2.aramaIn.Value() != "merhaba" {
		t.Fatalf("öneri yokken aramaIn değişmemeliydi, got %q", m2.aramaIn.Value())
	}
}

func TestModel_KeyArama_CtrlDOneriVarkenUygular(t *testing.T) {
	m := testModel(t)
	if err := m.depo.Ekle(AramaSonucu{Tip: TipKonu, KaynakID: "p1", Baslik: "kelebek bahçesi", Govde: "gövde", OlusturmaTS: time.Now()}); err != nil {
		t.Fatal(err)
	}
	m.ekran = ekArama
	m.aramaIn.Focus()
	m.aramaIn.SetValue("kelebk")
	m.yazimOnerisi = "kelebek"
	m.sayfaNo = 3 // uygulanınca sıfırlanmalı

	next, cmd := m.Update(keyMsg("ctrl+d"))
	m = next.(model)
	if m.aramaIn.Value() != "kelebek" {
		t.Fatalf("aramaIn öneriyle güncellenmeliydi, got %q", m.aramaIn.Value())
	}
	if m.sayfaNo != 0 {
		t.Fatalf("sayfaNo sıfırlanmalıydı, got %d", m.sayfaNo)
	}
	if cmd == nil {
		t.Fatal("uygulanan öneri yeni bir arama Cmd'i tetiklemeliydi")
	}
	msg, ok := cmd().(yerelAramaSonucuMsg)
	if !ok {
		t.Fatalf("beklenen mesaj tipi yerelAramaSonucuMsg, got %T", cmd())
	}
	if msg.sorgu != "kelebek" {
		t.Fatalf("yeni arama düzeltilmiş sorguyla yapılmalıydı, got %q", msg.sorgu)
	}
	if len(msg.sayfa.Sonuclar) != 1 {
		t.Fatalf("düzeltilmiş sorgu 1 sonuç bulmalıydı, got %d", len(msg.sayfa.Sonuclar))
	}
}
