# and-plugin-pembis — Pembiş Tema

AND arayüzünü anında pembe tonlara boyayan (ya da varsayılana geri döndüren) küçük bir tema eklentisi.

---

## Ne yapar

`POST /api/v1/theme` uç noktasını kullanarak AND'ın ana TUI'sinin vurgu/seçim/isim/rozet renklerini
değiştirir. Değişiklik **anında** uygulanır — AND'ı yeniden başlatmak gerekmez; DeltaX gibi diğer
eklentiler de açılışta aynı temayı `GET /api/v1/theme` ile okuyup kendi renklerine uygular
(bkz. `internal/tui/styles.go`, `Eklentiler/DeltaX/main.go`'daki `uygulaTema`).

Pembiş, ağdan/başka kullanıcılardan gelen hiçbir veriyi işlemez veya ekrana yazmaz — yalnızca
kendi sabit menüsünü gösterir.

## Kullanım

Ana menüden **pembis**'i seç.

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | Menüde gez |
| `enter` | Seçili işlemi uygula |
| `q` / `ctrl+c` | Çık |

Menü seçenekleri:

1. **Pembe Temayı Uygula** — vurgu/seçim/isim/rozet renklerini pembe tonlara ayarlar.
2. **Varsayılan Temaya Dön** — AND'ın öntanımlı turkuaz temasına geri döner.
3. **Çıkış**.

Bir işlem uygulandıktan sonra sonuç ekranında `enter` ya da `q` ile eklentiden çıkılır.

## Manifest

```json
{
  "name":        "pembis",
  "label":       "Pembiş Tema",
  "version":     "1.1.0",
  "description": "AND arayüzünü pembe tonlarda kız temasına boyar",
  "author":      "AND",
  "category":    "theme"
}
```
