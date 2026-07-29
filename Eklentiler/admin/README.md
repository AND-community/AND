# and-plugin-admin — Yönetici Paneli

Tüm forum konularını görüntüle, gerektiğinde sil.
**Kurucu veya moderatör yetkisi** gerektirir.

---

## Genel Bakış

AND forumunda konu onay sistemi yoktur: her konu yazıldığı anda ağa
yayılır ve, kim yazmış olursa olsun, oluşturulduktan **5 gün** sonra
otomatik olarak süresi dolar. `and-plugin-admin` binary'si kurucu/moderatöre
bu konuları izleme ve gerektiğinde erken silme arayüzü sağlar:

- Ağdaki tüm konuları listele (başlık, kategori, yazar)
- Uygunsuz bir konuyu süresi dolmadan kalıcı olarak sil

Kurucu ya da moderatör yetkisi yoksa binary başlamaz ve hata mesajıyla çıkar.

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
| Kurucu ya da moderatör kimliği | AND binary'sine gömülü `FounderPubKeyHex` ile eşleşen anahtar, ya da geçerli bir moderatör sertifikası |
| AND çalışıyor | `AND_API_ADDR` ortam değişkeni AND tarafından sağlanır |

---

## Kullanım

Ana menüden **Yönetici Paneli**'ni seç.

### Klavye kısayolları

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | Konular arasında gezin |
| `x` | Seçili konuyu **kalıcı olarak sil** — geri alınamaz |
| `r` | Listeyi yenile |
| `q` / `esc` | Çık |

---

## Silme

Silme işlemi **ağa yayın yapmaz**; yalnızca yerel veritabanından siler.
Ağdaki diğer peer'larda konu, kendi 5 günlük süresi dolana kadar görünür
olmaya devam edebilir.

Ağ genelinde bir peer'ı kalıcı olarak susturmak için `andmod ban` komutunu
kullan.

---

## Manifest

Binary `--manifest` argümanıyla çalıştırıldığında AND'ın okuduğu JSON:

```json
{
  "name":        "admin",
  "label":       "Yönetici Paneli",
  "version":     "3.0.0",
  "description": "Tüm forum konularını görüntüle ve gerektiğinde sil (kurucu/moderatör yetkisi gerekir)",
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
| 3.0.0 | Konu onay sistemi kaldırıldı; panel artık tüm konuları listeleyip silen tek ekrana indirgendi |
| 2.0.0 | Bağımsız binary mimarisine geçiş; HTTP IPC ile AND ile iletişim |
| 1.1.0 | Toplu onay (`A`), reddetme (`d`), TTL gösterimi, otomatik yenileme |
| 1.0.0 | İlk sürüm |
