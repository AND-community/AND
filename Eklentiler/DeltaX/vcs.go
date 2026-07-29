package main

// vcs.go — DeltaX'in içerik-adresli sürüm kontrol çekirdeği.
//
// Üç nesne tipi vardır:
//   'B' Blob   — ham dosya içeriği
//   'T' Agac   — bir dizin anlık görüntüsü (yol → blob hash eşlemi)
//   'C' Commit — bir ağaç + ebeveyn commit'ler + yazar/mesaj/zaman
//
// Nesneler sha256(tipByte + payload) anahtarıyla adreslenir ve
// nesneler/<hash[:2]>/<hash[2:]> altında saklanır. Aynı içerik iki kez
// yazılmaz (otomatik tekilleştirme).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxDosyaBoyutu = 200 * 1024 * 1024 // 200 MB — daha büyük dosyalar anlık görüntüye dahil edilmez
const maxDiffBoyutu = 512 * 1024         // 512 KB — daha büyük dosyalarda satır bazlı fark hesaplanmaz

// maxDiffSatir, satirDiff'in O(satır²) bellek kullanan LCS tablosunu sınırlar.
// Bayt sınırı tek başına yetmez: çok sayıda kısa satır içeren bir dosya
// maxDiffBoyutu altında kalırken yine de milyonlarca satıra sahip olabilir.
const maxDiffSatir = 2000

var reHex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ─── Nesne deposu ───────────────────────────────────────────────────────────

func nesneYaz(dataDir, projeID string, tip byte, payload []byte) (string, error) {
	full := append([]byte{tip}, payload...)
	sum := sha256.Sum256(full)
	hash := hex.EncodeToString(sum[:])

	dir := filepath.Join(projeNesneDizini(dataDir, projeID), hash[:2])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hash[2:])
	if _, err := os.Stat(path); err == nil {
		return hash, nil // içerik zaten var — tekilleştir
	}
	if err := os.WriteFile(path, full, 0o600); err != nil {
		return "", err
	}
	return hash, nil
}

func nesneYolu(dataDir, projeID, hash string) (string, error) {
	if !reHex64.MatchString(hash) {
		return "", fmt.Errorf("geçersiz nesne hash'i")
	}
	return filepath.Join(projeNesneDizini(dataDir, projeID), hash[:2], hash[2:]), nil
}

func nesneOku(dataDir, projeID, hash string) (tip byte, payload []byte, err error) {
	path, err := nesneYolu(dataDir, projeID, hash)
	if err != nil {
		return 0, nil, err
	}
	full, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	if len(full) < 1 {
		return 0, nil, fmt.Errorf("bozuk nesne: %s", hash)
	}
	return full[0], full[1:], nil
}

