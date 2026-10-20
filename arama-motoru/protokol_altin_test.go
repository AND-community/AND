package main

import (
	"encoding/json"
	"testing"

	"github.com/lucian95511/and/protokol/altin"
)

// Bu testler AND'ın kendi kodunun ürettiği örnek iletileri (protokol/testdata)
// bu programın türleri ve imza doğrulamasıyla işler. AND'ın ileti biçimi ya da
// imza yükü değişip burası güncellenmezse burada kırılır; önceden yalnızca
// "bire bir aynı olmalı" yorumları vardı.

func altinDepo(t *testing.T) *Depo {
	t.Helper()
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Kapat() }) //nolint:errcheck
	return d
}

func sayim(t *testing.T, d *Depo) int {
	t.Helper()
	n, err := d.SayimAl()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAltin_ForumIletileri(t *testing.T) {
	var konu, yanit, silme forumZarf
	for ad, hedef := range map[string]*forumZarf{"forum_konu.json": &konu, "forum_yanit.json": &yanit, "forum_silme.json": &silme} {
		if err := json.Unmarshal(altin.Oku(t, ad), hedef); err != nil {
			t.Fatalf("%s çözülemedi: %v", ad, err)
		}
	}
	if konu.Post == nil || !verifyPost(konu.Post) {
		t.Fatal("AND'ın imzaladığı konu burada doğrulanmalı")
	}
	if yanit.Reply == nil || !verifyReply(yanit.Reply) {
		t.Fatal("AND'ın imzaladığı yanıt burada doğrulanmalı")
	}
	if silme.Delete == nil || !verifyDelete(silme.Delete) {
		t.Fatal("AND'ın imzaladığı silme burada doğrulanmalı")
	}

	kurcalanmis := *konu.Post
	kurcalanmis.Body += "!"
	if verifyPost(&kurcalanmis) {
		t.Fatal("değiştirilmiş konu doğrulanmamalı")
	}

	d := altinDepo(t)
	forumMesajIsle(d, altin.Oku(t, "forum_konu.json"))
	forumMesajIsle(d, altin.Oku(t, "forum_yanit.json"))
	if n := sayim(t, d); n != 2 {
		t.Fatalf("konu ve yanıt indekslenmeli, %d kayıt", n)
	}
	sonuc, err := d.Ara("Protokol", 10)
	if err != nil || len(sonuc) == 0 || sonuc[0].KaynakID != konu.Post.ID {
		t.Fatalf("konu aranabilmeli: %+v %v", sonuc, err)
	}
	forumMesajIsle(d, altin.Oku(t, "forum_silme.json"))
	if sonuc, _ := d.Ara("Protokol", 10); len(sonuc) != 0 {
		t.Fatalf("yazarın imzalı silmesi konuyu kaldırmalı: %+v", sonuc)
	}
}

func TestAltin_SohbetMesaji(t *testing.T) {
	var m ChatMessage
	if err := json.Unmarshal(altin.Oku(t, "sohbet_mesaji.json"), &m); err != nil {
		t.Fatal(err)
	}
	if !m.imzali() || !verifyChat(&m) {
		t.Fatal("AND'ın imzaladığı sohbet mesajı burada doğrulanmalı")
	}
	d := altinDepo(t)
	chatMesajIsle(d, altin.Oku(t, "sohbet_mesaji.json"))
	if sonuc, err := d.Ara("herkes", 10); err != nil || len(sonuc) != 1 || sonuc[0].KaynakID != m.ID {
		t.Fatalf("sohbet mesajı aranabilmeli: %+v %v", sonuc, err)
	}
}

func TestAltin_SiteIletileri(t *testing.T) {
	var sayfa, silme siteZarf
	if err := json.Unmarshal(altin.Oku(t, "site_sayfa.json"), &sayfa); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(altin.Oku(t, "site_silme.json"), &silme); err != nil {
		t.Fatal(err)
	}
	if sayfa.Page == nil || !verifySitePage(sayfa.Page) {
		t.Fatal("AND'ın imzaladığı sayfa burada doğrulanmalı (JSON kaçışları dahil)")
	}
	if silme.Delete == nil || !verifySiteDelete(silme.Delete) {
		t.Fatal("AND'ın imzaladığı sayfa silmesi burada doğrulanmalı")
	}
	d := altinDepo(t)
	siteMesajIsle(d, altin.Oku(t, "site_sayfa.json"))
	if sonuc, err := d.Ara("işaretler", 10); err != nil || len(sonuc) != 1 {
		t.Fatalf("site sayfası aranabilmeli: %+v %v", sonuc, err)
	}
}
