# and-plugin-ozel_chat — Özel Chat

AND eşleriyle doğrudan (sunucusuz) mesajlaş, dosya gönder ve gelen dosya isteklerini onayla.

---

## Ekran

Tam ekran, AND'ın ortak tasarım sistemiyle (varsayılan Klasik teması ya da AND'da seçili tema):

- **Solda konuşmalar:** en son hareket eden üstte; okunmamış mesaj sayısı `● 3` olarak görünür.
- **Sağda mesaj akışı:** saat, ad (kendi mesajların vurgu renginde "sen") ve metin. Mesajlar
  yazma kutusunun hemen üstünde durur; uzun mesajlar metin sütununa hizalı sarılır.
- **Altta durum satırı:** bildirimler, bekleyen dosya istekleri ve o anki kısayollar.

Terminal 72 sütundan darsa tek bölmeye geçilir: listede liste, konuşmada akış gösterilir.

---

## Kısayollar

| Yer | Tuş | İş |
|-----|-----|----|
| Liste | `↑` `↓` | Konuşma seç |
| Liste | `enter` | Konuşmayı aç |
| Liste | `n` | Yeni konuşma |
| Liste | `esc` / `q` | Çık |
| Konuşma | `enter` | Mesajı gönder |
| Konuşma | `ctrl+f` | Dosya gönder (tam yol ya da sürükle-bırak) |
| Konuşma | `ctrl+n` | Yeni konuşma |
| Konuşma | `pgup` / `pgdown` | Eski mesajlara kaydır |
| Konuşma | `esc` / `tab` | Listeye dön |
| Yeni konuşma | `↑` `↓` `enter` | Bağlı AND düğümlerinden birini seç, ya da Peer ID yapıştırıp `enter` |
| Dosya isteği | `y` / `e` | Kabul et |
| Dosya isteği | `n` / `h` / `esc` | Reddet |

Bir dosya isteği tam sen mesaj yazarken gelirse `enter` onu **kabul etmez**; yalnızca `y`/`e`
kabul eder. Yanıtlanmayan istek 28 saniye sonra düşer ve AND tarafından reddedilir.

---

## Nasıl çalışır

- **Geçmiş:** Mesajlar `GET /api/v1/dm/history`'den okunur ve her saniye yenilenir. AND son 500
  mesajı (gelen ve giden) bu bilgisayarda şifreli saklar; Özel Chat kapalıyken ya da AND yeniden
  başlatıldıktan sonra da geçmiş yerindedir ve okunmamışlar işaretli görünür. Bu uç noktası
  olmayan eski bir AND'da eklenti `GET /api/v1/dm/poll` ile canlı dinlemeye geçer: o zaman
  yalnızca ekran açıkken gelenler görünür. DeltaX gibi eklentilerin DM üzerinden taşıdığı
  makine mesajları (`AND-PROJEAG/…`) gösterilmez ve okunmamış sayılmaz.
- **Çevrimdışı alıcı:** Karşı taraf o an bağlı değilse mesaj AND'ın giden kutusunda bekler
  ("bekliyor") ve o bağlandığında gider; bir hafta içinde bağlanmazsa "gönderilemedi" olur.
- **Okundu bilgisi:** AND'da tutulur (`/api/v1/dm/read`); ana menüdeki Özel Chat rozeti de aynı
  bilgiyle okunmamış mesaj sayısını gösterir. Eski bir AND'da eklentinin veri dizinindeki
  `ozel_chat_okundu.json` kullanılır.
- **Yeni konuşma:** Önerilen eşler yalnızca AND düğümleridir (`/api/v1/peers` yanıtında
  `"and": true`); bağlı eşlerin çoğu genel DHT düğümüdür ve özel mesaj alamaz. Peer ID elle
  girilirse biçimi (base58, `12D3KooW…` ya da `Qm…`) denetlenir.
- **Dosyalar:** Gönderim `POST /api/v1/file/send` ile yapılır; boyut sınırı AND'ınkiyle aynıdır.
  Gelen istekler `GET /api/v1/file/consent-poll` ile dinlenir. AND bir dosyayı ancak onayı
  gösterecek bir eklenti dinliyorsa kabul eder, yani dosya almak için bu ekran açık olmalıdır.

## Güvenlik

Mesaj gönderenden alıcıya doğrudan gider ve bağlantı iki uç arasında libp2p'nin Noise
protokolüyle şifrelidir; bağlantı bir röle üzerinden kurulsa bile röle yalnızca şifreli veri
görür. Gönderenin Peer ID'si bu el sıkışmada doğrulanır; ekrandaki ad karşı tarafın kendi
bildirdiği addır ve kontrol karakterlerinden arındırılarak gösterilir. Eklenti AND'dan yalnızca
`dm` ve `file` izinlerini ister. Ayrıntılar için [SECURITY.md](../../SECURITY.md).

---

## Derleme

```bash
# Eklentiler/ozel_chat/ altına, plugin.json yanına
go build -o Eklentiler/ozel_chat/and-plugin-ozel_chat ./Eklentiler/ozel_chat

# Windows
go build -o Eklentiler/ozel_chat/and-plugin-ozel_chat.exe ./Eklentiler/ozel_chat
```

Testler: `go test ./Eklentiler/ozel_chat/`
