# VS Code eklentisi AND'la uyumlu: forum, bağlantı ayarı, temalar

Tarih: 2026-11-25. İstek: "vs code eklentisini ande uyumlu olarak geliştir ve güncelle".
Onaylanan seçimler (AskUserQuestion): Canlı Panel'in Forum sekmesi yenilenir; AND Ayarlar'ına
"VS Code bağlantısı" anahtarı eklenir; kapsam onaylandı ("Onaylıyorum, başla").

## Neden

AND son sürümlerde ilerledi, and-vscode 0.9.0'da kaldı:

- Forum eklentiye taşındı ve eklenti API'si herkese konu listesi (`/forum/posts`), canlı değişim
  akışı (`/forum/changes`), kendi konusunu silme (`/forum/delete-own`), yazar profili (`/profile`)
  ve moderatöre yasaklama (`/mod/ban`) veriyor. VS Code'da normal üye konu listesini göremiyor,
  yalnızca ID'sini bildiği konuyu açabiliyor.
- AND'ın 10 hazır temasından 4'ü (Gül, Okyanus, Gün Batımı, Lavanta) ve Özel renk VS Code'un tema
  adı tablosunda yok.
- VS Code'un bağlanması `AND_ENABLE_EXTERNAL_API=1` istiyor; AND artık çift tıklamayla masaüstü
  uygulaması olarak açıldığı için ortam değişkeni pratikte ayarlanamıyor.

Başarı: AND'ı çift tıklayıp Ayarlar'dan "VS Code bağlantısı"nı açan biri VS Code'da forumu AND'daki
gibi gezer, okur, yanıtlar, konu açar, kendi konusunu siler, yazar profilini görür; moderatör
yasaklar. Tema adı her dilde doğru görünür.

## Hedefler / hedef dışı

- Hedef: Forum sekmesi (iki bölme), canlı güncelleme, profil, yasaklama, kalıcı bağlantı ayarı
  (masaüstü + terminal), eksik tema adları, 6 dil, testler, belgeler, sürüm 0.10.0, .vsix.
- Hedef dışı: VS Code'da ayrı forum ağacı (kullanıcı panel sekmesini seçti); Yönetici Paneli'nin
  yasak listesi/geçmiş/moderatör atama ekranları (AND'daki Yönetici Paneli'nde kalır); forum
  taslak dosyalarını (`yanit_taslaklari.json`, `taslaklar_<kategori>.json`) paylaşmak — forum
  eklentisiyle aynı anda yazma çakışması riski; yazılan metin panel açık kaldıkça korunur.
- Anti-hedef: bağlantı ayarı token'sız dinleyicinin erişimini genişletmemeli (yine yalnızca aynı
  işletim sistemi kullanıcısı); canlı güncelleme yazılmakta olan yanıtı silmemeli; eski AND'da
  (yeni uçlar yokken) panel bozulmamalı.

## Yaklaşımlar

Forum: (1) Canlı Panel'in Forum sekmesini yenilemek — seçildi: panelin geri kalanıyla aynı görünüm,
tek yerde. (2) Ek olarak VS Code ağaç görünümü — daha fazla iş, kullanıcı istemedi.
Bağlantı: (1) AND Ayarlar'ında kalıcı anahtar — seçildi. (2) Yalnızca `setx` belgesi — çift
tıklayan kullanıcı için yine zor.

## Tasarım

### AND: VS Code bağlantısı ayarı

- `pluginapi`: veri dizininde `harici_baglanti.json` (`{"acik": true}`, 0600, atomik yazım).
  `Start`, ortam değişkeni `AND_ENABLE_EXTERNAL_API=1` YA DA bu ayar açıksa harici dinleyiciyi açar
  (önceki koşullar aynen: `dataDir` boş değil, `HariciKapat` çağrılmamış).
- `(*Server).HariciDurumu() HariciDurum{Acik, Ortamdan bool}` ve `(*Server).HariciAyarla(acik bool)
  error`: ayarı kaydeder ve dinleyiciyi çalışırken açar/kapatır. Ortam değişkeni açıkken
  kapatmak `ErrHariciOrtamda` döner (değişken her açılışta yeniden açardı); `HariciKapat` ile
  kapatılmış sunucuda `ErrHariciKullanilamaz`.
- Masaüstü: `GET/POST /api/harici`; Ayarlar › Eklentiler sekmesinin başında "VS Code bağlantısı"
  kartı (açıklama + anahtar; ortam değişkeniyle açıksa anahtar kilitli ve notlu).
- Terminal: Ayarlar › Eklentiler sekmesinin başında durum satırı, `v` tuşu açar/kapatır.

### VS Code: Forum sekmesi

- Sol bölme: kategori seçici ("Tüm kategoriler" + konusu olan kategoriler, sayılarıyla), "Yeni konu"
  düğmesi, konu listesi (başlık; yazar, rol rozeti, yanıt sayısı; son etkinlik göreli zamanla).
  Sıra: son etkinlik, yoksa oluşturulma, yeniden eskiye.
- Sağ bölme: seçili konu (başlık, yazar — tıklanınca profil, kategori, tarih, gövde, yanıtlar,
  yanıt kutusu); kendi konunda ya da moderatörsen "Sil"; ya da yeni konu formu; ya da profil
  (ad, rol, konu/yanıt sayısı, ilk görülme, durum, hakkında, etiketler, bağlantı, GitHub, yasak
  durumu, site sayfalarının başlıkları; moderatöre "Yasakla" formu: 1 saat/1 gün/7 gün/30 gün/kalıcı
  + sebep).
- Gövdedeki `[code]…[/code]` / `[code=dil]` blokları kod bloğu olarak çizilir.
- Canlı: panel açıkken uzantı `/forum/changes`'ı uzun yoklamayla izler; değişimde listeyi ve açık
  konuyu yeniden ister. Yeniden çizimde yanıt kutusundaki metin korunur. Açık konu silindiyse
  "Bu konu artık yok" görünür.
- Eski AND (`/forum/posts` 404): moderatöre eski tüm-konular listesi, üyeye "ID ile aç" kutusu ve
  "AND'ı güncelle" notu. `/forum/changes` yoksa canlı izleme durur, Yenile düğmesi çalışır.
- Dar panelde (≤760 px) bölmeler alt alta.

### Temalar ve belgeler

- Tema adı tablosu `ui/theme.go` ile aynı 10 ad + "Özel".
- README (kurulum: önce Ayarlar anahtarı, ortam değişkeni alternatif; forum yetenekleri; eski
  "üye konuları göremez" sınırı kalkar), CHANGELOG 0.10.0, package.json 0.10.0; AND README/README.tr/
  SECURITY harici dinleyici bölümlerine ayar eklenir.

## Kurucuya kalanlar

CSS ayrıntıları, ileti adları, dosya bölünmesi (forum betiği/HTML'i ayrı modülde), testlerin sahte
DOM'u genişletme biçimi.
