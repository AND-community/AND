# and-plugin-admin — Yönetici Paneli

Onay bekleyen forum konularını incele, onayla veya reddet.  
**Kurucu yetkisi** gerektirir.

---

## Genel Bakış

AND forum sistemi varsayılan olarak tüm konuları moderasyon kuyruğuna alır (5 günlük TTL).  
`and-plugin-admin` binary'si kurucuya bu kuyruğu yönetme arayüzü sağlar:

- Bekleyen konuları listele ve içeriklerini oku
- Seçili konuyu Ed25519 imzalı onay mesajıyla tüm ağa yayınla
- Aynı yazarın tüm bekleyen konularını tek seferde onayla
- Uygunsuz bir konuyu reddet (yalnızca yerel silme; ağa yayın olmaz)

Kurucu yetkisi yoksa binary başlamaz ve hata mesajıyla çıkar.

---

## Kurulum

Bu binary AND ana binary'siyle aynı dizinde bulunmalıdır:

```bash
# Kaynaktan derle
go build -o and-plugin-admin ./Eklentiler/admin

# Windows
go build -o and-plugin-admin.exe ./Eklentiler/admin
```

AND bir sonraki başlangıcında eklentiyi otomatik keşfeder.

---

## Gereksinimler

| Koşul | Açıklama |
|-------|----------|
| Kurucu kimliği | AND binary'sine `-ldflags` ile gömülü `FounderPubKeyHex` ile eşleşen anahtar |
| AND çalışıyor | `AND_API_ADDR` ortam değişkeni AND tarafından sağlanır |

---

## Kullanım

Ana menüden **Yönetici Paneli**'ni seç.

### Liste ekranı

Onay bekleyen konular başlık, kategori, yazar ve kalıcılık talebiyle listelenir.  
`★` işareti kullanıcının kalıcılık talebinde bulunduğunu gösterir.

### Klavye kısayolları

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | Konular arasında gezin |
| `enter` | Seçili konunun tam içeriğini görüntüle |
| `r` | Listeyi yenile |
| `esc` | Ana menüye dön (ya da detaydan listeye dön) |

#### Detay ekranında

| Tuş | İşlev |
|-----|-------|
| `a` | Seçili konuyu onayla (Ed25519 imzalı, tüm ağa yayınlanır) |
| `A` | Aynı yazarın **tüm** bekleyen konularını onayla (toplu) |
| `d` | Seçili konuyu reddet (yalnızca yerel silme) |
| `↑` / `↓` ya da `j` / `k` | İçeriği kaydır |
| `esc` | Listeye dön |

---

## Onay süreci

`a` tuşuna basıldığında:

1. `approve|<postID>|<unix_timestamp>` payload'u Ed25519 özel anahtarıyla imzalanır
2. `ApprovalMsg` bir `Envelope` içinde GossipSub moderasyon topic'ine yayınlanır
3. Ağdaki tüm peer'lar mesajı alır, kurucu imzasını doğrular ve konuyu kalıcı hale getirir
4. Konu yerel veritabanında da kalıcı olarak işaretlenir

---

## Toplu onay (`A` tuşu)

Seçili konunun yazarına (`AuthorKey`) ait tüm bekleyen konular tek seferde onaylanır.  
Her konu için ayrı bir imzalı mesaj ağa gönderilir.

Bu özelliği yalnızca güvendiğin yazarlar için kullan.

---

## Reddetme (`d` tuşu)

Reddetme işlemi **ağa yayın yapmaz**; yalnızca yerel veritabanından siler.  
Ağdaki diğer peer'larda konu TTL dolana kadar görünür olmaya devam edebilir.

Ağ genelinde içerik kaldırmak için `andmod ban` komutunu kullan.

---

## Manifest

Binary `--manifest` argümanıyla çalıştırıldığında AND'ın okuduğu JSON:

```json
{
  "name":        "admin",
  "label":       "Yönetici Paneli",
  "version":     "2.0.0",
  "description": "Onay bekleyen konuları yönet (kurucu yetkisi gerekir)",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/admin/main.go](main.go)

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 2.0.0 | Bağımsız binary mimarisine geçiş; HTTP IPC ile AND ile iletişim |
| 1.1.0 | Toplu onay (`A`), reddetme (`d`), TTL gösterimi, otomatik yenileme |
| 1.0.0 | İlk sürüm |
