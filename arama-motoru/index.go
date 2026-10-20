package main

// index.go — yerel depolama: hem "hangi post/yanıt/mesaj ID'lerini zaten
// biliyorum" defterini (senkronizasyon isteklerinde ve tombstone/silme
// işlenirken kullanılır) hem de aranabilir içeriğin SQLite FTS5 tam metin
// indeksini tutar. Bu programın kendi ayrı veritabanı dosyasıdır — AND'ın
// forum.db'sine hiç dokunmaz.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

const veriDosyaAdi = "arama_motoru.db"

type SonucTip string

const (
	TipKonu   SonucTip = "konu"
	TipYanit  SonucTip = "yanit"
	TipSohbet SonucTip = "sohbet"
	// TipSite, AND'ın kişisel site sayfalarını (bkz. site.go, AND'ın
	// internal/site paketinin bağımsız yeniden uygulaması) temsil eder.
	TipSite SonucTip = "site"
)

type AramaSonucu struct {
	Tip         SonucTip
	KaynakID    string
	UstID       string
	Kategori    string
	Baslik      string
	Govde       string
	YazarAd     string
	YazarKey    string
	OlusturmaTS time.Time
	KaynakPeer  string // "" = yerel; else = ağdaki hangi eşten geldi

	// Sig, bu kaydın ORİJİNAL ed25519 imzasıdır (ingest anında verifyPost/
	// verifyReply/verifyChat/verifySitePage'i BAŞARIYLA geçen post/yanıt/
	// mesaj/sayfanın kendi Sig alanı). Bu kaydın "ağda ara" ile başka bir
	// eşe (bkz. search.go'daki agSonucu ve dogrulaAgSonucu) YANIT olarak
	// gönderilmesini ve alıcının kendi tarafında, kaydın diğer alanlarından
	// (KaynakID/UstID/Kategori/Baslik/Govde/YazarKey/OlusturmaTS) yeniden
	// kurduğu AYNI payload'a karşı AYNI ed25519 imzasını YENİDEN
	// doğrulayabilmesini sağlar — bkz. dogrulaAgSonucu'nun doc yorumu
	// (zaman damgası hassasiyeti ve highlight() işaretleyicileri bu yeniden
	// kurmanın orijinal imzalı baytlarla BİREBİR eşleşmesi için özenle
	// ele alınır).
	Sig string

	// Snippet, eşleşen sorgu terimini bağlamıyla (govde içinde nerede
	// geçtiğini) gösteren kısa bir alıntıdır — bkz. Ara'nın doc yorumu.
	// Boşsa (ör. eşleşme yalnızca başlıkta olduğunda) görüntüleme
	// tarafı Govde'nin kendisine düşer.
	Snippet string
}

type Depo struct {
	db *sql.DB
	// sayac, arama_fts'teki GÜNCEL satır sayısının bellek-içi bir
	// eşleniğidir (bkz. SayimAl'ın doc yorumu) — depoAc'ta bir kez
	// tohumlanır, Ekle/EkleYaGuncelle/Sil'in her BAŞARILI commit'inden
	// sonra güncellenir. atomic.Int64 kullanılır çünkü Depo, forum/chat/
	// site gossip işleyicileri, senkronizasyon goroutine'leri ve arama
	// sunucusu tarafından AYNI ANDA (kilit olmadan) okunup yazılır.
	sayac atomic.Int64
}

func depoAc(dataDir string) (*Depo, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("veri dizini boş olamaz")
	}
	yol := filepath.Join(dataDir, veriDosyaAdi)
	db, err := sql.Open("sqlite", "file:"+yol+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}
	// AND'ın kendi internal/storage.Open'ıyla AYNI iki ayar, aynı gerekçeyle:
	// WAL modu, sürekli gelen gossip yazımlarıyla (forum/chat/site) eşzamanlı
	// okuma (arama sorguları) arasında varsayılan rollback-journal modundan
	// çok daha az kilit çekişmesi sağlar; SetMaxOpenConns(1), aynı SQLite
	// dosyasına Go'nun bağlantı havuzundan BİRDEN FAZLA eşzamanlı bağlantı
	// açılmasını (ki bu yalnızca gereksiz kilit/yeniden-deneme maliyeti
	// getirir, SQLite zaten yazma erişimini tek seferde bir bağlantıya
	// serileştirir) daha en baştan engeller.
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("veritabanı ayarları uygulanamadı: %w", err)
	}
	db.SetMaxOpenConns(1)

	if err := migrateAddSigColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("şema göçü başarısız: %w", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS bilinen (
			id TEXT PRIMARY KEY,
			tip TEXT NOT NULL,
			silindi INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bilinen_tip ON bilinen(tip)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS arama_fts USING fts5(
			tip UNINDEXED,
			kaynak_id UNINDEXED,
			ust_id UNINDEXED,
			kategori,
			baslik,
			govde,
			yazar_ad,
			yazar_key UNINDEXED,
			olusturma_ts UNINDEXED,
			sig UNINDEXED,
			tokenize = 'unicode61 remove_diacritics 2'
		)`,
		// terim_sozlugu, arama_fts'in FTS5 "vocab" uzantısıyla sağladığı,
		// kendi verisini AYRICA saklamayan salt-okunur bir "sözlük"
		// görünümüdür: her indekslenmiş AYRIK terim için (term, doc, cnt) —
		// doc: terimin kaç FARKLI kayıtta geçtiği, cnt: toplam geçiş sayısı.
		// 'row' tipi seçildi (üç seçenek: row/col/instance) çünkü yazım
		// önerisi (bkz. yazimonerisi.go) için gereken tam olarak budur: terim
		// başına TEK satır. UNINDEXED sütunlar (tip, kaynak_id, ust_id,
		// yazar_key, olusturma_ts) buraya hiç girmez — öneri havuzu
		// UUID/zaman damgası gürültüsüyle kirlenmez.
		`CREATE VIRTUAL TABLE IF NOT EXISTS terim_sozlugu USING fts5vocab('arama_fts', 'row')`,
		// kayitli_arama, "takip et" özelliğinin (bkz. dosyanın sonundaki
		// "Kayıtlı arama takibi" bölümü) kalıcı depusudur — normal bir
		// tablodur (arama_fts'in aksine sanal DEĞİL), bu yüzden WHERE
		// eşitlik filtreleri ve UPDATE'ler ucuzdur. UNIQUE kısıtı, AYNI
		// (sorgu, tip, yazar_key) üçlüsünün iki kez kaydedilmesini
		// (kullanıcının "takip et"e yanlışlıkla iki kez basması gibi)
		// INSERT OR IGNORE ile sessizce engeller.
		`CREATE TABLE IF NOT EXISTS kayitli_arama (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sorgu TEXT NOT NULL,
			tip TEXT NOT NULL DEFAULT '',
			yazar_key TEXT NOT NULL DEFAULT '',
			olusturma_ts TEXT NOT NULL,
			yeni_eslesme INTEGER NOT NULL DEFAULT 0,
			son_eslesme_ts TEXT NOT NULL DEFAULT '',
			UNIQUE(sorgu, tip, yazar_key)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, fmt.Errorf("şema oluşturulamadı: %w", err)
		}
	}

	d := &Depo{db: db}
	// sayac, burada BİR KEZ (program başına, açılışta) gerçek bir COUNT(*)
	// ile tohumlanır — arama_fts sanal bir FTS5 tablosu olduğundan (bkz.
	// SayimAl'ın doc yorumu) bu sorgu bir tam tablo taramasıdır, ama TEK
	// seferlik açılış maliyeti kabul edilebilir; asıl kazanç bunu bir daha
	// HİÇ (ne periyodik durum tazelemesinde ne başka bir yerde) tekrar
	// çalıştırmamaktan gelir.
	var mevcutSayi int64
	if err := db.QueryRow(`SELECT count(*) FROM arama_fts`).Scan(&mevcutSayi); err != nil {
		db.Close()
		return nil, fmt.Errorf("mevcut kayıt sayısı okunamadı: %w", err)
	}
	d.sayac.Store(mevcutSayi)
	return d, nil
}

