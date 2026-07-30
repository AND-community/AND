# Güvenlik Politikası

## Desteklenen Sürümler

Güvenlik düzeltmeleri yalnızca en güncel kararlı sürüm için yayınlanır.

| Sürüm | Destek Durumu |
|-------|---------------|
| En son `main` | Aktif destek |
| Önceki sürümler | Destek yok |

---

## Güvenlik Açığı Bildirme

AND'da bir güvenlik açığı keşfettiysen lütfen **herkese açık GitHub issue açma**.  
Açık bildirilen güvenlik açıkları düzeltme yayınlanmadan önce kötüye kullanılabilir.

### Nasıl bildirirsin

Güvenlik açıklarını doğrudan aşağıdaki adrese gönder:

**E-posta:** `umut95511@gmail.com`  
**Konu satırı:** `[AND Security] <kısa açıklama>`

### Bildirimde neler olmalı

Raporunu şu başlıklar altında düzenle:

1. **Açığın türü** — ör. imza doğrulama atlaması, path traversal, IPC yetkisiz erişim
2. **Etkilenen bileşen** — hangi paket veya binary (ör. `internal/pluginapi`, `cmd/plugin_admin`)
3. **Sürüm bilgisi** — `./and --version` çıktısı
4. **Yeniden oluşturma adımları** — sırasıyla ve net biçimde
5. **Olası etkisi** — ne tür bir saldırıya kapı açabileceğini açıkla
6. **Varsa öneri** — düzeltme veya hafifletme önerisi

### Yanıt süreci

| Adım | Hedef süre |
|------|------------|
| İlk onay (alındı bildirimi) | 3 iş günü |
| Ön değerlendirme | 7 iş günü |
| Düzeltme geliştirme | Önem derecesine göre 14–30 gün |
| Kamuoyu bildirimi | Düzeltme yayınlandıktan sonra |

Düzeltme yayınlandığında, izin verirsen katkıcılar listesinde adını belirtiriz.

---

## Kapsam Dışı

Aşağıdaki durumlar bu politika kapsamında **değildir**:

- AND'a bağlı olmayan üçüncü taraf kütüphanelerdeki açıklar — ilgili projeye bildir
- Ağ altyapısı saldırıları (DoS, BGP manipülasyonu) — AND sunucusuz bir uygulamadır
- Kullanıcının kendi cihazında kendi `identity.dat` dosyasına erişmesi
- Sosyal mühendislik — kullanıcının kendi onayıyla paylaştığı 12 kelimelik anımsatıcı
- Eklenti binary'lerini ve AND binary'sini aynı dizine koyma güveni (aynı kullanıcı erişimi varsayılır)

---

## Güvenlik Modeli

AND'ın güvenlik tasarımını anlayarak değerlendirme yapman için temel bileşenler:

### Kimlik ve imzalama

- Her kullanıcı kimliği BIP-39 anımsatıcısından (128 bit entropi, 12 kelime) türetilen Ed25519 anahtar çiftidir.
- Tüm forum konuları, yanıtları, moderasyon kararları ve onay mesajları Ed25519 ile imzalanır.
- `identity.dat` dosyası AES-256-GCM ile şifrelenir; şifre çözme anahtarı PBKDF2 ile kullanıcı parolasından türetilir.
- Görünen takma ad (`AuthorName`) doğrulanmaz; imzalı `AuthorKey` güvenilir tanımlayıcıdır.

### Ağ güvenliği

- libp2p Noise protokolü ile transport katmanı şifrelemesi sağlanır.
- Banlanan peer'lar `ConnectionGater` arayüzü üzerinden bağlantı düzeyinde engellenir.
- Moderatör sertifikaları kurucu tarafından Ed25519 ile imzalanır; sertifikasız onay mesajları ağda geçersiz sayılır.
- GossipSub mesajları imzasız da yayılabilir; önem taşıyan kararlar (onay, ban) her zaman imzalıdır.

### Eklenti IPC güvenlik sınırları

- AND, eklenti API sunucusunu yalnızca `127.0.0.1` (localhost) üzerinde rastgele bir portta başlatır.
- `AND_API_ADDR` ortam değişkeni yalnızca AND tarafından başlatılan alt süreçlere iletilir.
- API, kimlik doğrulama içermez; güven modeli: aynı işletim sistemi kullanıcısı = güvenilir.
- Farklı kullanıcı hesaplarının erişemeyeceği yerel iletişim; ağa açık değildir.

### Yerel depolama

- `identity.dat`, `founder.key`, `bans/` içindeki dosyalar `0o600` izniyle oluşturulur (yalnızca sahibi okuyabilir).
- SQLite veritabanı şifrelenmeden yerel dosya sisteminde saklanır; cihaz güvenliği kullanıcının sorumluluğundadır.
- `taslaklar_*.json` dosyaları düz metin; hassas içerik taslak olarak kaydedilmemelidir.

### Bilinen sınırlamalar

- **İçerik kalıcılığı:** Ağda yayılan içerik geri alınamaz; silme mesajları yalnızca TTL dolmadan önce çalışır ve tüm peer'ların uygulamasına bağlıdır.
- **Bootstrap güveni:** DHT bootstrap node'ları kimlik doğrulaması yapılmadan kullanılır; Sybil saldırısına karşı ek koruma yoktur.
- **DM şifrelemesi:** Özel mesajlar yalnızca Noise transport şifrelemesiyle korunur; uçtan uca şifreleme yoktur.

---

## Teşekkür

Güvenli bir AND topluluğu inşa etmeye katkıda bulunan araştırmacılara teşekkür ederiz.  
Sorumlu açıklama ilkesine uyan tüm güvenlik araştırmacıları projenin SECURITY.md dosyasında adlarıyla anılır.
