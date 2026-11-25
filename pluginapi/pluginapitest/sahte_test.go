package pluginapitest

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lucian95511/and/pluginapi"
)

func TestKimlikVeEsler(t *testing.T) {
	and := Yeni().Ornek()
	c := and.Test(t)

	id, err := c.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if id.Name != VarsayilanKimlik.Ad || id.PeerID != VarsayilanKimlik.PeerID {
		t.Fatalf("kimlik = %+v", id)
	}
	esler, err := c.Peers()
	if err != nil {
		t.Fatal(err)
	}
	andSayisi := 0
	for _, e := range esler {
		if !pluginapi.GecerliPeerID(e.PeerID) {
			t.Errorf("örnek eşin Peer ID'si geçersiz: %q", e.PeerID)
		}
		if e.AND {
			andSayisi++
		}
	}
	if len(esler) != 3 || andSayisi != 2 {
		t.Fatalf("eşler = %+v", esler)
	}
	if v, err := c.APIVersion(); err != nil || v != pluginapi.APIVersion {
		t.Fatalf("API sürümü = %d", v)
	}
}

// Manifest'te istenmeyen bir izin gerçek AND'daki gibi 403 dönmeli: eklenti
// yazarı eksik izni testte görür.
func TestIzinsizYazma403(t *testing.T) {
	and := Yeni()
	c := and.Test(t) // izin yok
	err := c.CreatePost("Genel", "başlık", "gövde")
	if !pluginapi.IsForbidden(err) {
		t.Fatalf("izinsiz konu açmak 403 dönmeli, dönen: %v", err)
	}
	if len(and.Konular()) != 0 {
		t.Fatal("izinsiz istek konu açmamalı")
	}
}

