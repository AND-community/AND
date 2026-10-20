# AND Arama Motoru

![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)
![Lisans](https://img.shields.io/badge/lisans-AGPL--3.0-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)

**AND** ekosistemindeki (P2P forum + sohbet + kişisel site) içeriği arayan, **AND'dan tamamen bağımsız** çalışan bir program.
AND'ın kendisi kapalı olsa bile, bu program kendi ağ kimliğiyle ve kendi libp2p bağlantısıyla AND'ın P2P ağına doğrudan bir eş (peer) olarak katılır, içeriği toplayıp yerel bir tam metin indekste arattırır — hem AND'ın kaynak kodunu import etmeden hem de AND'ı bir alt-süreç olarak çalıştırmadan.

---

## İçindekiler

- [Genel Bakış](#genel-bakış)
- [Özellikler](#özellikler)
- [Neden "eklenti" değil de bağımsız bir program?](#neden-eklenti-değil-de-bağımsız-bir-program)
- [Mimari](#mimari)
- [Gereksinimler](#gereksinimler)
- [Kurulum ve çalıştırma](#kurulum-ve-çalıştırma)
- [Kullanım](#kullanım)
- [Komut satırı seçenekleri](#komut-satırı-seçenekleri)
- [Site sayfası paylaşımı](#site-sayfası-paylaşımı)
- [Gelişmiş sorgu sözdizimi](#gelişmiş-sorgu-sözdizimi)
- [Tema](#tema)
- [Veri dizini dosyaları](#veri-dizini-dosyaları)
- [Kapsam ve sınırlar](#kapsam-ve-sınırlar)
- [Ekosistem](#ekosistem)
- [Katkı](#katkı)
- [Güvenlik](#güvenlik)
- [Lisans](#lisans)

---

## Genel Bakış

AND Arama Motoru, AND'ın P2P forum/sohbet/kişisel-site ağına kendi bağımsız kimliğiyle katılan, ağda dolaşan içeriği topluca indeksleyip Google/Yandex benzeri bir arama deneyimiyle (tam metin arama, alakalılık sıralaması, yazım önerisi, sayfalama) aratmanı sağlayan bir terminal uygulamasıdır (TUI) ve aynı zamanda betiklenebilir bir komut satırı aracıdır (CLI).

AND'ın kaynak kodunu import etmez, AND'ı bir alt-süreç olarak da çalıştırmaz — protokolleri (imza biçimi, GossipSub konuları, senkronizasyon akışı) bağımsız olarak yeniden uygular ve ağa **gerçek, kendi başına bir eş** olarak katılır. Bu yüzden AND hiç çalışmıyor olsa bile ağdaki içeriği görebilir ve arattırabilirsin.

---

## Özellikler

| Özellik | Açıklama |
|---------|----------|
| Tam bağımsız çalışma | AND'a hiçbir bağlantı/ortam değişkeni gerekmez; kendi kimliği ve ağ bağlantısıyla çalışır |
| Tam metin arama | SQLite FTS5 tabanlı, Türkçe aksan-duyarsız, `bm25()` + tazelik boostuyla sıralanan arama |
| Çok atlamalı ağ araması | Kendi P2P protokolüyle (`/and-arama-motoru/1.0.0`) ağdaki diğer düğümlerin indekslerinde de arar |
| Gelişmiş sorgu sözdizimi | `"tam ifade"`, `-hariç`, `terim1 OR terim2` desteklenir |
| Yazım önerisi | Sıfır sonuçta "şunu mu demek istediniz?" önerisi (ekstra bağımlılık gerekmez) |
| Yazara göre arama | Bir sonucun yazarının tüm içeriğini tek tuşla listele |
| Betiklenebilir CLI | `-search`/`-site` ile TUI/ağ açmadan yerel indekste ara, `-json` ile makine-okur çıktı |
| Kişisel site paylaşım kodu | Bir site sayfasını `yazarAnahtarı/slug` koduyla, ya da (Windows ve Linux'ta) tek tıkla açılan `and-site://` bağlantısıyla doğrudan paylaş |
| Tıklanabilir bağlantılar | Sonuçlardaki `http(s)://` ve sayfa içi `and-site://` bağlantıları terminalde gerçekten tıklanabilir |
| Görsel kimlik | AND ile aynı tasarım dili (palet, logo, liste satırları, kısayol satırı); özelleştirilebilir tema |
| Panik kalkanı | Güvenilmeyen ağ içeriğindeki bir ayrıştırma hatası programı çökertmez |
| Çökme-güvenli depolama | Kimlik/eş listesi dosyaları atomik yazımla korunur |
| Sıralama modu | Sonuçları Alaka (bm25+tazelik) ya da yalnızca Tarih'e (Yeni) göre sırala (`ctrl+r`) |
| Kayıtlı arama / "takip et" | Bir aramayı takibe al; ağdan yeni eşleşen içerik geldikçe bir "N yeni" rozetiyle bildirim al |
| Hedefli P2P sorgu | Bir ağ sonucunun geldiği eşi doğrudan hedefleyip yalnızca onun güncel indeksini sorgula |
| Eş güven göstergesi | Durum ekranında, doğrudan sorgulanan eşlerin geçerli/geçersiz imzalı sonuç oranını gör |
| Site haritası ve "ağa düşen" kod arama | Yalnızca bir yazar anahtarıyla o yazarın TÜM site sayfalarını listele; yerelde olmayan bir kod otomatik olarak ağda da aranır |
| Sayfa içi çapraz bağlantı ve QR kodu | Site sayfası gövdesindeki `and-site://` bağlantılarında gezin; paylaşım kodunu terminalde QR kod olarak göster |
| Düzenleme senkronu | Bir site sayfasının DÜZENLENMİŞ hali de senkronizasyonla geriye dönük yakalanır — yalnızca hiç bilinmeyen sayfalar değil |

---

## Neden "eklenti" değil de bağımsız bir program?

Bu klasör AND'ın Go modülünün **dışında**: `AND/internal/...` paketlerini (ağ, forum, kimlik doğrulama mantığı) Go'nun paket görünürlük kuralları gereği import edemez. Bu yüzden AND'ın protokollerini (aşağıda listelenen) **bağımsız olarak yeniden uyguladı** — aynı GossipSub konuları, aynı imza biçimi, aynı senkronizasyon protokolü, ama kendi kodu. Sonuç: AND çalışmasa bile, bu program ağa kendi başına bağlanıp aynı içeriği görebilen gerçek bir eş.

## Mimari

1. **Kendi kimliği** (`identity.go`) — AND'ın kullandığı aynı türetme şemasıyla (BIP39 mnemonic → ed25519 anahtar) ama kullanıcının asıl AND kimliğinden tamamen bağımsız, ilk çalıştırmada üretilip `~/.and-arama-motoru/kimlik.json`'a kaydedilir.
2. **Ağ katmanı** (`network.go`) — kendi libp2p host'u; AND ile aynı eş keşif mekanizması: genel IPFS önyükleme eşlerine dayanan bir DHT (rendezvous: `and-community/1.0.0`) + yerel ağda mDNS (`and-community-mdns`).
3. **İçerik alımı** (`forum.go`, `chat.go`, `site.go`) — AND'ın forum (`github.com/lucian95511/and/forum` konusu), sohbet (`.../chat` konusu) ve kişisel site (`.../site/1.0.0` konusu) GossipSub yayınlarına abone olur; her post/yanıt/mesajın/sayfanın ed25519 imzasını AND'ın kendisiyle **bire bir aynı payload biçimiyle** doğrular — imzasız ya da kurcalanmış hiçbir şey indekslenmez. Yeni bağlanan her eşle AND'ın senkronizasyon protokollerini (`/and/sync/1.0.0`, `/and/chatsync/1.0.0`, `/and/sitesync/1.0.0`) konuşarak geçmişi de geriye dönük toplar. Site sayfaları forum post'larından/sohbet mesajlarından FARKLI olarak DEĞİŞEBİLİR (aynı yazar/slug yeniden yayınlanınca tamamen değişir) — bu yüzden yaz-bir-kez `Ekle` yerine "en yeni kazanır" upsert semantiğine sahip `Depo.EkleYaGuncelle` kullanılır.
4. **Yerel indeks** (`index.go`) — SQLite FTS5, Türkçe aksan-duyarsız arama; kendi ayrı veritabanı dosyası (`~/.and-arama-motoru/arama_motoru.db`), AND'ın `forum.db`'sine hiç dokunmaz. Sıralama, sütun ağırlıklı `bm25()` (başlık eşleşmesi gövdeden çok daha değerlidir) ile bir tazelik boostunu birleştirir — eşit derecede alakalı sonuçlar arasında yeni içerik hafifçe öne çıkar. Sonuçlar eşleşen terimi vurgulayan bir alıntıyla (snippet) gelir; bir yanıt sonucu, ait olduğu konunun başlığını/kategorisini de (konu zaten senkronize olduysa) gösterir.
5. **Kendi P2P arama protokolü — ÇOK ATLAMALI** (`search.go`) — bu programın AYRI bir libp2p protokolü (`/and-arama-motoru/1.0.0`): bu programın başka bir kopyasını çalıştıran eşlere doğrudan bir stream üzerinden sorgu gönderip onların kendi indekslerinden sonuç alabilirsin ("ağda ara", `ctrl+n`). AND'ın DM kanalının aksine boyut sınırı yok (tam gövde taşınır) ve gönderenin kimliği bağlantının kendisinden (sahtesi mümkün olmayan şekilde) bilinir. Sorgu yalnızca DOĞRUDAN bağlı eşlerle sınırlı değildir: her düğüm, kendi TTL'i (varsayılan 3 atlama) tükenmedikçe sorguyu kendi eşlerine de ileterek yanıtları toplar ve tek bir birleşik sonuçla geri döner (klasik "flood & aggregate" P2P arama deseni) — böylece ağın birkaç atlama ötesindeki bir düğümün indeksindeki içerik de bulunabilir. Aynı sorgunun ağda sonsuz dönmesi/gereksiz tekrarlanması, paylaşılan bir sorgu ID'sinin her düğümde yalnızca bir kez işlenmesiyle (bkz. `sorguOnbellek`) önlenir. Her sonuç, ONU GERÇEKTEN indeksleyen düğümün peer ID'siyle etiketlenir (ara düğümler tarafından değiştirilmeden taşınır) — kaç atlama öteden geldiğinden bağımsız olarak gerçek kaynağı bilirsin.
6. **Bilinen eş listesi** (`peers.go`) — şu an bağlı olduğun eşlerin adresleri periyodik olarak `~/.and-arama-motoru/bilinen_peerler.json`'a yazılır; bir sonraki açılışta DHT/mDNS keşfini beklemeden bunlara doğrudan bağlanmayı dener, böylece yeniden başlatma sonrası bağlantı gözle görülür şekilde hızlanır.
7. **Seviyeli, boyut sınırlı tanılama günlüğü** (`log.go`) — TUI'yi hiç etkilemeden (stdout'a değil), `[bilgi]`/`[uyarı]`/`[hata]` seviyeleriyle yaşam döngüsü olaylarını ve hataları `~/.and-arama-motoru/gunluk.log`'a yazar — "neden hiç sonuç yok" gibi sorunları teşhis etmek için. Dosya 5 MB'ı aşınca otomatik olarak `.old`'a döndürülüp sıfırdan başlanır, uzun süre açık kalan bir işlemde sınırsız büyümez. Kullanılan kütüphanelerin kendi günlükleri de (standart `log` paketi ve libp2p'nin go-log'u) terminal yerine bu dosyaya gider. Önceden Windows'ta mDNS'in sanal ağ arayüzleri için tekrarladığı "Failed to set multicast interface" uyarıları tam ekran arayüzün üstüne yazılıp ekranı bozuyordu; bu zararsız uyarı artık günlüğe yalnızca bir kez yazılır.
8. **Sıralı, doğru kapanış** (`main.go`) — Ctrl+C/kapatma sinyali önce tüm arka plan goroutine'lerine dur der ve GERÇEKTEN durmalarını bekler (bilinen eş listesinin son bir kez kaydedilmesi dahil), ancak ONDAN SONRA ağ/veritabanı kaynaklarını kapatır — ters sırada yapmak, hâlâ çalışan bir işlemin kapalı bir kaynağı kullanmaya çalışmasına ya da kapanış anındaki kaydın hiç yapılamamasına yol açabilirdi.
9. **Bağlantı açma** (`links.go`) — sonuç detayında geçen `http(s)://` bağlantıları otomatik bulunup numaralandırılır ve OSC 8 terminal hyperlink dizisiyle **gerçekten fare ile tıklanabilir** hale getirilir (Windows Terminal, VS Code'un tümleşik terminali, iTerm2, çoğu modern Linux terminali destekler — tıklanınca terminal url'yi doğrudan kendisi açar). Bunu desteklemeyen terminaller için de `o` (ilki) ya da `1`-`9` tuşlarıyla işletim sisteminin varsayılan tarayıcısında açma imkânı vardır (AND'ın DeltaX eklentisindeki `klasoruAc` ile aynı desen). Yalnızca http(s) şemaları tanınır — `file://`, `javascript:` gibi başka şemalar kasıtlı olarak reddedilir; içerik güvenilmeyen eşlerden geldiğinden bu önemli bir güvenlik sınırı.
10. **Betiklenebilir CLI modu** (`cli.go`) — `-search "sorgu"` bayrağı TUI'yi ve ağı HİÇ açmadan yerel indekste arar, sonuçları stdout'a yazıp çıkar (`-json` ile makine tarafından okunabilir çıktı, `-tip` ile tür filtresi, `-limit` ile sonuç sayısı, `-sayfa` ile sayfalama). Sonuç sıfırsa ve bir "şunu mu demek istediniz?" adayı bulunursa, `stdout`'u (ör. `| jq`) bozmadan `stderr`'e yazılır.
11. **Panik kalkanı** — içerik ağdaki GÜVENİLMEYEN eşlerden geldiğinden (bkz. yukarıdaki güvenlik ilkesi), ayrıştırma/işleme mantığındaki beklenmeyen bir kenar durumu artık TÜM PROGRAMI çökertmez: hem TUI'nin `Update` döngüsü hem de içerik/sorgu işleyen tüm arka plan goroutine'leri (`recoverGoroutine`) bir panik kalkanıyla sarılıdır — bir panik olursa günlüğe yazılıp yoksayılır, program çalışmaya devam eder.
12. **Ayarlanabilir günlük seviyesi ve TTL** (`main.go`) — `-log-level` ile tanılama günlüğünün ayrıntı düzeyi (`hata`/`uyari`/`bilgi`), `-ttl` ile "ağda ara"nın varsayılan atlama sayısı (1-5) çalışma zamanında ayarlanabilir.
13. **Yazara göre arama — "bu yazarın diğer içerikleri"** (`index.go`, `search.go`) — sonuç detayında `a` tuşu, seçili sonucun yazar anahtarına (spoofable YazarAd değil, gerçek kimlik) filtrelenmiş bir aramaya geçer; bu filtre anahtar kelime GEREKTİRMEZ — boş bırakılırsa o yazarın tüm içeriği en yeniden eskiye listelenir (gerçek arama motorlarındaki "yazar sayfası" davranışı). Hem yerel indekste hem "ağda ara"da (ağ protokolü de yazar filtresini taşır) çalışır; `ctrl+y` ile temizlenir. CLI'da karşılığı `-yazar <anahtar>` (`-search` olmadan bile tek başına kullanılabilir).
14. **Performans: WAL modu ve bağlantı havuzu** (`index.go`) — SQLite, AND'ın kendi `internal/storage`'ıyla aynı gerekçeyle WAL günlük modunda ve tek bağlantıya sabitlenmiş (`SetMaxOpenConns(1)`) çalışır: sürekli gelen gossip yazımlarıyla arama sorguları arasındaki kilit çekişmesini azaltır. `bilinen(tip)` üzerine bir indeks eklendi — sync'te her yeni eşe gönderilen "bilinen ID" listesi artık tam tablo taraması yapmıyor.
15. **Çökme-güvenli dosya yazımı** (`fsutil.go`) — `kimlik.json` ve `bilinen_peerler.json`, yarım/bozuk bırakılabilecek doğrudan bir yazma yerine, geçici dosya + atomik yeniden adlandırma (`atomicYaz`) ile yazılır; işlem tam yazma sırasında kesintiye uğrarsa (güç kesintisi, kill) dosya ya tamamen eskisi ya da tamamen yenisiyle kalır.
16. **AND ile aynı görsel dil ve gerçek bir tema sistemi** (`view.go`, `tema.go`) — AND'ın ortak tasarım sistemi `internal/ui` burada bağımsız olarak tekrarlanır (program ayrı bir Go modülü olduğu için paketi import edemez): AND'ın varsayılan "Klasik" temasıyla aynı ANSI 256 kod noktaları, aynı blok harfli "AND" logosu, seçili satırda tam genişlik zemin ve `▌` vurgu çubuğu, `━`/`─` doluluk çubukları ve tek satırlık `✓`/`✗` bildirimleri. Her ekranın altındaki satır artık eski tek satırlık durum çubuğu değil, AND'daki gibi "tuş açıklama" çiftlerinden oluşan bir kısayol satırıdır; sığmayan çift bölünmeden alt satıra geçer. Uzun satırlar (alıntılar, bağlantılar, paylaşım kodları) ekran genişliğine göre kırpılır ya da sarılır, çerçeve hiçbir genişlikte taşmaz. AND gibi **terminalin tamamını kullanır**: her ekran tam ekran bir çerçevenin içindedir (üst kenarda `AND` rozeti, tema adı ve sürüm), kısayol satırı ve bildirimler her zaman en alttadır, açılış ekranı çerçevenin ortasında durur. Sonuç gövdesi, kartlar ve giriş kutuları en fazla 100 sütun genişlikte kalır; Durum ekranının kartları geniş terminallerde iki sütuna dizilir. Renkler sabit değildir: program açılışta **AND'da seçili temayı** okur, kendi `~/.and-arama-motoru/tema.json`'u varsa o önceliklidir (bkz. [Tema](#tema)).
17. **Kişisel site sayfaları da aranabilir** (`site.go`) — AND'ın kişisel site özelliğinin (`internal/site`) bağımsız bir yeniden uygulaması: forum/sohbetle AYNI güvenlik disipliniyle (ed25519 imza, sanal olmayan gerçek kimlik) sayfaları indeksler, ama forum post/yanıtlarının "yaz-bir-kez" doğasından FARKLI olarak DEĞİŞEBİLİR bir içerik türü olduğunu bilir: aynı yazar/slug yeniden yayınlandığında indeksteki eski sürümün üzerine yazılır (silinip yeniden eklenmez), geç gelen (replay) bir eski sürüm ise elimizdeki daha yeniyi asla ezmez.
18. **Google/Yandex tarzı gelişmiş sorgu sözdizimi** (`index.go`'nun `ftsSorguOlustur`'u) — `"tam ifade"` (bitişik eşleşme), `-hariç` (sonuçlardan dışlama), `terim1 OR terim2` (ikisinden biri) desteklenir; hiçbiri kullanılmazsa çıktı eski davranışla bayt bayt aynıdır (geriye dönük uyumlu). FTS5'in kendi mini sorgu diline enjeksiyon mümkün değildir — her terim kaçırılıp kendi tırnaklı literal'i içine sarılır.
19. **"Şunu mu demek istediniz?" yazım önerisi** (`yazimonerisi.go`) — yeni bir bağımlılık GEREKTİRMEZ: SQLite FTS5'in `fts5vocab` uzantısıyla indeksteki tüm benzersiz terimleri okuyup elle yazılmış bir Levenshtein (düzenleme mesafesi) fonksiyonuyla en yakın bilinen terimi bulur. Yalnızca arama SIFIR sonuç döndürdüğünde tetiklenir; TUI'de `ctrl+d` ile öneriyi doğrudan uygulayabilirsin.
20. **Sayfalama** (`index.go`, `model.go`) — hem yerel aramada (`pgup`/`pgdown`) hem CLI'da (`-sayfa`) sonuçlar sayfa sayfa gezilebilir; ekstra bir `COUNT(*)` sorgusu koşturmadan "sonraki sayfa var mı" bilgisini vermek için her sayfa istenen sayıdan bir fazla satır çekilip son satır atılır.
21. **Arama konsolu istatistikleri** (`index.go`'nun `TipDagilimiAl`/`EnAktifYazarlarAl`/`GunlukIcerikTrendiAl`'ı) — Durum ekranı, Google Search Console/Yandex Webmaster'a benzer şekilde içerik türü dağılımını, en aktif yazarları ve son 14 günün günlük içerik trendini gösterir. Bu sorgular yalnızca Durum ekranına GİRİLDİĞİNDE bir kez koşturulur (mevcut 2 saniyelik durum tazelemesinde DEĞİL) — `SetMaxOpenConns(1)` ile tek bağlantıya sabitlenmiş veritabanında gereksiz yük oluşturmamak için.
22. **Kişisel site sayfaları için paylaşım kodu** (`index.go`'nun `SiteSayfasiBul`'u, "Kodla Git" ekranı) — bir site sayfasının kişisel siteler için zaten var olan kimliği (`yazarAnahtarı/slug`) artık Sonuç Detayı'nda **seçilebilir bir kod** olarak gösterilir (kopyalayıp birine mesajla/mailla gönderebilirsin). Alıcı bu kodu ana menüdeki "Kodla Git" ekranına yapıştırıp `enter`'a basınca (ya da CLI'da `-site <kod>` ile) sayfayı — o yazarı önce forumda/sohbette "tesadüfen bulmayı BEKLEMEDEN" — doğrudan açabilir. CLI'daki `-site` bayrağı YEREL bir sorgu olarak kalır (ağa çıkmaz, betiklenebilirliğin öngörülebilir/hızlı kalması için); TUI'deki "Kodla Git" ekranı ise madde 25'te açıklanan ağ geri düşümüne sahiptir. **Önemli sınır:** bu, yalnızca AND ya da AND Arama Motoru çalıştıran ve aynı P2P ağına bağlı kişiler arasında işler — gerçek bir web adresi/tarayıcı linki DEĞİLDİR (P2P mimarisinde bunun için ayrı, daha büyük bir HTTP köprüsü gerekir, kapsam dışıdır).
23. **Sıralama modu: Alaka ⇄ Yeni** (`index.go`'nun `SiralamaModu`/`AraGelismisSirali`'si, Ara ekranının `ctrl+r`'ı) — varsayılan (Alaka) sıralama sütun ağırlıklı `bm25()` + tazelik boostunu kullanmaya devam eder; `ctrl+r` bunu YOK SAYIP sonuçları saf kronolojik (en yeniden eskiye) sıralayan bir moda geçirir. `AraGelismis`'in dış imzası (mevcut çağıranlar: CLI, testler) KORUNUR — yeni `AraGelismisSirali` bunun üzerine ekstra bir parametre ekleyen bir üst kümedir.
24. **Kayıtlı arama / "takip et"** (`index.go`'nun "Kayıtlı arama takibi" bölümü, Ara ekranının `ctrl+t`'si, yeni "Takip Edilenler" ekranı) — geçerli sorgu/tür/yazar kombinasyonunu YEREL bir deftere kaydeder; bundan sonra forum/sohbet/site gossip'inden **gerçekten yeni** bir kayıt bu takibe uyduğunda (aynı `ftsSorguOlustur`/FTS5 MATCH mantığı, "bu kayıt sorgu X ile eşleşir mi" sorusu için YENİDEN KULLANILIR) bir sayaç artar; Ana Menü'de "Takip Edilenler (N)" rozeti olarak görünür. Tamamen yerel bir defterdir — ağa hiçbir şey yayınlanmaz, salt-okunur eş ilkesini bozmaz.
25. **Hedefli P2P sorgu** (`search.go`'nun `aramaTekEsIstegi`'si, Sonuç Detayı'nın `t` tuşu) — bir ağ sonucunun `KaynakPeer`'ını, TÜM eşlere yayılan `ctrl+n`'in aksine TEK bir hedef olarak (TTL=1, ileri iletim yok) sorgular; "bu eşin güncel indeksinde başka ne var" diye kontrol etmek için. Aynı `/and-arama-motoru/1.0.0` protokolünü ve TEK boğaz noktasındaki (`aramaIstekGonderRaw`) imza doğrulamasını kullanır, ayrı bir protokol GEREKTİRMEZ.
26. **Eş güven göstergesi** (`search.go`'nun `esGuvenDefteri`'si, Durum ekranı) — bu düğümün DOĞRUDAN sorguladığı her eşin kaç geçerli/geçersiz imzalı sonuç döndürdüğünü, süreç ömürlü bellek-içi bir defterde izler ve Durum ekranında en çok etkileşimde bulunulan 5 eşi listeler. Bu bir ENGELLEME mekanizması DEĞİLDİR (güvenlik zaten `dogrulaAgSonucu`'nun kendisiyle sağlanır) — yalnızca "hangi eşler şüpheli davranıyor" diye görsel bir ipucudur; diske yazılmaz, her yeniden başlatmada sıfırlanır.
27. **"Kodla Git" ağa düşsün** (`model.go`'nun `agdaSiteAraCmd`'si) — bir paylaşım kodu YEREL indekste bulunamadığında (ve bağlı eş varsa), TUI otomatik olarak kodun yazar anahtarını hedefleyen bir ağ sorgusu (madde 5'teki AYNI arama protokolü, `Metin=""` + `Tip=TipSite` + `YazarKey`) tetikler ve tam `KaynakID` eşleşmesini arar — henüz bu düğüme senkronize OLMAMIŞ ama ağda hâlâ mevcut bir sayfa da bu şekilde bulunabilir. Bulunan sayfa (zaten imza doğrulamasından geçmiş olduğundan) güvenle yerel indekse de kalıcılaştırılır.
28. **Site haritası** (`model.go`'nun "Kodla Git" ekranı) — "Kodla Git"e `yazarAnahtarı/slug` yerine yalnızca `yazarAnahtarı` (hiç `/` içermeyen bir girdi) yapıştırırsan, TEK bir sayfa yerine o yazarın TÜM site sayfaları (`index.go`'nun `yazarinTumIcerigi`'si YENİDEN KULLANILIR) Ara ekranında TipSite filtresiyle listelenir; oradan `ctrl+n` ile aynı yazarın ağdaki diğer sayfalarını da arayabilirsin.
29. **Sayfa içi çapraz bağlantı** (`links.go`'nun `siteBaglantilariBul`'u, Sonuç Detayı'nın `g` tuşu) — bir site sayfası gövdesinde geçen `and-site://` bağlantıları (bkz. madde 22) artık `http(s)://` bağlantıları gibi ayrıca listelenip OSC 8 ile tıklanabilir kılınır; `g` tuşu, protokol kaydı GEREKMEDEN, uygulama İÇİNDEN ilk iç bağlantıya gider (yerelde yoksa madde 27'deki AYNI ağ geri düşümüyle). Kişisel bir "wiki" gibi sayfadan sayfaya gezinmeyi sağlar.
30. **Paylaşım kodu için QR kod** (`qr.go`, Sonuç Detayı'nın `k` tuşu) — bir site sayfasının `and-site://` bağlantısını, `k` tuşuna basınca terminalde yarım blok karakterleriyle (▀▄█) taranabilir bir QR koda çevirir (`skip2/go-qrcode`, saf Go — CGO gerektirmez); kodu bir cihaza kopyala-yapıştır yapmak yerine telefonla doğrudan okutmak için.
31. **`and-site://` protokol kaydı artık Linux'ta da var** (`protokol_linux.go`) — Windows'un (`protokol_windows.go`, HKCU registry) yanı sıra Linux masaüstü ortamları için `xdg-mime`/`.desktop` dosyası mekanizmasıyla (yalnızca `$HOME/.local/share/applications` altına, kök yetkisi GEREKMEZ) `-protokol-kaydet`/`-protokol-kaldir` artık Linux'ta da çalışır. macOS için bu hâlâ eksik — Launch Services (`lsregister`) çıplak bir ikiliyi değil, `CFBundleURLTypes` bildiren gerçek bir `.app` paketini kayıt birimi olarak kabul eder; bu, `go build`'in tek bir ikili üretmesinin ötesinde ayrı bir paketleme adımı gerektirir, kapsam dışı bırakıldı.
32. **Düzenlenmiş site sayfaları da senkronla gelir** (`site.go`'nun `siteSyncIstek.KnownVersions`'ı, `index.go`'nun `SiteSurumleriAl`'ı) — site sync isteği artık yalnızca bilinen slug'ları değil, elimizdeki her sayfanın sürümünü (`UpdatedAt`, nanosaniye hassasiyetinde) de taşır; AND'ın sürüm farkına duyarlı yanıtlayıcısı (`internal/site/sync.go`) bilinen bir slug'ı, elindeki sürüm DAHA YENİYSE yine gönderir. Önceden bir sayfanın düzenlenmiş hali bu düğüme senkronla hiç ulaşmazdı — yalnızca o an canlı gossip'i görmüşsek indekslenirdi. Bu alanı tanımayan eski bir AND sürümü onu yok sayar (eski davranış); istek AND'ın 1 MB'lık istek sınırını aşacak kadar büyürse sürümler düşürülüp eski biçimde gönderilir (yeni sayfalar kaybedilmesin diye).
33. **Ağdan gelen metin terminale temizlenerek basılır; arama protokolü yükseltmeye karşı sınırlıdır** (`guvenlik.go`, `search.go`) — imzalı bir içerik yalnızca KİMİN yazdığını kanıtlar, ne yazdığının zararsız olduğunu değil. Başlık, gövde, ad, kategori ve paylaşım kodları ekrana (ve `-search` komut satırı çıktısına) basılmadan önce terminal kaçış dizilerinden (pano yazan OSC 52, bağlantıyı yönlendiren OSC 8, imleç oynatan CSI), C0/C1 kontrol karakterlerinden ve yazı yönü karakterlerinden temizlenir; tıklanabilir bağlantı adresinde kontrol karakteri varsa bağlantı hiç oluşturulmaz. Temizlik depolamada değil ekranda yapılır — kayıtlar başka eşlere imzalarıyla aktarıldığından metni değiştirmek imza doğrulamasını bozardı. Arama protokolünde: sorgular yalnızca bu protokolü konuşan eşlere gönderilir (bağlı eşlerin çoğu genel DHT düğümüdür), başka bir eşin sorgusu tek atlamada en fazla 8 eşe iletilir, aynı anda en fazla 16 gelen sorgu işlenir, sorgu metni 256 karakterle, gönderenin bildirdiği bitiş zamanı bu düğümün zaman aşımıyla sınırlanır ve geçersiz yazar anahtarı ya da içerik türü taşıyan istekler reddedilir. Arama sunucusu artık herhangi bir eşe bağlanmadan ÖNCE kaydedilir, böylece eşler bu düğümün arama motoru olduğunu ilk tanışmada öğrenir.

## Gereksinimler

- **Go 1.26 veya üzeri** (kaynaktan derlemek için — `go.mod`'daki sürüm)
- **İşletim sistemi:** Windows, Linux ya da macOS (libp2p ve `modernc.org/sqlite` üçü de saf Go/CGO'suz olduğundan çapraz derleme dahil sorunsuz çalışır)
- **Ağ erişimi:** çıkış (outbound) bağlantılara izin veren bir internet bağlantısı — DHT önyükleme ve eş keşfi için gereklidir; yalnızca yerel ağdaki eşlerle konuşmak istersen mDNS tek başına yeterlidir
- **Disk:** yalnızca indekslediğin içerik kadar (tipik kullanımda birkaç on MB'lık bir SQLite dosyası)
- CGO/harici derleyici **gerekmez** — tüm bağımlılıklar saf Go'dur

## Kurulum ve çalıştırma

```bash
go build -o and-arama-motoru .
./and-arama-motoru

# Windows
go build -o and-arama-motoru.exe .
.\and-arama-motoru.exe
```

AND'ın kendisiyle hiçbir bağlantı/ortam değişkeni gerekmez — bağımsız çalışır.

### Komut satırı seçenekleri

```
-version         sürüm bilgisini yazdır ve çık
-veri-dir <yol>  veri dizini (varsayılan: $ARAMA_MOTORU_VERI_DIR ya da ~/.and-arama-motoru)
-log-level <s>   günlük ayrıntı düzeyi: hata|uyari|bilgi (varsayılan: bilgi)
-ttl <n>         "ağda ara" için varsayılan atlama (TTL) sayısı, 1-5 (varsayılan: 3)

-search <sorgu>  yerel indekste ara, sonuçları yazdır ve çık (TUI/ağ açılmaz)
-tip <t>         -search ile: sonuçları türe göre sınırla (konu|yanit|sohbet|site)
-yazar <anahtar> yalnızca bu yazar anahtarına ait sonuçlar; -search boşsa bile
                 tek başına o yazarın tüm içeriğini listeler
-limit <n>       -search ile: en fazla kaç sonuç (varsayılan: 20)
-sayfa <n>       -search ile: sayfa numarası, 1'den başlar (varsayılan: 1)
-site <kod>      bir site paylaşım kodunu (yazarAnahtarı/slug) yerel indekste
                 ara, sayfayı yazdır ve çık (TUI/ağ açılmaz) — bkz. aşağıdaki
                 "Site sayfası paylaşımı"
-json            -search/-site ile: sonucu JSON olarak yazdır
-protokol-kaydet  and-site:// bağlantılarını bu programla açacak şekilde
                  işletim sistemine kaydet (Windows ve Linux) ve çık
-protokol-kaldir  and-site:// bağlantı kaydını kaldır (Windows ve Linux) ve çık
```

Sürüm, derleme zamanında `-ldflags "-X main.Version=vX.Y.Z"` ile ezilebilir (varsayılan `v0.1.0`); isteğe bağlı olarak `-X main.BuildCommit=...` ve `-X main.BuildDate=...` de doldurulabilir, doluysa `-version` çıktısında (Go sürümü/OS/mimariyle birlikte) gösterilirler. `-h`/`-help` çıktısı Genel/Arama/Site paylaşımı/Ağ olarak kategorilere ayrılmıştır.

Örnek — betiklenebilir arama:

```bash
./and-arama-motoru -search "merhaba" -json -limit 5 | jq '.[].baslik'
```

### Site sayfası paylaşımı

Kişisel site sayfalarının gerçek bir web adresi yoktur (P2P ağda böyle bir kavram yok) — ama her sayfanın zaten `yazarAnahtarı/slug` biçiminde bir kimliği var, bu da **paylaşım kodu** olarak kullanılabilir. Bir sayfanın Sonuç Detayı ekranında bu kod "Kod" etiketiyle gösterilir; fare ile seçip kopyalayabilir, ya da `c` tuşuna basarak doğrudan sistem panosuna kopyalayabilirsin (OSC 52 terminal protokolüyle — SSH üzerinden bile YEREL panona kopyalar; desteklemeyen terminallerde sessizce yok sayılır). Kodu alan kişi:

```bash
# CLI:
./and-arama-motoru -site "a3f9…92c1/hakkimda"

# ya da TUI'de: Ana Menü → "Kodla Git" → kodu yapıştır → enter
```

**Sınır:** bu yalnızca AND/AND Arama Motoru çalıştıran kişiler için çalışır — bir tarayıcıda açılabilecek gerçek bir web linki değildir. Alıcının o sayfayı zaten (forum/sohbet/senkronizasyon yoluyla) yerel indeksine almış olması GEREKMEZ: "Kodla Git" ekranı, kod yerelde bulunamazsa ve bağlı bir eş varsa otomatik olarak ağda da arar ("Kodla Git ağa düşsün" — bkz. Mimari'nin 27. maddesi); CLI'daki `-site` bayrağı ise betiklenebilirliğin hızlı/öngörülebilir kalması için YEREL aramada kalır.

Bir sayfa kodu yerine yalnızca `yazarAnahtarı` (hiç `/` içermeyen bir girdi) yapıştırırsan, "Kodla Git" o yazarın TÜM site sayfalarını ("site haritası") listeler — bkz. Mimari'nin 28. maddesi.

#### Tıklanabilir bağlantı olarak (and-site://)

Paylaşım kodu, aynı zamanda `and-site://<yazarAnahtarı>/<slug>` biçiminde özel bir URI şemasına da sarılır — Sonuç Detayı ekranında "Link" satırında OSC 8 ile tıklanabilir olarak gösterilir (destekleyen terminallerde fare ile tıklanabilir/seçilebilir). Bunu işletim sisteminin GERÇEKTEN tıklanabilir bir bağlantı olarak tanıması (ör. bir sohbet uygulamasında ya da tarayıcı adres çubuğunda) için bir kere kaydettirmen gerekir:

```bash
# Windows/Linux — yalnızca geçerli kullanıcı için (yönetici/kök hakkı gerekmez):
./and-arama-motoru -protokol-kaydet

# Kaydı kaldırmak için:
./and-arama-motoru -protokol-kaldir
```

Kaydettikten sonra bir `and-site://…` bağlantısına tıklamak, bu programı o kodla (`-site` bayrağıyla tamamen aynı şekilde) başlatır: TUI/ağ açılmaz, sayfa doğrudan yazdırılır. **Windows'ta** `HKEY_CURRENT_USER` altına, **Linux'ta** `$HOME/.local/share/applications` altına bir `.desktop` dosyası + `xdg-mime` ile kaydedilir. **macOS için henüz uygulanmadı** — Launch Services (`lsregister`), çıplak bir komut satırı ikilisini değil, `CFBundleURLTypes` bildiren gerçek bir `.app` paketini kayıt birimi olarak kabul eder; bu, ayrı bir paketleme adımı gerektirir, kapsam dışı bırakıldı. Bu platformda `-site` bayrağı, "Kodla Git" ekranı ve sayfa içi `and-site://` bağlantılarındaki `g` tuşu (bkz. Mimari'nin 29. maddesi) olduğu gibi çalışmaya devam eder, yalnızca "linke tıklayınca otomatik açılma" kolaylığı eksiktir.

Bu, yukarıdaki temel sınırı KALDIRMAZ: hâlâ yalnızca AND/AND Arama Motoru kurulu VE protokolü kaydettirmiş VE o içeriği zaten kendi indeksine almış biri için işler — gerçek bir web sunucusuna/DNS'e dayanmaz.

### Gelişmiş sorgu sözdizimi

Arama kutusu (hem TUI'de hem `-search`'te) Google/Yandex'e benzer üç ek söz dizimini anlar — hiçbiri kullanılmazsa davranış eskisiyle aynıdır:

```bash
./and-arama-motoru -search '"tam ifade"'      # bitişik eşleşme, prefix değil
./and-arama-motoru -search 'elma -armut'      # "armut" geçen sonuçlar dışlanır
./and-arama-motoru -search 'kedi OR köpek'    # ikisinden biri (BÜYÜK harf OR)
```

### Tema

Program, **AND'da seçili temayı kendiliğinden izler**: kendi `tema.json`'u yoksa açılışta AND'ın `theme.json` dosyasını okur (`AND_DATA_DIR` ayarlıysa orada, değilse AND'ın varsayılan veri dizininde) ve aynı renkleri kullanır. AND'da Ayarlar → Temalar'dan tema değiştirdiysen, Arama Motoru bir sonraki açılışta aynı temayla açılır. AND hiç kurulu değilse ya da tema seçilmemişse varsayılan renkler AND'ın kendi "Klasik" temasıyla birebir eşleşir.

Arama Motoru'na AND'dan farklı bir tema vermek istersen `~/.and-arama-motoru/tema.json`'u elle oluştur. Bu dosya varsa her zaman önceliklidir. Yalnızca değiştirmek istediğin alanları yaz, geri kalanı varsayılanında kalır:

```json
{
  "tema_adi": "Gece Mavisi",
  "accent": "39",
  "accent2": "86",
  "ok": "42",
  "warn": "220",
  "err": "203",
  "muted": "241",
  "sel_bg": "24",
  "sel_fg": "255",
  "rol": "33",
  "panel_bg": "236"
}
```

Varsayılan değerler (AND'ın Klasik teması): `accent` 63, `accent2` 86, `ok` 78, `warn` 214, `err` 203, `muted` 245, `sel_bg` 57, `sel_fg` 255, `rol` 33, `panel_bg` 236.

AND'ın yerleşik temalarının değerleri aşağıda; AND'da seçili temadan farklı bir yerleşik temayı yalnızca Arama Motoru'nda kullanmak istersen bunları `tema.json`'a yazman yeterli:

| AND teması | `accent` | `sel_bg` | `sel_fg` | `accent2` | `rol` |
|---|---|---|---|---|---|
| Turkuaz | 80 | 23 | 255 | 216 | 141 |
| Gece Moru | 99 | 55 | 255 | 86 | 75 |
| Kehribar | 214 | 94 | 230 | 180 | 117 |
| Orman | 114 | 22 | 255 | 223 | 117 |
| Mono | 255 | 240 | 255 | 252 | 250 |

Değerler lipgloss'un kabul ettiği herhangi bir renk dizgesi olabilir: ANSI 256 kod noktası (`"63"`) ya da `"#RRGGBB"` true-color hex. `tema_adi` doluysa çerçevenin üst kenarında, sürüm numarasının yanında görünür. Program her açılışta bu dosyayı okur; dosya bozuksa sessizce varsayılan temaya döner (hata vermez).

## Kullanım

Herhangi bir ekranda `f1`'e basmak, bu tablodaki TÜM kısayolları ve gelişmiş sorgu sözdizimini uygulama İÇİNDE gösteren bir yardım ekranı açar (`esc` ile bulunduğun ekrana geri döner, yazmakta olduğun sorgu/kod kaybolmaz). `f1` bilerek metin üretmeyen bir tuş: Arama/Kodla Git kutularına yazarken kısayolun kendisi asla sorguya karışmaz.

| Ekran | Açıklama |
|-------|----------|
| Ara | Her tuş vuruşunda yerel indekste anında ara; `tab` ile içerik türüne göre filtrele (Tümü/Konu/Yanıt/Sohbet/Site); `ctrl+n` ile ağda (çok atlamalı) sor; `ctrl+r` ile sıralamayı değiştir (Alaka ⇄ Yeni); `ctrl+t` ile geçerli aramayı takibe al; `ctrl+y` ile aktif yazar filtresini temizle; `pgup`/`pgdown` ile sayfalar arasında gez; sonuç sıfırsa ve bir "şunu mu demek istediniz?" önerisi varsa `ctrl+d` ile uygula |
| Kodla Git | Birinden aldığın bir site paylaşım kodunu (`yazarAnahtarı/slug`) yapıştırıp `enter` ile doğrudan o sayfaya git (yerelde yoksa, bağlı eş varsa otomatik olarak ağda da arar); yalnızca `yazarAnahtarı` yapıştırırsan o yazarın tüm site sayfalarını (site haritası) listeler — bkz. [Site sayfası paylaşımı](#site-sayfası-paylaşımı) |
| Takip Edilenler | "Takip et" ile kaydedilen aramaları listeler; her satırda okunmamış yeni eşleşme sayısı görünür; `enter` ile bir aramayı yükleyip Ara ekranına geç (rozeti de sıfırlar), `d` ile sil |
| Sonuç Detayı | `a` ile seçili sonucun yazarına filtrelenmiş bir aramaya geç ("bu yazarın diğer içerikleri"); ağ sonuçlarında `t` ile o eşi doğrudan hedefle; site sayfalarında "Kod" satırı paylaşım kodunu gösterir (`c` ile panoya kopyalar, `k` ile QR kodunu göster/gizle), "Link" satırı ise aynı kodun tıklanabilir `and-site://` bağlantısını gösterir; gövdede geçen `and-site://` iç bağlantılarda `g` ile gez — bkz. [Site sayfası paylaşımı](#site-sayfası-paylaşımı) |
| Durum | Kendi peer ID'n, bağlı eş sayısı, indeksteki kayıt sayısı; ayrıca içerik türü dağılımı, en aktif yazarlar, son 14 günün günlük içerik trendini ve doğrudan sorgulanan eşlerin güven göstergesini gösteren "arama konsolu" paneli |

## Veri dizini dosyaları

| Dosya | İçerik |
|-------|--------|
| `kimlik.json` | Bu düğümün BIP39 mnemonic'i (ağ kimliği) |
| `arama_motoru.db` | SQLite FTS5 arama indeksi |
| `bilinen_peerler.json` | Hızlı yeniden bağlanma için son bilinen eş adresleri |
| `tema.json` | *(isteğe bağlı)* Özel renk teması — bkz. [Tema](#tema) |
| `gunluk.log` | Tanılama günlüğü |

## Kapsam ve sınırlar

- **Yalnızca okuyucu bir eştir** — post/mesaj/site sayfası oluşturmaz, moderasyon zincirini (founder/moderatör sertifikaları, `mod_delete`) uygulamaz, başka eşlerin forum/sohbet/site senkronizasyon isteklerine yanıt vermez (yalnızca kendi arama protokolüne yanıt verir).
- **"Ağda ara" en fazla `aramaVarsayilanTTL` (varsayılan 3) atlama öteye ulaşır** — sonsuza kadar yayılmaz (kasıtlı bir sınır: hem trafiği makul tutmak hem de bir sorgunun ağda sonsuza kadar dolaşmasını önlemek için), ve yalnızca bu AYNI programın başka bir kopyasını çalıştıran eşler yanıt verebilir (AND'ın kendisi bu protokolü konuşmaz).
- **NAT/relay/hole-punching desteği AND'ınkinden daha sınırlı** (bkz. `network.go`'nun doc yorumu) — sıkı bir NAT arkasındaysan giden bağlantılar yine de çalışır, ama gelen "ağda ara" sorgularına ulaşılabilirlik AND kadar güvenilir olmayabilir.
- **İmza doğrulama mantığı AND'ın kaynak kodundan elle transkript edildi** (import edilemediği için) — `forum.go`/`chat.go`'nun doc yorumlarında bu açıkça belirtilir; testler kendi içindeki tutarlılığı (imzala→doğrula, kurcalama tespiti) kanıtlar.
- **Kademlia DHT keşfi, tüm Kademlia tabanlı sistemlerin (IPFS dahil) paylaştığı bilinen bir zayıflığa açıktır** ([GO-2024-3218](https://pkg.go.dev/vuln/GO-2024-3218) / CVE-2023-26248): kötü niyetli düğümler DHT'yi manipüle ederek belirli içeriğin bulunmasını zorlaştırabilir ("içerik sansürü"). Bu, `go-libp2p-kad-dht` kütüphanesinin PROTOKOL SEVİYESİNDE bir sınırıdır — henüz bir düzeltmesi yok. Etkisi bu programda kısmen azaltılır: keşif yalnızca DHT'ye bağlı değildir, mDNS (yerel ağ) ve bilinen-eş listesi (`peers.go`) de bağımsız keşif yolları sağlar; ayrıca AND'ın kendisi de AYNI kütüphaneyi kullandığından bu risk zaten ekosistemin tamamının paylaştığı, bu programa özgü olmayan bir durumdur.
- **Hedefli P2P sorgu (`t` tuşu), hedef eşin adresini bu düğümün peerstore'unda bulmayı gerektirir** — çok atlamalı bir yayılma zincirinde yalnızca DOLAYLI olarak görülmüş (hiç doğrudan bağlanılmamış) bir eş hedeflenirse dial başarısız olabilir; bu durumda normal bir "stream açılamadı" hatası gösterilir.
- **Eş güven göstergesi (Durum ekranı) süreç ömürlüdür** — diske yazılmaz, her yeniden başlatmada sıfırlanır ve bir engelleme mekanizması DEĞİLDİR, yalnızca görsel bir ipucudur (asıl güvenlik `dogrulaAgSonucu`'nun imza doğrulamasıyla sağlanır).
- **Kayıtlı arama takibi ("takip et"), yalnızca bu düğümün GÖRDÜĞÜ içeriği değerlendirir** — arka planda ağı proaktif olarak taramaz; bir eşleşme, ancak ilgili içerik gossip/senkronizasyon yoluyla normal şekilde bu düğüme ulaştığında sayılır.

## Ekosistem

AND Arama Motoru, "AND Proje" ekosisteminin üç bağımsız parçasından biridir — üçü de **aynı ağı** (aynı GossipSub konuları, aynı Ed25519 imza biçimi, aynı senkronizasyon protokolleri) konuşan ama birbirinden bağımsız çalışan, ayrı programlardır:

| Proje | Ne işe yarar |
|-------|--------------|
| **AND** | Ağın kendisini oluşturan asıl P2P terminal forum/sohbet uygulaması — bir düğüm olmadan ağda içerik yayınlanamaz |
| **AND Arama Motoru** *(bu proje)* | Ağdaki içeriği bağımsız olarak indeksleyip aratan salt-okunur bir eş |
| **and-vscode** | AND'ın eklenti API'si üzerinden VS Code içinde canlı Forum/DM/Sohbet/Ağ/Site paneli sunan bir editör uzantısı — kişisel site sayfaları editörde düzenlenip yayınlanır, paylaşım kodları buradaki "Kodla Git" ile birebir uyumludur |

Aynı aileden olduklarını gösteren ortak noktalar: aynı ANSI 256 renk paleti ve aynı "AND" blok-harf logosu, aynı BIP-39/Ed25519 kimlik modeli, aynı GossipSub/senkronizasyon protokolleri. Detaylar için ana projenin [README.md](../AND/README.md) dosyasındaki "Ekosistem" bölümüne bak.

## Katkı

Hata bildirimi, özellik önerisi ve kod katkısı için [CONTRIBUTING.md](CONTRIBUTING.md) dosyasına bak.

## Güvenlik

Bir güvenlik açığı bulduysan lütfen herkese açık bir issue **açma** — [SECURITY.md](SECURITY.md) dosyasındaki bildirim sürecini izle.

## Lisans

Bu proje [GNU Affero General Public License v3.0](LICENSE) ile lisanslanmıştır — AND'ın kendisiyle aynı lisans.
