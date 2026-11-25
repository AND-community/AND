# VS Code uyumu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** and-vscode 0.10.0: AND'daki forum yetenekleri Canlı Panel'de, AND Ayarlar'ında kalıcı "VS Code bağlantısı" anahtarı, eksik tema adları.

**Architecture:** AND tarafında `pluginapi` harici dinleyiciyi bir ayar dosyasından da açar ve çalışırken açıp kapatabilir; masaüstü ve terminal Ayarlar'ı bunu kullanır. VS Code tarafında forum verisi/izleme mantığı `src/live/forum/` altında saf modüllerde, Forum sekmesinin HTML/CSS/betiği ayrı bir modülde; `livePanel.ts` iletileri bağlar.

**Tech Stack:** Go 1.26 (pluginapi, internal/masaustu, internal/tui), TypeScript 5 + node:test (and-vscode), esbuild, vsce.

**Spec:** docs/superpowers/specs/2026-11-25-vscode-uyum-design.md

## Global Constraints

- Every user-visible Go string goes through dil (dil.T/dil.K/dil.F) with en/de/es/fr/ru in `dil/ceviriler/tui.json`; never `|` inside a Go dil string.
- Every and-vscode string goes through t/k/h/j with all 5 translations in `and-vscode/src/dil/ceviriler.json` (plural `|` allowed there, per kapsam.test.ts rules).
- Forum category values sent to AND stay Turkish; only display is translated.
- After JS string changes in `internal/masaustu/web/uygulama.js`, regenerate `internal/masaustu/metinler.go` (extractor regex `\b(?:t|tf|tb|K)\("…"`), enforced by `TestArayuzMetinleri`.
- Never run a bare `go build ./<main pkg>`; use `go vet` / `go build -o NUL`.
- No push. Exe rebuilds and installing the vsix into VS Code need fresh user approval.
- Setting file: `harici_baglanti.json`, JSON `{"acik": bool}`, mode 0600, atomic write.
- Ban presets: 1 saat=3600, 1 gün=86400, 7 gün=604800, 30 gün=2592000 seconds, kalıcı=`permanent:true`; reason required.

## Review Focus

- Live update arriving while the user types a reply: the reply text must survive the re-render.
- Old AND without `/forum/posts` or `/forum/changes` (404): panel must not show an error loop; falls back as the spec says.
- Toggling the setting off while VS Code is connected: listener closes, VS Code shows "bağlı değil" without crashing; toggling on again works (Windows named pipe re-created).
- Env var `AND_ENABLE_EXTERNAL_API=1` plus UI "off": refused with a clear message, toggle shown locked.
- Topic/author fields with HTML (`<script>`, quotes) and `[code]` bodies: always escaped.

---

### Task 1: pluginapi — persisted, runtime-togglable external listener

**Files:**
- Create: `pluginapi/harici.go`, `pluginapi/harici_test.go`
- Modify: `pluginapi/api.go` (Start, Server fields), `pluginapi/externalpipe_windows_test.go`, `pluginapi/externalpipe_unix_test.go` (dial helper)

**Interfaces:**
- Produces: `const HariciAyarDosyasi = "harici_baglanti.json"`; `func HariciAyari(dataDir string) bool`; `type HariciDurum struct{ Acik, Ortamdan bool }`; `func (s *Server) HariciDurumu() HariciDurum`; `func (s *Server) HariciAyarla(acik bool) error`; `var ErrHariciOrtamda, ErrHariciKullanilamaz error`.

- [ ] Step 1: tests in `harici_test.go`:
  - `TestHariciAyari_DosyaYokKapali` (no file → false), `TestHariciAyari_KaydetOku` (save true → true, file mode 0600 on unix only), `TestHariciAyari_BozukDosyaKapali`.
  - `TestHariciAyarla_CalisirkenAcKapa`: Start with temp dataDir (env unset) → `HariciDurumu().Acik == false` and dial fails; `HariciAyarla(true)` → GET `/api/v1/version` over the pipe/socket returns 200 and `HariciAyari(dir)==true`; `HariciAyarla(false)` → dial/GET fails and setting false; `HariciAyarla(true)` again → 200.
  - `TestStart_AyarAciksaHariciAcilir`: write setting true before Start → GET 200, `Acik==true`, `Ortamdan==false`.
  - `TestHariciAyarla_OrtamdanKapatilamaz`: `t.Setenv("AND_ENABLE_EXTERNAL_API","1")`, Start → `Ortamdan==true`; `HariciAyarla(false)` → `errors.Is(err, ErrHariciOrtamda)`, still reachable.
  - `TestHariciAyarla_HariciKapatildiysa`: `HariciKapat()` before Start → `HariciAyarla(true)` → `ErrHariciKullanilamaz`.
  - Dial helper `hariciIstemci(dataDir string) *http.Client` in each platform test file.