// migrateAddSigColumns, sig/imza_govdesi sütunları eklenmeden ÖNCE
// oluşturulmuş eski bir arama_fts tablosu varsa onu (ve ona bağlı
// terim_sozlugu görünümünü) DÜŞÜRÜR ve bilinen tablosunu temizler.
//
// Bu programın yerel indeksi tamamen YENİDEN OLUŞTURULABİLİR bir P2P ağ
// önbelleğidir (bkz. dosyanın en üstündeki doc yorumu — "geriye dönük
// senkronize edilen") — bu yüzden bir kerelik bu maliyeti göze almak (bir
// sonraki bağlantıda ağdan yeniden senkronize olmak), FTS5'in sınırlı/
// sürüme bağlı ALTER TABLE ADD COLUMN desteğine güvenmeden yeni sütunları
// güvenle eklemenin en basit yoludur. sig, arama sonuçlarının ağ üzerinden
// GÜVENLİ bir şekilde paylaşılabilmesi (alıcı tarafın ed25519 imzasını
// yeniden doğrulayabilmesi) için gereklidir — bkz. search.go'daki
// dogrulaAgSonucu'nun doc yorumu.
func migrateAddSigColumns(db *sql.DB) error {
	var tabloAdi string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'arama_fts'`).Scan(&tabloAdi)
	if err == sql.ErrNoRows {
		return nil // taze kurulum — göçe gerek yok
	}
	if err != nil {
		return err
	}

	var deneme string
	err = db.QueryRow(`SELECT sig FROM arama_fts LIMIT 1`).Scan(&deneme)
	if err == nil || err == sql.ErrNoRows {
		return nil // sütun zaten var (satır olsun olmasın hata vermedi)
	}
	if !strings.Contains(err.Error(), "no such column") {
		return err // beklenmeyen bir hata — göçe kalkışmadan yüzeye çıkar
	}

	for _, s := range []string{
		`DROP TABLE IF EXISTS terim_sozlugu`,
		`DROP TABLE IF EXISTS arama_fts`,
		`DELETE FROM bilinen`,
	} {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (d *Depo) Kapat() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

// Biliniyor, id'nin daha önce görülüp görülmediğini (ve silinip
// silinmediğini) bildirir.
func (d *Depo) Biliniyor(id string) (var_ bool, silindi bool) {
	var s int
	err := d.db.QueryRow(`SELECT silindi FROM bilinen WHERE id = ?`, id).Scan(&s)
	if err != nil {
		return false, false
	}
	return true, s != 0
}

// BilinenIDler, senkronizasyon isteklerinde "bunlara zaten sahibim"
// listesi olarak gönderilmek üzere tip'e göre tüm ID'leri döner (silinmiş
// olanlar dahil — bir tombstone'u zaten bildiğimizi de karşı tarafa
// söylemek, onu bize tekrar tekrar göndermesini önler).
func (d *Depo) BilinenIDler(tip SonucTip) ([]string, error) {
	rows, err := d.db.Query(`SELECT id FROM bilinen WHERE tip = ?`, tip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// BilinenAktifVeSilinenIDler, BilinenIDler'in tersine, tip'e göre bilinen
// ID'leri aktif (silindi=0) ve tombstone'lanmış (silindi=1) olarak İKİYE
// AYIRIP döner — site.go'nun sync isteğinin Known (yalnızca hâlâ elimizde
// olan aktif sayfalar) ve KnownTombstones (yalnızca tombstone'lar)
// alanlarını AND'ın kendi site/sync.go'sundaki AYNI ayrımla
// doldurabilmesi için gerekli; forum/chat'in kullandığı BilinenIDler
// (ikisini karışık döner) o protokoller için yeterli ama site için değil.
func (d *Depo) BilinenAktifVeSilinenIDler(tip SonucTip) (aktif []string, silinen []string, err error) {
	rows, err := d.db.Query(`SELECT id, silindi FROM bilinen WHERE tip = ?`, tip)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s int
		if err := rows.Scan(&id, &s); err != nil {
			return nil, nil, err
		}
		if s != 0 {
			silinen = append(silinen, id)
		} else {
			aktif = append(aktif, id)
		}
	}
	return aktif, silinen, rows.Err()
}

// SiteSurumleriAl, indeksteki her AKTİF site sayfasının elimizdeki
// sürümünü kaynak_id -> UnixNano olarak döner — site.go'nun sync isteğinin
// KnownVersions alanı için. Bir site sayfasında olusturma_ts, o an
// indeksteki sürümün UpdatedAt'idir (EkleYaGuncelle her daha yeni sürümde
// onu değiştirir), RFC3339Nano ile TAM hassasiyette saklanır — yanıtlayan
// AND düğümünün UnixNano'suyla birebir karşılaştırılabilir. Ayrıştırılamayan
// ya da UnixNano aralığı dışındaki bir zaman damgası haritaya girmez (o
// sayfa için eski "bilinen slug = güncel" kuralı geçerli olur).
func (d *Depo) SiteSurumleriAl() (map[string]int64, error) {
	rows, err := d.db.Query(`SELECT kaynak_id, olusturma_ts FROM arama_fts WHERE tip = ?`, TipSite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var id, ts string
		if err := rows.Scan(&id, &ts); err != nil {
			return nil, err
		}
		t, perr := time.Parse(time.RFC3339Nano, ts)
		if perr != nil {
			continue
		}
		if v, ok := unixNanoGuvenli(t); ok {
			out[id] = v
		}
	}
	return out, rows.Err()
}

// Ekle, yeni bir aranabilir kayıt ekler (zaten biliniyorsa no-op — sessizce
// atlar, sync/gossip'ten gelen tekrarlar zaten normal). Forum post/yanıt
// ve sohbet mesajları için doğru davranış budur — bunlar AND'da bir kez
// oluşturulup bir daha ASLA değişmez, aynı ID ikinci kez asla farklı bir
// içerikle gelmez. Değişebilen içerik (kişisel site sayfaları) için
// bunun yerine EkleYaGuncelle kullanılmalı.
func (d *Depo) Ekle(s AramaSonucu) error {
	if biliniyor, silindi := d.Biliniyor(s.KaynakID); biliniyor && !silindi {
		return nil
	} else if biliniyor && silindi {
		return nil // tombstone'lanmış bir ID'yi yeniden canlandırma
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`INSERT OR IGNORE INTO bilinen (id, tip, silindi) VALUES (?, ?, 0)`, s.KaynakID, s.Tip); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO arama_fts (tip, kaynak_id, ust_id, kategori, baslik, govde, yazar_ad, yazar_key, olusturma_ts, sig)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		s.Tip, s.KaynakID, s.UstID, s.Kategori, s.Baslik, s.Govde, s.YazarAd, s.YazarKey, s.OlusturmaTS.UTC().Format(time.RFC3339Nano), s.Sig); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.sayac.Add(1) // bkz. Depo.sayac'ın doc yorumu — yalnızca GERÇEK bir yeni satır commit edildiyse artır
	d.takipEslestir(s)
	return nil
}

// EkleYaGuncelle, DEĞİŞEBİLEN içerik (kişisel site sayfaları — aynı
// yazar/slug ikilisi yeniden yayınlanınca TAMAMEN değişir, bkz.
// site.go'nun doc yorumu) için "en yeni kazanır" upsert'idir. Ekle'nin
// "ilk yazan kazanır, bir KaynakID bir daha asla değişmez" sözleşmesinden
// KASITLI olarak farklıdır — forum/chat'e ASLA kullanılmamalı.
func (d *Depo) EkleYaGuncelle(s AramaSonucu) error {
	if _, silindi := d.Biliniyor(s.KaynakID); silindi {
		return nil // tombstone'lanmış bir slug'ı yeniden canlandırma
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	yeniKayit := false
	var eskiTS string
	err = tx.QueryRow(`SELECT olusturma_ts FROM arama_fts WHERE kaynak_id = ?`, s.KaynakID).Scan(&eskiTS)
	switch {
	case err == nil:
		if eski, perr := time.Parse(time.RFC3339Nano, eskiTS); perr == nil && !s.OlusturmaTS.After(eski) {
			return nil // bildiğimiz sürüm zaten en az bu kadar yeni — replay
		}
		if _, err := tx.Exec(`DELETE FROM arama_fts WHERE kaynak_id = ?`, s.KaynakID); err != nil {
			return err
		}
		// Bir GÜNCELLEME (delete+insert) arama_fts satır sayısını
		// DEĞİŞTİRMEZ — sayac burada artırılmaz.
	case err == sql.ErrNoRows:
		yeniKayit = true // İlk kez görülüyor — aşağıda normal şekilde eklenecek.
	default:
		return err
	}

	if _, err := tx.Exec(`INSERT OR IGNORE INTO bilinen (id, tip, silindi) VALUES (?, ?, 0)`, s.KaynakID, s.Tip); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO arama_fts (tip, kaynak_id, ust_id, kategori, baslik, govde, yazar_ad, yazar_key, olusturma_ts, sig)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		s.Tip, s.KaynakID, s.UstID, s.Kategori, s.Baslik, s.Govde, s.YazarAd, s.YazarKey, s.OlusturmaTS.UTC().Format(time.RFC3339Nano), s.Sig); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if yeniKayit {
		d.sayac.Add(1)
	}
	d.takipEslestir(s)
	return nil
}

// Sil, kaynakID'yi tombstone'lar (bilinen'de silindi=1 işaretler, FTS'ten
// kaldırır) — bir daha asla yeniden görünmez.
func (d *Depo) Sil(kaynakID string, tip SonucTip) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`INSERT INTO bilinen (id, tip, silindi) VALUES (?, ?, 1)
		ON CONFLICT(id) DO UPDATE SET silindi = 1`, kaynakID, tip); err != nil {
		return err
	}
	sonuc, err := tx.Exec(`DELETE FROM arama_fts WHERE kaynak_id = ?`, kaynakID)
	if err != nil {
		return err
	}
	silinenSatir, err := sonuc.RowsAffected()
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// silinenSatir normalde 0 ya da 1'dir (kaynak_id UNINDEXED bir eşitlik
	// filtresi olsa da mantıksal olarak benzersizdir) — zaten silinmiş/hiç
	// var olmamış bir ID için 0 döner, sayac o durumda DOKUNULMAZ.
	if silinenSatir > 0 {
		d.sayac.Add(-silinenSatir)
	}
	return nil
}

// sorguOgesi, sorguOgeleriniAyikla'nın kullanıcının ham arama metninden
// çıkardığı TEK bir söz dizimi parçasını temsil eder: ya bir "OR"
// operatörüdür (veya=true, diğer alanlar anlamsız) ya da aranacak bir
// terimdir (düz kelime ya da tırnaklı tam ifade, dışlanmış olabilir).
type sorguOgesi struct {
	veya     bool   // true ise bu, iki terimi birbirine bağlayan OR operatörüdür
	metin    string // ham (henüz FTS5 için kaçırılmamış) terim metni
	tamIfade bool   // tırnak içinde mi yazıldı (prefix eklenmeyen tam ifade/phrase eşleşmesi)
	haric    bool   // "-" ile mi başladı (sonuçlardan dışlanacak, FTS5'in NOT'una çevrilecek)
}

// sorguOgeleriniAyikla, ham sorgu metnini soldan sağa TEK GEÇİŞTE tarayarak
// sorguOgesi dizisine ayırır. Kurallar (Google/Yandex'in arama kutusuyla
// aynı, kullanıcıya FTS5'in KENDİ söz dizimini hiç öğretmeden):
//
//   - Boşluk dışındaki her şey bir terime aittir; ardışık boşluk/sekme
//     tek ayraç gibi davranır.
//   - Bir terimin BAŞINDA (boşluktan hemen sonra) tek bir " karakteri,
//     bir sonraki " ile (varsa) kapanan bir TAM İFADE başlatır — içindeki
//     boşluklar terim sınırı SAYILMAZ. Kapanış tırnağı yoksa (kullanıcı
//     unuttu), metnin SONUNA kadar her şey ifadeye dahil edilir; tarayıcı
//     arama kutularının/Google'ın da yaptığı gibi hata vermeden tolere
//     edilir.
//   - Bir terimin BAŞINDA tek bir "-" karakteri (hemen ardından boşluk
//     GELMİYORSA) o terimi DIŞLAMA (haric=true) olarak işaretler; "-"
//     kelime ya da tırnaklı ifadenin önüne gelebilir. Tek başına "-"
//     (ardından boşluk/hiçbir şey yoksa) ya da kelime İÇİNDE geçen "-"
//     (ör. "iş-yeri") düz metin sayılır, dışlama tetiklemez.
//   - Tam olarak "OR" (BÜYÜK harf, tek başına bir terim, tırnaksız) bir
//     VEYA operatörüdür. Küçük/karışık harfle yazılan "or" ya da tırnak
//     içindeki "OR" düz kelime olarak aranır.
func sorguOgeleriniAyikla(sorgu string) []sorguOgesi {
	runes := []rune(sorgu)
	i, n := 0, len(runes)
	var ogeler []sorguOgesi

	for i < n {
		for i < n && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= n {
			break
		}

		haric := false
		if runes[i] == '-' && i+1 < n && !unicode.IsSpace(runes[i+1]) {
			haric = true
			i++
		}

		if i < n && runes[i] == '"' {
			i++ // açılış tırnağını atla
			start := i
			for i < n && runes[i] != '"' {
				i++
			}
			metin := string(runes[start:i])
			if i < n {
				i++ // kapanış tırnağını atla (varsa)
			}
			if metin != "" {
				ogeler = append(ogeler, sorguOgesi{metin: metin, tamIfade: true, haric: haric})
			}
			continue
		}

		start := i
		for i < n && !unicode.IsSpace(runes[i]) {
			i++
		}
		kelime := string(runes[start:i])
		if !haric && kelime == "OR" {
			ogeler = append(ogeler, sorguOgesi{veya: true})
			continue
		}
		ogeler = append(ogeler, sorguOgesi{metin: kelime, haric: haric})
	}
	return ogeler
}

// ftsTerimiKacirVeSar, TEK bir terimi (kelime ya da tam ifade) FTS5'in
// kendi çift-tırnaklı dize söz dizimiyle sarar: içindeki her " → ""
// olarak KAÇIRILIR. Böylece terimin içinde ne olursa olsun (parantez,
// "AND"/"OR"/"NOT" kelimeleri, sütun-filtresi gibi görünen bir dize)
// FTS5 bunu SAF METİN olarak okur, asla operatör/sütun-filtresi olarak
// yorumlamaz — GÜVENLİK: FTS5'in MATCH argümanının kendi mini sorgu
// diline enjeksiyonu bu şekilde engellenir. tamIfade=false ise (düz
// kelime) sonuna prefix-eşleşme yıldızı (*) eklenir; tamIfade=true ise
// (tırnaklı ifade) eklenmez — tam ifade araması TAM eşleşmedir, prefix
// değil.
func ftsTerimiKacirVeSar(metin string, tamIfade bool) string {
	kacirilmis := strings.ReplaceAll(metin, `"`, `""`)
	if tamIfade {
		return `"` + kacirilmis + `"`
	}
	return `"` + kacirilmis + `"*`
}

