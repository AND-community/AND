# Eklenti arayüzü: eklentiler terminal yerine AND'ın arayüzünde

Tarih: 2026-11-24 · Durum: taslak, onay bekliyor

## Amaç ve neden

Masaüstü arayüzünde eklentiler bugün AND'ın penceresinin içinde ama **terminal ekranı olarak** açılıyor
(ConPTY + xterm.js). Kullanıcı bunu istemiyor: "eklentiler terminalde çalışmasın, arayüzde çalışsın;
onlar için uyumlu arayüz tasarla". Eklenti açıldığında AND'ın forum, sohbet ve ayarlar ekranlarıyla
aynı tasarımda (aynı listeler, düğmeler, formlar, sekmeler, renkler, yazı tipleri) bir ekran görmeli.

Kullanıcının seçtiği yol: **eklenti ekranını anlatır, AND çizer.** Eklenti ekranını bir bileşen
ağacı olarak (JSON) tarif eder; AND onu kendi tasarım sistemiyle çizer; kullanıcının tıklamaları ve
form gönderimleri eklentiye olay olarak geri gider. İlk taşınacak eklenti, kullanıcının seçimiyle
**DeltaX**.

## Hedefler

- Eklentiler masaüstünde AND'ın kendi bileşenleriyle görünür; terminal görünümü yoktur.
- Görünüm her eklentide aynıdır ve AND'ın temasını (Klasik ve diğer temalar, özel renk) izler; eklenti
  renk, yazı tipi ya da CSS seçemez, yalnızca anlamsal varyant seçer (birincil, tehlikeli, soluk…).
- Altyapı genel bir SDK'dır: AND'la gelen eklentiler de, dışarıdan kurulanlar da kullanabilir.
- Eklentinin mantığı eklentide kalır (DeltaX'in sürüm kontrolü, şifrelemesi AND'a taşınmaz).
- Bir eklentinin terminal (`and --terminal`) sürümü ile masaüstü sürümü **aynı durum ve iş mantığını**
  paylaşır; yalnızca çizim ve girdi katmanı farklıdır.
- Linux/macOS'ta da çalışır (borular her yerde var; ConPTY gerekmez).

## Hedef olmayanlar

- Eklentilerin terminal arayüzlerini kaldırmak: `and --terminal` eklentileri eskisi gibi çalıştırır.
- Eklentiden HTML, CSS, JavaScript ya da tuval (canvas) çizimi almak.
- v1'de yerel dosya seçme penceresi (yol, metin alanına yazılır; sonra eklenebilir).
- konu_ac ve Pembiş: konu_ac'ın işi masaüstünde zaten yerleşik ("Yeni konu" formu), Pembiş bir tema.

## Kaçınılacaklar (çalışsa bile başarısızlık sayılır)

- AND'a yabancı görünen eklenti ekranları (kendi stil dili olan, terminal taklidi yapan).
- Eklentinin AND penceresinde kod çalıştırabilmesi ya da metinle HTML enjekte edebilmesi.
- Bugünkü eklentileri bozmak: arayüz desteği olmayan (dışarıdan kurulmuş) eklentiler gömülü terminalde
  çalışmaya devam eder.
- Eski eklentileri ya da eski AND'ı kıran protokol değişiklikleri: protokol sürümlüdür ve yalnızca
  eklenerek büyür.
- Kullanıcının yazdığı metnin (form alanı) eklentiden gelen bir yenilemeyle silinmesi ya da imlecin
  zıplaması.

## Kısıtlar

- Herkese açık SDK'ya (`pluginapi`, `ui`, `pluginui`, `dil`, `protokol`) yalnızca ekleme yapılır.
  Yeni paket: `github.com/lucian95511/and/arayuz` (+ `arayuz/arayuztest`).
- AND'ın arayüzündeki her yeni metin `t()`/`tf()` ile, `dil/ceviriler/tui.json`'da 5 dile çevrili;
  `metinler.go` yeniden üretilir. Eklentinin metinleri eklentinin kendi `ceviri.json`'ındadır.
- İçerik güvenlik politikası (CSP) gevşetilmez; çizim yalnızca `textContent` ile yapılır.
- Eklentiler cgo kullanmaz; Windows'ta eklenti konsolsuz başlatılır (`CREATE_NO_WINDOW`): AND'ın
  kendisi artık konsolsuz açılıyor (bkz. b06ffb0) ve hiçbir pencere yanıp sönmemeli.
