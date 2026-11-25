# Tasarım: web sitesi ve GitHub organizasyonu AND'ın bugünkü hâline

Tarih: 2026-11-25. İstek: "and organizasyonun kısmınındaki web kısmını ve web siteyi güncele".

## Niye

Site (https://and-community.github.io) 2026-11-01'de yazıldı. O günden beri AND çok değişti ve site
ile organizasyon sayfası artık yanlış şeyler söylüyor:

- AND'ı "terminalde çalışan" bir uygulama diye anlatıyorlar. Oysa AND artık varsayılan olarak kendi
  masaüstü penceresiyle açılıyor; terminal arayüzü `--terminal` ile hâlâ var.
- Konu açmak için konu_ac'yi ayrıca klonlayıp derlemeyi söylüyorlar. Forum artık AND deposundaki
  forum eklentisi (`Eklentiler/forum`) ve konu_ac onun içine katıldı.
- Eklentilerin "aynı terminalde" açıldığını söylüyorlar. Masaüstünde eklentiler artık AND'ın
  penceresinde çalışıyor; arayüz protokolünü kullananlar AND'ın kendi bileşenleriyle çiziliyor.
- Sürümler, tema sayısı (6 → 10 + özel renk), eklenti kurulumu (`and plugin install/new`) ve İngilizce
  eklenti rehberi (PLUGINS.md) yer almıyor.

Başarı: siteye ya da organizasyon sayfasına gelen biri AND'ı indirdiğinde gördüğüyle karşılaşır ve
sitedeki her iddia koda uyar.

## Kullanıcının verdiği kararlar

- Sitenin girişindeki canlı kopya **masaüstü uygulamasını** gösterecek (terminal ana menüsü kopyası
  kalkar; terminal arayüzü metinde anılır).
- GitHub'daki **konu_ac deposu arşivlenecek** (ozel_chat'e yapıldığı gibi).

## Yaklaşımlar (kopyanın nasıl yapılacağı)

1. **Elle yazılmış HTML/CSS kopya, üreticide (seçilen, benim kararım).** Bugünkü terminal kopyası gibi
   `tools/olustur.js` içinde R tablosundan üretilir; renkler, ölçüler ve metinler gerçek uygulamadan
   (`internal/masaustu/web/uygulama.css`, `dil/ceviriler/tui.json`) alınır. Sitenin yapısına uyar,
   statiktir, sunucu istemez, iki dil aynı şablondan çıkar.
2. *Gerçek arayüzü (`uygulama.js`) sahte bir API ile gömmek.* En sadık kopya olurdu ama 2600 satırlık
   JS'yi, SSE'yi ve sunucu varsayımlarını siteye taşır; uygulamadaki her değişiklik siteyi kırabilir.
   Elendi.
3. *Tıklanabilir ekran görüntüsü.* En ucuzu ama temalar çalışmaz ve "canlı kopya" fikri kaybolur. Elendi.

## Hedefler

### A. Giriş bölümündeki kopya: AND'ın masaüstü penceresi

Referans: gerçek uygulamanın önizlemesinden alınan ekran görüntüleri (forum, konu, sohbet,
Ayarlar › Görünüm; önizleme tarifi [[and-masaustu-arayuzu]] belleğinde, `ONIZLEME_ARAYUZ` +
`ONIZLEME_ARAYUZ_AD=forum`).

Pencere:
- İnce bir pencere başlığı (küre + "AND", sağda süs amaçlı — □ × ). Gerçek uygulamada başlığı
  Windows çizer; burada "bu bir masaüstü uygulaması" dedirtmek için var (benim kararım).
- Kenar çubuğu (248 px): logo ve "AND"; "Ara ya da git… Ctrl K" kutusu (süs; tıklayınca bir şey
  yapmaz ya da aramayı forumda süzer, kurucuya kalmış); bölümler gerçek uygulamadaki sırayla:
  Topluluk (Forum ve altında kategoriler + sayılar, Sohbet), İletişim (Özel Chat, okunmamış rozeti),
  İçerik (Site), Yönetim (Yönetici Paneli), Geliştirme (DeltaX); altta Ayarlar, AND hakkında ve
  profil satırı (avatar, "umut", "Profil"). Çalışan eklentinin yanında yeşil nokta.
