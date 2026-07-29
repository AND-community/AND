# and-plugin-DeltaX — DeltaX

Özel gruplar kur, kendi sürüm kontrol sistemiyle birlikte proje geliştir. Kod ve proje geçmişi yalnızca grubu oluşturan üyeler arasında, merkezi forum/sohbet ağının tamamen dışında, uçtan uca şifreli olarak dolaşır — grubun **kendi özel ağı**.

---

## Genel Bakış

`and-plugin-DeltaX`, üç katmandan oluşur:

1. **Gruplar** — kurucusu tarafından davet edilen üyelerden oluşan, rastgele üretilmiş bir AES-256 sırrını (`Sir`) paylaşan özel topluluklar.
2. **Projeler** — her grubun içinde, içerik-adresli (blob/ağaç/kayıt tabanlı) bir sürüm kontrol deposu (dal, kayıt/commit, günlük, fark/diff, üç yönlü birleştirme dahil).
3. **Senkronizasyon** — AND'ın DM (`/and/dm/1.0.0`) ve dosya aktarım (`/and/file/2.0.0`) protokolleri üzerinden, yalnızca nokta-noktaya (peer-to-peer), grup sırrıyla şifrelenmiş paketlerle. Hiçbir veri forum'a, GossipSub sohbet kanallarına ya da başka bir merkezi bileşene değmez.

Eklenti ayrıca açılışta `GET /api/v1/theme` ile AND'ın o an aktif teması (ör. **pembis** eklentisiyle seçilmiş "Pembiş" teması) hangisiyse onu okur ve kendi renklerini buna göre ayarlar — böylece ana uygulama ile aynı renk şemasında açılır.

---

## Kurulum

```bash
go build -o and-plugin-DeltaX ./Eklentiler/DeltaX

# Windows
go build -o and-plugin-DeltaX.exe ./Eklentiler/DeltaX
```

AND, `Eklentiler/DeltaX/plugin.json` + yukarıdaki binary'yi otomatik keşfeder.

---

## Kullanım

Ana menüden **DeltaX**'ı seç.

### Ana menü

| Tuş | İşlev |
|-----|-------|
| `↑/↓`, `j/k` | Gez |
| `enter` | Seç |
| `q`/`esc` | Çık |

Seçenekler: **Gruplarım**, **Yeni Grup Oluştur**, **Davetler**, **Kimlik Bilgileri**, **Çıkış**.

### Kısayol yardımı, ilerleme göstergesi ve onay pencereleri

- **`?` — kısayollar**: liste/menü ekranlarının çoğunda (metin girişi olan ekranlar hariç) `?`'ye basınca o ekranın tüm kısayolları büyük, okunaklı bir kutuda açılır; herhangi bir tuşla kapanır.
- **Gerçek ilerleme göstergesi**: Kaydet/Gönder/Çek/Birleştir/Dışa Aktar gibi arka planda süren işlemler artık statik bir metin yerine dönen bir spinner ile gösterilir — işlemin donmadığını görebilirsin.
- **Onay pencereleri**: Gruptan çıkma/grubu kapatma gibi, **dal değiştirme** ve **birleştirme** de artık `y`/`n` ile onay isteyen aynı pencereyi kullanır — ikisi de çalışma klasöründeki **izlenmeyen ve `.projeagignore` ile yok sayılan dosyalar dahil her şeyi silip yeniden yazar**; pencere bunu açıkça belirtir. (İstisna: Dosyalar ekranındaki tekil dosya silme hâlâ hızlı "d'ye iki kez bas" deseninde — düşük riskli, sık kullanılan bir işlem için modal pencere gereksiz yavaşlık olurdu.)
- **Gruplarım'da arama**: `/` ile grup adına/kurucu adına göre canlı filtrele (Günlük ekranındaki arama ile aynı desen), `esc` ile aramadan çık.

### Grup akışı

