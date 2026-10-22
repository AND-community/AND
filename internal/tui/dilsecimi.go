package tui

// dilsecimi.go — arayüz dilinin seçimi: Ayarlar'daki Dil sekmesi ve giriş
// ekranındaki dil düğmesi. Metinler çizim anında dil.T ile çevrildiği için
// seçim hemen her ekrana yansır; yalnızca model kurulurken atanan yer
// tutucular yeniden kurulur. Seçim AND veri dizinine kaydedilir ve AND,
// başlattığı eklentilere AND_DIL ile iletir.

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/lucian95511/and/dil"
)

// settingsTabSayisi, Ayarlar'daki sekme sayısıdır (Bilgi, Eklentiler, Temalar, Dil).
const settingsTabSayisi = 4

// dilSirasi, kodun dil.Diller içindeki sırasıdır (bulunamazsa 0).
func dilSirasi(kod string) int {
	for i, d := range dil.Diller {
		if d.Kod == kod {
			return i
		}
	}
	return 0
}

// diliUygula, arayüz dilini değiştirir, seçimi kaydeder ve yer tutucuları
// yeni dilde yeniden kurar.
func (m *appModel) diliUygula(kod string) {
	if !dil.Ayarla(kod) {
		return
	}
	m.yerTutuculariniKur()
	if err := dil.Kaydet(m.dataDir, kod); err != nil {
		m.settingsNotice = dil.T("Dil seçimi kaydedilemedi: ") + err.Error()
		return
	}
	m.settingsNotice = dil.F("Arayüz dili: %s", dil.EtkinAd())
}

// yerTutuculariniKur, ana modeldeki giriş kutularının yer tutucularını
// etkin dilde kurar.
func (m *appModel) yerTutuculariniKur() {
	m.chatInput.Placeholder = dil.T("mesaj…")
	m.bioInput.Placeholder = dil.T("kendinden kısaca bahset…")
	m.statusInput.Placeholder = dil.T("şu an ne yapıyorsun?…")
	m.linkInput.Placeholder = dil.T("github.com/kullanici-adi")
	m.banReasonInput.Placeholder = dil.T("sebep…")
}

// etiketGenisligi, ui.KV satırlarındaki etiket sütununun genişliğidir: en az
// enAz, etkin dilde en uzun etiketten iki fazla. Türkçe etiketler enAz'a
// göre seçildiği için Türkçe düzen değişmez; daha uzun çeviriler (ör.
// "Código de recuperación") alt satıra kaymaz.
func etiketGenisligi(enAz int, etiketler ...string) int {
	w := enAz
	for _, e := range etiketler {
		w = max(w, lipgloss.Width(dil.T(e))+2)
	}
	return w
}
