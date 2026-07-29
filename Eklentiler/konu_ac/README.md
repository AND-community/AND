# and-plugin-konu-ac — Konu Açma Eklentisi

Yeni forum konusu oluştur, taslak kaydet, kategoriler arasında gezin.

---

## Genel Bakış

`and-plugin-konu-ac` forum yazma işlemlerini yönetir. Ana forum görünümü salt okunurdur; tüm konu oluşturma bu eklenti üzerinden gerçekleşir.

Bu eklenti menüde **görünmez** (`Label` alanı boştur). Forumdan `n` tuşuna basıldığında AND tarafından **doğrudan ana süreç içinde** açılır — ayrı bir binary başlatılmaz, geçiş anında gerçekleşir.

---

## Kurulum

```bash
go build -o and-plugin-konu-ac ./Eklentiler/konu_ac

# Windows
go build -o and-plugin-konu-ac.exe ./Eklentiler/konu_ac
```

Binary AND dizininde bulunmalıdır; AND başlangıçta manifest JSON dosyasını okur.

---

## Nasıl Açılır

Forumu açtıktan sonra `n` tuşuna bas.  
AND seçili kategoriyi otomatik iletir; kategori seçim ekranı atlanır.

---

## Kullanım

### Konu formu

Kategori satırında `◀` / `▶` tuşları ile kategori değiştirilebilir.

| Tuş | İşlev |
|-----|-------|
| `tab` | Başlık ↔ İçerik arasında geç |
| `enter` (Başlık) | İçerik alanına geç |
| `enter` (İçerik) | Alt satıra geç |
| `ctrl+k` (İçerik) | İmleç konumuna kod bloğu şablonu ekle |
| `ctrl+s` | Konuyu gönder |
| `ctrl+t` | Taslak listesini aç (taslak varsa) |
| `◀` / `▶` | Kategori değiştir (yalnızca Başlık alanındayken) |
| `esc` | İçeriği taslak olarak kaydet ve foruma dön |
| `ctrl+c` | Kaydetmeden çık |

### Taslak listesi

| Tuş | İşlev |
|-----|-------|
| `↑` / `↓` ya da `j` / `k` | Taslak seç |
| `enter` ya da `e` | Seçili taslağı forma yükle (düzenlemek için) |
| `p` | Seçili taslağı formu açmadan doğrudan gönder |
| `x` | Seçili taslağı sil |
| `esc` | Forma dön |

---

## Taslak sistemi

`esc` tuşuna basıldığında başlık veya içerik doluysa taslak otomatik kaydedilir.  
Aynı kategori tekrar açıldığında taslaklar `ctrl+t` ile geri yüklenebilir.

Taslaklar `AND_DATA_DIR/taslaklar_<kategori>.json` dosyalarında saklanır.  
Bu dosyalar yereldir, ağa gönderilmez ve `.gitignore`'da yer alır.

---

## Kod bloğu

İçerik içinde `[code]` ve `[/code]` arasına yazılan metin, konu görüntülenirken diğer yazılım forumlarında olduğu gibi satır numaralı, söz dizimi vurgulamalı bir kod kutusu olarak gösterilir. Etiketler büyük/küçük harfe duyarlı değildir.

Dili belirtmek için `[code=dil]` biçimi kullanılabilir (`python`, `go`, `js`/`javascript`, `c`/`cpp`/`java`, `rust` ve yaygın takma adları tanınır). Dil belirtilmezse yaygın dillerin anahtar kelimelerinin birleşimiyle genel bir vurgulama uygulanır.

İçerik alanında `ctrl+k` tuşuna basmak, imleç konumuna hazır bir `[code]` / `[/code]` şablonu ekler ve imleci aralarına bırakır — etiketleri elle yazmaya gerek kalmaz. Gönder (`ctrl+s`) veya taslak kaydet (`ctrl+d`) sırasında kapanmamış bir `[code]` etiketi varsa işlem durdurulup uyarı gösterilir.

```
Şu fonksiyonda bir sorun var:

[code=go]
func topla(a, b int) int {
    return a - b
}
[/code]

Neden yanlış sonuç veriyor?
```

---

## Karakter Sınırları

| Alan | Maksimum |
|------|---------|
| Başlık | 100 karakter |
| İçerik | 8192 karakter |

---

## Kategoriler

| | | | |
|--|--|--|--|
| Python | C / C++ | Rust | Go |
| JavaScript | Java / Kotlin | C# / .NET | PHP |
| Swift / Objective-C | Yazılım | Web | Mobil |
| Yapay Zeka | Veritabanı | DevOps | Bulut Bilişim |
| Linux | Bilişim | Ağ ve Sistem Yönetimi | Siber Güvenlik |
| Donanım | Gömülü Sistemler / IoT | Robotik | Blockchain / Web3 |
| Algoritma ve Veri Yapıları | Test ve Kalite | UI / UX Tasarım | Oyun Geliştirme |
| Açık Kaynak | Kariyer | Freelance | Eğitim Kaynakları |
| Etkinlikler ve Duyurular | Genel | | |

---

## Moderasyon notu

Forumda konu onay sistemi yoktur: gönderilen konu hemen ağda yayılır ve,
kim yazmış olursa olsun, oluşturulduktan **5 gün** sonra otomatik olarak
silinir. Kurucu/moderatör gerektiğinde bir konuyu bu süreyi beklemeden
Yönetici Paneli'nden erken silebilir (bkz. [Eklentiler/admin/README.md](../admin/README.md)).

---

## Manifest

```json
{
  "name":        "konu_ac",
  "label":       "",
  "version":     "2.5.0",
  "description": "Forum'da yeni konu oluşturma ve taslak yönetimi (menüde gizli, forumdan n ile açılır)",
  "author":      "AND"
}
```

`label` boş olduğu için bu eklenti ana menüde görünmez.

---

## Kaynak

Kaynak kod: [Eklentiler/konu_ac/main.go](main.go)

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 2.5.0 | Konu onay sistemi kaldırıldı; `ctrl+p` kalıcılık talebi kısayolu ve ilgili form alanı silindi — her konu artık kim yazarsa yazsın 5 gün sürüyor |
| 2.4.0 | Canlı önizleme kaldırıldı (ekran titremesine/kalıntıya sebep oluyordu); yerine `ctrl+k` ile tek tuşla kod bloğu şablonu ve gönderim öncesi kapanmamış `[code]` etiketi uyarısı eklendi; İçerik alanında sol/sağ ok tuşlarıyla imleç hareketi düzeltildi |
| 2.3.0 | `[code=dil]` ile dil etiketi ve rozeti; satır numaraları; temel söz dizimi vurgulama; içerik alanında `[code]` yazılırken canlı önizleme |
| 2.2.0 | `[code]...[/code]` ile yazılan içerik, konu görüntülenirken kod kutusu olarak gösterilir; forma bilgilendirme satırı eklendi |
| 2.1.0 | AND ana sürecinde inline çalışma (ayrı binary başlatılmaz); Tab ile alan geçişi; kategori ◀/▶; Enter ile İçerik'te alt satır |
| 2.0.0 | Bağımsız binary; HTTP IPC ile AND ile iletişim; taslak sistemi |
| 1.0.0 | İlk sürüm |
