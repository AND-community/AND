# Forum eklentisi, Site ve Yönetici Paneli arayüzü — Tasarım

Tarih: 2026-11-25. Önceki belge: `2026-11-24-eklenti-arayuzu-design.md` (protokol, çizici, 1–2. aşama).

## Amaç

Kullanıcının isteği: "yönetici paneli ve siteler eklentisini de terminal yerine bir arayüz tasarla ve
forumu da bir eklenti haline getir". Sorulduğunda forum için **"tamamen eklentiye taşınsın"** seçildi:
forumu hem terminalde hem masaüstünde bir eklenti çizer, AND çekirdeğinde ağ, veri ve API kalır; forum
Ayarlar'dan kapatılıp ayrı güncellenebilir; `konu_ac` foruma katılır.

Site ve Yönetici Paneli önceki belgenin 3. ve 4. aşamalarıdır (yaklaşım onaylı: "eklenti anlatır, AND
çizer"). Bu belge onların ekran eşlemesini ve forumun taşınmasını tanımlar.

## Hedefler

- Site ve Yönetici Paneli masaüstünde terminal yerine AND'ın arayüzüyle açılır (`"ui": 1`); terminalde
  bugünkü gibi çalışmaya devam eder.
- Yeni `forum` eklentisi: kategoriler, konu listesi, konu okuma, yanıt yazma (taslaklı), konu açma
  (taslaklı, eski konu_ac), kendi konusunu silme, yazar profili. Terminalde ve masaüstünde aynı mantık.
- Masaüstünde forum bugünkü kadar iyi görünür: makale gibi konu, iplik çizgili yanıtlar, mühür, kenar
  çubuğunda forumun altında kategoriler, Ctrl+K'de konu araması.
- AND çekirdeğinden forum ekranları kalkar (TUI `forum.go`, masaüstü forum görünümü).

## Hedef dışı

- Forum verisinin, imzaların, ağ protokolünün ya da eşitlemenin değişmesi (eski sürümlü eşlerle
  uyum aynen sürer).
- Yeni forum özellikleri (düzenleme, tepki, arama ekranı, sayfalı okuma). Site düzenleyicisine önizleme.
- GitHub'da konu_ac deposunu arşivlemek (kullanıcının kararı; yalnızca README notu düşülür).
- Linux/macOS yerel masaüstü penceresi.

## Olmaması gerekenler

- Masaüstü forumun bugünkünden kötü görünmesi ya da yavaşlaması.
- Taslak kaybı (yanıt taslakları ve konu_ac taslakları yeni eklentiye taşınır).
- Forum eklentisinin `forum` dışında izin istemesi.
- Protokol v1 konuşan dış eklentilerin bozulması.

## Yaklaşımlar

| Yol | Ne getirir | Neden seçilmedi / seçildi |
|---|---|---|
| **A. DeltaX/Özel Chat kalıbı** — forumun bubbletea modeli eklentiye taşınır, `Arayuz()` ekler; veri genel eklenti API'sinden (genişletilerek) gelir; masaüstü görünümü için protokole birkaç geriye uyumlu öğe eklenir | Tek mantık iki yüz; Ayarlar'dan kapatılabilir; ayrı güncellenir | **Seçildi** (kullanıcı "tamamen eklentiye" dedi; mevcut kalıp) |
| B. Eklenti forum.db'yi doğrudan açar | Az API işi | Eşitleme AND sürecinde; iki süreç aynı veritabanına yazamaz |
| C. Masaüstünü AND çizmeye devam eder, eklenti yalnızca terminalde | Az iş | "Tamamen eklenti" isteğine aykırı; iki forum kodu yaşar |

## Kararlar

Kullanıcının: forum tamamen eklentiye; konu_ac içine katılır; sıra Site → Yönetici Paneli → Forum.

Benim kararlarım (düzeltilebilir):

1. **Yazarı yasaklama Yönetici Paneli'ne taşınır.** Bugün terminal forumunda yazar profilinden `b`
   ile yasaklanıyor. Forum eklentisi yalnızca `forum` izni ister; yasaklama `moderation` izni gerektirir.
   Yönetici Paneli'nin Konular sekmesine "Yazarı yasakla", Yasaklar sekmesine "Anahtarla yasakla"
   eklenir. Forumdaki yazar profili yetkiliye "Yasaklamak için Yönetici Paneli" ipucunu gösterir.
   Sohbetteki (çekirdek) profil ekranının yasaklaması değişmez.
2. **İzin akışı herkesle aynı.** Forum AND'la gelir ve açık başlar; `forum` iznini ilk açılışta
   standart izin diyaloğuyla bir kez sorar. AND'ın kendi eklentilerine özel ön onay yok.
3. **Masaüstünde açılış görünümü forum eklentisidir;** kapalıysa ya da yoksa Sohbet.
4. **Kenar çubuğu ve Ctrl+K korunur:** eklenti kategori listesini AND'a `menu` mesajıyla bildirir; AND
   onları bugünkü gibi eklentinin altında gösterir. Tıklama eklentiye `git` mesajı olarak gider.
5. **Forum eklentisi AND deposunda** (`Eklentiler/forum`, Özel Chat gibi), kategori
   `communication`, sürüm 1.0.0. konu_ac AND'la gelmekten çıkar.

## 3. aşama — Site (1.3.1 → 1.4.0)

| Ekran (TUI) | Masaüstü |
|---|---|
| Liste | Araçlar: "Yeni sayfa" (birincil, `n`), "Taslaklar (N)". Gövde: sayfa `liste`si (başlık, alt `/slug`, sağda zaman). Satır düğmeleri: Düzenle, Yukarı, Aşağı (sıra), Sil (tehlikeli). Boşken "Henüz sayfan yok" + Yeni sayfa |
| Şablon | Diyalog: şablon listesi ("Boş sayfa" ilk), seçince Düzenle |
| Düzenle | Form: Slug (zorunlu, kod gibi), Başlık, Gövde (`metinAlani`, sınır = sayfa sınırı, sayaç). Araçlar: "Yayınla" (birincil, Ctrl+S), "Taslak olarak sakla", "Vazgeç". Kirli formdan çıkış onay ister |
| Taslaklar | `liste`: başlık, slug, zaman; düğmeler Aç, Sil |
| Sil onayı | `Onay` diyaloğu (tehlikeli) |

Gövde masaüstünde DeltaX'teki gibi bubbles textarea'sını atlar (sekme karakterleri korunur).

## 4. aşama — Yönetici Paneli (4.1.0 → 4.2.0)

`sekmeCubugu`: Konular, Yasaklar, Geçmiş, Moderatörler (yalnızca kurucu). Her sekme bir `tablo`.

| Sekme | Sütunlar | Satır düğmeleri / araçlar |
|---|---|---|
| Konular | Başlık, Kategori, Yazar, Tarih (aranabilir) | Sil (onay), Yazarı yasakla (diyalog: süre seçimi + gerekçe) |
| Yasaklar | Kullanıcı, Kalan süre, Gerekçe, Yasaklayan | Yasağı kaldır (onay). Araç: "Anahtarla yasakla" (anahtar, süre, gerekçe) |
| Geçmiş | Zaman, İşlem, Hedef, Moderatör | — |
| Moderatörler | Moderatör, Süre, Durum | Yetkiyi geri al (onay). Araç: "Moderatör ata" (anahtar, süre) |

Yeni uç: `POST /api/v1/mod/ban` (`moderation` izni) — gövde `{author_key, duration, reason}`; süre
TUI'deki hazır sürelerden biri. Yetki denetimi ve yayın mantığı TUI'den çekirdek bir pakete
(`moderation`) taşınır; TUI `app.go` ve API aynı işlevi çağırır.

## Forum eklentisi

### Çekirdekte kalan / kalkan

Kalır: `internal/forum` (depolama, eşitleme, imza, hız sınırı, silme kayıtları), moderasyon, ağ, ana
sayfa sayıları, profil istatistikleri, sohbetteki profil/yasak ekranı.

Kalkar: TUI `forumModel` (`internal/tui/forum.go`) ve `taslak.go`, ana menünün Forum dalı (yerine
eklenti girdisi), gizli konu_ac'ı `n` ile açma; masaüstünde konu listesi/konu/yeni konu çizimi,
`forumOlayi`, `/api/forum/konu` ve `/api/forum/yanit` uçları, `konuAcabilir`. Ctrl+K konu araması için
`GET /api/forum` okuma ucu kalır (yalnızca forum eklentisi etkinken kullanılır). TUI'de forumdan
bağımsız kullanılan yardımcılar (`forumNeZaman`, `profilMetni`, yasak işlevleri) yerinde kalır ya da
paylaşılan yere taşınır.

### Eklenti API'si eklemeleri (geriye uyumlu)

- `PostSummary`'e `created_at`, `last_activity`, `reply_count`, `author_role` (`omitempty`).
  `PostDetail` ve `ReplyInfo`'ya `author_role`. (Mühür için alan gerekmez: AND her konu ve yanıtı
  saklamadan önce imzasını doğrular, masaüstü bugün mührü her gönderide gösterir.)
