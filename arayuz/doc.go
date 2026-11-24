// Package arayuz, eklentilerin AND'ın masaüstü arayüzünde terminal yerine
// AND'ın kendi bileşenleriyle görünmesini sağlar. Eklenti ekranını bir
// bileşen ağacı (Ekran, Oge) olarak tarif eder; AND onu kendi tasarım
// sistemiyle çizer ve kullanıcının tıklamalarını, form gönderimlerini olay
// (Olay) olarak eklentiye geri yollar. Eklenti renk, yazı tipi ya da HTML
// seçemez; yalnızca anlamsal varyant seçer (birincil, tehlikeli, soluk…).
//
// # Protokol
//
// Eklenti manifest'ine "ui": 1 yazar. AND masaüstünde böyle bir eklentiyi
// standart girdi/çıktısı borularla AND'a bağlı olarak ve AND_ARAYUZ_PROTOKOLU
// ortam değişkeniyle başlatır. Taşıma UTF-8 JSON Lines'tır: eklenti her satıra
// bir ekranın tamamını ({"tur":"ekran","ekran":{…}}) ya da bir bildirimi
// yazar; AND her satıra bir olay ({"tur":"olay",…}), ilk satırda merhaba ve
// eklentinin ekranı gösterilince/gizlenince görünürlük yazar. Bilinmeyen
// alanlar iki yönde de yok sayılır; sürüm (Surum) yalnızca eklenerek büyür.
// Terminal arayüzü (and --terminal) eklentiyi her zaman terminal kipinde açar.
//
// # Kullanım
//
// Eklentinin bubbletea modeli View'in yanında Arayuz'ü de tanımlar; aynı
// model, aynı Update ve aynı komutlar iki kipte de çalışır:
//
//	func (m model) Arayuz() arayuz.Ekran {
//		return arayuz.Ekran{
//			Baslik: "Sayaç",
//			Govde: new(arayuz.Yigin(
//				arayuz.Metin(strconv.Itoa(m.n)),
//				arayuz.Dugme("arti", "Arttır", arayuz.Birincil),
//			)),
//		}
//	}
//
//	// Update'te: case arayuz.Olay: if msg.ID == "arti" { m.n++ }
//
//	if arayuz.Etkin() {
//		err = arayuz.Calistir(m)
//	} else {
//		_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
//	}
//
// Testler için bkz. arayuz/arayuztest.
package arayuz
