package tui

// harici.go — Ayarlar › Eklentiler'deki "VS Code bağlantısı": eklenti
// API'sinin, VS Code'daki AND eklentisinin bağlandığı harici dinleyicisini
// (bkz. pluginapi.Server.HariciAyarla) açıp kapatır.

import (
	"errors"

	"github.com/lucian95511/and/dil"
	"github.com/lucian95511/and/pluginapi"
)

// HariciDenetim, VS Code bağlantısını açıp kapatır; pluginapi.Server bunu
// karşılar.
type HariciDenetim interface {
	HariciDurumu() pluginapi.HariciDurum
	HariciAyarla(acik bool) error
}

// HariciBaglanti, Ayarlar'daki "VS Code bağlantısı" satırının denetimidir;
// nil ise satır görünmez (ör. eklenti API'si başlatılamadı).
var HariciBaglanti HariciDenetim

// HariciHataMetni, bağlantı açılıp kapatılamadığında gösterilen etkin
// dildeki metindir.
func HariciHataMetni(err error) string {
	switch {
	case errors.Is(err, pluginapi.ErrHariciOrtamda):
		return dil.T("VS Code bağlantısı AND_ENABLE_EXTERNAL_API ortam değişkeniyle açık. Kapatmak için değişkeni kaldırıp AND'ı yeniden başlat.")
	case errors.Is(err, pluginapi.ErrHariciKullanilamaz):
		return dil.T("VS Code bağlantısı bu AND'da kullanılamıyor.")
	default:
		return dil.T("VS Code bağlantısı değiştirilemedi: ") + err.Error()
	}
}

// hariciDegistir, VS Code bağlantısını tersine çevirir ve sonucu Ayarlar'ın
// bildirimine yazar.
func (m *appModel) hariciDegistir() {
	if HariciBaglanti == nil {
		return
	}
	acik := !HariciBaglanti.HariciDurumu().Acik
	switch err := HariciBaglanti.HariciAyarla(acik); {
	case err != nil:
		m.settingsNotice = HariciHataMetni(err)
	case acik:
		m.settingsNotice = dil.T("VS Code bağlantısı açıldı")
	default:
		m.settingsNotice = dil.T("VS Code bağlantısı kapatıldı")
	}
}