// ftsSorguOlustur, kullanıcının yazdığı ham arama metnini FTS5'in MATCH
// söz dizimine çevirir. Google/Yandex'e benzer şekilde üç söz dizimi
// destekler:
//
//   - "tam ifade"         → tırnak içindeki kelimeler AYRI AYRI prefix
//     terimler DEĞİL, TEK bir bitişik ifade (phrase)
//     olarak aranır (prefix eklenmez, tam eşleşme).
//   - -kelime / -"ifade"   → terim SONUÇLARDAN HARİÇ TUTULUR (FTS5'in NOT
//     operatörüne çevrilir).
//   - kelime1 OR kelime2   → iki terimden EN AZ BİRİNİN geçtiği sonuçlar
//     (FTS5'in OR operatörüne çevrilir). "OR" sadece
//     TÜMÜ BÜYÜK HARFLE, tek başına yazıldığında
//     operatör sayılır; literal "OR" aramak için
//     kullanıcı onu tırnaklamalı: "OR".
//
// Bu üç söz dizimi de kullanılmazsa (düz "kelime1 kelime2") çıktı ESKİ
// koddakiyle BAYT BAYT AYNIDIR (her kelime "kelime"* prefix-eşleşmesiyle
// AND'lanır) — geriye dönük uyumluluk KORUNUR.
//
// FTS5 ÖNCELİK KURALI (SQLite belgeleri): NOT > AND > OR — yani örneğin
// "a b OR c" implicit AND'in açık OR'dan daha sıkı bağlanmasıyla (a AND b)
// OR c olarak yorumlanır. Dışlama (-terim) terimleri bu yüzden HER ZAMAN
// pozitif/OR ifadesinin SONUNA, parantez içine alınmış pozitif ifadeye
// ardışık NOT'larla eklenir — ör. "a OR b -c" → `("a"* OR "b"*) NOT "c"*`
// — parantez, FTS5'in NOT'un OR'dan daha sıkı bağlandığı öncelik kuralı
// yüzünden yanlış gruplanmayı (a OR (b NOT c)) önlemek için ZORUNLUDUR.
//
// KENAR DURUM — SALT DIŞLAMA: Sorgu YALNIZCA dışlama terim(ler)inden
// oluşuyorsa (hiç pozitif/OR terimi yoksa, ör. tek başına "-hariç"),
// sonuç BOŞ DÖNER (çağıran taraf sıfır sonuç görür, hata YOK) — FTS5'te
// Google'ın aksine "TÜM dokümanları eşleştir" anlamına gelen bir taban
// ifade yoktur; sol operandı olmayan bir NOT sözdizimi hatası olurdu.
func ftsSorguOlustur(sorgu string) string {
	ogeler := sorguOgeleriniAyikla(sorgu)
	if len(ogeler) == 0 {
		return ""
	}

	var pozitif strings.Builder // düz AND'lanan + açık OR'lanan terimler
	var haricler []string       // sona " NOT ..." olarak eklenecek terimler
	sonOgeVeya := false         // pozitif'e en son eklenen şey " OR " mıydı

	for _, og := range ogeler {
		if og.veya {
			// Baştaki ya da art arda gelen OR'lar (pozitif boşken ya da bir
			// önceki öge de OR'ken) sessizce yok sayılır — sarkan/anlamsız
			// bir " OR " asla üretilmez.
			if pozitif.Len() > 0 && !sonOgeVeya {
				pozitif.WriteString(" OR ")
				sonOgeVeya = true
			}
			continue
		}

		terim := ftsTerimiKacirVeSar(og.metin, og.tamIfade)
		if og.haric {
			haricler = append(haricler, terim)
			continue
		}
		if pozitif.Len() > 0 && !sonOgeVeya {
			pozitif.WriteByte(' ') // implicit AND — FTS5'te yan yana yazmak yeterli
		}
		pozitif.WriteString(terim)
		sonOgeVeya = false
	}

	// Sondaki sarkan " OR "yı temizle (ör. "elma OR" girdisinde).
	poz := strings.TrimSuffix(pozitif.String(), " OR ")
	if poz == "" {
		// Salt dışlama (ya da hiç terim yok) — bkz. yukarıdaki doc yorumu.
		return ""
	}
	if len(haricler) == 0 {
		return poz
	}
	var b strings.Builder
	b.WriteByte('(')
	b.WriteString(poz)
	b.WriteByte(')')
	for _, h := range haricler {
		b.WriteString(" NOT ")
		b.WriteString(h)
	}
	return b.String()
}

