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
- `identity.dat` dosyası AES-256-GCM ile şifrelenir; şifre çözme anahtarı Argon2id ile (3 geçiş, 64 MB bellek, 4 iş parçacığı) kullanıcı parolasından türetilir. Parametreler dosya başlığında saklanır, böylece ileride artırılabilirler.
- Görünen takma ad (`AuthorName`) doğrulanmaz; imzalı `AuthorKey` güvenilir tanımlayıcıdır.

### Ağ güvenliği

- libp2p Noise protokolü ile transport katmanı şifrelemesi sağlanır.
- Banlanan peer'lar birden çok katmanda engellenir: bağlantı düzeyinde
  (`ConnectionGater` — banlı bir peer ile YENİ bir libp2p bağlantısı hiç
  kurulmaz), canlı GossipSub mesajlarında (yeni forum/sohbet içeriği
  reddedilir), senkronizasyon (backfill) akışında (`SyncWithPeer` — banlı
  bir yazarın geçmiş içeriği de senkron sırasında kabul edilmez) ve Özel
  Chat/dosya aktarımı akışlarında (`dmmgr`/`filemgr` — ban öncesi zaten
  kurulmuş bir bağlantı üzerinden gelen DM'ler/dosya istekleri de,
  bağlantı gater'ından bağımsız olarak, alıcı tarafında reddedilir;
  dosya aktarımında kullanıcıya onay isteği bile gösterilmez).
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
- Ağdan gelen her akış boyutla sınırlıdır: GossipSub iletileri 64 KB, senkron istek ve yanıtları, DM'ler ve
  dosya aktarımı çerçeveleri ayrı ayrı sınırlıdır; geçmiş senkronizasyonu aynı anda en fazla 4 eşle
  yapılır ve yalnızca AND protokolünü konuşan eşlerle akış açılır (`internal/peersync`).

### Dosya aktarımı

- Bir dosya ancak kullanıcı onaylarsa alınır; onayı gösterecek bir eklenti çalışmıyorsa aktarım reddedilir.
- Gönderenin bildirdiği dosya adı hem onay penceresinde gösterilmeden hem diske yazılmadan önce temizlenir:
  yalnızca son yol bileşeni kalır, kontrol ve yazı yönü karakterleri (ör. uzantıyı gizleyen U+202E) atılır,
  Windows'ta geçersiz karakterler (`:` dahil — NTFS alternatif veri akışı) değiştirilir, sondaki nokta ve
  boşluklar silinir, aygıt adları (`CON`, `NUL`, `COM1`…) öneklenir.
- Parça düzeni doğrulanır: parça boyutu ve sayısı bildirilen toplam boyutla tutarlı olmalıdır; hiçbir parça
  onaylanan boyutun ötesine yazılamaz ve dosya ancak bütün parçalar gelip boyutu tuttuğunda tamamlanır.
- Yarım kalmış bir aktarım yalnızca aynı gönderen tarafından, aynı ad ve boyutla sürdürülebilir — başka bir
  eş, bir başkasının yarım aktarımını kendi verisiyle tamamlayamaz.

### Terminal güvenliği

- Ağdan gelen her metin (konu başlıkları, gövdeler, adlar, eklenti çıktıları) ekrana basılmadan önce
  `internal/pluginui.Safe` ile temizlenir: ANSI/OSC kaçış dizileri (pano yazma, bağlantı yönlendirme,
  imleç oynatma), C0/C1 kontrol karakterleri ve yazı yönü karakterleri atılır.
- Kütüphanelerin günlükleri (libp2p, mDNS) tam ekran arayüz açıkken terminale değil veri dizinindeki
  `and.log`'a yazılır.

### Otomatik güncelleme

- Güncellemeler yalnızca HTTPS üzerinden ve GitHub'ın alan adlarından indirilir; indirilen dosya sürümün
  `SHA256SUMS.txt`'sine karşı doğrulanır, doğrulama yapılamazsa güncelleme uygulanmaz.
