// Package lisans, AND'ın lisans bildirimlerini taşır: AND'ın kendi lisansı
// (GNU AGPL-3.0) ve AND'ın ikili dosyalarına giren üçüncü taraf bileşenlerin
// lisansları. Metinler derlemeye gömülür; "and --lisans", "and --lisanslar"
// ve arayüzdeki Hakkında ekranı buradan okur.
//
// go.mod değişince bildirimler yeniden üretilir:
//
//	go generate ./internal/lisans
//
// Testler, bildirimin go.mod ile aynı modülleri ve sürümleri taşıdığını ve her
// bağımlılığın lisansının AGPL-3.0 ile birlikte dağıtılabilir olduğunu
// denetler; CI üretilen dosyaların güncel olduğunu ayrıca doğrular.
package lisans

//go:generate go run ./uret

import (
	_ "embed"
	"slices"
)

// KaynakKodu, AND'ın kaynak kodunun yayımlandığı adrestir. AGPL-3.0'ın 13.
// maddesi, programla ağ üzerinden etkileşen kullanıcılara kaynak kodunun
// sunulmasını ister; Hakkında ekranı ve --version bu adresi gösterir.
const KaynakKodu = "https://github.com/AND-community/AND"

// Ad, AND'ın lisansının adıdır.
const Ad = "GNU AGPL-3.0"

// Modul, AND'ın ikili dosyalarına giren üçüncü taraf bir bileşendir.
type Modul struct {
	Yol    string // modül yolu; Go standart kütüphanesi için "go"
	Surum  string
	Lisans string // SPDX kimlikleri, virgülle ayrılmış (ör. "Apache-2.0, MIT")
}

//go:embed AGPL-3.0.txt
var agpl string

//go:embed ucuncu_taraf.txt
var ucuncuTaraf string

// Metin, AND'ın lisansının (GNU AGPL-3.0) resmî İngilizce tam metnidir.
func Metin() string { return agpl }

// UcuncuTaraf, üçüncü taraf bileşenlerin özetini ve lisans metinlerini içeren
// bildirimdir (İngilizce).
func UcuncuTaraf() string { return ucuncuTaraf }

// Moduller, üçüncü taraf bileşenlerdir; ilk öğe Go standart kütüphanesidir.
func Moduller() []Modul { return slices.Clone(moduller) }
