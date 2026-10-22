# AND

**AND** — sunucusuz, eşten eşe (P2P) terminal forum topluluğu.  
Hesap yok, kayıt yok, merkezi sunucu yok. Kimliğin tek bir kurtarma kodundan ibarettir.

---

## İçindekiler

- [Genel Bakış](#genel-bakış)
- [Özellikler](#özellikler)
- [Mimari](#mimari)
- [Kurulum](#kurulum)
- [Kullanım](#kullanım)
- [Moderasyon](#moderasyon)
- [Eklenti Sistemi](#eklenti-sistemi)
- [Ekosistem](#ekosistem)
- [Yapılandırma](#yapılandırma)
- [Katkı](#katkı)
- [Güvenlik](#güvenlik)
- [Lisans](#lisans)

---

## Genel Bakış

AND, merkezi bir sunucuya ihtiyaç duymadan çalışan terminal tabanlı bir forum uygulamasıdır.  
Her düğüm hem istemci hem sunucu işlevi görür; mesajlar [libp2p](https://libp2p.io/) GossipSub protokolü üzerinden yayılır, veriler yalnızca yerel SQLite veritabanında saklanır.

Kullanıcı kimliği bir BIP-39 anımsatıcısından (12 sözcük) türetilen Ed25519 anahtar çiftiyle temsil edilir. Bu anahtar çifti hem uygulama kimliği hem libp2p düğüm kimliği olarak kullanılır; tek bir kurtarma ifadesi her şeyi geri getirir.

---

## Özellikler

| Özellik | Açıklama |
|---------|----------|
| Sunucusuz | Merkezi altyapı yoktur; ağ eşler arası çalışır |
| BIP-39 kimlik | 128 bit entropi (12 kelimelik BIP-39 anımsatıcısına eşdeğer; ekranda 32 karakterlik kurtarma kodu olarak gösterilir) → Ed25519 anahtar → libp2p kimliği |
| Ed25519 imzalı içerik | Her konu, yanıt ve moderasyon kararı imzalanır |
| SQLite yerel depo | WAL modunda yüksek eşzamanlılık, şema versiyonlama |
| Forum kategorileri | Python, Rust, Go, Siber Güvenlik, Yapay Zeka ve daha fazlası |
| Kişisel site | Her kullanıcı, birbirinden bağımsız çok sayılı sayfa (slug ile adreslenen) yayınlayabileceği kendi "kişisel site"sini oluşturabilir — imzalı, GossipSub'la yayılan, geriye dönük senkronize edilen (bir sayfanın düzenlenmiş hali dahil) |
| Özel mesajlaşma | libp2p stream (`/and/dm/1.0.0`) üzerinden P2P doğrudan mesaj |
| Dosya aktarımı | Yeniden başlatılabilir parçalı aktarım (`/and/file/2.0.0`); onay isteği dialog'u |
| Moderasyon sistemi | Esnek süreli (1 saat–ömür boyu) ban/unban, devredilebilir/iptal edilebilir moderatör sertifikaları, Yönetici Paneli eklentisi + denetim geçmişi |
| Dinamik eklenti sistemi | Bağımsız binary'ler; AND'ı yeniden derlemeden eklenti ekle |
| Otomatik güncelleme | GitHub Releases üzerinden arka planda güncelleme |
| Terminal arayüzü | [Bubbletea](https://github.com/charmbracelet/bubbletea) tabanlı, tam ekran TUI; AND, eklentiler ve AND Arama Motoru aynı tasarım sistemini paylaşır (`internal/ui`) |
| Temalar | 6 yerleşik tema (Klasik, Turkuaz, Gece Moru, Kehribar, Orman, Mono) + tema eklentileri; seçilen tema eklentilere ve VS Code paneline de yansır |
| Arayüz dilleri | Türkçe (varsayılan), English, Deutsch, Español, Français, Русский; Ayarlar → Dil ya da giriş ekranında `ctrl+l` ile anında değişir — bkz. [Arayüz dili](#arayüz-dili) |

---

## Mimari

```
cmd/
  and/              — ana uygulama giriş noktası
  andmod/           — moderasyon CLI aracı (andmod grant, ban)

Eklentiler/
  admin/            — yönetici paneli eklentisi (bağımsız binary)
  konu_ac/          — konu açma (menüde gizli; AND içinde inline açılır)
  ozel_chat/        — özel mesajlaşma + dosya aktarımı eklentisi
  site/             — kişisel site oluşturma/düzenleme eklentisi
  ornek/            — yeni eklenti geliştirme rehberi

internal/
  crypto/           — BIP-39 → Ed25519 kimlik, AES-GCM şifreli depolama
  network/          — libp2p düğümü, GossipSub, DHT keşfi, sync protokolü
  forum/            — konu/yanıt yönetimi, imza doğrulama, TTL, tombstone senkronu
  chatlog/          — genel P2P sohbet kanalı (imzalı mesaj, hız sınırı, senkron)
  moderation/       — kurucu/moderatör sertifikası, esnek süreli ban/unban, denetim
                       geçmişi, ConnectionGater
  profile/          — kullanıcı profili (bio/durum/link/etiket), rol rozetleri
  site/             — çok sayfalı kişisel site (imzalı sayfa yayını/silme, kendi senkron protokolü)
  peersync/         — geçmiş senkronizasyonunun zamanlaması: hangi eşle, ne zaman (bkz. aşağıda)
  ui/               — ortak tasarım sistemi: tema/palet, stiller, tam ekran sayfa iskeleti, bileşenler
  storage/          — SQLite WAL, şema versiyonlama
  pluginapi/        — eklenti HTTP IPC API sunucusu + istemci kütüphanesi
  pluginmgr/        — and-plugin-* binary keşfi ve başlatma
  pluginui/         — eklenti/uzak içerik render'ı için ANSI/kontrol karakteri temizliği
  dmmgr/            — libp2p DM stream handler + ozel_chat'e long-poll proxy
  filemgr/          — parçalı dosya gönderme/alma; onay (consent) mekanizması
  githubverify/     — GitHub OAuth Device Flow ile hesap doğrulama
  bbcode/           — forum gövdesi için basit BBCode → terminal render
  tui/              — Bubbletea TUI (menü, forum, sohbet, profil, moderasyon paneli, ayarlar)
  updater/          — GitHub Releases kontrolü ve uygulama güncelleme
```

### Veri akışı — forum konusu

```
Kullanıcı → n tuşu (forum ekranında)
                ↓  inline Bubbletea ekranı (konu_ac)
            AND ana süreç → Forum.CreatePost()
                ↓  Ed25519 imzala
            GossipSub yayını (libp2p)
                ↓
        Tüm peer'lar → imza doğrula → SQLite
```

### Veri akışı — özel mesaj (DM)

```
and-plugin-ozel-chat  →  POST /api/v1/dm/send  →  AND (dmmgr.Broker)
                                                         ↓  libp2p stream
                                                     Hedef peer

Hedef peer  →  /and/dm/1.0.0 stream handler  →  dmmgr.Broker.Deliver()
                                                         ↓  şifreli geçmiş + long-poll
                                              and-plugin-ozel-chat (görünür)
```

AND, gelen ve giden özel mesajları bu bilgisayarda şifreli saklar (`dm_gecmis.bin`) ve
`GET /api/v1/dm/history` ile verir. Böylece Özel Chat kapalıyken gelen bir mesaj kaybolmaz;
ekran açıldığında konuşma listesinde okunmamış olarak görünür. İnsanlar arası mesajlardan en
fazla 500, eklentilerin makine mesajlarından (ör. DeltaX'in `AND-PROJEAG/…` grup protokolü;
bkz. `protokol.MakineIletisiMi`) ayrıca en fazla 300 tutulur; makine mesajları okunmamış
sayısına katılmaz ve Özel Chat'te görünmez.

### Veri akışı — dosya aktarımı

```
Gönderen: and-plugin-ozel-chat  →  POST /api/v1/file/send  →  AND (filemgr.Broker)
                                                                      ↓  /and/file/2.0.0 stream
                                                                  Hedef peer

Hedef peer  →  /and/file/2.0.0 stream handler  →  consent isteği bekle (30 sn)
                       ↓  GET /api/v1/file/consent-poll (long-poll)
               and-plugin-ozel-chat → dialog göster → POST /api/v1/file/consent
                       ↓  kabul edilirse
               Parçalı aktarım → disk → GET /api/v1/file/poll ile bildirim
```

### Geçmiş senkronizasyonu

GossipSub yalnızca o an bağlı olanlara ulaşır. Çevrimdışıyken yayınlanan konuları, sohbet
mesajlarını ve site sayfalarını AND, eşlerle ayrıca konuştuğu senkron protokolleriyle
(`/and/sync/1.0.0`, `/and/chatsync/1.0.0`, `/and/sitesync/1.0.0`) geriye dönük toplar.
Ne zaman ve kimle senkron yapılacağına `internal/peersync` karar verir:

| Durum | Davranış |
|---|---|
| Yeni bir eş bağlandı | Kısa bir beklemeden sonra (identify protokolleri öğrensin diye) senkron yapılır |
| Eş AND protokolünü konuşmuyor | Atlanır. Bağlı eşlerin çoğu genel IPFS/DHT düğümüdür; onlarla akış açmak boşa gider |
| Eş kopup yeniden bağlandı | Son başarılı senkrondan 2 dakika geçtiyse tekrar senkron yapılır (aradaki içerik gelir) |
| Senkron başarısız oldu | 1 dakika sonra yeniden denenir |
| Her 10 dakikada bir | Bağlı AND eşlerinden en uzun süredir senkronlanmamış 3'üyle senkron yapılır; bağlantı hiç kopmasa da kaybolan gossip mesajları zamanla gelir |

Aynı anda en fazla 4 senkron çalışır ve aynı eşle iki senkron asla eşzamanlı açılmaz. Açılışta
yüzlerce eş birden bağlansa da yüzlerce akış birden açılmaz. Ana menünün üst bölümü kaç
bağlı eşin AND düğümü olduğunu ve son senkronun ne zaman yapıldığını gösterir.

---

## Kurulum

### Gereksinimler

- Go 1.21 veya üzeri
- Windows, Linux veya macOS (amd64 / arm64)

### Kaynaktan derleme

```bash
git clone https://github.com/lucian95511/and.git
cd and

# Ana binary ve yönetim aracı
go build -o and     ./cmd/and
go build -o andmod  ./cmd/andmod

# Eklenti binary'leri (and ile aynı dizine koy)
go build -o and-plugin-admin      ./Eklentiler/admin
go build -o and-plugin-konu-ac    ./Eklentiler/konu_ac
go build -o and-plugin-ozel-chat  ./Eklentiler/ozel_chat
go build -o and-plugin-pembis     ./Eklentiler/Pembis

# DeltaX — Eklentiler/DeltaX/ altına derlenmeli (plugin.json yanında)
go build -o Eklentiler/DeltaX/and-plugin-DeltaX ./Eklentiler/DeltaX

# Site — Eklentiler/site/ altına derlenmeli (plugin.json yanında)
go build -o Eklentiler/site/and-plugin-site ./Eklentiler/site
```

Windows'ta her binary adına `.exe` uzantısı ekle.

### ldflags — versiyon ve repo bilgisi

Otomatik güncelleme ve `--version` için:

```bash
go build \
  -ldflags "-X github.com/lucian95511/and/internal/updater.Version=v1.0.0 \
            -X github.com/lucian95511/and/internal/updater.GitHubRepo=lucian95511/and" \
  -o and ./cmd/and
```

### Kurucu anahtarını binary'ye gömmek

```bash
# Kurucu public key'ini öğren
./andmod pubkey

# Binary'yi kurucu public key'i ile derle
go build \
  -ldflags "-X github.com/lucian95511/and/internal/moderation.FounderPubKeyHex=<64-karakter-hex>" \
  -o and ./cmd/and
```

Kurucu public key'i binary'ye gömülüdür; `founder.key` dosyası ilk çalıştırmada otomatik oluşur.

### GitHub hesap doğrulamasını etkinleştirmek

Ayarlar'daki "GitHub hesabını doğrula" (v) özelliği, GitHub'ın OAuth Device
Flow'unu kullanır ve bir Client ID gerektirir:

1. https://github.com/settings/developers → **New OAuth App**.
2. Authorization callback URL alanına herhangi bir değer yazılabilir (Device
   Flow bunu kullanmaz), ör. `https://example.com`.
3. Uygulama ayarlarında **Enable Device Flow**'u işaretle.
4. Oluşan **Client ID**'yi al (Client Secret'a ihtiyaç YOK — Device Flow
   secret gerektirmeyen "public client" akışıdır).

```bash
go build \
  -ldflags "-X github.com/lucian95511/and/internal/githubverify.ClientID=<client-id>" \
  -o and ./cmd/and
```

Client ID ayarlanmazsa özellik sessizce devre dışı kalmaz — kullanıcı "v"ye
bastığında açık bir hata gösterilir ("ClientID yapılandırılmamış").

---

## Kullanım

### Dizin yapısı (çalıştırma)

Tüm binary'leri aynı dizine koy:

```
/herhangi/bir/dizin/
  and[.exe]
  andmod[.exe]
  and-plugin-admin[.exe]
  and-plugin-konu-ac[.exe]
  and-plugin-ozel-chat[.exe]
```

AND başlangıçta `and-plugin-*` adındaki binary'leri otomatik bulur.

### İlk çalıştırma

```bash
./and
```

İlk açılışta yeni kimlik oluşturulur:

1. Bir takma ad seç (görünüm amaçlıdır; imzaları etkilemez)
2. Kimliğini korumak için güçlü bir parola gir
3. **Kurtarma kodunu güvenli bir yere yaz** (`XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX`, 32 karakter) — parolanı unutursan ya da başka bir bilgisayara geçersen kimliğini geri getirmenin tek yolu budur. Kod, yazdığını doğrulamak için bir kez daha sorulur

Sonraki açılışlarda yalnızca parolanı girmen yeterlidir.  
Kimlik dosyası: `%APPDATA%\and\identity.dat` (Linux/macOS: `~/.config/and/identity.dat`)

Giriş ekranında `ctrl+l` (ya da `F2`) arayüz dilini değiştirir; seçim kaydedilir ve sonraki
açılışlarda da kullanılır (bkz. [Arayüz dili](#arayüz-dili)).

### Ana menü

AND terminalin tamamını kullanır. Ana menünün üstünde amblem, kimliğin ve kısa sayılar yer alır.
Altında solda menü ağacı, sağda seçili öğenin ayrıntısı bulunur: Forum için son konular ve
kategori dağılımı, Sohbet için son mesajlar ve en çok yazanlar, eklentiler için açıklama, sürüm
ve durum. Çok geniş terminallerde ayrıntının yanına ikinci bir sütun gelir. Dar ya da kısa
terminallerde ayrıntı paneli ve büyük amblem kendiliğinden gizlenir.

```
╭─  AND  ─────────────────────────────────────────────────────────────────── Klasik teması · v0.1.0 ─╮
│                                                                                                    │
│   █████╗ ███╗   ██╗██████╗                                                                         │
│  ██╔══██╗████╗  ██║██╔══██╗      umut                                             Forum  142 konu  │
│  ███████║██╔██╗ ██║██║  ██║      12D3KooW9r…dWngf13xEF                        Sohbet  1.204 mesaj  │
│  ██╔══██║██║╚██╗██║██║  ██║      ● 4 eş bağlı                              Eklentiler  4 / 5 açık  │
│  ██║  ██║██║ ╚████║██████╔╝                                                                        │
│  ╚═╝  ╚═╝╚═╝  ╚═══╝╚═════╝                                                                         │
│                                                                                                    │
│  ────────────────────────────────────────────────────────────────────────────────────────────────  │
│                                                                                                    │
│  ▾ Topluluk                    │  Forum                                              34 kategori   │
│  ▌ ├── Forum                   │  Kategorilere ayrılmış konular. Bir konu yayınlandığı anda        │
│    └── Sohbet                  │  eşlere yayılır ve 5 gün sonra kendiliğinden silinir.             │
│                                │                                                                   │
│  ▾ Eklentiler                  │  Son konular                                                      │
│    ├── Yönetici Paneli         │                                                                   │
│    ├── DeltaX                  │  ▸ libp2p ile NAT arkasından bağlantı nasıl kurulur?              │
│    └── Site                    │    Go  ·  zeynep  ·  2 sa önce  ·  3 yanıt                        │
│                                │                                                                   │
│  ▾ Sistem                      │  ▸ Ed25519 imzalarında sık yapılan hatalar                        │
│    ├── Ayarlar                 │    Siber Güvenlik  ·  mehmet  ·  5 sa önce                        │
│    └── Çıkış                   │                                                                   │
│  ────────────────────────────────────────────────────────────────────────────────────────────────  │
│  ↑↓ gezin · enter aç · q çıkış                                                                     │
│                                                                                                    │
╰────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

> Yüklü eklenti binary'leri (`and-plugin-*`) otomatik keşfedilir ve "Eklentiler" grubuna eklenir.
> Seçili eklentide `space` eklentiyi kapatır/açar; kapalı eklentinin yanında "kapalı" yazar.
> `Label: ""` olan eklentiler (örn. `konu_ac`) menüde görünmez; doğrudan forumdan açılır.
> "Yönetici Paneli" girişi yalnızca kurucuya ya da geçerli (iptal edilmemiş) bir moderatör
> sertifikası taşıyanlara görünür — başka hiçbir kullanıcı bu girişi hiç görmez.

### Ayarlar ekranı

Dört sekmesi vardır; `←`/`→` (ya da `tab`) ile geçilir, `esc` ana menüye döner.

| Sekme | İçerik |
|---|---|
| Bilgi | Sürüm, platform, veri dizini; kimlik (ad, genel anahtar, rol, aktivite), kurtarma kodu (`m` ile göster/gizle) ve profil alanları (`b` bio, `s` durum, `w` bağlantı, `t` etiketler, `v` GitHub doğrulama) |
| Eklentiler | Yüklü eklentilerin adı, sürümü, durumu ve açıklaması; `space` ile aç/kapat |
| Temalar | Yerleşik temalar (renk örnekleriyle) ve tema eklentileri; `enter` seçili temayı anında uygular |
| Dil | Arayüz dili; `enter` seçili dili anında uygular ve kaydeder |

### Görünüm ve temalar

AND'ın tüm ekranları ve eklentileri aynı görsel dili kullanır. Her ekran terminalin tamamını kaplayan
bir çerçevenin içindedir. Çerçevenin üst kenarında solda `AND` rozeti, sağda etkin tema ve sürüm
yazar; ekranın başında "Forum › Go › Konu" biçiminde bir başlık yolu, en altında da "tuş açıklama"
biçiminde kısayol satırı bulunur. Seçili satır tam genişlikte boyanır ve solunda `▌` vurgu çubuğu
olur; bildirimler tek satırlık `✓`/`✗` biçimindedir. Konu gövdesi gibi uzun yazılar, satırlar göz
yormasın diye en fazla 100 sütunda sarılır. Bu dil `internal/ui` paketinde tanımlıdır (palet,
stiller, tam ekran sayfa iskeleti ve bileşenler); eklentiler de aynı paketi kullanır.
[AND Arama Motoru](../AND%20arama%20motoru/README.md) ayrı bir Go modülü olduğu için aynı paleti
kendi içinde birebir tekrarlar.

Yerleşik temalar:

| Tema | Vurgu | Kullanım |
|---|---|---|
| Klasik (varsayılan) | mavi-mor | AND'ın ilk günden beri kullandığı görünüm |
| Turkuaz | turkuaz | Daha serin, yeşilimsi mavi |
| Gece Moru | mor | Daha sıcak, mor ağırlıklı |
| Kehribar | kehribar | Sarı/turuncu vurgu; eski fosfor ekranları andırır |
| Orman | yeşil | Yeşil vurgu, toprak tonlu adlar |
| Mono | beyaz/gri | Renksiz; ekran görüntüsü ve sunum için |

Her temada seçili satır metni ve ikincil metin koyu bir terminal zemininde en az 4.5:1 kontrast
verir (WCAG AA). Yalnızca kalın başlıklarda, tuşlarda, amblemde ve çerçevede kullanılan vurgu
rengi için alt sınır 3:1'dir. Bunlar `internal/ui` testlerinde doğrulanır.

Seçilen tema veri dizinindeki `theme.json` dosyasına yazılır ve eklentilere
`GET /api/v1/theme` ile ulaşır: her eklenti açılışta temayı okuyup kendini aynı renklerle boyar,
[and-vscode](../and-vscode/README.md)'nin Canlı Paneli de vurgusunu buradan alır. Tema eklentileri
(ör. `and-plugin-pembis`) `POST /api/v1/theme` ile kendi renklerini uygulayabilir. Eski
sürümler mavi-mor varsayılanı yanlışlıkla "Turkuaz" adıyla kaydediyordu; böyle bir `theme.json`
otomatik olarak Klasik temaya taşınır.

### Arayüz dili

AND'ın arayüzü altı dildedir: Türkçe (varsayılan), English, Deutsch, Español, Français ve
Русский. Dil, Ayarlar → Dil sekmesinden ya da giriş ekranında `ctrl+l` (`F2`) ile seçilir; ekran
hemen yeni dilde çizilir ve seçim veri dizinindeki `dil.json` dosyasına yazılır. `AND_DIL`
ortam değişkeni (ör. `AND_DIL=en ./and`) kayıtlı seçimi geçersiz kılar. Tarihler (ay adları),
binlik ayırıcılar ve sayıya bağlı tekil/çoğul biçimler de (Rusçadaki üç biçim dahil) seçilen
dile göre yazılır.

Yalnızca arayüz çevrilir: forum konuları, kategori değerleri ve sohbet mesajları ağda her zaman
aynıdır. Örneğin kategori ağda "Yapay Zeka" olarak taşınır, İngilizce arayüzde "Artificial
Intelligence" diye gösterilir — farklı dillerdeki eşler aynı konuları görür.

Çeviriler `dil` paketinde (yalnızca standart kütüphaneyi kullanır) ve `dil/ceviriler/*.json`
dosyalarındadır. Türkçe kaynak metin anahtardır; çevirisi olmayan bir metin Türkçe gösterilir.
Kodda metinler `dil.T("…")`, biçimli olanlar `dil.F("%d konu", n)`, paket düzeyindeki tablolar
`dil.K("…")` ile işaretlenir. `dil` testleri, kaynaklardaki her işaretli metnin, her forum
kategorisinin ve paketle gelen eklentilerin ad/açıklamalarının bütün dillerde çevrildiğini ve
çevirilerin aynı biçim fiillerini (`%s`, `%d`) taşıdığını denetler; `internal/tui` testleri de
ekranları her dilde çizip tam ekran düzeninin bozulmadığını ve Türkçe metin kalmadığını
doğrular. Yeni bir metnin eksik çevirilerinin iskeleti şöyle alınır:

```bash
DIL_ISKELET=eksik.json go test ./dil -run Kapsam
```

AND, başlattığı eklentilere etkin dili `AND_DIL` ile iletir; eklentiler `dil.OrtamdanAyarla()`
ile aynı dili kullanabilir ve kendi çevirilerini `dil.Ekle` ile ekleyebilir.

### Klavye kısayolları — forum

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | konu seç |
| `enter` | konuyu aç |
| `n` | yeni konu aç (inline konu_ac ekranı) |
| `tab` | kategori değiştir |
| `esc` | ana menüye dön |

### Kişisel site

Kendi çok sayfalı kişisel siteni oluşturmak/düzenlemek artık **Site**
eklentisiyle yapılır (bkz. [Eklentiler/site/README.md](Eklentiler/site/README.md)
— klavye kısayolları, doğrulama kuralları) — orada görülen ve düzenlenen
yalnızca KENDİ sayfalarındır; başka bir kullanıcının sitesi, onun profil
ekranında (bkz. aşağıdaki profil kısayolları) "Site" bölümü altında
salt-okunur olarak görünür.

Bir sayfa en fazla 8000 bayt, bir yazarın en fazla 20 sayfası olabilir.

### Klavye kısayolları — profil ekranı (forum + sohbet)

Forumda bir konudaki yazarın adına git, ya da sohbette `ctrl+p` ile
seçili mesajın yazarının profilini aç:

| Tuş | İşlev |
|-----|-------|
| `b` | banla (yalnızca kurucu/geçerli sertifikalı moderatör görür) |
| `u` | banı kaldır (yalnızca hedef zaten banlıysa ve yetkiniz varsa görünür) |
| `esc` | önceki ekrana dön |

### Peer ID'ni öğrenmek

Ana menüde kimlik bilgisi ekranında kendi Peer ID'in görünür.  
Başka bir kullanıcıyla özel mesajlaşmak için bu ID'yi paylaşabilirsin.

### Özel bootstrap node'ları

Bilinen AND düğümlerine hızlı bağlanmak için `%APPDATA%\and\bootstrap.txt` dosyasına multiaddr satırları ekle:

```
/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
# bu satır yorum — yoksayılır
/dns4/and-node.example.com/tcp/4001/p2p/12D3KooW...
```

---

## Moderasyon

AND'ın moderasyon sistemi kurucu merkezli ve sertifika tabanlıdır; hem
uygulama içinden (TUI) hem `andmod` CLI'dan yönetilebilir.

Forumda konu onay sistemi yoktur: bir konu yazıldığı anda ağda yayılır ve
her türlü konu, kim tarafından yazılırsa yazılsın, oluşturulduktan **5 gün**
sonra otomatik olarak silinir. Moderatörlerin/kurucunun rolü, süresini
beklemeden istenmeyen bir konuyu erken silebilmek (bkz.
[Yönetici Paneli](#yerleşik-eklentiler)) ve peer banlayabilmektir.

### Roller

| Rol | Yetki | Nasıl elde edilir |
|-----|-------|-------------------|
| **Normal kullanıcı** | Konu yaz, yanıt ver, sohbet et | Kimlik oluştur |
| **Moderatör** | Herhangi bir konuyu sil, peer banla/banını kaldır | Kurucudan sertifika al (en fazla 7 gün geçerli; kalıcı da olabilir) |
| **Kurucu** | Yukarıdakilerin hepsi + moderatör ata/iptal et, kendi banını da kaldırabilir | Binary'ye gömülü public key'e sahip olmak |

Bir moderatör sertifikası iptal edilirse (`revoke`), o andan itibaren
hâlâ süresi dolmamış olsa bile geçersiz sayılır — hem ban atma hem ban
kaldırma yetkisini anında kaybeder.

### Ban süresi

Bir peer banlanırken süre esnektir: **1 saat**, **1 gün**, **7 gün**,
**30 gün** ya da **ömür boyu (kalıcı)**. Bir ban tek bir "kapsam"a
(chat/forum ayrı ayrı) bölünmez — ağ genelinde birleşik bir yasaktır ve
hem forum/chat mesajlarını hem de banlı peer'ın diğer düğümlere doğrudan
bağlanmasını (transport katmanında, `ConnectionGater` ile) engeller.

Bir ban hatalıysa ya da süresi dolmadan kaldırılması gerekiyorsa, banı
veren moderatörden bağımsız olarak **herhangi bir** geçerli moderatör ya
da kurucu banı kaldırabilir (unban).

### Uygulama içinden (TUI)

Ban/unban aksiyonları CLI'a inmeden, doğrudan uygulama içinden yapılabilir:

- **Profil ekranından** (forumda bir konudaki yazarın profiline gir, ya
  da sohbette `ctrl+p` ile bir mesajın yazarının profilini aç): kurucu
  veya geçerli sertifikalı bir moderatör iseniz `b` ile banlayabilir,
  hedef zaten banlıysa `u` ile banını kaldırabilirsiniz. Profilde ayrıca
  hedefin güncel ban durumu (⛔ Banlı, kalan süre/ömür boyu, sebep)
  görünür.
- **Yönetici Paneli eklentisi** (ana menüde yalnızca kurucuya/moderatöre
  görünen "Yönetici Paneli" girişi, bkz. [Eklentiler/admin/README.md](Eklentiler/admin/README.md)):
  Konular sekmesine ek olarak üç sekme —
  - *Banlar* — tüm aktif banları tek listede gösterir, `u` ile oradan
    da kaldırılabilir.
  - *Geçmiş* — kalıcı bir denetim kaydı: kim kimi ne zaman neden
    banladı/banını kaldırdı (süresi dolmuş/kaldırılmış kayıtlar dahil,
    hiç silinmez).
  - *Moderatörler* (yalnızca kurucu) — `g` ile yeni moderatör ata (süre
    seçenekleriyle), `r` ile mevcut birinin sertifikasını iptal et.

### andmod CLI

Uygulama açık değilken ya da otomasyon/betik içinden moderasyon yapmak
için:

```bash
# Kendi public key'ini yazdır
./andmod pubkey

# Moderatör yetkisi ver (7 gün — üst sınır)
./andmod grant <hedef_pubkey_hex> --days 7

# Süresiz moderatör yetkisi ver
./andmod grant <hedef_pubkey_hex> --permanent

# Moderatör olarak peer banla (esnek süre)
./andmod ban <peer_id> "sebep" --cert bans/mod_<kisa>.json --duration 1h
./andmod ban <peer_id> "sebep" --cert bans/mod_<kisa>.json --permanent

# Kurucu kimliğiyle doğrudan banla (sertifika gerekmez)
./andmod ban <peer_id> "sebep" --as-founder --duration 24h

# Bir banı geri al (moderatör sertifikasıyla ya da kurucu olarak)
./andmod unban <peer_id> --cert bans/mod_<kisa>.json
./andmod unban <peer_id> --as-founder

# Bir moderatörün yetkisini iptal et (yalnızca kurucu)
./andmod revoke <moderatör_pubkey_hex>
```

Oluşturulan `.json` dosyaları `%APPDATA%\and\bans\` dizinine yazılır.
AND yalnızca **açılışta bir kez** (5 saniye sonra) bu dizini tarayıp
ağda yayınlar — uygulama zaten çalışırken CLI ile bir işlem yaparsan,
AND yeniden başlatılana kadar etkisiz kalır. Anında etkili olması
gereken işlemler için TUI'nin Yönetici Paneli eklentisini tercih et.

---

## Eklenti Sistemi

AND eklentileri **bağımsız yürütülebilir binary**'lerdir. Her eklenti ayrı bir Go programı olarak derlenir ve AND'dan bağımsız dağıtılabilir.

### Nasıl çalışır

1. AND başlangıçta kendi dizinindeki tüm `and-plugin-*` dosyalarını tarar.
2. Her binary için yan yana bir `and-plugin-<isim>.json` manifest dosyası okunur.
3. Kullanıcı menüden eklenti açtığında AND, eklenti binary'sini şu ortam değişkenleriyle başlatır:

   | Değişken | Açıklama |
   |----------|----------|
   | `AND_API_ADDR` | AND'ın localhost HTTP API adresi (ör. `127.0.0.1:48291`) |
   | `AND_DATA_DIR` | AND veri dizini (taslak dosyaları için) |
   | `AND_CATEGORY` | (yalnızca konu_ac) Forumdan seçilen kategori |
   | `AND_DIL` | Etkin arayüz dili (`tr`, `en`, `de`, `es`, `fr`, `ru`; bkz. [Arayüz dili](#arayüz-dili)) |

4. Eklenti, `AND_API_ADDR` üzerinden AND'a HTTP istekleri göndererek kimlik, forum, DM ve dosya işlemlerini gerçekleştirir.
5. Eklenti TUI'sı kapandığında AND kontrolü geri alır.

> **İstisna:** `konu_ac` eklentisi binary başlatılmaz; AND kodu içinde doğrudan inline Bubbletea ekranı olarak açılır. Bu sayede geçiş anında gerçekleşir ve siyah ekran oluşmaz.

### Bağımsız harici araçlar (ör. VS Code eklentisi)

Yukarıdaki `AND_API_ADDR`/`AND_API_TOKEN` mekanizması yalnızca AND'ın **kendisinin başlattığı**
eklenti alt süreçlerine ulaşır — bağımsız başlatılmış bir süreç (ör. bir editör eklentisi) bu
ortam değişkenlerini hiçbir zaman göremez, çünkü rastgele port + rastgele token her açılışta
yalnızca bellekte tutulur ve hiçbir dosyaya yazılmaz.

Bunu güvenli bir şekilde çözmek için `AND_ENABLE_EXTERNAL_API=1` ortam değişkeniyle
etkinleştirilebilen, **opt-in ve varsayılan kapalı** ikinci bir dinleyici vardır
(`internal/pluginapi/externalpipe_windows.go` / `externalpipe_unix.go`):

- **Windows:** yalnızca dinleyiciyi açan kullanıcıya (owner) erişim izni veren bir security
  descriptor'lı (`D:P(A;;GA;;;OW)`) **named pipe**.
- **Linux/macOS:** `AND_DATA_DIR/api.sock` yolunda `0600` izinli bir **Unix domain socket**.

Bu ikinci dinleyicide bearer token **yoktur** — erişim bir sırla değil doğrudan işletim sistemi
tarafından (yalnızca aynı kullanıcı) kısıtlanır; pipe/socket adı `AND_DATA_DIR`'den türetilir ve
gizli değildir (bkz. `and-vscode/src/live/liveAddr.ts` — aynı formülle bağımsızca hesaplar).
`AND_ENABLE_EXTERNAL_API` ayarlanmadıysa bu dinleyici hiç açılmaz ve mevcut davranış hiçbir
şekilde değişmez. Kullanım örneği: [and-vscode](../and-vscode/README.md)'deki canlı Forum/DM/Sohbet/Ağ/Site paneli ve kişisel site sayfalarının VS Code editöründe düzenlenip yayınlanması.

İçerik yayınlayan/gönderen uç noktalar (forum, site, sohbet, DM, dosya) ayrıca yerel bir hız
sınırına tabidir (`internal/pluginapi/ratelimit.go`) — token'lı dinleyicideki HERHANGİ bir
eklenti ya da harici API'deki herhangi bir yerel süreç, düğümün kendi geçerli imzasıyla ağı
hızlıca doldurabilir; bu katman olmadan böyle bir hata/kötüye kullanım fark edilmeden ağa
yayılırdı. AND'ın kendi TUI'sini hiç etkilemez, yalnızca dakikada onlarca çağrıyı aşan
otomatik/hatalı bir kullanımda devreye girip `429 Too Many Requests` döner.

### Yerleşik eklentiler

| Binary | Menü Etiketi | Açıklama |
|--------|-------------|----------|
| `and-plugin-admin` | Yönetici Paneli | Konular/Banlar/Geçmiş sekmeleri (kurucu/moderatör) + Moderatörler sekmesi (yalnızca kurucu) — bkz. [Eklentiler/admin/README.md](Eklentiler/admin/README.md) |
| `and-plugin-konu-ac` | *(gizli)* | Yeni konu formu; forumdan `n` tuşuyla inline açılır |
| `and-plugin-ozel-chat` | Özel Chat | Doğrudan P2P mesajlaşma: konuşma listesi ve okunmamış sayıları, oturum geçmişi, dosya gönderme ve gelen dosya onayı (bkz. [Eklentiler/ozel_chat/README.md](Eklentiler/ozel_chat/README.md)) |
| `and-plugin-pembis` | Pembiş Tema | Arayüzü pembe tonlarda kız temasına boyar (bkz. [Eklentiler/Pembis/README.md](Eklentiler/Pembis/README.md)) |
| `and-plugin-DeltaX` | DeltaX | Özel gruplar + kendi sürüm kontrol sistemi; P2P uçtan uca şifreli senkronizasyon (bkz. [Eklentiler/DeltaX/README.md](Eklentiler/DeltaX/README.md)) |
| `and-plugin-site` | Site | Kendi çok sayfalı kişisel siteni oluştur/düzenle/sil — rol kısıtlaması yok (bkz. [Eklentiler/site/README.md](Eklentiler/site/README.md)) |

### Yeni eklenti yazmak

Ayrıntılı şablon ve API referansı için [Eklentiler/ornek/README.md](Eklentiler/ornek/README.md) dosyasına bak.

Kısa adımlar:

```bash
# 1. Eklentiler/<isim>/ dizini oluştur ve main.go yaz
mkdir Eklentiler/meklentim

# 2. Derle ve AND dizinine koy
go build -o and-plugin-meklentim ./Eklentiler/meklentim

# 3. Manifest JSON oluştur
./and-plugin-meklentim --manifest > and-plugin-meklentim.json

# AND bir sonraki başlatmada eklentiyi otomatik keşfeder
```

Eklentiler yalnızca istedikleri izinlerle çalışır: `plugin.json`'daki `permissions` alanı (ör.
`["dm", "file"]`) ilk açılışta kullanıcıya sorulur; herkese açık içeriği okumak izin istemez. İzin
listesi ve uç noktaları için [Eklentiler/ornek/README.md](Eklentiler/ornek/README.md#i̇zinler).

---

## Ekosistem

**AND Arama Motoru** (AND'ın yanındaki `AND arama motoru/` klasöründe, kendi deposunda duran ayrı bir sistem), AND'ın P2P ağına kendi kimliğiyle katılıp forum/sohbet içeriğini yerel bir SQLite FTS5 indeksinde arattıran bağımsız bir eş. AND'ın iç paketlerini kullanmaz, AND'ı bir alt-süreç olarak çalıştırmaz ve AND kapalıyken de çalışabilir — **aynı ağı** (aynı GossipSub konuları, aynı Ed25519 imza biçimi, aynı senkronizasyon protokolleri) konuşan iki ayrı program. Konu adları ve protokol kimlikleri iki programın da kullandığı `protokol/` paketinden gelir (arama motoru onu yan klasördeki AND'dan alır); AND'ın gerçek koduyla üretilen örnek iletiler (`protokol/testdata/`) arama motorunun testlerinde doğrulanır, böylece ileti biçimi bir tarafta değişirse testler kırılır.

Aynı aileden olduklarını gösteren ortak noktalar:
- Aynı ANSI 256 renk paleti (accent/hata/başarı/soluk tonları — bkz. `internal/tui/styles.go`'daki `colorAccent` vb.) ve aynı "AND" blok-harf logosu.
- Kimlik türetme şeması bire bir aynı (BIP-39 mnemonic → Ed25519), ama iki program birbirinden TAMAMEN bağımsız kimliklere sahiptir.
- Forum/sohbet/kişisel-site protokollerinin imza payload biçimleri ve GossipSub konu adları bire bir aynı — bu yüzden Arama Motoru, AND'ın ürettiği içeriği doğrulayıp indeksleyebilir. Bu, `protokol/testdata/`'daki örneklerle her test çalışmasında sınanır.

Ayrıntılar için arama motorunun kendi README'sine bak (`../AND arama motoru/README.md`).

---

## Yapılandırma

Tüm veriler `%APPDATA%\and\` (Windows) veya `~/.config/and/` (Linux/macOS) altında saklanır:

| Dosya / Dizin | Açıklama |
|---|---|
| `identity.dat` | AES-256-GCM şifreli Ed25519 kimlik (parola korumalı) |
| `founder.key` | Kurucunun Ed25519 public key'i (binary'e gömülüden türetilir) |
| `forum.db` | SQLite forum + sohbet veritabanı (WAL modu) |
| `bans/` | Moderasyon kararları (ban/unban/revoke/sertifika JSON'ları — `andmod` ve TUI'nin Moderatörler sekmesi tarafından yazılır) |
| `active_bans.json` | Şu an etkili banların anlık görüntüsü (yeniden başlatmada geri yüklenir) |
| `active_revocations.json` | İptal edilmiş moderatör sertifikalarının listesi |
| `mod_history.json` | Moderasyon denetim geçmişi (Yönetici Paneli eklentisinin "Geçmiş" sekmesi) |
| `granted_mods.json` | Bu düğümün (kurucuysa) verdiği moderatör sertifikalarının yerel defteri |
| `mod_rates.json` | Moderatör başına saatlik ban/unban/revoke hız sınırı sayaçları |
| `dosyalar/` | Alınan dosyalar |
| `bootstrap.txt` | Ek bootstrap node multiaddr'ları (isteğe bağlı) |
| `taslaklar_<kategori>.json` | Konu taslakları (konu_ac eklentisi, yerel — gitignored) |
| `theme.json` | Seçili tema (Ayarlar → Temalar ya da bir tema eklentisi yazar; yoksa Klasik kullanılır) |
| `dil.json` | Seçili arayüz dili (Ayarlar → Dil ya da giriş ekranında `ctrl+l`; yoksa Türkçe; `AND_DIL` ortam değişkeni geçersiz kılar) |
| `dm_gecmis.bin` | Özel mesaj geçmişi, giden kutusu ve okundu bilgisi; kimlikten türetilen anahtarla AES-256-GCM şifreli (`AND_DM_GECMISI=0` ile yalnızca bellekte tutulur) |
| `and.log` | Arayüz açıkken kütüphanelerin (libp2p, mDNS) yazdığı uyarı ve hatalar; tam ekran arayüzü bozmasınlar diye terminal yerine buraya gider. 5 MB'ı aşınca açılışta sıfırlanır |

> `identity.dat`, `founder.key` ve `bans/` içeriği asla başkalarıyla paylaşılmamalı, git'e eklenmemeli.

---

## Katkı

Katkıda bulunmak için [CONTRIBUTING.md](CONTRIBUTING.md) dosyasını oku.

---

## Güvenlik

Güvenlik açığı bildirmek için [SECURITY.md](SECURITY.md) dosyasını oku.  
Açıkları herkese açık issue olarak bildirme; doğrudan e-posta ile iletişime geç.

---

## Lisans

GNU Affero Genel Kamu Lisansı v3.0 (AGPL-3.0) — ayrıntılar için [LICENSE](LICENSE) dosyasına bak.