- Derlemeye bir sürüm imzalama anahtarı gömülürse (`-X …/updater.UpdateSigningKeyHex=…`), özet dosyasının
  Ed25519 imzası (`SHA256SUMS.txt.sig`) da doğrulanır. İmza olmadan, sürüme yazabilen biri binary'yi ve özet
  dosyasını birlikte değiştirebilirdi. Sürüm iş akışı bunu kendisi yapar: genel anahtar depoda
  `.github/update-signing-pubkey.hex` dosyasında durur (gizli değildir, herkes denetleyebilir), özel anahtar
  `UPDATE_SIGNING_KEY` sırrındadır. Genel anahtar dosyası varken sır eksikse ya da eşleşmiyorsa sürüm
  yayınlanmaz; böylece imzasız, dolayısıyla güncellenemeyen bir sürüm çıkmaz. Kurulum komutları
  `.github/workflows/release.yml`'deki "Özet dosyasını imzala" adımındadır.
- AND güncellenince aynı sürümle yayınlanan eklentiler de güncellenir; her eklenti dosyası ve manifest'i
  aynı (imzası doğrulanmış) `SHA256SUMS.txt`'ye karşı denetlenir. Sürümde dosyası olmayan eklentilere
  (ör. üçüncü taraf) dokunulmaz; doğrulanamayan bir eklenti eski haliyle kalır. Yeni sürüm yeni bir izin
  isterse kullanıcıya bir sonraki açılışta yeniden sorulur.
- Sürüm imzalama genel anahtarı (Ed25519, hex):
  `40f36782755018ac90475013594691b41886813ab842148d537b4f44fddd89a4`. Bir sürümün
  `SHA256SUMS.txt.sig` dosyasını bu anahtarla kendin de doğrulayabilirsin. Anahtar değişirse bu
  satır ve `.github/update-signing-pubkey.hex` birlikte değişir; biri değişip öteki değişmediyse
  güvenme.

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
- **Eklenti izinleri:** Her eklenti başlatılırken kendi anahtarını alır; anahtar yalnızca eklentinin
  `plugin.json`'da istediği ve kullanıcının ilk açılışta onayladığı izinleri taşır, eklenti kapanınca
  geçersizleşir. İzinler: özel mesajlar (`dm`), dosya aktarımı (`file`), genel sohbete yazma (`chat`),
  forumda konu ve yanıt (`forum`), kişisel site (`site`), moderasyon (`moderation`), tema (`theme`).
  Herkese açık içeriği okumak (kimlik, eşler, forum, siteler, genel sohbet) izin istemez. Verilmemiş
  bir izni gerektiren istek `403` ile reddedilir. AND'ın kendi tam yetkili anahtarı hiçbir eklentiye
  verilmez. Onaylar `plugins_state.json`'da durur ve Ayarlar › Eklentiler'de `r` ile geri alınır;
  eklenti bir güncellemede yeni bir izin isterse yeniden sorulur. Her uç noktanın hangi izni
  gerektirdiği `routeTable()`'da yazılıdır ve bir test her uç noktanın bilinçli bir izin taşıdığını denetler.
- Anahtar aramaları ham anahtarın değil SHA-256 özetinin üzerinden yapılır; sunucuların başlık okuma
  süresi ve başlık boyutu sınırlıdır, istek gövdeleri her uç noktada boyutla sınırlıdır ve yazma
  işlemleri hız sınırına tabidir.
