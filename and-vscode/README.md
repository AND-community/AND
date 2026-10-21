# AND for VS Code

AND ekosistemiyle tam uyumlu bir VS Code uzantısı. İki bağımsız katmandan oluşur:

1. **DeltaX sürüm kontrolü** — `Eklentiler/DeltaX` (Go) ile aynı disk üzerindeki veri formatını
   (`AND_DATA_DIR/DeltaX/…`) okuyup yazar. Commit/dal/birleştirme/diff gibi tamamen yerel işlemler
   **AND kapalıyken de** çalışır; grup oluşturma/davet/yönetici/Gönder(push)/Çek(pull) ise (TUI'de
   olduğu gibi) AND'ın canlı P2P bağlantısını gerektirir — bkz. [Grup senkronizasyonu](#deltax-grup-senkronizasyonu-davet--yönetici--gönder--çek).
2. **Canlı AND paneli** (Forum/DM/Sohbet/Ağ/Site) — AND açıkken, `internal/pluginapi`'nin yerel
   API'sine bağlanır; bkz. [Canlı Panel](#canlı-panel-forum--dm--sohbet--ağ) bölümü. Kişisel site
   sayfaların gerçek bir VS Code editöründe düzenlenip **Ctrl+S ile ağa yayınlanır** — bkz.
   [Kişisel Site](#kişisel-site-editörde-düzenle-ctrls-ile-yayınla).

Bu uzantı **AND TUI ile aynı anda, birbirini bozmadan** kullanılabilir: DeltaX katmanı ikisi de
aynı içerik-adresli nesne deposunu (`nesneler/`), aynı `meta.json`/`MERGE_HEAD` dosyalarını
okur/yazar; canlı panel ise AND'ın zaten çalışan sürecine ayrı bir bağlantı üzerinden konuşur.

## DeltaX — Neler yapılabilir

| Özellik | Nasıl |
|---|---|
| Gruplar/Projeler listesi | Etkinlik çubuğundaki **DeltaX** simgesi |
| Bir projeyi çalışma alanı olarak açma | Proje ağacında proje adına tıkla / "Projeyi Aç" |
| Yeni proje oluşturma (yerel) | Grup üstünde sağ tık → "Yeni Proje" |
| Durum (Eklenen/Değişen/Silinen) | Kaynak Denetimi paneli, otomatik yenilenir |
| Diff | Bir değişikliğe tıkla → VS Code'un yerleşik diff editörü (HEAD ↔ çalışma dosyası) |
| Kaydet (Commit) | Kaynak Denetimi giriş kutusuna mesaj yaz, Ctrl+Enter |
| Günlük (Log) | Komut Paleti → "DeltaX: Günlük" — arama + commit başına değişen dosyalar, tıklayınca diff |
| Dal değiştir / yeni dal | Komut Paleti → "DeltaX: Dal Değiştir" / "DeltaX: Yeni Dal Oluştur" |
| Uzak dalı birleştir | Komut Paleti → "DeltaX: Uzak Dalı Birleştir" (`proje.uzaklar`'daki, TUI ile daha önce Gönder/Çek edilmiş uçlar) |
| Dışa aktar (.zip) | Proje sağ tık → "Projeyi Dışa Aktar (.zip)" |
| `.projeagignore` düzenle | Komut Paleti → "DeltaX: .projeagignore Dosyasını Aç/Oluştur" |

## DeltaX grup senkronizasyonu (davet / yönetici / Gönder / Çek)

Bu bölümdeki işlemler AND'ın canlı P2P bağlantısını (bkz. [Canlı Panel](#canlı-panel-forum--dm--sohbet--ağ)
kurulum adımları) gerektirir — DM ve dosya aktarımı üzerinden, `Eklentiler/DeltaX/sync.go`'daki
`AND-PROJEAG/1` protokolünün TypeScript karşılığıyla çalışır (bkz. `src/live/deltax/`). AND TUI'sindeki
DeltaX eklentisiyle **tamamen birlikte çalışır**: TUI'de oluşturduğun bir grubu VS Code'dan da
yönetebilir, VS Code'da başlattığın bir Gönder/Çek'i TUI'deki bir üye alabilir ve tersi — ikisi de
aynı `gruplar/`/`projeler/` JSON'larını ve aynı `AND-PROJEAG/1` DM protokolünü kullanır.

| Özellik | Nasıl |
|---|---|
| Yeni grup oluştur | Etkinlik çubuğu → DeltaX görünümü başlığı → "Yeni Grup" (`deltax.createGroup`) |
| Üye davet et | Grup sağ tık → "Üye Davet Et" (`deltax.inviteMember`, yalnızca kurucu/yönetici) — bilinen eşler arasından seç ya da Peer ID'yi elle gir. Kişi çevrimdışıysa AND daveti bekletir, bağlandığında iletir |
| Gelen daveti kabul/reddet | Davet geldiğinde bildirim, "Kabul Et"/"Reddet". Bildirimi kapatırsan davet kaybolmaz: gruplar görünümünün başındaki **Bekleyen davetler** satırından ya da "DeltaX: Bekleyen Davetler" (`deltax.showInvites`) ile yanıtlanır |
| Yönetici ata/kaldır | Grup sağ tık → "Yönetici Ata/Kaldır" (`deltax.manageAdmins`, yalnızca kurucu) |
| Gruptan çık / grubu kapat | Grup sağ tık → "Gruptan Çık / Grubu Kapat" (`deltax.leaveOrCloseGroup`) — kurucu ya da yöneticiysen grup kalıcı olarak kapanır, değilsen yalnızca sen ayrılırsın |
| Gönder (push) | Proje sağ tık → "Gönder (Push)" (`deltax.pushProject`) — tek bir çevrimiçi üyeye ya da "Çevrimiçi herkese" |
| Çek iste (pull) | Proje sağ tık → "Çek İste (Pull)" (`deltax.pullFromMember`) — hedef üye çevrimiçiyse otomatik Gönder yapar |
| Gelen paketi uygulama | Otomatik — bilinen bir grup üyesinden gelen `.pack` sessizce kabul edilip nesne deposuna uygulanır, `proje.uzaklar` güncellenir; ardından "DeltaX: Uzak Dalı Birleştir" ile yereldeki dala birleştirilebilir |

Güven modeli TUI ile birebir aynıdır: yetki kararları yalnızca AND'ın bağlantıdan doğruladığı
gönderen Peer ID'sine dayanır; davet/roster güncellemeleri yalnızca zaten bilinen kurucu/yönetici
peer ID'sinden kabul edilir; bir "kabul" yalnızca gerçekten davet gönderilmiş eşten ve 30 gün içinde
gelirse uygulanır; gelen paketler grup sırrıyla (AES-256-GCM) şifre çözülür ve her nesnenin sha256'sı
yeniden doğrulanır — bkz. `src/live/deltax/zarfIsleyici.ts`, `src/live/deltax/packSync.ts`.

**Mesajlar kaybolmaz.** Gelen grup mesajları AND'ın DM geçmişinden okunur; nereye kadar işlendiği
VS Code'un uzantıya ayırdığı depolama alanında tutulur. VS Code kapalıyken gelen bir davet, üye
listesi, ayrılma ya da kapatma bildirimi açılışta sırayla işlenir; ilk açılışta önceki mesajlar
işlenmez. Davet ve yanıtı, üye listesi, ayrılma ve kapatma giden kutusuyla gider. Push bildirimi ve
çek isteği doğrudan gider, çünkü dosya aktarımı iki tarafın da çevrimiçi olmasını gerektirir. Bir
saatten eski çek istekleri yanıtlanmaz. Birden çok VS Code penceresi açıksa mesajları yalnızca biri
işler; aynı DM için her pencerede ayrı bildirim çıkmaz.

---

## Canlı Panel (Forum / DM / Sohbet / Ağ)

AND açıkken (ve aşağıdaki tek seferlik kurulum yapıldıysa) Komut Paleti → **"AND: Canlı Panel"**
ile: kimlik/rol bilgisi, bağlı eşler, genel sohbet, doğrudan mesaj (DM), forumda yeni konu açma
(AND'da onay kuyruğu yoktur — gönderi oluşturulduğu anda ağa yayınlanır), bir konuyu ID ile
görüntüleyip yanıtlama ve — kurucu/moderatörseniz — tüm gönderileri görüntüleyip istenmeyen bir
konuyu silme.

Durum çubuğunda sol altta **"AND: bağlı" / "AND: bağlı değil"** göstergesi vardır; tıklayınca
paneli açar.

Panelin solunda bölümler arasında geçiş yapılan bir gezinme rayı bulunur (Kimlik, Ağ, Sohbet,
Doğrudan mesaj, Forum, Site, Dosyalar). Panel daraltılınca ray yalnızca ikonlara iner. Yüzeyler ve
form öğeleri VS Code temanızı izler. AND'da seçili tema (TUI → Ayarlar → Temalar) ise marka
işaretinde, etkin bölümde ve birincil düğmelerde kullanılır. Temayı TUI'de değiştirince panel de
birkaç saniye içinde, Yenile'ye basmadan yeni renklere geçer. Sohbet ve DM kutularında **Enter** gönderir.
Peer ID ve genel anahtarın yanındaki düğme değeri panoya kopyalar.

**Doğrudan mesaj** sekmesi AND'ın DM geçmişini konuşmalar hâlinde gösterir: solda eşler ve okunmamış
sayıları, sağda seçili konuşma. Konuşmalar ve okunmamışlar AND TUI'si ve Özel Chat ile ortaktır;
VS Code'da okuduğun konuşma TUI'nin ana menü rozetinden de düşer, durum çubuğundaki rozet de aynı
sayıyı gösterir. Karşı taraf çevrimdışıysa mesaj AND'ın giden kutusunda **bekliyor** olarak görünür
ve o bağlandığında gider; bir hafta içinde gidemezse **gönderilemedi** olur. DeltaX'in DM ile taşınan
grup mesajları burada hiç görünmez.

### Nasıl bağlanır — mimari

AND'ın eklenti API'si (`internal/pluginapi`) normalde yalnızca AND'ın kendisinin başlattığı
eklenti alt süreçlerine (`AND_API_ADDR`/`AND_API_TOKEN` ortam değişkenleriyle) açılır — bağımsız
başlatılmış bir VS Code uzantısı bu değişkenleri asla göremez. Bunu çözmek için AND'ın Go
kaynağına küçük, opt-in bir ekleme yapıldı: `AND_ENABLE_EXTERNAL_API=1` ortam değişkeniyle
açıldığında AND, mevcut token'lı TCP dinleyicisinin YANINA (onu değiştirmeden) **ikinci bir
dinleyici** açar:

- **Windows:** sabit, türetilmiş adlı bir **named pipe** (`\\.\pipe\and-plugin-api-<hash>`),
  security descriptor'ı yalnızca pipe'ın sahibine (owner) erişim izni verecek şekilde ayarlanmış
  (`D:P(A;;GA;;;OW)`) — aynı makinedeki başka bir kullanıcı, hatta Administrators grubu bile
  erişemez.
- **Linux/macOS:** `AND_DATA_DIR/api.sock` yolunda, `0600` izinli bir **Unix domain socket**.

Bu ikinci dinleyicide **bearer token yoktur** — erişim kararını bir sırla değil doğrudan işletim
sistemi verir, bu yüzden pipe/socket adının kendisi gizli olmak zorunda değildir (adı bilmek
yalnızca bağlanmayı DENEMEYE izin verir; gerçek erişim OS tarafından kısıtlanır). Uzantı bu adı,
AND'ın kullandığı `AND_DATA_DIR` yoluyla birebir aynı formülle (bkz. `src/live/liveAddr.ts` ↔
`internal/pluginapi/externalpipe_*.go`) kendisi hesaplar; ayrıca bir keşif dosyasına gerek yoktur.

### Kurulum (tek seferlik)

AND'ı başlatmadan önce ortam değişkenini ayarlayın:

```powershell
# PowerShell
$env:AND_ENABLE_EXTERNAL_API = "1"
.\and.exe
```

```bash
# bash
AND_ENABLE_EXTERNAL_API=1 ./and
```

Bu değişken ayarlanmadıysa AND, mevcut davranışını (yalnızca kendi başlattığı eklentilere
token'lı erişim) hiçbir şekilde değiştirmez — ikinci dinleyici hiç açılmaz.

### Kapsam ve sınırlamalar

- **Forum'da tüm gönderileri gözatma (`getAllPosts`) yalnızca kurucu/moderatör
  rolüyle çalışır** — bu, AND'ın API'sinin kendi tasarımıdır (bkz. `enforceModeratorRole`),
  uzantı tarafından eklenen bir kısıtlama değildir. Normal bir üye olarak yeni konu açabilir,
  bilinen bir konu ID'sine yanıt yazabilir ve ID'sini bildiğiniz bir konuyu görüntüleyebilirsiniz.
- DM'ler panel kapalıyken de arka planda AND'ın DM geçmişinden okunur (2 saniyede bir, yalnızca
  yeni kayıtlar istenir); gelen DM bildirim olarak düşer. VS Code kapalıyken gelenler açılışta tek bir
  özet bildirimle haber verilir. Geçmiş tutmayan eski bir AND'da canlı dinlemeye (long-poll) geçilir;
  o zaman yalnızca VS Code açıkken gelenler görünür.
- Genel sohbet ve dosya istekleri sunucu tarafında en fazla 5 saniye bekleyen bir long-poll ile
  alınır. Sohbet sekmesi açılışta (ve "Yenile"de) AND'ın kalıcı geçmişinden son 200 mesajı yükler,
  canlı gelenleri bunlarla tekrarsız birleştirir.
- Bu katman AND kapalıyken hiçbir şey yapmaz — DeltaX'in yerel dosya tabanlı özellikleri bundan
  etkilenmez.

## Kişisel Site (editörde düzenle, Ctrl+S ile yayınla)

AND'ın çok sayfalı kişisel sitesi (bkz. `Eklentiler/site`) VS Code'dan da yönetilebilir — ve sayfalar
bir form kutusunda değil, **gerçek bir VS Code editöründe** düzenlenir (Markdown vurgulama, bul/değiştir,
çoklu imleç, geri al…). Canlı bağlantı gerekir (bkz. [Kurulum](#kurulum-tek-seferlik)).

| Özellik | Nasıl |
|---|---|
| Sayfaları listele | Canlı Panel → **Site** sekmesi (sayfa sayısı N/20, son güncelleme, bayt kullanımı, önizleme) |
| Yeni sayfa | Site sekmesi → "Yeni sayfa" ya da Komut Paleti → **"AND: Yeni Site Sayfası"** — slug ve başlık sorulur, boş bir taslak açılır; ilk kayıtta yayınlanır |
| Düzenle | Site sekmesi → "Düzenle" ya da **"AND: Site Sayfası Aç…"** |
| Yayınla | Editörde **kaydet (Ctrl+S)** — AND TUI'sindeki site eklentisinin `ctrl+s`'i gibi "kaydet ve ağa yayınla" |
| Paylaş | Editör başlığındaki bağlantı düğmesi, Site sekmesi ya da **"AND: Site Paylaşım Kodunu Kopyala"** — `yazarAnahtarı/slug` kodunu (istenirse `and-site://` bağlantısını) panoya kopyalar; alan kişi sayfayı AND Arama Motoru'nun "Kodla Git" ekranında ya da `-site` ile açar |
| Sil | Site sekmesi → "Sil" ya da **"AND: Site Sayfasını Sil…"** (onay ister) |

- **Belge biçimi:** ilk satır `# Başlık`, bir boş satırdan sonrası sayfanın gövdesidir. Satır sonları
  LF'ye çevrilir; başlığın ve gövdenin baştaki/sondaki boşlukları kırpılır (TUI ile aynı). AND gövdeyi
  Markdown olarak render ETMEZ — vurgulama yalnızca yazarken kolaylık içindir.
- **Sınırlar canlı gösterilir:** durum çubuğunda `başlık 12/100 · gövde 1.234/8.000 bayt`; sınırı aşan
  kısım editörde tam aşıldığı karakterden itibaren hata olarak işaretlenir. Sınırlar AND'ın kendisiyle
  aynıdır ve karakter değil **bayt** sayar (ğ, ş gibi Türkçe karakterler 2 bayttır). Sınıra ya da
  kurala (boş gövde) uymayan bir kayıt ağa hiç gönderilmeden reddedilir.
- **Kaydetmek bazen birkaç saniye sürebilir:** AND eşleri aynı sayfanın 5 sn'den sık gelen canlı
  güncellemelerini düşürür (`internal/site.minPageEditInterval`). Uzantı bu yüzden aynı sayfanın
  ardışık yayınlarını en az 6 sn aralıkla gönderir; gerekirse kayıt, durum çubuğunda "… sn içinde
  yayınlanacak" göstererek bekler.
- **Otomatik kaydetme yayın yapmaz:** kaydetmek ağa yayınlamak demek olduğundan bu sayfalar için
  `files.autoSave` varsayılan olarak kapalıdır (`[and-site]` dil ayarı) — yazarken yarım bir taslak
  ağa yayılmaz. Bu dil için otomatik kaydetmeyi bilerek açsan bile otomatik bir kayıt reddedilir;
  yayın için Ctrl+S gerekir.
- **Çakışma:** açık bir site editörü varken uzantı sayfaları 15 sn'de bir AND'dan yeniden okur. Sayfa
  başka bir yerde (ör. AND TUI'sinde) değişirse, değişmemiş bir editör yenilenir; kaydedilmemiş
  değişikliklerin varsa VS Code'un yerleşik "dosya daha yeni — Karşılaştır / Üzerine Yaz" akışı çalışır.
- **Sınırlamalar:** silinen bir slug bir daha kullanılamaz (AND'ın silme kaydı kalıcıdır — böyle bir
  yayın açık bir hatayla reddedilir); bir sayfanın slug'ı değiştirilemez (yeniden adlandırma yok);
  en fazla 20 sayfa. Yalnızca KENDİ sayfaların düzenlenebilir — başkalarının sayfaları AND'ın profil
  ekranında ya da AND Arama Motoru'nda görüntülenir.

## ⚠️ Dal değiştirme / birleştirme çalışma klasörünü tamamen siler

DeltaX'in TUI'siyle birebir aynı davranış: bir dala geçerken (`dalaGec`), çalışma klasöründeki
**her şey** — `.projeagignore` ile hariç tutulan ya da hiç izlenmeyen dosyalar dahil — önce
silinir, sonra hedef dalın kayıtlı içeriği yeniden yazılır. Uzantı bunu yapmadan önce (a) çalışma
dizininin temiz olduğunu zorunlu kılar ve (b) açık bir onay penceresi gösterir; yine de önemli
yerel-olmayan dosyaları çalışma klasörünün dışında tutun.

## Kurulum / Geliştirme

Canlı panel için AND tarafında `go build ./...` ile yeniden derleme gerekir (yalnızca
`internal/pluginapi` içine `github.com/Microsoft/go-winio` bağımlılığı eklendi — `go.mod`/`go.sum`
zaten güncellendi, ek bir adım gerekmez). Bu değişiklik `go build ./...`, `go vet ./...` ve
`go test ./...` ile doğrulanır.

VS Code uzantısı tarafı için:

```bash
cd and-vscode
npm install
npm run build        # dist/extension.js üretir (esbuild)
npm run typecheck     # tsc --noEmit
npm test              # DeltaX veri katmanı, src/live/deltax/ (crypto/tarball/packSync) ve src/live/site/ testleri (node:test)
```

VS Code'da bu klasörü açıp **F5** ile bir Extension Development Host penceresi başlatabilir,
ya da paketleyip kurabilirsiniz:

```bash
npm run package       # and-vscode-<versiyon>.vsix üretir (@vscode/vsce) — bkz. package.json "version"
code --install-extension and-vscode-<versiyon>.vsix
```

Geliştirme sırasında canlı yeniden derleme için `npm run watch`.

## Ayarlar

| Ayar | Açıklama |
|---|---|
| `deltax.dataDir` | `AND_DATA_DIR` üzerine yaz. Boşsa AND'ın kendi varsayılanı (`os.UserConfigDir()/and`) otomatik hesaplanır. Canlı panelin pipe/socket adını hesaplarken de aynı değer kullanılır — AND gerçekte farklı bir `AND_DATA_DIR` ile başlatıldıysa burada da aynısını belirtin. |
| `deltax.authorName` | DeltaX commit'lerinde yazar adı. Boşsa işletim sistemi kullanıcı adı kullanılır. AND'daki takma addan bağımsızdır. |

## Mimari — Go karşılıkları

Bu uzantının veri katmanı, `Eklentiler/DeltaX` (Go) içindeki dosyaların TypeScript'e birebir
taşınmasıdır; hash algoritmaları ve JSON alan adları bilerek aynı tutulmuştur ki iki taraf da
aynı `nesneler/`, `meta.json`, `MERGE_HEAD` dosyalarını sorunsuz okuyup yazabilsin.

| TypeScript | Go karşılığı | Ne yapar |
|---|---|---|
| `src/objectStore.ts` | `vcs.go` (nesne deposu) | İçerik-adresli blob/ağaç/commit okuma-yazma |
| `src/treeStore.ts` | `vcs.go` (ağaç/durum/checkout) | Ağaç oluşturma, durum hesaplama, dal geçişi |
| `src/commitStore.ts` | `vcs.go` (commit/günlük/ortak ata) | Commit okuma/yazma, günlük, ortak ata |
| `src/mergeEngine.ts` | `vcs.go` (üçlü birleştirme) | Üç yönlü birleştirme + çakışma işaretleri |
| `src/ignoreRules.ts` | `ignore.go` | `.projeagignore` desen eşleştirme |
| `src/groupProjectStore.ts` | `store.go` | Grup/proje meta verisi, MERGE_HEAD |
| `src/exportZip.ts` | `export.go` | Projeyi `.zip` olarak dışa aktarma |
| `src/dataDir.ts` | `cmd/and/main.go` (`appDir`) | `AND_DATA_DIR` varsayılan konum tespiti |

Canlı panel katmanı (`src/live/`) yukarıdaki DeltaX veri katmanından tamamen ayrıdır ve AND'ın
çalışan sürecine bağlanır:

| TypeScript | Go karşılığı | Ne yapar |
|---|---|---|
| `src/live/liveAddr.ts` | `internal/pluginapi/externalpipe_windows.go` / `externalpipe_unix.go` | Pipe adı / socket yolu türetme (birebir aynı formül) |
| `src/live/liveClient.ts` | `internal/pluginapi/api.go` (`Client`) | Named pipe/Unix socket üzerinden HTTP istekleri (token yok) |
| `src/live/liveConnection.ts` | — | Bağlantı durumu, periyodik yeniden deneme |
| `src/live/backgroundSync.ts` | — | Arka plan akışları: DM geçmişi (imleçle), sohbet/dosya/onay long-poll'ları |
| `src/live/dmCore.ts` | `Eklentiler/DeltaX/gecmis.go` (`dmImleci`), `protokol/dm.go` | DM imleci, makine mesajı denetimi, konuşma görünümü, okunmamış sayısı; `vscode`'a bağımlı değildir |
| `src/live/dmMerkezi.ts` | `Eklentiler/ozel_chat` | Panel ve rozet için DM durumu: AND'ın geçmişi ve okundu bilgisi, eski AND'da yerel yedek |
| `src/live/lider.ts` | — | Birden çok VS Code penceresinden yalnızca birinin DM'leri işlemesi |
| `src/live/livePanel.ts` | — | Canlı Panel'in uzantı tarafı: webview mesajlarını AND API çağrılarına bağlar, canlı akışları panele iletir |
| `src/live/panelHtml.ts` | — | Panelin HTML/CSS/betiği (Kimlik/Ağ/Sohbet/DM/Forum/Site/Dosyalar); `vscode`'a bağımlı değildir, sahte DOM ile birim testi vardır |
| `src/live/themeColor.ts` | `internal/ui/theme.go` (`Presets`) | AND tema renklerini (xterm-256 kodu ya da hex) CSS'e çevirir, vurgu üzerindeki yazı rengini seçer |
| `src/live/liveTypes.ts` | `internal/pluginapi/api.go` (struct'lar) | JSON tip tanımları |
| `src/live/site/sitePage.ts` | `internal/site/site.go` (sınırlar, `reSlug`) + `Eklentiler/site` (`kaydet()`) | Saf yardımcılar: belge biçimi, doğrulama, bayt taşma ofseti, paylaşım kodu, yayın aralığı |
| `src/live/site/siteFileSystem.ts` | `internal/pluginapi` `/api/v1/site/*` | `and-site-edit:` sanal dosya sistemi — okuma, kaydet = yayınla, silme, değişiklik olayları |
| `src/live/site/siteEditorStatus.ts` | `Eklentiler/site` (canlı bayt sayaçları) | Durum çubuğu sayacı + sınır aşımı tanılamaları |
| `src/live/site/siteCommands.ts` | `Eklentiler/site` (liste/yeni/sil) | Yeni / Aç / Sil / Paylaşım Kodunu Kopyala komutları |

DeltaX grup senkronizasyon katmanı (`src/live/deltax/`) `Eklentiler/DeltaX/sync.go` + `crypto.go`'nun
birebir karşılığıdır — DM/dosya taşıması için yukarıdaki `liveClient.ts`'i kullanır:

| TypeScript | Go karşılığı | Ne yapar |
|---|---|---|
| `src/live/deltax/crypto.ts` | `crypto.go` | Grup sırrı üretimi, AES-256-GCM şifreleme/çözme |
| `src/live/deltax/tarball.ts` | `archive/tar` + `compress/gzip` (Go stdlib) | Bağımlılıksız USTAR tar + gzip okuyucu/yazıcı |
| `src/live/deltax/packSync.ts` | `sync.go` (paket oluşturma/uygulama/gelen paket işleme) | `paketOlustur`/`paketUygula`/`paketGonderenGuvenilirMi`/`pushPaketiIsle` |
| `src/live/deltax/protocol.ts` | `sync.go` (zarf + davet/roster/push/pull mesajları) | Zarf kodlama/çözme, DM gönderen fonksiyonlar, Gönder/Çek |
| `src/live/deltax/groupOps.ts` | `store.go` (`Grup` üstündeki yöntemler) | Yetki/üyelik kontrolleri, roster mutasyonu |
| `src/live/deltax/zarfIsleyici.ts` | `model.go` (`zarfIsle`) | Gelen grup mesajlarını doğrulanmış gönderene göre işler; `vscode`'a bağımlı değildir, testlidir |
| `src/live/deltax/davetler.ts` | `gecmis.go` (`davetDefteri`), `bekleyen_davetler.json` | Gönderilen davetlerin defteri (30 gün) ve bekleyen gelen davetler |
| `src/live/deltax/syncService.ts` | `model.go` (push/consent poll işleyicileri) | VS Code katmanı: bildirimler, davet soruları, gelen paket ve onaylar |
| `src/live/deltax/groupActions.ts` | `keys.go` (grup paneli tuş işleyicileri) | VS Code komutları: grup/davet/yönetici/gönder/çek |

Not: Ağaç/commit nesnelerini yazarken JSON serileştirmesinin Go'nunkiyle bayt bayt aynı olması
**gerekmez** — her nesnenin hash'i kendi yazdığı baytlardan türetildiği için bu iki taraf arasında
"aynı içerik = aynı hash" garantisi zaten sağlanır (kısmi çiftleşme dışında bir tutarsızlık
oluşmaz). Blob hash'i ise (`sha256('B' + ham bayt)`) iki tarafta da bit bit aynı formülle
hesaplanır — durum karşılaştırmalarının doğruluğu buna dayanır.
