package main

// store.go — yerel kalıcılık katmanı: gruplar ve projeler.
//
// Hiçbir veri merkezi bir sunucuya gönderilmez. Her düğüm kendi
// AND_DATA_DIR/DeltaX/ dizininde grup ve proje durumunu tutar; eşler
// arası senkronizasyon sync.go içindeki DM + dosya aktarım protokolüyle
// yapılır (bkz. sync.go).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const pluginDirName = "DeltaX"

func kokDizin(dataDir string) string       { return filepath.Join(dataDir, pluginDirName) }
func gruplarDizini(dataDir string) string  { return filepath.Join(kokDizin(dataDir), "gruplar") }
func projelerDizini(dataDir string) string { return filepath.Join(kokDizin(dataDir), "projeler") }
func gecicilerDizini() string              { return filepath.Join(os.TempDir(), "and-deltax") }

func rastgeleID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ─── Bekleyen davetler ──────────────────────────────────────────────────────
//
// Kabul edilmeden eklenti kapatılırsa/AND yeniden başlatılırsa davetin iz
// bırakmadan kaybolmaması için (öncesinde yalnızca bellekte tutuluyordu)
// diske yazılır — grup/proje dosyalarıyla aynı kalıcılık deseni.

func davetlerYolu(dataDir string) string {
	return filepath.Join(kokDizin(dataDir), "bekleyen_davetler.json")
}