- Mevcut davranışlar korunur: izin onayı, uyumluluk (`min_api`), bütünlük (`Dogrula`), açılışta
  kendiliğinden başlatma ve yeşil nokta (bkz. 602f446), Ayarlar'dan kapatınca durma.

## Yaklaşımlar

| Yol | Artısı | Eksisi |
|---|---|---|
| **1. Eklenti anlatır, AND çizer (seçildi)** | Uyum her zaman garanti; DeltaX'in mantığı yerinde kalır; dışarıdan eklentiler de kullanır; pencereye yabancı kod girmez; her platformda çalışır | Önce altyapı (protokol, SDK, çizici) kurulmalı; her eklentinin çizim katmanı yeniden yazılır; ifade gücü bileşen kataloğuyla sınırlı |
| 2. AND'la gelenleri masaüstüne elle çizmek | Özel Chat, Site, Yönetici için en hızlı sonuç | DeltaX'in mantığı AND'a taşınmalı; dışarıdan eklentiler terminalde kalır; her eklenti iki kez yazılır |
| 3. Eklenti kendi web sayfasını sunar | En esnek | Uyum eklenti yazarına kalır; pencerede eklentinin JavaScript'i çalışır; köprü ve güvenlik işi büyük |

Kullanıcı 1'i seçti (2026-11-24). İlk eklenti: DeltaX (kullanıcının seçimi).

## Mimari

```
 AND penceresi (uygulama.js)                AND (Go, internal/masaustu)              Eklenti (ör. DeltaX)
 ┌──────────────────────────┐  SSE ekran  ┌─────────────────────────────┐  stdout  ┌──────────────────────┐
 │ çizici: ağaç → DOM       │◀────────────│ arayüz oturumu: doğrula,    │◀─────────│ arayuz.Calistir(m)   │
 │ (AND bileşenleri, CSS)   │             │ temizle, son ekranı sakla   │  JSON    │ m.Arayuz() → Ekran   │
 │ tık / form / sekme       │────────────▶│ olayı satır olarak yaz      │─────────▶│ Update(arayuz.Olay)  │
 └──────────────────────────┘ POST olay   └─────────────────────────────┘  stdin   └──────────────────────┘
```

### Etkinleşme ve geri dönüş

- Manifest'e yeni alan: `"ui": 1` (`pluginapi.Manifest.UI int`, `json:"ui,omitempty"`). Anlamı:
  eklenti AND'ın arayüz protokolünün bu sürümünü konuşur. Alan yoksa ya da 0 ise eklenti bugünkü gibi
  gömülü terminalde açılır.
- Masaüstü, `ui` değeri 1 ≤ ui olan eklentiyi **arayüz kipinde** başlatır: standart girdi/çıktı
  borularla AND'a bağlı, sözde terminal yok, ortamda `AND_ARAYUZ_PROTOKOLU=1` (AND'ın konuştuğu en
  yüksek sürüm). Windows'ta `CREATE_NO_WINDOW`. Standart hata AND'ın günlüğüne (`and.log`, eklenti
  adı önekli, satır başına en çok 4 KiB, saniyede en çok 20 satır) gider.
- Eklenti `ui` > AND'ın sürümü ise AND onu arayüz kipinde başlatmaz; gömülü terminalde açar (eklentinin
  terminal sürümü her zaman vardır).
- Terminal arayüzü (`and --terminal`) eklentileri her zaman bugünkü gibi açar; ortam değişkeni yoktur.
- Masaüstünde arayüz kipindeki eklentilerin "Ayrı pencerede aç" düğmesi yoktur (kullanıcı: eklentiler
  terminalde çalışmasın). Arayüz desteği olmayan eklentilerde bugünkü düğme ve terminal kalır.
- Eski AND `ui` alanını tanımaz, eklentiyi terminalde açar; eklenti ortam değişkenini görmez ve terminal
  arayüzünü çalıştırır. Geriye uyumluluk böylece kendiliğinden sağlanır.

### Protokol (sürüm 1)

Taşıma: UTF-8 JSON Lines; her satır bir nesne. Bilinmeyen alanlar iki yönde de yok sayılır.

**AND → eklenti** (eklentinin standart girdisi):

