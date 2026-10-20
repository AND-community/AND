# Güvenlik Politikası

## Desteklenen Sürümler

Güvenlik düzeltmeleri yalnızca en güncel kararlı sürüm için yayınlanır.

| Sürüm | Destek Durumu |
|-------|---------------|
| En son `main` | Aktif destek |
| Önceki sürümler | Destek yok |

---

## Güvenlik Açığı Bildirme

AND Arama Motoru'nda bir güvenlik açığı keşfettiysen lütfen **herkese açık GitHub issue açma**.
Açık bildirilen güvenlik açıkları düzeltme yayınlanmadan önce kötüye kullanılabilir.

### Nasıl bildirirsin

Güvenlik açıklarını doğrudan aşağıdaki adrese gönder:

**E-posta:** `umut95511@gmail.com`
**Konu satırı:** `[AND Arama Motoru Security] <kısa açıklama>`

### Bildirimde neler olmalı

Raporunu şu başlıklar altında düzenle:

1. **Açığın türü** — ör. imza doğrulama atlaması, sorgu enjeksiyonu, kaynak tükenmesi (DoS)
2. **Etkilenen dosya/fonksiyon** — ör. `search.go`'daki ağ protokolü işleyicisi
3. **Sürüm bilgisi** — `./and-arama-motoru -version` çıktısı
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

- Bu programa bağlı olmayan üçüncü taraf kütüphanelerdeki açıklar — ilgili projeye bildir (bkz. aşağıdaki "Bilinen sınırlamalar"daki DHT istisnası)
- AND'ın kendi kaynak kodundaki açıklar — bunları [AND'ın SECURITY.md](../AND/SECURITY.md) dosyasına bildir
- Ağ altyapısı saldırıları (DoS, BGP manipülasyonu) — bu program sunucusuz bir P2P eştir
- Kullanıcının kendi cihazında kendi `kimlik.json` dosyasına erişmesi
- Sosyal mühendislik — kullanıcının kendi onayıyla paylaştığı 12 kelimelik anımsatıcı ya da site paylaşım kodu

---

## Güvenlik Modeli

Bu programın güvenlik tasarımını anlayarak değerlendirme yapman için temel bileşenler:

### İçerik doğrulama

- Bu program **yalnızca okuyucu bir eştir**: post/mesaj/site sayfası oluşturmaz, moderasyon zincirini (founder/moderatör sertifikaları, ban/unban, `mod_delete`) uygulamaz.
- Ağdan alınan HER post/yanıt/sohbet mesajı/site sayfası, AND'ın kendisiyle **bire bir aynı ed25519 imza payload biçimiyle** doğrulanır (`forum.go`/`chat.go`/`site.go`) — imzasız ya da kurcalanmış hiçbir şey indekslenmez.
- İmza doğrulama mantığı AND'ın kaynak kodundan elle transkript edilmiştir (Go'nun paket görünürlük kuralları import etmeyi engellediği için) — bu, AND'ın protokolü değiştiğinde bu programın da elle güncellenmesi gerektiği anlamına gelir; testler kendi içindeki tutarlılığı (imzala→doğrula, kurcalama tespiti) kanıtlar.

### Ağ güvenliği

- libp2p Noise protokolü ile transport katmanı şifrelemesi sağlanır.
- Kendi P2P arama protokolü (`/and-arama-motoru/1.0.0`) yalnızca bu programın başka bir kopyasını çalıştıran eşlerle konuşur; AND'ın kendisi bu protokolü konuşmaz ve etkilenmez.
- "Ağda ara" sorguları en fazla `aramaVarsayilanTTL` (varsayılan 3) atlama öteye ulaşır — sınırsız yayılma ve sorgu döngüsü, paylaşılan bir sorgu ID önbelleğiyle (`sorguOnbellek`) engellenir.
- Ağdan gelen içerikteki bir ayrıştırma/işleme hatası tüm programı çökertmez: TUI'nin `Update` döngüsü ve tüm arka plan goroutine'leri bir panik kalkanıyla (`recoverGoroutine`) sarılıdır.
- Sonuçlardaki bağlantılar yalnızca `http(s)://` şemasıyla tıklanabilir/açılabilir kılınır — `file://`, `javascript:` gibi şemalar kasıtlı olarak reddedilir (içerik güvenilmeyen eşlerden geldiği için önemli bir sınır).

### Yerel depolama

- `kimlik.json` ve `bilinen_peerler.json`, yarım/bozuk bırakılabilecek doğrudan bir yazma yerine geçici dosya + atomik yeniden adlandırma ile yazılır.
- SQLite indeks dosyası (`arama_motoru.db`) şifrelenmeden yerel dosya sisteminde saklanır; cihaz güvenliği kullanıcının sorumluluğundadır. Bu dosya yalnızca ağdan toplanan, zaten P2P'de herkese açık yayınlanmış içeriği tutar — kullanıcıya özel gizli bir veri değildir.
- Site paylaşım kodu (`yazarAnahtarı/slug`) yalnızca YEREL indekste arama yapar; ağa çıkmaz.

### Bilinen sınırlamalar

- **Kademlia DHT keşfi bilinen bir zayıflığa açıktır** ([GO-2024-3218](https://pkg.go.dev/vuln/GO-2024-3218) / CVE-2023-26248): kötü niyetli düğümler DHT'yi manipüle ederek belirli içeriğin bulunmasını zorlaştırabilir ("içerik sansürü"). Bu, `go-libp2p-kad-dht` kütüphanesinin protokol seviyesinde bir sınırıdır ve henüz bir düzeltmesi yoktur; bu programa özgü değildir, AND'ın kendisi de aynı kütüphaneyi kullanır. Etkisi kısmen azaltılır: keşif yalnızca DHT'ye bağlı değildir, mDNS ve bilinen-eş listesi de bağımsız keşif yolları sağlar.
- **Bootstrap güveni:** DHT bootstrap node'ları kimlik doğrulaması yapılmadan kullanılır; Sybil saldırısına karşı ek koruma yoktur.
- **NAT/relay/hole-punching desteği AND'ınkinden daha sınırlıdır** — sıkı bir NAT arkasındaysan gelen "ağda ara" sorgularına ulaşılabilirlik AND kadar güvenilir olmayabilir.
- **İçerik kalıcılığı:** Bir kez indekslenen içerik, kaynağı silinmiş olsa bile bu programın yerel indeksinde kalabilir — bu program AND'ın tombstone/silme mesajlarını da doğrulayıp uygular, ama ağdaki tüm eşlerin bu mesajı görmüş olmasına bağlıdır.

---

## Teşekkür

Güvenli bir AND ekosistemi inşa etmeye katkıda bulunan araştırmacılara teşekkür ederiz.
Sorumlu açıklama ilkesine uyan tüm güvenlik araştırmacıları bu dosyada adlarıyla anılır.
