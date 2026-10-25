// Canlı Panel'in webview HTML'i ve AND temasının webview'a çevrilmesi. "vscode"
// modülüne BAĞIMLI DEĞİLDİR — bu sayede panelin betiği VS Code dışında, sahte
// bir DOM üzerinde birim testiyle (panelHtml.test.ts) sınanabilir.

import * as crypto from "crypto";
import { FORUM_CATEGORIES, ThemeReq } from "./liveTypes";
import { ansiOrHexToCss, readableTextOn } from "./themeColor";
import { betikDegeri, bicimle, cogulBicimi, etkin, ham, k, t } from "../dil/dil";

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

/**
 * ThemeReq'teki ("63" ya da "#ff6600") renkleri webview'a gönderilecek CSS hex
 * koduna çevirir. accentFg, vurgu renginde zeminli düğmelerin yazı rengidir —
 * temaya göre siyah ya da beyaz seçilir (açık bir vurguda beyaz yazı okunmaz).
 */
export function resolveThemeColors(theme: ThemeReq): {
  themeName?: string;
  accent?: string;
  accentFg?: string;
  selBg?: string;
  selFg?: string;
  muted?: string;
  badge?: string;
} {
  const accent = ansiOrHexToCss(theme.accent);
  return {
    themeName: theme.theme_name,
    accent,
    accentFg: readableTextOn(accent),
    selBg: ansiOrHexToCss(theme.sel_bg),
    selFg: ansiOrHexToCss(theme.sel_fg),
    muted: ansiOrHexToCss(theme.muted),
    badge: ansiOrHexToCss(theme.badge),
  };
}

// İkonlar: tek çizgi kalınlığında, currentColor ile boyanan küçük SVG'ler —
// webview'a dış bir yazı tipi/paket yüklemeden (CSP'yi gevşetmeden) VS Code'un
// kendi ikon diliyle uyumlu bir görünüm verir.
const ICONS: Record<string, string> = {
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4 3.6-6.5 8-6.5s8 2.5 8 6.5"/>',
  network:
    '<circle cx="12" cy="5" r="2.5"/><circle cx="5" cy="18" r="2.5"/><circle cx="19" cy="18" r="2.5"/><path d="M10.8 7.2 6.2 15.8M13.2 7.2l4.6 8.6M7.5 18h9"/>',
  chat: '<path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z"/>',
  mail: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/>',
  forum:
    '<path d="M14 9a2 2 0 0 1-2 2H7l-4 3V5a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2z"/><path d="M18 9h1a2 2 0 0 1 2 2v9l-4-3h-5a2 2 0 0 1-2-2v-1"/>',
  globe:
    '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.7 3.8 5.7 3.8 9s-1.3 6.3-3.8 9c-2.5-2.7-3.8-5.7-3.8-9S9.5 5.7 12 3z"/>',
  file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  refresh: '<path d="M20 11a8 8 0 0 0-14.9-3.5M4 4v4h4M4 13a8 8 0 0 0 14.9 3.5M20 20v-4h-4"/>',
};

function icon(name: string): string {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name] ?? ""}</svg>`;
}

/** AND'ın hazır tema adları (internal/ui/theme.go); panelin alt köşesinde etkin dilde gösterilir. */
const TEMA_ADLARI = [k("Klasik"), k("Turkuaz"), k("Gece Moru"), k("Kehribar"), k("Orman"), k("Mono")];

/**
 * Canlı Panel'in HTML'i. Görsel dil iki katmanlıdır: yüzeyler, metin ve form
 * öğeleri VS Code'un KENDİ belirteçlerinden gelir (panel her VS Code temasında
 * yerli görünür); AND'ın etkin teması (/api/v1/theme → "theme" mesajı) ise
 * yalnızca marka işaretinde, etkin gezinme öğesinde ve birincil düğmelerde
 * kullanılır — TUI'de seçilen tema burada da hissedilir. Test/ekran görüntüsü
 * için dışa açıktır.
 */
export function renderHtml(webview: { cspSource: string }): string {
  const nonce = crypto.randomBytes(16).toString("hex");
  // Kategori değerleri ağ protokolünün parçasıdır ve Türkçe gönderilir; yalnızca görünen ad çevrilir.
  const categoryOptions = FORUM_CATEGORIES.map((c) => `<option value="${escapeHtml(c)}">${escapeHtml(t(c))}</option>`).join(
    ""
  );
  /** HTML'e yazılacak çeviri. */
  const h = (kaynak: string): string => escapeHtml(t(kaynak));
  /**
   * Betiğe dize değişmezi olarak gömülecek çeviri. Tekil/çoğul biçimlerinin
   * hepsi gömülür; biçimi ve fiilleri betikteki tf/th sayıya göre seçip doldurur.
   */
  const j = (kaynak: string): string => betikDegeri(ham(kaynak));
  const ceviriTablosu = (kaynaklar: readonly string[]): string =>
    betikDegeri(Object.fromEntries(kaynaklar.map((k) => [k, t(k)])));

  return `<!doctype html>