- Güven modeli: harici dinleyici için aynı işletim sistemi kullanıcısı = güvenilir (VS Code eklentisi
  AND'ın kendi arayüzüyle eşdeğer erişime sahiptir). AND'ın başlattığı eklentiler ise yalnızca
  onaylanan izinlerle sınırlıdır. Farklı kullanıcı hesaplarının erişemeyeceği yerel iletişimdir; ağa
  açık değildir.

### Yerel depolama

- `identity.dat`, `founder.key`, `bans/` içindeki dosyalar `0o600` izniyle oluşturulur (yalnızca sahibi okuyabilir).
- SQLite veritabanı şifrelenmeden yerel dosya sisteminde saklanır; cihaz güvenliği kullanıcının sorumluluğundadır.
- `taslaklar_*.json` dosyaları düz metin; hassas içerik taslak olarak kaydedilmemelidir.

### Bilinen sınırlamalar

- **İçerik kalıcılığı:** Ağda yayılan içerik geri alınamaz; silme mesajları yalnızca TTL dolmadan önce çalışır ve tüm peer'ların uygulamasına bağlıdır.
- **Bootstrap güveni:** DHT bootstrap node'ları kimlik doğrulaması yapılmadan kullanılır; Sybil saldırısına karşı ek koruma yoktur.
- **DHT sansürü (GO-2024-3218):** go-libp2p-kad-dht'de düzeltmesi olmayan bilinen bir açık, saldırganın DHT'deki sağlayıcı kayıtlarını bastırarak eş keşfini zorlaştırmasına izin verir. İçeriğin kendisi etkilenmez (her ileti imzalıdır); en kötü durumda bazı eşler birbirini DHT üzerinden bulamaz. mDNS ile yerel ağdaki eşler ve zaten bağlı olanlar etkilenmez. govulncheck'in bildirdiği tek açıktır; düzeltme yayınlanınca güncellenecek.
- **DM şifrelemesi:** Özel mesajlar gönderenden alıcıya doğrudan bir libp2p akışıyla gider ve bağlantı iki uç arasında Noise ile şifrelenir: bağlantı bir röle üzerinden kurulsa bile röle yalnızca şifreli veri görür, alıcının kimliği el sıkışmada doğrulanır. Açık metin yalnızca iki uçtaki AND süreçlerinde ve `dm` izni verilmiş eklentilerde bulunur. Kimin kiminle ne zaman konuştuğu (üst veri) röleden ve ağı izleyen birinden gizlenmez.
- **DM geçmişi:** AND son 500 özel mesajı (eklentilerin makine mesajları ayrıca en fazla 300), giden kutusunu ve konuşma başına okundu bilgisini `dm_gecmis.bin`'de saklar. Dosya, kimliğin özel anahtarından HKDF ile türetilen bir anahtarla AES-256-GCM şifrelidir; kimlik dosyası parolayla korunduğundan geçmiş de ancak AND'a giriş yapılabildiğinde okunur. Başka bir kimlikle açılan dosyanın üzerine yazılmaz, `.okunamadi` uzantısıyla kenara alınır. `AND_DM_GECMISI=0` ile geçmiş yalnızca bellekte tutulur. Geçmişi eklenti API'si (`/api/v1/dm/history`) üzerinden yalnızca `dm` izni onaylanmış eklentiler (ve `AND_ENABLE_EXTERNAL_API=1` iken harici süreçler) okuyabilir.
- **Giden kutusu:** Ulaşılamayan bir alıcıya gönderilen özel mesaj (eklenti isterse) şifreli geçmiş dosyasında bekler ve alıcı bağlandığında gönderilir; bir haftada gidemezse "gönderilemedi" olarak işaretlenir.
- **Tek kurucu anahtarı:** Tüm moderasyon yetkisi tek bir kurucu Ed25519 anahtarına dayanır — çoklu imza, yedek kurucu ya da anahtar döndürme (rotation) mekanizması yoktur. Kurucunun `identity.dat`'ı kaybolur/çalınırsa, moderasyon yetkisi kalıcı olarak devredilemez hale gelir; bu, `identity.dat`'ın (ve varsa yedeklerinin) güvenliğini kritik önemli kılar.
- **Moderatör sertifikası dağıtımı ve keşfi:** Bir sertifika, kurucunun onu ilgili kişiye elle (ör. Özel Chat ile) iletmesini gerektirir; ağda "şu anki moderatörler kimler" sorusuna cevap verecek yayınlanmış bir kayıt yoktur (bkz. Yönetici Paneli eklentisinin "Moderatörler" sekmesi — bu yalnızca YEREL, o düğümün kendi verdiği sertifikaların defteridir).
- **Yerel denetim geçmişi tahrif edilebilir:** `mod_history.json` imza doğrulamalı değildir, yalnızca yerel bir kayıt kolaylığıdır; diskine erişimi olan biri bu dosyayı düzenleyebilir/silebilir. Gerçek ban/unban kararlarının kendisi (ağa yayılan `BanMsg`/`UnbanMsg`) her zaman imzalıdır ve bundan etkilenmez.

---

## Teşekkür

Güvenli bir AND topluluğu inşa etmeye katkıda bulunan araştırmacılara teşekkür ederiz.  
Sorumlu açıklama ilkesine uyan tüm güvenlik araştırmacıları projenin SECURITY.md dosyasında adlarıyla anılır.