1. **Yeni Grup Oluştur** — grup adını gir; rastgele bir grup anahtarı üretilir.
2. Grup panelinde `i` ile bir üyeyi Peer ID'siyle davet et (kurucu ya da bir yönetici davet edebilir).
3. Davet edilen kişi **Davetler** ekranında `a` ile kabul eder — kabul otomatik olarak davet edene bildirilir ve güncel üye listesi gruptaki herkese yeniden yayınlanır, böylece herkesin listesi yakınsar.
4. Grup panelinde `n` ile yeni bir proje oluştur.

### Yönetici ata (yedek kurucu)

Kurucu, grup panelinde `y` ile bir üyeyi **yönetici** yapabilir (ya da yöneticilikten çıkarabilir). Yöneticiler kurucuyla aynı şekilde üye davet edebilir ve grubu kapatabilir — kurucunun cihazı/erişimi kaybolursa grup yönetimsiz kalmasın diye. Yalnızca gerçek kurucu yönetici atayabilir/kaldırabilir; bu yetkiyi bir yönetici devredemez.

### Gruptan çıkma / grubu kapatma

Grup panelinde `x` tuşuna bas, ardından `y` ile onayla:

- **Sıradan bir üyeysen** → *Gruptan Çık*: kurucuya ve tüm yöneticilere bildirilir (biri ulaşılabilirse seni listeden çıkarıp güncel listeyi yeniden yayınlar); grubun yerel kaydın senin tarafında silinir.
- **Kurucu ya da bir yöneticiysen** → *Grubu Kapat*: tüm üyelere grup dağıtıldı bildirimi gönderilir, alıcı tarafta grup otomatik olarak kendi listelerinden kalkar; grubun yerel kaydı herkeste silinir.

Her iki durumda da **proje dosyaların diskte kalır** — hiçbir yerel çalışma silinmez, yalnızca grubun/roster'ın kendisi kaldırılır. `n` ile vazgeçebilirsin.

### Proje paneli

| Seçenek | Açıklama |
|---------|----------|
| Durum | Çalışma klasörü ile HEAD arasındaki eklenen/değişen/silinen dosyalar |
| Kaydet (Commit) | Anlık görüntüyü al, mesaj yazıp yeni bir kayıt oluştur |
| Günlük | Aktif daldaki kayıt geçmişi — `/` ile mesaj/yazar/hash'e göre canlı ara |
| Dallar | Dal oluştur / dallar arasında geçiş yap (`n` yeni dal) |
| Fark (Diff) | Değişen/yeni bir dosyanın satır bazlı farkı |
| Gönder (Push) | Yeni kayıtları bir üyeye ya da çevrimiçi herkese şifreli paket olarak gönder |
| Uzak Dallar (Birleştir) | Bir üyeden alınan değişiklikleri üç yönlü birleştirmeyle aktif dala uygula |
| Üyeden Çek İste (Pull) | Bir üyeden kendi değişikliklerini sana göndermesini iste |
| Dosyalar (Düzenle) | AND'dan çıkmadan dosya oluştur/düzenle/sil (gömülü metin düzenleyici) |
| Çalışma Klasörünü Aç | Projenin dosyalarının bulunduğu klasörü sistem dosya yöneticisinde aç |
| Dışa Aktar (.zip) | Projenin tüm geçmişini (nesneler + çalışma klasörü) tek bir .zip'e paketle |

Projenin dosyaları **Çalışma Klasörü**'nde tutulur (proje panelinde yolu gösterilir) — dosyaları kendi editörünle ya da eklentinin **Dosyalar (Düzenle)** ekranıyla orada düzenle, sonra **Kaydet (Commit)** ile bir kayıt oluştur.

### .projeagignore

Çalışma klasörünün köküne bir `.projeagignore` dosyası koyarak (satır başına bir desen) belirli dosya/klasörleri anlık görüntülerin (Durum/Kaydet/Gönder) dışında tutabilirsin — ör. derleme çıktıları, `node_modules`. `/` içermeyen bir desen (ör. `node_modules`, `*.log`) yolun herhangi bir segmentiyle eşleşirse o yolu (ve altındakileri) dışarıda bırakır; `/` içeren desenler köke göre tam yol olarak eşleşir. Dosyanın kendisi normal şekilde izlenir, böylece kurallar grup üyeleri arasında da paylaşılır.