- 52 px'lik araç çubuğu her ekranın başında (ikon + başlık, sağda o ekranın düğmeleri).
- Durum çubuğu: solda ağ durumu ("● 23 eş bağlı · 4 AND düğümü"), sağda "AND v0.1.0".

Ekranlar (kenar çubuğundan tıklanır, seçili öğe vurgu çizgisiyle gösterilir):
1. **Forum** (açılış): solda süzme kutusu ve konu listesi (başlık; kategori · yazar · zaman; yanıt
   sayısı rozeti; seçili konuda sol vurgu çizgisi), sağda okuma alanı (başlık, yazar avatarı + adı +
   zaman + imza mührü, gövde, `[code]` bloğu, yanıtlar iplik çizgisiyle). 3–4 konunun içeriği vardır;
   listeden tıklanınca okuma alanı değişir. Kenar çubuğundaki kategori listeyi süzer. Araç çubuğunda
   "Tüm konular" seçimi ve "Yeni konu" düğmesi (süs).
2. **Sohbet**: gün ayıracı, birkaç ileti, altta "Herkese açık bir ileti yaz…", sayaç, Gönder (süs).
3. **Özel Chat**: solda konuşmalar (okunmamış sayıları), sağda ileti akışı ve giriş kutusu.
4. **Site**: sayfa listesi ve seçili sayfanın önizlemesi.
5. **Yönetici Paneli**: yasak listesi (ad/anahtar, gerekçe, kalan süre).
6. **DeltaX**: bir projenin durum kartı ve son commit'ler.
7. **Ayarlar**: sekmeler Profil / Görünüm / Dil / Eklentiler / Ağ. Görünüm sekmesi çalışır: 10 tema
   kartı (gerçek uygulamadaki küçük ağ haritası çizimiyle); karta tıklamak temayı değiştirir.
   Eklentiler sekmesi kategorilere ayrılmış eklenti listesini gösterir. Öteki sekmeler kısa birer
   içerik gösterir ya da yalnızca sekme olarak görünür (kurucuya kalmış).
8. **AND hakkında**: Genel sekmesi (sürüm, lisans, kısa tanım).

Davranış:
- Tema seçimi bugünkü gibi bütün sayfayı ve kopyayı boyar ve localStorage'da kalır. Kopyanın altındaki
  tema düğmeleri 10 temaya çıkar; kopyanın içindeki Ayarlar › Görünüm kartları aynı seçimi yapar.
- Klavye: kenar çubuğu gerçek düğmelerden oluşur (Tab ile gezilir, odak halkası görünür); konu listesi
  de öyle. ↑↓ desteği bugünkü kopyada varsa kenar çubuğunda korunur.
- Kopyanın içinde uygulamanın yazı aileleri kullanılır (Segoe UI Variable → system-ui; kod için
  Cascadia Mono → ui-monospace). Sitenin geri kalanı kendi Mono/Slab yazılarıyla kalır.
- Dar ekran: pencere sayfa genişliğine sığar, yatay kaydırma olmaz. Yaklaşık 760 px altında kenar
  çubuğu yalnızca ikonlardan oluşan bir şeride iner; forumda liste ile okuma alanından biri görünür
  (konuya girince okuma, geri düğmesiyle liste). 390 px'te de kullanılabilir olmalı.
- Kopyanın altındaki not: "Bu, AND'ın masaüstü penceresi; uygulamanın kendi renkleri ve metinleriyle
  HTML'de çizildi. Kenar çubuğundan bir ekran seç. AND'ın tam ekran bir terminal arayüzü de var:
  `and --terminal`." (EN ve TR).
- Tek hareket anı korunur ya da yenisine dönüşür: açılışta eşlerin bağlanması (bugünkü) → durum
  çubuğundaki eş sayısının artması gibi küçük bir şey. Abartılı animasyon yok.