func nesneVarMi(dataDir, projeID, hash string) bool {
	path, err := nesneYolu(dataDir, projeID, hash)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// ─── Ağaç / Commit tipleri ──────────────────────────────────────────────────

type AgacGirdi struct {
	Yol   string `json:"yol"`
	Hash  string `json:"hash"`
	Boyut int64  `json:"boyut"`
}

type Agac struct {
	Girdiler []AgacGirdi `json:"girdiler"`
}

type Commit struct {
	Agac       string    `json:"agac"`
	Ebeveynler []string  `json:"ebeveynler,omitempty"`
	Yazar      string    `json:"yazar"`
	Mesaj      string    `json:"mesaj"`
	TS         time.Time `json:"ts"`
}

func agacOku(dataDir, projeID, hash string) (Agac, error) {
	var a Agac
	if hash == "" {
		return a, nil
	}
	tip, payload, err := nesneOku(dataDir, projeID, hash)
	if err != nil {
		return a, err
	}
	if tip != 'T' {
		return a, fmt.Errorf("beklenmeyen nesne tipi: ağaç bekleniyordu")
	}
	err = json.Unmarshal(payload, &a)
	return a, err
}

func agacHaritala(dataDir, projeID, hash string) (map[string]AgacGirdi, error) {
	a, err := agacOku(dataDir, projeID, hash)
	if err != nil {
		return nil, err
	}
	m := make(map[string]AgacGirdi, len(a.Girdiler))
	for _, g := range a.Girdiler {
		m[g.Yol] = g
	}
	return m, nil
}

func commitOku(dataDir, projeID, hash string) (Commit, error) {
	var c Commit
	tip, payload, err := nesneOku(dataDir, projeID, hash)
	if err != nil {
		return c, err
	}
	if tip != 'C' {
		return c, fmt.Errorf("beklenmeyen nesne tipi: commit bekleniyordu")
	}
	err = json.Unmarshal(payload, &c)
	return c, err
}

func commitOlustur(dataDir, projeID, agacHash string, ebeveynler []string, yazar, mesaj string) (string, error) {
	c := Commit{Agac: agacHash, Ebeveynler: ebeveynler, Yazar: yazar, Mesaj: mesaj, TS: time.Now().UTC()}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return nesneYaz(dataDir, projeID, 'C', payload)
}

// ─── Çalışma dizini ↔ ağaç ──────────────────────────────────────────────────

func agacOlustur(dataDir, projeID, calismaDir string) (hash string, girdiler []AgacGirdi, atlanan []string, err error) {
	desenler := yokSayDesenleriOku(calismaDir)
	err = filepath.WalkDir(calismaDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		rel, rerr := filepath.Rel(calismaDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if yokSayilsinMi(desenler, rel) {
			return nil
		}
		if info.Size() > maxDosyaBoyutu {
			atlanan = append(atlanan, rel)
			return nil
		}
		data, rerr2 := os.ReadFile(path)
		if rerr2 != nil {
			return rerr2
		}
		bh, werr2 := nesneYaz(dataDir, projeID, 'B', data)
		if werr2 != nil {
			return werr2
		}
		girdiler = append(girdiler, AgacGirdi{Yol: rel, Hash: bh, Boyut: int64(len(data))})
		return nil
	})
	if err != nil {
		return "", nil, atlanan, err
	}
	sort.Slice(girdiler, func(i, j int) bool { return girdiler[i].Yol < girdiler[j].Yol })
	payload, merr := json.Marshal(Agac{Girdiler: girdiler})
	if merr != nil {
		return "", nil, atlanan, merr
	}
	hash, err = nesneYaz(dataDir, projeID, 'T', payload)
	return hash, girdiler, atlanan, err
}

type workDosya struct {
	Hash  string
	Boyut int64
}

func calismaSnapshotHesapla(calismaDir string) (map[string]workDosya, error) {
	sonuc := map[string]workDosya{}
	desenler := yokSayDesenleriOku(calismaDir)
	err := filepath.WalkDir(calismaDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		if info.Size() > maxDosyaBoyutu {
			return nil
		}
		rel, rerr := filepath.Rel(calismaDir, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if yokSayilsinMi(desenler, rel) {
			return nil
		}
		data, rerr2 := os.ReadFile(path)
		if rerr2 != nil {
			return rerr2
		}
		sum := sha256.Sum256(append([]byte{'B'}, data...))
		sonuc[rel] = workDosya{Hash: hex.EncodeToString(sum[:]), Boyut: int64(len(data))}
		return nil
	})
	return sonuc, err
}

type DurumSonucu struct {
	Eklenen []string
	Degisen []string
	Silinen []string
}

func (d DurumSonucu) temizMi() bool {
	return len(d.Eklenen) == 0 && len(d.Degisen) == 0 && len(d.Silinen) == 0
}

func durumHesapla(dataDir, projeID, calismaDir, headTree string) (DurumSonucu, error) {
	work, err := calismaSnapshotHesapla(calismaDir)
	if err != nil {
		return DurumSonucu{}, err
	}
	head, err := agacHaritala(dataDir, projeID, headTree)
	if err != nil {
		return DurumSonucu{}, err
	}
	var d DurumSonucu
	for yol, wf := range work {
		if hg, ok := head[yol]; !ok {
			d.Eklenen = append(d.Eklenen, yol)
		} else if hg.Hash != wf.Hash {
			d.Degisen = append(d.Degisen, yol)
		}
	}
	for yol := range head {
		if _, ok := work[yol]; !ok {
			d.Silinen = append(d.Silinen, yol)
		}
	}
	sort.Strings(d.Eklenen)
	sort.Strings(d.Degisen)
	sort.Strings(d.Silinen)
	return d, nil
}

// dalaGec, çalışma dizinini hedef ağacın içeriğiyle tamamen değiştirir.
// Çağıran taraf, veri kaybını önlemek için önce durumHesapla ile çalışma
// dizininin temiz olduğunu doğrulamalıdır.
func dalaGec(dataDir, projeID, calismaDir, hedefTree string) error {
	entries, err := os.ReadDir(calismaDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(calismaDir, e.Name())); err != nil {
			return err
		}
	}
	agac, err := agacOku(dataDir, projeID, hedefTree)
	if err != nil {
		return err
	}
	for _, g := range agac.Girdiler {
		if err := dosyayiCalismayaYaz(dataDir, projeID, calismaDir, g); err != nil {
			return err
		}
	}
	return nil
}

func dosyayiCalismayaYaz(dataDir, projeID, calismaDir string, g AgacGirdi) error {
	_, data, err := nesneOku(dataDir, projeID, g.Hash)
	if err != nil {
		return err
	}
	full := filepath.Join(calismaDir, filepath.FromSlash(g.Yol))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o600)
}

