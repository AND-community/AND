# Forum eklentisi, Site ve Yönetici Paneli arayüzü — Uygulama planı

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Site ve Yönetici Paneli masaüstünde AND'ın arayüzüyle açılır; forum çekirdekten çıkıp hem
terminalde hem masaüstünde çalışan `forum` eklentisine taşınır (konu_ac içine katılır).

**Architecture:** DeltaX/Özel Chat kalıbı: her eklentinin bubbletea modeli `Arayuz()` ile ekranını
anlatır, olaylar sentetik tuşlarla aynı işleyicilere gider. Forum verisi genel eklenti API'sinden gelir
(isteğe bağlı arayüzlerle geriye uyumlu genişletilir). Protokol sürüm 2: `gonderi` bileşeni, `menu`
(eklenti→AND) ve `git` (AND→eklenti) mesajları; AND bilinmeyen mesaj türünü yok sayar.

**Tech Stack:** Go 1.26, bubbletea, `arayuz`/`arayuztest`, `pluginapi`, masaüstü `uygulama.js`/`uygulama.css`.

**Spec:** `docs/superpowers/specs/2026-11-25-forum-site-yonetici-design.md` (önceki:
`2026-11-24-eklenti-arayuzu-design.md`).

## Global Constraints

- Her kullanıcıya görünen metin `dil` üzerinden ve 5 çeviriyle (en, de, es, fr, ru); eklenti metinleri
  kendi `ceviri.json`'unda; dil metinlerinde `|` yok.
- Forum verisi, imzalar, ağ protokolü değişmez. `pluginapi`'ye yalnızca ekleme: yeni JSON alanları, yeni
  uçlar, **isteğe bağlı** arayüzler (tür denetimiyle); dışa açık bir arayüze yöntem eklenmez.
- Protokol v1 eklentileri çalışmaya devam eder; v2 eklentisi `Merhaba.Surum < 2` iken `menu` ve
  `gonderi` kullanmaz.
