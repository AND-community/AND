package main

// yazimonerisi.go — Google/Yandex tarzı "şunu mu demek istediniz?" yazım
// önerisi. Yeni bir bağımlılık GEREKTİRMEZ: index.go'nun depoAc'ta zaten
// oluşturduğu terim_sozlugu (fts5vocab 'row' modu) sanal tablosundan
// indeksteki TÜM benzersiz terimleri okur, elle yazılmış bir Levenshtein
// (düzenleme mesafesi) fonksiyonuyla sorgudaki her kelimeye en yakın
// bilinen terimi bulur. Yalnızca arama SIFIR sonuç döndürdüğünde
// tetiklenir (bkz. model.go/cli.go'nun çağrı noktaları) — her aramada
// çalıştırmak gereksiz bir maliyet olurdu.

import (
	"strings"
	"unicode"
)

// mesafeEsigi, kelimenin uzunluğuna göre "yeterince yakın" kabul edilecek
// en fazla düzenleme mesafesini belirler. Kısa kelimelerde (ör. 3 harf)
// mesafe 1 bile anlamı tamamen değiştirebileceğinden eşik düşük tutulur;
// uzun kelimelerde tek bir yazım hatası oranı aynı kalsa da mutlak mesafe
// artabileceğinden eşik gevşetilir.
func mesafeEsigi(uzunluk int) int {
	switch {
	case uzunluk <= 3:
		return 1
	case uzunluk <= 7:
		return 2
	default:
		return 3
	}
}

// terimNormallestir, unicode61 remove_diacritics 2 tokenizer'ının katlama
// davranışıyla EŞLEŞECEK şekilde (ç→c, ğ→g, ö→o, ü→u, ş→s katlanır; noktasız
// ı KATLANMAZ — bu ikisi ampirik olarak doğrulandı) küçük harfe çevirir ve
// Türkçe aksanları sadeleştirir. Böylece kullanıcının yazdığı "İstanbul" ile
// indeksteki (tokenizer'dan geçmiş) "istanbul" terimi aynı düzleme gelir.
func terimNormallestir(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'ç':
			b.WriteRune('c')
		case 'ğ':
			b.WriteRune('g')
		case 'ö':
			b.WriteRune('o')
		case 'ü':
			b.WriteRune('u')
		case 'ş':
			b.WriteRune('s')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// duzenlemeMesafesi, a ve b arasındaki Levenshtein (düzenleme) mesafesini
// rune bazında (Türkçe çok baytlı karakterlerin DOĞRU sayılması için —
// byte bazlı olsaydı ör. 'ş' 2 byte olduğundan mesafe yanlış çıkardı)
// Wagner–Fischer iki-satırlı dinamik programlamayla hesaplar. Bellek O(min
// (len(a), len(b))) — tam matris yerine yalnızca iki satır tutulur.
func duzenlemeMesafesi(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) > len(rb) {
		ra, rb = rb, ra
	}
	onceki := make([]int, len(ra)+1)
	simdiki := make([]int, len(ra)+1)
	for i := range onceki {
		onceki[i] = i
	}
	for j := 1; j <= len(rb); j++ {
		simdiki[0] = j
		for i := 1; i <= len(ra); i++ {
			maliyet := 1
			if ra[i-1] == rb[j-1] {
				maliyet = 0
			}
			sil := onceki[i] + 1
			ekle := simdiki[i-1] + 1
			degistir := onceki[i-1] + maliyet
			en := sil
			if ekle < en {
				en = ekle
			}
			if degistir < en {
				en = degistir
			}
			simdiki[i] = en
		}
		onceki, simdiki = simdiki, onceki
	}
	return onceki[len(ra)]
}

// terimBilgisi, terim_sozlugu'ndan (fts5vocab 'row' modu) okunan tek bir
// satırı temsil eder: terim ve o terimin geçtiği belge sayısı (doc).
type terimBilgisi struct {
	terim       string
	belgeSayisi int
}