func TestKonuVeYanit(t *testing.T) {
	and := Yeni()
	c := and.Test(t, pluginapi.IzinForum)
	if err := c.CreatePost("Genel", "Merhaba", "ilk konu"); err != nil {
		t.Fatal(err)
	}
	konular := and.Konular()
	if len(konular) != 1 || konular[0].AuthorName != VarsayilanKimlik.Ad {
		t.Fatalf("konular = %+v", konular)
	}
	if err := c.CreateReply(konular[0].ID, "yanıt"); err != nil {
		t.Fatal(err)
	}
	d, err := c.GetPost(konular[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Merhaba" || len(d.Replies) != 1 || d.Replies[0].Body != "yanıt" {
		t.Fatalf("konu ayrıntısı = %+v", d)
	}
}

// Eski kategori adları (ör. "genel") gerçek ağda reddedilir; sahte AND da
// reddetmeli.
func TestBilinmeyenKategori(t *testing.T) {
	and := Yeni()
	c := and.Test(t, pluginapi.IzinForum)
	if err := c.CreatePost("genel", "x", "y"); err == nil {
		t.Fatal("bilinmeyen kategori kabul edildi")
	}
}

func TestOzelMesaj(t *testing.T) {
	and := Yeni().Ornek()
	c := and.Test(t, pluginapi.IzinDM)
	esler, _ := c.Peers()
	hedef := esler[0].PeerID
	if err := c.SendDM(hedef, "selam"); err != nil {
		t.Fatal(err)
	}
	giden := and.GidenDMler()
	if len(giden) != 1 || giden[0].PeerID != hedef || giden[0].Text != "selam" {
		t.Fatalf("giden = %+v", giden)
	}
	// AND olmayan eşe doğrudan gönderilemez, kuyruğa alınabilir.
	if err := c.SendDM(esler[2].PeerID, "x"); err == nil {
		t.Fatal("AND olmayan eşe doğrudan DM gönderildi")
	}
	if err := c.SendDMQueued(esler[2].PeerID, "x"); err != nil {
		t.Fatal(err)
	}

	gecmis, err := c.DMHistory()
	if err != nil {
		t.Fatal(err)
	}
	// Ornek'in gelen mesajı + iki giden.
	if len(gecmis) != 3 || gecmis[0].Outgoing || !gecmis[2].Pending {
		t.Fatalf("geçmiş = %+v", gecmis)
	}

	// Canlı akış: PollDM beklerken gelen mesaj.
	sonuc := make(chan []pluginapi.DMMsg, 1)
	go func() {
		m, _ := c.PollDM()
		sonuc <- m
	}()
	bitis := time.Now().Add(3 * time.Second)
	for {
		and.mu.Lock()
		abone := len(and.dmAbone)
		and.mu.Unlock()
		if abone > 0 || time.Now().After(bitis) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	and.DMGelsin(hedef, "ayse", "canlı")
	m := <-sonuc
	if len(m) != 1 || m[0].Text != "canlı" || m[0].VerifiedPeerID != hedef {
		t.Fatalf("poll = %+v", m)
	}

	if err := c.MarkDMRead(hedef, time.Now()); err != nil {
		t.Fatal(err)
	}
	isaretler, err := c.DMReadMarks()
	if err != nil || isaretler[hedef].IsZero() {
		t.Fatalf("okundu = %v, %v", isaretler, err)
	}
}

func TestSohbetSiteTema(t *testing.T) {
	and := Yeni()
	c := and.Test(t, pluginapi.IzinSohbet, pluginapi.IzinSite, pluginapi.IzinTema)

	if err := c.SendChat("herkese selam"); err != nil {
		t.Fatal(err)
	}
	if g := and.SohbetGecmisi(); len(g) != 1 || g[0].From != VarsayilanKimlik.Ad {
		t.Fatalf("sohbet = %+v", g)
	}

	if err := c.PublishSitePage("hakkimda", "Hakkımda", "merhaba"); err != nil {
		t.Fatal(err)
	}
	if s := and.SiteSayfalari(); len(s) != 1 || s[0].Slug != "hakkimda" {
		t.Fatalf("sayfalar = %+v", s)
	}

	if err := c.SetTheme(pluginapi.ThemeReq{ThemeName: "Orman"}); err != nil {
		t.Fatal(err)
	}
	bitis := time.Now().Add(3 * time.Second)
	for len(and.TemaIstekleri()) == 0 && time.Now().Before(bitis) {
		time.Sleep(5 * time.Millisecond)
	}
	if ist := and.TemaIstekleri(); len(ist) != 1 || ist[0].ThemeName != "Orman" {
		t.Fatalf("tema istekleri = %+v", ist)
	}
}

// Test, eklentinin kendi koduyla (NewClientFromEnv) bağlanabilmesi için
// ortam değişkenlerini ayarlamalı.
func TestOrtamDegiskenleri(t *testing.T) {
	and := Yeni()
	and.Test(t)
	c, err := pluginapi.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Identity(); err != nil {
		t.Fatal(err)
	}
	if pluginapi.DataDir() == "" || os.Getenv("AND_API_TOKEN") == "" {
		t.Fatal("AND_DATA_DIR / AND_API_TOKEN ayarlanmadı")
	}
}

func TestYasakVeModerator(t *testing.T) {
	s := Yeni()
	k := VarsayilanKimlik
	k.Kurucu = true
	s.KimlikAyarla(k)
	c := s.Test(t, pluginapi.IzinModerasyon)
	hedef := strings.Repeat("ab", 32)
	if err := c.Ban(hedef, time.Hour, false, "spam"); err != nil {
		t.Fatal(err)
	}
	if b, _ := c.AllBans(); len(b) != 1 || b[0].AuthorKey != hedef || b[0].Reason != "spam" || b[0].Permanent {
		t.Fatalf("yasaklar: %+v", b)
	}
	if err := c.Ban(k.PubKey, time.Hour, false, "x"); !pluginapi.IsForbidden(err) {
		t.Fatalf("kendini yasaklamak 403 olmalı: %v", err)
	}
	if err := c.Unban(hedef); err != nil || len(s.Yasaklar()) != 0 {
		t.Fatalf("unban: %v %+v", err, s.Yasaklar())
	}
	if g, _ := c.ModHistory(); len(g) != 2 || g[0].Action != "unban" {
		t.Fatalf("geçmiş: %+v", g)
	}
	if _, err := c.GrantModerator(hedef, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeModerator(hedef); err != nil {
		t.Fatal(err)
	}
	if m := s.Moderatorler(); len(m) != 1 || !m[0].Revoked || !m[0].Permanent {
		t.Fatalf("moderatörler: %+v", m)
	}
}

func TestForumEklentisiUclari(t *testing.T) {
	s := Yeni()
	c := s.Test(t, pluginapi.IzinForum)
	baskasi := s.KonuEkle("Genel", "Selam", "gövde", "ayse")
	s.YanitEkle(baskasi, "yanıt", "mert")
	konular, err := c.ForumPosts()
	if err != nil || len(konular) != 1 || konular[0].ReplyCount != 1 || konular[0].LastActivity.Before(konular[0].CreatedAt) {
		t.Fatalf("ForumPosts: %+v %v", konular, err)
	}
	d, err := c.ForumChanges(0)
	if err != nil || d.Surum != 2 || len(d.Konular) != 1 || d.Konular[0] != baskasi {
		t.Fatalf("ForumChanges(0): %+v %v", d, err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.YanitEkle(baskasi, "bir yanıt daha", "mert")
	}()
	if d, err = c.ForumChanges(2); err != nil || d.Surum != 3 {
		t.Fatalf("bekleyen ForumChanges: %+v %v", d, err)
	}
	if err := c.DeleteOwnPost(baskasi); !pluginapi.IsForbidden(err) {
		t.Fatalf("başkasının konusu 403 olmalı: %v", err)
	}
	if err := c.CreatePost("Genel", "Benim", "gövde"); err != nil {
		t.Fatal(err)
	}
	var benim string
	for _, k := range s.Konular() {
		if k.Title == "Benim" {
			benim = k.ID
		}
	}
	if err := c.DeleteOwnPost(benim); err != nil || len(s.Konular()) != 1 {
		t.Fatalf("kendi konusu silinmeli: %v %d", err, len(s.Konular()))
	}
	p, err := c.Profile(konular[0].AuthorKey)
	if err != nil || p.Name != "ayse" || p.PostCount != 1 {
		t.Fatalf("Profile: %+v %v", p, err)
	}
	if _, err := c.Profile(strings.Repeat("ee", 32)); !pluginapi.IsNotFound(err) {
		t.Fatalf("bilinmeyen yazar 404 olmalı: %v", err)
	}
}