```json
{"tur":"merhaba","surum":1,"dil":"tr"}
{"tur":"gorunur","deger":true}
{"tur":"olay","olay":"tikla","id":"commit"}
{"tur":"olay","olay":"sec","id":"gruplar","deger":"g-42"}
{"tur":"olay","olay":"ac","id":"gruplar","deger":"g-42"}
{"tur":"olay","olay":"gonder","id":"yeniGrup","degerler":{"ad":"Arayüz ekibi"}}
{"tur":"olay","olay":"degisti","id":"arama","deger":"diff"}
{"tur":"olay","olay":"sekme","id":"projeSekmeleri","deger":"gecmis"}
{"tur":"olay","olay":"diyalogKapat"}
{"tur":"olay","olay":"geri"}
```

- `merhaba`: ilk satır. `gorunur`: kullanıcı eklentinin ekranına geçti/ayrıldı (eklenti, örneğin
  mesajları ancak görünürken okundu sayabilir). Başlangıçta eklenti görünmüyor sayılır; AND ilk
  `gorunur: true`'yu kullanıcı ekranı açınca gönderir (açılışta arka planda başlayan eklenti bir süre
  görünmeden çalışır).
- `dil` yalnızca `merhaba`'da gelir: kullanıcı AND'ın dilini değiştirirse çalışan eklenti eski dilde
  kalır, bir sonraki başlatmada yeni dili alır (bugünkü terminal eklentileri gibi).
- `sec`: listede bir öğe seçildi. `ac`: öğeye çift tıklandı ya da seçiliyken Enter'a basıldı.
- Bir liste ya da tablo satırındaki düğmenin `tikla` olayı `deger` olarak o satırın `id`'sini taşır.
- `gonder`: form gönderildi; `degerler` formun bütün alanları (`girdi`, `metinAlani`, `secim`: metin;
  `onayKutusu`: `"1"`/`"0"`).
- `degisti`: yalnızca `canli: true` olan girdilerde, 250 ms susmadan sonra.
- `diyalogKapat`: kullanıcı diyaloğu Esc ya da × ile kapattı. `geri`: diyalog yokken Esc ya da Alt+←.

**Eklenti → AND** (eklentinin standart çıktısı):

```json
{"tur":"ekran","ekran":{ "baslik":"DeltaX", "...": "..." }}
{"tur":"bildirim","metin":"Kayıt oluşturuldu","durum":"iyi"}
```

- `ekran`: ekranın **tamamı** (fark değil). SDK aynı ekranı iki kez göndermez. AND ekranları en çok
  her 50 ms'de bir arayüze iletir (sonuncusu kazanır).
- `bildirim`: AND'ın köşe bildirimi (`durum`: bilgi, iyi, uyari, hata).

**Sınırlar** (AND uygular; aşan satır atılır, günlüğe yazılır, eklentinin ekranında hata kartı
gösterilir): satır ≤ 4 MiB; ağaç derinliği ≤ 24; öğe sayısı ≤ 5000; bir listede/tabloda ≤ 2000 öğe
(fazlası için eklenti sayfalar); düz metin alanı ≤ 64 KiB; `kod`, `metinAlani`, `fark` içerikleri
toplamda ≤ 3 MiB; `id` ≤ 512 karakter.

### Bileşen kataloğu (v1)

Her öğe `{"tur": "...", ...}`. Etkileşimli öğelerin `id`'si eklentinin seçtiği bir dizgedir ve
olaylarda geri döner. Varyantlar anlamsaldır; renk ve ölçü AND'ındır. İkonlar AND'ın ikon kümesinden
adla seçilir (bilinmeyen ad → ikon yok).

Kök — `Ekran`:

| Alan | Anlamı |
|---|---|
| `baslik`, `altBaslik`, `ikon` | AND'ın 52 px araç çubuğu (bugünkü `ustCubuk`) |
| `yol` | Kırıntılar `[{id, metin}]`; tıklanınca `tikla` olayı (DeltaX: DeltaX › Grup › Proje) |
| `araclar` | Araç çubuğunun sağındaki denetimler: `dugme` ve `secim` (ör. DeltaX'te dal seçimi) |
| `govde` | Ekranın içeriği (bir öğe) |
| `diyalog` | Varsa kalıcı diyalog: `{id, baslik, govde, dugmeler}`; Esc → `diyalogKapat`. `gonder: true` olan bir diyalog düğmesi diyalog gövdesindeki ilk `form`'u gönderir |
| `durum` | Altta ince durum satırı (ör. "2/3 üye çevrimiçi") |
| `mesgul` | true iken üstte ince ilerleme çizgisi |

Yerleşim: `yigin` (dikey; `aralik`: kucuk/orta/buyuk), `satir` (yatay; `hizala`, `sarmala`),
`bolunmus` (`sol` sabit ~320 px kendi kaydırmasıyla, `sag` kalan alan; dar pencerede alt alta),
`bolum` (`baslik`, `aciklama`, `araclar`, içerik), `kart`, `sekmeler` (`id`, `secili`,
`sekmeler: [{id, etiket, ikon, sayac}]`; yalnızca sekme çubuğu: içeriği eklenti seçili sekmeye göre
ayrıca verir), `ayrac`.

İçerik: `metin` (`metin` ya da zengin satır `parcalar: [{metin, stil}]`; `stil`: normal, soluk,
kalin, kod, iyi, uyari, hata, vurgu; `boyut`: kucuk/normal/buyuk), `baslik` (`duzey` 2–3), `rozet`
(`metin`, `renk`: notr/vurgu/iyi/uyari/hata), `anahtarDeger` (`satirlar: [{anahtar, deger, kod,
kopyala}]`), `bos` (boş durum: `ikon`, `baslik`, `metin`, isteğe bağlı `dugme`; AND'ın ağ haritalı
boş durumu), `kod` (eş aralıklı blok, `satirNo`), `fark` (`dosyalar: [{yol, durum (A/M/D), eklenen,
silinen, satirlar: [{tur: "+"|"-"|" "|"@", metin, eski, yeni}]}]`; dosya başına katlanır),
`ilerleme` (`deger` 0–1 ya da belirsiz), `baglanti` (`id` → `tikla`, ya da `url`: yalnızca http/https,
AND adresi göstererek onay sorar ve varsayılan tarayıcıda açar).

Etkileşimli:

| Öğe | Alanlar | Olay |
|---|---|---|
| `dugme` | `id`, `metin`, `ikon`, `varyant` (normal/birincil/hayalet/tehlikeli), `devreDisi`, `gonder` (içinde bulunduğu formu gönderir; olay `gonder` olur), `kisayol`, `onay: {baslik, metin, dugme}` (AND önce sorar), `kopyala` (tıklayınca bu metni panoya kopyalar; olay gitmez) | `tikla` |
| `liste` | `id`, `ogeler: [{id, baslik, alt, ikon, rozetler: [{metin, renk}], sag, soluk, dugmeler}]`, `secili`, `bos`, `arama` (AND'ın içinde süzme kutusu) | `sec`, `ac`, satır düğmesi `tikla` (`deger` = satır `id`) |
| `tablo` | `id`, `sutunlar: [{baslik, hizala}]`, `satirlar: [{id, hucreler, dugmeler}]` (hücre: metin ya da `parcalar`) | `sec`, satır düğmesi `tikla` (`deger` = satır `id`) |
| `form` | `id`, içerik (alanlar ve düğmeler); `gonder: true` olan `dugme` formu gönderir; tek satırlı girdide Enter da gönderir, `metinAlani`'nda Ctrl+Enter | `gonder` |
| `girdi` | `id`, `etiket`, `deger`, `yerTutucu`, `tip` (metin/sifre), `enUzun`, `zorunlu`, `aciklama`, `hata`, `oneriler: [{deger, etiket, alt}]`, `canli`, `odak` | `degisti` (`canli` ise) |
| `metinAlani` | `id`, `etiket`, `deger`, `satir`, `kod` (eş aralıklı, Tab girinti ekler), `enUzun` | — (form ile) |
| `secim` | `id`, `etiket`, `secenekler: [{deger, etiket}]`, `deger`, `aninda` (seçince `degisti` gönderir; dal seçimi gibi) | `degisti` (`aninda` ise) |
| `onayKutusu` | `id`, `etiket`, `deger` | — (form ile) |
| `mesajAkisi` | `id`, `mesajlar: [{id, kimden, metin, zaman, giden, durum}]` (Özel Chat için; 2. aşama) | — |

**Kullanıcının yazdığı metin korunur:** AND bir girdinin değerini yalnızca eklenti `deger`'i bir önceki
ekrana göre **değiştirdiyse** günceller (ör. gönderimden sonra boşalttıysa); yoksa kullanıcının yazdığı
kalır. Odak, imleç/seçim ve kaydırma konumları yeniden çizimde korunur (anahtar: öğe `id`'si, yoksa
ağaçtaki yolu).

### AND tarafı (`internal/masaustu`)

- **Oturumlar.** Gömülü terminallerin (`terminal.go`) yanında arayüz oturumları: eklenti adı, durum
  (çalışıyor/bitti, çıkış kodu), son doğrulanmış ekran (JSON), sürüm numarası. Kenar çubuğundaki yeşil
  nokta ve açılışta kendiliğinden başlatma iki tür oturum için de aynı çalışır; `icerideBaslat`
  eklentinin `ui` alanına göre birini seçer. İki tür tek bir listeyle (`/api/eklenti/oturumlar`:
  `{id, ad, tur: "arayuz"|"terminal", bitti, kod}`) arayüze verilir.
- **Uç noktalar** (bugünkü korumalar: çerez, `X-AND` başlığı, Origin denetimi):
  `GET /api/arayuz/akis` (SSE: `ekran {id, surum, ekran}`, `bildirim`, `bitti`), `GET /api/arayuz/ekran?id=`
  (son ekran), `POST /api/arayuz/olay` (`{id, ...olay}` → eklentiye bir satır), `POST /api/arayuz/gorunur`,
  `POST /api/arayuz/kapat`.
- **Doğrulama ve temizlik** `arayuz.Denetle` ile (paylaşılan kod; AND ve testler kullanır): bilinmeyen
  `tur` → yerine soluk "Bu öğe bu AND sürümünde gösterilemiyor" metni; tüm dizgeler `pluginui.Safe`
  (satır sonu ve sekme kalır); ikon adları beyaz listede; `url` yalnızca http/https; sınırlar yukarıda.
- **Eklenti kapanınca** ekranın yerine bugünkü "kapandı / Yeniden aç" durumu gelir; diyalog kapanır.
- **Açılışta kendiliğinden başlatma**: değişmez; arayüz kipindeki eklenti de aynı kurallarla başlar.

### Çizici (`web/`)

- Eklenti ekranı bugünkü eklenti görünümünün (`eklentiCiz`) yerini alır: AND'ın araç çubuğu ekranın
  `baslik`/`yol`/`araclar`'ından, gövde ağaçtan çizilir. Kenar çubuğu, Ctrl+K paleti ve durum çubuğu
  aynı kalır.
- Her bileşen AND'ın bugünkü sınıflarını ve tasarım belirteçlerini kullanır (liste seçimi vurgu
  çubuğuyla, alt çizgili sekmeler, düğme varyantları, ağ haritalı boş durum, ayrı katmanlı kartlar);
  yeni stiller `uygulama.css`'te "Eklenti arayüzü" bölümünde.
- Yalnızca `textContent`/`createElement`; eklenti metni hiçbir zaman HTML olarak yorumlanmaz.
- `kisayol` biçimi: küçük harf, değiştiriciler `ctrl`/`alt`/`shift` `+` ile, sonra tuş (`a`–`z`,
  `0`–`9`, `enter`, `delete`, `f1`–`f12`); ör. `ctrl+s`, `ctrl+enter`, `f2`. Esc ve AND'ın kendi
  kısayolları (Ctrl+K…) ayrılmıştır; çakışan kısayol yok sayılır. Kısayollar yalnızca eklentinin ekranı
  açıkken geçerlidir; yazı alanındayken yalnızca Ctrl/Alt'lı olanlar çalışır. Diyalogda Esc kapatır,
  odak diyalogda tutulur.
- Ekran yenilenince seçili öğe görünür alanda tutulur; listede ↑/↓ seçimi değiştirir (`sec`), Enter
  `ac` gönderir.
- AND'ın kendi yeni metinleri (ör. "Bu öğe bu AND sürümünde gösterilemiyor", "Süz", "Kopyalandı")
  `t()` ile.

### SDK (`github.com/lucian95511/and/arayuz`)

- **Türler**: `Ekran`, `Oge` (JSON'a birebir giden tek yapı ya da tür başına yapılar; uygulayıcının
  seçimi) ve okunaklı kurucular, ör. `arayuz.Liste("gruplar", ogeler...).Secili(id)`,
  `arayuz.Dugme("commit", dil.T("Kaydet")).Birincil().Kisayol("ctrl+s")`.
- **Olaylar**: `arayuz.Olay{Olay, ID, Deger string; Degerler map[string]string}` ve
  `arayuz.Gorunurluk{Gorunur bool}` birer `tea.Msg`'dir.
- **Çalıştırma**: `arayuz.Etkin() bool` (ortam değişkenine bakar) ve `arayuz.Calistir(m Model) error`,
  `type Model interface { tea.Model; Arayuz() Ekran }`. `Calistir` bubbletea'yı çizicisiz çalıştırır
  (`tea.WithoutRenderer()`, `tea.WithInput(nil)`, sinyal işleyicisiz): standart girdiden okunan olaylar
  `p.Send` ile modele gider; her `Update`'ten sonra `Arayuz()` hesaplanır ve değiştiyse yazılır.
  Böylece eklentinin bugünkü `tea.Cmd`/`tea.Msg` düzeni (zamanlayıcılar, API çağrıları, arka plan
  işleri) olduğu gibi çalışır. İlk ekran `Init`'ten hemen sonra, `merhaba` beklenmeden gönderilir;
  `merhaba` modele `arayuz.Merhaba{Surum, Dil}` mesajı olarak gelir. `Calistir` gerçek standart çıktıyı yalnız protokole ayırır ve
  `os.Stdout`'u standart hataya yönlendirir (yanlışlıkla yapılan `fmt.Print` protokolü bozmasın).
  Standart girdi kapanınca (AND kapandı) program biter.
- **Test**: `arayuz/arayuztest`: modeli olaylarla sürer, son ekranı döner, `id`'ye göre öğe bulur ve
  metinleri toplar; `diltest.HerDilde` ile her dilde ekran denetimi yapılabilir.
- Sürüm sabiti `arayuz.Surum = 1`. Belgeler: EKLENTILER.md ve PLUGINS.md'ye "Arayüz" bölümü.

### DeltaX'in taşınması (1. aşamanın hedefi)

İlke: **DeltaX'in `ekran` durumu iki arayüzün ortak kaynağıdır.** `keys.go`'daki tuş işleyicilerinin
yaptığı işler eylem yöntemlerine ayrılır (ör. `grupOlustur(ad)`, `commitAt(mesaj)`, `dalaGec(ad)`,
`gonder(hedef)`); tuşlar ve arayüz olayları aynı yöntemleri çağırır. Yeni `arayuz.go` `Arayuz()`'ü
mevcut durumdan çizer; yeni `olaylar.go` olayları eylemlere çevirir. `main.go`:
`if arayuz.Etkin() { return arayuz.Calistir(m) }`. `plugin.json`'a `"ui": 1`; sürüm 1.8.0.

Ekranların grafik karşılığı (TUI'nin 27 ekranı birebir kopyalanmaz):

| Görünüm | İçerik | TUI ekranları |
|---|---|---|
| **Giriş** | Sol: Gruplar (süzme, "Yeni grup"), Davetler bölümü (sayaçlı; Kabul/Reddet). Sağ: kimlik kartı (Peer ID, kopyala), son kayıtlar (tıklayınca o projenin Geçmiş'i) | ekAnaMenu, ekKimlik, ekGrupListesi, ekDavetler |
| **Grup** | Sol: Projeler ("Yeni proje", son kayıtları). Sağ: Üyeler (rol rozetleri, çevrimiçi noktası), "Üye davet et" (eş önerili form), yönetici atama, "Gruptan çık" / "Grubu kapat" (tehlikeli + onay) | ekGrupPaneli, ekUyeDavet, ekYoneticiSecim |
| **Proje** | Araç çubuğu: dal seçimi (kirliyse onay), Gönder, Çek, Dışa aktar, Klasörü aç. Üstte durum kartı (son kayıt, değişiklik sayıları, uzaklarda yeni kayıt). Sekmeler aşağıda | ekProjePaneli |
| › Değişiklikler | A/M/D rozetli liste; seçilince sağda `fark`; satırda "Geri al" (onaylı). Altta kayıt formu (mesaj + Kaydet, Ctrl+Enter) | ekDurum, ekCommitMesaj, ekDiff |
| › Geçmiş | Süzülebilir kayıt listesi; seçilince ayrıntı (anahtar-değer, değişen dosyalar, `fark`), "Buradan dal oluştur" | ekGunluk, ekKayit, ekDiff |
| › Dallar | Dallar (etkin rozeti, son kayıt), Geç / Birleştir / Sil (onaylı), "Yeni dal" formu | ekDallar, ekYeniDal |
| › Uzak | Üye başına uzak dallar ve yeni kayıt sayıları; üyeden Çek, "Çevrimiçi herkesten çek" | ekUzakDallar, ekPullIste, ekGonderSecim |
| › Dosyalar | Dosya listesi; açınca `metinAlani` (`kod`) düzenleyici + Kaydet (Ctrl+S), Yeni dosya, Sil | ekDosyaListesi, ekYeniDosya, ekDosyaDuzenle |
| Diyaloglar | Yeni grup, Yeni proje, Dışa aktar (hedef klasör yolu), gelen dosya onayı (her görünümün üstünde), genel onaylar, birleştirme çatışmaları (uyarı kartı) | ekGrupOlustur, ekProjeOlustur, ekDisaAktar, ekDosyaOnay, ekOnay |

`ekYardim` grafik arayüzde gerekmez. DeltaX'in TUI'si değişmeden çalışmaya devam eder (testleri yeşil
kalır). Büyük farklarda DeltaX dosya başına en çok 2000 satır gönderir ve kalanını "… N satır daha"
satırıyla belirtir (protokol sınırları aşılmasın); listeler 2000 öğeyi aşarsa en yenileri gösterir.

## Aşamalar

1. **Altyapı + DeltaX** (bu belgenin kapsamı): `arayuz` + `arayuztest`, manifest alanı, AND'da arayüz
   oturumları + uç noktalar + çizici + stiller, açılışta başlatma ve yeşil nokta entegrasyonu, belgeler,
   DeltaX'in taşınması. DeltaX ayrı depodadır: orada ayrı commit; exe yeniden derlemesi kullanıcının
   onayıyla.
2. Özel Chat (`mesajAkisi`, okundu bilgisi `gorunur` ile).
3. Site (uzun metin düzenleme, şablonlar, taslaklar).
4. Yönetici Paneli (`tablo`, sekmeler).
5. `and eklenti yeni` iskeleti ve `ornek` arayüz kipiyle gelir.

Her aşama ayrı plan ve ayrı commit dizisi; 2–5 bu belgenin kataloğunu kullanır, gerekirse kataloğa
geriye uyumlu öğe ekler.

## Doğrulama

- Go: `arayuz` JSON gidiş-dönüş, `Denetle` sınırları ve temizlik, `Calistir` (sahte stdin/stdout ile
  küçük bir model), `arayuztest`.
- AND: protokolü konuşan sahte eklentiyle (bugünkü `TestMain` kipi gibi) başlatma, ilk ekran, olay →
  yeni ekran, sınır aşımı, kapanma, açılışta başlatma, Ayarlar'dan kapatma; Windows'ta konsol
  penceresi açılmaması.
- DeltaX: `arayuztest` ile akışlar (grup kur, proje kur, kayıt at, fark gör, dal oluştur/geç, dosya
  düzenle) ve 6 dilde her görünüm; mevcut TUI testleri yeşil.
- Gözle: önizleme düzeneğine gerçek DeltaX exe'siyle çalışan bir kip (`ONIZLEME_ARAYUZ=<exe>`);
  CDP ile her görünümün ve diyaloğun ekran görüntüsü, dar ve geniş pencere, koyu temalar.
- Kapanış: `go vet` (windows/linux/darwin), `go test ./...`, LF ve CRLF çekim benzetimleri, metin
  kapsam testleri.

## Uygulayıcıya bırakılanlar

Bileşenlerin görsel ayrıntıları (AND'ın tasarım diline uyarak), Go kurucu API'sinin biçimi, JS
dosyalarının düzeni (`uygulama.js` içinde bölüm ya da ayrı dosya; ayrıysa metin çıkarıcısı onu da
taramalı), yeniden çizimde durumu koruma tekniği (yukarıdaki gereksinimleri karşılayan herhangi biri),
DeltaX görünümlerinin yerleşim ayrıntıları.

## Benim verdiğim kararlar (kullanıcı kontrol etsin)

- Protokol taşıması stdin/stdout JSON Lines (HTTP değil): ek port ve anahtar yok, her platformda çalışır.
- Arayüz kipindeki eklentilerde masaüstünden terminal sürümüne geçiş yok ("Ayrı pencerede aç" kalkar).
- Dosya/klasör yolları v1'de metin alanıyla seçilir (yerel dosya penceresi yok).
- JSON alan ve bileşen adları Türkçe (SDK'nın geri kalanı gibi).
- SDK bubbletea üstüne kurulur: bugünkü eklentiler `Update`/`Cmd` düzenini korur.