- `GET /api/v1/forum/changes?since=N` (temel izin): forum değiştikçe artan bir sürüm sayısı. `N`
  güncelse en çok 25 sn bekler; yanıt `{surum, konular: [id], hepsi}` (çok değişiklikte `hepsi`).
- `POST /api/v1/forum/delete-own` (`forum` izni): kendi konusunu siler (`DeleteOwnPost`).
- `GET /api/v1/profile?key=` (temel izin): ad, katılım, konu/yanıt/sohbet sayıları, rol; terminal
  profil ekranının gösterdikleri.

### Protokol sürüm 2 (geriye uyumlu)

- AND `merhaba`'da `surum: 2` gönderir. SDK `Merhaba.Surum`'u modele iletir; eklenti v2 mesajlarını
  yalnızca `Surum >= 2` iken yollar.
- AND bundan sonra tanımadığı mesaj türünü yok sayar (oturumu düşürmez, bir kez günlükler).
- Yeni bileşen **`gonderi`**: `id`, `yazar`, `yazarAlt` (kısa anahtar), `kendi`, `rozetler` (rol),
  `zaman`, `muhur` (imza doğrulandı; forum her gönderide doğru verir), `govde` (alt öğeler: `zengin`,
  `kod`), `dugmeler`, `yanit` (iplik görünümü). Masaüstü bugünkü `yazarSatiri` ve konu/yanıt CSS'ini
  kullanır; v1 AND'da bilinmeyen öğe olarak görünür, bu yüzden eklenti yalnızca `Surum >= 2` iken
  kullanır, aksi hâlde `kart` + `zengin` ile çizer.
