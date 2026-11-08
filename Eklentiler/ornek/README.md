# Örnek Eklenti (`ornek`)

AND'ın eklenti API'sinin çoğunu tek bir programda gösteren geliştirici örneği. AND'la birlikte gelir ve
Eklentiler menüsünde "Örnek Eklenti" adıyla görünür.

> **Yeni bir eklenti mi yazacaksın?** Bu klasörü kopyalama: `and eklenti yeni <ad>` AND deposunun
> dışında, kendi Go modülünde derlenen, testleri ve sahte AND'ı hazır bir proje kurar. Kurma, izinler,
> SDK, API, görünüm, çeviri, test ve dağıtım: [EKLENTILER.md](../../EKLENTILER.md)
> (English: [PLUGINS.md](../../PLUGINS.md)).

## Ne gösterir

| Özellik | Nerede |
|---|---|
| Manifest: `plugin.json` ve `--manifest` | `plugin.json`, `main.go` |
| AND'a bağlanma: `pluginapi.NewClientFromEnv()`, tema (`GetTheme` + `ui.Apply`), dil (`dil.OrtamdanAyarla`) | `main.go` |
| Kimlik ve rol (`Identity`, `Role`) | Kimlik ekranı |
| Tüm konuları listeleme, ayrıntı ve silme (kurucu ya da moderatör, `moderation` izni) | Konular ekranı |
| Çok adımlı konu açma formu: kategori (`protokol.Kategoriler`), başlık, gövde (`forum` izni) | Konu oluştur ekranı |
| Özel mesaj gönderme (`dm` izni) | Özel mesaj ekranı |
| `AND_DATA_DIR`'de ayar dosyası (atomik yazma) | Ayarlar ekranı |
| Ağdan gelen metni `pluginui.Safe` ile temizleme, istek sürerken ikinci eylemi engelleme | `main.go` |
| AND'ın görsel dili (`ui.Page`, `ui.Header`, `ui.Row`, `ui.KeyHints`) ve 6 dilde arayüz | `main.go`, `ceviri.json` |

İstediği izinler: `forum`, `moderation`, `dm`; AND ilk açılışta sorar.

## Derleme

Bu eklenti AND'ın modülünün içindedir ve AND'ın iç paketlerinden birini (`internal/dosya`) kullanır;
bu yüzden yalnızca AND deposunun içinden derlenir. AND, programı `plugin.json`'ın yanında arar:

```bash
go build -o Eklentiler/ornek/and-plugin-ornek ./Eklentiler/ornek       # Linux / macOS
go build -o Eklentiler/ornek/and-plugin-ornek.exe ./Eklentiler/ornek   # Windows
go test ./Eklentiler/ornek
```

AND'ın dışında yazılan bir eklenti `internal/` paketlerini kullanamaz; ihtiyaç duyduğu her şey herkese
açık paketlerdedir (`pluginapi`, `pluginapi/pluginapitest`, `ui`, `pluginui`, `dil`, `protokol`).