Metinler R tablosunda EN ve TR olarak durur; AND'ın çeviri kataloğunda karşılığı olanlar oradan
birebir alınır.

### B. Sayfa metinleri (M tablosu, EN ve TR)

Her iddia koda karşı doğrulanır (ilk sitedeki kural). Değişecek yerler:
- `aciklama`, `ozet`, `ogAlt`, alt bilgi ve `tools/og.html`: "terminalde çalışır" yerine "kendi
  penceresinde (ya da terminalde) çalışır". OG resimleri (og.png, og-tr.png) metin değişirse yeniden
  alınır.
- `ekranBaslik`, `ekranNot`: masaüstü penceresi.
- Özellikler: yeni satır "Masaüstü ve terminal"; "Eklentiler" satırı: AND'ın penceresinde çalışır,
  kategorilere ayrılır, `and plugin install` ile dışarıdan kurulur; "Temalar ve diller": 10 tema ve
  özel renk; "Arama" satırı aynen kalır (arama motoru gerçekten terminal uygulaması); yeni "VS Code"
  satırı (canlı Forum sekmesi, AND'daki ayarla açılan bağlantı) — benim kararım: uzantı
  ekosistemin parçası ve sitede hiç anılmıyor.
- Nasıl çalışır › son paragraf: ağdaki yenilikler doğru anlatılıyor mu kontrol edilir (relay, eş
  değişimi, bilinen eşler); gerekirse bir cümle eklenir.
- AND'ı edin: giriş cümlesi (Windows'ta kendi penceresi; Linux/macOS'ta varsayılan tarayıcıda açılır;
  `--terminal` ile terminalde). Kaynaktan derleme: konu_ac klonu çıkar; komutlar AND, forum eklentisi
  (ve istenirse öteki eklentiler) olur. Go sürümü go.mod'daki `go` satırıyla aynı (1.26.4).
- Güvenlik: "Terminalin" satırı genişler: masaüstü arayüzü yalnızca 127.0.0.1'de, tek kullanımlık
  açılış koduyla ve sıkı CSP ile sunulur; ağdan gelen içerik HTML sayılmaz; terminalde kaçış dizileri
  temizlenir. "Eklentiler" satırına: arayüz protokolüyle gelen ekranlar AND tarafından doğrulanır.
- Projeler: AND satırı forum eklentisini de anar; konu_ac satırı "arşivlendi; forum eklentisine
  katıldı" olur.
- Sorular: "Hangi sistemlerde çalışır?" masaüstü/tarayıcı/terminal gerçeğine göre; "Şifremi
  unuttum" ve "Kurtarma kodumu kaybettim" masaüstündeki gerçek yollara göre (kod okunarak doğrulanır;
  masaüstünde Bilgi sekmesi yok); "Kendi eklentimi yazabilir miyim?" → `and plugin new` ve PLUGINS.md
  (EN) / EKLENTILER.md (TR).
- `GUNCELLEME` tarihi ve sitemap.

### C. Site deposunun kendi belgeleri

`README.md` ve `README.tr.md`: kopyanın artık masaüstü penceresini (`internal/masaustu/web`) taklit
ettiği, uygulama değişince nerenin güncelleneceği.

### D. GitHub organizasyonu

- `AND-community/.github` → `profile/README.md` (EN) ve `profile/README.tr.md` (TR):
  - Üst kısım: "terminal-native" yerine masaüstü + terminal.
  - "What is AND?": "All you need is a terminal…" cümlesi.
  - Özellik tablosu: masaüstü arayüzü satırı; Plugins satırı (pencere içinde, arayüz protokolü,
    kategoriler, dışarıdan kurulum).
  - Depolar tablosu: AND satırına forum eklentisi; konu_ac "Archived. Merged into AND's Forum plugin".
  - Getting started: konu_ac klonu yerine forum eklentisi.
  - Plugin system bölümü baştan: pencere içinde çalışma, `"ui"` alanı, `"category"`, `and plugin new` /
    `install`, PLUGINS.md (İngilizce rehber).
  - Security model: masaüstü yerel sunucusu.
  - Tech stack: masaüstü katmanı (WebView2 / go-webview2, xterm.js).
