package storage

// yedek.go — veritabanının kaybolmasına karşı korumalar.
//
// Forum, sohbet ve site içeriğinin çoğu eşlerden yeniden senkronlanabilir;
// ama kendi profilin (profillerin senkron protokolü yok), çevrimdışıyken
// yazdıkların ve hiçbir eşin almadığı her şey yalnızca bu dosyadadır. Bu
// yüzden:
//   - veritabanının yanındaki yedek/ dizinine günde bir yedek alınır (en
//     yeni üçü tutulur) ve var olan bir veritabanı yükseltilmeden önce de
//     yedeklenir (en yeni ikisi tutulur);
//   - açılışta PRAGMA quick_check çalışır; dosya bozuksa silinmez, kenara
//     alınır, en yeni sağlam yedek geri yüklenir ve bozuk dosyadan hâlâ
//     okunabilen kayıtlar aktarılır.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"modernc.org/sqlite"

	"github.com/lucian95511/and/internal/dosya"
)

// YedekDizini, yedeklerin veritabanının yanında tutulduğu alt dizindir.
const YedekDizini = "yedek"

const (
	gunlukYedekTut     = 3
	surumYedekTut      = 2
	gunlukYedekAraligi = 24 * time.Hour
	// Yedekler sayıdan bağımsız olarak bu yaşı geçince de silinir: süresi
	// dolan ya da silinen içerik (konular 5 günde silinir) yedeklerde
	// gizlilik bildiriminde yazandan uzun kalmasın (bkz. GIZLILIK.md).
	gunlukYedekOmru = 3 * 24 * time.Hour
	surumYedekOmru  = 7 * 24 * time.Hour
	zamanBicimi     = "20060102-150405"

	// SQLite birincil hata kodları (genişletilmiş kodların alt 8 biti).
	sqliteCorrupt = 11
	sqliteNotADB  = 26
)

// KurtarmaRaporu, Open'ın bozuk bir veritabanından nasıl kurtardığını
// anlatır.
type KurtarmaRaporu struct {
	BozukDosya string // kenara alınan bozuk dosya (silinmez)
	Yedek      string // geri yüklenen yedek; boşsa boş bir veritabanıyla başlandı
	Kurtarilan int    // bozuk dosyadan okunup aktarılabilen kayıt sayısı
	Sebep      string // bozulmanın açıklaması
}

// Kurtarma, Open bozuk bir dosyadan kurtardıysa raporunu, aksi hâlde nil
// döner.
func (s *DB) Kurtarma() *KurtarmaRaporu { return s.kurtarma }

// bozukHatasi, PRAGMA quick_check'in bulduğu bir bozulmadır.
type bozukHatasi struct{ ayrinti string }

func (e *bozukHatasi) Error() string { return "storage: veritabanı bozuk: " + e.ayrinti }

func (s *DB) saglamlikDenetle() error {
	var sonuc string
	if err := s.db.QueryRow(`PRAGMA quick_check(1)`).Scan(&sonuc); err != nil {
		return fmt.Errorf("storage: quick_check: %w", err)
	}
	if sonuc != "ok" {
		return &bozukHatasi{ayrinti: sonuc}
	}
	return nil
}

// bozukMu, hatanın dosyanın kendisindeki bir bozulmadan kaynaklandığını
// söyler. Kilit, izin, disk dolu gibi geçici ya da çevresel hatalar bozulma
// sayılmaz: onlarda dosyaya dokunulmaz.
func bozukMu(err error) bool {
	var b *bozukHatasi
	if errors.As(err, &b) {
		return true
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqliteCorrupt, sqliteNotADB:
			return true
		}
	}
	return false
}

