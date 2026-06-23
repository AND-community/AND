# Katkı Rehberi

AND projesine katkıda bulunmak istediğin için teşekkürler.  
Bu belge; hata bildirme, özellik önerme ve kod katkısı süreçlerini açıklar.

---

## İçindekiler

- [Davranış Kuralları](#davranış-kuralları)
- [Başlamadan Önce](#başlamadan-önce)
- [Hata Bildirme](#hata-bildirme)
- [Özellik Önerme](#özellik-önerme)
- [Geliştirme Ortamı](#geliştirme-ortamı)
- [Kod Standartları](#kod-standartları)
- [Pull Request Süreci](#pull-request-süreci)
- [Eklenti Katkısı](#eklenti-katkısı)
- [Commit Mesajları](#commit-mesajları)

---

## Davranış Kuralları

Bu proje [Davranış Kuralları](CODE_OF_CONDUCT.md) kapsamındadır.  
Katkıda bulunarak bu kurallara uymayı kabul etmiş sayılırsın.

---

## Başlamadan Önce

- Aynı konuda açılmış bir issue veya PR var mı kontrol et.
- Büyük değişiklikler için kod yazmadan önce bir issue aç ve tartış.
- Güvenlik açıkları için [SECURITY.md](SECURITY.md) yönergelerini izle; herkese açık issue açma.

---

## Hata Bildirme

Hata bildirirken şunları belirt:

1. **AND versiyonu** — `./and --version` çıktısı veya derleme tarihi
2. **İşletim sistemi ve mimarisi** — örn. Windows 11 amd64, Ubuntu 24.04 arm64
3. **Hatayı yeniden oluşturma adımları** — sırasıyla ve net biçimde
4. **Beklenen davranış** — ne olması gerekiyordu
5. **Gerçekleşen davranış** — ne oldu; varsa hata mesajı veya panic çıktısı
6. **Standart hata çıktısı** — `./and` başlatılırken `stderr`'de görünen mesajlar

Kimlik bilgilerini (`identity.dat`, 12 kelimelik anımsatıcı, `founder.key`) **asla** paylaşma.

---

## Özellik Önerme

Yeni özellik önermeden önce:

- AND'ın P2P ve sunucusuz yapısıyla uyumlu mu değerlendir.
- Önerinin mevcut güvenlik modelini (Ed25519 imzalama, ban sistemi) zayıflatıp zayıflatmadığını düşün.
- Bir issue aç; motivasyonu, önerilen arayüzü ve alternatifler varsa açıkla.

Küçük iyileştirmeler (hata düzeltme, dokümantasyon) için doğrudan PR açılabilir.

---

## Geliştirme Ortamı

### Gereksinimler

- Go 1.21 veya üzeri
- Git

### Kurulum

```bash
git clone https://github.com/lucian95511/and.git
cd and
go mod download
go build ./...
go test ./...
```

### Derleme

```bash
# Tüm paketleri derle (ana binary + eklentiler)
go build ./...

# Çalıştırılabilir binary'leri oluştur
go build -o and          ./cmd/and
go build -o andmod       ./cmd/andmod
go build -o and-plugin-admin      ./Eklentiler/admin
go build -o and-plugin-moderator  ./Eklentiler/moderator
go build -o and-plugin-konu-ac    ./Eklentiler/konu_ac
go build -o and-plugin-ozel-chat  ./Eklentiler/ozel_chat
```

### Test

```bash
# Tüm testleri çalıştır
go test ./...

# Belirli bir paketi test et
go test ./internal/pluginapi/...
go test ./internal/dmmgr/...
go test ./internal/pluginmgr/...

# Statik analiz
go vet ./...
```

### Proje yapısı

```
cmd/
  and/              — ana uygulama
  andmod/           — moderasyon CLI
  plugin_admin/     — yönetici paneli binary'si
  plugin_moderator/ — moderatör paneli binary'si
  plugin_konu_ac/   — konu açma binary'si
  plugin_ozel_chat/ — özel mesaj binary'si

internal/
  crypto/           — BIP-39, Ed25519, kimlik depolama
  network/          — libp2p, GossipSub, DHT
  forum/            — konu yönetimi, imza, TTL
  moderation/       — sertifika, ban, ConnectionGater
  storage/          — SQLite WAL
  pluginapi/        — eklenti HTTP IPC sunucusu + istemci
  pluginmgr/        — binary keşif ve başlatma
  dmmgr/            — DM akış yönetimi
  tui/              — Bubbletea TUI
  updater/          — otomatik güncelleme
```

Genel bağımlılık kuralları:

- `internal/` paketleri yalnızca `cmd/` ve diğer `internal/` paketleri tarafından kullanılır.
- `pluginapi` paketi eklenti binary'leri tarafından da (`cmd/plugin_*`) kullanılır.
- Döngüsel bağımlılık yasaktır.

---

## Kod Standartları

### Biçimlendirme

- `gofmt` veya `goimports` ile biçimlendir; CI bunu zorunlu kılar.
- `go vet` sıfır uyarı vermelidir.

### Yorum

- Yorum yalnızca **neden** sorusuna cevap veriyorsa yaz; **ne** yapıldığını iyi isimlendirilmiş kod zaten anlatır.
- Hata mesajları küçük harfle başlar, noktalama işareti ile bitmez.

### Güvenlik

- Kullanıcıdan gelen tüm veriler sisteme girmeden doğrulanmalıdır.
- İmza doğrulamasını hiçbir yerde atlama.
- Yeni kriptografik ilkel kullanmadan önce mevcut Ed25519 altyapısının yeterli olup olmadığını değerlendir.
- Hata durumlarını sessizce yutma; döndür veya logla.

### Eşzamanlılık

- Paylaşılan state'e her erişimde uygun mutex kullan.
- Her goroutine'nin nasıl kapandığı net olmalı; goroutine sızıntısı yaratma.
- Context iptalini dinleyen select bloğu ekle.

### Test

- Yeni fonksiyonlar için birim testi yaz.
- Tablo tabanlı testleri (`table-driven tests`) tercih et.
- Test isimleri `Test<FonksiyonAdı>_<senaryo>` biçiminde olmalı.
- Geçici dosyalar için `t.TempDir()` kullan.
- Ağ veya libp2p bağımlılığı gerektiren testler için gerçek düğüm yerine mock backend kullan.

---

## Pull Request Süreci

### 1. Fork yap ve branch oluştur

```bash
git checkout -b ozellik/kisa-aciklama
```

Branch adları:

| Önek | Kullanım |
|------|----------|
| `ozellik/` | Yeni özellik |
| `duzelt/` | Hata düzeltme |
| `refactor/` | Yeniden yapılandırma |
| `dokuman/` | Yalnızca belge değişikliği |
| `guvenlik/` | Güvenlik düzeltmesi |

### 2. Değişikliklerini yap

- Kod standartlarına uy.
- İlgili testleri ekle veya güncelle.
- `go build ./...` ve `go test ./...` başarıyla geçmeli.
- Eklenti değişikliği içeriyorsa ilgili `README.md`'yi güncelle.

### 3. PR aç

- Başlık kısa ve açıklayıcı olsun (70 karakter altı).
- Açıklamada: ne değişti, neden değişti, nasıl test edildi.
- İlgili issue varsa `Closes #123` ile bağla.

### 4. İnceleme süreci

- En az bir onay gerekir.
- İstenen değişiklikleri yap; tartışmalı noktalarda gerekçeni açıkla.
- CI geçmelidir.

### 5. Merge

Squash merge tercih edilir; temiz commit geçmişi tutulur.

---

## Eklenti Katkısı

### Yeni bir eklenti eklemek

AND eklentileri bağımsız binary'lerdir. Yeni bir eklenti için:

1. `Eklentiler/<isim>/main.go` dosyasını oluştur.
2. `--manifest` davranışını ve `pluginapi.NewClientFromEnv()` kullanımını uygula.
3. Binary adı `and-plugin-<isim>[.exe]` kuralına uymalıdır.
4. Eklenti README'si için `Eklentiler/<isim>/README.md` oluştur.

Detaylı şablon ve API referansı: [Eklentiler/ornek/README.md](Eklentiler/ornek/README.md)

### Mevcut eklentileri değiştirmek

- `Eklentiler/admin/`, `Eklentiler/moderator/`, `Eklentiler/konu_ac/`, `Eklentiler/ozel_chat/` içindeki `main.go` dosyalarını düzenle.
- Manifest'teki `version` alanını semantik versiyonlama ile güncelle.
- İlgili `Eklentiler/<isim>/README.md`'yi güncelle.

---

## Commit Mesajları

```
<tür>: <kısa özet> (70 karakter altı)

<isteğe bağlı gövde: neden, ne değişti, bağlam>
```

### Tür tablosu

| Tür | Ne zaman |
|-----|----------|
| `feat` | Yeni özellik |
| `fix` | Hata düzeltme |
| `refactor` | Davranış değiştirmeyen yeniden yapılandırma |
| `test` | Test ekle veya düzelt |
| `docs` | Yalnızca dokümantasyon |
| `chore` | Bağımlılık güncelleme, araç yapılandırması |
| `security` | Güvenlik düzeltmesi |

### Örnekler

```
feat: and-plugin-ozel-chat long-poll ile gelen DM desteği

fix: pluginmgr — and-plugin- öneki olmayan dosyaları atla

security: DM paketi 16 KB ile sınırlandırıldı

docs: eklenti geliştirme rehberi yeni binary sistemi için güncellendi
```

---

Katkın için şimdiden teşekkürler.