- Organizasyon açıklaması (`PATCH /orgs/AND-community`): "terminal forum community" yerine masaüstü +
  terminal.
- Depo açıklamaları ve konuları: AND (açıklama; konulara `desktop` eklenir, `terminal`/`tui` kalır),
  and-community.github.io (açıklama: masaüstü penceresinin canlı kopyası), konu_ac (açıklama "Archived:
  merged into AND's Forum plugin" ve arşivleme). Arama motoru, DeltaX, Pembis olduğu gibi.

### E. AND'ın README'si (kullanıcı kapsama kattı)

`README.md` ve `README.tr.md` (ikisi birlikte): ilk satırlardaki "terminal forum" tanımı (satır 7 ve
34 civarı) masaüstü + terminal olur; "Desktop interface" bölümündeki eskimiş maddeler düzelir
(eklentiler AND'ın penceresinde çalışır, arayüz protokolü olanlar AND'ın bileşenleriyle çizilir;
forum artık çekirdek ekranı değil, forum eklentisi). Başka eskimiş iddia bulunursa aynı turda
düzeltilir. Bağlantı çapaları (site ve profil README'ye bağlanıyor) bozulmaz.

### F. Yayın

- Site: üret (`node tools/olustur.js`), yerelde doğrula, commit, `git push origin main` (site
  dakikalar içinde canlanır), canlı sitede doğrula.
- Organizasyon profili: geçici klon, commit, push, GitHub'da kontrol.
- API değişiklikleri (açıklamalar, konular, arşiv) saklı kimlik bilgisiyle; sonra kimlik bilgisi
  olmadan doğrulanır. Arşivlenmiş depo salt okunur olduğundan konu_ac'nin açıklaması arşivlemeden
  ÖNCE değiştirilir.
- AND README değişikliği AND deposunda commit'lenir ve `git push origin main` ile gönderilir.
- Sıra: önce AND README (site ve profil oraya bağlanıyor), sonra site, sonra profil, en son
  açıklamalar ve konu_ac arşivi.

## Hedef olmayanlar

- Ctrl+K hızlı erişim paletinin kopyası, kopyada gerçek yazma/gönderme.
- Yeni sürüm (tag) yayımlamak; site "sürüm yok" durumunda bugünkü gibi kaynaktan derlemeye düşer.
- Arama motoru, DeltaX ve Pembis depolarının açıklamaları.

## Kesinlikle olmaması gerekenler

- Varsayılan görünümün Klasik mavi-mor olmaktan çıkması (kullanıcının açık tercihi).
- Koda uymayan bir iddia (ör. var olmayan bir kısayol, Linux'ta yerel pencere).
- Telefonda yatay kaydırma ya da kopyanın sayfayı taşması.
- Sitenin dış kaynak yüklemeye başlaması (bugün yalnızca GitHub API'si çağrılıyor; çerez ve analiz yok).
- Kopyanın içinde gerçek uygulamanın dosyalarına çalışma anında bağımlılık.

## Kısıtlar

- Statik site, GitHub Pages (main, kök, `.nojekyll`). Sayfalar üretilir; elle HTML düzenlenmez.
- Yazı tipleri: sitenin kendi alt kümeleri (Monaspace türevi, yeniden adlandırılmış). Kopya sistem
  yazılarını kullanır; yeni web yazısı eklenmez.
- İki dil aynı şablondan çıkar; yapıları ayrışmaz.
- Push yalnızca `git push origin main`, kullanıcı istediği için (bu istek yayını kapsıyor).

## Kurucuya kalanlar

- Kopyadaki örnek veri (konular, adlar, iletiler): bugünkü kopyanın İngilizce/Türkçe örnekleri ve
  `KISI` renkleri temel alınır; gerçekçi ve kısa.
- İkonlar: gerçek uygulamadaki ikon çiziminin (SVG yolları `uygulama.js`'te) aynısı ya da sadeleşmiş
  hâli.
- Hareket anının ayrıntısı, dar ekrandaki kırılma noktaları, öteki Ayarlar sekmelerinin içeriği.