// tumTerimler, indeksteki tüm benzersiz terimleri terim_sozlugu sanal
// tablosundan okur. Bu fonksiyon KENDİ rows'unu tam tüketip kapattıktan
// SONRA döner — çağıranların üstünde AÇIK başka bir rows varken
// çağrılMAMALIDIR (bkz. index.go'nun AraGelismis/tamamla doc yorumundaki
// SetMaxOpenConns(1) kilitlenme uyarısı, aynı kısıtlama burada da geçerli).
func (d *Depo) tumTerimler() ([]terimBilgisi, error) {
	rows, err := d.db.Query(`SELECT term, doc FROM terim_sozlugu ORDER BY term`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var terimler []terimBilgisi
	for rows.Next() {
		var t terimBilgisi
		if err := rows.Scan(&t.terim, &t.belgeSayisi); err != nil {
			return nil, err
		}
		terimler = append(terimler, t)
	}
	return terimler, rows.Err()
}

// terimOnekBiliniyorMu, kelime*'nin (arama sorgularımızın varsayılan
// öneki eşleştirme biçimi — bkz. ftsTerimiKacirVeSar) indeksteki EN AZ BİR
// terimin ÖN EKİ olup olmadığını denetler. true dönerse, sorgu zaten en az
// bir belgeyle eşleşiyor demektir (ya da eşleşecekti — sıfır sonuç başka
// bir filtreden, ör. tip/yazar kısıtından gelmiş olabilir) ve yazım önerisi
// ANLAMSIZDIR; yalnızca hiçbir terim bu köke sahip DEĞİLSE gerçek bir
// yazım hatası adayıdır.
func terimOnekBiliniyorMu(terimler []terimBilgisi, kelime string) bool {
	kelime = terimNormallestir(kelime)
	if kelime == "" {
		return false
	}
	for _, t := range terimler {
		if strings.HasPrefix(t.terim, kelime) {
			return true
		}
	}
	return false
}

// enYakinTerim, terimler içinde kelime'ye Levenshtein mesafesi esik'i
// AŞMAYAN ve o mesafede en küçük olan terimi bulur. Eşit mesafeli birden
// fazla aday arasında, terim_sozlugu'nun 'doc' sütunundan gelen belge
// sayısı (kaç farklı içerikte geçtiği) en yüksek olan tercih edilir —
// sezgi: daha yaygın bir terim, kullanıcının kastettiği kelime olma
// ihtimali daha yüksektir (arama motorlarının klasik "popülerlik" sinyali).
func enYakinTerim(terimler []terimBilgisi, kelime string, esik int) (terim string, bulunduMu bool) {
	kelime = terimNormallestir(kelime)
	if kelime == "" {
		return "", false
	}
	enIyiMesafe := esik + 1
	enIyiBelgeSayisi := -1
	for _, t := range terimler {
		if t.terim == kelime {
			continue
		}
		m := duzenlemeMesafesi(kelime, t.terim)
		if m > esik {
			continue
		}
		if m < enIyiMesafe || (m == enIyiMesafe && t.belgeSayisi > enIyiBelgeSayisi) {
			enIyiMesafe = m
			enIyiBelgeSayisi = t.belgeSayisi
			terim = t.terim
			bulunduMu = true
		}
	}
	return terim, bulunduMu
}

// YazimOnerisiUret, sorgudaki (tırnak/hariç-tutma/OR gibi özel sözdizimi
// ATILMIŞ, yalnızca düz kelimeler üzerinde çalışan) her kelime için,
// bilinen bir kökle eşleşmeyen kelimeleri en yakın bilinen terimle
// değiştirmeye çalışır. En az bir kelime GERÇEKTEN değiştirilebildiyse
// önerilen tam sorguyu ve true döner; hiçbir kelime bilinmeyen DEĞİLSE
// (ör. sonuç sıfır olması başka bir nedenden, ör. filtre kısıtından
// geliyorsa) ya da hiçbiri için yakın bir terim bulunamadıysa "", false
// döner.
func (d *Depo) YazimOnerisiUret(sorgu string) (oneri string, bulunduMu bool) {
	kelimeler := strings.Fields(sorgu)
	if len(kelimeler) == 0 {
		return "", false
	}
	terimler, err := d.tumTerimler()
	if err != nil || len(terimler) == 0 {
		return "", false
	}

	degisti := false
	sonuc := make([]string, len(kelimeler))
	for i, kelime := range kelimeler {
		temiz := strings.TrimFunc(kelime, func(r rune) bool {
			return r == '"' || r == '-' || unicode.IsSpace(r)
		})
		if temiz == "" || temiz == "OR" {
			sonuc[i] = kelime
			continue
		}
		if terimOnekBiliniyorMu(terimler, temiz) {
			sonuc[i] = kelime
			continue
		}
		yeni, bulundu := enYakinTerim(terimler, temiz, mesafeEsigi(len([]rune(temiz))))
		if !bulundu {
			sonuc[i] = kelime
			continue
		}
		sonuc[i] = strings.Replace(kelime, temiz, yeni, 1)
		degisti = true
	}
	if !degisti {
		return "", false
	}
	return strings.Join(sonuc, " "), true
}