// ─── Commit geçmişi ─────────────────────────────────────────────────────────

type LogGirdisi struct {
	Hash   string
	Commit Commit
}

// gunlukAl, ilk-ebeveyn zincirini izleyerek commit geçmişini döndürür.
func gunlukAl(dataDir, projeID, head string, limit int) []LogGirdisi {
	var out []LogGirdisi
	gorulen := map[string]bool{}
	h := head
	for h != "" && len(out) < limit && !gorulen[h] {
		gorulen[h] = true
		c, err := commitOku(dataDir, projeID, h)
		if err != nil {
			break
		}
		out = append(out, LogGirdisi{Hash: h, Commit: c})
		if len(c.Ebeveynler) == 0 {
			break
		}
		h = c.Ebeveynler[0]
	}
	return out
}

// gunlukFiltrele, sorgu boşsa girdilerin tamamını, değilse yalnızca mesajı,
// yazarı ya da hash'i sorguyu (büyük/küçük harfe duyarsız) içerenleri döndürür.
func gunlukFiltrele(girdiler []LogGirdisi, sorgu string) []LogGirdisi {
	sorgu = strings.TrimSpace(sorgu)
	if sorgu == "" {
		return girdiler
	}
	q := strings.ToLower(sorgu)
	var out []LogGirdisi
	for _, g := range girdiler {
		if strings.Contains(strings.ToLower(g.Commit.Mesaj), q) ||
			strings.Contains(strings.ToLower(g.Commit.Yazar), q) ||
			strings.Contains(strings.ToLower(g.Hash), q) {
			out = append(out, g)
		}
	}
	return out
}

func atalariTopla(dataDir, projeID, baslangic string) map[string]bool {
	gorulen := map[string]bool{}
	if baslangic == "" {
		return gorulen
	}
	kuyruk := []string{baslangic}
	for len(kuyruk) > 0 {
		h := kuyruk[0]
		kuyruk = kuyruk[1:]
		if h == "" || gorulen[h] {
			continue
		}
		gorulen[h] = true
		c, err := commitOku(dataDir, projeID, h)
		if err != nil {
			continue
		}
		kuyruk = append(kuyruk, c.Ebeveynler...)
	}
	return gorulen
}

// ortakAta, iki commit'in en yakın ortak atasını (birleştirme tabanı) bulur.
func ortakAta(dataDir, projeID, a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	if a == b {
		return a
	}
	atalarA := atalariTopla(dataDir, projeID, a)
	gorulen := map[string]bool{}
	kuyruk := []string{b}
	for len(kuyruk) > 0 {
		h := kuyruk[0]
		kuyruk = kuyruk[1:]
		if h == "" || gorulen[h] {
			continue
		}
		gorulen[h] = true
		if atalarA[h] {
			return h
		}
		c, err := commitOku(dataDir, projeID, h)
		if err != nil {
			continue
		}
		kuyruk = append(kuyruk, c.Ebeveynler...)
	}
	return ""
}

// ─── Satır bazlı fark (LCS) ─────────────────────────────────────────────────

type DiffSatiri struct {
	Tip   byte // ' ' aynı, '+' eklendi, '-' silindi
	Metin string
}

