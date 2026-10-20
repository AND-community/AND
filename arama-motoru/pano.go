package main

// pano.go — sistem panosuna (clipboard) kopyalama. links.go'nun OSC 8
// (tıklanabilir bağlantı) deseniyle AYNI ailede bir terminal kontrol
// dizisi kullanır: OSC 52. Yeni bir bağımlılık GEREKTİRMEZ (yalnızca
// standart kütüphanenin encoding/base64'ü) ve desteklemeyen terminallerde
// (OSC 8'de olduğu gibi) zararsızca yok sayılır — hiçbir hata/çökme
// olmaz, yalnızca kopyalama sessizce gerçekleşmez.

import "encoding/base64"

// panoyaKopyalaDizisi, metni OSC 52 terminal kontrol dizisiyle sistem
// panosuna yazar — destekleyen terminallerde (Windows Terminal, iTerm2,
// kitty, WezTerm, çoğu modern terminal emülatörü; SSH üzerinden bile
// çalışır, uzak bir makinedeyken bile YEREL panoya kopyalar) metin
// GERÇEKTEN panoya kopyalanır.
//
// Bu dizi, links.go'daki tiklanabilirBaglanti'nın OSC 8 dizisiyle AYNI
// şekilde bir View() çıktısına GÖMÜLEREK gönderilir — bubbletea'nın
// renderer'ı satır genişliğini charmbracelet/x/ansi'nin StringWidth'iyle
// ölçer, bu da OSC ailesindeki kontrol dizilerini (8 dahil, bu kod
// tabanında zaten kanıtlanmış) doğru şekilde atlar; yani hiçbir görünür
// genişlik tüketmez, satır hizalamasını bozmaz. Yalnızca BİR render
// karesinde gömülüp hemen sonra temizlenir (bkz. model.go'nun
// panoTemizleMsg'i) — aksi halde her karede aynı diziyi tekrar tekrar
// göndermek gereksiz olurdu.
func panoyaKopyalaDizisi(metin string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(metin)) + "\x07"
}