<html lang="${etkin()}">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
<style>
  :root {
    --and-accent: #5f5fff; --and-accent-fg: #ffffff; --and-selbg: #5f00ff; --and-selfg: #eeeeee; --and-badge: #0087ff;
    --line: var(--vscode-widget-border, rgba(128,128,128,0.22));
    --muted: var(--vscode-descriptionForeground, #9d9d9d);
  }
  * { box-sizing: border-box; }
  html, body { height: 100%; }
  body { margin: 0; font: 13px/1.5 var(--vscode-font-family); color: var(--vscode-foreground); background: var(--vscode-editor-background); }
  .app { display: grid; grid-template-columns: 188px 1fr; height: 100vh; }

  .rail { display: flex; flex-direction: column; border-right: 1px solid var(--line); background: var(--vscode-sideBar-background, transparent); padding: 14px 0 12px; }
  .brand { display: flex; align-items: center; gap: 10px; padding: 0 12px 16px 14px; }
  .mark { font-family: var(--vscode-editor-font-family); font-weight: 700; letter-spacing: 0.04em; background: var(--and-accent); color: var(--and-accent-fg); padding: 1px 7px; border-radius: 4px; }
  .conn { font-size: 12px; color: var(--muted); display: inline-flex; align-items: center; gap: 6px; }
  .conn::before { content: ""; width: 7px; height: 7px; border-radius: 50%; background: var(--vscode-charts-yellow, #cca700); }
  .conn.ok::before { background: var(--vscode-charts-green, #89d185); }
  .icon-btn { margin-left: auto; display: grid; place-items: center; width: 26px; height: 26px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--muted); cursor: pointer; }
  .icon-btn svg { width: 15px; height: 15px; }
  .icon-btn:hover { background: var(--vscode-toolbar-hoverBackground, var(--vscode-list-hoverBackground)); color: var(--vscode-foreground); }
  #nav button { display: flex; align-items: center; gap: 10px; width: 100%; padding: 7px 14px 7px 11px; border: 0; border-left: 3px solid transparent; background: none; color: var(--vscode-foreground); font: inherit; text-align: left; cursor: pointer; }
  #nav button svg { width: 16px; height: 16px; flex: none; opacity: 0.75; }
  #nav button:hover { background: var(--vscode-list-hoverBackground); }
  #nav button.active { background: var(--and-selbg); color: var(--and-selfg); border-left-color: var(--and-accent); }
  #nav button.active svg { opacity: 1; }
  .count { margin-left: auto; min-width: 18px; padding: 0 6px; border-radius: 9px; font-size: 11px; text-align: center; background: var(--vscode-badge-background); color: var(--vscode-badge-foreground); }
  .count:empty { display: none; }
  .rail-foot { margin-top: auto; padding: 12px 14px 0; font-size: 12px; color: var(--muted); }

  main { overflow-y: auto; padding: 22px 28px 48px; }
  section { display: none; max-width: 760px; }
  section.active { display: block; }
  .sec-head { margin-bottom: 18px; }
  h1 { font-size: 16px; font-weight: 600; margin: 0 0 2px; }
  h2 { font-size: 13px; font-weight: 600; margin: 26px 0 8px; }
  h3 { font-size: 12px; font-weight: 600; margin: 18px 0 4px; color: var(--muted); }
  .lede { margin: 0; color: var(--muted); }
  .muted { color: var(--muted); }
  .mono { font-family: var(--vscode-editor-font-family); font-size: 12px; }
  .grow { flex: 1; min-width: 0; }

  .kv { display: grid; grid-template-columns: 128px minmax(0, 1fr); gap: 10px 16px; margin: 0; }
  .kv dt, .kv dd { min-height: 26px; display: flex; align-items: center; }
  .kv dt { color: var(--muted); }
  .kv dd { margin: 0; gap: 6px; min-width: 0; }
  .kv dd .mono { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .kv .icon-btn { margin-left: 0; }
  .role { font-weight: 600; color: var(--and-badge); }

  .list { list-style: none; margin: 0; padding: 0; border-top: 1px solid var(--line); }
  .list > li { display: flex; align-items: flex-start; gap: 12px; padding: 10px 2px; border-bottom: 1px solid var(--line); }
  .list:empty { border-top: 0; }
  .list:empty::before, .stream:empty::before { content: attr(data-empty); display: block; padding: 6px 0 14px; color: var(--muted); }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; flex: none; }
  .preview { margin: 4px 0; color: var(--muted); white-space: pre-line; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .title-link { font-weight: 600; cursor: pointer; }
  .post { margin-top: 16px; border: 1px solid var(--line); border-radius: 6px; padding: 14px 16px; }
  .post h2 { margin: 0 0 2px; font-size: 14px; }
  .post > .body { margin: 12px 0 4px; }
  .post > p { margin: 0; }
  .post .replies { margin-top: 6px; }
  .post .replies > li:last-child { border-bottom: 0; }
  .title-link:hover { text-decoration: underline; }

  .stream { list-style: none; margin: 0 0 14px; padding: 0; max-height: 56vh; overflow-y: auto; }
  .msg { display: grid; grid-template-columns: 28px minmax(0, 1fr); column-gap: 10px; padding: 6px 0; }
  .avatar { grid-row: span 2; width: 28px; height: 28px; border-radius: 50%; display: grid; place-items: center; font-size: 12px; font-weight: 600; background: hsl(var(--h) 38% 27%); color: hsl(var(--h) 70% 86%); }
  .who { font-weight: 600; color: hsl(var(--h) 60% 72%); }
  .msg .who { margin-right: 8px; }
  .msg.own .who { color: var(--and-accent); }
  .msg time { font-size: 11px; color: var(--muted); }
  .msg .text, .post .body, .replies .body { white-space: pre-wrap; overflow-wrap: anywhere; }
  body.vscode-light .avatar { background: hsl(var(--h) 60% 88%); color: hsl(var(--h) 55% 28%); }
  body.vscode-light .who, body.vscode-light .msg.own .who { color: hsl(var(--h) 55% 32%); }

  .msg .state { font-size: 11px; margin-left: 6px; color: var(--muted); }
  .msg .state.err { color: var(--vscode-errorForeground, #f14c4c); }

  .dm { display: grid; grid-template-columns: minmax(170px, 240px) minmax(0, 1fr); gap: 20px; }
  .convs { list-style: none; margin: 10px 0 0; padding: 0; max-height: 60vh; overflow-y: auto; border-top: 1px solid var(--line); }
  .convs:empty::before { content: attr(data-empty); display: block; padding: 8px 0; color: var(--muted); }
  .conv { padding: 7px 8px 7px 6px; border-bottom: 1px solid var(--line); border-left: 3px solid transparent; cursor: pointer; }
  .conv:hover { background: var(--vscode-list-hoverBackground); }
  .conv.sel { background: var(--vscode-list-inactiveSelectionBackground, var(--vscode-list-hoverBackground)); border-left-color: var(--and-accent); }
  .conv-top { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .conv-top .who { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .conv-prev { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .dm-head { padding-bottom: 8px; margin-bottom: 4px; border-bottom: 1px solid var(--line); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .dm-head .mono { margin-left: 6px; }

  .composer { display: flex; gap: 8px; align-items: center; }
  .panel { border: 1px solid var(--line); border-radius: 6px; padding: 14px 16px; }
  .field { display: grid; gap: 4px; margin-bottom: 12px; }
  label { font-size: 12px; color: var(--muted); }
  input[type=text], textarea, select { width: 100%; padding: 6px 9px; border: 1px solid var(--vscode-input-border, var(--line)); border-radius: 4px; background: var(--vscode-input-background); color: var(--vscode-input-foreground); font: inherit; }
  input::placeholder, textarea::placeholder { color: var(--vscode-input-placeholderForeground, var(--muted)); }
  textarea { min-height: 90px; resize: vertical; }
  input:focus, textarea:focus, select:focus { outline: 1px solid var(--vscode-focusBorder); outline-offset: -1px; }
  .btn { flex: none; padding: 5px 12px; border: 1px solid transparent; border-radius: 4px; font: inherit; cursor: pointer; white-space: nowrap; }
  .btn.sm { padding: 2px 9px; font-size: 12px; }
  .btn.primary { background: var(--and-accent); color: var(--and-accent-fg); font-weight: 600; }
  .btn.primary:hover { filter: brightness(1.08); }
  .btn.primary:disabled { opacity: 0.45; cursor: default; filter: none; }
  .btn.secondary { background: var(--vscode-button-secondaryBackground); color: var(--vscode-button-secondaryForeground); }
  .btn.secondary:hover { background: var(--vscode-button-secondaryHoverBackground, var(--vscode-button-secondaryBackground)); }
  .btn.ghost { background: none; color: var(--vscode-foreground); border-color: var(--line); }
  .btn.ghost:hover { background: var(--vscode-list-hoverBackground); }
  .btn.danger:hover { color: var(--vscode-errorForeground, #f14c4c); border-color: currentColor; }
  button:focus-visible, .title-link:focus-visible { outline: 1px solid var(--vscode-focusBorder); outline-offset: 2px; }

  #banner { display: none; position: sticky; top: -22px; z-index: 2; margin: 0 0 16px; padding: 8px 12px; border-left: 3px solid; border-radius: 4px; }
  #banner.show { display: block; }
  #banner.ok { background: var(--vscode-inputValidation-infoBackground); color: var(--vscode-inputValidation-infoForeground, inherit); border-color: var(--and-accent); }
  #banner.err { background: var(--vscode-inputValidation-errorBackground); color: var(--vscode-inputValidation-errorForeground, inherit); border-color: var(--vscode-inputValidation-errorBorder, #be1100); }

  @media (max-width: 560px) {
    .app { grid-template-columns: 52px 1fr; }
    #nav button span, .rail-foot { display: none; }
    #nav button { justify-content: center; padding: 9px 0; }
    .brand { flex-direction: column; padding: 0 0 14px; }
    .conn { font-size: 0; }
    .icon-btn { margin: 0; }
    main { padding: 18px 16px 40px; }
    .kv { grid-template-columns: 1fr; gap: 2px 0; }
    .kv dt { min-height: 0; }
    .kv dd { margin-bottom: 10px; }
  }
  @media (max-width: 760px) {
    .dm { grid-template-columns: 1fr; gap: 12px; }
    .convs { max-height: 28vh; }
  }
</style>
</head>
<body>
<div class="app">
  <aside class="rail">
    <div class="brand">
      <span class="mark">AND</span>
      <span id="connBadge" class="conn">${h("bağlanıyor…")}</span>
      <button class="icon-btn" id="refreshBtn" title="${h("Yenile")}" aria-label="${h("Yenile")}">${icon("refresh")}</button>
    </div>
    <nav id="nav" aria-label="${h("Bölümler")}">
      <button data-tab="identity" class="active">${icon("user")}<span>${h("Kimlik")}</span></button>
      <button data-tab="peers">${icon("network")}<span>${h("Ağ")}</span><span id="peerCount" class="count"></span></button>
      <button data-tab="chat">${icon("chat")}<span>${h("Sohbet")}</span></button>
      <button data-tab="dm">${icon("mail")}<span>${h("Doğrudan mesaj")}</span><span id="dmCount" class="count"></span></button>
      <button data-tab="forum">${icon("forum")}<span>${h("Forum")}</span></button>
      <button data-tab="site">${icon("globe")}<span>${h("Site")}</span><span id="siteCount" class="count"></span></button>
      <button data-tab="files">${icon("file")}<span>${h("Dosyalar")}</span></button>
    </nav>
    <div class="rail-foot" id="themeName"></div>
  </aside>
  <main id="main">
    <div id="banner" role="status" aria-live="polite"></div>

    <section id="tab-identity" class="active">
      <div class="sec-head"><h1>${h("Kimlik")}</h1><p class="lede">${h("Bu AND düğümünün kimliği ve ağdaki rolün.")}</p></div>
      <dl id="identityBody" class="kv"><dt>${h("Durum")}</dt><dd class="muted">${h("Yükleniyor…")}</dd></dl>
    </section>

    <section id="tab-peers">
      <div class="sec-head"><h1>${h("Ağ")}</h1><p class="lede">${h("Şu an doğrudan bağlı olduğun eşler.")}</p></div>
      <ul id="peersBody" class="list" data-empty="${h("Bağlı eş yok. AND ağa bağlandıkça eşler burada görünür.")}"></ul>
    </section>

    <section id="tab-chat">
      <div class="sec-head"><h1>${h("Sohbet")}</h1><p class="lede">${h("Herkese açık genel kanal. Mesajlar imzalıdır ama şifreli değildir; ağdaki herkes okuyabilir.")}</p></div>
      <ul class="stream" id="chatList" data-empty="${h("Henüz mesaj yok. İlk mesajı sen yaz.")}"></ul>
      <div class="composer">
        <input type="text" id="chatInput" placeholder="${h("Genel kanala yaz…")}" />
        <button class="btn primary" id="chatSend">${h("Gönder")}</button>
      </div>
    </section>

    <section id="tab-dm">
      <div class="sec-head"><h1>${h("Doğrudan mesaj")}</h1><p class="lede" id="dmLede">${h("Uçtan uca şifreli; yalnızca karşı tarafa gider. Karşı taraf çevrimdışıysa AND mesajı bekletir ve bağlandığında iletir. Konuşmalar ve okunmamışlar AND ve Özel Chat ile ortaktır.")}</p></div>
      <div class="dm">
        <div>
          <div class="composer">
            <input type="text" id="dmNewPeer" class="mono" placeholder="${h("Yeni konuşma: Peer ID")}" list="peerIdsList" />
            <button class="btn ghost sm" id="dmNewBtn">${h("Aç")}</button>
          </div>
          <ul class="convs" id="dmConvs" data-empty="${h("Henüz konuşma yok.")}"></ul>
        </div>
        <div>
          <div class="dm-head muted" id="dmHead">${h("Bir konuşma seç ya da Peer ID ile yeni bir konuşma aç.")}</div>
          <ul class="stream" id="dmList" data-empty="${h("Bu konuşmada henüz mesaj yok.")}"></ul>
          <div class="composer">
            <input type="text" id="dmInput" placeholder="${h("Mesajını yaz…")}" />
            <button class="btn primary" id="dmSend">${h("Gönder")}</button>
          </div>
        </div>
      </div>
    </section>

    <section id="tab-forum">
      <div class="sec-head"><h1>${h("Forum")}</h1><p class="lede">${h("Yeni bir konu aç ya da ID'sini bildiğin bir konuyu görüntüle. Konular yayınlandığı anda ağa yayılır ve 5 gün sonra silinir.")}</p></div>
      <div class="panel">
        <div class="field"><label for="postCategory">${h("Kategori")}</label><select id="postCategory">${categoryOptions}</select></div>
        <div class="field"><label for="postTitle">${h("Başlık")}</label><input type="text" id="postTitle" /></div>
        <div class="field"><label for="postBody">${h("İçerik")}</label><textarea id="postBody"></textarea></div>
        <button class="btn primary" id="postCreate">${h("Konuyu yayınla")}</button>
      </div>

      <h2>${h("Konu görüntüle")}</h2>
      <div class="composer">
        <input type="text" id="viewPostId" class="mono" placeholder="${h("Konu ID")}" />
        <button class="btn secondary" id="viewPostBtn">${h("Göster")}</button>
      </div>
      <div id="postDetailBody"></div>

      <div id="modSection" style="display:none">
        <h2>${h("Moderasyon")}</h2>
        <p class="lede">${h("Ağdaki tüm konular. Silme, konuyu herkes için kaldırır.")}</p>
        <ul id="allPostsBody" class="list" data-empty="${h("Konu yok.")}"></ul>
      </div>
    </section>

    <section id="tab-site">
      <div class="sec-head"><h1>${h("Kişisel site")}</h1><p class="lede">${bicimle(h("Sayfaların VS Code editöründe açılır; %s. Paylaşım kodunu alan kişi sayfayı AND Arama Motoru'ndaki Kodla Git ekranında ya da %s bayrağıyla açabilir."), [`<strong>${h("kaydetmek (Ctrl+S) sayfayı ağa yayınlar")}</strong>`, `<span class="mono">-site</span>`])}</p></div>
      <div class="composer" style="margin-bottom:14px">
        <button class="btn primary" id="siteNewBtn">${h("Yeni sayfa")}</button>
        <button class="btn ghost" id="siteRefreshBtn">${h("Yenile")}</button>
      </div>
      <div id="siteBody"><p class="muted">${h("Yükleniyor…")}</p></div>
    </section>

    <section id="tab-files">
      <div class="sec-head"><h1>${h("Dosyalar")}</h1><p class="lede">${h("Bir eşe dosya gönder; gelen istekleri onayla. Alınan dosyalar AND'ın veri dizinine kaydedilir.")}</p></div>
      <div class="panel">
        <div class="field"><label for="filePeerId">${h("Alıcı")}</label><input type="text" id="filePeerId" class="mono" placeholder="Peer ID" list="peerIdsList" /></div>
        <datalist id="peerIdsList"></datalist>
        <button class="btn primary" id="fileSendBtn">${h("Dosya seç ve gönder")}</button>
      </div>
      <h2>${h("Bekleyen istekler")}</h2>
      <ul id="consentBody" class="list" data-empty="${h("Bekleyen istek yok.")}"></ul>
      <h2>${h("Alınan dosyalar")}</h2>
      <ul id="receivedFilesBody" class="list" data-empty="${h("Henüz dosya alınmadı.")}"></ul>
    </section>
  </main>
</div>
<script nonce="${nonce}">
  const vscode = acquireVsCodeApi();
  const banner = document.getElementById('banner');
  // Etkin dil ve çeviri yardımcıları: kurallar uzantının dil modülünden (src/dil/dil.ts) gömülür.
  const DIL = ${betikDegeri(etkin())};
  const cogulBicimi = ${cogulBicimi.toString()};
  const bicimle = ${bicimle.toString()};
  const KATEGORI = ${ceviriTablosu(FORUM_CATEGORIES)};
  const TEMA = ${ceviriTablosu(TEMA_ADLARI)};
  // tf: çevrilmiş kalıbı doldurur, tekil/çoğul biçimini ilk sayı argümanı seçer.
  function tf(kalip, ...args) {
    let c = kalip;
    if (c.includes('|')) {
      const n = args.find(a => typeof a === 'number');
      const b = c.split('|');
      c = b[Math.min(cogulBicimi(DIL, n === undefined ? 0 : n), b.length - 1)];
    }
    return bicimle(c, args);
  }
  // th: tf'nin HTML'e yazılan biçimi — kalıp kaçırılır, argümanlar zaten kaçırılmış HTML'dir.
  function th(kalip, ...args) { return tf(esc(kalip), ...args); }
  function kat(c) { return KATEGORI[c] || c; }
  const COPY_ICON = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/></svg>';
  let isModerator = false;
  let bannerTimer;

  function showBanner(text, ok) {
    banner.textContent = text;
    banner.className = 'show ' + (ok ? 'ok' : 'err');
    clearTimeout(bannerTimer);
    bannerTimer = setTimeout(() => { banner.className = ''; }, 4000);
  }

  // Tırnaklar da kaçırılır: bu çıktı data-*/value özniteliklerine de yazılıyor ve
  // değerlerin bir kısmı (transfer_id, sender_id, …) uzak eşlerden geliyor.
  function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  // Bir ada/anahtara her yerde AYNI kalan bir renk tonu — sohbette kimin yazdığı
  // tek bakışta ayırt edilsin diye (TUI'deki kişi renkleriyle aynı fikir).
  function hue(s) {
    let h = 2166136261;
    const str = String(s);
    for (let i = 0; i < str.length; i++) { h ^= str.charCodeAt(i); h = Math.imul(h, 16777619); }
    return (h >>> 0) % 360;
  }

  function fmtTime(ts) {
    const d = new Date(ts);
    return isNaN(d) ? '' : d.toLocaleTimeString(DIL, { hour: '2-digit', minute: '2-digit' });
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  }

  function bindCopy(root) {
    root.querySelectorAll('[data-copy]').forEach(b => b.addEventListener('click', () =>
      vscode.postMessage({ type: 'copyText', text: b.dataset.copy, label: b.dataset.label })));
  }

  let activeTab = 'identity';
  document.querySelectorAll('#nav button').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('#nav button').forEach(b => b.classList.remove('active'));
      document.querySelectorAll('section').forEach(s => s.classList.remove('active'));
      btn.classList.add('active');
      document.getElementById('tab-' + btn.dataset.tab).classList.add('active');
      activeTab = btn.dataset.tab;
      if (activeTab === 'dm') dmMarkRead();
    });
  });

  function onEnter(inputId, buttonId) {
    document.getElementById(inputId).addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); document.getElementById(buttonId).click(); }
    });
  }

  document.getElementById('refreshBtn').addEventListener('click', () => vscode.postMessage({ type: 'refreshAll' }));
  document.getElementById('chatSend').addEventListener('click', () => {
    const el = document.getElementById('chatInput');
    if (!el.value.trim()) return;
    vscode.postMessage({ type: 'sendChat', message: el.value });
    el.value = '';
  });
  onEnter('chatInput', 'chatSend');
  document.getElementById('dmSend').addEventListener('click', () => {
    const el = document.getElementById('dmInput');
    if (!dmSelected) { showBanner(${j("Önce bir konuşma seç ya da Peer ID ile yeni bir konuşma aç.")}, false); return; }
    if (!el.value.trim()) return;
    vscode.postMessage({ type: 'sendDM', peerId: dmSelected, message: el.value });
    el.value = '';
  });
  onEnter('dmInput', 'dmSend');
  document.getElementById('dmNewBtn').addEventListener('click', () => {
    const el = document.getElementById('dmNewPeer');
    const peer = el.value.trim();
    if (!peer) { showBanner(${j("Konuşmak istediğin kişinin Peer ID’sini gir.")}, false); return; }
    el.value = '';
    dmSelect(peer);
  });
  onEnter('dmNewPeer', 'dmNewBtn');
  document.getElementById('postCreate').addEventListener('click', () => {
    const category = document.getElementById('postCategory').value;
    const title = document.getElementById('postTitle').value.trim();
    const body = document.getElementById('postBody').value.trim();
    if (!title || !body) { showBanner(${j("Başlık ve içerik boş olamaz.")}, false); return; }
    vscode.postMessage({ type: 'createPost', category, title, body });
  });
  document.getElementById('viewPostBtn').addEventListener('click', () => {
    const id = document.getElementById('viewPostId').value.trim();
    if (!id) return;
    vscode.postMessage({ type: 'viewPost', id });
  });
  onEnter('viewPostId', 'viewPostBtn');
  document.getElementById('fileSendBtn').addEventListener('click', () => {
    const peerId = document.getElementById('filePeerId').value.trim();
    if (!peerId) { showBanner(${j("Önce alıcının Peer ID’sini gir.")}, false); return; }
    vscode.postMessage({ type: 'pickFileToSend', peerId });
  });
  document.getElementById('siteNewBtn').addEventListener('click', () => vscode.postMessage({ type: 'siteNew' }));
  document.getElementById('siteRefreshBtn').addEventListener('click', () => vscode.postMessage({ type: 'siteRefresh' }));

  // Geçmiş (chatHistory) ile canlı akış (chatMsg) aynı mesajı iki kez getirebilir;
  // zaman damgası iki yolda farklı saat dilimi ofsetiyle yazılabildiğinden metin
  // değil ayrıştırılmış an karşılaştırılır. Geçmiş listeyi SİLİP yeniden yazmaz,
  // mevcut mesajlarla BİRLEŞTİRİLİR: geçmiş hesaplandıktan sonra canlı gelen bir
  // mesaj (geçmişte henüz yoktur) aksi halde ekrandan kaybolurdu.
  const CHAT_DISPLAY_LIMIT = 500;
  const seenChat = new Set();
  let chatItems = [];
  function chatKey(m) { return JSON.stringify([m.from, Date.parse(m.sent_at), m.text]); }
  function addChat(m) {
    const key = chatKey(m);
    if (seenChat.has(key)) return false;
    seenChat.add(key);
    chatItems.push(m);
    return true;
  }
  function renderChat() {
    chatItems.sort((a, b) => (Date.parse(a.sent_at) || 0) - (Date.parse(b.sent_at) || 0));
    chatItems = chatItems.slice(-CHAT_DISPLAY_LIMIT);
    document.getElementById('chatList').innerHTML = '';
    chatItems.forEach(m => appendMsg('chatList', m.from, m.text, m.sent_at));
  }

  // opts (DM için): own — kendi mesajın; hueKey — renk tonunun anahtarı (ad
  // yerine Peer ID: aynı adı kullanan iki kişi aynı renge boyanmasın);
  // state/stateErr — giden kutusu durumu.
  function appendMsg(listId, from, text, ts, opts) {
    opts = opts || {};
    const ul = document.getElementById(listId);
    const own = 'own' in opts ? !!opts.own : !!(window.__identity && from === window.__identity.name);
    const li = document.createElement('li');
    li.className = 'msg' + (own ? ' own' : '');
    li.style.setProperty('--h', String(hue(opts.hueKey || from)));
    const initial = (String(from).trim().charAt(0) || '?').toLocaleUpperCase(DIL);
    const state = opts.state ? '<span class="state' + (opts.stateErr ? ' err' : '') + '">' + esc(opts.state) + '</span>' : '';
    li.innerHTML = '<span class="avatar" aria-hidden="true">' + esc(initial) + '</span>' +
      '<div><span class="who">' + esc(from) + '</span><time>' + esc(fmtTime(ts)) + '</time>' + state + '</div>' +
      '<div class="text">' + esc(text) + '</div>';
    ul.appendChild(li);
    ul.scrollTop = ul.scrollHeight;
  }

  // ─── Doğrudan mesaj ───
  // Konuşmalar ve okunmamış sayıları uzantıdan hazır gelir (AND'ın DM geçmişi
  // ve okundu bilgisi). Seçili konuşma, DM sekmesi açıkken okunmamış mesaj
  // içeriyorsa okundu bildirilir.
  let dmConvs = [];
  let dmSelected = '';

  function shortId(p) { return p.length > 14 ? p.slice(0, 12) + '…' : p; }
  function convName(k) { return k && k.ad ? k.ad : shortId(dmSelected); }

  function renderDmList() {
    const total = dmConvs.reduce((n, k) => n + (k.okunmamis || 0), 0);
    document.getElementById('dmCount').textContent = total ? String(total) : '';
    const ul = document.getElementById('dmConvs');
    ul.innerHTML = dmConvs.map(k => {
      const last = k.mesajlar[k.mesajlar.length - 1];
      const preview = last ? (last.giden ? tf(${j("Sen: %s")}, last.metin) : last.metin) : '';
      return '<li class="conv' + (k.peerId === dmSelected ? ' sel' : '') + '" tabindex="0" data-conv="' + esc(k.peerId) + '">' +
        '<div class="conv-top"><span class="who" style="--h:' + hue(k.peerId) + '">' + esc(k.ad || shortId(k.peerId)) + '</span>' +
        '<span class="count">' + (k.okunmamis ? k.okunmamis : '') + '</span></div>' +
        '<div class="conv-prev muted">' + esc(preview) + '</div></li>';
    }).join('');
    ul.querySelectorAll('[data-conv]').forEach(li => li.addEventListener('click', () => dmSelect(li.dataset.conv)));
  }

  function renderDmThread() {
    const head = document.getElementById('dmHead');
    document.getElementById('dmList').innerHTML = '';
    if (!dmSelected) {
      head.className = 'dm-head muted';
      head.textContent = ${j("Bir konuşma seç ya da Peer ID ile yeni bir konuşma aç.")};
      return;
    }
    const k = dmConvs.find(x => x.peerId === dmSelected);
    head.className = 'dm-head';
    head.innerHTML = '<strong>' + esc(k ? convName(k) : ${j("Yeni konuşma")}) + '</strong><span class="mono muted">' + esc(dmSelected) + '</span>';
    (k ? k.mesajlar : []).forEach(m => appendMsg('dmList', m.giden ? ${j("Sen")} : convName(k), m.metin, m.at, {
      own: m.giden,
      hueKey: m.giden ? 'ben' : k.peerId,
      state: m.basarisiz ? ${j("gönderilemedi")} : (m.bekliyor ? ${j("bekliyor")} : ''),
      stateErr: !!m.basarisiz,
    }));
  }

  function dmMarkRead() {
    const k = dmConvs.find(x => x.peerId === dmSelected);
    if (activeTab === 'dm' && k && k.okunmamis > 0) vscode.postMessage({ type: 'dmOpen', peerId: dmSelected });
  }

  function dmSelect(peerId) {
    dmSelected = peerId;
    renderDmList();
    renderDmThread();
    dmMarkRead();
  }

  function renderIdentity(identity, role) {
    const body = document.getElementById('identityBody');
    const roller = [];
    if (role.is_founder) roller.push(${j("Kurucu")});
    if (role.is_moderator) roller.push(${j("Moderatör")});
    const rol = roller.length ? '<span class="role">' + esc(roller.join(', ')) + '</span>' : esc(${j("Üye")});
    const kopya = (deger, etiket) => '<button class="icon-btn" data-copy="' + esc(deger) + '" data-label="' + esc(etiket) + '" title="' + esc(tf(${j("%s kopyala")}, etiket)) + '" aria-label="' + esc(tf(${j("%s kopyala")}, etiket)) + '">' + COPY_ICON + '</button>';
    body.innerHTML =
      '<dt>' + esc(${j("Ad")}) + '</dt><dd><strong>' + esc(identity.name || ${j("adsız")}) + '</strong></dd>' +
      '<dt>' + esc(${j("Rol")}) + '</dt><dd>' + rol + '</dd>' +
      '<dt>Peer ID</dt><dd><span class="mono" title="' + esc(identity.peer_id) + '">' + esc(identity.peer_id) + '</span>' + kopya(identity.peer_id, 'Peer ID') + '</dd>' +
      '<dt>' + esc(${j("Genel anahtar")}) + '</dt><dd><span class="mono" title="' + esc(identity.pub_key) + '">' + esc(identity.pub_key) + '</span>' + kopya(identity.pub_key, ${j("Genel anahtar")}) + '</dd>';
    bindCopy(body);
    isModerator = role.is_founder || role.is_moderator;
    document.getElementById('modSection').style.display = isModerator ? '' : 'none';
  }

  function renderPeers(peers) {
    peers = peers || [];
    document.getElementById('peerCount').textContent = peers.length ? String(peers.length) : '';
    document.getElementById('peersBody').innerHTML = peers.map(p =>
      '<li><div class="grow"><div class="mono">' + esc(p.peer_id) + '</div>' +
      '<div class="muted mono">' + p.addrs.map(esc).join('<br>') + '</div></div>' +
      '<button class="icon-btn" data-copy="' + esc(p.peer_id) + '" data-label="Peer ID" title="' + esc(tf(${j("%s kopyala")}, 'Peer ID')) + '" aria-label="' + esc(tf(${j("%s kopyala")}, 'Peer ID')) + '">' + COPY_ICON + '</button></li>'
    ).join('');
    bindCopy(document.getElementById('peersBody'));
    document.getElementById('peerIdsList').innerHTML = peers.map(p => '<option value="' + esc(p.peer_id) + '"></option>').join('');
  }

  function renderPostDetail(d) {
    const body = document.getElementById('postDetailBody');
    const yanitlar = d.replies || [];
    let html = '<article class="post"><h2>' + esc(d.title) + '</h2>' +
      '<p class="muted">' + th(${j("%s yazdı, %s kategorisinde.")}, '<span class="who" style="--h:' + hue(d.author_key || d.author_name) + '">' + esc(d.author_name) + '</span>', esc(kat(d.category))) + '</p>' +
      '<div class="body">' + esc(d.body) + '</div>' +
      '<h3>' + esc(yanitlar.length ? tf(${j("%d yanıt")}, yanitlar.length) : ${j("Henüz yanıt yok")}) + '</h3>' +
      '<ul class="list replies">' + yanitlar.map(r =>
        '<li><div class="grow"><span class="who" style="--h:' + hue(r.author_key || r.author_name) + '">' + esc(r.author_name) + '</span>' +
        '<div class="body">' + esc(r.body) + '</div></div></li>').join('') + '</ul>' +
      '<div class="composer" style="margin-top:10px"><input type="text" id="replyBody" placeholder="' + esc(${j("Yanıtını yaz…")}) + '" />' +
      '<button class="btn primary" id="replySend">' + esc(${j("Yanıtla")}) + '</button></div></article>';
    body.innerHTML = html;
    document.getElementById('replySend').addEventListener('click', () => {
      const el = document.getElementById('replyBody');
      if (!el.value.trim()) return;
      vscode.postMessage({ type: 'reply', postId: d.id, body: el.value });
    });
    onEnter('replyBody', 'replySend');
  }

  function renderMod(all) {
    const list = document.getElementById('allPostsBody');
    list.innerHTML = (all || []).map(p =>
      '<li><div class="grow"><strong>' + esc(p.title) + '</strong><div class="muted">' + esc(kat(p.category)) + ', ' + esc(p.author_name) + '</div></div>' +
      '<button class="btn ghost sm danger" data-delete="' + esc(p.id) + '">' + esc(${j("Sil")}) + '</button></li>'
    ).join('');
    list.querySelectorAll('[data-delete]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'deletePost', postId: b.dataset.delete })));
  }

  function renderSite(data) {
    const body = document.getElementById('siteBody');
    document.getElementById('siteCount').textContent = data.pages.length + '/' + data.max;
    document.getElementById('siteNewBtn').disabled = data.pages.length >= data.max;
    if (!data.pages.length) { body.innerHTML = '<p class="muted">' + esc(${j("Henüz bir sayfan yok. İlk sayfanı oluşturmak için Yeni sayfa’yı kullan.")}) + '</p>'; return; }
    const sayi = (n) => Number(n).toLocaleString(DIL);
    body.innerHTML = '<ul class="list">' + data.pages.map(p =>
      '<li><div class="grow"><div><span class="title-link" tabindex="0" data-site-open="' + esc(p.slug) + '">' + esc(p.title || p.slug) + '</span>' +
      ' <span class="mono muted">/' + esc(p.slug) + '</span></div>' +
      (p.preview ? '<p class="preview">' + esc(p.preview) + '</p>' : '') +
      '<div class="muted">' + th(${j("%s güncellendi, %s / %s bayt")}, esc(new Date(p.updated_at).toLocaleString(DIL)), esc(sayi(p.bytes)), esc(sayi(data.maxBody))) + '</div></div>' +
      '<div class="actions"><button class="btn secondary sm" data-site-open="' + esc(p.slug) + '">' + esc(${j("Düzenle")}) + '</button>' +
      '<button class="btn ghost sm" data-site-code="' + esc(p.slug) + '">' + esc(${j("Paylaşım kodu")}) + '</button>' +
      '<button class="btn ghost sm danger" data-site-delete="' + esc(p.slug) + '">' + esc(${j("Sil")}) + '</button></div></li>'
    ).join('') + '</ul>';
    body.querySelectorAll('[data-site-open]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'siteOpen', slug: b.dataset.siteOpen })));
    body.querySelectorAll('[data-site-code]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'siteCopyCode', slug: b.dataset.siteCode })));
    body.querySelectorAll('[data-site-delete]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'siteDelete', slug: b.dataset.siteDelete })));
  }

  let pendingConsents = [];
  let receivedFiles = [];

  function renderConsents() {
    const body = document.getElementById('consentBody');
    body.innerHTML = pendingConsents.map(c =>
      '<li><div class="grow"><strong>' + esc(c.filename) + '</strong> <span class="muted">' + formatSize(c.size) + '</span>' +
      '<div class="muted mono">' + esc(c.sender_id) + '</div></div>' +
      '<div class="actions"><button class="btn primary sm" data-accept="' + esc(c.transfer_id) + '">' + esc(${j("Kabul et")}) + '</button>' +
      '<button class="btn ghost sm" data-reject="' + esc(c.transfer_id) + '">' + esc(${j("Reddet")}) + '</button></div></li>'
    ).join('');
    body.querySelectorAll('[data-accept]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'respondConsent', transferId: b.dataset.accept, accept: true })));
    body.querySelectorAll('[data-reject]').forEach(b => b.addEventListener('click', () => vscode.postMessage({ type: 'respondConsent', transferId: b.dataset.reject, accept: false })));
  }

  function renderReceivedFiles() {
    const body = document.getElementById('receivedFilesBody');
    body.innerHTML = receivedFiles.map((f, i) =>
      '<li><div class="grow"><strong>' + esc(f.filename) + '</strong> <span class="muted">' + formatSize(f.size) + '</span>' +
      '<div class="muted">' + th(${j("%s gönderdi, %s")}, esc(f.from), esc(new Date(f.received_at).toLocaleString(DIL))) + '</div></div>' +
      '<button class="btn ghost sm" data-reveal="' + i + '">' + esc(${j("Klasörde göster")}) + '</button></li>'
    ).join('');
    body.querySelectorAll('[data-reveal]').forEach(b => b.addEventListener('click', () => {
      const f = receivedFiles[Number(b.dataset.reveal)];
      vscode.postMessage({ type: 'revealFile', path: f.save_path });
    }));
  }

  window.addEventListener('message', (event) => {
    const msg = event.data;
    switch (msg.type) {
      case 'connectionState': {
        const badge = document.getElementById('connBadge');
        badge.textContent = msg.connected ? ${j("bağlı")} : ${j("bağlı değil")};
        badge.className = 'conn' + (msg.connected ? ' ok' : '');
        break;
      }
      case 'identity':
        window.__identity = msg.data;
        if (window.__role) renderIdentity(window.__identity, window.__role);
        break;
      case 'role':
        window.__role = msg.data;
        if (window.__identity) renderIdentity(window.__identity, window.__role);
        break;
      case 'peers':
        renderPeers(msg.data);
        break;
      case 'chatHistory':
        (msg.data || []).forEach(addChat);
        renderChat();
        break;
      case 'chatMsg':
        if (addChat(msg.data)) appendMsg('chatList', msg.data.from, msg.data.text, msg.data.sent_at);
        break;
      case 'sitePages':
        renderSite(msg.data);
        break;
      case 'siteUnavailable':
        document.getElementById('siteBody').innerHTML = '<p class="muted">' + th(${j("Kişisel site sayfaları alınamadı: %s")}, esc(msg.message)) + '</p>';
        break;
      case 'dmData': {
        const d = msg.data || {};
        // Eklentilerin makine mesajları (DeltaX) uzantıda ayıklanır; burada da gösterilmez.
        dmConvs = (d.konusmalar || []).map(k => Object.assign({}, k, {
          mesajlar: (k.mesajlar || []).filter(m => String(m.metin).indexOf('AND-PROJEAG/') !== 0),
        })).filter(k => k.mesajlar.length > 0);
        document.getElementById('dmLede').textContent = d.gecmisYok
          ? ${j("Bu AND sürümü mesaj geçmişi tutmuyor: yalnızca bu oturumda gelen ve gönderilen mesajlar görünür.")}
          : ${j("Uçtan uca şifreli; yalnızca karşı tarafa gider. Karşı taraf çevrimdışıysa AND mesajı bekletir ve bağlandığında iletir. Konuşmalar ve okunmamışlar AND ve Özel Chat ile ortaktır.")};
        renderDmList();
        renderDmThread();
        dmMarkRead();
        break;
      }
      case 'postDetail':
        renderPostDetail(msg.data);
        break;
      case 'forumMod':
        renderMod(msg.all);
        break;
      case 'fileMsg':
        receivedFiles.unshift(msg.data);
        renderReceivedFiles();
        break;
      case 'consentReq':
        pendingConsents.push(msg.data);
        renderConsents();
        break;
      case 'consentResolved':
        pendingConsents = pendingConsents.filter(c => c.transfer_id !== msg.transferId);
        renderConsents();
        break;
      case 'theme': {
        const t = msg.data || {};
        const root = document.documentElement.style;
        const setOrClear = (name, value) => value ? root.setProperty(name, value) : root.removeProperty(name);
        setOrClear('--and-accent', t.accent);
        setOrClear('--and-accent-fg', t.accentFg);
        setOrClear('--and-selbg', t.selBg);
        setOrClear('--and-selfg', t.selFg);
        setOrClear('--and-badge', t.badge);
        document.getElementById('themeName').textContent = t.themeName ? tf(${j("%s teması")}, TEMA[t.themeName] || t.themeName) : '';
        break;
      }
      case 'info':
        showBanner(msg.message, true);
        break;
      case 'error':
        showBanner(msg.message, false);
        break;
    }
  });

  vscode.postMessage({ type: 'refreshAll' });
</script>
</body>
</html>`;
}