// SnippetOncesi/SnippetSonrasi, eşleşen terimi Snippet içinde işaretleyen
// sentinel karakterlerdir — normal metinde pratikte hiç geçmeyen kontrol
// karakterleri seçildi ki görüntüleme tarafı bunları güvenle ayırıp
// vurgulu (bold) render edebilsin, bkz. view.go.
const (
	SnippetOncesi  = "\x01"
	SnippetSonrasi = "\x02"
)

// bm25SutunAgirliklari, arama_fts'in CREATE VIRTUAL TABLE'daki sütun
// sırasıyla (tip=0, kaynak_id=1, ust_id=2, kategori=3, baslik=4, govde=5,
// yazar_ad=6, yazar_key=7, olusturma_ts=8) BİREBİR eşleşen, virgülle
// ayrılmış bm25() ağırlık listesidir. UNINDEXED sütunlar (tip, kaynak_id,
// ust_id, yazar_key, olusturma_ts) FTS5'te asla eşleşme ÜRETMEDİĞİNDEN bu
// pozisyonlara verilen ağırlık SQLite tarafından zaten yok sayılır — yine
// de pozisyonel sırayı korumak için 0.0 yazılıyor.
//
// baslik (8.0) en yüksek ağırlığı alır: gerçek arama motorlarında başlık
// eşleşmesi gövde eşleşmesinden kat kat daha değerli bir alaka sinyalidir
// (bu projede ÖLÇÜLDÜ: eski "ORDER BY rank" — eşit ağırlıklı bm25'in bir
// takma adı — baslik'ta VE govde'de geçen iki dokümana BİREBİR AYNI skoru
// veriyordu). kategori (2.0) orta düzey. govde (1.0) TABAN ağırlıktır,
// diğerleri buna görelidir. yazar_ad (0.5) govde'nin bile altındadır:
// sorgu teriminin salt yazar adıyla eşleşmesi zayıf bir alaka sinyalidir.
const bm25SutunAgirliklari = "0.0, 0.0, 0.0, 2.0, 8.0, 1.0, 0.5, 0.0, 0.0"

// tazelikAgirligi ve tazelikYariOmurGun, AraGelismis'in ORDER BY
// ifadesindeki tazelik bonusunu ayarlar. tazelikPuani = 1/(1 +
// yaşGün/tazelikYariOmurGun) formülüyle (0,1] aralığına sıkıştırılır:
// yepyeni içerik ≈1, tazelikYariOmurGun kadar eski içerik ≈0.5, çok eski
// içerik →0 — yani eski içerik CEZALANDIRILMAZ, yalnızca eşit derecede
// alakalı yeni içerik hafifçe ÖNE alınır. bm25() KÜÇÜK (daha negatif) =
// DAHA İYİ eşleşme döndürdüğünden, tazelik terimi bm25'ten TOPLANMAZ,
// ÇIKARILIR — pozitif bir tazelik puanını çıkarmak skoru daha negatif
// yapıp yeni içeriği yukarı taşır.
const (
	tazelikAgirligi    = 2.0
	tazelikYariOmurGun = 30.0 // gün
)

// aramaSayfaBoyu, sayfalama ekranında (bkz. model.go'nun m.sayfaNo'su)
// bir "sayfa"da gösterilecek en fazla sonuç sayısıdır. Google/Yandex'in
// sayfa başına ~10-20 sonuç gösterme geleneğine paralel.
const aramaSayfaBoyu = 20