- [ ] Step 2: run `go test ./pluginapi -run 'Harici|AyarAciksa' -count=1` → FAIL (undefined).
- [ ] Step 3: implement: move the listener start/stop into `hariciBaslat()`/`hariciDurdur()` under `hariciMu sync.Mutex`; Start uses `enableExternalAPI() || HariciAyari(dataDir)`; ctx.Done stops via `hariciDurdur`.
- [ ] Step 4: run `go test ./pluginapi -count=1` → PASS; `go vet ./pluginapi`, `GOOS=linux go vet ./pluginapi`.
- [ ] Step 5: commit "Eklenti API'si: VS Code bağlantısı ayardan açılır, çalışırken açılıp kapanır".

### Task 2: AND Ayarlar — desktop card, terminal key, wiring, docs

**Files:**
- Modify: `internal/masaustu/cekirdek.go` (field `Harici HariciDenetim`), `internal/masaustu/api.go` (routes), new handler file `internal/masaustu/harici.go`, `internal/masaustu/web/uygulama.js` (Eklentiler tab card), `internal/masaustu/web/uygulama.css` if needed, `internal/masaustu/metinler.go` (regenerated), `internal/masaustu/masaustu_test.go` (endpoint tests), `internal/masaustu/onizleme_test.go` (fake toggle so preview shows the card)
- Modify: `internal/tui/app.go` (Eklentiler tab line, `v` key, hint), `internal/tui/app_test.go`
- Modify: `cmd/and/main.go` (wire `apiSrv`), `dil/ceviriler/tui.json`, `README.md`, `README.tr.md`, `SECURITY.md`

**Interfaces:**
- Consumes: Task 1 `HariciDurum`, `HariciDurumu`, `HariciAyarla`, errors.
- Produces: `type HariciDenetim interface{ HariciDurumu() pluginapi.HariciDurum; HariciAyarla(bool) error }` in masaustu; `tui.HariciBaglanti HariciDenetim` package var (nil → line hidden); desktop `GET /api/harici` → `{"var":bool,"acik":bool,"ortamdan":bool}`, `POST /api/harici {"acik":bool}` → same shape or 409 with translated error.

- [ ] Step 1: tests: `TestHarici_DurumVeAyar` (fake denetim: GET reports, POST toggles), `TestHarici_OrtamdanReddedilir` (fake returns ErrHariciOrtamda → 409 + message), `TestHarici_YoksaVarFalse` (nil Harici → `var:false`); TUI `TestAyarlar_VSCodeBaglantisiTusu` (fake: `v` on Eklentiler tab toggles and shows notice; line hidden when nil).
- [ ] Step 2: run them → FAIL.
- [ ] Step 3: implement handlers, UI card (desktop: first block of Eklentiler tab, h3 "VS Code bağlantısı", paragraph, anahtar; ortamdan → disabled + note), TUI line + key; wire in main; translations; regenerate metinler.go.
- [ ] Step 4: `go test ./internal/masaustu ./internal/tui ./dil/... ./cmd/and -count=1` → PASS; vet ×3 OS.
- [ ] Step 5: docs; commit "Ayarlar: VS Code bağlantısı anahtarı (masaüstü ve terminal)".

### Task 3: and-vscode — API types, client, forum data and live watcher

**Files:**
- Modify: `and-vscode/src/live/liveTypes.ts` (PostSummary optional `created_at`, `last_activity`, `reply_count`, `author_role`; `ForumChanges`, `ProfileInfo`, `ProfileBan`, `BanReq`), `and-vscode/src/live/liveClient.ts`
- Create: `and-vscode/src/live/forum/forumVeri.ts`, `and-vscode/src/live/forum/forumVeri.test.ts`, `and-vscode/src/live/forum/forumIzleyici.ts`, `and-vscode/src/live/forum/forumIzleyici.test.ts`
- Modify: `and-vscode/package.json` test script (add `out-test/live/forum/*.test.js`)