// kurtar, bozuk veritabanını kenara alır ve yerine sağlam bir tane koyar.
func kurtar(path string, sebep error) (*DB, error) {
	bozukYol, err := dosya.KenaraAl(path, "bozuk")
	if err != nil {
		return nil, fmt.Errorf("storage: bozuk veritabanı kenara alınamadı: %w (bozulma: %v)", err, sebep)
	}
	// WAL dosyası hangi veritabanına ait olduğunu bilmez: yerinde kalırsa
	// yeni dosyaya uygulanıp onu bozardı. Bozuk kopyanın yanına taşınır ki
	// o kopya son yazılanlarla birlikte açılabilsin.
	if err := os.Rename(path+"-wal", bozukYol+"-wal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(path + "-wal")
	}
	_ = os.Remove(path + "-shm")

	rapor := &KurtarmaRaporu{BozukDosya: bozukYol, Sebep: sebep.Error()}
	var s *DB
	for _, aday := range yedekAdaylari(path) {
		if err := kopyala(aday, path); err != nil {
			continue
		}
		if acilan, err := ac(path); err == nil {
			s, rapor.Yedek = acilan, aday
			break
		}
		for _, ek := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(path + ek)
		}
	}
	if s == nil {
		acilan, err := ac(path)
		if err != nil {
			return nil, fmt.Errorf("storage: kurtarılan veritabanı açılamadı: %w (bozulma: %v)", err, sebep)
		}
		s = acilan
	}
	rapor.Kurtarilan = s.aktar(bozukYol)
	s.kurtarma = rapor
	return s, nil
}

// aktarimIfadeleri, bozuk veritabanından okunabilen kayıtları aktarır.
// Değişmez kayıtlar (konu, yanıt, mesaj, mezar taşı) zaten varsa atlanır;
// profil ve site sayfalarında yenisi kazanır.
var aktarimIfadeleri = []string{
	`INSERT OR IGNORE INTO main.posts(id,category,author_name,author_key,title,body,created_at,sig,approved,expires_at,permanent_req)
	 SELECT id,category,author_name,author_key,title,body,created_at,sig,approved,expires_at,permanent_req FROM bozuk.posts`,
	`INSERT OR IGNORE INTO main.replies(id,post_id,author_name,author_key,body,created_at,sig)
	 SELECT id,post_id,author_name,author_key,body,created_at,sig FROM bozuk.replies`,
	`INSERT OR IGNORE INTO main.tombstones(post_id,msg_json) SELECT post_id,msg_json FROM bozuk.tombstones`,
	`INSERT INTO main.profiles(author_key,bio,status,tags,link,github_user,updated_at,mod_cert,sig)
	 SELECT author_key,bio,status,tags,link,github_user,updated_at,mod_cert,sig FROM bozuk.profiles WHERE true
	 ON CONFLICT(author_key) DO UPDATE SET bio=excluded.bio, status=excluded.status, tags=excluded.tags,
	 	link=excluded.link, github_user=excluded.github_user, updated_at=excluded.updated_at,
	 	mod_cert=excluded.mod_cert, sig=excluded.sig
	 WHERE excluded.updated_at > profiles.updated_at`,
	`INSERT OR IGNORE INTO main.chat_messages(id,author_key,author_name,text,sent_at,expires_at,sig)
	 SELECT id,author_key,author_name,text,sent_at,expires_at,sig FROM bozuk.chat_messages`,
	`INSERT INTO main.site_pages(author_key,slug,title,body,updated_at,sig)
	 SELECT author_key,slug,title,body,updated_at,sig FROM bozuk.site_pages WHERE true
	 ON CONFLICT(author_key,slug) DO UPDATE SET title=excluded.title, body=excluded.body,
	 	updated_at=excluded.updated_at, sig=excluded.sig
	 WHERE excluded.updated_at > site_pages.updated_at`,
	`INSERT OR IGNORE INTO main.site_tombstones(author_key,slug,msg_json)
	 SELECT author_key,slug,msg_json FROM bozuk.site_tombstones`,
}