// AramaSayfasi, AraGelismis/yazarinTumIcerigi'nin sayfalanmış dönüş
// değeridir.
type AramaSayfasi struct {
	Sonuclar []AramaSonucu
	// SonrakiVarMi, limit'ten BİR FAZLA satır çekilip son satırın
	// atılmasıyla belirlenir — ayrı bir COUNT(*) ... MATCH sorgusuna (FTS5
	// eşleşmesini iki kez hesaplamaya) gerek kalmadan "sonraki sayfa var
	// mı" bilgisini verir.
	SonrakiVarMi bool
}

// Ara, tüm içerik türlerinde ve tüm yazarlarda, ilk sayfada arar —
// AraGelismis(sorgu, "", "", limit, 0).Sonuclar ile aynıdır. Bu ve
// AraFiltreli'nin dış imzaları KORUNUR (offset=0'a sabitlenmiş
// sarmalayıcılar) — sayfalamayla ilgilenmeyen çağıranlar etkilenmez.
func (d *Depo) Ara(sorgu string, limit int) ([]AramaSonucu, error) {
	sayfa, err := d.AraGelismis(sorgu, "", "", limit, 0)
	return sayfa.Sonuclar, err
}

// AraFiltreli, tip boş değilse yalnızca o türde (TipKonu/TipYanit/
// TipSohbet/TipSite) arar, ilk sayfayı döner — sonuç listesini tek bir
// içerik türüyle sınırlamak isteyen "gelişmiş arama" filtresi (bkz.
// model.go'nun m.filtre'si). AraGelismis(sorgu, tip, "", limit, 0).Sonuclar
// ile aynıdır.
func (d *Depo) AraFiltreli(sorgu string, tip SonucTip, limit int) ([]AramaSonucu, error) {
	sayfa, err := d.AraGelismis(sorgu, tip, "", limit, 0)
	return sayfa.Sonuclar, err
}

// AraGelismis, AraFiltreli'nin üzerine yazarKey filtresi VE sayfalama
// (limit/offset) ekler: yazarKey boş değilse yalnızca o yazar anahtarına
// (tam eşleşme — YazarAd'ın aksine sahtesi mümkün olmayan gerçek kimlik)
// ait sonuçlar döner. "Bu yazarın diğer içerikleri" (bkz. model.go'nun
// keyDetay'ındaki 'a' tuşu) burada bu filtreyle uygulanır.
//
// Sıralama: bm25SutunAgirliklari ile sütun-ağırlıklı bm25() + yukarıdaki
// tazelik bonusu TEK bir ifadede birleştirilir — hem "başlık eşleşmesi
// gövdeden değerlidir" hem "eşit alakada yeni içerik hafifçe öne çıkar"
// ilkeleri aynı anda uygulanır. Bu, AraGelismisSirali(..., SiralamaAlaka,
// ...) ile AYNIDIR — dış imzası (sıralama modu parametresi OLMADAN)
// KORUNUR, sıralamayla ilgilenmeyen çağıranlar (CLI, mevcut testler)
// etkilenmez.
func (d *Depo) AraGelismis(sorgu string, tip SonucTip, yazarKey string, limit, offset int) (AramaSayfasi, error) {
	return d.AraGelismisSirali(sorgu, tip, yazarKey, SiralamaAlaka, limit, offset)
}

// SiralamaModu, AraGelismisSirali'nin sonuçları nasıl sıralayacağını
// belirler — gerçek arama motorlarının "Alaka/Tarih" sıralama seçeneğine
// karşılık gelir (bkz. model.go'nun ctrl+r'si).
type SiralamaModu int

const (
	// SiralamaAlaka, AraGelismis'in ESKİDEN BERİ tek davranışıyla BİREBİR
	// AYNIDIR: sütun ağırlıklı bm25() + tazelik bonusu.
	SiralamaAlaka SiralamaModu = iota
	// SiralamaYeni, alaka puanını YOK SAYAR — yazarinTumIcerigi'nin
	// anahtar-kelimesiz dalıyla AYNI ilkeyle (tek sinyal tazelik) yalnızca
	// olusturma_ts'e göre en yeniden eskiye sıralar; MATCH filtresi (ve
	// dolayısıyla hangi kayıtların sonuç kümesinde olduğu) DEĞİŞMEZ, yalnızca
	// bu kümenin GÖSTERİM sırası değişir.
	SiralamaYeni
)

// AraGelismisSirali, AraGelismis'in üzerine bir sıralama modu ekler —
// bkz. SiralamaModu'nun doc yorumu.
func (d *Depo) AraGelismisSirali(sorgu string, tip SonucTip, yazarKey string, siralama SiralamaModu, limit, offset int) (AramaSayfasi, error) {
	sorgu = strings.TrimSpace(sorgu)
	if sorgu == "" {
		// Anahtar kelimesiz ama yazarKey doluysa: gerçek arama motorlarının
		// "yazar sayfası" davranışı — bir terim gerektirmeden o yazarın TÜM
		// içeriğini en yeniden eskiye listeler (bkz. model.go'nun 'a' tuşu).
		// siralama burada KASITLI olarak yok sayılır — yazarinTumIcerigi'nin
		// kendi doc yorumundaki gerekçeyle (bm25 sinyali yok, tek sıralama
		// zaten tazelik) SiralamaYeni ile aynı sonucu üretir.
		if yazarKey != "" {
			return d.yazarinTumIcerigi(tip, yazarKey, limit, offset)
		}
		return AramaSayfasi{}, nil
	}
	ftsSorgu := ftsSorguOlustur(sorgu)
	if ftsSorgu == "" {
		return AramaSayfasi{}, nil
	}
	// govde sütun indeksi 5'tir (tip=0, kaynak_id=1, ust_id=2, kategori=3,
	// baslik=4, govde=5) — CREATE VIRTUAL TABLE'daki sütun sırasıyla eşleşmeli.
	// highlight() TÜM govde'yi (kırpmadan) işaretli döner — detay ekranında
	// kullanılır; snippet() ise kısa, bağlamlı bir alıntı döner (sonuç
	// listesinde kullanılır). İkisi de aynı sentinel işaretleyicileri
	// kullanır (bkz. view.go'daki vurgula).
	filtre := ""
	var filtreArgs []any
	if tip != "" {
		filtre += ` AND tip = ?`
		filtreArgs = append(filtreArgs, string(tip))
	}
	if yazarKey != "" {
		filtre += ` AND yazar_key = ?`
		filtreArgs = append(filtreArgs, yazarKey)
	}
	// ORDER BY'daki ikincil "kaynak_id" anahtarı, eşit skorlu satırlarda
	// belirsiz/kararsız sıralamayı (dolayısıyla sayfa sınırında kayıt
	// kayması/tekrarı riskini) önler — SQLite eşit ORDER BY değerlerinde
	// satır sırasını GARANTİ ETMEZ.
	var sirala string
	var siralaArgs []any
	switch siralama {
	case SiralamaYeni:
		sirala = `olusturma_ts DESC, kaynak_id`
	default: // SiralamaAlaka
		sirala = `(bm25(arama_fts, ` + bm25SutunAgirliklari + `))
			- ? * (1.0 / (1.0 + (julianday('now') - julianday(olusturma_ts)) / ?)), kaynak_id`
		siralaArgs = []any{tazelikAgirligi, tazelikYariOmurGun}
	}
	// İki aşama: iç sorgu yalnızca sayfadaki satırların rowid'lerini seçer
	// (sıralama için yalnızca bm25 hesaplanır); highlight() ve snippet()
	// yalnızca bu satırlar için üretilir. Önceden ORDER BY ... LIMIT'ten önce
	// eşleşen HER satır için tam gövde vurgulanıyordu: 10.000 kayıtta ~1.000
	// eşleşen bir sorgu 38 ms sürüyordu. Dış sorgu aynı ifadeyle yeniden
	// sıralar (aynı MATCH, aynı bm25 değerleri). +1: "sonraki sayfa var mı"
	// testi için bir fazlası çekilir (bkz. AramaSayfasi.SonrakiVarMi).
	sorguSQL := `SELECT tip, kaynak_id, ust_id, kategori, baslik,
		highlight(arama_fts, 5, ?, ?), yazar_ad, yazar_key, olusturma_ts, sig,
		snippet(arama_fts, 5, ?, ?, '…', 24)
		FROM arama_fts WHERE arama_fts MATCH ? AND rowid IN (
			SELECT rowid FROM arama_fts WHERE arama_fts MATCH ?` + filtre + `
			ORDER BY ` + sirala + ` LIMIT ? OFFSET ?)
		ORDER BY ` + sirala
	args := []any{SnippetOncesi, SnippetSonrasi, SnippetOncesi, SnippetSonrasi, ftsSorgu, ftsSorgu}
	args = append(args, filtreArgs...)
	args = append(args, siralaArgs...)
	args = append(args, limit+1, offset)
	args = append(args, siralaArgs...)

	rows, err := d.db.Query(sorguSQL, args...)
	if err != nil {
		return AramaSayfasi{}, fmt.Errorf("arama başarısız: %w", err)
	}

	var sonuclar []AramaSonucu
	for rows.Next() {
		var s AramaSonucu
		var tsStr string
		if err := rows.Scan(&s.Tip, &s.KaynakID, &s.UstID, &s.Kategori, &s.Baslik, &s.Govde, &s.YazarAd, &s.YazarKey, &tsStr, &s.Sig, &s.Snippet); err != nil {
			rows.Close() //nolint:errcheck
			return AramaSayfasi{}, err
		}
		if ts, perr := time.Parse(time.RFC3339Nano, tsStr); perr == nil {
			s.OlusturmaTS = ts
		}
		sonuclar = append(sonuclar, s)
	}
	tarErr := rows.Err()
	rows.Close() //nolint:errcheck
	if tarErr != nil {
		return AramaSayfasi{}, tarErr
	}

	// Yanıt başlıklarının çözümü (konuBasligiVeKategori) KASITLI olarak
	// ayrı, KAPANMIŞ bir rows'tan SONRA yapılır: depoAc'ın SetMaxOpenConns(1)
	// ile tek bağlantıya sabitlediği havuzda, hâlâ AÇIK bir rows'u
	// tüketirken AYNI *sql.DB üzerinde ikinci bir sorgu açmak, o tek
	// bağlantı serbest kalana kadar sonsuza dek bekleyen bir KİLİTLENMEYE
	// (kendi kendini bekleyen bir döngüye) yol açar — bu gerçekten
	// yaşanmış bir regresyondu, testler onu yakaladı.
	tamamla(d, sonuclar)

	sonrakiVarMi := len(sonuclar) > limit
	if sonrakiVarMi {
		sonuclar = sonuclar[:limit]
	}
	return AramaSayfasi{Sonuclar: sonuclar, SonrakiVarMi: sonrakiVarMi}, nil
}

