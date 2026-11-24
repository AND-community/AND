// Package pluginapi, AND eklentilerinin AND ile konuştuğu yerel HTTP
// API'sidir: eklentinin kullandığı istemci (Client), AND'ın çalıştırdığı
// sunucu (Server), JSON tipleri, manifest (plugin.json) ve izinler.
//
// Bu paket AND deposunun dışından içe aktarılabilir; eklenti yazmak için AND'ı
// çatallamak gerekmez. Bir eklenti ayrı bir Go modülüdür ve şunları kullanır:
//
//	github.com/lucian95511/and/pluginapi                 AND'a istekler, manifest, izinler
//	github.com/lucian95511/and/pluginapi/pluginapitest   testler ve "go run . --deneme" için sahte AND
//	github.com/lucian95511/and/ui                        AND'ın görsel dili: tema, tam ekran sayfa, bileşenler
//	github.com/lucian95511/and/pluginui                  ağdan gelen metni ekrana basmadan önce temizleme
//	github.com/lucian95511/and/dil                       arayüzü AND'ın dilinde (6 dil) açma
//	github.com/lucian95511/and/protokol                  forum kategorileri gibi ağ sabitleri
//	github.com/lucian95511/and/arayuz                    masaüstünde terminal yerine AND'ın bileşenleriyle görünme
//	github.com/lucian95511/and/arayuz/arayuztest         arayüz kipinin testleri
//
// Yeni bir eklenti iskeleti "and eklenti yeni <ad>" ile kurulur; derlenen
// eklenti "and eklenti kur <klasör|arşiv>" ile AND'a eklenir. Ayrıntılar
// deponun PLUGINS.md (İngilizce) ve EKLENTILER.md (Türkçe) belgelerindedir.
//
// # Uyumluluk
//
// AND, eklentiyi kendi dizininde, şu ortam değişkenleriyle başlatır:
// AND_API_ADDR ve AND_API_TOKEN (NewClientFromEnv okur), AND_DATA_DIR
// (DataDir) ve AND_DIL (dil.OrtamdanAyarla). Eklentiye verilen anahtar
// yalnızca manifest'te istenen ve kullanıcının onayladığı izinleri taşır;
// verilmeyen bir izni gerektiren istek 403 döner (IsForbidden).
//
// APIVersion yalnızca geriye dönük uyumlu biçimde büyür: uç noktalar ve
// JSON alanları eklenir, mevcutlar kaldırılmaz. Yeni bir uç noktaya dayanan
// eklenti manifest'ine min_api yazar; daha eski bir AND onu başlatmaz ve
// kullanıcıya AND'ı güncellemesini söyler. Eski bir AND'da olmayan bir uç
// nokta 404 döner (IsNotFound); eklenti buna göre geri çekilebilir.
package pluginapi
