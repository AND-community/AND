# Değişiklik Günlüğü

## 0.7.1

- DeltaX **üye davet** listesi artık yalnızca AND düğümlerini önerir. Bağlı eşlerin çoğu genel DHT
  düğümüdür ve DeltaX mesajı alamaz; AND bunları eş listesinde `and: true` ile ayırt eder. Bu bilgiyi
  göndermeyen eski bir AND'da liste eskisi gibi tüm eşleri gösterir. Elle Peer ID girme seçeneği her
  durumda açık.

## 0.7.0

**Canlı Panel yeniden tasarlandı.** AND ekosisteminin geri kalanıyla (TUI, eklentiler, Arama Motoru)
birlikte yenilenen görsel dilin VS Code karşılığı:

- Yatay sekme şeridinin yerine **sol gezinme rayı**: ikonlu bölümler, bağlı eş sayısı ve site sayfası
  sayacı (N/20), bağlantı durumu noktası ve Yenile düğmesi. Panel 560 px'ten darsa ray yalnızca
  ikonlara iner.
- Yüzeyler, metin ve form öğeleri **VS Code'un kendi tema belirteçlerinden** gelir, dolayısıyla panel
  koyu, açık ve yüksek karşıtlıklı temaların hepsinde yerli görünür. AND'ın **etkin teması** (TUI'de
  Ayarlar → Temalar'da seçilen) marka işaretinde, etkin bölümde ve birincil düğmelerde kullanılır.
  Düğme yazısının rengi temanın vurgusuna göre siyah ya da beyaz seçilir; açık bir vurguda beyaz yazı
  okunmaz kalmaz. AND'ın teması okunamazsa panel AND'ın varsayılan **Klasik** (mavi-mor) renklerini
  kullanır. TUI'de tema değişince panel, açık olduğu sürece 5 saniye içinde kendiliğinden yeni
  renklere geçer; Yenile'ye basmak gerekmez.
- **Sohbet ve DM** mesaj akışı biçiminde: baş harfli avatar, saat ve kişi başına sabit bir renk tonu
  (aynı kişi her yerde aynı renkte). Kendi mesajların vurgu renginde görünür. **Enter gönderir.**
- **Kimlik:** Peer ID ve genel anahtar eş aralıklı yazıyla gösterilir, yanlarında **kopyala**
  düğmesi var. Ağ sekmesindeki eşlerde de aynı düğme bulunur.
- Forum konusu yanıtlarıyla birlikte ayrı bir çerçevede açılır; boş listeler ne yapılacağını söyleyen
  bir metin gösterir; bildirim bandı panelin üstüne yapışık kalır.
- Panelin HTML'i ve betiği `vscode` modülüne bağımlı olmayan ayrı bir dosyaya (`src/live/panelHtml.ts`)
  taşındı ve **birim testleri** eklendi (`panelHtml.test.ts`). Testler betiği sahte bir DOM üzerinde
  çalıştırıp mesaj işleyicilerini, HTML kaçırmayı, sohbet birleştirmeyi, Enter ile göndermeyi,
  kopyalamayı ve tema değişkenlerini sınıyor.

## 0.6.0

**Kişisel site artık VS Code'dan da yönetilebiliyor** — AND'a eklenen kişisel site özelliğinin
(`internal/site`, `Eklentiler/site`) `/api/v1/site/*` uç noktaları uzantıya taşındı. Sayfalar bir form
kutusunda değil, `and-site-edit:` sanal dosya sistemi üzerinden **gerçek bir VS Code editöründe** açılır;
kaydetmek (Ctrl+S), TUI'deki site eklentisinin `ctrl+s`'i gibi sayfayı ağa yayınlar.

- Canlı Panel'e **Site** sekmesi: sayfa listesi (N/20, son güncelleme, bayt kullanımı, önizleme),
  Yeni Sayfa / Düzenle / Paylaşım Kodunu Kopyala / Sil.
