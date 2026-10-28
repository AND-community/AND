package storage

// yedek_test.go — veritabanı kaybına karşı korumalar: işlem içinde
// migrasyon, yükseltme öncesi ve günlük yedek, bozuk dosyadan kurtarma.

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMigrate_HataliAdimYarimKalmaz: bir migrasyon adımının ikinci ifadesi
// başarısız olursa ilk ifadenin değişikliği de geri alınır ve adım
// uygulanmış sayılmaz. (Eskiden adımlar işlem dışında çalışıyordu: yarım
// kalan bir adım veriyi değiştirmiş ama sürümünü yazmamış olabiliyordu.)
func TestMigrate_HataliAdimYarimKalmaz(t *testing.T) {
	eski := migrations
	t.Cleanup(func() { migrations = eski })
	migrations = []migration{
		{version: 1, sql: `CREATE TABLE sayac (n INTEGER NOT NULL)`},
		{version: 2, sql: `INSERT INTO sayac(n) VALUES (1); INSERT INTO olmayan_tablo(n) VALUES (2);`},
	}

	yol := filepath.Join(t.TempDir(), "forum.db")
	if db, err := Open(yol); err == nil {
		db.Close()
		t.Fatal("hatalı migrasyonla Open başarılı olmamalı")
	}

	ham, err := sql.Open("sqlite", dsn(yol))
	if err != nil {
		t.Fatal(err)
	}
	defer ham.Close()
	var n int
	if err := ham.QueryRow(`SELECT COUNT(*) FROM sayac`).Scan(&n); err != nil {
		t.Fatalf("sayac okunamadı: %v", err)
	}
	if n != 0 {
		t.Errorf("başarısız adımın ilk ifadesi geri alınmalıydı; sayac'ta %d satır var", n)
	}
	var v2 int
	if err := ham.QueryRow(`SELECT COUNT(*) FROM schema_versions WHERE version=2`).Scan(&v2); err != nil {
		t.Fatal(err)
	}
	if v2 != 0 {
		t.Error("başarısız adım uygulanmış olarak kaydedilmemeli")
	}
}

// eskiSemaliVeritabani, yalnızca v1'in uygulandığı ve bir konu içeren eski
// bir veritabanı kurar (bkz. TestMigrate_LegacyV1UpgradesCleanly).
func eskiSemaliVeritabani(t *testing.T, yol string) {
	t.Helper()
	ham, err := sql.Open("sqlite", dsn(yol))
	if err != nil {
		t.Fatal(err)
	}
	defer ham.Close()
	if _, err := ham.Exec(`
		CREATE TABLE schema_versions (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
		CREATE TABLE posts (
			id TEXT PRIMARY KEY, category TEXT NOT NULL, author_name TEXT NOT NULL,
			author_key TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL,
			created_at INTEGER NOT NULL, sig TEXT NOT NULL
		);
		CREATE TABLE replies (
			id TEXT PRIMARY KEY, post_id TEXT NOT NULL, author_name TEXT NOT NULL,
			author_key TEXT NOT NULL, body TEXT NOT NULL, created_at INTEGER NOT NULL, sig TEXT NOT NULL
		);
		CREATE TABLE tombstones (post_id TEXT PRIMARY KEY, msg_json TEXT NOT NULL);
		INSERT INTO schema_versions(version, applied_at) VALUES (1, 0);
		INSERT INTO posts(id, category, author_name, author_key, title, body, created_at, sig)
			VALUES ('eski1', 'Go', 'alice', 'aabbcc', 'Eski Konu', 'Eski icerik', 1000, 'sig1');
	`); err != nil {
		t.Fatalf("eski şema kurulamadı: %v", err)
	}
}

func yedekler(t *testing.T, dizin string) []string {
	t.Helper()
	girdiler, err := os.ReadDir(filepath.Join(dizin, YedekDizini))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, g := range girdiler {
		out = append(out, g.Name())
	}
	return out
}

// TestOpen_YukseltmedenOnceYedekAlir: var olan bir veritabanına yeni
// migrasyonlar uygulanmadan önce, yükseltilmemiş hâlinin yedeği alınır —
// hatalı bir migrasyon (ör. v6'nın bütün konulara süre ataması gibi geri
// dönüşsüz bir değişiklik) bu yedekten geri alınabilir.
func TestOpen_YukseltmedenOnceYedekAlir(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "forum.db")
	eskiSemaliVeritabani(t, yol)

	db, err := Open(yol)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()

	adlar := yedekler(t, dizin)
	if len(adlar) != 1 || !strings.HasPrefix(adlar[0], "forum-v1-oncesi-") || !strings.HasSuffix(adlar[0], ".db") {
		t.Fatalf("yükseltme öncesi tek bir yedek bekleniyordu, bulunan: %v", adlar)
	}
	ham, err := sql.Open("sqlite", dsn(filepath.Join(dizin, YedekDizini, adlar[0])))
	if err != nil {
		t.Fatal(err)
	}
	defer ham.Close()
	var enBuyuk int
	if err := ham.QueryRow(`SELECT MAX(version) FROM schema_versions`).Scan(&enBuyuk); err != nil {
		t.Fatal(err)
	}
	if enBuyuk != 1 {
		t.Errorf("yedek yükseltilmemiş (v1) hâli içermeli; en büyük sürüm %d", enBuyuk)
	}
	var baslik string
	if err := ham.QueryRow(`SELECT title FROM posts WHERE id='eski1'`).Scan(&baslik); err != nil || baslik != "Eski Konu" {
		t.Errorf("yedekteki konu %q (hata %v)", baslik, err)
	}
}

