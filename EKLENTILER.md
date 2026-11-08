# AND eklentileri: kurma ve yazma

English: [PLUGINS.md](PLUGINS.md)

Bir AND eklentisi kendi terminal arayüzü olan ayrı bir programdır. AND onu Eklentiler menüsünden
başlatır, eklenti AND'la yalnızca bilgisayarın içindeki bir HTTP API'si üzerinden konuşur ve
kapanınca AND kaldığı yerden devam eder. Eklenti eklemek için AND'ı yeniden derlemek ya da çatallamak
gerekmez.

- [Eklenti kurmak](#eklenti-kurmak)
- [İlk eklentini yazmak](#i̇lk-eklentini-yazmak)
- [plugin.json](#pluginjson)
- [Çalışma ortamı](#çalışma-ortamı)
- [İzinler](#i̇zinler)
- [SDK paketleri](#sdk-paketleri)
- [API uç noktaları](#api-uç-noktaları)
- [Görünüm](#görünüm)
- [Dil desteği](#dil-desteği)
- [Test etmek](#test-etmek)
- [Paketleme ve dağıtım](#paketleme-ve-dağıtım)
- [Güvenlik kuralları](#güvenlik-kuralları)
- [Go dışındaki diller](#go-dışındaki-diller)
- [Lisans](#lisans)

## Eklenti kurmak

```sh
and eklenti kur ./hava                              # bir klasörden
and eklenti kur hava-1.0.0.zip                      # bir .zip ya da .tar.gz arşivinden
and eklenti kur https://ornek.org/hava-1.0.0.zip --sha256 <yazarın-yayımladığı-özet>
and eklenti liste                                   # AND'la gelenler ve dışarıdan kurulanlar
and eklenti dogrula hava-1.0.0.zip                  # kurmadan denetle
and eklenti kaldir hava
```

Komutların İngilizce adları da çalışır: `and plugin install|list|check|remove|new`.

Kurmadan önce AND paketi açar ve denetler, sonra sana eklentiyi, yazarını, programın platformunu,
SHA-256 özetlerini ve istediği izinleri gösterip onay ister (`-e`/`--evet` sormadan kurar):

- `plugin.json` kökte ya da arşivdeki tek bir klasörün içinde olmalı; eklentinin adı harf ya da rakamla
  başlar, yalnızca harf, rakam, `_` ve `-` içerir.
- Bu bilgisayar için bir program bulunmalı: `and-plugin-<ad>[.exe]` ya da çok platformlu paketlerde
  `and-plugin-<ad>_<işletim>_<mimari>[.exe]`. Programın hangi işletim sistemi ve mimari için derlendiği
  dosya başlığından okunur; **kurarken hiçbir şey çalıştırılmaz.**
- Arşivdeki yollar paketin dışına çıkamaz; bağlantılar, aygıt dosyaları ve 512 MB'ı aşan paketler
  reddedilir. https dışındaki adreslerden indirilmez.
- AND'la gelen bir eklentinin adını taşıyan paket kurulmaz (ör. sahte bir `konu_ac`).

Eklenti veri dizinindeki `eklentiler/<ad>/` klasörüne kurulur (Windows'ta `%APPDATA%\and\eklentiler`,
Linux'ta `~/.config/and/eklentiler`, macOS'ta `~/Library/Application Support/and/eklentiler`); AND
programının klasörüne yazma izni gerekmez. `eklentiler/kurulumlar.json`, programın ve `plugin.json`'ın
SHA-256'sını tutar. AND, kurulduktan sonra dosyaları değişen bir eklentiyi başlatmaz; kaydı olmayan bir
klasörü eklenti saymaz. AND açıkken kurduğun eklenti bir sonraki açılışta menüde görünür.

İlk açılışta eklentinin istediği izinler sorulur; dışarıdan kurulan eklentilerde onay ekranı, eklentinin
AND'la gelmediğini ve AND geliştiricilerinin onu incelemediğini de söyler. Aynı eklentiyi yeni bir
paketle yeniden kurmak onu günceller: verdiğin izinler korunur, yeni bir izin istenirse yeniden sorulur.
AND'ın otomatik güncellemesi dışarıdan kurulan eklentilere dokunmaz.

`and eklenti kaldir` klasörü, kaydı, açık/kapalı durumunu ve verilen izinleri siler. Eklentinin veri
dizinine kendi yazdığı dosyalar silinmez; AND hangileri olduğunu bilemez.

> Bir eklenti senin bilgisayarında, senin kullanıcı hesabınla çalışan bir programdır. İzinler yalnızca
> AND'ın API'sini sınırlar; dosyalarına erişimini sınırlamaz. Yalnızca güvendiğin kaynaklardan kur.

## İlk eklentini yazmak

Gerekenler: Go (AND'ın `go.mod`'undaki sürüm ya da daha yenisi) ve AND.

```sh
and eklenti yeni hava                # ./hava klasörüne çalışan bir eklenti projesi
cd hava
go mod tidy                          # AND'ın SDK'sını ve bağımlılıklarını indirir
go run . --deneme                    # AND'ı açmadan, sahte verili bir AND'la dene
go test ./...
go build -o and-plugin-hava.exe .    # Linux/macOS'ta: -o and-plugin-hava
and eklenti kur .
```

Seçenekler: `--modul github.com/sen/hava` (Go modül yolu; varsayılan `example.com/<ad>`),
`--yazar`, `--etiket` (menüde görünen ad) ve `--and <klasör>`. Sonuncusu, `go.mod`'a AND'ın yerel bir
kopyasını gösteren bir `replace` satırı yazar: henüz yayımlanmamış bir AND değişikliğine karşı geliştirmek
için.

Oluşan proje:

| Dosya | Ne işe yarar |
|---|---|
| `plugin.json` | Manifest; AND onu programı çalıştırmadan okur |
| `main.go` | `--manifest` (gömülü `plugin.json`'ı yazar), `--deneme`, dil, tema ve Bubble Tea programı |
| `model.go` | Ekran: kimliği, bağlı AND düğümlerini ve son sohbet mesajlarını tam ekran bir `ui.Page`'de gösterir |
| `model_test.go` | Sahte AND'la ekran ve manifest testleri |
| `ceviri.json`, `ceviri.go`, `ceviri_test.go` | Arayüz metinlerinin 5 dile çevirisi ve eksiksizlik testi |
| `README.md`, `README.tr.md` | Derleme, kurma ve dağıtma notları |

Eklenti ayrı bir Go modülüdür ve AND'ı `github.com/lucian95511/and` modül yolundan içe aktarır (depo
`github.com/AND-community/AND`'dadır; eski yol oraya yönlenir).

Eklentiyi değiştirdikten sonra yeniden derle ve `and eklenti kur .` komutunu yine çalıştır: AND, kurulu
kopyanın özetiyle tutmayan bir programı başlatmaz.

## plugin.json

```json
{
  "name": "hava",
  "label": "Hava Durumu",
  "version": "1.0.0",
  "description": "Forum için hava durumu",
  "author": "Ayşe Yılmaz",
  "permissions": ["forum"],
  "min_api": 2
}
```

| Alan | Zorunlu | Açıklama |
|---|---|---|
| `name` | Evet | Eklentinin adı; programın adı `and-plugin-<name>[.exe]` olur. Harf ya da rakamla başlar; harf, rakam, `_`, `-`; en fazla 48 karakter |
| `label` | Hayır | Menüde görünen ad. Boşsa eklenti menüde görünmez (yalnızca AND'ın kendisinin başlattığı `konu_ac` gibi) |
| `version` | Hayır | Sürüm; menüde ve `and eklenti liste`'de görünür |
| `description` | Hayır | Menüdeki kısa açıklama |
| `author` | Hayır | Yazar |
| `permissions` | Hayır | İstenen izinler ([İzinler](#i̇zinler)); yoksa yalnızca herkese açık okumalar |
| `min_api` | Hayır | Gereken en düşük eklenti API sürümü; AND daha eskiyse eklenti kurulmaz ve başlatılmaz |

Metin alanlarındaki kontrol karakterleri ve terminal kaçış dizileri AND tarafından silinir; tanınmayan
izinler yok sayılır.

## Çalışma ortamı

AND eklentiyi kendi klasöründe (`plugin.json`'ın yanında), aynı terminalde başlatır ve eklenti çıkana
kadar ekranı ona bırakır. Ortam değişkenleri:

| Değişken | Anlamı | SDK |
|---|---|---|
| `AND_API_ADDR` | AND'ın API adresi (`127.0.0.1:<port>`) | `pluginapi.NewClientFromEnv()` |
| `AND_API_TOKEN` | Bu açılışa özel API anahtarı; eklenti kapanınca geçersizleşir, hiçbir dosyaya yazılmaz | `pluginapi.NewClientFromEnv()` |
| `AND_DATA_DIR` | AND'ın veri dizini | `pluginapi.DataDir()` |
| `AND_DIL` | Arayüz dili (`tr`, `en`, `de`, `es`, `fr`, `ru`) | `dil.OrtamdanAyarla()` |
| `AND_CATEGORY` | Yalnızca `konu_ac`: forumda seçili kategori | `pluginapi.Category()` |

Veri dizinine yazacağın dosyaların adına eklentinin adını önek yap (`hava_ayarlar.json`); kimlik ya da
anahtar saklama.

## İzinler

Eklentiye verilen anahtar yalnızca `plugin.json`'da istenen **ve** kullanıcının onayladığı izinleri taşır.
Kimlik, roller, eşler, forum konuları, kişisel siteler, genel sohbet ve tema her eklenti tarafından izinsiz
okunabilir. Verilmemiş bir izni gerektiren istek `403` döner (`pluginapi.IsForbidden(err)`).

| İzin | Açtıkları |
|---|---|
| `dm` | Özel mesaj göndermek, almak, geçmişi ve okundu bilgisi (`/api/v1/dm/*`) |
| `file` | Dosya göndermek, gelen dosya isteklerini görmek ve yanıtlamak (`/api/v1/file/*`) |
| `chat` | Genel sohbete yazmak (`POST /api/v1/chat/send`) |
| `forum` | Konu açmak ve yanıt yazmak (`POST /api/v1/forum/post`, `/forum/reply`) |
| `site` | Kişisel sitede sayfa yayımlamak ve silmek (`/api/v1/site/publish`, `/site/delete`) |
| `moderation` | Kurucu ya da moderatörün yetkisiyle konu silmek, ban kaldırmak, moderatör atamak (`/api/v1/forum/delete`, `/api/v1/mod/*`) |
| `theme` | AND'ın temasını değiştirmek (`POST /api/v1/theme`) |

Yalnızca gerçekten kullandığın izinleri iste; kullanıcı her birini onay ekranında görür. Bir
güncellemede yeni bir izin istersen AND kullanıcıya yeniden sorar.

`pluginapi.APIVersion` eklenti API'sinin sürümüdür (şu an 2). API yalnızca geriye dönük uyumlu biçimde
büyür; yeni bir uç noktaya dayanıyorsan `min_api` yaz. Bağlı AND'ın sürümü `client.APIVersion()` ile
okunur; eski bir AND'da olmayan bir uç nokta `404` döner (`pluginapi.IsNotFound(err)`).

## SDK paketleri

AND deposunun dışından içe aktarılabilen paketler:

| Paket | Ne için |
|---|---|
| `github.com/lucian95511/and/pluginapi` | İstemci (`Client`), JSON tipleri, `Manifest`, izin sabitleri (`IzinForum`…), `APIVersion` |
| `github.com/lucian95511/and/pluginapi/pluginapitest` | Testlerde ve `--deneme`'de kullanılan sahte AND |
| `github.com/lucian95511/and/ui` | AND'ın görsel dili: tema, tam ekran sayfa, başlık, liste satırı, tuş ipuçları |
| `github.com/lucian95511/and/pluginui` | `Safe`: ağdan gelen metni ekrana basmadan önce temizler |
| `github.com/lucian95511/and/dil`, `dil/diltest` | Arayüzü AND'ın dilinde açmak ve çevirilerin eksiksizliğini test etmek |
| `github.com/lucian95511/and/protokol` | Ağ sabitleri; ör. forum kategorileri `protokol.Kategoriler` |

`internal/` altındaki paketler AND'ın iç ayrıntısıdır ve dışarıdan içe aktarılamaz.

## API uç noktaları

Tüm istekler `X-And-Token: <AND_API_TOKEN>` başlığını taşır; hatalar `{"error": "açıklama"}` döner
(geçersiz istek 400, izin yok 403, bulunamadı 404, hız sınırı 429). Go'da her uç noktanın bir `Client`
yöntemi vardır.

| Yöntem ve yol | İzin | `Client` |
|---|---|---|
| `GET /api/v1/version` | — | `APIVersion` |
| `GET /api/v1/identity`, `/role`, `/peers` | — | `Identity`, `Role`, `Peers` (`AND: true` olan eşler AND düğümüdür) |
| `GET /api/v1/forum/post?id=` | — | `GetPost` |
| `GET /api/v1/forum/all-posts` | — (yalnızca kurucu ya da moderatör) | `AllPosts` |
| `POST /api/v1/forum/post`, `/forum/reply` | `forum` | `CreatePost(kategori, başlık, gövde)`, `CreateReply` |
| `POST /api/v1/forum/delete` | `moderation` | `DeletePost` |
| `GET /api/v1/site/pages` | — | `SitePages` (kullanıcının kendi sayfaları) |
| `POST /api/v1/site/publish`, `/site/delete` | `site` | `PublishSitePage`, `DeleteSitePage` |
| `GET /api/v1/chat/all`, `/chat/poll` | — | `AllChat`, `PollChat` |
| `POST /api/v1/chat/send` | `chat` | `SendChat` |
| `POST /api/v1/dm/send` | `dm` | `SendDM`, `SendDMQueued` (alıcı çevrimdışıysa giden kutusuna) |
| `GET /api/v1/dm/history?from=`, `/dm/poll` | `dm` | `DMHistory`, `DMHistoryFrom`, `PollDM` |
| `GET` `POST /api/v1/dm/read` | `dm` | `DMReadMarks`, `MarkDMRead` |
| `POST /api/v1/file/send`, `/file/consent`; `GET /file/poll`, `/file/consent-poll` | `file` | `SendFile`, `RespondConsent`, `PollFile`, `PollConsent` |
| `GET /api/v1/mod/bans`, `/mod/history`, `/mod/grants`; `POST /mod/unban`, `/mod/grant`, `/mod/revoke` | `moderation` | `AllBans`, `ModHistory`, `GrantedMods`, `Unban`, `GrantModerator`, `RevokeModerator` |
| `GET /api/v1/theme` | — | `GetTheme` |
| `POST /api/v1/theme` | `theme` | `SetTheme` |

`poll` uçları 5 saniyeye kadar bekleyen uzun yoklamalardır ve yalnızca istek açıkken geleni verir;
kaçırmamak için `dm/history?from=<son kimlik>` kullan. İçerik yayımlayan uçlar yerel olarak hız
sınırlıdır (`pluginapi/ratelimit.go`); insan temposunda hiç görülmez.

## Görünüm

Renkleri ve kutuları kendin yazma; `ui` paketini kullan. Paket AND'ın etkin temasıyla boyanır ve bütün
yerleşik eklentiler onu kullanır.

```go
if tema, err := istemci.GetTheme(); err == nil {
    ui.Apply(ui.Theme(tema))
}

func (m model) View() string {
    w := ui.ContentWidth(m.width)
    govde := ui.Header(w, []string{"Hava", "İstanbul"}, ui.S.Muted.Render("güncel")) + "\n\n"
    for i, s := range m.satirlar {
        govde += ui.Row(w, i == m.secili, ui.T(s.ad), ui.M("  "+s.aciklama)) + "\n"
    }
    alt := ui.Divider(w) + "\n" + ui.KeyHints(w, ui.H("↑↓", "seç"), ui.H("q", "çıkış"))
    return ui.Page(m.width, m.height, govde, alt)
}
```

| Bileşen | Ne için |
|---|---|
| `ui.Page`, `ui.ContentWidth`, `ui.ContentHeight` | Terminali kaplayan çerçeve; gövde üstte, alt bilgi en altta |
| `ui.Header`, `ui.Section`, `ui.Divider` | Konum izli başlık, bölüm başlığı, ayraç |
| `ui.Row`, `ui.T`, `ui.M`, `ui.C` | Seçilebilir liste satırı; taşan metin `…` ile kırpılır |
| `ui.KeyHints`, `ui.H` | Alttaki tuş ipuçları; sığmayan ipucu alt satıra geçer |
| `ui.Notice` | Tek satır bildirim (`KindSuccess`, `KindError`, `KindWarning`, `KindPending`) |
| `ui.Tabs`, `ui.KV`, `ui.Bar`, `ui.Spread`, `ui.Block`, `ui.VLine` | Sekmeler, hizalı etiket–değer, doluluk çubuğu, yan yana paneller |
| `ui.Measure`, `ui.Wrap`, `ui.Truncate` | Okuma genişliği (en çok 100 sütun), sarma, kırpma |
| `ui.S.*`, `ui.P.*` | Hazır stiller ve palet renkleri |

## Dil desteği

`main`'de `dil.OrtamdanAyarla()` çağır, metinleri `dil.T("…")` ve `dil.F("%d konu", n)` ile yaz ve
çevirilerini `ceviri.json`'a koy. Kaynak dil Türkçedir; her metnin `en`, `de`, `es`, `fr`, `ru`
karşılığı olmalı. Çoğul biçimler `|` ile ayrılır (Rusçada üç biçim); bu yüzden metnin kendisinde `|`
kullanma. `diltest.Kapsam` eksik, fazla ya da biçim fiilleri tutmayan çevirileri yakalar;
`DIL_ISKELET=eksik.json go test` eksikleri bir dosyaya döker.

## Test etmek

`pluginapitest` AND'ın gerçek API sunucusunu bellekteki verilerle çalıştırır. Uç noktalar, girdi
doğrulaması, hız sınırları ve izin denetimi gerçek AND'daki gibidir.

```go
func TestKonuAcar(t *testing.T) {
    and := pluginapitest.Yeni().Ornek()                 // örnek eşler, konular, mesajlar
    istemci := and.Test(t, manifestOku().Permissions...) // yalnızca manifest'teki izinler
    if err := konuAc(istemci, "Genel", "Merhaba"); err != nil {
        t.Fatal(err)
    }
    if len(and.Konular()) != 3 {
        t.Fatal("konu açılmadı")
    }
}
```

- `and.Test(t, izinler...)` ortam değişkenlerini de ayarlar; `pluginapi.NewClientFromEnv()` kullanan
  kod da sahte AND'a bağlanır.
- Hazırlık: `KimlikAyarla`, `EsEkle`, `KonuEkle`, `YanitEkle`, `SohbetEkle`, `DMGelsin`,
  `DosyaGelsin`.
- Sonuç: `Konular`, `SohbetGecmisi`, `GidenDMler`, `SiteSayfalari`, `GidenDosyalar`, `OnayYaniti`,
  `TemaIstekleri`.
- Bilinmeyen bir forum kategorisi gerçek ağdaki gibi reddedilir.
- `Baslat(dizin, izinler...)` testler dışında da çalışır; iskeletin `--deneme` bayrağı onu kullanır.

## Paketleme ve dağıtım

Tek bir `.zip` ya da `.tar.gz` yayımla: `plugin.json` ve program kökte ya da tek bir klasörün içinde.
Programlar AND'ın kendi sürüm dosyaları gibi adlandırılırsa tek arşiv her platforma yeter; kurulum
kullanıcının bilgisayarına uyanı seçer, ötekileri kurmaz.

```
hava-1.0.0/
  plugin.json
  README.md
  LICENSE
  and-plugin-hava_windows_amd64.exe
  and-plugin-hava_linux_amd64
  and-plugin-hava_darwin_arm64
```

```sh
GOOS=windows GOARCH=amd64 go build -o dist/and-plugin-hava_windows_amd64.exe .
GOOS=linux   GOARCH=amd64 go build -o dist/and-plugin-hava_linux_amd64 .
GOOS=darwin  GOARCH=arm64 go build -o dist/and-plugin-hava_darwin_arm64 .
```

Arşivin SHA-256'sını da yayımla (`sha256sum hava-1.0.0.zip`); kullanıcılar
`and eklenti kur <adres> --sha256 <özet>` ile indirdiklerinin seninkiyle aynı olduğunu doğrular. GitHub
sürüm dosyalarının adresleri doğrudan kullanılabilir.

## Güvenlik kuralları

- Ağdan gelen her metni (adlar, başlıklar, mesajlar, dosya adları) ekrana basmadan önce
  `pluginui.Safe`'ten geçir: terminal kaçış dizileri panoya yazabilir, ekranı bozabilir.
- Bir mesajın kimden geldiğine `DMMsg.VerifiedPeerID` ya da `DMRecord.PeerID` ile karar ver; mesajın
  içinde ya da `From`'da yazan ad doğrulanmamıştır.
- Durum dosyalarını önce geçici bir dosyaya yazıp yeniden adlandırarak kaydet; yarıda kalan bir yazma
  eski dosyayı bozmasın.
- `AND_API_TOKEN`'ı günlüğe, dosyaya ya da başka bir sürece verme.

## Go dışındaki diller

Protokol HTTP ve JSON'dur; eklenti herhangi bir dilde yazılabilir. İki koşul var: kurulumda kabul edilen
program **yerel bir çalıştırılabilir dosyadır** (Windows'ta PE, Linux'ta ELF, macOS'ta Mach-O; betik
dosyaları değil) ve istekler `AND_API_ADDR`'a `X-And-Token` başlığıyla gider. JSON alanları
`pluginapi/api.go`'daki tiplerle aynıdır; [and-vscode](and-vscode/README.md) aynı API'yi TypeScript'ten
kullanır.

## Lisans

AND ve eklentilerin içe aktardığı SDK paketleri GNU AGPL-3.0 lisanslıdır. Bu paketleri programına
bağlayan bir eklenti genel kabule göre AND'a dayanan bir çalışmadır: dağıtırken kaynak koduyla birlikte
AGPL-3.0 (ya da onunla uyumlu bir lisans) altında yayımla. AND'ın paketlerini içe aktarmayan, API'yi
yalnızca HTTP üzerinden kullanan bir program ayrı bir çalışma olarak değerlendirilebilir; emin
değilsen bir hukukçuya danış.

## AND deposundaki eklentiler

Bu depodaki eklentiler (`Eklentiler/`) AND'la birlikte gelir ve AND'ın sürümleriyle güncellenir.
Kaynaktan derlerken her biri kendi klasörüne, `plugin.json`'ın yanına derlenir (bkz.
[README.tr.md](README.tr.md#kaynaktan-derleme)). [Eklentiler/ornek](Eklentiler/ornek/) AND'ın iç
paketlerini de kullanan daha ayrıntılı bir örnektir; yeni bir eklentiye `and eklenti yeni` ile başla.
