# AND

**AND** — sunucusuz, eşten eşe (P2P) terminal forum topluluğu.  
Hesap yok, kayıt yok, merkezi sunucu yok. Kimliğin 12 kelimeden ibarettir.

---

## İçindekiler

- [Genel Bakış](#genel-bakış)
- [Özellikler](#özellikler)
- [Mimari](#mimari)
- [Kurulum](#kurulum)
- [Kullanım](#kullanım)
- [Moderasyon](#moderasyon)
- [Eklenti Sistemi](#eklenti-sistemi)
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
| BIP-39 kimlik | 12 kelimelik anımsatıcı → Ed25519 anahtar → libp2p kimliği |
| Ed25519 imzalı içerik | Her konu, yanıt ve moderasyon kararı imzalanır |
| SQLite yerel depo | WAL modunda yüksek eşzamanlılık, şema versiyonlama |
| Forum kategorileri | Python, Rust, Go, Siber Güvenlik, Yapay Zeka ve daha fazlası |
| Özel mesajlaşma | libp2p stream (`/and/dm/1.0.0`) üzerinden P2P doğrudan mesaj |
| Dosya aktarımı | Yeniden başlatılabilir parçalı aktarım (`/and/file/2.0.0`); onay isteği dialog'u |
| Moderasyon sistemi | Kurucu sertifikası + devredilebilir moderatör sertifikaları |
| Dinamik eklenti sistemi | Bağımsız binary'ler; AND'ı yeniden derlemeden eklenti ekle |
| Otomatik güncelleme | GitHub Releases üzerinden arka planda güncelleme |
| Terminal arayüzü | [Bubbletea](https://github.com/charmbracelet/bubbletea) tabanlı TUI |

---

## Mimari

```
cmd/
  and/              — ana uygulama giriş noktası
  andmod/           — moderasyon CLI aracı (andmod grant, ban, trusted)

Eklentiler/
  admin/            — yönetici paneli eklentisi (bağımsız binary)
  moderator/        — moderatör paneli eklentisi (bağımsız binary)
  konu_ac/          — konu açma (menüde gizli; AND içinde inline açılır)
  ozel_chat/        — özel mesajlaşma + dosya aktarımı eklentisi
  ornek/            — yeni eklenti geliştirme rehberi

internal/
  crypto/           — BIP-39 → Ed25519 kimlik, AES-GCM şifreli depolama
  network/          — libp2p düğümü, GossipSub, DHT keşfi, sync protokolü
  forum/            — konu/yanıt yönetimi, imza doğrulama, TTL
  moderation/       — kurucu/moderatör sertifikası, ban sistemi, ConnectionGater
  storage/          — SQLite WAL, şema versiyonlama
  pluginapi/        — eklenti HTTP IPC API sunucusu + istemci kütüphanesi
  pluginmgr/        — and-plugin-* binary keşfi ve başlatma
  dmmgr/            — libp2p DM stream handler + ozel_chat'e long-poll proxy
  filemgr/          — parçalı dosya gönderme/alma; onay (consent) mekanizması
  tui/              — Bubbletea TUI (menü, forum, konu açma inline, sohbet, ayarlar)
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
                                                         ↓  long-poll
                                              and-plugin-ozel-chat (görünür)
```

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
go build -o and-plugin-moderator  ./Eklentiler/moderator
go build -o and-plugin-konu-ac    ./Eklentiler/konu_ac
go build -o and-plugin-ozel-chat  ./Eklentiler/ozel_chat
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

---

## Kullanım

### Dizin yapısı (çalıştırma)

Tüm binary'leri aynı dizine koy:

```
/herhangi/bir/dizin/
  and[.exe]
  andmod[.exe]
  and-plugin-admin[.exe]
  and-plugin-moderator[.exe]
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
3. **12 kelimelik anımsatıcını güvenli bir yere yaz** — tek kurtarma yolun budur

Sonraki açılışlarda yalnızca parolanı girmen yeterlidir.  
Kimlik dosyası: `%APPDATA%\and\identity.dat` (Linux/macOS: `~/.config/and/identity.dat`)

### Ana menü

```
AND — ana menü

  [1] Forum              forum gözat, konu oku
  [2] Sohbet             genel P2P sohbet kanalı
  [3] Yönetici Paneli    onay bekleyen konular (kurucu)
  [4] Moderatör Paneli   onay bekleyen konular (moderatör)
  [5] Özel Chat          P2P doğrudan mesaj ve dosya aktarımı
  [6] Ayarlar            yüklü eklentiler ve sürüm bilgisi

  enter  seç    q/esc  çıkış
```

> Yüklü eklenti binary'leri (`and-plugin-*`) otomatik keşfedilir ve listeye eklenir.  
> `Label: ""` olan eklentiler (örn. `konu_ac`) menüde görünmez; doğrudan forumdan açılır.

### Ayarlar ekranı

AND sürümünü ve tüm yüklü eklentilerin adını, sürümünü, durumunu ve açıklamasını gösterir.  
`esc` ile ana menüye dönülür.

### Klavye kısayolları — forum

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | konu seç |
| `enter` | konuyu aç |
| `n` | yeni konu aç (inline konu_ac ekranı) |
| `tab` | kategori değiştir |
| `esc` | ana menüye dön |

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

AND'ın moderasyon sistemi kurucu merkezli ve sertifika tabanlıdır.

### Roller

| Rol | Yetki | Nasıl elde edilir |
|-----|-------|-------------------|
| **Normal kullanıcı** | Konu yaz, yanıt ver | Kimlik oluştur |
| **Moderatör** | Konu onayla/reddet | Kurucudan `.json` sertifika al |
| **Kurucu** | Moderatör ata, toplu onayla | Binary'ye gömülü public key'e sahip olmak |

### andmod CLI

```bash
# Kendi public key'ini yazdır
./andmod pubkey

# Moderatör yetkisi ver (30 gün)
./andmod grant <hedef_pubkey_hex> --days 30

# Süresiz moderatör yetkisi ver
./andmod grant <hedef_pubkey_hex> --permanent

# Moderatör olarak peer banla (30 gün)
./andmod ban <peer_id> "sebep" --cert bans/mod_<kisa>.json --days 30

# Yazarı güvenilir olarak işaretle (90 gün; onay beklemiyor)
./andmod trusted <yazar_pubkey_hex> --days 90
```

Oluşturulan `.json` dosyaları `%APPDATA%\and\bans\` dizinine yazılır.  
AND yeniden başlatıldığında bu dosyaları otomatik olarak ağda yayınlar.

### Onay akışı

Varsayılan olarak tüm konular moderasyon kuyruğuna girer ve 5 günlük TTL'e tabidir:

```
Yeni konu  →  Beklemede (5 gün TTL)
                    ↓
       Kurucu veya moderatör  →  Ed25519 imzalı ApprovalMsg
                                            ↓
                                 GossipSub'da tüm ağa yayılır
                                            ↓
                                    Konusu kalıcı hale gelir
```

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

4. Eklenti, `AND_API_ADDR` üzerinden AND'a HTTP istekleri göndererek kimlik, forum, DM ve dosya işlemlerini gerçekleştirir.
5. Eklenti TUI'sı kapandığında AND kontrolü geri alır.

> **İstisna:** `konu_ac` eklentisi binary başlatılmaz; AND kodu içinde doğrudan inline Bubbletea ekranı olarak açılır. Bu sayede geçiş anında gerçekleşir ve siyah ekran oluşmaz.

### Yerleşik eklentiler

| Binary | Menü Etiketi | Açıklama |
|--------|-------------|----------|
| `and-plugin-admin` | Yönetici Paneli | Onay kuyruğu (kurucu: onayla / reddet / toplu onayla) |
| `and-plugin-moderator` | Moderatör Paneli | Onay kuyruğu (sertifikalı moderatör) |
| `and-plugin-konu-ac` | *(gizli)* | Yeni konu formu; forumdan `n` tuşuyla inline açılır |
| `and-plugin-ozel-chat` | Özel Chat | Peer ID ile doğrudan P2P mesajlaşma ve dosya aktarımı |

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

---

## Yapılandırma

Tüm veriler `%APPDATA%\and\` (Windows) veya `~/.config/and/` (Linux/macOS) altında saklanır:

| Dosya / Dizin | Açıklama |
|---|---|
| `identity.dat` | AES-256-GCM şifreli Ed25519 kimlik (parola korumalı) |
| `founder.key` | Kurucunun Ed25519 public key'i (binary'e gömülüden türetilir) |
| `forum.db` | SQLite forum veritabanı (WAL modu) |
| `bans/` | Moderasyon kararları (ban, sertifika, onay JSON'ları) |
| `dosyalar/` | Alınan dosyalar |
| `bootstrap.txt` | Ek bootstrap node multiaddr'ları (isteğe bağlı) |
| `taslaklar_<kategori>.json` | Konu taslakları (konu_ac eklentisi, yerel — gitignored) |

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

MIT Lisansı — ayrıntılar için [LICENSE](LICENSE) dosyasına bak.