**Interfaces:**
- Produces: client `listPosts(): Promise<PostSummary[]>` (GET `/api/v1/forum/posts`), `forumChanges(since: number): Promise<ForumChanges>` (GET `/api/v1/forum/changes?since=`, timeout 40 s), `deleteOwnPost(id)` (POST `/api/v1/forum/delete-own` `{post_id}`), `getProfile(key)` (GET `/api/v1/profile?key=`), `ban(req: BanReq)` (POST `/api/v1/mod/ban`).
- `forumVeri.ts`: `siralaKonular(p: PostSummary[]): PostSummary[]`; `kategoriSayilari(p): {kategori: string; sayi: number}[]` (FORUM_CATEGORIES order, only >0); `konuListesiYukle(c, rol): Promise<{konular: PostSummary[]; liste: boolean}>` (404 on /posts → mods: getAllPosts with `liste:true`; others `{konular:[], liste:false}`).
- `forumIzleyici.ts`: `class ForumIzleyici { constructor(c: {forumChanges(since:number):Promise<ForumChanges>}, degisti: (d: {konular: string[]; hepsi: boolean}) => void, bekle?: (ms:number)=>Promise<void>); baslat(): void; durdur(): void }` — first call `since=0` only records `surum`; later results with konular/hepsi call `degisti`; 404 stops; other errors back off 5 s.

- [ ] Step 1: tests: sorting (last_activity, fallback created_at, missing both last), category counts order, list fallback for member/mod on 404 (LiveApiError status 404), watcher: first response not reported, change reported with ids, `hepsi` reported, 404 stops the loop, error → waits then retries, `durdur` ends loop.
- [ ] Step 2: `npm test` → FAIL.
- [ ] Step 3: implement.
- [ ] Step 4: `npm run typecheck && npm test` → PASS.
- [ ] Step 5: commit "and-vscode: forum listesi, canlı değişim izleyici, profil ve yasaklama istemcisi".

### Task 4: and-vscode — Forum tab UI, panel wiring, theme names

**Files:**
- Create: `and-vscode/src/live/forum/forumHtml.ts` (exports `forumCss()`, `forumHtml(h)`, `forumBetigi(j)` used by renderHtml)
- Modify: `and-vscode/src/live/panelHtml.ts` (forum section via forumHtml, TEMA_ADLARI += Gül, Okyanus, Gün Batımı, Lavanta, Özel; message cases), `and-vscode/src/live/livePanel.ts` (handlers `forumLoad`, `forumOpen`, `createPost`, `reply`, `deletePost{own}`, `profile`, `ban`; watcher lifecycle), `and-vscode/src/live/panelHtml.test.ts` (fake DOM additions), `and-vscode/src/dil/ceviriler.json`

**Interfaces:**
- Consumes: Task 3 client/forumVeri/ForumIzleyici.
- Webview→ext: `forumLoad`, `forumOpen{id}`, `createPost{category,title,body}`, `reply{postId,body}`, `deletePost{postId,own}`, `profile{key}`, `ban{key,seconds?,permanent?,reason}`.
- Ext→webview: `forumPosts{konular, liste}`, `postDetail{data}`, `postGone{id}`, `profile{data}`, `forumCreated{title}`, existing `info`/`error`.

- [ ] Step 1: tests (panelHtml.test.ts): list renders escaped titles + category filter counts; clicking a topic posts `forumOpen`; detail shows Sil only for own topic or moderator, posts `deletePost` with `own`; re-render of the same post keeps reply input value; `postGone` shows the gone note; `[code]` body rendered in `<pre>` and escaped; profile renders and ban form posts `ban` with preset seconds and refuses empty reason; `liste:false` shows the ID box; new-topic form validates and posts `createPost`; theme table has all 11 names in every language.
- [ ] Step 2: `npm test` → FAIL.
- [ ] Step 3: implement.
- [ ] Step 4: `npm run typecheck && npm test` → PASS.
- [ ] Step 5: commit "and-vscode: Forum sekmesi AND'daki gibi — konular, canlı, profil, yasaklama; yeni tema adları".

### Task 5: and-vscode 0.10.0 — docs, version, package

**Files:**
- Modify: `and-vscode/README.md`, `and-vscode/CHANGELOG.md`, `and-vscode/package.json`, `and-vscode/package-lock.json` (version), status texts mentioning only the env var (`extension.ts:314`, `editorActions.ts:19`, `siteFileSystem.ts:301`, `liveConnection.ts` comment) → mention the Settings switch.

- [ ] Step 1: update texts + translations; `npm run typecheck && npm test` → PASS.
- [ ] Step 2: `npx vsce package` → `and-vscode-0.10.0.vsix` created.
- [ ] Step 3: full AND verification: `go vet ./...`, `go test -count=1 ./...`, GOOS=linux/darwin vet.
- [ ] Step 4: commit "and-vscode 0.10.0: AND'la uyumlu forum, bağlantı ayarı, yeni temalar".
