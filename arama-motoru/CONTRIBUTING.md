# Katkı Rehberi

AND Arama Motoru'na katkıda bulunmak istediğin için teşekkürler.
Bu belge; hata bildirme, özellik önerme ve kod katkısı süreçlerini açıklar.

---

## İçindekiler

- [Başlamadan Önce](#başlamadan-önce)
- [Hata Bildirme](#hata-bildirme)
- [Özellik Önerme](#özellik-önerme)
- [Geliştirme Ortamı](#geliştirme-ortamı)
- [Kod Standartları](#kod-standartları)
- [Pull Request Süreci](#pull-request-süreci)
- [Commit Mesajları](#commit-mesajları)

---

## Başlamadan Önce

- Aynı konuda açılmış bir issue veya PR var mı kontrol et.
- Büyük değişiklikler için kod yazmadan önce bir issue aç ve tartış.
- Güvenlik açıkları için [SECURITY.md](SECURITY.md) yönergelerini izle; herkese açık issue açma.
- Bu program AND'ın Go modülünün **dışında**, ayrı bir modüldür (`and-arama-motoru`) — AND'ın `internal/...` paketlerini import edemez, protokolleri (imza biçimi, GossipSub konuları, senkron akışı) kasıtlı olarak bağımsız yeniden uygular. AND'ın protokollerinde bir değişiklik varsa bu proje de elle güncellenmelidir; otomatik senkron yoktur.

---

## Hata Bildirme

Hata bildirirken şunları belirt:

1. **Sürüm** — `./and-arama-motoru -version` çıktısı
2. **İşletim sistemi ve mimarisi** — örn. Windows 11 amd64, Ubuntu 24.04 arm64
3. **Hatayı yeniden oluşturma adımları** — sırasıyla ve net biçimde
4. **Beklenen davranış** — ne olması gerekiyordu
5. **Gerçekleşen davranış** — ne oldu; varsa hata mesajı veya panic çıktısı
6. **Tanılama günlüğü** — `~/.and-arama-motoru/gunluk.log`'un ilgili kısmı (`-log-level bilgi` ile yeniden üret)

Kimlik bilgilerini (`kimlik.json`, 12 kelimelik anımsatıcı) **asla** paylaşma.

---

## Özellik Önerme

Yeni özellik önermeden önce:

- Bu programın **yalnızca okuyucu bir eş** olma sınırıyla (bkz. README'deki [Kapsam ve sınırlar](README.md#kapsam-ve-sınırlar)) uyumlu mu değerlendir — içerik oluşturma/moderasyon bu projenin kapsamı dışındadır, bunlar için AND'ın kendisine katkıda bulun.
- Önerinin AND ile protokol uyumluluğunu bozup bozmadığını düşün (imza payload biçimi, GossipSub konu adları, senkron protokolleri AND ile bire bir aynı kalmalı).
- Bir issue aç; motivasyonu, önerilen arayüzü ve alternatifler varsa açıkla.

Küçük iyileştirmeler (hata düzeltme, dokümantasyon) için doğrudan PR açılabilir.

---

## Geliştirme Ortamı

### Gereksinimler

- Go 1.26 veya üzeri
- Git

### Kurulum

```bash
git clone https://github.com/AND-community/and-arama-motoru.git
cd and-arama-motoru
go mod download
go build .
go test ./...
```

### Derleme

```bash
go build -o and-arama-motoru .

# Windows
go build -o and-arama-motoru.exe .

# Sürüm bilgisi gömerek
go build -ldflags "-X main.Version=v0.2.0 -X main.BuildCommit=$(git rev-parse --short HEAD)" -o and-arama-motoru .
```

### Test

```bash
# Tüm testleri çalıştır
go test ./...

# Statik analiz
go vet ./...

# Belirli bir dosyayla ilgili testler
go test -run TestArama ./...
```

### Proje yapısı

Bu, tek bir Go modülü içinde düz (flat) dosyalardan oluşan bağımsız bir programdır — AND'ın aksine `cmd/`/`internal/` alt paket ayrımı yoktur:

```
identity.go       — BIP39/Ed25519 kimlik türetme ve saklama
network.go        — libp2p host, DHT/mDNS eş keşfi
forum.go/chat.go/site.go
                  — AND protokollerinin bağımsız yeniden uygulaması (imza
                    doğrulama, senkron, gossip abonelikleri)
index.go          — SQLite FTS5 indeks, sıralama, sayfalama, istatistikler
search.go         — kendi çok atlamalı P2P arama protokolü
model.go/view.go  — Bubbletea TUI durum makinesi ve render
cli.go            — betiklenebilir -search/-site komut satırı modu
peers.go          — bilinen eş listesi kalıcılığı
log.go            — seviyeli, boyut sınırlı tanılama günlüğü
links.go          — tıklanabilir bağlantı tespiti (OSC 8)
fsutil.go         — atomik dosya yazımı
tema.go           — çalışma zamanı renk teması
yazimonerisi.go   — Levenshtein tabanlı "şunu mu demek istediniz?"
main.go           — bayrak ayrıştırma, yaşam döngüsü, sıralı kapanış
```

Genel kural: bu proje AND'ın kaynak kodunu **import etmez** (Go paket görünürlük kuralları buna izin vermez) ve AND'ı alt-süreç olarak da **çalıştırmaz** — protokol uyumluluğu tamamen elle senkronize tutulan bağımsız kodla sağlanır.

---

## Kod Standartları

### Biçimlendirme

- `gofmt` veya `goimports` ile biçimlendir.
- `go vet` sıfır uyarı vermelidir.

### Yorum

- Yorum yalnızca **neden** sorusuna cevap veriyorsa yaz; **ne** yapıldığını iyi isimlendirilmiş kod zaten anlatır.
- AND'ın bir protokolünü/davranışını yeniden uyguluyorsan, hangi AND dosyasının/fonksiyonunun karşılığı olduğunu yorumda belirt (bkz. mevcut kod tabanındaki örnekler) — bu, iki proje arasındaki uyumu gelecekte denetlemeyi kolaylaştırır.

### Güvenlik

- Ağdan gelen HİÇBİR veri (post, mesaj, site sayfası, arama sonucu) imza doğrulanmadan indekslenmemeli/görüntülenmemeli.
- İmza payload biçimini değiştirirken AND'ın kendi kaynak kodundaki karşılığıyla bayt bayt eşleştiğinden emin ol — aksi halde bu program AND ağıyla protokol uyumluluğunu kaybeder.
- Ağdan gelen içerikteki bir ayrıştırma/işleme hatası tüm programı çökertmemeli — yeni arka plan goroutine'leri panik kalkanıyla (`recoverGoroutine`) sarılmalı.
- Yalnızca `http(s)://` şemalı bağlantılar tıklanabilir/açılabilir kılınmalı; başka şemalar (`file://`, `javascript:` vb.) reddedilmeli.

### Eşzamanlılık

- Paylaşılan state'e her erişimde uygun mutex kullan.
- Her goroutine'nin nasıl kapandığı net olmalı; goroutine sızıntısı yaratma.
- Context iptalini dinleyen select bloğu ekle.

### Test

- Yeni fonksiyonlar için birim testi yaz.
- Tablo tabanlı testleri (`table-driven tests`) tercih et.
- Test isimleri `Test<FonksiyonAdı>_<senaryo>` biçiminde olmalı.
- Geçici dosyalar için `t.TempDir()` kullan.
- Ağ/libp2p bağımlılığı gerektiren testler için gerçek düğüm yerine mock kullan; gerçek iki-düğümlü uçtan-uca testler yalnızca protokol uyumluluğunu doğrulamak gerektiğinde yazılmalı.

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
- `go build .` ve `go test ./...` başarıyla geçmeli.
- README'de belgelenen bir davranışı değiştiriyorsan README'yi de güncelle.

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
feat: -yazar bayrağına CLI'da sayfalama desteği ekle

fix: ftsSorguOlustur — tırnak içindeki kaçış karakterini düzelt

security: site sayfası paylaşım kodu yalnızca yerel indekste aranır hale getirildi

docs: gelişmiş sorgu sözdizimi örnekleri güncellendi
```

---

Katkın için şimdiden teşekkürler.
