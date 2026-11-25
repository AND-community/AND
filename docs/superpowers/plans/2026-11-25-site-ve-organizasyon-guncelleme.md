# Web sitesi, organizasyon ve README güncellemesi — uygulama planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** AND'ın README'sini, web sitesini (giriş bölümünde masaüstü penceresinin canlı kopyasıyla) ve GitHub organizasyonunu AND'ın bugünkü hâline getirip yayımlamak.

**Architecture:** Site statik ve üretilir: `tools/olustur.js` içindeki R (kopya) ve M (sayfa) tablolarından `index.html`, `tr/index.html`, `404.html`, `sitemap.xml`, `tools/og.html` çıkar; davranış `assets/js/site.js`'te, görünüm `assets/css/site.css`'te. Terminal ana menüsü kopyası (`kopya()`, `.terminal`/`.t-*`) yerini masaüstü penceresi kopyasına (`pencere()`, `.pencere`/`.p-*`) bırakır. Yeni `tools/denetle.js` üretilen çıktıyı bağımlılıksız denetler; etkileşim headless Edge + CDP ile oturumun scratchpad'inde sınanır.

**Tech Stack:** Node 24 (bağımlılıksız betikler), düz HTML/CSS/JS, GitHub Pages; GitHub REST API (saklı GCM kimliğiyle); git.

**Spec:** `docs/superpowers/specs/2026-11-25-site-ve-organizasyon-guncelleme-design.md` (AND deposu, 14b75f5).

**Depolar:** AND = `AND Proje/AND/`; site = `AND Proje/AND web sitesi/`; organizasyon profili = `AND-community/.github` (yerel kopyası yok, geçici klon); konu_ac = `AND Proje/AND/Eklentiler/konu_ac/` (ayrı depo).

## Global Constraints

- Varsayılan tema Klasik mavi-mor kalır (`data-tema="klasik"`, TEMALAR[0]).
- Site dışarıdan yalnızca GitHub API'sine gider; çerez, analiz, yeni web yazısı yok. Kopya sistem yazılarını kullanır: başlık `"Segoe UI Variable Display", "Segoe UI", system-ui, -apple-system, "Noto Sans", Roboto, sans-serif`; metin `"Segoe UI Variable Text", …` (aynı yedekler); kod `"Cascadia Mono", "Cascadia Code", Consolas, "JetBrains Mono", ui-monospace, monospace` (uygulama.css `--baslik/--metin/--kod`).
- İngilizce ve Türkçe sayfa aynı şablondan çıkar; yapıları (id'ler, öğe sayıları) birebir aynı.
- Üretilen dosyalar elle düzenlenmez; değişiklik `tools/olustur.js`'te yapılıp `node tools/olustur.js` çalıştırılır.
- Sitedeki ve README'lerdeki her iddia AND'ın koduna karşı doğrulanır (kod okunarak; tahmin yok).
- 390 px genişlikte yatay kaydırma yok; konsolda hata yok.
- Push yalnızca `git push origin main`, ayrı bir `cd` komutundan sonra kendi başına; öncesinde `pwd` ile doğru depoda olunduğu görülür. Force push yok. Tag yok.
- Arşivlenmiş depo salt okunurdur: konu_ac'nin açıklaması arşivlemeden önce değişir.
- Kimlik bilgisi (token) yalnızca ortam değişkeninde/stdin'de; komut satırına ve çıktıya yazılmaz.
- Commit mesajları Türkçe, sonunda `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. Tarayıcısında eski ya da bilinmeyen bir tema kimliği kayıtlı ziyaretçi: yeni 4 tema (gul, okyanus, gun-batimi, lavanta) kabul edilmeli, bilinmeyen kimlik Klasik'e düşmeli → Task 2'deki denetim (satır içi regex + site.js listesi) ve Task 5'teki CDP denetimi (`localStorage['and-tema']='lavanta'` ile açılış).
2. Yalnızca klavyeyle gezen ziyaretçi: Tab kenar çubuğuna girer, Enter/Space ekranı açar, odak halkası görünür, seçili öğe `aria-current="page"` taşır → Task 4 CDP adımı.
3. 390 px telefon: yatay kaydırma yok; forumda önce liste, konuya dokununca okuma ve geri düğmesi; uzun Türkçe kategori adları ("Etkinlikler ve Duyurular") taşmaz → Task 4 ve Task 5 CDP adımları.
4. Hareket azaltma tercihi (`prefers-reduced-motion: reduce`): eş sayısı animasyonu oynamaz, durum çubuğu son hâliyle gelir → Task 4 CDP adımı.
5. JavaScript'i kapalı ya da betiği yüklenmeyen ziyaretçi: kopya anlamlı bir Forum ekranı gösterir (Forum paneli HTML'de görünür, ötekiler `hidden`) → Task 3 denetimi.

---

### Task 1: AND README'leri (EN + TR) bugünkü hâline

**Files:**
- Modify: `AND/README.md`, `AND/README.tr.md`

**Interfaces:**
- Produces: değişmeyen çapalar — site ve profil bunlara bağlanır: `README.md#building-from-source`, `README.md#installation`, `README.tr.md#kaynaktan-derleme`, `README.md#desktop-interface`, `README.md#command-line` (varsa TR karşılıkları). Bu task başlık metinlerini değiştirmez.

- [ ] **Step 1: Eskimiş yerleri listele (denetim önce kırmızı)**

Run (AND kökünde):
```bash
grep -nE "terminal forum|terminal-based forum|Plugins\*\* are terminal programs|own terminal window|terminal forum topluluğu|terminal tabanlı|kendi terminal penceresinde" README.md README.tr.md
```
Expected: en az satır 7, 34 ve 415 civarı (EN) ile TR karşılıkları listelenir. Ayrıca iki dosyada "Desktop interface"/"Masaüstü arayüzü" bölümünü baştan sona oku; "Screens" maddesinde forumun çekirdek ekranı gibi anlatılıp anlatılmadığını not et.

- [ ] **Step 2: Düzelt**

Gerçekler (koddan doğrulandı; değiştirirken yeniden bak): varsayılan arayüz masaüstü penceresi (`--terminal`/`--tui` ya da `AND_ARAYUZ=terminal` ile terminal); Windows'ta WebView2 penceresi, Linux/macOS'ta ve WebView2 yokken varsayılan tarayıcı; masaüstünde eklentiler AND'ın penceresinde çalışır (manifest `"ui"` alanı olanlar AND'ın bileşenleriyle çizilir — `arayuz` paketi; olmayanlar pencere içindeki terminalde, ConPTY gerektirir; yoksa ayrı konsol penceresi); forum `Eklentiler/forum` eklentisidir; eklentiler kategorilere ayrılır. Tanım satırları (7, 34): "serverless, peer-to-peer forum community with a desktop app and a terminal interface" anlamında. TR dosyada aynı düzeltmeler. Başlıkları yeniden adlandırma.