### Birleştirme çakışmaları

Aynı dosya iki tarafta da farklı şekilde değiştirilmişse, dosya çalışma klasöründe şu işaretlerle yazılır:

```
<<<<<<< YEREL
(senin sürümün)
=======
(uzak sürüm)
>>>>>>> UZAK
```

Dosyayı elle düzenleyip işaretleri kaldırdıktan sonra **Kaydet (Commit)** ile birleştirme kaydını tamamla (`MERGE_HEAD` otomatik temizlenir).

---

## Teknik Detaylar

### Sürüm kontrol modeli

İçerik-adresli bir sürüm kontrol modeli: `Blob` (ham içerik), `Agac` (yol → blob hash eşlemi), `Commit` (ağaç + ebeveyn(ler) + yazar/mesaj/zaman). Nesneler `sha256(tip + içerik)` ile adreslenir ve `nesneler/<hash[:2]>/<hash[2:]>` altında tekilleştirilerek saklanır. Dal geçişleri temiz çalışma dizini gerektirir; birleştirmeler ortak atayı bularak (fast-forward veya üç yönlü) uygulanır.

### Ağ protokolü

| Kanal | Kullanım |
|-------|----------|
| DM (`/and/dm/1.0.0`) | Küçük kontrol mesajları — davet, üye/yönetici listesi güncellemesi, push bildirimi, pull isteği, gruptan ayrılma, grup kapatma. `AND-PROJEAG/1\|`-önekli base64 JSON olarak taşınır; bu önek diğer eklentilerin normal DM trafiğinden ayırt edilmesini sağlar. |
| Dosya aktarımı (`/and/file/2.0.0`) | Asıl veri — commit/ağaç/blob nesnelerinin `projeag-<grupID>-<projeID>-<zaman>.pack` adlı gzip'li tar arşivi. Arşiv, göndermeden önce grup sırrıyla AES-256-GCM ile şifrelenir. |

### Güvenlik modeli

- **Grup sırrı:** Her grubun 32 baytlık rastgele bir AES anahtarı vardır; yalnızca davet edilen üyelere DM ile iletilir. Paketler bu anahtarla şifrelenir — grubun dışındaki hiç kimse (aynı AND ağına bağlı olsa bile) içeriği çözemez.
- **Bütünlük:** Paket içindeki her nesnenin adı kendi sha256 hash'idir; açılırken hash yeniden hesaplanıp doğrulanır, uyuşmayan girdiler sessizce atlanır.
- **Otomatik dosya onayı yalnızca grup üyelerinden:** AND'ın dosya aktarım onay akışı korunur. `projeag-` önekli bir dosya yalnızca gönderenin libp2p tarafından doğrulanmış Peer ID'si (`FileConsentReq.SenderID` — sahtecilik yapılamaz) yerel olarak bilinen bir grubun üyesiyse otomatik kabul edilir; aksi halde her zamanki gibi elle onay istenir.
- **Pull isteklerinde hedef doğrulama:** Bir üyenin pull isteğine otomatik yanıt verilirken istekte belirtilen Peer ID, o an yerelde bilinen grup üye listesiyle karşılaştırılır — böylece biri sahte bir hedef Peer ID vererek veriyi grup dışına yönlendiremez.
- **Kurucu ve yönetici doğrulaması.** Davet/roster/kapatma mesajları yalnızca gönderenin peer ID'si zaten bilinen `KurucuPeer` ya da `Yoneticiler` listesiyle eşleşiyorsa kabul edilir — gruba hiç üye olmayan biri sahte bir mesajla rostere el koyamaz.

### Kalıcılık

Tüm veriler `AND_DATA_DIR/DeltaX/` altında yerel olarak saklanır — hiçbir şey merkezi bir sunucuya gönderilmez:

```
DeltaX/
  gruplar/<grupID>.json
  projeler/<projeID>/meta.json       (dallar, aktif dal, uzak dallar)
  projeler/<projeID>/nesneler/…      (içerik-adresli commit/ağaç/blob deposu)
  projeler/<projeID>/calisma/…       (çalışma klasörü — burada dosyaları düzenlersin)
  projeler/<projeID>/MERGE_HEAD      (yalnızca çakışmalı bir birleştirme beklerken var olur)
```