- Komutlar: "AND: Yeni Site Sayfası", "AND: Site Sayfası Aç…", "AND: Site Sayfasını Sil…",
  "AND: Site Paylaşım Kodunu Kopyala" (ayrıca site editörünün başlığında bir düğme). Paylaşım kodu
  (`yazarAnahtarı/slug`) ve `and-site://` bağlantısı, AND Arama Motoru'nun "Kodla Git" ekranı/`-site`
  bayrağıyla birebir uyumludur.
- Canlı bayt sayaçları (durum çubuğu) ve sınırı aşan kısmı tam aşıldığı karakterden itibaren işaretleyen
  tanılamalar — AND'ın sınırlarıyla aynı, karakter değil bayt sayar. Kurala uymayan bir kayıt ağa hiç
  gönderilmez.
- Aynı sayfanın ardışık yayınları en az 6 sn aralıkla gönderilir — AND eşleri 5 sn'den sık gelen canlı
  güncellemeleri düşürdüğü için (`minPageEditInterval`) bir düzenleme sessizce kaybolmaz.
- Otomatik kaydetme bu sayfalar için varsayılan olarak kapalıdır (yeni `and-site` dili) ve açılsa bile
  yayın yapmaz — yazarken yarım bir taslak ağa yayılmaz.
- Sayfa başka bir istemciden değişirse açık editörler bunu öğrenir (değişiklik olayları + açık site
  editörü varken 15 sn'de bir yeniden okuma); VS Code'un yerleşik çakışma akışı çalışır.
- **Sohbet geçmişi:** Sohbet sekmesi artık açılışta boş başlamıyor — `/api/v1/chat/all`'dan son 200
  mesaj yüklenir ve canlı gelenlerle tekrarsız birleştirilir (aynı an farklı saat dilimi ofsetiyle
  gelse bile).
- Webview'daki HTML kaçırma (`esc`) artık tırnakları da kaçırıyor — uzak eşlerden gelen değerler
  `data-*`/`value` özniteliklerine de yazıldığı için.

Go tarafında bu sürümle birlikte iki düzeltme yapıldı (VS Code'dan düzenleme bunları sık tetikleyeceği için):
`internal/site` senkronizasyonu artık bir sayfanın **düzenlenmiş** halini de geriye dönük getiriyor
(önceden yalnızca hiç bilinmeyen sayfalar gelirdi), ve silinmiş bir slug'ı yeniden yayınlamak artık
"yayınlandı" deyip sessizce kaybolmak yerine açık bir hata döndürüyor.

## 0.5.0

**DeltaX grup senkronizasyonu artık VS Code'dan da yapılabiliyor** — önceden "kapsam dışı" olarak
işaretlenip yalnızca AND TUI'sinden yapılabilen grup yaşam döngüsü tamamen taşındı:

- Yeni grup oluşturma (`deltax.createGroup`).
- Üye davet etme (`deltax.inviteMember`, yalnızca kurucu/yönetici) — bilinen eşler arasından seç ya
  da Peer ID'yi elle gir; gelen davetler bildirim üzerinden Kabul Et/Reddet ile yanıtlanır.
- Yönetici atama/kaldırma (`deltax.manageAdmins`, yalnızca kurucu).
- Gruptan çıkma / grubu kapatma (`deltax.leaveOrCloseGroup`).
- Gönder — push (`deltax.pushProject`, tek üyeye ya da "Çevrimiçi herkese") ve Çek iste — pull
  (`deltax.pullFromMember`).
- Gelen paketler (bilinen bir grup üyesinden) arka planda otomatik doğrulanıp uygulanır —
  `proje.uzaklar` güncellenir, elle bir şey yapmaya gerek kalmaz.

Bunun için `Eklentiler/DeltaX/sync.go`'daki `AND-PROJEAG/1` protokolü (zarf kodlama, davet/roster/
push/pull DM mesajları, AES-256-GCM şifreli tar.gz paketleri) `src/live/deltax/` altında birebir
TypeScript'e taşındı — tar+gzip için bağımlılıksız, standart USTAR uyumlu bir okuyucu/yazıcı
(`tarball.ts`) yazıldı. Uyumluluk, gerçek Go `archive/tar`/`compress/gzip`/`crypto/aes` çıktısı ile
çift yönlü (Go→TS ve TS→Go) doğrulandı. **Go tarafında hiçbir değişiklik gerekmedi** — mevcut
`internal/pluginapi` DM/dosya aktarımı uç noktaları yeterliydi. AND TUI'siyle tamamen birlikte
çalışır: ikisi de aynı `gruplar/`/`projeler/` JSON'larını ve aynı DM protokolünü kullanır.

## 0.4.1

- Activity bar ikonu (`resources/activitybar.svg`) değiştirildi: eski küp/kutu şekli anlamsız bir
  kare gibi görünüyordu. Yeni ikon "DeltaX" adını çağrıştıran bir üçgen (Δ) + içinde "X" işareti.

## 0.4.0

- **AND tema senkronizasyonu:** Canlı Panel artık açılışta `GET /api/v1/theme`'i okuyup
  (pembis eklentisiyle değiştirilmiş olabilecek) aktif AND temasını uygulıyor — vurgu, seçim,
  soluk ve rozet renkleri xterm-256/hex'ten CSS'e çevrilip panelin başlık, sekme ve buton
  renklerine yansıtılıyor (VS Code'un kendi teması her zaman zarif bir geri dönüş olarak kalıyor).
- **Arka plan bildirimleri:** DM ve dosya aktarımı için sohbet/DM/dosya uzun-poll döngüleri artık
  Canlı Panel kapalıyken de (`BackgroundSync`) çalışmaya devam ediyor. Panel kapalıyken gelen bir
  DM ya da dosya isteği artık VS Code bildirimi (DM için "Görüntüle", dosya isteği için doğrudan
  "Kabul Et"/"Reddet") ve durum çubuğunda okunmamış sayacıyla ($(bell) N) bildiriliyor; panel
  açıldığında sayaç sıfırlanır. Genel sohbet, gürültü yapmaması için bildirim üretmez.
- **Editör içi kısayollar:** "AND: Seçili Kodu Forumda Paylaş" (editör sağ tık/komut paleti —
  seçili metni ya da tüm dosyayı bir forum konusu olarak, dil etiketli kod bloğu içinde paylaşır)
  ve "AND: Dosyayı Bir Eşe Gönder" (gezgin sağ tık — bağlı eşler listesinden seçip ya da Peer ID
  elle girip dosyayı `/api/v1/file/send` ile gönderir) komutları eklendi.

## 0.3.0

- Canlı panele **Dosyalar** sekmesi eklendi: bir eşe yerel dosya seçip gönderme (`/api/v1/file/send`),
  gelen dosya onay isteklerini kabul/reddetme (`/api/v1/file/consent-poll` + `/api/v1/file/consent`)
  ve alınan dosyaları (kaydedildikleri konumu sistem dosya yöneticisinde açma dahil) listeleme
  (`/api/v1/file/poll`) — AND'ın P2P dosya aktarımı artık DeltaX'in yanı sıra VS Code'dan da kullanılabiliyor.
- Peer ID girişleri (Dosyalar sekmesi) artık Ağ/Eşler sekmesinden gelen bağlı eş listesiyle
  otomatik tamamlanıyor (`datalist`).

## 0.2.5

**İlk gerçek derleme doğrulaması** (önceki tüm turlar yalnızca elle kod incelemesiydi — bu ortamda
Node.js kurulu değildi; artık kuruldu ve gerçek `tsc`/`esbuild`/`vsce` ile doğrulandı):

- **Düzeltildi:** `mergeEngine.ts`'te `tsc` tarafından yakalanan gerçek bir tip hatası —
  `Buffer.alloc(0)`'dan çıkarsanan dar `Buffer<ArrayBuffer>` tipi, `readObject()`'in döndürdüğü
  daha geniş `Buffer<ArrayBufferLike>` ile uyuşmuyordu. `lData`/`rData` artık açıkça `Buffer` tipiyle
  tanımlanıyor.
- `npm run typecheck`, `npm run build` ve `npm run package` uçtan uca başarıyla çalıştı;
  üretilen `.vsix` VS Code'a gerçekten kuruldu ve kabul edildi (`and-ekosistemi.and-vscode@0.2.5`).
- `LICENSE`, `repository`/`bugs`/`homepage` alanları eklendi (Marketplace listelemesi için).
- Bilinen, düşük öncelikli durum: `npm audit`, yalnızca geliştirme bağımlılıklarında (esbuild'in
  geliştirme sunucusu, vsce'nin markdown linter'ı) 4 uyarı gösteriyor — bunlar paketlenen koda
  hiç girmiyor, üretim/son kullanıcı riski yok; düzeltmeleri büyük sürüm atlamaları gerektiriyor.

## 0.2.4

- Uzantı klasörü/paket adı `vscode-deltax`/`deltax-vscode`'dan `and-vscode`'a değiştirildi —
  uzantı artık yalnızca DeltaX'i değil AND'ın tüm canlı özelliklerini de kapsadığından
  ("AND" görünen adıyla) daha genel bir isimlendirmeye geçildi.
- Marketplace logosu eklendi (`resources/icon.png`, `package.json`'da `icon` alanı).

## 0.2.3

Üçüncü bir kod incelemesi turunda bulunanların düzeltmesi:

- **Güvenlik — düzeltildi:** `pathWithinDir` artık `filepath.EvalSymlinks` ile sembolik bağları da
  çözüyor — önceden, `AND_DATA_DIR` dışında duran ama içindeki bir dosyaya (ör. `identity.dat`)
  işaret eden bir sembolik bağ, dosya gönderme korumasını tamamen atlatabiliyordu. Yeni
  `TestPathWithinDir` bunu (ve göreli yol/alt dizin durumlarını) regresyona karşı kilitliyor.
- **Veri bütünlüğü — düzeltildi:** `computeStatus()` bir `refresh()` sırasında hata verirse
  `DeltaXRepository` artık durumu sessizce "temiz" olarak sıfırlamıyor — yeni bir `statusError`
  bayrağıyla `isClean()` bu durumda HER ZAMAN `false` döner. Önceden bir hata (ör. geçici dosya
  kilidi, AND TUI'sinin aynı anda yazması), asıl durum hiç doğrulanmadan dal değiştirme/birleştirme
  gibi yıkıcı işlemlerin "temiz" sanılıp devam etmesine yol açabilirdi.
- **İyileştirildi:** `internal/pluginapi/api.go`'da tekrar eden `if x == nil { x = []T{} }`
  deseni tek bir `nonNilSlice[T]` yardımcısına taşındı (getPeers/getPending/getAllPosts).
- **İyileştirildi:** `allowExternal` route filtrelemesini gerçek HTTP istekleriyle doğrulayan yeni
  bir `TestBuildMuxExternalFiltering` testi eklendi — önceden bu mekanizma yalnızca "aspirasyoneldi"
  (hiçbir test, `allowExternal:false` bir route'un harici dinleyicide gerçekten yok sayıldığını
  kanıtlamıyordu).
- **İyileştirildi:** `branchActions.ts`'teki tekrar eden "onay/kontrol sonrası taze durum denetimi"
  bloğu tek bir `recheckCleanBeforeDestructiveAction()` yardımcısına çıkarıldı ve üç yönlü
  birleştirme yoluna da (onay penceresi olmasa dahi) uygulandı; ayrıca `mergeRemoteBranch`'in
  fast-forward yolundaki gereksiz bir tam çalışma dizini taraması kaldırıldı.
- **Düzeltildi:** Log paneli artık bir proje için workspace klasörü değişip geri döndüğünde
  (ör. başka bir projeye geçilip dönüldüğünde) eski, dispose edilmiş bir `DeltaXRepository`
  referansından değil, her zaman güncel repo'dan içerik gösteriyor (`refreshLogPanel` artık
  proje ID'si yerine güncel `repo`'nun kendisini alıyor).
- **İyileştirildi:** `objectStore.ts`'te tip baytı + payload'ı birleştiren kod tek bir
  `taggedBuffer()` yardımcısına toplandı; `disposableBag.ts` artık kendi no-op disposable'ını
  el yordamıyla değil, VS Code SDK'sının `Disposable.from()` idiomuyla oluşturuyor.

## 0.2.2

İkinci bir kod incelemesi turunda (7 açı + doğrulama, üzerine kendi ek gözden geçirmem) bulunanların düzeltmesi:

- **Güvenlik — düzeltildi:** `pathWithinDir` göreli bir `local_path`'i (ör. `"identity.dat"`) mutlak yola
  hiç çözmüyordu; `AND_DATA_DIR` dışına dosya gönderme koruması bu şekilde tamamen atlatılabiliyordu.
  Artık her iki yol da `filepath.Abs` ile çözülüyor.
- **Güvenlik/mimari — düzeltildi:** Token'lı ve token'sız (harici) dinleyiciler artık gerçekten AYRI
  `*http.ServeMux` nesneleri kullanıyor; hangi endpoint'in harici erişime açık olduğu, her route için
  açıkça `allowExternal` alanıyla belirtilen bir tabloda (`routeTable()`), varsayılan olarak KAPALI
  (`false`) şekilde tanımlanıyor — artık yalnızca bir yorum satırına güvenilmiyor.
- **Veri bütünlüğü — düzeltildi:** Onay penceresi (dal değiştirme/birleştirme) açıkken kullanıcı bir
  dosya kaydederse, taze kontrol artık onay SONRASINDA da yeniden yapılıyor — önceki düzeltme yalnızca
  pencereden ÖNCEki durumu kontrol ediyordu, pencere açıkken yapılan değişiklikleri kaçırabiliyordu.
- **Veri bütünlüğü — düzeltildi:** `buildTreeFromWorkdir`'in nadir "önbellek biliyor ama nesne depoda
  yok" yedek yolunda artık ağaca gerçekten yazılan hash kullanılıyor (önbellekteki olası eski hash değil).
- **Veri bütünlüğü — düzeltildi:** Dal değiştirme/birleştirme (`checkoutTree`) artık dosya önbelleğini
  işlem sonunda tamamen temizliyor — yavaş bir diskte/kaba mtime çözünürlüklü bir dosya sisteminde
  "racily clean" zaman penceresine güvenmek yerine yapısal bir garanti.
- **Düzeltildi:** `refreshLogPanel` artık günlük panelini KAPATMAK yerine içeriğini yerinde tazeliyor
  — kullanıcı paneli açık tutarken commit/birleştirme yaptığında sekmesi artık aniden kapanmıyor.
- **Düzeltildi:** `getAllPosts`/`getPendingPosts` da (getPeers gibi) artık `null` yerine her zaman `[]`
  döndürüyor; canlı panelin moderasyon görünümüne null-güvenlik eklendi.
- **Düzeltildi:** Yeni SDDL regresyon testi artık ACE alanlarını (trustee) TAM EŞLEŞME ile karşılaştırıyor
  — önceki alt-dizgi taraması, "WD" gibi bir SID kodunun "RCWDWO" gibi alakasız bir haklar listesi
  içinde geçmesiyle yanlış pozitif/negatif verebiliyordu.
- **Düzeltildi:** `MaxFileBytes` yorumu artık gerçek değeriyle (20 GB, önceden yanlışlıkla "2 GB"
  yazıyordu) eşleşiyor.
- **İyileştirildi:** `fileHashCache` artık `DeltaXRepository` içinde encapsüle (private) — dışarıdan
  yalnızca `buildWorkdirTree()`/`checkout()` üzerinden kullanılıyor.
- **İyileştirildi:** `disposableBag.ts`, dizi elemanlarını `splice` ile kaydırmak yerine yerinde
  zararsız bir disposable ile değiştiriyor — uzantı deactivate olurken `context.subscriptions`
  üzerinde gezinirken bir öğenin kendi kendini dispose etmesinin sırayı bozup bir sonraki öğeyi
  atlamasını (ve sızdırmasını) önler.

## 0.2.1

Kapsamlı bir kod incelemesinde (8 açılı bulgu taraması + doğrulama) bulunan sorunların düzeltmesi:

- **Düzeltildi:** `.projeagignore`'daki `/` içeren desenlerde (`build/*.log` gibi) `*`/`?` joker
  karakterleri artık Go'nun `filepath.Match`'i gibi `/` ayracını aşamıyor — önceden bu, DeltaX
  TUI'si ile VS Code'un aynı proje için farklı dosyaları yok saymasına/izlemesine yol açabiliyordu.
- **Düzeltildi:** Dal değiştirme/birleştirme, dosya izleyicisinin 400ms debounce penceresi
  yüzünden bir kaydın hemen ardından hâlâ "temiz" görünüp çalışma klasörünü silebiliyordu; artık
  bu işlemlerden hemen önce durum senkron olarak tazeleniyor.
- **Düzeltildi:** `deltax.dataDir` boşken `AND_DATA_DIR` ortam değişkeni artık okunmuyor — AND'ın
  kendisi bu değişkeni kendi veri dizini için hiç okumadığından, tesadüfen tanımlıysa canlı panelin
  sessizce hiç bağlanamamasına yol açabiliyordu.
- **Düzeltildi:** "Uzak Dalı Birleştir" artık commit'te olduğu gibi boyut sınırı nedeniyle atlanan
  dosya sayısını da bildiriyor.
- **Düzeltildi:** Canlı panel ve günlük paneli artık kapatıldıklarında `context.subscriptions`'tan
  düzgünce çıkarılıyor (paylaşılan `disposableBag.ts` yardımcı fonksiyonuyla) — uzun oturumlarda
  bellek birikimini önler.
- **İyileştirildi:** Durum hesaplama ve commit/birleştirme artık mtime/boyut tabanlı bir önbellek
  kullanıyor — değişmemiş dosyalar her taramada yeniden okunup hash'lenmiyor (büyük projelerde
  performans).
- AND tarafında (`internal/pluginapi/api.go`): `AND_DATA_DIR` dışına dosya gönderme koruması artık
  Windows/macOS'ta büyük/küçük harf farkıyla atlatılamıyor; route kaydı, token'lı ve token'sız
  dinleyicinin aynı endpoint kümesini paylaştığını netleştiren ayrı bir fonksiyona taşındı;
  `getPeers` artık `null` yerine her zaman `[]` döndürüyor; yeni pipe/socket dinleyicisinin izin
  ayarları için ek regresyon testleri eklendi.

## 0.2.0

- Canlı AND paneli eklendi (Komut Paleti → "AND: Canlı Panel"): kimlik/rol, bağlı eşler, genel
  sohbet, DM, forumda yeni konu/yanıt, (kurucu/moderatör için) bekleyen gönderileri
  onaylama/reddetme/silme ve yazar güvenilirlik listesine ekleme.
- Durum çubuğunda AND canlı bağlantı göstergesi.
- Bağlantı, AND'ın Go kaynağına eklenen opt-in (`AND_ENABLE_EXTERNAL_API=1`) bir named pipe
  (Windows, sahibe özel security descriptor) / Unix domain socket (0600 izin) üzerinden yapılır —
  bearer token yoktur, erişim doğrudan işletim sistemi tarafından kısıtlanır.

## 0.1.0

- İlk sürüm.
- Gruplar/Projeler ağacı (etkinlik çubuğu görünümü).
- DeltaX proje çalışma klasörleri için Kaynak Denetimi (SCM) entegrasyonu: durum, native diff, commit.
- Commit günlüğü paneli (arama + commit başına değişen dosyalar).
- Dal değiştirme / yeni dal oluşturma.
- Zaten senkronize edilmiş bir uzak dalı yerel olarak birleştirme (fast-forward + üç yönlü birleştirme + çakışma işaretleri).
- Projeyi `.zip` olarak dışa aktarma.
- `.projeagignore` düzenleme kısayolu.