- [ ] **Step 3: Denetimi yeniden çalıştır**

Run: Step 1'deki grep. Expected: çıktı yok. Ayrıca:
```bash
grep -nE "^#{2,3} (Building from source|Installation|Desktop interface|Command line)" README.md
grep -nE "^#{2,3} (Kaynaktan derleme|Kurulum)" README.tr.md
```
Expected: her başlık bulunur (çapalar sağlam).

- [ ] **Step 4: Commit**

```bash
git add README.md README.tr.md
git commit -m "README: masaüstü arayüzü, pencere içi eklentiler ve forum eklentisi bugünkü hâliyle"
```

---

### Task 2: Denetim betiği ve 10 tema

**Files:**
- Create: `AND web sitesi/tools/denetle.js`, `AND web sitesi/tools/tema-tonlari.js`
- Modify: `AND web sitesi/tools/olustur.js` (TEMALAR, `bas()` ve `bulunamadi()` içindeki satır içi tema regex'i), `assets/css/site.css` (4 yeni `:root[data-tema=…]` bloğu), `assets/js/site.js` (`TEMALAR`, `ZEMIN`, `M.en/tr.temaAdi`)

**Interfaces:**
- Produces: `TEMA_KIMLIKLERI = ['klasik','turkuaz','gece-moru','kehribar','orman','gul','okyanus','gun-batimi','lavanta','mono']` (ui.Presets sırası). `node tools/denetle.js` → her denetim için `✓ ad` ya da `✗ ad: neden`; herhangi biri başarısızsa çıkış kodu 1. Sonraki task'lar bu betiğe `denetim(ad, fn)` ile yeni denetimler ekler.
- TEMALAR yeni girdileri (`renk` = [vurgu, seçim, isim, rozet]; ANSI 256 → hex):
  - `gul`: tr 'Gül', en 'Rose', `['#ff87af', '#af005f', '#ffd7af', '#87d7ff']`
  - `okyanus`: tr 'Okyanus', en 'Ocean', `['#00afff', '#005f87', '#87ffd7', '#ffaf5f']`
  - `gun-batimi`: tr 'Gün Batımı', en 'Sunset', `['#ff875f', '#af0000', '#ffd787', '#af87ff']`
  - `lavanta`: tr 'Lavanta', en 'Lavender', `['#d7afff', '#5f5faf', '#afd7d7', '#ffafd7']`
  - Yeni temalarda `--sel-fg: #eeeeee`, `--muted: #8a8a8a`, `--text: #ebe9f4` (varsayılanlar).

- [ ] **Step 1: `tools/denetle.js`'i ilk denetimlerle yaz**

Bağımlılıksız (fs, path, assert yok — kendi `denetim(ad, fn)` sarmalayıcısı; fn hata fırlatırsa ✗). İlk denetimler:
- `temalar-css`: site.css'te `:root` (klasik) dışındaki 9 kimliğin her biri için `:root[data-tema="<id>"]` bloğu var.
- `temalar-js`: site.js'te `TEMALAR` dizisi tam olarak TEMA_KIMLIKLERI; `ZEMIN` ve iki dilin `temaAdi` nesnesi bu 10 anahtarı taşır.
- `temalar-regex`: index.html, tr/index.html ve 404.html'deki satır içi `localStorage.getItem("and-tema")` betiğinin regex'i 10 kimliğin hepsini kabul eder ve `pembe` gibi bilinmeyen birini reddeder (regex'i metinden çıkar, `new RegExp` ile sına).
- `temalar-dugme`: iki sayfada `data-tema-sec="<id>"` her kimlik için en az bir kez geçer.
- `temalar-kontrast`: her tema için (eksik değişken `:root`'tan gelir) WCAG oranları: `--text`/`--ink` ≥ 15, `--soft`/`--ink` ≥ 7.5, `--accent-ink`/`--ink` ≥ 7, `--muted`/`--ink` ≥ 4.5.

- [ ] **Step 2: Kırmızı olduğunu gör**

Run: `node tools/denetle.js` (site kökünde). Expected: `temalar-*` denetimleri ✗ (4 tema eksik), çıkış kodu 1.

- [ ] **Step 3: `tools/tema-tonlari.js` — türetilmiş tonları hesapla**

`node tools/tema-tonlari.js` dört yeni tema için `--ink`, `--ink-deep`, `--accent-ink`, `--soft`, `--line` değerlerini basar. Yöntem: site.css başındaki not bu tonların OKLab'da karıştırılarak hesaplandığını söylüyor; kural kodda yok. Mevcut 5 renkli temaya (Mono hariç) uyan bir OKLab karışım kuralı (sabit koyu taban + temanın vurgu/seçim renginden oran) bul; kural bu 5 temanın mevcut değerlerini kanal başına ±3 içinde yeniden üretmeli (betik bunu da yazdırır). Mevcut temaların değerleri DEĞİŞMEZ.

- [ ] **Step 4: Temaları ekle**

olustur.js TEMALAR'a 4 girdi (ui.Presets sırasıyla, Mono en sonda), site.css'e 4 blok (Step 3 çıktısı + Interfaces'teki vurgu/seçim/isim/rozet), site.js `TEMALAR`/`ZEMIN` (`ZEMIN[id]` = o temanın `--ink`'i)/`temaAdi`, `bas()` ve `bulunamadi()`'daki regex. Sonra `node tools/olustur.js`.

- [ ] **Step 5: Yeşil olduğunu gör**

Run: `node tools/olustur.js && node tools/denetle.js`. Expected: bütün denetimler ✓, çıkış 0.

- [ ] **Step 6: Commit**

```bash
git add tools/denetle.js tools/tema-tonlari.js tools/olustur.js assets/css/site.css assets/js/site.js index.html tr/index.html 404.html
git commit -m "Temalar: AND'ın 10 hazır teması (Gül, Okyanus, Gün Batımı, Lavanta eklendi) ve denetim betiği"
```

---

### Task 3: Masaüstü penceresi kopyası — işaretleme ve görünüm

**Files:**
- Modify: `AND web sitesi/tools/olustur.js` (R tablosu baştan; `LOGO`, `amblem()` yalnızca og.html için kalır; `menu()`, `paneller()`, `kopya()`, `durum()`, `cubuk()` yerine `pencere(dil)` ve yardımcıları; `sayfa()` içinde `kopya(dil)` → `pencere(dil)`), `assets/css/site.css` ("Ana ekran kopyası" bölümü → "Masaüstü penceresi kopyası"), `tools/denetle.js` (yeni denetimler)
- Referans (salt okunur): `AND/internal/masaustu/web/uygulama.css` (renk/ölçü/sınıf), `uygulama.js` (`IKONLAR` satır 62, `temaOnizleme` satır 1283, `kategoriIkonu` satır 769), `AND/dil/ceviriler/tui.json`, `AND/Eklentiler/forum/ceviri.json`, `AND/Eklentiler/admin/ceviri.json`, `AND/pluginui/ortak.go`; ekran görüntüleri scratchpad `ekran/forum-konu.png`, `gorunum.png`, `sohbet.png` (yoksa [[and-masaustu-arayuzu]] tarifinden yeniden al).

**Interfaces (site.js ile sözleşme — Task 4 bunlara dayanır):**
- Kök: `div.pencere[data-pencere]`; dar kipte okuma açıkken kökte `data-okuma` özniteliği.
- Kenar çubuğu: `nav.p-kenar` içinde `button.p-nav[data-ekran="<id>"]`; id'ler `forum, sohbet, ozel_chat, site, admin, deltax, ayarlar, hakkinda`. Başlangıçta yalnızca forum `aria-current="page"`.
- Forum kategorileri: `button.p-kat[data-kategori="<kat>"]` (Forum'un altında, sayılarla).
- Ekranlar: `section.p-ekran[data-ekran-icerik="<id>"]`; forum dışındakiler `hidden`.
- Forum listesi: `button.p-konu[data-konu="<n>"][data-kategori="<kat>"]` (ilki `aria-current="true"`); okuma: `article.p-okuma[data-konu-icerik="<n>"]` (ilki dışındakiler `hidden`); dar kip geri düğmesi `button.p-geri[data-geri]`.
- Ayarlar sekmeleri: `button.p-sekme[data-sekme="profil|gorunum|dil|eklentiler|ag"]` + `div[data-sekme-icerik="<ad>"]`; başlangıçta `gorunum` seçili.
- Tema kartları: `button.p-tema-karti[data-tema-sec="<id>"]` — mevcut `temaUygula` bunlara da `aria-pressed`/`.etkin` verir (Task 2'nin TEMA_KIMLIKLERI, 10 kart).
- Durum çubuğu: `[data-es]` (eş/düğüm metni; açılış animasyonu buraya yazar), sağda `AND v0.1.0` (`SURUM`).
- Tema adı: `[data-tema-adi]` kopyada en az bir yerde (Ayarlar › Görünüm başlığı yanında).

- [ ] **Step 1: Denetimleri yaz (kırmızı)**

`tools/denetle.js`'e:
- `kopya-eski-yok`: iki sayfada `class="terminal"`, `t-agac`, `data-panel-icerik` geçmez.
- `kopya-ekranlar`: her `data-ekran="<id>"` düğmesi için tam bir `data-ekran-icerik="<id>"` var (8 ekran); yalnızca forum ekranında `hidden` yok (Review Focus 5).
- `kopya-konular`: her `data-konu="<n>"` için bir `data-konu-icerik="<n>"`; en az 4 konu; her konunun `data-kategori`'si bir `data-kategori` kenar düğmesinde de var.
- `kopya-sekmeler`: 5 `data-sekme` ↔ 5 `data-sekme-icerik`; `data-tema-sec` kartları 10 tane (`p-tema-karti`).
- `kopya-esitlik`: EN ve TR sayfalarında `data-ekran`, `data-konu`, `data-sekme`, `data-kategori` öznitelik dizileri aynı sırada aynı sayıda (değerler kimlik olduğundan aynı).
- `id-tekil`: iki sayfada her `id="…"` bir kez.

Run: `node tools/denetle.js`. Expected: yeni denetimler ✗.

- [ ] **Step 2: R tablosunu yaz (EN + TR)**

Anahtarlar: `kenar` (arama yer tutucusu, bölüm başlıkları Topluluk/İletişim/İçerik/Yönetim/Geliştirme, öğe adları, profil satırı), `forum` (araç çubuğu: "Tüm konular", "Yeni konu"; süzme yer tutucusu; kategoriler `[kimlik, ad, sayı]`; konular `{kat, baslik, yazar, zaman, yanitSayisi, govde: [paragraf | {kod, dil}], yanitlar: [[yazar, zaman, metin]]}` ≥ 4 konu), `sohbet`, `ozelChat` (konuşmalar + seçili konuşmanın iletileri), `site` (sayfalar + seçili sayfanın başlık/gövdesi), `admin` (yasaklar: ad/anahtar kısaltması, gerekçe, kalan süre), `deltax` (proje adı, dal, durum kartı satırları, son commit'ler), `ayarlar` (sekme adları; Görünüm: "Tema" başlığı + açıklama; Profil/Dil/Ağ/Eklentiler sekmelerinin kısa içerikleri; Eklentiler kategorilere ayrılmış liste: İletişim: Forum 1.0.0, Özel Chat 2.2.0; İçerik: Site 1.4.0; Yönetim: Yönetici Paneli 4.2.0; Geliştirme: DeltaX 1.8.0; Tema: Pembiş 1.2.0), `hakkinda`, `durum` (`es(n)`, `dugum(n)`, `baglaniyor`). Metinlerin AND'da karşılığı olanlar kataloglardan birebir (EN = katalogdaki `en`); kişi adları bugünkü örneklerden (`KISI` renkleri: zeynep, mehmet, deniz, ayşe; kullanıcı "umut"). Sürümler plugin.json'lardan (Task başında yeniden oku).

- [ ] **Step 3: `pencere(dil): string` ve yardımcılarını yaz**

`ikon(ad): string` — uygulama.js `IKONLAR`'dan kullanılan yolları kopyalayan satır içi SVG (viewBox 0 0 24 24, stroke currentColor, 1.8, round). `temaKarti(t, dil): string` — `temaOnizleme`'nin çizimi (viewBox 0 0 160 64; hatlar isim/seçim/vurgu, rozet durağı) + ad + etkin işareti. Pencere: ince başlık (küre `/assets/img/logo.png` + "AND", sağda `aria-hidden` — □ ×), kenar çubuğu, ekranlar, durum çubuğu. Kopyanın içindeki her etkileşimli öğe gerçek `<button type="button">`; süs düğmeler (Yeni konu, Gönder, arama kutusu) `tabindex="-1"` ve `aria-hidden="true"` ya da `disabled` — ziyaretçiyi oyalamaz.

- [ ] **Step 4: CSS'i yaz**

site.css'te `.terminal`/`.t-*` kuralları silinir (og.html'in kullandığı `.t-amblem` ve `.hap` kalır — `tools/og.html`'de geçen sınıfları koru). Yeni bölüm `.pencere` kapsamında uygulama.css'in değişkenlerini kendi adlarıyla tanımlar (`--p-zemin`, `--p-yuzey-1..3`, `--p-cizgi*`, `--p-yazi*`, `--p-iyi` …) ve `--p-vurgu: var(--accent)`, `--p-isim: var(--name)`, `--p-rozet: var(--badge)`, `--p-secim: var(--sel)` ile sayfa temasına bağlanır (tema değişince kopya da boyanır). Ölçüler: kenar 248 px, araç çubuğu 52 px, köşe 8/12 px, seçili öğede 3 px sol vurgu çizgisi. Pencere yüksekliği masaüstünde `min(720px, 80vh)` civarı, içerik kendi içinde kayar (sayfa değil). Dar kip `@container` ya da `@media (max-width: 760px)`: kenar çubuğu 56 px ikon şeridi (etiketler `gorsel-gizli`), forumda liste ile okumadan biri (`[data-okuma]`). Not: konteyner sorgularında `em` kullan (`ch` değil; [[and-ecosystem-workflow]]).

- [ ] **Step 5: Kopyanın altındaki not ve tema düğmeleri**

`M.en/tr.ekranBaslik` ("AND's desktop window" / "AND'ın masaüstü penceresi"), `ekranNot` (spec A'daki metin; `and --terminal` `<code>` içinde — `e()` kaçışından sonra eklenir). Tema düğmeleri 10'a çıkmış olur (Task 2).

- [ ] **Step 6: Üret ve yeşil gör**

Run: `node tools/olustur.js && node tools/denetle.js`. Expected: hepsi ✓.

- [ ] **Step 7: Gözle bak**

`node tools/sunucu.js .`'i arka planda başlat; CDP ile 1440×900 ve 390×844'te EN ve TR giriş bölümünün görüntüsünü al; referans görüntülerle yan yana koy. Expected: kenar çubuğu, araç çubuğu, konu listesi, okuma alanı, kod bloğu ve durum çubuğu gerçek uygulamanınkine benzer; yatay kaydırma yok (`document.documentElement.scrollWidth <= innerWidth`).

- [ ] **Step 8: Commit**

```bash
git add tools/olustur.js tools/denetle.js assets/css/site.css index.html tr/index.html
git commit -m "Giriş: terminal ana menüsü yerine AND'ın masaüstü penceresinin kopyası"
```

---

### Task 4: Kopyanın davranışı (site.js)

**Files:**
- Modify: `AND web sitesi/assets/js/site.js` ("Ana ekran kopyası: menü ve eklentiler" ve "Açılış: eşler bağlanır" bölümleri baştan)
- Test: scratchpad `araclar/kopya-etkilesim.js` (CDP; depoya girmez)

**Interfaces:**
- Consumes: Task 3'ün öznitelik sözleşmesi; mevcut `temaUygula(id, kaydet)`.
- Produces: yok (son kullanıcı davranışı).

- [ ] **Step 1: Etkileşim testini yaz**

`kopya-etkilesim.js <devtools-port> <site-url>`: headless Edge'de (profil dizini scratchpad'de) sayfayı açar ve sırayla doğrular, her biri için ✓/✗ basar, biri ✗ ise çıkış 1:
1. `[data-ekran="sohbet"]` tıklanınca yalnızca `[data-ekran-icerik="sohbet"]` görünür ve o düğme `aria-current="page"`.
2. `[data-konu="2"]` tıklanınca yalnızca `[data-konu-icerik="2"]` görünür.
3. `[data-kategori="<ilk konunun kategorisi olmayan bir kat>"]` tıklanınca listede yalnızca o kategorinin konuları görünür; Forum düğmesine tıklamak süzmeyi kaldırır.
4. Ayarlar → `[data-sekme="eklentiler"]` → o içerik görünür; `gorunum` → `[data-tema-sec="okyanus"]` kartı tıklanınca `html[data-tema="okyanus"]` ve `localStorage['and-tema']==='okyanus'`, kopyanın altındaki Okyanus düğmesi `aria-pressed="true"`.
5. Klavye: `[data-ekran="deltax"]`'e `focus()`, Enter'a bas (CDP `Input.dispatchKeyEvent`) → deltax ekranı görünür; `document.activeElement` hâlâ o düğme (Review Focus 2).
6. 390×844: `scrollWidth <= innerWidth`; forumda liste görünür, okuma gizli; bir konuya dokununca okuma görünür ve `[data-geri]` görünür; geri → liste (Review Focus 3).
7. `Emulation.setEmulatedMedia` ile `prefers-reduced-motion: reduce`: yüklemeden hemen sonra `[data-es]` son metni ("23") içerir (Review Focus 4).
8. Konsolda hata yok (`Runtime.exceptionThrown`, `Log.entryAdded` level error; GitHub "latest release" 404'ü beklenen tek kayıt).

- [ ] **Step 2: Kırmızı olduğunu gör**

Run: `node kopya-etkilesim.js 9333 http://127.0.0.1:8123/`. Expected: 1–6 ✗ (eski site.js `.t-agac` arıyor).

- [ ] **Step 3: Davranışı yaz**

Ekran değiştirme (`data-ekran` → panel; `aria-current`), kategori süzme, konu seçme, Ayarlar sekmeleri, dar kip (`matchMedia('(max-width: 760px)')` ya da kökün genişliği; `data-okuma` + geri), açılış animasyonu `[data-es]`'e (mevcut `[[1,0],[4,1],[9,2],[15,3],[23,4]]` dizisi, hareket azaltmada hiç oynamaz). Tema kartları ek kod istemez (`[data-tema-sec]` belge düzeyinde dinleniyor). Eski `.t-agac`/eklenti aç-kapa/`data-acik-sayi`/`data-yuklu-sayi` kodu silinir. Dosya başındaki açıklama yorumu güncellenir.

- [ ] **Step 4: Yeşil olduğunu gör**

Run: `node tools/olustur.js && node tools/denetle.js && node <scratchpad>/araclar/kopya-etkilesim.js 9333 http://127.0.0.1:8123/` ve aynısı `/tr/` için. Expected: hepsi ✓.

- [ ] **Step 5: Commit**

```bash
git add assets/js/site.js
git commit -m "Giriş: masaüstü kopyasında ekranlar, forum, ayarlar ve dar ekran davranışı"
```

---

### Task 5: Sayfa metinleri, paylaşım görseli ve site belgeleri

**Files:**
- Modify: `AND web sitesi/tools/olustur.js` (M.en, M.tr, `KOD`, `GUNCELLEME`, `ogSablonu()` alt satırı), `README.md`, `README.tr.md` (sitenin), `tools/denetle.js`
- Regenerate: `assets/img/og.png`, `assets/img/og-tr.png` (tools/og.html'den CDP 1200×630; `?dil=tr`)

**Interfaces:**
- Consumes: Task 1'in çapaları; Task 2–4'ün çıktısı.

- [ ] **Step 1: Denetimleri yaz (kırmızı)**

`tools/denetle.js`'e:
- `metin-terminal`: iki sayfada ve og.html'de şu kalıplar YOK: `/runs in (a|your) terminal/i`, `/in your terminal\./i`, `/terminalinde çalışır|bir terminalde çalışır|Terminalde eşten eşe/`.
- `metin-konu-ac`: `#derleme-komutlari` bloğunda `konu_ac` yok, `Eklentiler/forum/and-plugin-forum` var.
- `metin-surumler`: sayfalarda `DeltaX` ile birlikte `1.7.1`, `Özel Chat`/`Private Chat` ile `2.1.1`, `Site`/`Website` ile `1.3.1` geçmez.
- `metin-esitlik`: EN ve TR'de `.kv dt` sayısı, `details` sayısı, `.agac li` sayısı ve `section[id]` listesi aynı.
- `metin-guncelleme`: sitemap `lastmod` = bugünün tarihi (`2026-11-25` ya da yayın günü).

Run: `node tools/denetle.js`. Expected: yeni denetimler ✗.

- [ ] **Step 2: M tablosunu güncelle (EN + TR, spec B maddeleri)**

Her madde için kodda doğrula (kaynak dosyayı metinde değil kendinde bul):
- `aciklama`, `ozet`, `ogAlt`, alt bilgi: kendi penceresinde ya da terminalde.
- Özellikler: yeni "Desktop and terminal"/"Masaüstü ve terminal"; "Plugins": pencerede çalışır, kategoriler, `and plugin install` (cmd/and/eklenti.go'daki gerçek İngilizce takma ad); "Themes and languages": 10 tema + özel renk; yeni "VS Code" satırı (and-vscode 0.10.0: Live Panel'de canlı Forum sekmesi; bağlantı AND Ayarlar › Eklentiler'deki VS Code anahtarıyla açılır — `pluginapi/harici.go`).
- Nasıl çalışır › `son`: `internal/network` (relay'ler, eş değişimi `/and/pex/1.0.0`, bilinen eşler) ile karşılaştır; yanlışsa düzelt, eksikse bir cümle.
- Kurulum: `giris` (Windows: kendi penceresi; Linux/macOS: varsayılan tarayıcı; `and --terminal`); `derle`: Go 1.26.4 ve "AND ile forum eklentisini derler"; `KOD`:
  ```
  git clone https://github.com/AND-community/AND.git
  cd AND
  go build -o and ./cmd/and
  go build -o Eklentiler/forum/and-plugin-forum ./Eklentiler/forum
  ./and
  ```
- Güvenlik: "Your terminal" → "Interface"/"Arayüz" (yerel sunucu 127.0.0.1 + tek kullanımlık açılış kodu + sıkı CSP + ağ içeriği HTML sayılmaz; terminalde kaçış dizileri temizlenir — `internal/masaustu/sunucu.go`); "Plugins": arayüz ekranları AND'da doğrulanır (`arayuz.Coz`).
- Projeler: AND satırı forum eklentisini anar; konu_ac: "archived; merged into the Forum plugin"/"arşivlendi; forum eklentisine katıldı".
- Sorular: sistemler (masaüstü/tarayıcı/terminal), şifre unutma (masaüstü giriş kartındaki "Kurtarma kodun var mı?" ve terminalde ctrl+r), kurtarma kodu kaybı (kodu yeniden gösterme yalnızca terminal arayüzünde: Ayarlar › Bilgi, `m` — `internal/tui`'de doğrula), eklenti yazma (`and plugin new`; `PLUGINS.md` İngilizce, `EKLENTILER.md` Türkçe).
- `GUNCELLEME` = yayın günü. `ogSablonu()` alt satırı: EN "Peer-to-peer forum, chat and personal sites, on your desktop or in your terminal. Free and open source." / TR "Masaüstünde ya da terminalde eşten eşe forum, sohbet ve kişisel siteler. Ücretsiz ve açık kaynak."

- [ ] **Step 3: Sitenin README'leri**

`README.md`/`README.tr.md`: kopya artık `internal/masaustu/web` (uygulama.css, uygulama.js) ve eklentilerin arayüzünü taklit eder; uygulama değişince R tablosu ve `.pencere` stilleri güncellenir; `node tools/denetle.js` nasıl çalıştırılır; `assets/js/site.js` satırının açıklaması.

- [ ] **Step 4: Yeşil gör ve bağlantıları denetle**

Run: `node tools/olustur.js && node tools/denetle.js`. Expected: hepsi ✓. Sonra iki sayfadaki bütün `https://` bağlantılarını çıkarıp `curl -s -o /dev/null -w "%{http_code}" -L` ile sına. Expected: hepsi 200 (çapalı GitHub bağlantılarında çapanın README'de var olduğunu Task 1'deki başlıklarla karşılaştır).

- [ ] **Step 5: Paylaşım görsellerini yeniden al**

`tools/og.html` ve `tools/og.html?dil=tr` → CDP 1200×630 → `assets/img/og.png`, `og-tr.png`. Gözle bak: yeni alt satır sığıyor.

- [ ] **Step 6: Commit**

```bash
git add tools/olustur.js tools/denetle.js index.html tr/index.html 404.html sitemap.xml tools/og.html assets/img/og.png assets/img/og-tr.png README.md README.tr.md
git commit -m "Metinler: masaüstü uygulaması, forum eklentisi, pencere içi eklentiler, 10 tema ve VS Code"
```

---

### Task 6: Yayın — AND README ve site

**Files:** yok (yayın ve doğrulama)

- [ ] **Step 1: Son yerel doğrulama**

Site: `node tools/olustur.js && git status --short` (üretim sonrası fark yok) → `node tools/denetle.js` ✓ → etkileşim testi EN+TR ✓ → CDP ile 1440, 1024, 760, 390 genişlikte tam sayfa görüntüsü al ve bak (taşma, kesilme, okunurluk; 10 temanın her birinde giriş bölümü bir kez).
AND: `git log origin/main..main --oneline` → spec, plan ve README commit'leri; yalnızca .md değiştiği için `git diff --stat origin/main..main` yalnızca .md dosyaları gösterir.

- [ ] **Step 2: AND'ı gönder**

`cd "…/AND Proje/AND"` (ayrı komut) → `pwd` → `git push origin main`. Expected: `… main -> main`. `git ls-remote origin refs/heads/main` yerel main'e eşit.

- [ ] **Step 3: Siteyi gönder**

`cd "…/AND Proje/AND web sitesi"` → `pwd` → `git push origin main`.

- [ ] **Step 4: Canlıda doğrula**

~1–2 dakika sonra (Pages): `curl -s https://and-community.github.io/ | grep -c 'data-pencere'` ≥ 1; `/tr/` aynı; `site.css`, `site.js`, `og.png` canlı baytları yereldekilerle aynı (`curl -s … | sha256sum`). Etkileşim testini canlı adrese karşı çalıştır. Expected: hepsi ✓ (tek beklenen konsol kaydı releases/latest 404).

---

### Task 7: Organizasyon profili, açıklamalar ve konu_ac arşivi

**Files:**
- Modify (geçici klon `AND Proje/org-profil-gecici/`): `profile/README.md`, `profile/README.tr.md`

- [ ] **Step 1: Klonla ve eski yerleri listele**

`AND Proje/` altında: `git clone https://github.com/AND-community/.github.git org-profil-gecici` (varsa önce içine bak; eski bir kopya kaldıysa `git pull`). Run: `grep -nE "terminal-native|All you need is a terminal|same terminal|konu_ac|Konu Aç|in Turkish\)|TUI \|" profile/README.md profile/README.tr.md`. Expected: spec D'deki yerler listelenir.

- [ ] **Step 2: Düzelt (EN + TR birlikte, spec D)**

Üst satır, "What is AND?", özellik tablosu (yeni "Desktop interface" satırı; Plugins satırı), depolar tablosu (AND satırına forum eklentisi; konu_ac "*Archived.* Merged into AND's Forum plugin (`Eklentiler/forum`)"), Getting started (Task 5'teki `KOD` ile aynı komutlar + Windows notu), Plugin system bölümü (pencere içinde; `"ui"` ve `"category"` alanlı örnek manifest — alan adlarını `pluginapi/manifest` tanımından al; `and plugin new` / `and plugin install`; rehber `PLUGINS.md` İngilizce), Security model (Interface: 127.0.0.1, tek kullanımlık kod, CSP), Tech stack (Desktop: WebView2 via `go-webview2`, xterm.js; TUI satırı kalır). TR dosyada aynı yapı.

- [ ] **Step 3: Denetle**

Step 1'deki grep → yalnızca bilerek bırakılan konu_ac "Archived" satırları kalır. Bağlantıları curl ile sına (200). GitHub'ın Markdown API'siyle (`POST https://api.github.com/markdown`, kimliksiz) iki dosyayı HTML'e çevirip tablo ve kod bloklarının bozulmadığına bak.

- [ ] **Step 4: Commit ve gönder**

```bash
git add profile/README.md profile/README.tr.md
git commit -m "Profil: masaüstü uygulaması, forum eklentisi ve yeni eklenti sistemi"
```
Ayrı komutlarla `cd "…/AND Proje/org-profil-gecici"` → `pwd` → `git push origin main`. Sonra `cd "…/AND Proje"` (klasörü silmek güvenlik denetimine takılabilir; takılırsa kullanıcıya bırak, zorlamadan).

- [ ] **Step 5: Açıklamalar, konular, arşiv (API)**

Saklı GCM kimliğiyle (`git credential fill`, token yalnızca stdin/ortamda) sırasıyla:
1. `PATCH /orgs/AND-community` `description`: "Serverless, peer-to-peer forum community with a desktop app, a terminal interface and plugins." (≤160 karakter).
2. `PATCH /repos/AND-community/AND` `description`: "Serverless peer-to-peer forum with a desktop app and a terminal interface: no accounts, no central server. Forum, chat, private messages, personal sites and plugins over libp2p."; `PUT …/topics`: mevcut konular + `desktop`.
3. `PATCH /repos/AND-community/and-community.github.io` `description`: "The AND website, served by GitHub Pages: an English/Turkish introduction with a live replica of AND's desktop app."; konulardan `terminal` çıkar, `desktop` eklenir.
4. `PATCH /repos/AND-community/konu_ac` `description`: "Archived: merged into AND's Forum plugin (Eklentiler/forum in AND-community/AND)." → SONRA `PATCH … {"archived": true}`.
Expected: her çağrı 200.

- [ ] **Step 6: Kimliksiz doğrula**

`curl -s https://api.github.com/orgs/AND-community` ve dört depo için açıklama/konular/`archived`; `https://raw.githubusercontent.com/AND-community/.github/main/profile/README.md` yeni metni içerir. Expected: hepsi beklenen değerde; konu_ac `archived: true`.

- [ ] **Step 7: Bellek kaydı**

`and-session-log.md`'ye bu turun girdisi (commit'ler, yayınlar, kalanlar); `and-ecosystem-workflow.md`'de site kopyasının artık masaüstü penceresini taklit ettiği, `tools/denetle.js` ve 10 tema notu.