func davetleriKaydet(dataDir string, davetler []bekleyenDavet) error {
	if dataDir == "" {
		return errors.New("AND_DATA_DIR tanımlı değil")
	}
	if err := os.MkdirAll(kokDizin(dataDir), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(davetler, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(davetlerYolu(dataDir), data, 0o600)
}

// davetleriYukle, kalıcı davet listesini okur — dosya yoksa ya da
// bozuksa (gruplariListele/projeleriListele ile aynı hoşgörülü desen) boş
// slice döner, eklenti başlangıçta hiç davet yokmuş gibi devam eder.
func davetleriYukle(dataDir string) []bekleyenDavet {
	data, err := os.ReadFile(davetlerYolu(dataDir))
	if err != nil {
		return nil
	}
	var davetler []bekleyenDavet
	if json.Unmarshal(data, &davetler) != nil {
		return nil
	}
	return davetler
}

// ─── Üye / Grup ─────────────────────────────────────────────────────────────

// Uye bir grup üyesinin kimlik bilgilerini tutar.
type Uye struct {
	Ad     string `json:"ad"`
	PeerID string `json:"peer_id"`
	PubKey string `json:"pub_key,omitempty"`
}

// Grup, merkezi ağdan bağımsız özel bir topluluktur. Sir alanı grup
// üyeleri arasında paylaşılan AES-256 anahtarıdır — proje paketleri bu
// anahtarla şifrelenir, böylece veriler yalnızca grup üyeleri tarafından
// çözülebilir (bkz. crypto.go, sync.go).
type Grup struct {
	ID         string `json:"id"`
	Ad         string `json:"ad"`
	Sir        string `json:"sir"`
	KurucuAd   string `json:"kurucu_ad"`
	KurucuPeer string `json:"kurucu_peer"`
	Uyeler     []Uye  `json:"uyeler"`
	// Yoneticiler, kurucu tarafından yetkilendirilmiş "yedek kurucu"
	// üyelerdir — kurucuyla aynı şekilde üye davet edebilir ve grubu
	// kapatabilirler. Kurucunun cihazı/erişimi kaybolursa grup yönetimsiz
	// kalmasın diye vardır. Yalnızca gerçek kurucu bu listeyi değiştirebilir.
	Yoneticiler []Uye     `json:"yoneticiler,omitempty"`
	OlusturmaTS time.Time `json:"olusturma_ts"`
}

func (g *Grup) benKurucuMuyum(kendiPeerID string) bool { return g.KurucuPeer == kendiPeerID }

func (g *Grup) yoneticiMi(peerID string) bool {
	for _, y := range g.Yoneticiler {
		if y.PeerID == peerID {
			return true
		}
	}
	return false
}

// yetkiliMi, peerID'nin kurucu ya da yönetici olup olmadığını (yani üye
// davet etme / grubu kapatma yetkisi olup olmadığını) döndürür.
func (g *Grup) yetkiliMi(peerID string) bool {
	return g.benKurucuMuyum(peerID) || g.yoneticiMi(peerID)
}

func (g *Grup) uyeMi(peerID string) bool {
	for _, u := range g.Uyeler {
		if u.PeerID == peerID {
			return true
		}
	}
	return false
}

// uyeEkleGuncelle üyeyi ekler ya da peer ID eşleşiyorsa bilgilerini günceller.
func (g *Grup) uyeEkleGuncelle(u Uye) {
	for i, mevcut := range g.Uyeler {
		if mevcut.PeerID == u.PeerID {
			g.Uyeler[i] = u
			return
		}
	}
	g.Uyeler = append(g.Uyeler, u)
}

// uyeCikar, peer ID'si eşleşen üyeyi listeden kaldırır.
func (g *Grup) uyeCikar(peerID string) {
	for i, u := range g.Uyeler {
		if u.PeerID == peerID {
			g.Uyeler = append(g.Uyeler[:i], g.Uyeler[i+1:]...)
			return
		}
	}
}

// yoneticiDurumunuDegistir, u'yu yönetici yapar (varMi=true) ya da
// yöneticilikten çıkarır (varMi=false). Kurucu bu şekilde yönetici
// atayamaz/görevden alamaz — kurucu zaten benKurucuMuyum ile ayrı kontrol edilir.
func (g *Grup) yoneticiDurumunuDegistir(u Uye, varMi bool) {
	for i, y := range g.Yoneticiler {
		if y.PeerID == u.PeerID {
			if !varMi {
				g.Yoneticiler = append(g.Yoneticiler[:i], g.Yoneticiler[i+1:]...)
			}
			return
		}
	}
	if varMi {
		g.Yoneticiler = append(g.Yoneticiler, u)
	}
}

func grupYolu(dataDir, id string) string { return filepath.Join(gruplarDizini(dataDir), id+".json") }

// grupSil, grubun yerel kaydını siler. Grubun projeleri (nesneler, çalışma
// klasörü) kasıtlı olarak diskte bırakılır — kaydedilmemiş yerel çalışma bu
// yüzden kaybolmaz; kullanıcı gruba tekrar katılırsa proje verileri yerinde
// kalmaya devam eder.
func grupSil(dataDir, id string) error {
	err := os.Remove(grupYolu(dataDir, id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func grupKaydet(dataDir string, g Grup) error {
	if dataDir == "" {
		return errors.New("AND_DATA_DIR tanımlı değil")
	}
	if err := os.MkdirAll(gruplarDizini(dataDir), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(grupYolu(dataDir, g.ID), data, 0o600)
}

func grupOku(dataDir, id string) (Grup, error) {
	var g Grup
	data, err := os.ReadFile(grupYolu(dataDir, id))
	if err != nil {
		return g, err
	}
	err = json.Unmarshal(data, &g)
	return g, err
}

func gruplariListele(dataDir string) []Grup {
	entries, err := os.ReadDir(gruplarDizini(dataDir))
	if err != nil {
		return nil
	}
	var out []Grup
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if g, err := grupOku(dataDir, id); err == nil {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OlusturmaTS.After(out[j].OlusturmaTS) })
	return out
}

// gruplarFiltrele, grup adı ya da kurucu adına göre (büyük/küçük harf duyarsız,
// alt dizge) filtreler — gunlukFiltrele (vcs.go) ile aynı desen.
func gruplarFiltrele(gruplar []Grup, sorgu string) []Grup {
	sorgu = strings.TrimSpace(sorgu)
	if sorgu == "" {
		return gruplar
	}
	q := strings.ToLower(sorgu)
	var out []Grup
	for _, g := range gruplar {
		if strings.Contains(strings.ToLower(g.Ad), q) || strings.Contains(strings.ToLower(g.KurucuAd), q) {
			out = append(out, g)
		}
	}
	return out
}

// ─── Proje ──────────────────────────────────────────────────────────────────

// Proje bir grup içindeki tek bir sürüm kontrollü depoyu temsil eder.
// Dallar dal adı → commit hash eşlemesidir; Uzaklar ise "peerID/dal"
// biçiminde anahtarlarla diğer üyelerden alınan en son bilinen uç
// noktaları (bir üyenin ulaştığı en son kayıt) tutar.
type Proje struct {
	ID          string            `json:"id"`
	GrupID      string            `json:"grup_id"`
	Ad          string            `json:"ad"`
	OlusturmaTS time.Time         `json:"olusturma_ts"`
	Dallar      map[string]string `json:"dallar"`
	AktifDal    string            `json:"aktif_dal"`
	Uzaklar     map[string]string `json:"uzaklar,omitempty"`
}

func projeKokDizini(dataDir, id string) string {
	return filepath.Join(projelerDizini(dataDir), id)
}
func projeMetaYolu(dataDir, id string) string {
	return filepath.Join(projeKokDizini(dataDir, id), "meta.json")
}
func projeNesneDizini(dataDir, id string) string {
	return filepath.Join(projeKokDizini(dataDir, id), "nesneler")
}
func projeCalismaDizini(dataDir, id string) string {
	return filepath.Join(projeKokDizini(dataDir, id), "calisma")
}
func projeBirlesmeYolu(dataDir, id string) string {
	return filepath.Join(projeKokDizini(dataDir, id), "MERGE_HEAD")
}

func projeOlustur(dataDir, grupID, ad string) (Proje, error) {
	p := Proje{
		ID:          rastgeleID(),
		GrupID:      grupID,
		Ad:          ad,
		OlusturmaTS: time.Now().UTC(),
		Dallar:      map[string]string{"ana": ""},
		AktifDal:    "ana",
	}
	if err := os.MkdirAll(projeNesneDizini(dataDir, p.ID), 0o700); err != nil {
		return p, err
	}
	if err := os.MkdirAll(projeCalismaDizini(dataDir, p.ID), 0o700); err != nil {
		return p, err
	}
	return p, projeKaydet(dataDir, p)
}

func projeKaydet(dataDir string, p Proje) error {
	if dataDir == "" {
		return errors.New("AND_DATA_DIR tanımlı değil")
	}
	if err := os.MkdirAll(projeKokDizini(dataDir, p.ID), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(projeMetaYolu(dataDir, p.ID), data, 0o600)
}

func projeOku(dataDir, id string) (Proje, error) {
	var p Proje
	data, err := os.ReadFile(projeMetaYolu(dataDir, id))
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(data, &p)
	if p.Dallar == nil {
		p.Dallar = map[string]string{}
	}
	return p, err
}

func projeleriListele(dataDir, grupID string) []Proje {
	entries, err := os.ReadDir(projelerDizini(dataDir))
	if err != nil {
		return nil
	}
	var out []Proje
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := projeOku(dataDir, e.Name())
		if err != nil || p.GrupID != grupID {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OlusturmaTS.After(out[j].OlusturmaTS) })
	return out
}

func birlesmeBekleniyorMu(dataDir, projeID string) (string, bool) {
	data, err := os.ReadFile(projeBirlesmeYolu(dataDir, projeID))
	if err != nil {
		return "", false
	}
	h := strings.TrimSpace(string(data))
	return h, h != ""
}

func birlesmeIsaretle(dataDir, projeID, ebeveynHash string) error {
	return os.WriteFile(projeBirlesmeYolu(dataDir, projeID), []byte(ebeveynHash), 0o600)
}

func birlesmeTemizle(dataDir, projeID string) {
	_ = os.Remove(projeBirlesmeYolu(dataDir, projeID))
}