// tamamla, bir sonuç kümesindeki her yanıtın (Tip==TipYanit) eksik
// Baslik/Kategori'sini üst konudan çözer — bkz. AraGelismis/
// yazarinTumIcerigi'nin bunu neden KENDİ sorgu rows'ları KAPANDIKTAN
// SONRA, ayrı bir geçişte yaptığının doc yorumu.
func tamamla(d *Depo, sonuclar []AramaSonucu) {
	for i := range sonuclar {
		s := &sonuclar[i]
		if s.Tip == TipYanit && s.Baslik == "" && s.UstID != "" {
			if baslik, kategori, bulundu := d.konuBasligiVeKategori(s.UstID); bulundu {
				s.Baslik = baslik
				s.Kategori = kategori
			}
		}
	}
}

// yazarinTumIcerigi, AraGelismis'in anahtar-kelimesiz-ama-yazar-filtreli
// dalıdır: arama_fts MATCH kullanmaz (FTS5, boş bir sorguyla eşleşmez),
// bunun yerine doğrudan yazar_key'e göre filtreleyip olusturma_ts'e göre
// en yeniden eskiye sıralar — snippet/highlight yok (eşleşen bir terim
// olmadığından vurgulanacak bir şey de yok), Govde olduğu gibi döner.
//
// ORDER BY BİLEREK olusturma_ts DESC olarak KALIR — MATCH içermeyen bu
// sorguda bm25()/rank çağırmak, tüm satırlarda sessizce NULL döner (FTS5
// belgelenmiş davranışı); tazelik boostuyla karıştırmak da anlamsızdır,
// çünkü zaten TEK sıralama sinyali tazeliktir.
func (d *Depo) yazarinTumIcerigi(tip SonucTip, yazarKey string, limit, offset int) (AramaSayfasi, error) {
	sorguSQL := `SELECT tip, kaynak_id, ust_id, kategori, baslik, govde, yazar_ad, yazar_key, olusturma_ts, sig
		FROM arama_fts WHERE yazar_key = ?`
	args := []any{yazarKey}
	if tip != "" {
		sorguSQL += ` AND tip = ?`
		args = append(args, string(tip))
	}
	sorguSQL += ` ORDER BY olusturma_ts DESC, kaynak_id LIMIT ? OFFSET ?`
	args = append(args, limit+1, offset)

	rows, err := d.db.Query(sorguSQL, args...)
	if err != nil {
		return AramaSayfasi{}, fmt.Errorf("arama başarısız: %w", err)
	}

	var sonuclar []AramaSonucu
	for rows.Next() {
		var s AramaSonucu
		var tsStr string
		if err := rows.Scan(&s.Tip, &s.KaynakID, &s.UstID, &s.Kategori, &s.Baslik, &s.Govde, &s.YazarAd, &s.YazarKey, &tsStr, &s.Sig); err != nil {
			rows.Close() //nolint:errcheck
			return AramaSayfasi{}, err
		}
		if ts, perr := time.Parse(time.RFC3339Nano, tsStr); perr == nil {
			s.OlusturmaTS = ts
		}
		sonuclar = append(sonuclar, s)
	}
	tarErr := rows.Err()
	rows.Close() //nolint:errcheck
	if tarErr != nil {
		return AramaSayfasi{}, tarErr
	}

	// bkz. AraGelismis'in aynı noktadaki doc yorumu: kapanmış rows'tan
	// SONRA, ayrı bir geçişte — kilitlenmeyi önlemek için.
	tamamla(d, sonuclar)

	sonrakiVarMi := len(sonuclar) > limit
	if sonrakiVarMi {
		sonuclar = sonuclar[:limit]
	}
	return AramaSayfasi{Sonuclar: sonuclar, SonrakiVarMi: sonrakiVarMi}, nil
}

// konuBasligiVeKategori, bir yanıtın ait olduğu konunun başlığını/
// kategorisini yerel indeksten çözer — canlı bir sorgu (ingest sırasında
// denormalize edip saklamak yerine), çünkü bir yanıt gossip'te üst
// konudan ÖNCE gelebilir; canlı çözüm, konu daha sonra senkronize
// olduğunda kendiliğinden düzelir.
//
// Performans notu: bu ve SiteSayfasiBul'un "WHERE kaynak_id = ?" filtresi,
// arama_fts SANAL bir FTS5 tablosu olduğundan (kaynak_id UNINDEXED —
// FTS5 yalnızca MATCH sorgularını hızlandırır, düz eşitlik filtreleri
// için ikincil bir indeks TUTMAZ) teorik olarak bir tam tablo taramasıdır.
// Bunu O(1)'e indirmenin bilinen yolu (arama_fts'in rowid'sini bilinen'in
// rowid'siyle kasıtlı hizalayıp ikinci bir rowid-tabanlı sorguya
// çevirmek) VAR OLAN kullanıcıların veritabanları için riskli bir şema
// göçü gerektirir — bu programın beklenen ölçeğinde (kişisel/topluluk
// P2P forumu, milyonlarca satır DEĞİL) bu taramanın pratik maliyeti
// düşük olduğundan, bu risk KASITLI olarak alınmadı (bkz. SayimAl'ın
// aksine — o hem HER 2 saniyede bir çalıştığından hem de basit bir
// bellek-içi sayaçla TAMAMEN kaçınılabilir olduğundan ayrı ele alındı).
func (d *Depo) konuBasligiVeKategori(postID string) (baslik, kategori string, bulundu bool) {
	err := d.db.QueryRow(`SELECT baslik, kategori FROM arama_fts WHERE kaynak_id = ? AND tip = ? LIMIT 1`,
		postID, TipKonu).Scan(&baslik, &kategori)
	if err != nil {
		return "", "", false
	}
	return baslik, kategori, true
}

