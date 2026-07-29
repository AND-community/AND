// Package bbcode, forum ve eklenti metinlerinde kullanılan basit
// [code] / [code=dil] ... [/code] işaretlemesini, satır numaralı ve
// söz dizimi vurgulamalı bir "kod kutusu"na dönüştürür.
package bbcode

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// kodBlokRegex [code] veya [code=dil] ile başlayıp [/code] ile biten,
// çok satırlı ve büyük/küçük harf duyarsız blokları yakalar.
var kodBlokRegex = regexp.MustCompile(`(?is)\[code(?:=([a-zA-Z0-9_+#-]*))?\]\s*\n?(.*?)\s*\[/code\]`)

const bgKod = lipgloss.Color("236")

var (
	stKutu      = lipgloss.NewStyle().Background(bgKod).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("241")).Padding(0, 1)
	stDilRozeti = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("222")).Bold(true).Padding(0, 1)
	stSatirNo   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Background(bgKod)
	stAyrac     = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Background(bgKod)
	stNormal    = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(bgKod)
	stKeyword   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true).Background(bgKod)
	stString    = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Background(bgKod)
	stComment   = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true).Background(bgKod)
	stNumber    = lipgloss.NewStyle().Foreground(lipgloss.Color("216")).Background(bgKod)
)

var kodAcilisRegex = regexp.MustCompile(`(?i)\[code(?:=[a-zA-Z0-9_+#-]*)?\]`)
var kodKapanisRegex = regexp.MustCompile(`(?i)\[/code\]`)

// IcerirKod, metin içinde bir [code] etiketi olup olmadığını bildirir.
func IcerirKod(metin string) bool {
	return strings.Contains(strings.ToLower(metin), "[code")
}

// Render, metin içindeki [code]/[code=dil]...[/code] bloklarını vw
// genişliğine sığan, satır numaralı ve vurgulanmış bir kutuya çevirir;
// geri kalan metne dokunmaz.
func Render(metin string, vw int) string {
	if !IcerirKod(metin) {
		return metin
	}
	genislik := vw - 6
	if genislik < 10 {
		genislik = 10
	}
	return kodBlokRegex.ReplaceAllStringFunc(metin, func(esleme string) string {
		alt := kodBlokRegex.FindStringSubmatch(esleme)
		if len(alt) < 3 {
			return esleme
		}
		dil := strings.ToLower(strings.TrimSpace(alt[1]))
		icerik := strings.TrimRight(alt[2], "\n")
		kutu := renderKutu(icerik, dil, genislik)
		if dil != "" {
			return "\n" + stDilRozeti.Render(" "+dil+" ") + "\n" + kutu + "\n"
		}
		return "\n" + kutu + "\n"
	})
}

// Dengeli, metindeki [code]/[code=dil] açılış etiketleriyle [/code] kapanış
// etiketlerinin sayıca eşit olup olmadığını bildirir. Gönderi veya yanıt
// kaydedilmeden önce kapanmamış bir kod bloğuna karşı uyarmak için kullanılır.
func Dengeli(metin string) bool {
	return len(kodAcilisRegex.FindAllString(metin, -1)) == len(kodKapanisRegex.FindAllString(metin, -1))
}

func renderKutu(icerik, dil string, genislik int) string {
	satirlar := strings.Split(icerik, "\n")
	noGenislik := len(strconv.Itoa(len(satirlar)))
	kw := anahtarKelimeler(dil)

	var b strings.Builder
	for i, s := range satirlar {
		no := fmt.Sprintf("%*d", noGenislik, i+1)
		b.WriteString(stSatirNo.Render(no))
		b.WriteString(stAyrac.Render(" │ "))
		b.WriteString(vurgula(s, kw))
		if i < len(satirlar)-1 {
			b.WriteString("\n")
		}
	}
	return stKutu.Width(genislik).Render(b.String())
}

// vurgula tek bir kod satırını yorum/dize/sayı/anahtar-kelime olarak
// renklendirir. Basit karakter taramalı bir sözde-sözdizim çözümleyicidir;
// tam bir dil ayrıştırıcısı değildir.
func vurgula(satir string, kw map[string]bool) string {
	var out strings.Builder
	r := []rune(satir)
	n := len(r)
	i := 0
	for i < n {
		c := r[i]
		switch {
		case c == '#' || (c == '/' && i+1 < n && r[i+1] == '/'):
			out.WriteString(stComment.Render(string(r[i:])))
			i = n
		case c == '"' || c == '\'':
			j := i + 1
			for j < n && r[j] != c {
				if r[j] == '\\' && j+1 < n {
					j++
				}
				j++
			}
			if j < n {
				j++
			}
			out.WriteString(stString.Render(string(r[i:j])))
			i = j
		case unicode.IsDigit(c):
			j := i
			for j < n && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			out.WriteString(stNumber.Render(string(r[i:j])))
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < n && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_') {
				j++
			}
			kelime := string(r[i:j])
			if kw[kelime] {
				out.WriteString(stKeyword.Render(kelime))
			} else {
				out.WriteString(stNormal.Render(kelime))
			}
			i = j
		default:
			out.WriteString(stNormal.Render(string(c)))
			i++
		}
	}
	return out.String()
}
