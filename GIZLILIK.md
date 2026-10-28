# Gizlilik Bildirimi

**Sürüm 2** · 29 Ekim 2026

Bu bildirim, AND'ın hangi verileri işlediğini, nerede sakladığını ve kimlerin görebileceğini
anlatır. AND bir yazılımdır: ağı işleten, kullanıcı verilerini toplayan ya da saklayan merkezi
bir kuruluş yoktur. Bildirimin özeti uygulamada **AND hakkında › Gizlilik** sekmesindedir ve
ilk açılışta [Kullanım Koşulları](KULLANIM_KOSULLARI.md) ile birlikte onaya sunulur.

---

## Kısaca

- Hesap, e-posta ya da telefon numarası istenmez. Kimliğin bu cihazda üretilen bir anahtar
  çiftidir ve şifrenle korunur.
- Forum konuları, yanıtlar, genel sohbet mesajları, profilin ve kişisel site sayfaların
  **herkese açıktır**: eşlere yayılır ve onların cihazlarında da saklanır.
- Forum konuları 5 gün, sohbet mesajları 48 saat sonra kendiliğinden silinir. Yayılmış bir
  içeriğin her eşten silineceği garanti edilemez.
- Bağlandığın eşler IP adresini görebilir; bu, eşten eşe iletişimin doğal bir sonucudur.
- Özel mesajlar iki uç arasında şifrelenir; kimin kiminle ne zaman konuştuğu ise ağdan tamamen
  gizlenemez.
- AND kullanım istatistiği (telemetri) toplamaz.

---

## 1. Verilerin denetimi kimde?

AND'ın geliştiricileri kullanıcılara ait verileri toplamaz, görmez ve hiçbir sunucuda saklamaz.
Veriler yalnızca AND çalıştıran cihazlarda bulunur: kendi cihazındakilerin denetimi sende,
eşlerin cihazlarındaki kopyaların denetimi o eşlerdedir. Ağa yayımladığın bir içerik, onu alan
her eşin cihazına kopyalanır.

## 2. Hangi veriler, nerede ve ne kadar süre?

| Veri | Nerede | Kimler görebilir | Ne kadar süre |
|---|---|---|---|
| Kimlik (Ed25519 anahtar çifti) | `identity.dat`; şifrenden Argon2id ile türetilen anahtarla AES-256-GCM şifreli | Yalnızca sen. Açık anahtarın ve ondan türeyen Peer ID'n herkese açıktır | Sen silene kadar |
| Görünen ad ve profil (biyografi, durum, bağlantı, etiketler) | Senin ve eşlerin cihazları | Ağdaki herkes | Sen değiştirene kadar; eski sürümler eşlerde kalmış olabilir |
| GitHub kullanıcı adı (yalnızca doğrulama yaparsan) | Profilinde | Ağdaki herkes | Profilinde kaldıkça |
| Forum konuları ve yanıtları | `forum.db` (senin ve eşlerin cihazları); şifresiz | Ağdaki herkes | 5 gün; moderatörler daha önce silebilir |
| Genel sohbet mesajları | `forum.db`; imzalı ama şifresiz | Ağdaki herkes | 48 saat |
| Kişisel site sayfaları | Senin ve eşlerin cihazları | Ağdaki herkes | Sen silene kadar |
| Özel mesajlar | `dm_gecmis.bin`; kimliğinden türetilen anahtarla AES-256-GCM şifreli | Sen, alıcı ve `dm` izni verdiğin eklentiler | Son 500 mesaj; `AND_DM_GECMISI=0` ile yalnızca bellekte |
| Gönderilen ve alınan dosyalar | Alıcının `dosyalar/` dizini | Gönderen ve alıcı | Alıcı silene kadar |
| Moderasyon kararları (ban, sertifika) | `bans/`, `active_bans.json` ve eşler | Kararlar ağda herkese açıktır | Kararın süresince; denetim geçmişi yerel olarak tutulur |
| Ayarlar (tema, dil, eklenti izinleri, kurallar onayı) | `theme.json`, `dil.json`, `plugins_state.json`, `kurallar_onayi.json` | Yalnızca sen | Sen silene kadar |
| Veritabanı yedekleri | `yedek/` (yalnızca bu cihaz); `forum.db`'nin kopyaları, şifresiz | Yalnızca sen | Günlük yedekler en çok 3 gün, sürüm yükseltmesinden önce alınanlar en çok 7 gün; silinen ya da süresi dolan içerik bu sürede yedekte kalabilir |
| Gönderilmemiş forum yanıtları | `yanit_taslaklari.json`; şifresiz | Yalnızca sen | Yanıt gönderilene ya da 30 gün geçene kadar |
| Kimlik özeti | `kimlik_ozeti.json`: açık anahtarın ve görünen adın (ikisi de zaten herkese açık); kurtarma kodunu yanlış yazarsan uyarmak için | Yalnızca sen | Sen silene kadar |
| Okunamayan ya da yerine yenisi yazılan dosyaların kopyaları | `*.bozuk-<tarih>`, `dm_gecmis.bin.okunamadi-<tarih>`, `identity.dat.eski-<tarih>`; özgün dosya kadar korumalı | Yalnızca sen | AND silmez; sen silene kadar |
| Uygulama günlüğü | `and.log` | Yalnızca sen | 5 MB'ı aşınca `and.log.1`'e döner |