---

## Sınırlamalar

- **Yalnızca eklenti açıkken senkronize olur.** DM/dosya polling yalnızca eklenti çalışırken sürer; kapalıyken gönderilen davet/push bildirimleri kaçırılır (dosya paketinin kendisi grup üyesiyse otomatik kabul edilip işlenir, ancak DM bildirimi kaçırılabilir).
- **Kurucu değişmez/devredilemez.** Yalnızca "yönetici" (yedek kurucu) ataması yapılabilir — bkz. Kullanım/"Yönetici ata". Kurucu gruptan "çıkamaz" — yalnızca grubu tamamen kapatabilir.
- **Basitleştirilmiş birleştirme.** Metin tabanlı, satır düzeyinde bir üç yönlü birleştirme yapılır; ikili (binary) dosyalarda çakışma her zaman tüm dosya olarak işaretlenir.
- **Büyük dosyalar.** Anlık görüntüye dahil edilecek tekil dosyalar 200 MB, satır bazlı fark (diff) için 512 KB, gömülü düzenleyiciyle açılabilecek dosyalar için 2 MB sınırı vardır; üzerindekiler atlanır/bildirilir (harici bir editör/dosya yöneticisi kullanılabilir).
- **`.projeagignore` ile izlenen/izlenmeyen dosya ayrımı yapılmaz.** Bir dosya daha önce kaydedilmiş, sonradan ignore listesine eklenmişse, sonraki Durum/Kaydet ekranlarında "silinen" olarak görünür.

---

## Manifest

```json
{
  "name":        "DeltaX",
  "label":       "DeltaX",
  "version":     "1.4.1",
  "description": "Özel gruplar kurup kendi sürüm kontrol sistemiyle birlikte proje geliştirme — merkezi ağ dışında, yalnızca grup üyeleri arasında uçtan uca şifreli senkronizasyon",
  "author":      "AND"
}
```

---

## Kaynak

Kaynak kod: [Eklentiler/DeltaX/](.)

| Dosya | İçerik |
|-------|--------|
| `main.go` | Manifest, giriş noktası, stiller |
| `model.go` | Bubble Tea modeli, async komutlar, DM zarf işleme, `Update` |
| `keys.go` | Ekran bazlı klavye olayı işleyicileri |
| `view.go` | Ekran render fonksiyonları |
| `store.go` | Grup/proje kalıcılığı |
| `vcs.go` | İçerik-adresli nesne deposu, dal/commit/diff/birleştirme, günlük araması |
| `crypto.go` | Grup sırrı AES-256-GCM şifreleme |
| `sync.go` | DM kontrol protokolü, paket oluşturma/uygulama, push/pull/davet/yönetici |
| `ignore.go` | `.projeagignore` desen eşleştirme |
| `editor.go` | Gömülü dosya listeleme/okuma/yazma/silme |
| `export.go` | Projeyi `.zip` olarak dışa aktarma |

---

## Sürüm Geçmişi

| Sürüm | Değişiklik |
|-------|------------|
| 1.3.0 | Eklenti adı **ANDSnap**'den **DeltaX**'e değiştirildi (dizin, manifest, veri klasörü `AND_DATA_DIR/DeltaX/` dahil) |
| 1.2.0 | Eklenti adı **proje_ag**'den **ANDSnap**'e değiştirildi (dizin, manifest, veri klasörü `AND_DATA_DIR/ANDSnap/` dahil) |
| 1.1.0 | `.projeagignore`, yönetici (yedek kurucu) ataması, gömülü dosya düzenleyici, günlükte arama, projeyi `.zip` olarak dışa aktarma, AND temasını otomatik uygulama, ana menüde `esc` ile çıkış |
| 1.0.0 | İlk sürüm — gruplar, kendi sürüm kontrol sistemi, uçtan uca şifreli P2P push/pull/birleştirme |