func satirDiff(eski, yeni []byte) []DiffSatiri {
	a := strings.Split(string(eski), "\n")
	b := strings.Split(string(yeni), "\n")
	n, m := len(a), len(b)

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var out []DiffSatiri
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, DiffSatiri{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, DiffSatiri{'-', a[i]})
			i++
		default:
			out = append(out, DiffSatiri{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, DiffSatiri{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, DiffSatiri{'+', b[j]})
	}
	return out
}

// ─── Üçlü birleştirme (three-way merge) ─────────────────────────────────────

// birlestirGirdi, tek bir dosya yolu için temel/yerel/uzak durumlarını
// karşılaştırıp sonucu belirler.
func birlestirGirdi(bok bool, b AgacGirdi, lok bool, l AgacGirdi, rok bool, r AgacGirdi) (girdi AgacGirdi, varMi, catisma bool) {
	switch {
	case lok && rok && l.Hash == r.Hash:
		return l, true, false
	case !lok && !rok:
		return AgacGirdi{}, false, false
	case lok && !rok:
		if bok && b.Hash == l.Hash {
			return AgacGirdi{}, false, false // uzakta silinmiş, yerel değişmemiş → sil
		}
		if !bok {
			return l, true, false // yalnız yerelde yeni
		}
		return l, true, true // yerel değişti, uzak sildi → çakışma
	case !lok && rok:
		if bok && b.Hash == r.Hash {
			return AgacGirdi{}, false, false
		}
		if !bok {
			return r, true, false
		}
		return r, true, true
	default: // lok && rok, hash'ler farklı
		if bok && b.Hash == l.Hash {
			return r, true, false // yalnız uzak değişti
		}
		if bok && b.Hash == r.Hash {
			return l, true, false // yalnız yerel değişti
		}
		return l, true, true // ikisi de değişti → çakışma
	}
}

func birlesikYollar(setler ...map[string]AgacGirdi) []string {
	gorulen := map[string]bool{}
	var out []string
	for _, s := range setler {
		for yol := range s {
			if !gorulen[yol] {
				gorulen[yol] = true
				out = append(out, yol)
			}
		}
	}
	sort.Strings(out)
	return out
}

// birlestirmeUygula, temel/yerel/uzak ağaçları üçlü birleştirir ve sonucu
// doğrudan çalışma dizinine yazar. Çakışan dosyalara
// <<<<<<< / ======= / >>>>>>> işaretleri eklenir; kullanıcı bunları elle
// çözüp tekrar kaydetmelidir.
func birlestirmeUygula(dataDir, projeID, calismaDir, temelTree, yerelTree, uzakTree string) ([]string, error) {
	temel, err := agacHaritala(dataDir, projeID, temelTree)
	if err != nil {
		return nil, err
	}
	yerel, err := agacHaritala(dataDir, projeID, yerelTree)
	if err != nil {
		return nil, err
	}
	uzak, err := agacHaritala(dataDir, projeID, uzakTree)
	if err != nil {
		return nil, err
	}

	var catismalar []string
	for _, yol := range birlesikYollar(temel, yerel, uzak) {
		b, bok := temel[yol]
		l, lok := yerel[yol]
		r, rok := uzak[yol]
		girdi, varMi, catisma := birlestirGirdi(bok, b, lok, l, rok, r)

		full := filepath.Join(calismaDir, filepath.FromSlash(yol))
		if !varMi {
			_ = os.Remove(full)
			continue
		}
		if catisma {
			catismalar = append(catismalar, yol)
			var lData, rData []byte
			if lok {
				_, lData, _ = nesneOku(dataDir, projeID, l.Hash)
			}
			if rok {
				_, rData, _ = nesneOku(dataDir, projeID, r.Hash)
			}
			icerik := fmt.Sprintf("<<<<<<< YEREL\n%s\n=======\n%s\n>>>>>>> UZAK\n", string(lData), string(rData))
			if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
				return catismalar, err
			}
			if err := os.WriteFile(full, []byte(icerik), 0o600); err != nil {
				return catismalar, err
			}
			continue
		}
		if err := dosyayiCalismayaYaz(dataDir, projeID, calismaDir, girdi); err != nil {
			return catismalar, err
		}
	}
	return catismalar, nil
}

// dosyaFarkiHesapla, HEAD ağacındaki bir dosyayla çalışma dizinindeki
// karşılığı arasında satır bazlı fark üretir. Dosya HEAD'te yoksa
// (yeni eklenmiş) ya da çalışma dizininde yoksa (silinmiş) ilgili taraf
// boş kabul edilir.
func dosyaFarkiHesapla(dataDir, projeID, calismaDir, headTree, yol string) ([]DiffSatiri, error) {
	headMap, err := agacHaritala(dataDir, projeID, headTree)
	if err != nil {
		return nil, err
	}
	var eski []byte
	if g, ok := headMap[yol]; ok {
		if g.Boyut > maxDiffBoyutu {
			return nil, fmt.Errorf("dosya çok büyük — satır bazlı fark atlandı: %s", yol)
		}
		_, eski, err = nesneOku(dataDir, projeID, g.Hash)
		if err != nil {
			return nil, err
		}
	}
	full := filepath.Join(calismaDir, filepath.FromSlash(yol))
	var yeni []byte
	if info, err := os.Stat(full); err == nil {
		if info.Size() > maxDiffBoyutu {
			return nil, fmt.Errorf("dosya çok büyük — satır bazlı fark atlandı: %s", yol)
		}
		yeni, err = os.ReadFile(full)
		if err != nil {
			return nil, err
		}
	}
	if satirSayisi(eski) > maxDiffSatir || satirSayisi(yeni) > maxDiffSatir {
		return nil, fmt.Errorf("dosya çok fazla satır içeriyor — satır bazlı fark atlandı: %s", yol)
	}
	return satirDiff(eski, yeni), nil
}

func satirSayisi(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	return bytes.Count(data, []byte{'\n'}) + 1
}

func kisaHash(h string) string {
	if len(h) > 10 {
		return h[:10]
	}
	return h
}