func TestOpen_YeniVeritabanindaYedekYok(t *testing.T) {
	dizin := t.TempDir()
	db, err := Open(filepath.Join(dizin, "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if adlar := yedekler(t, dizin); len(adlar) != 0 {
		t.Errorf("yeni bir veritabanı için yedek alınmamalı: %v", adlar)
	}
}

// TestGunlukYedek: yedek geçerli bir veritabanıdır; 24 saat dolmadan
// ikincisi alınmaz; en yeni üç günlük yedek tutulur.
func TestGunlukYedek(t *testing.T) {
	dizin := t.TempDir()
	db, err := Open(filepath.Join(dizin, "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InsertPost(samplePost("p1")); err != nil {
		t.Fatal(err)
	}

	ilk, err := db.GunlukYedek()
	if err != nil || ilk == "" {
		t.Fatalf("ilk günlük yedek alınamadı: %q, %v", ilk, err)
	}
	kopya, err := Open(ilk)
	if err != nil {
		t.Fatalf("yedek açılamadı: %v", err)
	}
	posts, err := kopya.AllPosts()
	kopya.Close()
	if err != nil || len(posts) != 1 || posts[0].ID != "p1" {
		t.Fatalf("yedekteki konular %+v (hata %v)", posts, err)
	}

	if ikinci, err := db.GunlukYedek(); err != nil || ikinci != "" {
		t.Fatalf("24 saat dolmadan yeni yedek alınmamalı: %q, %v", ikinci, err)
	}

	// Var olan yedekleri eskit: her çağrıda yeni bir yedek alınır, en
	// yeni üçü kalır.
	for i := range 4 {
		for _, ad := range yedekler(t, dizin) {
			y := filepath.Join(dizin, YedekDizini, ad)
			bilgi, err := os.Stat(y)
			if err != nil {
				t.Fatal(err)
			}
			eskiZaman := bilgi.ModTime().Add(-25 * time.Hour)
			if err := os.Chtimes(y, eskiZaman, eskiZaman); err != nil {
				t.Fatal(err)
			}
		}
		if yeni, err := db.GunlukYedek(); err != nil || yeni == "" {
			t.Fatalf("%d. eskitmeden sonra yedek alınmalıydı: %q, %v", i+1, yeni, err)
		}
	}
	if adlar := yedekler(t, dizin); len(adlar) != 3 {
		t.Errorf("en yeni 3 günlük yedek kalmalı, bulunan: %v", adlar)
	}
}

// bozukYaz, veritabanı dosyasının başlığını çöpe çevirir: SQLite onu artık
// veritabanı olarak tanımaz ("file is not a database").
func bozukYaz(t *testing.T, yol string) {
	t.Helper()
	if err := os.WriteFile(yol, []byte(strings.Repeat("bu bir veritabanı değil ", 400)), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(yol + "-wal")
	_ = os.Remove(yol + "-shm")
}

// TestOpen_BozukDosyadaKenaraAlipBaslar: açılamayan bir veritabanı AND'ın
// açılmasını engellemez (eskiden "init forum" hatasıyla çıkıyordu; kullanıcı
// dosyayı elle silmek zorunda kalıyordu). Bozuk dosya silinmez, kenara
// alınır.
func TestOpen_BozukDosyadaKenaraAlipBaslar(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "forum.db")
	bozukYaz(t, yol)

	db, err := Open(yol)
	if err != nil {
		t.Fatalf("bozuk veritabanı kurtarılmalıydı: %v", err)
	}
	defer db.Close()
	k := db.Kurtarma()
	if k == nil {
		t.Fatal("kurtarma raporu boş")
	}
	if k.Yedek != "" {
		t.Errorf("yedek yokken boş başlanmalıydı; geri yüklenen %q", k.Yedek)
	}
	if veri, err := os.ReadFile(k.BozukDosya); err != nil || !strings.HasPrefix(string(veri), "bu bir veritabanı değil") {
		t.Errorf("bozuk dosya kenarda olduğu gibi durmalı (%s): %v", k.BozukDosya, err)
	}
	if err := db.InsertPost(samplePost("yeni")); err != nil {
		t.Errorf("kurtarılan veritabanı kullanılabilir olmalı: %v", err)
	}
}

func TestOpen_SaglamVeritabanindaKurtarmaYok(t *testing.T) {
	db := openTestDB(t)
	if k := db.Kurtarma(); k != nil {
		t.Errorf("sağlam veritabanında kurtarma raporu olmamalı: %+v", k)
	}
}

// TestOpen_BozuktaEnYeniYedegiGeriYukler: bozuk veritabanının yerine en
// yeni yedek konur.
func TestOpen_BozuktaEnYeniYedegiGeriYukler(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "forum.db")
	db, err := Open(yol)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertPost(samplePost("yedekte")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GunlukYedek(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	bozukYaz(t, yol)

	db, err = Open(yol)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	k := db.Kurtarma()
	if k == nil || k.Yedek == "" {
		t.Fatalf("yedek geri yüklenmeliydi: %+v", k)
	}
	posts, err := db.AllPosts()
	if err != nil || len(posts) != 1 || posts[0].ID != "yedekte" {
		t.Errorf("geri yüklenen konular %+v (hata %v)", posts, err)
	}
}

// TestOpen_BozuktanOkunabilenKayitlariKurtarir: yalnızca bir dizin sayfası
// bozuk olan bir veritabanında tablolar hâlâ okunabilir; kurtarma bu
// kayıtları yeni veritabanına aktarır. Mezar taşı olan konular aktarılsa
// bile geri dirilmez.
func TestOpen_BozuktanOkunabilenKayitlariKurtarir(t *testing.T) {
	dizin := t.TempDir()
	yol := filepath.Join(dizin, "forum.db")
	db, err := Open(yol)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"k1", "k2", "silinen"} {
		if err := db.InsertPost(samplePost(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertProfile(RawProfile{AuthorKey: "aabbcc", Bio: "kendi profilim", UpdatedAt: time.Now(), Sig: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertTombstone("silinen", `{"post_id":"silinen"}`); err != nil {
		t.Fatal(err)
	}
	var kok, sayfa int
	if err := db.db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE name='idx_posts_cat'`).Scan(&kok); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`PRAGMA page_size`).Scan(&sayfa); err != nil {
		t.Fatal(err)
	}
	db.Close() // WAL ana dosyaya işlenir

	f, err := os.OpenFile(yol, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(strings.Repeat("\xff", sayfa)), int64(kok-1)*int64(sayfa)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err = Open(yol)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	k := db.Kurtarma()
	if k == nil {
		t.Fatal("bozuk dizin sayfası fark edilmeliydi")
	}
	if k.Kurtarilan == 0 {
		t.Errorf("okunabilen kayıtlar aktarılmalıydı: %+v", k)
	}
	posts, err := db.AllPosts()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, p := range posts {
		ids[p.ID] = true
	}
	if !ids["k1"] || !ids["k2"] {
		t.Errorf("k1 ve k2 kurtarılmalıydı: %v", ids)
	}
	if ids["silinen"] {
		t.Error("mezar taşı olan konu geri dirilmemeli")
	}
	profiller, err := db.AllProfiles()
	if err != nil || len(profiller) != 1 || profiller[0].Bio != "kendi profilim" {
		t.Errorf("profil kurtarılmalıydı: %+v (hata %v)", profiller, err)
	}
}

// TestGunlukYedek_EskiYedeklerSilinir: yedekler sayıdan bağımsız olarak
// yaşlarına göre de silinir — günlükler üç, yükseltme öncesiler yedi günden
// eskiyse. Böylece süresi dolan ya da silinen içerik (konular 5 günde
// silinir) yedeklerde gizlilik bildiriminde yazandan uzun kalmaz.
func TestGunlukYedek_EskiYedeklerSilinir(t *testing.T) {
	dizin := t.TempDir()
	db, err := Open(filepath.Join(dizin, "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	yd := filepath.Join(dizin, YedekDizini)
	if err := os.MkdirAll(yd, 0o700); err != nil {
		t.Fatal(err)
	}
	eskit := func(ad string, yas time.Duration) string {
		y := filepath.Join(yd, ad)
		if err := os.WriteFile(y, []byte("eski yedek"), 0o600); err != nil {
			t.Fatal(err)
		}
		z := time.Now().Add(-yas)
		if err := os.Chtimes(y, z, z); err != nil {
			t.Fatal(err)
		}
		return y
	}
	cokEskiGunluk := eskit("forum-20260101-000000.db", 4*24*time.Hour)
	tazeSurum := eskit("forum-v12-oncesi-20260101-000000.db", 6*24*time.Hour)
	cokEskiSurum := eskit("forum-v11-oncesi-20260101-000000.db", 8*24*time.Hour)

	if _, err := db.GunlukYedek(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cokEskiGunluk); !os.IsNotExist(err) {
		t.Error("üç günden eski günlük yedek silinmeli")
	}
	if _, err := os.Stat(cokEskiSurum); !os.IsNotExist(err) {
		t.Error("yedi günden eski yükseltme öncesi yedek silinmeli")
	}
	if _, err := os.Stat(tazeSurum); err != nil {
		t.Errorf("yedi günden yeni yükseltme öncesi yedek kalmalı: %v", err)
	}
}
