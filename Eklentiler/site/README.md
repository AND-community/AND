# and-plugin-site — Site

Kendi çok sayfalı kişisel sitenizi oluşturun, düzenleyin ve yayınlayın.

---

## Genel Bakış

`and-plugin-site`, AND'ın "kişisel site" özelliğinin (bkz.
[internal/site](../../internal/site/)) TUI katmanıdır — eskiden AND'ın
çekirdek TUI'sinde ana menüdeki "Site" girişi olarak yer alan bu ekran,
diğer içerik-oluşturma araçlarıyla (Yönetici Paneli, DeltaX vb.) aynı
şekilde bağımsız bir eklentiye taşındı.

Her yazar, birbirinden bağımsız slug'larla (ör. `home`, `projeler`)
adreslenen, Ed25519 imzalı, GossipSub üzerinden ağa yayılan düz metin
sayfalar yayınlayabilir. Bu eklenti yalnızca **kendi** sayfalarınızı
yönetir — başka bir yazarın sitesini görüntülemek için ayrı bir ekran
yoktur; başkalarının yayınladığı sayfalar forumdaki profil ekranında
salt okunur olarak gösterilir (bkz. ana [README.md → Ekosistem](../../README.md)).

Rol kısıtlaması yoktur: herhangi bir kimlik kendi sitesini yönetebilir —
her uç nokta zaten çağıranın kendi kimliğine örtük olarak sınırlıdır
(`internal/pluginapi.SiteBackend`). İmzalama her zaman AND ana sürecinde
(`cmd/and`'in `siteAdapter`'ında, dolayısıyla `internal/site.Store`'da)
gerçekleşir — bu eklenti hiçbir zaman özel anahtar görmez.

---

## Kurulum

```bash
go build -o and-plugin-site ./Eklentiler/site

# Windows
go build -o and-plugin-site.exe ./Eklentiler/site
```

Binary, `plugin.json` ile aynı dizinde (`Eklentiler/site/`) bulunmalıdır;
AND başlangıçta manifest JSON dosyasını okur.

---

## Kullanım

### Sayfa listesi

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | sayfa seç |
| `n` | yeni sayfa oluştur (en fazla 20 sayfa) |
| `c` | seçili sayfayı ŞABLON olarak kullanarak yeni bir sayfa oluştur (başlık/içerik kopyalanır, slug boş bırakılır) |
| `enter` / `e` | seçili sayfayı düzenle (slug sabit kalır) |
| `d` | seçili sayfayı silme onayı iste |
| `t` | yerel taslakları görüntüle (bkz. aşağıdaki Taslaklar) |
| `r` | listeyi yenile |
| `q` / `ctrl+c` / `esc` | eklentiden çık |

### Düzenleme formu

| Tuş | İşlev |
|-----|-------|
| `tab` / `shift+tab` | alanlar arası geç (yeni sayfada slug/başlık/içerik; düzenlemede yalnızca başlık/içerik) |
| `ctrl+s` | kaydet ve ağa yayınla |
| `ctrl+d` | GÜNCEL içeriği yerel bir taslak olarak kaydet — ağa YAYINLANMAZ, formdan çıkılmaz (bkz. aşağıdaki Taslaklar) |
| `esc` | vazgeç, listeye dön (kaydedilmemiş değişiklik varsa önce onay ister — aşağıya bak) |
| `ctrl+c` | eklentiden çık |

Her alanın yanında GÜNCEL bayt sayısı sınırıyla birlikte gösterilir (ör.
"42/100 bayt") — sınır aşılırsa kırmızıyla uyarır. Bu, widget'ın kendi
karakter sınırından (rune sayar) daha doğrudur: Türkçe özel karakterler
("ğ", "ş", "ı" vb.) UTF-8'de 2 bayt olduğundan, widget'ın kabul ettiği bir
girdi yine de sunucunun bayt sınırını aşabilir — `ctrl+s` bu durumu ağa
hiç çıkmadan yerel olarak yakalayıp anlaşılır bir hata gösterir.

Kaydedilmemiş bir değişiklik varken `esc`'e basmak formu SESSİZCE atmaz:
ilk `esc` bir uyarı gösterir ("kaydedilmemiş değişiklikler var"), ikinci
`esc` çıkışı onaylar. `esc` dışında herhangi bir tuşa basmak (yazmaya
devam etmek dahil) bu uyarıyı iptal edip düzenlemeye kaldığın yerden devam
ettirir.

### Silme onayı

| Tuş | İşlev |
|-----|-------|
| `y` / `Y` | sil |
| başka bir tuş | vazgeç |

Silme, ağa kalıcı bir silme kaydı (tombstone) yayınlar: silinen bir slug bir
daha kullanılamaz. Aynı slug'la yeniden yayınlamak açık bir hatayla
reddedilir ("… daha önce silindi") — önceden "yayınlandı" mesajı gösterilip
sayfa sessizce hiçbir yerde saklanmıyordu.

Bir sayfa en fazla 8000 bayt, bir yazarın en fazla 20 sayfası olabilir;
slug yalnızca küçük harf, rakam ve tire içerebilir. Bu sınırların gerçek
denetimi sunucu tarafındadır (`internal/site.Store.PublishPage`) — bu
eklenti yalnızca anında geri bildirim için hafif bir istemci tarafı
kontrolü yapar.

### Taslaklar

Düzenleme formunda `ctrl+d`, o anki içeriği **ağa hiç yayınlamadan**
yerel bir dosyaya (`taslaklar_site.json`, AND'ın veri dizininde) kaydeder
— uzun bir sayfayı bitirmeden bırakabilmek ya da ağ olmadan da yazmaya
devam edebilmek için. Bir slug için en fazla bir taslak tutulur; aynı
slug'a tekrar `ctrl+d` basmak öncekinin üzerine yazar. Bir sayfa
başarıyla yayınlandığında (`ctrl+s`), o slug için varsa BAYAT kalmış
taslak otomatik temizlenir.

Ana listede `t` tuşu taslakları gösterir:

| Tuş | İşlev |
|-----|-------|
| `↑`/`↓` ya da `k`/`j` | taslak seç |
| `enter` / `e` | taslağı düzenleme formuna aç |
| `d` | taslağı sil (onay istenmez — yalnızca yerel, düşük risk) |
| `esc` / `q` | listeye dön |

Bir taslağın slug'ı zaten yayınlanmış bir sayfaya aitse, taslak o
sayfanın **düzenlemesi** olarak açılır (slug sabit); aksi halde **yeni
bir sayfa** olarak açılır. Taslaklar tamamen yereldir — ağa hiçbir zaman
gönderilmez, imzalanmaz ya da senkronize edilmez.

---

## Manifest

```json
{
  "name":        "site",
  "label":       "Site",
  "version":     "1.2.0",
  "description": "Kendi çok sayfalı kişisel sitenizi oluşturun, düzenleyin ve yayınlayın",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/site/](.) — `main.go` (manifest, bootstrap),
`model.go` (durum ve `Update`), `view.go` (render).

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 1.2.0 | Yerel taslak desteği eklendi (`ctrl+d` ile kaydet, `t` ile listele/yükle/sil — ağa yayınlamadan çalışmayı sürdürebilme); sayfa listesinde `c` ile var olan bir sayfayı şablon alarak yeni sayfa oluşturma eklendi |
| 1.1.0 | Düzenleme formuna GERÇEK (rune değil bayt) sınırları gösteren canlı sayaçlar eklendi; kaydedilmemiş değişikliklerle `esc`'e basmak artık çift onay ister; gövde alanının yüksekliği pencereye göre ölçeklenir |
| 1.0.0 | AND'ın çekirdek TUI'sindeki (`internal/tui/site.go`) "Site" ekranı buraya taşındı — `internal/pluginapi`'ye eklenen `SiteBackend`/`/api/v1/site/*` uç noktaları üzerinden çalışır |