Veri dizini: Windows'ta `%APPDATA%\and`, Linux'ta `~/.config/and`, macOS'ta
`~/Library/Application Support/and`. Dizindeki dosyaların bir bölümü (ör. `forum.db`) şifresizdir;
cihazının güvenliği sana bağlıdır.

## 3. Ağda kimlerle iletişim kurulur?

- **Eşler:** Diğer AND kullanıcıları ve libp2p düğümleri. Bağlandığın her eş IP adresini ve Peer
  ID'ni görür. Bağlantılar Noise protokolüyle şifrelidir.
- **Herkese açık IPFS/libp2p başlangıç düğümleri ve DHT:** Eş bulmak için kullanılır; Peer ID'n
  ve ulaşılabilir adreslerin DHT'de yayımlanır.
- **Röle düğümleri:** NAT arkasındaysan bağlantın bir röle üzerinden kurulabilir. Röle, iki ucun
  adresini ve bağlantının varlığını görür; içerik şifreli kaldığı için okuyamaz.
- **GitHub:** Güncelleme denetimi için GitHub'ın sürüm API'sine bağlanılır; GitHub IP adresini ve
  AND sürümünü görür. GitHub hesabını doğrularsan GitHub'ın OAuth akışı kullanılır.
- **Telemetri yoktur:** AND kullanım istatistiği, hata raporu ya da benzeri bir veriyi hiçbir
  yere göndermez.

## 4. Ne herkese açık, ne gizli?

- Forum, genel sohbet, profil ve kişisel site içerikleri Ed25519 ile **imzalanır**, ama
  **şifrelenmez**: ağdaki herkes okuyabilir ve kimin yazdığını doğrulayabilir.
- Özel mesajlar ve dosya aktarımları iki uç arasında doğrudan, şifreli bir bağlantıyla gider.
  Açık metin yalnızca iki uçtaki AND süreçlerinde ve `dm` izni verdiğin eklentilerde bulunur.
- Kimin kiminle ne zaman konuştuğu (üst veri) röleden ve ağı izleyen birinden tamamen gizlenemez.

## 5. Silme

- Bu cihazdaki bütün AND verilerini silmek için AND'ı kapatıp veri dizinini sil. Kimlik dosyasını
  silersen kurtarma kodun olmadan kimliğini geri getiremezsin.
- Forum konuları 5 gün, sohbet mesajları 48 saat sonra uyumlu bütün eşlerde kendiliğinden silinir.
- Ağa yayılmış bir içeriği geri çekmek her eşte garanti edilemez: silme istekleri eşlerin
  uygulamasına bağlıdır ve değiştirilmiş istemciler kopyaları saklayabilir. Yayımlamadan önce düşün.

## 6. Eklentiler

Eklentiler AND'dan ayrı programlardır. Her eklenti herkese açık içeriği (forum, siteler, genel
sohbet, kimliğinin açık kısmı) izinsiz okuyabilir; özel mesaj, dosya, moderasyon gibi işlemler
için ilk açılışta senden izin ister. İzinleri **Ayarlar › Eklentiler**'den geri alabilirsin.
Üçüncü taraf eklentilerin verileri nasıl işlediğinden o eklentilerin yazarları sorumludur.

## 7. Reşit olmayanlar

AND yaş doğrulaması yapmaz. İçerikler herkese açık bir toplulukta paylaşıldığı için reşit
olmayanların AND'ı veli gözetiminde kullanması önerilir.

## 8. Güvenlik

Güvenlik modeli ve bilinen sınırlamalar için [SECURITY.md](SECURITY.md)'ye bak. Güvenlik
açıklarını herkese açık bir issue yerine oradaki adrese bildir.

## 9. Değişiklikler

Bu bildirimde kullanıcıyı etkileyen bir değişiklik olursa sürüm numarası artırılır; AND yeni
sürümü bir sonraki açılışta yeniden onaya sunar.

- **Sürüm 2** (29 Ekim 2026): bu cihazda tutulan veritabanı yedekleri, gönderilmemiş forum yanıtları,
  kimlik özeti ve okunamadığı için kenara alınan dosyaların kopyaları eklendi (bkz. 2. bölüm).

## 10. İletişim

Gizlilikle ilgili soruların için depoda bir issue açabilirsin. Kişisel veri içeren konuları
herkese açık yazma; güvenlikle ilgili olanlar için [SECURITY.md](SECURITY.md)'deki adresi kullan.
