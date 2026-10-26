package main

// komut.go — arayüz açılmadan yanıtlanan komut satırı bayrakları: sürüm,
// lisans ve üçüncü taraf bildirimleri. GNU AGPL-3.0, programın telif
// bildirimini, garanti vermediğini ve lisansın nasıl görüleceğini kolayca
// erişilebilir kılmayı ister; arayüzde aynı bilgiler "AND hakkında" ekranındadır.

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/internal/lisans"
	"github.com/lucian95511/and/internal/updater"
)

// komutSatiri, bilinen bir bayrağı işleyip true döner. Bayrak yoksa ya da
// tanınmıyorsa false döner ve AND tam ekran arayüzle açılır; bilinmeyen
// argümanlar eskisi gibi yok sayılır (güncelleyici yeniden başlarken
// argümanları aynen geçirir).
func komutSatiri(args []string, out io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	var metin string
	switch args[0] {
	case "-v", "--version":
		metin = surumMetni()
	case "--lisans", "--license":
		metin = lisans.Metin()
	case "--ucuncu-taraf", "--third-party":
		metin = lisans.UcuncuTaraf()
	case "-h", "--help":
		metin = yardimMetni()
	default:
		return false, nil
	}
	_, err := io.WriteString(out, metin)
	return true, err
}

// surumMetni, --version çıktısıdır: sürüm, derleme bilgisi ve yasal bildirim.
func surumMetni() string {
	return strings.Join([]string{
		fmt.Sprintf("AND %s (%s, %s/%s)", updater.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH),
		dil.T("Telif hakkı (C) 2026 AND katkıcıları"),
		dil.F("Lisans: %s (tam metin: and --lisans)", lisans.Ad),
		dil.F("Kaynak kodu: %s", lisans.KaynakKodu),
		"",
		dil.T("Bu program özgür yazılımdır: GNU AGPL-3.0 koşullarıyla değiştirip yeniden dağıtabilirsin."),
		dil.T("Yasaların izin verdiği ölçüde HİÇBİR GARANTİSİ YOKTUR."),
		dil.T("Üçüncü taraf bileşenlerin lisansları: and --ucuncu-taraf"),
	}, "\n") + "\n"
}

func yardimMetni() string {
	satir := func(bayrak, aciklama string) string { return fmt.Sprintf("  %-34s %s", bayrak, aciklama) }
	return strings.Join([]string{
		dil.T("Kullanım: and [bayrak]"),
		"",
		dil.T("Bayrak verilmezse AND tam ekran arayüzle açılır."),
		"",
		satir("-v, --version", dil.T("sürüm, telif ve lisans bilgisini yazar")),
		satir("    --lisans, --license", dil.T("AND'ın lisansının (GNU AGPL-3.0) tam metnini yazar")),
		satir("    --ucuncu-taraf, --third-party", dil.T("üçüncü taraf bileşenlerin lisans bildirimlerini yazar")),
		satir("-h, --help", dil.T("bu yardımı yazar")),
	}, "\n") + "\n"
}
