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
- Banlanan peer'lar üç katmanda engellenir: bağlantı düzeyinde
  (`ConnectionGater` — banlı bir peer ile libp2p bağlantısı hiç kurulmaz),
  canlı GossipSub mesajlarında (yeni forum/sohbet içeriği reddedilir) ve
  senkronizasyon (backfill) akışında (`SyncWithPeer` — banlı bir yazarın
  geçmiş içeriği de senkron sırasında kabul edilmez).
- Moderatör sertifikaları kurucu tarafından Ed25519 ile imzalanır;
  sertifikasız onay mesajları (moderatör yolu) ağda geçersiz sayılır.
  Kurucunun kendisi için ayrı, sertifikasız bir doğrudan-imza yolu vardır
  (ban/unban) — kendi anahtarına karşı doğrudan doğrulanır, farklı bir
  imza payload'u kullanır, bu yüzden bir moderatör mesajı kurucu mesajı
  gibi yeniden oynatılamaz (ve tam tersi).
- Bir moderatör sertifikası iptal edildiğinde (`revoke`), sertifikanın
  imzalandığı an iptal anından ÖNCEYSE artık hiçbir işlemde (ban, unban)
  kabul edilmez — süresi henüz dolmamış olsa bile. İptalden SONRA aynı
  kişiye yeniden verilen bir sertifika bundan etkilenmez.
- Ban/unban/iptal/moderatör-atama gibi kendi düğümünden kaynaklanan
  (`Issue*`) her işlem, ağa yayınlanmadan önce kendi kendine kurucunun
  gerçek anahtarına karşı doğrulanır — yanlış bir kimlikle çağrılırsa
  hiçbir yerde geçerli olmayacak bir mesaj sessizce "başarılı" diye
  bildirilmez, hemen hata döner.
- GossipSub mesajları imzasız da yayılabilir; önem taşıyan kararlar (konu, yanıt, ban, unban, sertifika iptali) her zaman imzalıdır.

### Eklenti IPC güvenlik sınırları

- AND, eklenti API sunucusunu yalnızca `127.0.0.1` (localhost) üzerinde rastgele bir portta başlatır.
- `AND_API_ADDR`/`AND_API_TOKEN` ortam değişkenleri yalnızca AND tarafından başlatılan alt
  süreçlere iletilir; bu ana dinleyiciye yapılan her istek `X-And-Token` başlığıyla doğrulanır —
  doğru token'ı bilmeyen bir istek reddedilir.
- **İkinci, opt-in bir dinleyici** (`AND_ENABLE_EXTERNAL_API=1` ile açılır; bkz.
  `internal/pluginapi/externalpipe_windows.go` / `externalpipe_unix.go`) AND'ın kendi başlatmadığı
  süreçlerin (ör. `and-vscode` uzantısı) bağlanabilmesi için vardır. Bu dinleyicide bearer token
  YOKTUR — erişim kararını bir sır değil doğrudan işletim sistemi verir: Windows'ta sahibe özel bir
  security descriptor'lı (`D:P(A;;GA;;;OW)`) named pipe, Unix'te `0600` izinli bir domain socket.
  Yalnızca bu dinleyicide açık şekilde işaretlenmiş (`allowExternal:true`) route'lara erişilebilir —
  varsayılan kapalıdır (bkz. `internal/pluginapi/api.go`'daki `routeTable()`).
- Güven modeli: her iki dinleyici için de aynı işletim sistemi kullanıcısı = güvenilir. Farklı
  kullanıcı hesaplarının erişemeyeceği yerel iletişimdir; ağa açık değildir.

### Yerel depolama

- `identity.dat`, `founder.key`, `bans/` içindeki dosyalar `0o600` izniyle oluşturulur (yalnızca sahibi okuyabilir).
- SQLite veritabanı şifrelenmeden yerel dosya sisteminde saklanır; cihaz güvenliği kullanıcının sorumluluğundadır.
- `taslaklar_*.json` dosyaları düz metin; hassas içerik taslak olarak kaydedilmemelidir.

### Bilinen sınırlamalar

- **İçerik kalıcılığı:** Ağda yayılan içerik geri alınamaz; silme mesajları yalnızca TTL dolmadan önce çalışır ve tüm peer'ların uygulamasına bağlıdır.
- **Bootstrap güveni:** DHT bootstrap node'ları kimlik doğrulaması yapılmadan kullanılır; Sybil saldırısına karşı ek koruma yoktur.
- **DM şifrelemesi:** Özel mesajlar yalnızca Noise transport şifrelemesiyle korunur; uçtan uca şifreleme yoktur.
- **Tek kurucu anahtarı:** Tüm moderasyon yetkisi tek bir kurucu Ed25519 anahtarına dayanır — çoklu imza, yedek kurucu ya da anahtar döndürme (rotation) mekanizması yoktur. Kurucunun `identity.dat`'ı kaybolur/çalınırsa, moderasyon yetkisi kalıcı olarak devredilemez hale gelir; bu, `identity.dat`'ın (ve varsa yedeklerinin) güvenliğini kritik önemli kılar.
- **Moderatör sertifikası dağıtımı ve keşfi:** Bir sertifika, kurucunun onu ilgili kişiye elle (ör. Özel Chat ile) iletmesini gerektirir; ağda "şu anki moderatörler kimler" sorusuna cevap verecek yayınlanmış bir kayıt yoktur (bkz. Moderasyon Paneli'nin "Moderatörler" sekmesi — bu yalnızca YEREL, o düğümün kendi verdiği sertifikaların defteridir).
- **Yerel denetim geçmişi tahrif edilebilir:** `mod_history.json` imza doğrulamalı değildir, yalnızca yerel bir kayıt kolaylığıdır; diskine erişimi olan biri bu dosyayı düzenleyebilir/silebilir. Gerçek ban/unban kararlarının kendisi (ağa yayılan `BanMsg`/`UnbanMsg`) her zaman imzalıdır ve bundan etkilenmez.

---

## Teşekkür

Güvenli bir AND topluluğu inşa etmeye katkıda bulunan araştırmacılara teşekkür ederiz.  
Sorumlu açıklama ilkesine uyan tüm güvenlik araştırmacıları projenin SECURITY.md dosyasında adlarıyla anılır.
