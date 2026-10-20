# and-plugin-admin — Yönetici Paneli

Konu yönetimi, ban listesi, moderasyon geçmişi ve moderatör atama — tek panelde.

---

## Genel Bakış

`and-plugin-admin`, kurucu ve moderatörlerin tüm idari işlemlerini tek
yerde toplar. Dört sekme:

| Sekme | Kimler görür | İşlev |
|-------|--------------|-------|
| **Konular** | kurucu + moderatör | Tüm forum konularını listele, gerekirse sil |
| **Banlar** | kurucu + moderatör | Aktif banları listele, `u` ile kaldır |
| **Geçmiş** | kurucu + moderatör | Salt okunur ban/unban denetim kaydı |
| **Moderatörler** | yalnızca kurucu | Yeni moderatör ata, mevcut birinin sertifikasını iptal et |

Panel yalnızca kurucu veya geçerli sertifikalı bir moderatör olarak
çalıştırıldığında açılır; aksi halde AND ana menüsünde görünmez ve
doğrudan çalıştırılırsa hata verip çıkar.

İmzalama her zaman AND ana sürecinde (`cmd/and`'in `modAdapter`'ında)
gerçekleşir — bu eklenti hiçbir zaman özel anahtar (private key) görmez,
yalnızca `internal/pluginapi` üzerinden yapılandırılmış istekler gönderir.

---

## Kurulum

```bash
go build -o and-plugin-admin ./Eklentiler/admin

# Windows
go build -o and-plugin-admin.exe ./Eklentiler/admin
```

Binary AND dizininde bulunmalıdır; AND başlangıçta manifest JSON dosyasını okur.

---

## Kullanım

Sekmeler arasında `tab`, `←`/`→` ya da `h`/`l` ile geçilir.

### Konular

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | konu seç |
| `x` | seçili konuyu silme onayı iste |
| `e` / `y` / `enter` (onay ekranında) | silmeyi onayla |
| `esc` / `n` / `h` (onay ekranında) | vazgeç |
| `r` | listeyi yenile |

### Banlar

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | ban kaydı seç |
| `u` | seçili banı kaldırma onayı iste |
| `e` / `y` / `enter` (onay ekranında) | onayla |
| `esc` / `n` / `h` (onay ekranında) | vazgeç |
| `r` | listeyi yenile |

Yazar anahtarı çözülemeyen (yalnızca bağlantı/peer düzeyinde bilinen) bir
ban kaydı için `u` bir hata bildirimi gösterir — bu kayıtlar yalnızca
profil ekranından ya da `andmod` CLI'dan kaldırılabilir.

### Geçmiş

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | kayıtlarda gez |
| `r` | listeyi yenile |

### Moderatörler (yalnızca kurucu)

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | kayıt seç |
| `g` | yeni moderatör atama formunu aç |
| `r` | seçili (iptal edilmemiş) sertifikayı iptal etme onayı iste |
| `e` / `y` / `enter` (onay ekranında) | onayla |
| `esc` / `n` / `h` (onay ekranında) | vazgeç |

Atama formu iki adımlıdır: önce hedefin 64 karakterlik hex public key'i
yazılır (`enter` ile ilerlenir; geçersiz uzunluk/format reddedilir),
ardından `←`/`→` ile süre seçilir (1 gün / 3 gün / 7 gün / ömür boyu) ve
`enter` ile sertifika oluşturulur. `backspace` bir önceki adıma döner.

Oluşturulan sertifika dosyası yalnızca **yerel** olarak yazılır —
hedefe iletmek (ör. Özel Chat ile dosya göndererek) kurucunun
sorumluluğundadır; bkz. [README.md → Moderasyon](../../README.md#moderasyon).

Genel çıkış: her ekrandan `q` / `ctrl+c` / (onay/form dışındayken) `esc`.

---

## Manifest

```json
{
  "name":        "admin",
  "label":       "Yönetici Paneli",
  "version":     "4.0.0",
  "description": "Konu yönetimi, ban listesi, moderasyon geçmişi ve moderatör atama (kurucu/moderatör yetkisi gerekir)",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/admin/](.) — `main.go` (manifest, bootstrap, paylaşılan stiller), `model.go` (sekme yönetimi, ortak `Update`/`View`), `konular.go`, `banlar.go`, `gecmis.go`, `moderatorler.go`.

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 4.0.0 | Eskiden çekirdek TUI'de ayrı bir "Moderasyon Paneli" ekranı olan Banlar/Geçmiş/Moderatörler işlevleri bu eklentiyle birleştirildi; dört sekmeli tek panel |

> Not: 4.0.0 öncesi sürüm geçmişi (yalnızca Konular sekmesini içeren dönem)
> eklentinin dosyalarının harici bir olayla diskten silinmesi sonucu
> kayboldu; eklenti bu sürümle sıfırdan yeniden yazıldı.
