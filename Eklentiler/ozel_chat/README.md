# and-plugin-ozel-chat — Özel Mesajlaşma

Başka bir AND kullanıcısıyla doğrudan ve özel mesajlaş; dosya gönder.

---

## Genel Bakış

`and-plugin-ozel-chat` binary'si, libp2p stream protokolü (`/and/dm/1.0.0`) üzerinden iki peer arasında doğrudan mesajlaşma ve dosya aktarımı sağlar.

Mesajlar GossipSub üzerinden yayılmaz; yalnızca hedef peer'a iletilir.  
AND, `/and/dm/1.0.0` akışını ve `/and/file/2.0.0` dosya protokolünü dinler; eklenti bu olayları long-poll HTTP ile alır.

---

## Kurulum

```bash
go build -o and-plugin-ozel-chat ./Eklentiler/ozel_chat

# Windows
go build -o and-plugin-ozel-chat.exe ./Eklentiler/ozel_chat
```

---

## Kullanım

Ana menüden **Özel Chat**'i seç.

### Adımlar

1. **Peer ID girişi** — hedef kişinin libp2p Peer ID'sini gir (ör. `12D3KooWAbc…`)
2. `enter` ile mesajlaşma ekranına geç
3. Mesajını yaz, `enter` ile gönder

Karşı tarafın Peer ID'si AND ana menüsünde kimlik bilgisi ekranında görünür.

### Peer ID girişi ekranı

| Tuş | İşlev |
|-----|-------|
| Yazma | Peer ID'yi gir |
| `enter` | Bağlantıyı onayla, mesajlaşma ekranına geç |
| `esc` ya da `q` | Ana menüye dön |

### Mesajlaşma ekranı

| Tuş | İşlev |
|-----|-------|
| Yazma | Mesajı yaz |
| `enter` | Mesajı gönder |
| `ctrl+f` | Dosya gönderme ekranına geç |
| `esc` | Peer ID girişine dön |
| `ctrl+c` | Çık |

### Dosya gönderme ekranı

| Tuş | İşlev |
|-----|-------|
| Yazma | Dosya yolunu gir |
| `enter` | Dosyayı gönder |
| `esc` | Mesajlaşma ekranına dön |

---

## Dosya Onay İsteği

Başka bir kullanıcı sana dosya göndermek istediğinde ekranda onay isteği penceresi belirir:

```
╭─ Dosya Transfer İsteği ─────────────────╮
│ Gönderen : 12D3Koo…xyz                  │
│ Dosya    : rapor.pdf                    │
│ Boyut    : 5.2 MB                       │
│                                         │
│ Bu kullanıcının dosya göndermesine      │
│ izin veriyor musun?                     │
│                                         │
│ [y] Kabul   [n] Reddet                  │
╰─────────────────────────────────────────╯
```

| Tuş | İşlev |
|-----|-------|
| `y` | Transferi kabul et |
| `n` ya da `esc` | Transferi reddet |

30 saniye içinde yanıt verilmezse transfer otomatik olarak reddedilir.

---

## Teknik Detaylar

### Protokoller

| Protokol | Kullanım |
|----------|----------|
| `/and/dm/1.0.0` | Anlık metin mesajları (libp2p stream) |
| `/and/file/2.0.0` | Yeniden başlatılabilir, parçalı dosya aktarımı |

### Gelen mesajlar — long poll

Eklenti her 5 saniyede `GET /api/v1/dm/poll` isteği gönderir.  
AND mesaj gelene kadar bekler; gelince JSON array döner, yoksa 5 saniye sonra boş array döner.

### Gelen dosyalar — long poll

Eklenti paralel olarak `GET /api/v1/file/poll` isteği gönderir.  
Dosya tamamen alındığında AND dosyayı diske yazar ve yolu eklentiye bildirir.

### Onay akışı — long poll

Eklenti paralel olarak `GET /api/v1/file/consent-poll` isteği gönderir.  
Transfer isteği geldiğinde AND 30 saniye boyunca yanıt bekler; eklenti `POST /api/v1/file/consent` ile kabul/red bildirir.

### Güvenlik sınırları

- Gelen DM'ler 16 KB ile sınırlıdır; daha büyük paketler atılır
- Her akış için 30 saniyelik okuma zaman aşımı uygulanır
- Gönderme işlemi için 30 saniyelik zaman aşımı uygulanır

### Veri saklama

Mesaj geçmişi yalnızca oturum boyunca bellekte tutulur.  
AND kapatıldığında veya eklenti çıktığında mesajlar silinir; kalıcı mesajlaşma yoktur.  
Dosyalar AND kaydetme dizinine (`%APPDATA%\and\dosyalar\`) kalıcı olarak kaydedilir.

---

## Sınırlamalar

- **Uçtan uca şifreleme yoktur.** Mesajlar libp2p Noise protokolü ile aktarım katmanında şifrelenir, ancak uygulama düzeyinde ek şifreleme uygulanmaz.
- **Kalıcı iletim yoktur.** Alıcı çevrimdışıysa mesaj iletilmez ve kaybolur.
- **Görünen ad doğrulanmaz.** `From` alanı gönderenin beyanıdır; gerçek kimlik için Peer ID'yi kullan.

---

## Manifest

```json
{
  "name":        "ozel_chat",
  "label":       "Özel Chat",
  "version":     "2.1.0",
  "description": "Peer ID ile doğrudan özel mesajlaşma ve dosya aktarımı — libp2p stream",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/ozel_chat/main.go](main.go)

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 2.1.0 | Dosya gönderme (`ctrl+f`); dosya onay isteği dialog; consent-poll / consent API |
| 2.0.0 | Bağımsız binary; long-poll HTTP IPC; dmmgr üzerinden akış proxy'si |
| 1.0.0 | İlk sürüm (gömülü plugin sistemi) |