// SiteSayfasiBul, bir "paylaşım kodu" ile TAM eşleşen tek bir site
// sayfasını yerel indeksten döner. Kod, site.go'nun siteKaynakID'sinin
// ürettiği "yazarAnahtarı/slug" biçimindeki değerin AYNISIDIR — yeni bir
// kodlama İCAT EDİLMEDİ, TipSite satırlarının zaten kaynak_id'si olarak
// sakladığı değer doğrudan paylaşılabilir bir kimlik olarak kullanılır
// (bkz. view.go'nun viewDetay'ının bunu "Kod" etiketiyle gösterdiği yer,
// model.go'nun "Kodla Git" ekranı, cli.go'nun -site bayrağı).
//
// Bu YEREL bir sorgudur, ağa çıkmaz — Ara/AraFiltreli'nin aynı ilkesi:
// sayfa bu düğüme henüz senkronize olmadıysa (yazarla hiç
// karşılaşılmadıysa) bulunduMu=false döner, hata DEĞİL.
//
// Performans notu: konuBasligiVeKategori'nin doc yorumundaki AYNI "kaynak_id
// eşitlik filtresi arama_fts'te bir tam tarama gerektirir" tartışması
// burası için de geçerlidir — kasıtlı olarak kabul edilmiş bir sınır.
func (d *Depo) SiteSayfasiBul(kod string) (AramaSonucu, bool, error) {
	kod = strings.TrimSpace(kod)
	if kod == "" {
		return AramaSonucu{}, false, nil
	}
	var s AramaSonucu
	var tsStr string
	err := d.db.QueryRow(`SELECT tip, kaynak_id, ust_id, kategori, baslik, govde, yazar_ad, yazar_key, olusturma_ts
		FROM arama_fts WHERE kaynak_id = ? AND tip = ?`, kod, TipSite).
		Scan(&s.Tip, &s.KaynakID, &s.UstID, &s.Kategori, &s.Baslik, &s.Govde, &s.YazarAd, &s.YazarKey, &tsStr)
	if err == sql.ErrNoRows {
		return AramaSonucu{}, false, nil
	}
	if err != nil {
		return AramaSonucu{}, false, fmt.Errorf("site sayfası aranamadı: %w", err)
	}
	if ts, perr := time.Parse(time.RFC3339Nano, tsStr); perr == nil {
		s.OlusturmaTS = ts
	}
	return s, true, nil
}

// SayimAl, indeksteki GÜNCEL kayıt sayısını döner. Bu ARTIK bir SQL
// sorgusu DEĞİLDİR — d.sayac'ın (bkz. Depo.sayac'ın doc yorumu) salt bir
// bellek-içi okumasıdır. Önceden burada her çağrıda `SELECT count(*) FROM
// arama_fts` çalıştırılıyordu; arama_fts sanal bir FTS5 tablosu olduğundan
// (WHERE/MATCH'siz bir count() satır sayısını almak için TÜM tabloyu
// taramak zorundadır — normal bir tabloda olduğu gibi ucuz bir metadata
// okuması DEĞİLDİR) bu, model.go'nun 2 saniyelik durum tazeleme
// tick'inde SÜREKLİ tekrarlanan, indeks büyüdükçe pahalılaşan gereksiz
// bir tam tablo taramasıydı — üstelik depoAc'ın SetMaxOpenConns(1)
// sınırlamasıyla TEK bağlantıyı arama/senkronizasyon yazımlarıyla
// paylaşarak. Artık hiçbir ek maliyeti yok.
func (d *Depo) SayimAl() (int, error) {
	return int(d.sayac.Load()), nil
}