// aktar, bozukYol'daki veritabanından okunabilen kayıtları bu veritabanına
// aktarır ve aktarılan satır sayısını döner. Her tablo ayrı denenir:
// okunamayan bir tablo ötekileri engellemez. Dosya hiç açılamıyorsa 0 döner.
func (s *DB) aktar(bozukYol string) int {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx) // ATTACH bağlantıya özgüdür
	if err != nil {
		return 0
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS bozuk`, bozukYol); err != nil {
		return 0
	}
	defer conn.ExecContext(ctx, `DETACH DATABASE bozuk`) //nolint:errcheck

	toplam := 0
	for _, ifade := range aktarimIfadeleri {
		sonuc, err := conn.ExecContext(ctx, ifade)
		if err != nil {
			continue
		}
		if n, err := sonuc.RowsAffected(); err == nil {
			toplam += int(n)
		}
	}
	// Yedek, bozuk dosyadan gelen bir mezar taşından eski olabilir: silinmiş
	// bir konu ya da sayfa geri dirilmesin.
	for _, ifade := range []string{
		`DELETE FROM replies WHERE post_id IN (SELECT post_id FROM tombstones)`,
		`DELETE FROM posts WHERE id IN (SELECT post_id FROM tombstones)`,
		`DELETE FROM site_pages WHERE EXISTS (SELECT 1 FROM site_tombstones t
			WHERE t.author_key = site_pages.author_key AND t.slug = site_pages.slug)`,
	} {
		_, _ = conn.ExecContext(ctx, ifade)
	}
	return toplam
}

// Yedekle, veritabanının tutarlı bir anlık görüntüsünü hedef'e yazar
// (VACUUM INTO). Önce geçici bir dosyaya yazılıp diske işlenir, sonra
// yerine taşınır: yarıda kalan bir yedek geçerli bir yedek gibi görünmez.
func (s *DB) Yedekle(hedef string) error {
	gecici := hedef + dosya.GeciciEk
	_ = os.Remove(gecici)
	if _, err := s.db.Exec(`VACUUM INTO ?`, gecici); err != nil {
		_ = os.Remove(gecici)
		return fmt.Errorf("storage: yedek: %w", err)
	}
	if err := diskeIsle(gecici); err != nil {
		_ = os.Remove(gecici)
		return fmt.Errorf("storage: yedek: %w", err)
	}
	if err := os.Rename(gecici, hedef); err != nil {
		_ = os.Remove(gecici)
		return fmt.Errorf("storage: yedek: %w", err)
	}
	return nil
}

// GunlukYedek, en yeni günlük yedek bir günden eskiyse (ya da hiç yoksa)
// yeni bir yedek alır; en yeni üç günlük yedeği tutar, ömrünü dolduran
// yedekleri siler. Yedek alındıysa yolunu, vakti gelmemişse "" döner.
func (s *DB) GunlukYedek() (string, error) {
	defer s.buda(surumOncesiMu, surumYedekTut, surumYedekOmru)
	if en, ok := enYeni(s.yedekler(gunlukMu)); ok && time.Since(en.zaman) < gunlukYedekAraligi {
		return "", nil
	}
	hedef, err := s.yedekAl(s.tabanAd() + "-" + time.Now().Format(zamanBicimi))
	if err != nil {
		return "", err
	}
	s.buda(gunlukMu, gunlukYedekTut, gunlukYedekOmru)
	return hedef, nil
}

func (s *DB) surumOncesiYedek(surum int) error {
	if _, err := s.yedekAl(fmt.Sprintf("%s-v%d-oncesi-%s", s.tabanAd(), surum, time.Now().Format(zamanBicimi))); err != nil {
		return err
	}
	s.buda(surumOncesiMu, surumYedekTut, surumYedekOmru)
	return nil
}

// yedekAl, yedek dizinine ad.db adında (o ad alınmışsa ad-2.db, …) bir
// yedek yazar.
func (s *DB) yedekAl(ad string) (string, error) {
	dizin := s.yedekDizini()
	if err := os.MkdirAll(dizin, 0o700); err != nil {
		return "", fmt.Errorf("storage: yedek dizini: %w", err)
	}
	dosya.ArtiklariTemizle(dizin)
	hedef := filepath.Join(dizin, ad+".db")
	for n := 2; ; n++ {
		if _, err := os.Lstat(hedef); errors.Is(err, os.ErrNotExist) {
			break
		}
		hedef = filepath.Join(dizin, fmt.Sprintf("%s-%d.db", ad, n))
	}
	if err := s.Yedekle(hedef); err != nil {
		return "", err
	}
	return hedef, nil
}

func (s *DB) yedekDizini() string { return filepath.Join(filepath.Dir(s.yol), YedekDizini) }

func (s *DB) tabanAd() string { return tabanAd(s.yol) }

func tabanAd(yol string) string {
	ad := filepath.Base(yol)
	return strings.TrimSuffix(ad, filepath.Ext(ad))
}

type yedekDosyasi struct {
	yol   string
	zaman time.Time
}

// gunlukMu / surumOncesiMu, yedek dizinindeki bir adın taban'ın günlük ya
// da yükseltme öncesi yedeği olup olmadığını söyler.
func gunlukMu(taban, ad string) bool {
	geri, ok := strings.CutPrefix(ad, taban+"-")
	return ok && strings.HasSuffix(ad, ".db") && geri != "" && geri[0] >= '0' && geri[0] <= '9'
}

func surumOncesiMu(taban, ad string) bool {
	return strings.HasPrefix(ad, taban+"-v") && strings.Contains(ad, "-oncesi-") && strings.HasSuffix(ad, ".db")
}

func yedekleriListele(dizin, taban string, tur func(taban, ad string) bool) []yedekDosyasi {
	girdiler, err := os.ReadDir(dizin)
	if err != nil {
		return nil
	}
	var out []yedekDosyasi
	for _, g := range girdiler {
		if g.IsDir() || !tur(taban, g.Name()) {
			continue
		}
		bilgi, err := g.Info()
		if err != nil {
			continue
		}
		out = append(out, yedekDosyasi{yol: filepath.Join(dizin, g.Name()), zaman: bilgi.ModTime()})
	}
	// En yenisi başta.
	slices.SortFunc(out, func(a, b yedekDosyasi) int { return b.zaman.Compare(a.zaman) })
	return out
}

func (s *DB) yedekler(tur func(taban, ad string) bool) []yedekDosyasi {
	return yedekleriListele(s.yedekDizini(), s.tabanAd(), tur)
}

func enYeni(yedekler []yedekDosyasi) (yedekDosyasi, bool) {
	if len(yedekler) == 0 {
		return yedekDosyasi{}, false
	}
	return yedekler[0], true
}

// buda, türün en yeni tut yedeği dışındakileri ve omur'den eski olanları
// siler.
func (s *DB) buda(tur func(taban, ad string) bool, tut int, omur time.Duration) {
	for i, y := range s.yedekler(tur) {
		if i < tut && time.Since(y.zaman) <= omur {
			continue
		}
		for _, ek := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(y.yol + ek)
		}
	}
}

// yedekAdaylari, path için geri yüklenebilecek bütün yedeklerdir (günlük ve
// yükseltme öncesi), en yenisi başta.
func yedekAdaylari(path string) []string {
	dizin := filepath.Join(filepath.Dir(path), YedekDizini)
	herhangi := func(taban, ad string) bool { return gunlukMu(taban, ad) || surumOncesiMu(taban, ad) }
	var out []string
	for _, y := range yedekleriListele(dizin, tabanAd(path), herhangi) {
		out = append(out, y.yol)
	}
	return out
}

// kopyala, kaynak'ı hedef'e "ya tamamı ya hiç" kopyalar.
func kopyala(kaynak, hedef string) error {
	k, err := os.Open(kaynak)
	if err != nil {
		return err
	}
	defer k.Close()
	f, err := os.CreateTemp(filepath.Dir(hedef), filepath.Base(hedef)+".*"+dosya.GeciciEk)
	if err != nil {
		return err
	}
	gecici := f.Name()
	_, err = io.Copy(f, k)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(gecici, hedef)
	}
	if err != nil {
		_ = os.Remove(gecici)
	}
	return err
}

func diskeIsle(yol string) error {
	f, err := os.OpenFile(yol, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
