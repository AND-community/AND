# and-plugin-moderator — Moderatör Paneli

Geçerli bir moderatör sertifikasıyla onay bekleyen forum konularını incele, onayla veya reddet.

---

## Genel Bakış

`and-plugin-moderator` binary'si, kurucu tarafından yetkilendirilmiş moderatörlere forum kuyruğunu yönetme arayüzü sağlar.

Kurucu ile aynı onay yetkisine sahiptir; tek fark toplu onay (`A` tuşu) yalnızca kurucuya aittir.

Geçerli bir sertifikan yoksa veya sertifikan süresi dolmuşsa binary başlamaz ve hata mesajıyla çıkar.

---

## Kurulum

```bash
go build -o and-plugin-moderator ./Eklentiler/moderator

# Windows
go build -o and-plugin-moderator.exe ./Eklentiler/moderator
```

Binary AND binary'siyle aynı dizinde olmalıdır.

---

## Gereksinimler

| Koşul | Açıklama |
|-------|----------|
| Moderatör sertifikası | Kurucunun imzaladığı, süresi dolmamış `.json` dosyası |
| Sertifika konumu | `%APPDATA%\and\bans\mod_<kisa>.json` |
| AND çalışıyor | `AND_API_ADDR` ortam değişkeni AND tarafından sağlanır |

---

## Sertifika Alma

Kurucu, `andmod grant` komutuyla sertifika oluşturur ve sana iletir:

```bash
# Kurucunun çalıştırdığı komut
./andmod grant <senin_pubkey_hex> --days 30
```

Oluşturulan `.json` dosyasını `%APPDATA%\and\bans\` dizinine yerleştir.  
AND yeniden başlatıldığında sertifika otomatik tanınır.

Kendi public key'ini öğrenmek için:

```bash
./andmod pubkey
```

---

## Kullanım

Ana menüden **Moderatör Paneli**'ni seç.

### Klavye kısayolları

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | Konular arasında gezin |
| `enter` | Seçili konunun tam içeriğini görüntüle |
| `r` | Listeyi yenile |
| `esc` | Ana menüye dön (veya detaydan listeye dön) |

#### Detay ekranında

| Tuş | İşlev |
|-----|-------|
| `a` | Seçili konuyu onayla (sertifikalı imza ile ağa yayınlanır) |
| `d` | Seçili konuyu reddet (yalnızca yerel silme) |
| `↑` / `↓` ya da `j` / `k` | İçeriği kaydır |
| `esc` | Listeye dön |

---

## Onay süreci

`a` tuşuna basıldığında:

1. `approve|<postID>|<unix_timestamp>` payload'u Ed25519 özel anahtarıyla imzalanır
2. İmzaya moderatör sertifikası eklenerek `ApprovalMsg` oluşturulur
3. Mesaj GossipSub moderasyon topic'ine yayınlanır
4. Ağdaki peer'lar moderatör imzasını **ve** sertifikanın kurucu imzasını doğrular
5. Her iki imza geçerliyse konu kalıcı hale getirilir

---

## Reddetme (`d` tuşu)

Reddetme işlemi ağa yayın yapmaz; yalnızca yerel veritabanından siler.  
Ağ genelinde içerik kaldırmak için `andmod ban` komutunu kullan.

---

## Sertifika süresi dolarsa

Sertifikan süresi dolduğunda panel başlamaz.  
Yeni sertifika almak için kurucuyla iletişime geç; yeni `.json` dosyasını `bans/` dizinine yerleştir.

---

## Manifest

```json
{
  "name":        "moderator",
  "label":       "Moderatör Paneli",
  "version":     "2.0.0",
  "description": "Onay bekleyen konuları yönet (moderatör sertifikası gerekir)",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/moderator/main.go](main.go)

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 2.0.0 | Bağımsız binary mimarisine geçiş; HTTP IPC |
| 1.1.0 | Reddetme (`d`), TTL gösterimi, otomatik yenileme |
| 1.0.0 | İlk sürüm |
