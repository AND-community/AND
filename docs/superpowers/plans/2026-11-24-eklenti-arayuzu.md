# Eklenti Arayüzü (1. aşama: altyapı + DeltaX) Uygulama Planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eklentiler masaüstünde terminal yerine AND'ın kendi bileşenleriyle açılır; ilk taşınan eklenti DeltaX.

**Architecture:** Eklenti, AND'ın borular üzerinden konuştuğu JSON Lines protokolüyle ekranını bir bileşen
ağacı olarak gönderir; AND (Go) doğrulayıp temizler ve SSE ile pencereye iletir; pencere (uygulama.js) ağacı
AND'ın tasarım sistemiyle çizer, tıklama ve formları geri yollar. Public `arayuz` SDK'sı bubbletea'yı
çizicisiz çalıştırır; DeltaX `Arayuz()` ekleyip olayları mevcut tuş işleyicilerine sentetik tuşlarla verir.

**Tech Stack:** Go 1.26 (stdlib, bubbletea v1.3.10), vanilla JS + CSS (gömülü `web/`), headless Edge + CDP ile gözle doğrulama.

**Spec:** `docs/superpowers/specs/2026-11-24-eklenti-arayuzu-design.md`

## Global Constraints

- Public SDK'ya yalnızca ekleme: yeni paketler `arayuz`, `arayuz/arayuztest`; `pluginapi.Manifest`'e `UI int \`json:"ui,omitempty"\``. Mevcut hiçbir dışa açık ad değişmez.
- Ortam değişkeni `AND_ARAYUZ_PROTOKOLU` (değer: AND'ın konuştuğu en yüksek sürüm, bugün `1`); `arayuz.Surum = 1`.
- Sınırlar: satır ≤ 4 MiB; derinlik ≤ 24; öğe ≤ 5000; liste/tablo ≤ 2000 öğe; düz metin ≤ 64 KiB; `kod`+`metinAlani`+`fark` içerikleri toplam ≤ 3 MiB; `id` ≤ 512; AND→pencere ekran iletimi en çok 50 ms'de bir; `degisti` 250 ms susmadan sonra.
- Eklenti stderr'i `and.log`'a: satır başına ≤ 4 KiB, saniyede ≤ 20 satır, önek `eklenti <ad>: `.
- Windows'ta arayüz kipindeki eklenti `CREATE_NO_WINDOW` ile başlar; hiçbir konsol penceresi görünmez.
- AND'ın yeni her metni `t()`/`tf()`/`tb()`/`K()` ile; `dil/ceviriler/tui.json`'a en, de, es, fr, ru; `internal/masaustu/metinler.go` yeniden üretilir (`TestArayuzMetinleri`). Çoğul `|` yok.
- DeltaX'in yeni metinleri `Eklentiler/DeltaX/ceviri.json`'a 5 dilde; AND kataloğunda olanlar tekrar edilmez.
- CSP gevşetilmez; pencere eklenti metnini yalnızca `textContent`/`createElement` ile yazar (`TestArayuzHTMLYorumlamaz` yeşil kalır).
- DeltaX'in TUI'si ve mevcut testleri değişmeden yeşil kalır; arayüzden çıkış tuşları (`q`, `ctrl+c`, ana menüde `esc`) asla gönderilmez.
- Commit'ler: AND deposu (Görev 1–7), `Eklentiler/DeltaX` ayrı depo (Görev 8–10). Push yok. Exe derlemeleri yalnızca kullanıcının onayıyla.

## Review Focus

1. Kullanıcı bir alana yazarken eklenti yeni ekran gönderir (DeltaX'in eş yoklaması her birkaç saniyede) → yazılan metin, imleç ve odak korunur. (Görev 6, vitrin kipinde CDP denetimi)
2. Eklenti çöker, stdout'a JSON olmayan satır ya da 4 MiB'tan büyük satır yazar → AND çökmez; bozuk satır günlüğe gider, ekranda hata kartı, kapanınca "kapandı / Yeniden aç". (Görev 5 testleri)
3. Çok büyük dosya ya da fark (DeltaX) → protokol sınırı aşılmaz: fark dosya başına 2000 satır + "… N satır daha", 2 MiB'tan büyük dosya düzenleyicide açılmaz, açıklayıcı ileti gösterilir. (Görev 9–10 testleri)
4. İşlem sürerken arka arkaya tıklama (iki kez "Kaydet") → tek işlem; `m.pending` engeller. (Görev 9 testi)
5. Ağdan gelen adlarda/mesajlarda ESC dizileri, NUL, yön değiştiren karakterler → temizlenir, düz metin görünür. (Görev 2 testi)

---

### Task 1: `arayuz` protokol türleri ve kurucular

**Files:**
- Create: `arayuz/doc.go`, `arayuz/tur.go`, `arayuz/kurucu.go`
- Test: `arayuz/tur_test.go`

**Interfaces:**
- Produces (tur.go): `const Surum = 1`, `const OrtamDegiskeni = "AND_ARAYUZ_PROTOKOLU"`; bileşen türü sabitleri `TurYigin="yigin", TurSatir="satir", TurBolunmus="bolunmus", TurBolum="bolum", TurKart="kart", TurSekmeler="sekmeler", TurAyrac="ayrac", TurMetin="metin", TurBaslik="baslik", TurRozet="rozet", TurAnahtarDeger="anahtarDeger", TurBos="bos", TurKod="kod", TurFark="fark", TurIlerleme="ilerleme", TurBaglanti="baglanti", TurDugme="dugme", TurListe="liste", TurTablo="tablo", TurForm="form", TurGirdi="girdi", TurMetinAlani="metinAlani", TurSecim="secim", TurOnayKutusu="onayKutusu", TurMesajAkisi="mesajAkisi"` ve `TurBilinmeyen="bilinmeyen"` (yalnız AND koyar).
- Yapılar (JSON anahtarı = alan adının küçük harfle başlayan hali, hepsi `omitempty`; `Oge.Tur` her zaman yazılır): `Ekran{Baslik, AltBaslik, Ikon string; Yol []Kirinti; Araclar []Oge; Govde *Oge; Diyalog *Diyalog; Durum string; Mesgul bool}`, `Kirinti{ID, Metin}`, `Diyalog{ID, Baslik string; Govde *Oge; Dugmeler []Oge}`, `Onay{Baslik, Metin, Dugme}`, `Parca{Metin, Stil}`, `Rozet{Metin, Renk}`, `ListeOgesi{ID, Baslik, Alt, Ikon string; Rozetler []Rozet; Sag string; Soluk bool; Dugmeler []Oge}`, `Sutun{Baslik, Hizala}`, `TabloSatiri{ID string; Hucreler [][]Parca; Dugmeler []Oge}`, `Cift{Anahtar, Deger string; Kod, Kopyala bool}`, `Sekme{ID, Etiket, Ikon string; Sayac int}`, `Secenek{Deger, Etiket}`, `Oneri{Deger, Etiket, Alt}`, `FarkDosyasi{Yol, Durum string; Eklenen, Silinen int; Satirlar []FarkSatiri}`, `FarkSatiri{Tur, Metin string; Eski, Yeni int}`, `Mesaj{ID, Kimden, Metin, Zaman string; Giden bool; Durum string}`.
- `Oge` alanları: `Tur, ID, Metin, Baslik, Alt, Aciklama, Etiket, Ikon, Deger, YerTutucu, Tip, Varyant, Stil, Boyut, Renk, Hizala, Aralik, Kisayol, Kopyala, URL, Bos, Secili, Hata string; Duzey, Satir, EnUzun int; Oran *float64; DevreDisi, Gonder, Zorunlu, Canli, Odak, Kod, Arama, Aninda, SatirNo, Sarmala, Isaretli bool; Onay *Onay; Parcalar []Parca; Cocuklar []Oge; Sol, Sag, Dugme *Oge; Araclar []Oge; Ogeler []ListeOgesi; Sutunlar []Sutun; Satirlar []TabloSatiri; Ciftler []Cift; Sekmeler []Sekme; Secenekler []Secenek; Oneriler []Oneri; Dosyalar []FarkDosyasi; Mesajlar []Mesaj`. (`URL` → `"url"`, `ID` → `"id"`.)
- Produces (kurucu.go): `type Ayar func(*Oge)`; kurucular `Yigin(c ...Oge) Oge`, `SatirOge(c ...Oge) Oge`, `Bolunmus(sol, sag Oge) Oge`, `Bolum(baslik string, c ...Oge) Oge`, `Kart(c ...Oge) Oge`, `Ayrac() Oge`, `SekmeCubugu(id, secili string, s ...Sekme) Oge`, `Metin(s string, a ...Ayar) Oge`, `Zengin(p ...Parca) Oge`, `Baslik(duzey int, s string) Oge`, `RozetOge(metin, renk string) Oge`, `AnahtarDeger(c ...Cift) Oge`, `Bos(ikon, baslik, metin string, a ...Ayar) Oge`, `Kod(s string, a ...Ayar) Oge`, `Fark(d ...FarkDosyasi) Oge`, `Ilerleme(oran float64) Oge` (oran < 0 → `Oran` nil, belirsiz), `Baglanti(id, metin string) Oge`, `Dugme(id, metin string, a ...Ayar) Oge`, `Liste(id string, o []ListeOgesi, a ...Ayar) Oge`, `Tablo(id string, s []Sutun, r []TabloSatiri, a ...Ayar) Oge`, `Form(id string, c ...Oge) Oge`, `Girdi(id, etiket, deger string, a ...Ayar) Oge`, `MetinAlani(id, etiket, deger string, a ...Ayar) Oge`, `Secim(id, etiket, deger string, s []Secenek, a ...Ayar) Oge`, `OnayKutusu(id, etiket string, isaretli bool) Oge`, `MesajAkisi(id string, m ...Mesaj) Oge`.
- Ayarlar (değişken): `Birincil, Hayalet, Tehlikeli, Gonderir, Zorunlu, Canli, Aninda, Odakli, Aranabilir, KodGibi, SatirNumarali, Sarmalar, SifreAlani, Soluk, Kalin`; (işlev): `IleIkon(ad)`, `IleKisayol(k)`, `IleOnay(baslik, metin, dugme)`, `IleKopyala(metin)`, `IleSecili(id)`, `IleBos(metin)`, `IleYerTutucu(s)`, `IleAciklama(s)`, `IleHata(s)`, `IleEnUzun(n)`, `IleSatirSayisi(n)`, `IleOneriler(o ...Oneri)`, `IleStil(s)`, `IleBoyut(s)`, `IleAralik(s)`, `IleHizala(s)`, `IleAraclar(o ...Oge)`, `IleDugme(d Oge)`, `DevreDisiIse(b bool)`.

- [ ] **Step 1: Write the failing test** `TestEkran_JSONBicimi`: `Ekran{Baslik:"DeltaX", Yol:[]Kirinti{{ID:"ana",Metin:"DeltaX"}}, Govde: ptr(Liste("gruplar", []ListeOgesi{{ID:"g1",Baslik:"Ekip",Rozetler:[]Rozet{{Metin:"kurucu",Renk:"vurgu"}}}}, IleSecili("g1"), Aranabilir))}` JSON'u `{"baslik":"DeltaX","yol":[{"id":"ana","metin":"DeltaX"}],"govde":{"tur":"liste","id":"gruplar","secili":"g1","arama":true,"ogeler":[{"id":"g1","baslik":"Ekip","rozetler":[{"metin":"kurucu","renk":"vurgu"}]}]}}` ile anlamca eşit (map'e çözüp `reflect.DeepEqual`). `TestKurucular`: `Dugme("kaydet","Kaydet",Birincil,Gonderir,IleKisayol("ctrl+s")).Varyant=="birincil" && .Gonder && .Kisayol=="ctrl+s"`; `Ilerleme(-1).Oran==nil`; `Ilerleme(0.5)` → `*Oran==0.5`; `Metin("a",Soluk).Stil=="soluk"`; `OnayKutusu("x","X",true)` JSON'da `"isaretli":true`; boş `Oge{Tur:"ayrac"}` JSON'u tam olarak `{"tur":"ayrac"}`.
- [ ] **Step 2:** `go test ./arayuz -run 'TestEkran_JSONBicimi|TestKurucular' -v` → derleme hatası (paket yok).
- [ ] **Step 3:** `doc.go` (paket belgesi: protokol, sürüm, örnek kullanım), `tur.go`, `kurucu.go`'yu yukarıdaki imzalarla yaz.
- [ ] **Step 4:** Aynı komut → PASS; `go vet ./arayuz`.
- [ ] **Step 5:** `git add arayuz && git commit -m "arayuz: eklenti arayüzü protokol türleri ve kurucular"`

### Task 2: `arayuz.Coz` — doğrulama, sınırlar, temizlik

**Files:**
- Create: `arayuz/coz.go`, `arayuz/ikonlar.go`
- Test: `arayuz/coz_test.go`

**Interfaces:**
- Consumes: Task 1 türleri.
- Produces: `type Bildirim struct{Metin, Durum string}` (`durum`: bilgi/iyi/uyari/hata, başka değer → bilgi); `type EklentiMesaji struct{Tur string; Ekran *Ekran; Bildirim *Bildirim}` (wire: `{"tur":"ekran","ekran":{…}}`, `{"tur":"bildirim","metin":…,"durum":…}`); `func Coz(satir []byte) (EklentiMesaji, error)`; hata değerleri `ErrSinir`, `ErrBicim` (errors.Is ile ayırt edilir); `const SatirSiniri = 4 << 20`; `var Ikonlar = []string{...}` (uygulama.js `IKONLAR`'ın anahtarları: forum sohbet eklenti ayarlar bilgi guc arti gonder kopyala ara dis geri kilit kure ag profil palet dil kalkan belge tamam uyari kapat asagi imza yildiz muhur kod yanitla yenile).
- `Coz` kuralları: JSON değilse/`tur` bilinmiyorsa `ErrBicim`; satır > `SatirSiniri`, derinlik > 24, öğe > 5000, `Ogeler`/`Satirlar`/`Mesajlar` > 2000, düz metin alanı > 64 KiB, `kod`/`metinAlani`/`fark` içerikleri toplamı > 3 MiB, `id` > 512 → `ErrSinir`. Her dizge `pluginui.Safe`'ten geçer. Bilinmeyen bileşen türü → `Oge{Tur: TurBilinmeyen}` (çocukları atılır). `Ikonlar`'da olmayan ikon → "". `URL` http/https değilse "". `Varyant`/`Stil`/`Renk`/`Boyut`/`Hizala`/`Aralik`/`Tip` bilinmeyen değerde "".

- [ ] **Step 1: Write the failing tests:** `TestCoz_Ekran` (geçerli ekran gidiş-dönüş eşit); `TestCoz_Temizlik`: baslik `"a\x1b[31mb‮c\x00"` → `"abc"`, kod içeriğindeki `"\t"` ve `"\n"` korunur, `Ikon:"yok"` → `""`, `URL:"javascript:alert(1)"` → `""`, `Varyant:"kırmızı"` → `""`; `TestCoz_BilinmeyenTur`: `{"tur":"ekran","ekran":{"govde":{"tur":"grafik3d","cocuklar":[{"tur":"metin"}]}}}` → `Govde.Tur=="bilinmeyen"`, `len(Cocuklar)==0`; `TestCoz_Sinirlar`: tablo-güdümlü — 25 iç içe `yigin`, 5001 `metin`, 2001 liste öğesi, 64 KiB+1 baslik, 3 MiB+1 `kod`, 513 karakter `id`, `SatirSiniri+1` bayt → her biri `errors.Is(err, ErrSinir)`; `"{"` → `ErrBicim`; `{"tur":"neyse"}` → `ErrBicim`; `TestCoz_Bildirim`: `{"tur":"bildirim","metin":"ok","durum":"garip"}` → `Durum=="bilgi"`.
- [ ] **Step 2:** `go test ./arayuz -run TestCoz -v` → FAIL (tanımsız `Coz`).
- [ ] **Step 3:** `coz.go`'yu yaz: önce `len(satir)` denetimi, `json.Unmarshal`, sonra tek bir özyinelemeli yürüyüş (derinlik, sayaçlar, temizlik birlikte). `ikonlar.go`'ya `Ikonlar`.
- [ ] **Step 4:** `go test ./arayuz -v` → PASS.
- [ ] **Step 5:** `git commit -m "arayuz: Coz ile doğrulama, sınırlar ve temizlik"`

### Task 3: `arayuz.Calistir` — bubbletea'yı protokolle çalıştırma

**Files:**
- Create: `arayuz/calistir.go`
- Test: `arayuz/calistir_test.go`

**Interfaces:**
- Produces: `type Model interface{ tea.Model; Arayuz() Ekran }`; olay mesajları `type Olay struct{Olay, ID, Deger string; Degerler map[string]string}`, sabitler `OlayTikla="tikla", OlaySec="sec", OlayAc="ac", OlayGonder="gonder", OlayDegisti="degisti", OlaySekme="sekme", OlayDiyalogKapat="diyalogKapat", OlayGeri="geri"`; `type Merhaba struct{Surum int; Dil string}`; `type Gorunurluk struct{Gorunur bool}`; AND'ın yazdığı satır `type AndMesaji struct{Tur string; Surum int; Dil string; Olay, ID string; Deger json.RawMessage; Degerler map[string]string}` (`tur`: merhaba/gorunur/olay; `gorunur`'da `deger` bool, `olay`'da dizge); `func Etkin() bool` (`os.Getenv(OrtamDegiskeni)` ≥ 1); `func Calistir(m Model) error`; `func Bildir(metin, durum string) tea.Cmd` (→ `Bildirim` mesajı; `Calistir` onu modele iletmeden yazar); dahili `func calistir(m Model, girdi io.Reader, cikti io.Writer) error`.
- Davranış: `tea.NewProgram(sarmal, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())`; `Init` sonrası ve her `Update` sonrası `Arayuz()` JSON'u bir öncekinden farklıysa tek satır yazılır (yazma muteksle); girdi satırları `AndMesaji` olarak çözülür → `Merhaba`/`Gorunurluk`/`Olay` olarak `p.Send`; bozuk satır atlanır; girdi EOF → `p.Quit()`. `Calistir` gerçek stdout'u yakalar, `os.Stdout = os.Stderr` yapar.

- [ ] **Step 1: Write the failing tests** (sayaç modeli: `Arayuz()` → `Ekran{Baslik: strconv.Itoa(n)}`, `Olay{Olay:"tikla",ID:"arti"}` gelince `n++`, `Olay{ID:"bildir"}` gelince `Bildir("selam","iyi")` döner): `TestCalistir_IlkEkranVeOlay`: `io.Pipe` ile `calistir` başlat; ilk satır `{"tur":"ekran","ekran":{"baslik":"0"}}`; `{"tur":"olay","olay":"tikla","id":"arti"}` yazınca sıradaki satır başlığı `"1"`; aynı ekranı üreten olay (`id:"hicbir"`) yeni satır üretmez (300 ms içinde satır yok); `TestCalistir_Bildirim`: `id:"bildir"` → `{"tur":"bildirim","metin":"selam","durum":"iyi"}` satırı; `TestCalistir_MerhabaVeGorunurluk`: `{"tur":"merhaba","surum":1,"dil":"en"}` ve `{"tur":"gorunur","deger":true}` modele `Merhaba{1,"en"}` ve `Gorunurluk{true}` olarak ulaşır; `TestCalistir_GirdiKapaninca`: girdi kapatılınca `calistir` 2 sn içinde `nil` döner; `TestCalistir_BozukSatir`: `"çöp\n"` sonrası geçerli olay yine işlenir; `TestEtkin`: `t.Setenv(OrtamDegiskeni,"1")` → true, `""`/`"x"` → false.
- [ ] **Step 2:** `go test ./arayuz -run 'TestCalistir|TestEtkin' -v` → FAIL.
- [ ] **Step 3:** `calistir.go`'yu yaz.
- [ ] **Step 4:** `go test ./arayuz -count=3 -race` CI'da; burada `go test ./arayuz -count=3 -v` → PASS.
- [ ] **Step 5:** `git commit -m "arayuz: Calistir ile bubbletea modelini protokolle çalıştırma"`

### Task 4: `arayuz/arayuztest` — eklenti testleri için sürücü

**Files:**
- Create: `arayuz/arayuztest/surucu.go`
- Test: `arayuz/arayuztest/surucu_test.go`

**Interfaces:**
- Consumes: `arayuz.Model`, `arayuz.Olay`, `arayuz.Coz`.
- Produces: `type Surucu struct`; `func Yeni(m arayuz.Model) *Surucu` (Init çalıştırılmaz); `(s) Gonder(msg tea.Msg) tea.Cmd`; `(s) Olay(o arayuz.Olay) tea.Cmd`; kısayollar `(s) Tikla(id string) tea.Cmd`, `(s) TiklaSatir(id, satir string) tea.Cmd`, `(s) Sec(id, deger string) tea.Cmd`, `(s) Ac(id, deger string) tea.Cmd`, `(s) FormGonder(id string, d map[string]string) tea.Cmd`, `(s) SekmeSec(id, deger string) tea.Cmd`, `(s) DiyalogKapat() tea.Cmd`; `(s) Model() arayuz.Model`; `(s) Ekran() arayuz.Ekran`; `(s) Yurut(cmd tea.Cmd, tur int)` (komutu çalıştırır, `tea.BatchMsg`/`sequenceMsg`'i açar, her komutu 2 sn sınırla bekler, dönmeyeni bırakır, çıkan mesajları `Gonder`'ler; yeni komutlarla en çok `tur` tur); `func Bul(e arayuz.Ekran, id string) (arayuz.Oge, bool)` (govde, araclar, diyalog govdesi ve düğmeleri, liste/tablo satır düğmeleri dahil); `func Metinler(e arayuz.Ekran) []string`; `func Denetle(t testing.TB, e arayuz.Ekran)` (JSON'a çevirip `arayuz.Coz`'dan hatasız ve `TurBilinmeyen`'siz geçtiğini doğrular).

- [ ] **Step 1: Write the failing tests:** Task 3'teki sayaç modeliyle `Tikla("arti")` → `Ekran().Baslik=="1"`; `Bul` diyalogdaki `id:"evet"` düğmesini ve liste satırındaki `id:"sil"` düğmesini bulur; `Metinler` başlık, kırıntı ve liste öğesi metinlerini içerir; `Yurut` bir `tea.Batch(çabuk, asla_donmeyen)` için 2 sn'de döner ve çabuk komutun mesajını işler; `Denetle` 25 derinlikli ekranda testi başarısız sayar (alt `testing.T` ile).
- [ ] **Step 2:** `go test ./arayuz/... -v` → FAIL.
- [ ] **Step 3:** `surucu.go`'yu yaz.
- [ ] **Step 4:** `go test ./arayuz/... -v` → PASS.
- [ ] **Step 5:** `git commit -m "arayuz: arayuztest sürücüsü"`

### Task 5: AND'da arayüz oturumları ve uç noktalar

**Files:**
- Modify: `pluginapi/api.go:33-46` (`Manifest.UI`), `internal/masaustu/eklenti.go` (`icerideBaslat`, `eklentiAc`, `eklentiEtkin`, `menuOgesi`), `internal/masaustu/masaustu.go` (`Uygulama.arayuz`, `Kapat`), `internal/masaustu/api.go` (rotalar)
- Create: `internal/masaustu/arayuz.go`, `internal/masaustu/surec_windows.go`, `internal/masaustu/surec_diger.go`
- Test: `internal/masaustu/arayuz_test.go`, `internal/masaustu/masaustu_test.go` (TestMain'e sahte arayüz eklentisi kipi)

**Interfaces:**
- Consumes: `arayuz.Coz`, `arayuz.AndMesaji`, `arayuz.Surum`, `arayuz.OrtamDegiskeni`, mevcut `yayin`, `cekirdek.eklentiAnahtari`, `kendiligindenBaslayacaklar`.
- Produces: `type arayuzOturumu struct{id, ad, etiket string; ...}` ve `type arayuzler struct`; `func yeniArayuzler() *arayuzler`; `(y) baslat(ad, etiket string, cmd *exec.Cmd, geriAl func()) (*arayuzOturumu, error)` (stdin/stdout boru, stderr → sınırlı günlük yazıcı, `surecHazirla(cmd)` Windows'ta `CREATE_NO_WINDOW`, ilk satır olarak `merhaba{surum: arayuz.Surum, dil: dil.Etkin()}`); `(y) calisan(ad) *arayuzOturumu`, `(y) bul(id)`, `(y) hepsiniDurdur()`; `(o) gonder(m arayuz.AndMesaji) error`; SSE olayları `ekran {id, surum, ekran}`, `bildirim {id, metin, durum}`, `hata {id, metin}` (sınır aşımı/bozuk satır), `bitti {id, kod}`. Uç noktalar: `GET /api/arayuz/akis`, `GET /api/arayuz/ekran?id=` → `{surum, ekran, bitti, kod}`, `POST /api/arayuz/olay {id, olay, oid, deger, degerler}` (oid = öğe id'si), `POST /api/arayuz/gorunur {id, gorunur}`, `POST /api/arayuz/kapat {id}`, `GET /api/eklenti/oturumlar` → `{oturumlar:[{id, ad, tur:"arayuz"|"terminal", bitti, kod}]}`. `icerideBaslat` imzası `(d *cekirdek, pl pluginmgr.Plugin, sutun, satir int) (oturumOzeti, error)`, `type oturumOzeti struct{ID, Tur string}`; `1 ≤ pl.Manifest.UI ≤ arayuz.Surum` → arayüz oturumu (env `AND_ARAYUZ_PROTOKOLU=1`, `AND_GOMULU=1`), değilse terminal. `eklentiAc` yanıtına `tur` eklenir. `menuOgesi`'ne `Arayuz bool \`json:"arayuz"\``. Ayarlar'dan kapatma ve `Kapat` iki türü de durdurur.

- [ ] **Step 1: Write the failing tests** (TestMain: `AND_SAHTE_ARAYUZ=yanki` → `arayuz.Calistir` ile küçük model: başlık = son olayın `id`'si; `=cop` → önce `"çöp\n"`, sonra 4 MiB+1 baytlık satır, sonra geçerli ekran; `=coker` → ilk ekrandan sonra `os.Exit(7)`): `TestArayuz_EkranVeOlay` (başlat → `/api/arayuz/ekran` başlığı ilk ekran; `POST /api/arayuz/olay {oid:"merhaba"}` → 5 sn içinde başlık `"merhaba"`; `/api/eklenti/oturumlar`'da `tur:"arayuz"`); `TestArayuz_BozukSatirlar` (`cop` kipinde süreç yaşar, son geçerli ekran gelir, `hata` olayı yayınlanır); `TestArayuz_Coker` (`bitti` ve `kod:7`, API anahtarı geri alınır); `TestArayuz_HizSiniri` (sahte eklenti 100 ekranı arka arkaya yollar → aboneye gelen `ekran` olayı ≤ 10, sonuncusu en son ekran); `TestArayuz_IcerideBaslatSecimi` (`UI:1` → `tur:"arayuz"`, `UI:0` → `"terminal"`, `UI:2` → `"terminal"`); `TestArayuz_KapatmaVeAyarlar` (`/api/arayuz/kapat` ve `/api/eklenti/etkin {etkin:false}` → `bitti`); `TestArayuz_KendiligindenBaslar` (onaylı `UI:1` eklenti `eklentileriBaslat` ile arayüz oturumu olarak başlar); Windows'ta `TestArayuz_KonsolYok` (sahte eklenti `GetConsoleWindow()`'u ekrana yazar → `"0"`).
- [ ] **Step 2:** `go test ./internal/masaustu -run TestArayuz -v` → FAIL.
- [ ] **Step 3:** `Manifest.UI` alanı; `arayuz.go` (okuma döngüsü `bufio.Reader` ile satır satır, 4 MiB'ı aşan satırı sonuna kadar atar; ekran iletimi `time.AfterFunc` ile 50 ms birleştirme), süreç dosyaları, `eklenti.go`/`masaustu.go`/`api.go` değişiklikleri.
- [ ] **Step 4:** `go test ./internal/masaustu -v` ve `go test ./pluginapi/... ./internal/pluginmgr/...` → PASS; `GOOS=linux go vet ./internal/masaustu && GOOS=darwin go vet ./internal/masaustu`.
- [ ] **Step 5:** `git commit -m "Masaüstü: eklenti arayüzü oturumları ve uç noktaları"`

### Task 6: Çizici, stiller, metinler ve vitrin önizlemesi

**Files:**
- Modify: `internal/masaustu/web/uygulama.js` (yeni bölüm "Eklenti arayüzü"; `eklentiCiz`, `calisiyor`, `terminalleriYukle` → oturum listesi), `internal/masaustu/web/uygulama.css` (bölüm "Eklenti arayüzü"), `dil/ceviriler/tui.json`, `internal/masaustu/metinler.go`, `internal/masaustu/onizleme_test.go`
- Create: `internal/masaustu/vitrin_test.go` (bütün bileşenleri gösteren `arayuz.Model`; TestMain'de `AND_SAHTE_ARAYUZ=vitrin`)
- Test: `internal/masaustu/masaustu_test.go` (`TestArayuzIkonlari`)

**Interfaces:**
- Consumes: Task 5 uç noktaları ve SSE olayları; `menuOgesi.Arayuz`.
- Produces (JS): `A` (eklenti adı → `{oturum, ekran, surum, onceki, bitti, kod}`), `arayuzAkisiniAc()`, `arayuzCiz(s)`, `ogeCiz(o, yol, s)`, `arayuzOlayi(s, olay, oid, deger, degerler)`, `durumuYakala(kok)` / `durumuGeriYukle(kok, durum)`. Arayüz kipindeki eklentide "Ayrı pencerede aç" düğmesi yok.
- Yeni AND metinleri (t() ile; 5 dile çevrilir): `"Bu öğe bu AND sürümünde gösterilemiyor."`, `"Eklentinin ekranı gösterilemedi."`, `"Süz…"`, `"Kopyalandı"`, `"Emin misin?"`, `"Vazgeç"` (katalogda varsa yeniden kullan), `"%d satır daha"`, `"Ekran bekleniyor…"`.
- Değer koruma kuralı: bir girdinin DOM değeri yalnızca yeni ekrandaki `deger`, önceki ekrandakinden (`s.onceki`) farklıysa yazılır; odak, `selectionStart/End` ve `[data-kaydir]` kaplarının `scrollTop`'u `id` (yoksa ağaç yolu) anahtarıyla korunur.
- Klavye: `kisayol` ayrıştırıcısı; diyalogda Esc → `diyalogKapat`, değilse Esc/Alt+← → `geri`; listede ↑/↓ → `sec`, Enter → `ac`; `canli` girdide 250 ms → `degisti`.
- Olaylar: eklentinin ekranı gösterilince `POST /api/arayuz/gorunur {gorunur:true}`, başka görünüme geçilince `false`; SSE `bildirim` → mevcut `bildir(metin, durum)`; `hata` → gövdenin üstünde hata kartı ("Eklentinin ekranı gösterilemedi." + ayrıntı); `bitti` → bugünkü "kapandı / Yeniden aç" durumu (`eklentiDurumuCiz`). `dugme.onay` → mevcut `onayla()`; `dugme.kopyala` → `navigator.clipboard.writeText` + "Kopyalandı"; `baglanti.url` → onaylı `/api/disari`; kırıntı → `tikla`.

- [ ] **Step 1: Write the failing test** `TestArayuzIkonlari`: `arayuz.Ikonlar`'daki her ad `uygulama.js`'teki `IKONLAR` nesnesinde anahtar olarak var (regex `^\s{4}(\w+):` ile `IKONLAR = {` bloğundan); ve `TestArayuzMetinleri` yeni metinlerle kırmızıya döner (metinler.go henüz eski).
- [ ] **Step 2:** `go test ./internal/masaustu -run 'TestArayuzIkonlari|TestArayuzMetinleri' -v` → FAIL.
- [ ] **Step 3:** Çiziciyi ve stilleri yaz: bileşenler AND'ın mevcut sınıflarını kullanır (`ust-cubuk`, `sekmeler`/`sekme`, `bos-durum` + `bosHarita`, düğme varyantları, liste seçim çubuğu); `bolunmus` dar pencerede (≤ 760 px) alt alta. `metinler.go`'yu çıkarıcıyla yeniden üret; `tui.json`'a çeviriler; `DIL_ISKELET=... go test ./dil -run Kapsam` ile eksik kalmadığını doğrula. Önizlemeye `ONIZLEME_VITRIN=1` (test ikilisini `AND_SAHTE_ARAYUZ=vitrin` ile arayüz eklentisi "Vitrin" olarak başlatır) ve `ONIZLEME_ARAYUZ=<exe>` (`pluginapitest.Yeni().Ornek().Baslat(veriDizini, "dm","file")` ortamıyla gerçek eklentiyi `UI:1` olarak başlatır) kiplerini ekle. Vitrin modeli her 1 sn'de bir sayaç değiştirip yeni ekran gönderir (Review Focus 1).
- [ ] **Step 4:** `go test ./internal/masaustu ./dil/... -v` → PASS; `node --check internal/masaustu/web/uygulama.js`. Gözle: önizleme + headless Edge CDP (`ONIZLEME_SURE=120s ONIZLEME_VITRIN=1 go test -tags onizleme -run TestOnizleme ./internal/masaustu`): Vitrin'in tam sayfa ve 700 px genişlik ekran görüntüleri; bir girdiye "deneme" yaz, 3 sn bekle (3 yeni ekran gelir) → değer hâlâ "deneme" ve odak girdide; diyalog Esc ile kapanır; satır düğmesi doğru `deger`'i yollar (vitrin son olayı ekrana yazar).
- [ ] **Step 5:** `git commit -m "Masaüstü: eklenti arayüzü çizicisi, stiller ve vitrin önizlemesi"`

### Task 7: Belgeler

**Files:**
- Modify: `EKLENTILER.md`, `PLUGINS.md` (yeni "Arayüz" / "Interface" bölümü + çalışma ortamı tablosuna `AND_ARAYUZ_PROTOKOLU`), `pluginapi/doc.go` (paket listesine `arayuz`, `arayuz/arayuztest`)

- [ ] **Step 1:** Bölümü yaz: `"ui": 1`, `arayuz.Etkin`/`Calistir`/`Model`, olaylar, bileşen tablosu (spec'teki katalog), sınırlar, `arayuztest` örneği, terminal kipiyle aynı modelin paylaşılması, kısa örnek (sayaç). İki dil aynı içerik.
- [ ] **Step 2:** `go vet ./pluginapi` → temiz; Markdown'daki kod örneği `arayuz` imzalarıyla uyuşur (gözle).
- [ ] **Step 3:** `git commit -m "Belgeler: eklenti arayüzü"`

### Task 8: DeltaX — arayüz kipi, Giriş ve Grup görünümleri

**Files:**
- Modify: `Eklentiler/DeltaX/main.go` (`manifest.UI = 1`; `if arayuz.Etkin() { err = arayuz.Calistir(m) }`), `Eklentiler/DeltaX/plugin.json` (`"ui": 1`, `"version": "1.8.0"`), `Eklentiler/DeltaX/model.go` (alanlar `arayuzKipi bool`, `sonSekme ekran`, `diffSatirlar []DiffSatiri`), `Eklentiler/DeltaX/isleyiciler.go` (`arayuz.Merhaba` → `m.arayuzKipi=true; m.gruplar=gruplariListele(...)`; `diffGeldi` ham satırları saklar; `Update` sonunda arayüz kipinde `m.ekran==ekProjePaneli && !m.pending` ise `m.sekmeyiAc(m.sonSekme)`)
- Create: `Eklentiler/DeltaX/arayuz.go` (`func (m model) Arayuz() arayuz.Ekran`, `girisGorunumu`, `grupGorunumu`, `arkaPlan()`), `Eklentiler/DeltaX/olaylar.go` (`func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd)`, `func (m model) tus(k string) (tea.Model, tea.Cmd)` → `m.handleKey(tea.KeyMsg{...})`)
- Test: `Eklentiler/DeltaX/arayuz_test.go`

**Interfaces:**
- Consumes: Task 1–4 (`arayuz.*`, `arayuztest.*`), mevcut `handleKey`, `gruplarGorunur`, `davetOnerileri`, `istemci` sahtesi (`model_test.go`'daki).
- Produces: öğe id'leri (sonraki görevler ve testler kullanır): kırıntılar `yolAna`, `yolGrup`; Giriş: `gruplar` (liste, `sec`/`ac` → grup), `yeniGrup` (düğme), `grupFormu` (`ad`), `davetler` (satır düğmeleri `davetKabul`, `davetRed`, `deger` = `GrupID`), `kimlikKopyala`, `sonKayitlar` (`sec`, `deger` = `grupID/projeID/hash` → o grubu ve projeyi açar, Geçmiş sekmesinde o kaydı seçer); Grup: `projeler`, `yeniProje`, `projeFormu` (`ad`), `uyeler`, `uyeDavet`, `davetFormu` (`peer`, öneriler `davetOnerileri`), `yoneticiAta`, `yoneticiler` (diyalog listesi), `gruptanCik`; ortak diyalog düğmeleri `onayEvet`, `onayHayir`. `arkaPlan()`: diyalog ekranının altında çizilecek görünüm (ekGrupOlustur → Giriş; ekProjeOlustur/ekUyeDavet/ekYoneticiSecim → Grup; ekOnay → `onayGeriEkran`; ekDosyaOnay → `onayOnceki`). Olay → tuş eşlemesi örnekleri: `gruplar` `ac` → `m.grupIdx = gruplarGorunur() içindeki konum; tus("enter")`; `grupFormu` `gonder` → `m.ekran=ekGrupOlustur; m.grupAdIn.SetValue(ad); tus("enter")`; `diyalogKapat`/`geri` → bulunduğu ekranda `tus("esc")`, ama `ekAnaMenu`/`ekGrupListesi`/`ekKimlik`/`ekDavetler`'de hiçbir şey; `m.notice` ekranın gövdesinin üstünde `isErr`'e göre hata/iyi kartı olarak; `m.pending` → `Ekran.Mesgul`.

- [ ] **Step 1: Write the failing tests** (`arayuztest.Yeni(newModel(sahte, kimlik, t.TempDir()))` + `Gonder(arayuz.Merhaba{1,"tr"})`): `TestArayuz_GrupOlustur` (`Tikla("yeniGrup")` → diyalog `grupFormu` var; `FormGonder("grupFormu", {"ad":""})` → ekranda "Grup adı boş olamaz."; `{"ad":"Ekip"}` + `Yurut` → `gruplar` listesinde "Ekip"); `TestArayuz_GrubaGir` (`Ac("gruplar", id)` → `Ekran().Yol` = DeltaX › Ekip, `projeler` listesi var); `TestArayuz_ProjeOlustur`; `TestArayuz_DavetGecersiz` (`FormGonder("davetFormu", {"peer":"abc"})` → "Bu bir Peer ID değil" iletisi); `TestArayuz_GeriCikisYapmaz` (Giriş'te `Olay{Olay:"geri"}` → dönen komut `tea.Quit` değil, model hâlâ çalışıyor); `TestArayuz_GecerliEkran` (her görünümde `arayuztest.Denetle`).
- [ ] **Step 2:** `go test ./Eklentiler/DeltaX -run TestArayuz -v` → FAIL.
- [ ] **Step 3:** Yukarıdaki dosyaları yaz.
- [ ] **Step 4:** `go test ./Eklentiler/DeltaX -v` → PASS (eski TUI testleri dahil).
- [ ] **Step 5:** `git -C Eklentiler/DeltaX add -A && git -C Eklentiler/DeltaX commit -m "Arayüz kipi: Giriş ve Grup görünümleri"`

### Task 9: DeltaX — Proje: Değişiklikler, Geçmiş, Dallar

**Files:**
- Modify: `Eklentiler/DeltaX/arayuz.go` (`projeGorunumu`, `degisikliklerSekmesi`, `gecmisSekmesi`, `dallarSekmesi`, `farkOgesi(satirlar []DiffSatiri, yol string) arayuz.Oge`), `Eklentiler/DeltaX/olaylar.go`
- Test: `Eklentiler/DeltaX/arayuz_test.go`

**Interfaces:**
- Consumes: Task 8 (`olayIsle`, `tus`, `sonSekme`, `diffSatirlar`).
- Produces: id'ler: `projeSekmeleri` (sekmeler `degisiklikler`→"d", `gecmis`→"l", `dallar`→"b", `uzak`→"m", `dosyalar`→"f"; `m.sekmeyiAc(e ekran)` bu tuşları `projeIsleminiCalistir` ile çalıştırır ve `sonSekme`'yi yazar), araç çubuğu `dal` (secim, `aninda`; değişince hedef dal `m.dallar`'a hazırlanır, `m.dalIdx` hedefe, `onayDalDegistir` onayı açılır), `gonder`, `cek`, `disaAktar`, `klasoruAc`; Değişiklikler: `degisiklikler` (A/M/D rozetli; `sec` → fark: `durumIdx` + `tus("enter")`; satır düğmesi `geriAl` → `tus("g")`), `commitFormu` (`mesaj`; `ekCommitMesaj` + `commitMsgIn` + `tus("enter")`), `yenile`; Geçmiş: `gecmisArama` (`canli`; `gunlukAramaIn` değeri), `kayitlar` (`sec` → `tus("enter")`), `kayitDosyalari` (`sec` → kayıt farkı), `buradanDal`; Dallar: `dallarListesi` (satır düğmeleri `dalaGec`, `dalSil` — ikisi de DeltaX'in onay diyaloğuyla), `yeniDal`, `dalFormu` (`ad`). Fark görünümü: dosya başına en çok 2000 satır, fazlası tek `@` satırı `dil.F("… %d satır daha", n)`.

- [ ] **Step 1: Write the failing tests** (gerçek geçici proje: grup + proje kur, çalışma klasörüne `a.txt` yaz): `TestArayuz_DegisikliklerVeKayit` (`SekmeSec("projeSekmeleri","degisiklikler")` + `Yurut` → `degisiklikler`'de `a.txt` "A" rozetli; `Sec` → `fark` öğesi `+` satırlı; `FormGonder("commitFormu",{"mesaj":"ilk"})` + `Yurut` → Geçmiş'te "ilk"); `TestArayuz_CiftKaydet` (iki `FormGonder("commitFormu",…)` arka arkaya, arada `Yurut` yok → ikinci komut nil; Review Focus 4); `TestArayuz_BuyukFark` (2500 satırlık dosya → fark öğesinde 2000 `+` satır ve "500 satır daha"; Review Focus 3); `TestArayuz_DalOlusturVeGec` (`dalFormu` `{"ad":"deneme"}` → listede; `TiklaSatir("dalaGec","deneme")` → diyalog `onayEvet`; `Tikla("onayEvet")` + `Yurut` → araç çubuğu `dal` değeri "deneme"); `TestArayuz_DalAdiGecersiz` (`{"ad":"a b"}` → "Dal adında boşluk ya da eğik çizgi olamaz.").
- [ ] **Step 2:** `go test ./Eklentiler/DeltaX -run TestArayuz -v` → FAIL.
- [ ] **Step 3:** Görünümleri ve olayları yaz.
- [ ] **Step 4:** `go test ./Eklentiler/DeltaX -v` → PASS.
- [ ] **Step 5:** `git -C Eklentiler/DeltaX commit -am "Arayüz kipi: Proje — Değişiklikler, Geçmiş, Dallar"`

### Task 10: DeltaX — Uzak, Dosyalar, diyaloglar ve çeviriler

**Files:**
- Modify: `Eklentiler/DeltaX/arayuz.go` (`uzakSekmesi`, `dosyalarSekmesi`, `diyalogu() *arayuz.Diyalog` — ekGonderSecim, ekPullIste, ekDisaAktar, ekOnay (metin `viewOnay`'dan ayrılan `onayMetni() (baslik, metin string)` ile), ekDosyaOnay, ekYeniDal, ekYeniDosya, ekYoneticiSecim, ekGrupOlustur, ekProjeOlustur, ekUyeDavet), `Eklentiler/DeltaX/view.go` (`onayMetni` ayrımı; `viewOnay` onu kullanır, çıktısı değişmez), `Eklentiler/DeltaX/olaylar.go`, `Eklentiler/DeltaX/ceviri.json`
- Test: `Eklentiler/DeltaX/arayuz_test.go`, `Eklentiler/DeltaX/gorunum_dil_test.go`

**Interfaces:**
- Produces: id'ler: Uzak: `uzaklar` (satır düğmesi `birlestir` → `uzakIdx` + `tus("enter")` → onay), `herkestenCek`; diyaloglar: `gonderHedefleri` (`ac` → `pushIdx` + `tus("enter")`), `herkeseGonder` (`tus("a")`), `cekHedefleri`, `herkestenCekDiyalog`, `disaAktarFormu` (`hedef`, varsayılan değer `filepath.Join(kullanıcı ev dizini, "Downloads")`), `dosyaOnayEvet`/`dosyaOnayHayir` (`tus("y")`/`tus("n")`); Dosyalar: `dosyalar` (`ac` → `tus("enter")`), satır düğmesi `dosyaSil` (AND'ın `onay`'ıyla; olayda `dosyaSilOnayi=yol` + `tus("d")`), `yeniDosya`, `yeniDosyaFormu` (`yol`), `duzenleyici` (form; `icerik` metinAlani `kod`, `satirNo`; Kaydet `ctrl+s` → `dosyaDuzenleTA.SetValue(icerik)` + `tus("ctrl+s")`), `duzenleyiciKapat`. 2 MiB'tan büyük dosya düzenleyicide açılmaz: `dosyaAcildi` arayüz kipinde `len(icerik) > 2<<20` ise `m.notice = dil.T("Bu dosya düzenleyicide açılamayacak kadar büyük; klasörü açıp başka bir düzenleyiciyle düzenle.")`.

- [ ] **Step 1: Write the failing tests:** `TestArayuz_DosyaDuzenle` (`yeniDosyaFormu` `{"yol":"b.txt"}` → `duzenleyici` açık; `FormGonder("duzenleyici",{"icerik":"selam"})` + `Yurut` → diskte `b.txt` = "selam"); `TestArayuz_BuyukDosya` (3 MiB dosya `Ac("dosyalar","c.txt")` + `Yurut` → `duzenleyici` yok, ileti var; Review Focus 3); `TestArayuz_DosyaOnayi` (sahte istemci onay isteği → her görünümde diyalog `dosyaOnayEvet`; `Tikla` → sahte istemcide kabul kaydı); `TestArayuz_OnayMetniAyni` (`viewOnay()` çıktısı değişiklik öncesiyle aynı — altın dize); `TestArayuz_HerDilde` (`diltest.HerDilde`: 6 dilde her görünüm ve diyalog için `arayuztest.Metinler` → `diltest.YeniDenetci(t, ozelAdlar, catalog)` kalıntısız; `arayuztest.Denetle`).
- [ ] **Step 2:** `go test ./Eklentiler/DeltaX -run 'TestArayuz' -v` → FAIL.
- [ ] **Step 3:** Diyalogları, sekmeleri, olayları yaz; yeni metinleri `ceviri.json`'a 5 dilde ekle (`diltest.Kapsam` yeşil).
- [ ] **Step 4:** `go test ./Eklentiler/DeltaX -v` → PASS.
- [ ] **Step 5:** `git -C Eklentiler/DeltaX commit -am "Arayüz kipi: Uzak, Dosyalar, diyaloglar ve çeviriler (1.8.0)"`

### Task 11: Uçtan uca doğrulama ve teslim

**Files:**
- Modify (yalnız bulgu olursa): ilgili dosyalar; hafıza notları (`and-masaustu-arayuzu.md`, `and-session-log.md`)

- [ ] **Step 1:** `(Get-PSDrive C).Free` ≥ 5 GB; `gofmt -l` boş; `go vet ./...` ve `GOOS=linux`/`GOOS=darwin go vet ./arayuz/... ./internal/masaustu ./Eklentiler/DeltaX`; `go test -count=1 ./...` → çıkış 0.
- [ ] **Step 2:** LF ve CRLF çekim benzetimi: `git clone --no-local` (LF: `-c core.autocrlf=false -c core.eol=lf`; CRLF: `-c core.autocrlf=true`) scratchpad'e, `go test ./...` ikisinde de yeşil (DeltaX ayrı depo olduğundan klonda yoktur; beklenen).
- [ ] **Step 3:** Kullanıcıdan exe onayı (AskUserQuestion). Onaylanırsa: eski `and.exe` ve `Eklentiler/DeltaX/and-plugin-DeltaX.exe`'yi scratchpad `exe_yedek_2`'ye kopyala; `CGO_ENABLED=0 go build -o and.exe ./cmd/and` ve `go build -o Eklentiler/DeltaX/and-plugin-DeltaX.exe ./Eklentiler/DeltaX`.
- [ ] **Step 4:** Gözle: `ONIZLEME_ARAYUZ=<DeltaX exe>` önizlemesi + CDP: Giriş, Yeni grup diyaloğu, Grup, Proje/Değişiklikler (fark açık), Geçmiş (kayıt ayrıntısı), Dallar, Uzak, Dosyalar (düzenleyici), dosya onayı diyaloğu; geniş (1280×800) ve dar (720×800); Klasik ve bir açık renkli olmayan başka tema. Her görüntüye bak; taşma, kesilme, okunmayan kontrast yok. Gerçek pencerede kısa deneme (geçici APPDATA): DeltaX'e tıklayınca terminal değil arayüz açılıyor, konsol penceresi yok.
- [ ] **Step 5:** Kullanıcıya rapor; hafıza notlarını güncelle. (Push yok.)