- Forum eklentisi yalnızca `["forum"]` izni ister.
- go.mod değişirse `go generate ./internal/lisans`. Yeni bağımlılık beklenmiyor.
- Görev başına commit (`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`), push yok. exe'ler
  yalnızca kullanıcının onayıyla. `AND/Claude outputs/` ve iç içe eklenti depolarına dokunulmaz
  (yalnızca Görev 8'de konu_ac README notu).
- Her görevin sonu: `go test -count=1 ./...` (ilgili paketler + görevin dokunduğu her şey) yeşil,
  `go vet` temiz.

## Review Focus

1. **Çok uzun konu:** 2500 yanıtlı ya da 64 KiB gövdeli konu protokol sınırlarını (5000 öğe, 4 MiB satır)
   aşmamalı; masaüstü son 200 yanıtı gösterir, "Daha eski yanıtlar" ile genişler → Görev 7 testi.
2. **Forum eklentisi kapalı/izinsiz/yok:** masaüstü Sohbet'e düşer, `#forum/<id>` hata vermez; TUI
   menüsünde Forum girdisi görünmez → Görev 8 testleri.
3. **Yanıt yazarken canlı güncelleme:** yeni yanıt gelince yazılmakta olan metin silinmez → Görev 7 testi.
4. **Bozuk ya da eksik taslak dosyaları** (`yanit_taslaklari.json`, `taslaklar_<kat>.json`): eklenti
   çökmez, boş taslakla devam eder → Görev 6 testi.
5. **v2 eklenti, v1 AND:** `Surum 1` merhabasında `menu` satırı yazılmaz, `gonderi` yerine `kart`+`zengin`
   → Görev 5 ve 7 testleri. AND bilinmeyen türü yok sayar → Görev 5 testi.

---

### Task 1: Site arayüz kipi (1.3.1 → 1.4.0)

**Files:**
- Create: `Eklentiler/site/arayuz.go`, `Eklentiler/site/arayuz_test.go`
- Modify: `Eklentiler/site/model.go` (alan `arayuzKipi bool`; `Update`'te `arayuz.Merhaba`/`arayuz.Olay`),
  `Eklentiler/site/main.go` (`if arayuz.Etkin() { arayuz.Calistir(m) }`), `plugin.json` (`"ui": 1`,
  `"version": "1.4.0"`), `ceviri.json`, `README.md`

**Interfaces:**
- Produces: `func (m model) Arayuz() arayuz.Ekran`, `func (m model) olayIsle(o arayuz.Olay) (tea.Model, tea.Cmd)`,
  `func tusMsg(s string) tea.KeyMsg` (Özel Chat'teki gibi).
- Kimlikler: liste `sayfalar` (satır düğmeleri `duzenle`, `yukari`, `asagi`, `sil`), araçlar `yeniSayfa`,
  `taslaklar`; diyalog listesi `sablonlar`; form `sayfaFormu` (alanlar `slug`, `baslik`, `govde`;
  düğmeler `yayinla` (Gonderir, Ctrl+S), `taslakSakla`, `vazgec`); taslak listesi `taslakListesi`
  (düğmeler `ac`, `sil`).

- [ ] **Step 1: Testleri yaz** (`arayuztest` ile, `pluginapitest` sahte sunucusu ya da sitenin mevcut test istemcisi):
  - `TestArayuz_BosListe`: sayfa yokken `sayfalar` boş metni "Henüz sayfan yok" ve `yeniSayfa` var.
  - `TestArayuz_YeniSayfaSablonlaDuzenle`: `yeniSayfa` → `sablonlar` diyaloğu, ilk öğe "Boş sayfa";
    bir şablon seçilince `sayfaFormu` açılır ve `govde` şablon gövdesini taşır.
  - `TestArayuz_YayinlaFormdanGider`: `FormGonder("sayfaFormu", {slug:"hakkimda", baslik:"Hakkımda", govde:"a\tb"})`
    → sahte API'ye giden gövde `"a\tb"` (sekme korunur), ekran listeye döner, bildirim gelir.
  - `TestArayuz_KirliFormdanCikisOnayIster`: alan değiştirilip `vazgec`/`geri` → `Onay` diyaloğu.
  - `TestArayuz_SilOnay`: `sil` → tehlikeli onay; onaylanınca silme isteği gider.
  - `TestArayuz_Siralama`: `asagi` ilk sayfayı ikinci sıraya taşır.
  - `TestArayuz_TaslakAcSil`: `taslaklar` → `taslakListesi`; `ac` formu taslakla doldurur.
  - `TestArayuz_HerDilde`: 6 dilde her ekran `arayuztest.Denetle`'den geçer, çevrilmemiş metin kalmaz.
- [ ] **Step 2: Çalıştır, kırmızıyı gör:** `go test ./Eklentiler/site/ -run TestArayuz` → derleme/eksik hatası.
- [ ] **Step 3: Uygula.** Olaylar sentetik tuşlarla mevcut işleyicilere gider (`tusListe`, `tusDuzenle`…).
  Masaüstünde `govde` bubbles textarea'sını atlar (DeltaX `duzenleyiciIcerik` gibi), çünkü textarea
  sekmeyi boşluğa çevirir.
- [ ] **Step 4: Yeşil:** `go test -count=1 ./Eklentiler/site/` → PASS.
- [ ] **Step 5: Commit:** "Site: AND masaüstünde terminal yerine AND'ın arayüzüyle (1.4.0)".

### Task 2: Yasaklama uç noktası (`/api/v1/mod/ban`)

**Files:**
- Modify: `internal/moderation/moderation.go` (`ResolveBanAuthority` buraya taşınır, TUI'deki
  `resolveBanAuthority` onu çağırır ya da silinir), `pluginapi/api.go`, `pluginapi/pluginapitest/sahte.go`,
  `cmd/and/main.go` (`modAdapter.Ban`), `internal/tui/*` (taşınan işlevin çağrıları)
- Test: `pluginapi/api_test.go` (ya da `izinler_test.go`), `internal/moderation/moderation_test.go`, `internal/tui/ban_test.go`

**Interfaces:**
- Produces:
  - `func ResolveBanAuthority(mod *Moderator, dataDir, myKey, targetAuthorKey string) (canBan, isFounder bool, cert *ModeratorCert)` (moderation paketi).
  - `type BanReq struct { AuthorKey string \`json:"author_key"\`; DurationSeconds int64 \`json:"duration_seconds,omitempty"\`; Permanent bool \`json:"permanent,omitempty"\`; Reason string \`json:"reason"\` }`
  - `type BanBackend interface { Ban(ctx context.Context, authorKey string, duration time.Duration, permanent bool, reason string) error }` — `s.mb` üzerinde tür denetimiyle; yoksa 501.
  - Route `POST /api/v1/mod/ban`, `izin: IzinModerasyon`, `allowExternal: true`.
  - `func (c *Client) Ban(authorKey string, duration time.Duration, permanent bool, reason string) error`.
  - Hatalar: boş/boşluk sebep → 400; anahtar 64 hex değil → 400; yetki yok, hedef kendisi ya da kurucu → 403.

- [ ] **Step 1: Testleri yaz:** `TestModBan_IzinTablosu` (route tablosu testi yeni ucu `moderation` ile görür),
  `TestModBan_BosSebep400`, `TestModBan_GecersizAnahtar400`, `TestModBan_YetkisizReddedilir` (sahte arka uç
  yetki hatası → 403), `TestModBan_SureVeKalici` (sahte arka uç `duration=24h`, `permanent=false` alır;
  `permanent=true` iken süre yok sayılır), `TestModBan_ArkaUcYoksa501`; moderation:
  `TestResolveBanAuthority_KendiniVeKurucuyuYasaklayamaz`.
- [ ] **Step 2: Kırmızı:** `go test ./pluginapi/ ./internal/moderation/ -run 'ModBan|ResolveBan'`.
- [ ] **Step 3: Uygula.** `modAdapter.Ban` = `moderation.ResolveBanAuthority` + `moderation.IssueBanFor`
  (TUI `issueBanCommon` ile aynı yol; süre `time.Now().UTC().Add(d)`).
- [ ] **Step 4: Yeşil:** `go test -count=1 ./pluginapi/... ./internal/moderation/ ./internal/tui/ ./cmd/and/`.
- [ ] **Step 5: Commit:** "Eklenti API'si: yasaklama ucu (mod/ban)".

### Task 3: Yönetici Paneli arayüz kipi (4.1.0 → 4.2.0)

**Files:**
- Create: `Eklentiler/admin/arayuz.go`, `Eklentiler/admin/arayuz_test.go`, `Eklentiler/admin/yasakla.go`
- Modify: `model.go`, `konular.go`, `banlar.go`, `main.go`, `plugin.json` (`"ui": 1`, `4.2.0`), `ceviri.json`, `README.md`

**Interfaces:**
- Consumes: `Client.Ban` (Task 2).
- Produces: `Arayuz()`, `olayIsle`; TUI'de yeni tuşlar: Konular'da `b` (seçili konunun yazarını yasakla),
  Yasaklar'da `n` (anahtarla yasakla) — ikisi de aynı yasak formunu (`yasakFormu`) açar.
- Kimlikler: `sekmeler` (`konular`, `yasaklar`, `gecmis`, `moderatorler` — sonuncusu yalnızca kurucuda);
  tablolar `konuTablosu` (satır düğmeleri `sil`, `yasakla`), `yasakTablosu` (`kaldir`), `gecmisTablosu`,
  `modTablosu` (`geriAl`); araçlar `anahtarlaYasakla`, `moderatorAta`; formlar `yasakFormu` (alanlar
  `anahtar` (yalnızca anahtarla yasaklarken), `sure` (secim: `1s`, `1g`, `7g`, `30g`, `kalici` → "1 saat",
  "1 gün", "7 gün", "30 gün", "Ömür boyu"), `sebep` (zorunlu)), `atamaFormu` (`anahtar`, `sure`).

- [ ] **Step 1: Testleri yaz:** `TestArayuz_SekmelerRoleGore` (moderatör 3, kurucu 4 sekme),
  `TestArayuz_KonuSilOnay`, `TestArayuz_YazariYasakla` (`yasakla` → `yasakFormu`, `sure=7g`, sebep →
  sahte `Ban(yazar, 168h, false, sebep)`), `TestArayuz_AnahtarlaYasaklaGecersizAnahtar` (alan hatası, istek
  gitmez), `TestArayuz_BosSebepGonderilmez`, `TestArayuz_YasakKaldirOnay`, `TestArayuz_ModeratorAta`,
  `TestArayuz_HerDilde`; TUI: `TestKonular_BTusuYasakFormuAcar`.
- [ ] **Step 2: Kırmızı:** `go test ./Eklentiler/admin/ -run 'Arayuz|BTusu'`.
- [ ] **Step 3: Uygula** (sentetik tuşlarla mevcut işleyicilere; tablo satır kimliği konu/anahtar).
- [ ] **Step 4: Yeşil:** `go test -count=1 ./Eklentiler/admin/`.
- [ ] **Step 5: Commit:** "Yönetici Paneli: AND masaüstünde arayüzle, yazar yasaklama (4.2.0)".

### Task 4: Forum değişim sürümü ve API eklemeleri

**Files:**
- Modify: `internal/forum/forum.go` (+ yeni `internal/forum/degisim.go`), `pluginapi/api.go`,
  `pluginapi/pluginapitest/sahte.go`, `cmd/and/main.go` (`forumAdapter`'a `profile *profile.Store`,
  `chat *chatlog.Store`; yeni yöntemler)
- Test: `internal/forum/degisim_test.go`, `pluginapi/api_test.go`

**Interfaces:**
- forum: `func (f *Forum) Surum() uint64`; `func (f *Forum) Bekle(ctx context.Context, since uint64) (surum uint64, konular []string, hepsi bool)`
  — `Surum() > since` ise hemen döner, değilse değişiklik ya da ctx bitene kadar bekler; son 256
  değişiklik (sürüm, konu kimliği) halkada; `since` halkanın gerisindeyse `hepsi=true`. Sürüm; yeni konu,
  yeni yanıt (ağdan ya da yerelden — `newPosts`/`newReplies`'a yazılan her yer), konu silme (yerel, mod,
  silme kaydı) ile artar. Bekleyenler kapat-ve-yenile kanalıyla uyandırılır (tek tüketicili `NewPosts`'a
  dokunulmaz).
- pluginapi:
  - `PostSummary` += `CreatedAt time.Time \`json:"created_at"\``, `LastActivity time.Time \`json:"last_activity"\``,
    `ReplyCount int \`json:"reply_count"\``, `AuthorRole string \`json:"author_role,omitempty"\``.
    `PostDetail` ve `ReplyInfo` += `AuthorRole`. Rol, masaüstü `cekirdek.rol` ile aynı kural (üye boş).
  - `type ForumChanges struct { Surum uint64 \`json:"surum"\`; Konular []string \`json:"konular"\`; Hepsi bool \`json:"hepsi"\` }`
  - `type ProfileInfo struct` — terminal `profilMetni`'nin gösterdiği her şey (ad, anahtar, rol, katılım,
    konu/yanıt/sohbet sayıları ve varsa profil metni/site bağlantısı); alan adları İngilizce JSON.
  - İsteğe bağlı arayüzler (`s.f` üzerinde): `ForumChangesBackend { ForumChanges(ctx context.Context, since uint64) (ForumChanges, error) }`,
    `OwnDeleteBackend { DeleteOwnPost(ctx context.Context, postID string) error }`,
    `ProfileBackend { Profile(key string) (*ProfileInfo, error) }`; yoksa 501.
  - Route'lar: `GET /api/v1/forum/changes?since=N` (IzinTemel; ≤ 25 sn bekler; `since` sayı değilse 400),
    `POST /api/v1/forum/delete-own` gövde `{"post_id"}` (IzinForum; başkasının → 403, yok → 404),
    `GET /api/v1/profile?key=` (IzinTemel; 64 hex değilse 400; bilinmiyorsa 404).
  - Client: `ForumChanges(since uint64) (ForumChanges, error)` (bu çağrı ≥ 35 sn zaman aşımlı),
    `DeleteOwnPost(postID string) error`, `Profile(key string) (*ProfileInfo, error)`.

- [ ] **Step 1: Testleri yaz:** `TestSurum_YeniKonuYanitSilmeArtirir`, `TestBekle_HemenDoner`,
  `TestBekle_DegisiklikteUyanir`, `TestBekle_CtxBitince`, `TestBekle_HalkaGeridenHepsi`; pluginapi:
  route izin tablosu yeni üç uçla, `TestForumChanges_UzunYoklama` (değişiklik 100 ms sonra → yanıt
  yeni sürümle), `TestForumChanges_GecersizSince400`, `TestDeleteOwn_Baskasinin403`,
  `TestProfile_GecersizAnahtar400`, `TestAllPosts_YeniAlanlar` (sahte arka uç tarih/yanıt sayısı/rol döner,
  JSON'da görünür), `TestEskiIstemciYeniAlanlariYokSayar` (eski `PostSummary` biçimine çözülebilir).
- [ ] **Step 2: Kırmızı:** `go test ./internal/forum/ ./pluginapi/ -run 'Surum|Bekle|ForumChanges|DeleteOwn|Profile|YeniAlan'`.
- [ ] **Step 3: Uygula.**
- [ ] **Step 4: Yeşil:** `go test -count=1 ./internal/forum/ ./pluginapi/... ./cmd/and/`.
- [ ] **Step 5: Commit:** "Eklenti API'si: forum değişimleri, kendi konusunu silme, profil".

### Task 5: Protokol sürüm 2 (`gonderi`, `menu`, `git`)

**Files:**
- Modify: `arayuz/tur.go` (`Surum = 2`, `TurGonderi`, `MenuOgesi`, `Oge`'ye gönderi alanları),
  `arayuz/kurucu.go`, `arayuz/coz.go`, `arayuz/calistir.go`, `arayuz/arayuztest/surucu.go`,
  `internal/masaustu/arayuz.go`, `internal/masaustu/api.go`, `internal/masaustu/web/uygulama.js`,
  `internal/masaustu/web/uygulama.css`, `internal/masaustu/vitrin_test.go`, `EKLENTILER.md`, `PLUGINS.md`
- Test: `arayuz/coz_test.go`, `arayuz/calistir_test.go`, `arayuz/arayuztest/surucu_test.go`, `internal/masaustu/arayuz_test.go`

**Interfaces:**
- `gonderi` öğesi JSON: `{"tur":"gonderi","id","yazar","yazarAlt","kendi","rozetler":[...],"zaman":"<metin>","muhur":true,"yanit":true,"cocuklar":[zengin|kod|metin],"dugmeler":[...]}`
  (`zaman` Özel Chat `Mesaj.Zaman` gibi düz metin; alan adları mevcut `Oge` adlandırmasına uyar — çocuk
  listesi ve düğmeler `Oge`'de zaten hangi alandaysa o). Kurucu: `func Gonderi(id, yazar string, cocuklar []Oge, ayarlar ...Ayar) Oge`;
  ayarlar `IleYazarAlt(string)`, `IleZaman(string)`, `IleRozetler(...Rozet)`, değerler `Muhurlu`, `YanitGonderisi`, `KendiGonderisi`.
  Çözümde `cocuklar` yalnızca `zengin`, `kod`, `metin` olabilir (başkası `TurBilinmeyen`).
- `type MenuOgesi struct { ID string \`json:"id"\`; Etiket string \`json:"etiket"\`; Ikon string \`json:"ikon,omitempty"\`; Sayi int \`json:"sayi,omitempty"\` }`;
  satır `{"tur":"menu","ogeler":[...]}`; `EklentiMesaji.Menu []MenuOgesi`; ≤ 50 öğe (fazlası kırpılır),
  etiket temizlenir ve ≤ 80 rune, kimlik ≤ 512, tanınmayan ikon boşaltılır.
- `var ErrBilinmeyenTur = errors.New(...)`: `Coz` bilinmeyen `tur` için bunu (sarılı) döner.
  AND `isle`'de `errors.Is(err, arayuz.ErrBilinmeyenTur)` ise oturum başına bir kez günlükler, `hata`
  olayı yaymaz.
- SDK: `AndMesaji.Hedef string \`json:"hedef,omitempty"\``; `type Git struct{ Hedef string }`;
  `func Menu(ogeler []MenuOgesi) tea.Cmd`. `sarmal`, geçen `Merhaba`'nın sürümünü saklar; `Surum < 2` iken
  menü satırı yazılmaz. `Merhaba` modele yine iletilir (eklenti `Surum`'a göre `gonderi` kullanır).
- arayuztest: `func (s *Surucu) Git(hedef string)`, `func (s *Surucu) Menu() []arayuz.MenuOgesi` (son menü),
  `Yeni` varsayılan olarak `Merhaba{Surum: arayuz.Surum}` gönderir (bugünkü davranış neyse korunur,
  sürüm 2 olur); `YeniSurumle(m, surum int)` eski AND'ı taklit eder.
- AND: oturum son menüyü saklar; SSE `menu` olayı `{id, ad, ogeler}`; `/api/eklenti/oturumlar` her oturumla
  `menu`'yü döner; `POST /api/arayuz/git` gövde `{id, hedef}` → `AndMesaji{Tur:"git", Hedef}` (hedef ≤ 512,
  oturum yoksa 404).
- JS: `gonderi` mevcut `yazarSatiri` + konu/yanıt sınıflarıyla çizilir; kenar çubuğunda eklenti girdisinin
  altında menü öğeleri (bugünkü `alt-nav` görünümü, sayı rozetiyle); Ctrl+K menü öğelerini listeler;
  tıklama `eklentiGoster(ad)` + `/api/arayuz/git`.

- [ ] **Step 1: Testleri yaz:** `TestCoz_GonderiGidisDonus`, `TestCoz_GonderiCocukSuzulur`,
  `TestCoz_MenuSinirlar` (60 öğe → 50; uzun etiket kırpılır; bilinmeyen ikon boş),
  `TestCoz_BilinmeyenTur` (`errors.Is(err, ErrBilinmeyenTur)`), `TestCalistir_GitMesaji`,
  `TestCalistir_MenuSurum1deYazilmaz`, `TestCalistir_MenuSurum2deYazilir`, `TestSurucu_GitVeMenu`;
  AND: `TestArayuz_MenuYayinlanirVeOturumlardaDoner`, `TestArayuz_GitIletilir`,
  `TestArayuz_BilinmeyenTurOturumuDusurmez` (sahte kip `AND_SAHTE_ARAYUZ=gelecek`: önce bilinmeyen tür, sonra
  ekran; ekran gelir, `hata` olayı yok); `TestVitrinGecerli` vitrine eklenen `gonderi` ile geçer.
- [ ] **Step 2: Kırmızı:** `go test ./arayuz/... ./internal/masaustu/ -run 'Gonderi|Menu|BilinmeyenTur|Git|Vitrin'`.
- [ ] **Step 3: Uygula** (Go, JS, CSS, belgeler: EKLENTILER.md/PLUGINS.md "Protokol sürüm 2").
- [ ] **Step 4: Yeşil + görsel:** `go test -count=1 ./arayuz/... ./internal/masaustu/`; vitrin önizlemesini
  başsız Edge ile aç, `gonderi` ve kenar çubuğu menüsünün ekran görüntüsüne bak.
- [ ] **Step 5: Commit:** "arayuz: protokol sürüm 2 — gonderi, menu, git".

### Task 6: Forum eklentisi — terminal ve veri katmanı

**Önce:** bugünkü masaüstü forumunun "önce" ekran görüntülerini al (liste, konu, yeni konu; önizleme
verisiyle) ve scratchpad'e kaydet — Görev 7 karşılaştırması için.

**Files:**
- Create: `Eklentiler/forum/` — `plugin.json` (`{"name":"forum","label":"Forum","version":"1.0.0","description":"Kategoriler, konular ve yanıtlar; yeni konu açma ve taslaklar","author":"AND","category":"communication","permissions":["forum"],"ui":1}`),
  `main.go`, `model.go`, `veri.go`, `taslak.go`, `gorunum.go`, `yenikonu.go`, `profil.go`, `ceviri.go`,
  `ceviri.json`, `README.md`, testler (`model_test.go`, `taslak_test.go`, `ekran_test.go`, `ceviri_test.go`, `smoke_test.go`)
- Kaynak: `internal/tui/forum.go`, `internal/tui/taslak.go` ve testleri; `Eklentiler/konu_ac/main.go`
  (yalnızca okunur; kopyalanıp uyarlanır)

**Interfaces:**
- Consumes: Task 4 Client yöntemleri ve alanları; `protokol` kategori listesi (`forum.Categories` yerine;
  eklenti `internal/forum`'u içe aktarmaz).
- Produces: `type model struct` (ekranlar `ekKategoriler`, `ekKonular`, `ekKonu`, `ekYanit`, `ekProfil`,
  `ekYeniKonu`, `ekSilOnay`); `func newModel(c istemci, dataDir string, ben pluginapi.IdentityInfo, rol pluginapi.RoleInfo) model`
  — `istemci`, testte sahtelenebilen küçük bir arayüzdür (kullanılan Client yöntemleri).
  `func degisimBekle(c istemci, since uint64) tea.Cmd` → `degisimMsg{ForumChanges, err}`; model her
  mesajdan sonra bir sonrakini kurar (hata olursa 5 sn sonra yeniden).
- Taslaklar: yanıt taslakları `filepath.Join(dataDir, "yanit_taslaklari.json")` bugünkü `internal/tui/taslak.go`
  biçimiyle; konu taslakları `taslaklar_<kategori>.json` konu_ac biçimiyle. Bozuk/eksik dosya = boş, hata yok.
  Kaydetme en çok 2 sn'de bir ve çıkışta.
- Profil ekranı: `Client.Profile`; yasak tuşu yok; kullanıcı kurucu/moderatörse "Yasaklamak için Yönetici
  Paneli'ni kullan" ipucu.
- Kendi konusu: Konu ekranında `x` → onay → `DeleteOwnPost`.

- [ ] **Step 1: Testleri yaz** (sahte `istemci` ile): TUI forum testlerinden taşınanlar (gezinme,
  yanıt taslağı geri gelir, çok uzun yanıt hatası sonrası metin düzenlenebilir, "r" kutuyu silmez),
  `TestTaslak_BozukDosyaCokmez`, `TestTaslak_EksikDosyaBos`, `TestDegisim_YeniKonuListeyiGunceller`,
  `TestDegisim_HataSonrasiYenidenDener`, `TestKendiKonusunuSil` (başkasının konusunda `x` hiçbir şey
  yapmaz), `TestProfil_YasakTusuYok_YetkiliyeIpucu`, `TestYeniKonu_KategoriTaslaklari` (konu_ac testlerinden),
  `TestEkranlar_HerDilde`, `TestManifest_WellFormed`, `TestCeviriKapsami`.
- [ ] **Step 2: Kırmızı:** `go test ./Eklentiler/forum/`.
- [ ] **Step 3: Uygula** (`internal/tui/forum.go`'dan taşı, `*forum.Forum` çağrılarını `istemci`'ye çevir;
  gövde çizimi bugünkü TUI'deki gibi `internal/bbcode` ile; metinler `ceviri.json`'a 5 dille).
- [ ] **Step 4: Yeşil:** `go test -count=1 ./Eklentiler/forum/`.
- [ ] **Step 5: Commit:** "Forum eklentisi: terminal arayüzü, canlı güncelleme ve taslaklar".

### Task 7: Forum eklentisi — masaüstü arayüzü

**Files:**
- Create: `Eklentiler/forum/arayuz.go`, `Eklentiler/forum/arayuz_test.go`
- Modify: `Eklentiler/forum/model.go` (`arayuzKipi`, `gorunur`, `surum int`; `Merhaba`/`Gorunurluk`/`Olay`/`Git`),
  `main.go` (`if arayuz.Etkin() { arayuz.Calistir(m) }`), `internal/masaustu/onizleme_test.go`
  (`ONIZLEME_ARAYUZ_AD=forum` önizleme forum verisiyle)

**Interfaces:**
- Consumes: Task 5 (`Gonderi`, `Menu`, `Git`), Task 6 modeli.
- Kimlikler: araçlar `kategori` (secim, "Tümü" + kategoriler, Aninda), `yeniKonu` (Birincil, `n`); liste
  `konular` (alt: yazar · kategori; sağ: son etkinlik; rozet: yanıt sayısı); konu ekranı `konu` (gonderi),
  yanıtlar `y:<id>` (gonderi, YanitGonderisi), `dahaEski` (düğme), form `yanitFormu` (`yanit` metinAlani —
  değeri taslak; `gonder` Gonderir, Ctrl+Enter), kendi konusunda araç `sil` (Tehlikeli + onay); yeni konu
  formu `konuFormu` (`kategori`, `baslik`, `govde`) ve `taslakListesi`; profil `profil` (kart + anahtarDeger).
- Menü: `Surum >= 2` iken her veri değişiminde, yalnızca konusu olan kategoriler:
  `MenuOgesi{ID: "kategori:"+ad, Etiket: dil.T(ad), Ikon: "forum", Sayi: n}`.
- Git: `kategori:<ad>` → liste süzülür; `konu:<id>` → konu açılır (yoksa listede kalıp bildirim).
- Yanıt sayfalama: varsayılan son 200 yanıt; `dahaEski` her basışta 200 daha.
- `Surum < 2`: `gonderi` yerine `kart` + `zengin`; menü yok.
- Konu yalnızca görünürken yenilenir; liste her zaman güncel tutulur.

- [ ] **Step 1: Testleri yaz:** `TestArayuz_ListeVeKategoriSuzgeci`, `TestArayuz_KonuAcVeYanitGonder`
  (gönderilen metin `CreateReply`'a gider, taslak silinir), `TestArayuz_YanitYazarkenYeniYanitMetniKorur`
  (`Degisti("yanit","yarım")` → değişim mesajı → `yanit` alanının değeri hâlâ "yarım"),
  `TestArayuz_GitKonu`, `TestArayuz_GitKategori`, `TestArayuz_MenuKategoriler`, `TestArayuz_Surum1deGonderiVeMenuYok`,
  `TestArayuz_YeniKonuTaslak`, `TestArayuz_KendiKonusunuSil`, `TestArayuz_CokYanitSayfalanir` (2500 yanıt,
  64 KiB gövde → `Denetle` geçer, 200 `y:` öğesi, `dahaEski` 400 yapar), `TestArayuz_HerDilde`.
- [ ] **Step 2: Kırmızı:** `go test ./Eklentiler/forum/ -run TestArayuz`.
- [ ] **Step 3: Uygula.**
- [ ] **Step 4: Yeşil + görsel:** `go test -count=1 ./Eklentiler/forum/ ./internal/masaustu/`; forum exe'sini
  scratchpad'e derle (`go build -o <scratchpad>/and-plugin-forum.exe ./Eklentiler/forum`), önizlemeyi
  `ONIZLEME_ARAYUZ=<o exe> ONIZLEME_ARAYUZ_AD=forum` ile aç; liste, konu, yeni konu, uzun konu ekran
  görüntülerini "önce" görüntüleriyle karşılaştır; fark varsa CSS/eşleme düzelt.
- [ ] **Step 5: Commit:** "Forum eklentisi: AND masaüstünde arayüzle".

### Task 8: Çekirdekten forum ekranlarının kaldırılması ve bütünleştirme

**Files:**
- Delete: `internal/tui/forum.go` (forumModel ve görünümleri), `internal/tui/taslak.go`, `internal/tui/taslak_test.go`
  (testler Task 6'da taşındı)
- Create: `internal/tui/ban.go` (`banDurationPreset`, `banDurationPresets`, `banExpiry`, `issueBanCommon`,
  `issueUnbanCommon`, `banOnayGorunumu`, `unbanOnayGorunumu` — sohbetteki profil kullanır),
  `internal/tui/zaman.go` (`forumNeZaman`, `forumKalanSure`, `yerelTarih`, `forumKisalt` gibi forum dışında
  kullanılanlar), `profilMetni` `internal/tui/profil.go`'ya
- Modify: `internal/tui/app.go` (`screenForum` ve "Forum" dalı kalkar; Topluluk dalı = forum eklentisi
  (varsa ve etkinse) + Sohbet; forum eklentisi kategori dallarında tekrar görünmez), `internal/tui/home.go`
  (seçili öğe forum eklentisiyken kategori çubukları; gizli konu_ac açıklaması), `internal/tui/dil_test.go`;
  `internal/masaustu/api.go` (`GET /api/forum/konu`, `POST /api/forum/konu`, `POST /api/forum/yanit`,
  `konuAcabilir` kalkar; `GET /api/forum` Ctrl+K için kalır), `internal/masaustu/cekirdek.go` (kullanılmayan
  `forum` SSE olayı kalkar), `uygulama.js` (forum görünümü, konu listesi/detayı, yeni konu diyaloğu,
  `forumOlayi` kalkar; Topluluk grubunda forum eklentisi girdisi + menüsü; `varsayilanGorunum()` = forum
  eklentisi menüde ise `eklenti/forum`, değilse `sohbet`; `#forum` ve `#forum/<id>` → `eklenti/forum` +
  `git konu:<id>`; `git("forum")` çağrıları `varsayilanGorunum()`'e), `uygulama.css` (kullanılmayan forum
  stilleri; `gonderi`'nin kullandıkları kalır), `metinler.go` (yeniden üret), `onizleme_test.go`,
  `masaustu_test.go`; paketleme: `internal/pluginmgr` (AND'la gelenler), `.github/workflows/release.yml`,
  `.gitignore`; belgeler: `EKLENTILER.md`, `PLUGINS.md`, `README.md`, `README.tr.md`, `CONTRIBUTING.md`
  (konu_ac → forum); `Eklentiler/site/model.go`'daki konu_ac atfı
- konu_ac deposu (`Eklentiler/konu_ac`, ayrı git deposu): `README.md` ve `README.tr.md` başına "Bu eklenti
  AND'ın Forum eklentisine katıldı" notu; o depoda ayrı commit

**Interfaces:**
- Consumes: Task 5 menü/git uçları, Task 6–7 eklentisi.

- [ ] **Step 1: Testleri yaz/güncelle:** `TestMenu_ForumEklentisiToplulukta`, `TestMenu_ForumYoksaGirdiYok`
  (devre dışı ya da kurulu değil), `TestMenu_ForumKategoriDalindaTekrarlanmaz`, `TestHome_ForumEklentisiSecilinceKategoriler`;
  masaüstü: `TestApi_EskiForumUclariYok` (konu/yanıt uçları 404/405), `TestEklentiler_ForumAndlaGelir`
  (AND'la gelenlerde `forum` var ve ilk kurulumda etkin, `konu_ac` yok; kategori testi), dil kapsam testi.
  İzin: forum ilk açılışta standart izin akışıyla (masaüstü `izinSor`, TUI `izin.go`) bir kez sorar.
- [ ] **Step 2: Kırmızı:** `go test ./internal/tui/ ./internal/masaustu/ ./internal/pluginmgr/`.
- [ ] **Step 3: Uygula;** sonra `git grep -n 'konu_ac\|forumModel\|konuAcabilir\|/api/forum/konu'` yalnızca
  bilinçli kalanları (geçmiş belgeler, konu_ac deposu) göstermeli. Kullanılmayan çekirdek dil metinlerini sil.
- [ ] **Step 4: Yeşil + görsel:** `go test -count=1 ./...`; önizlemede (forum eklentisi exe'siyle) açılışın
  forum olduğu, kenar çubuğu kategorileri, Ctrl+K konu araması, `#forum/<id>` yönlendirmesi, forum
  kapalıyken Sohbet'e düşüş, Site ve Yönetici Paneli ekranları — ekran görüntüleriyle.
- [ ] **Step 5: Commit'ler:** AND: "Forum çekirdekten eklentiye taşındı; konu_ac foruma katıldı";
  konu_ac deposu: "README: Forum eklentisine katıldı".

### Task 9: Son doğrulama

- [ ] gofmt (LF klon: `core.autocrlf false` + `core.eol lf` + yeniden checkout), `go vet` (windows/linux/darwin),
  `go test -count=1 ./...` (CRLF çalışma ağacı ve LF klon), `bash .github/scripts/govulncheck.sh`,
  `go test ./internal/lisans/`.
- [ ] Bütün dalın gözden geçirmesi (code-reviewer.md'ye göre; ayrı ajan kullanıcı istemedikçe yok →
  öz-inceleme, kullanıcıya söylenir), bulgular için tek düzeltme geçişi (RED→GREEN).
- [ ] Kullanıcıya exe yeniden derleme onayı sorulur: `and.exe`, `and-plugin-forum.exe`, `and-plugin-site.exe`,
  `and-plugin-admin.exe` (forum exe'si olmadan kullanıcının kendi kurulumunda forum görünmez).
- [ ] Bellek: oturum kaydı ve `and-masaustu-arayuzu` güncellenir.