- Eklentiden AND'a **`menu`**: `{"tur":"menu","ogeler":[{id, etiket, ikon, sayi}]}` (≤ 50). AND
  eklentinin kenar çubuğu girdisinin altında ve Ctrl+K'de gösterir.
- AND'dan eklentiye **`git`**: `{"tur":"git","hedef":"..."}`. Menü tıklaması, Ctrl+K konusu ve eski
  `#forum/<id>` bağlantıları bununla gelir. SDK: `arayuz.Git{Hedef}` mesajı, `arayuz.Menu(...)` komutu.
  Forumun hedefleri: `kategori:<ad>`, `konu:<id>`.

### Ekranlar

| Ekran | Terminal | Masaüstü |
|---|---|---|
| Kategoriler | bugünkü TUI | Ayrı ekran yok (bugünkü gibi): kenar çubuğunda `menu`, araç çubuğunda kategori `secim`i |
| Konu listesi | bugünkü TUI | `liste`: başlık, alt (yazar · kategori), sağda son etkinlik, yanıt rozeti; kategori seçiliyse süzülür. Araçlar: kategori `secim`i, "Yeni konu" (birincil, `n`) |
| Konu | bugünkü TUI | `gonderi` + yanıtlar (`gonderi`, `yanit`), altta yanıt formu (taslaklı, Ctrl+Enter). Kendi konusunda "Sil" (onay) |
| Yeni konu | konu_ac ekranı | Form: kategori `secim`, başlık, gövde; kategori başına taslaklar |
| Yazar profili | bugünkü TUI (yasak yok) | `kart` + `anahtarDeger` |

