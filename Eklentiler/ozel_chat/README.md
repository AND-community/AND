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

- **Geçmiş:** Mesajlar `GET /api/v1/dm/history`'den okunur ve her saniye yenilenir. AND, oturum
  boyunca gelen ve giden mesajları (en fazla 500) bellekte tutar; Özel Chat kapalıyken gelen
  mesajlar da açılınca okunmamış olarak görünür. Bu uç noktası olmayan eski bir AND'da eklenti
  `GET /api/v1/dm/poll` ile canlı dinlemeye geçer: o zaman yalnızca ekran açıkken gelenler görünür.
- **Okundu bilgisi:** Hangi konuşmayı en son ne zaman okuduğun eklentinin veri dizinindeki
  `ozel_chat_okundu.json` dosyasına yazılır. Mesaj içeriği diske yazılmaz.
- **Yeni konuşma:** Önerilen eşler yalnızca AND düğümleridir (`/api/v1/peers` yanıtında
  `"and": true`); bağlı eşlerin çoğu genel DHT düğümüdür ve özel mesaj alamaz. Peer ID elle
  girilirse biçimi (base58, `12D3KooW…` ya da `Qm…`) denetlenir.
- **Dosyalar:** Gönderim `POST /api/v1/file/send` ile yapılır; boyut sınırı AND'ınkiyle aynıdır.
  Gelen istekler `GET /api/v1/file/consent-poll` ile dinlenir. AND bir dosyayı ancak onayı
  gösterecek bir eklenti dinliyorsa kabul eder, yani dosya almak için bu ekran açık olmalıdır.

## Güvenlik

Bağlantı libp2p'nin Noise protokolüyle şifrelidir; **uçtan uca şifreleme değildir**. Gönderenin
Peer ID'si AND tarafından doğrulanır; ekrandaki ad karşı tarafın kendi bildirdiği addır ve
kontrol karakterlerinden arındırılarak gösterilir. Ayrıntılar için [SECURITY.md](../../SECURITY.md).

---

## Derleme

```bash
# Eklentiler/ozel_chat/ altına, plugin.json yanına
go build -o Eklentiler/ozel_chat/and-plugin-ozel_chat ./Eklentiler/ozel_chat

# Windows
go build -o Eklentiler/ozel_chat/and-plugin-ozel_chat.exe ./Eklentiler/ozel_chat
```

Testler: `go test ./Eklentiler/ozel_chat/`