// TipDagilimiAl, Durum ekranındaki "arama konsolu" panelinin ilk
// bileşenidir: indeksteki içeriğin türe (Konu/Yanıt/Sohbet/Site) göre
// dağılımını döner. Yalnızca Durum ekranına girildiğinde çağrılır (mevcut
// 2 saniyelik tick'te DEĞİL) — depoAc'ın SetMaxOpenConns(1) sınırlamasıyla
// gereksiz sorgu yükünden kaçınmak için.
func (d *Depo) TipDagilimiAl() (map[SonucTip]int, error) {
	rows, err := d.db.Query(`SELECT tip, count(*) FROM arama_fts GROUP BY tip`)
	if err != nil {
		return nil, fmt.Errorf("tip dağılımı alınamadı: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	dagilim := make(map[SonucTip]int)
	for rows.Next() {
		var tip string
		var n int
		if err := rows.Scan(&tip, &n); err != nil {
			return nil, err
		}
		dagilim[SonucTip(tip)] = n
	}
	return dagilim, rows.Err()
}

// YazarIstatistigi, EnAktifYazarlarAl'ın bir satırını temsil eder.
type YazarIstatistigi struct {
	YazarKey     string
	YazarAd      string
	IcerikSayisi int
}

// EnAktifYazarlarAl, en çok içerik üreten yazarları (yazar_key'e göre
// gruplanmış — sahtesi mümkün olmayan gerçek kimlik, YazarAd'ın aksine)
// azalan sırada döner. yazar_key boş olan (anonim/kaynağı belirsiz)
// satırlar hariç tutulur — bunlar bir "yazar" temsil etmez.
func (d *Depo) EnAktifYazarlarAl(limit int) ([]YazarIstatistigi, error) {
	rows, err := d.db.Query(`
		SELECT yazar_key, max(yazar_ad), count(*) AS n
		FROM arama_fts
		WHERE yazar_key != ''
		GROUP BY yazar_key
		ORDER BY n DESC, yazar_key
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("en aktif yazarlar alınamadı: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var sonuc []YazarIstatistigi
	for rows.Next() {
		var y YazarIstatistigi
		if err := rows.Scan(&y.YazarKey, &y.YazarAd, &y.IcerikSayisi); err != nil {
			return nil, err
		}
		sonuc = append(sonuc, y)
	}
	return sonuc, rows.Err()
}

// GunlukSayim, GunlukIcerikTrendiAl'ın bir satırını temsil eder — Gun,
// "YYYY-MM-DD" biçiminde (SQLite date() çıktısı).
type GunlukSayim struct {
	Gun  string
	Sayi int
}

// GunlukIcerikTrendiAl, son gun gündeki içerik ÜRETİM tarihine
// (olusturma_ts — içeriğin özgün oluşturulma zamanı) göre günlük dağılımı
// döner. Bu KASITLI olarak "ne zaman indekslendi" değil "ne zaman
// oluşturuldu" sorusuna cevap verir; içeriğin bu düğüme ne zaman ULAŞTIĞINI
// (gerçek indeks büyüme eğrisini) izlemek şema değişikliği (yeni bir
// eklenme_ts sütunu) gerektirir ve kapsam dışı bırakılmıştır.
func (d *Depo) GunlukIcerikTrendiAl(gun int) ([]GunlukSayim, error) {
	rows, err := d.db.Query(`
		SELECT date(olusturma_ts) AS g, count(*) AS n
		FROM arama_fts
		WHERE date(olusturma_ts) >= date('now', ?)
		GROUP BY g
		ORDER BY g`, fmt.Sprintf("-%d days", gun))
	if err != nil {
		return nil, fmt.Errorf("günlük içerik trendi alınamadı: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var sonuc []GunlukSayim
	for rows.Next() {
		var s GunlukSayim
		if err := rows.Scan(&s.Gun, &s.Sayi); err != nil {
			return nil, err
		}
		sonuc = append(sonuc, s)
	}
	return sonuc, rows.Err()
}

// ─── Kayıtlı arama takibi ("takip et") ──────────────────────────────────────
//
// Bir kullanıcı, Ara ekranındaki geçerli sorgu/tür/yazar filtresi
// kombinasyonunu "takip et"e alabilir (bkz. model.go'nun ctrl+t'si).
// Ağdan (forum/sohbet/site gossip'i) yeni bir kayıt bu düğümün YEREL
// indeksine GERÇEKTEN eklendiğinde/güncellendiğinde (bkz. Ekle/
// EkleYaGuncelle'nin çağırdığı takipEslestir), o kaydı eşleştiren her
// kayıtlı arama için yeni_eslesme sayacı artırılır — kullanıcı bir sonraki
// açtığında "3 yeni" gibi bir rozet görür (bkz. TakipListele/TakipGoruldu).
//
// Bu tamamen YEREL bir defterdir: ağa hiçbir şey yayınlanmaz, yalnızca bu
// düğümün zaten (gossip/sync yoluyla) gördüğü içerik değerlendirilir —
// salt-okunur eş ilkesini (bkz. README'nin "Kapsam ve sınırlar"ı) bozmaz.

// KayitliArama, TakipListele'nin bir satırıdır.
type KayitliArama struct {
	ID           int64
	Sorgu        string
	Tip          SonucTip
	YazarKey     string
	OlusturmaTS  time.Time
	YeniEslesme  int
	SonEslesmeTS time.Time
}

// TakipEkle, geçerli arama kombinasyonunu (sorgu boş olabilir ama bu
// durumda yazarKey dolu olmalı — ikisi de boşsa "tüm ağı takip et" anlamına
// gelir ki bu anlamsızdır) kayıtlı aramalara ekler. Aynı üçlü zaten
// kayıtlıysa (bkz. UNIQUE kısıtı) sessizce no-op'tur — ikinci bir "takip
// et" basışı hata vermez, yalnızca tekrar eklemez.
func (d *Depo) TakipEkle(sorgu string, tip SonucTip, yazarKey string) error {
	sorgu = strings.TrimSpace(sorgu)
	if sorgu == "" && yazarKey == "" {
		return fmt.Errorf("takip edilecek bir sorgu ya da yazar filtresi gerekli")
	}
	_, err := d.db.Exec(`INSERT OR IGNORE INTO kayitli_arama (sorgu, tip, yazar_key, olusturma_ts)
		VALUES (?, ?, ?, ?)`, sorgu, string(tip), yazarKey, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// TakipSil, bir kayıtlı aramayı kalıcı olarak kaldırır.
func (d *Depo) TakipSil(id int64) error {
	_, err := d.db.Exec(`DELETE FROM kayitli_arama WHERE id = ?`, id)
	return err
}

// TakipGoruldu, bir kayıtlı aramanın yeni_eslesme sayacını sıfırlar —
// kullanıcı onu açıp sonuçlarını gördüğünde (bkz. model.go'nun ekTakip
// ekranı) çağrılır.
func (d *Depo) TakipGoruldu(id int64) error {
	_, err := d.db.Exec(`UPDATE kayitli_arama SET yeni_eslesme = 0 WHERE id = ?`, id)
	return err
}

// TakipListele, tüm kayıtlı aramaları en yeni eklenenden en eskiye döner.
func (d *Depo) TakipListele() ([]KayitliArama, error) {
	rows, err := d.db.Query(`SELECT id, sorgu, tip, yazar_key, olusturma_ts, yeni_eslesme, son_eslesme_ts
		FROM kayitli_arama ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("kayıtlı aramalar alınamadı: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []KayitliArama
	for rows.Next() {
		var k KayitliArama
		var tip, olusturmaTS, sonEslesmeTS string
		if err := rows.Scan(&k.ID, &k.Sorgu, &tip, &k.YazarKey, &olusturmaTS, &k.YeniEslesme, &sonEslesmeTS); err != nil {
			return nil, err
		}
		k.Tip = SonucTip(tip)
		if ts, perr := time.Parse(time.RFC3339Nano, olusturmaTS); perr == nil {
			k.OlusturmaTS = ts
		}
		if ts, perr := time.Parse(time.RFC3339Nano, sonEslesmeTS); perr == nil {
			k.SonEslesmeTS = ts
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// TakipToplamYeni, TÜM kayıtlı aramalardaki toplam okunmamış eşleşme
// sayısını döner — ana menüde "Takip Edilenler" seçeneğinin yanındaki
// rozet için (bkz. view.go'nun viewAnaMenu'sü).
func (d *Depo) TakipToplamYeni() (int, error) {
	var toplam int
	err := d.db.QueryRow(`SELECT COALESCE(SUM(yeni_eslesme), 0) FROM kayitli_arama`).Scan(&toplam)
	return toplam, err
}

// takipEslestir, YENİ (ya da güncellenmiş) bir kaydın hangi kayıtlı
// aramaları eşleştirdiğini bulup onların yeni_eslesme sayaçlarını artırır.
// Ekle/EkleYaGuncelle'nin BAŞARILI bir commit'ten SONRA çağırdığı, hataları
// yutan (best-effort) bir iç fonksiyondur — bir takip eşleştirmesinin
// başarısız olması ana ingest akışını (asıl iş: içeriği indekslemek)
// ASLA engellememelidir.
//
// Eşleştirme, tıpkı dogrulaAgSonucu'nun yaptığı gibi, YENİ bir mini-sorgu
// dili İCAT ETMEK yerine indeksin KENDİ arama mantığını (ftsSorguOlustur +
// FTS5 MATCH) yeniden kullanır — "bu kayıt sorgu X ile eşleşir mi" sorusu,
// "kaynak_id = bu kayıt VE MATCH X" filtresiyle birebir aynı anlama gelir.
func (d *Depo) takipEslestir(s AramaSonucu) {
	rows, err := d.db.Query(`SELECT id, sorgu FROM kayitli_arama
		WHERE (tip = '' OR tip = ?) AND (yazar_key = '' OR yazar_key = ?)`, string(s.Tip), s.YazarKey)
	if err != nil {
		return
	}
	type aday struct {
		id    int64
		sorgu string
	}
	var adaylar []aday
	for rows.Next() {
		var a aday
		if rows.Scan(&a.id, &a.sorgu) == nil {
			adaylar = append(adaylar, a)
		}
	}
	rows.Close() //nolint:errcheck
	if len(adaylar) == 0 {
		return
	}

	simdi := time.Now().UTC().Format(time.RFC3339Nano)
	for _, a := range adaylar {
		eslesti := false
		if strings.TrimSpace(a.sorgu) == "" {
			// Sorgu yok — yalnızca tür/yazar filtresi zaten yukarıdaki WHERE
			// ile sağlandı, bu YENİ kayıt doğrudan eşleşir.
			eslesti = true
		} else if ftsSorgu := ftsSorguOlustur(a.sorgu); ftsSorgu != "" {
			var varMi int
			if err := d.db.QueryRow(`SELECT 1 FROM arama_fts WHERE kaynak_id = ? AND arama_fts MATCH ?`,
				s.KaynakID, ftsSorgu).Scan(&varMi); err == nil {
				eslesti = true
			}
		}
		if eslesti {
			_, _ = d.db.Exec(`UPDATE kayitli_arama SET yeni_eslesme = yeni_eslesme + 1, son_eslesme_ts = ? WHERE id = ?`,
				simdi, a.id)
		}
	}
}