Gövdeler BBCode'dur; eklenti `internal/bbcode` ile `zengin` parçalara ve `kod` bloklarına çevirir.
Liste 2000 satırı geçerse "Daha fazla" ile sayfalanır. Canlı güncelleme `forum/changes` ile; ekran
görünmüyorken de liste güncel tutulur, konu yalnızca görünürken yenilenir.

### Geçiş

- İlk açılışta AND'ın `yanit_taslaklari.json`'u ve konu_ac'ın `taslaklar_<kategori>.json` dosyaları
  eklentinin veri dizinine kopyalanır (asılları silinmez; bir işaret dosyası tekrarını önler).
- Masaüstü yanıt taslakları bugün yalnızca bellekte (`D.forum.taslaklar`) tutulur, pencere kapanınca
  kaybolur; aktarılacak bir şey yoktur. Eklentiyle birlikte masaüstü taslakları da diske yazılır.
- konu_ac AND'la gelen eklentilerden çıkar (`yerlesikAdlar`, sürüm iş akışı, belgeler). Ayrı deposuna
  "Forum eklentisine katıldı" README notu commit'lenir.
- Belgeler: EKLENTILER.md/PLUGINS.md (forum eklentisi, protokol v2: `gonderi`, `menu`, `git`), README.

## Sıra ve teslim

1. Site arayüzü. 2. Yönetici Paneli arayüzü + `mod/ban`. 3. API eklemeleri + protokol v2. 4. Forum
eklentisi (terminal + masaüstü, konu_ac'ın katılması, taslak geçişi). 5. Çekirdekten forum
ekranlarının kaldırılması, yönlendirme, açılış görünümü, belgeler. Her adım ayrı commit dizisi, push
yok. exe'ler kullanıcının onayıyla en sonda yeniden derlenir (and, forum, site, admin).

## Doğrulama

- Her eklenti: `arayuztest` ile ekran/olay testleri ve 6 dilde ekran testi; terminal testleri yeşil.
- API: yeni uçların izin tablosu testi (`TestRouteTableIzinleri`), `mod/ban` yetki testleri,
  `forum/changes` bekleme/zaman aşımı, `delete-own` başkasının konusunu silemez.
- Protokol: v1 eklenti v2 AND'da çalışır; v2 eklenti `Surum 1`'de `menu` yollamaz; bilinmeyen tür
  oturumu düşürmez.
- Masaüstü: başsız Edge ile Site, Yönetici, Forum ekran görüntüleri; forum eski görünümle yan yana
  karşılaştırılır. Kenar çubuğu kategorileri, Ctrl+K konu araması, `#forum/<id>` yönlendirmesi.
- Tam `go test ./...`, vet (3 OS), gofmt, LF/CRLF klonları, govulncheck, lisans bildirimleri.

## Builder'a kalanlar

İkon seçimleri (mevcut 30 ikondan), `gonderi`'nin CSS ayrıntısı, sayfa boyu, `changes` iç yapısı,
Ctrl+K'nin konu sonuç sayısı, TUI ana menüde forumun yeri (kategori sırası içinde).

## Riskler

Forum en çok kullanılan ekran; gerileme riski en büyük burada. Veri katmanına dokunulmaz, TUI forum
testleri eklentiye taşınır, masaüstü önce/sonra ekran görüntüleriyle karşılaştırılır.
