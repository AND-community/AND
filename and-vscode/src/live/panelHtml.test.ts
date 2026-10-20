import { test } from "node:test";
import assert from "node:assert/strict";
import { renderHtml, resolveThemeColors } from "./panelHtml";

// Canlı Panel'in webview betiği, küçük bir sahte DOM üzerinde çalıştırılır:
// gerçek tarayıcı olmadan mesaj işleyicileri, kaçırma (XSS), sohbet birleştirme,
// Enter ile gönderme, kopyalama ve tema renkleri sınanır.

type Listener = (e?: unknown) => void;

const camel = (a: string): string => a.replace(/-([a-z])/g, (_, ch: string) => ch.toUpperCase());

class FakeEl {
  children: FakeEl[] = [];
  listeners: Record<string, Listener[]> = {};
  dataset: Record<string, string> = {};
  props: Record<string, string> = {};
  style: Record<string, unknown> = {
    setProperty: (k: string, v: string) => (this.props[k] = v),
    removeProperty: (k: string) => delete this.props[k],
  };
  className = "";
  textContent = "";
  value = "";
  disabled = false;
  lastQuery: FakeEl[] = [];
  classList = { add(): void {}, remove(): void {} };
  private html = "";

  set innerHTML(v: string) {
    this.html = v;
    this.children = [];
  }
  get innerHTML(): string {
    return this.html + this.children.map((c) => c.innerHTML).join("");
  }
  set scrollTop(_: number) {}
  get scrollHeight(): number {
    return 0;
  }
  appendChild(c: FakeEl): void {
    this.children.push(c);
  }
  addEventListener(t: string, f: Listener): void {
    (this.listeners[t] ??= []).push(f);
  }
  click(): void {
    (this.listeners.click ?? []).forEach((f) => f());
  }
  /** Yalnızca "[data-x]" seçicileri: eşleşen her etiket, TÜM data-* öznitelikleriyle sahte düğüme çevrilir. */
  querySelectorAll(sel: string): FakeEl[] {
    const m = /^\[data-([a-z-]+)\]$/.exec(sel);
    if (!m) {
      return [];
    }
    const re = new RegExp(`<[^>]*\\sdata-${m[1]}="[^"]*"[^>]*>`, "g");
    this.lastQuery = (this.html.match(re) ?? []).map((tag) => {
      const e = new FakeEl();
      for (const [, k, v] of tag.matchAll(/data-([a-z-]+)="([^"]*)"/g)) {
        e.dataset[camel(k)] = v;
      }
      return e;
    });
    return this.lastQuery;
  }
}

function boot() {
  const html = renderHtml({ cspSource: "vscode-resource:" });
  const m = /<script nonce="([0-9a-f]+)">([\s\S]*?)<\/script>/.exec(html);
  assert.ok(m, "nonce'lu script bloğu bulunamadı");
  assert.ok(html.includes(`script-src 'nonce-${m[1]}'`), "CSP aynı nonce'u izin vermeli");

  const els: Record<string, FakeEl> = {};
  const el = (id: string): FakeEl => (els[id] ??= new FakeEl());
  const root = new FakeEl();
  const posted: unknown[] = [];
  let onMessage: Listener = () => {};
  const document = {
    getElementById: el,
    querySelectorAll: () => [],
    createElement: () => new FakeEl(),
    documentElement: { style: root.style },
  };
  const window = { addEventListener: (t: string, f: Listener) => t === "message" && (onMessage = f) };
  const api = () => ({ postMessage: (msg: unknown) => posted.push(msg) });
  new Function("document", "window", "acquireVsCodeApi", "setTimeout", "clearTimeout", m[2])(
    document,
    window,
    api,
    () => 0,
    () => {}
  );
  const send = (data: unknown) => onMessage({ data });
  const enter = (id: string) => els[id].listeners.keydown[0]({ key: "Enter", shiftKey: false, preventDefault() {} });
  return { html, els, el, root, posted, send, enter };
}

test("panel: açılışta yenileme ister, CSP dışarıdan betik/stil yüklemez", () => {
  const { html, posted } = boot();
  assert.deepEqual(posted, [{ type: "refreshAll" }]);
  assert.match(html, /default-src 'none'/);
  assert.doesNotMatch(html, /<script src=|<link /);
  for (const id of ["connBadge", "refreshBtn", "identityBody", "peersBody", "chatList", "chatInput", "chatSend", "dmPeer", "dmInput", "dmSend", "postCategory", "postCreate", "viewPostBtn", "siteNewBtn", "siteBody", "fileSendBtn", "consentBody", "receivedFilesBody", "themeName"]) {
    assert.ok(html.includes(`id="${id}"`), `${id} eksik`);
  }
});

test("panel: kimlik kaçırılır, kopyala düğmesi copyText gönderir", () => {
  const { els, posted, send } = boot();
  send({ type: "identity", data: { name: "umut", peer_id: "12D3KooABC", pub_key: "a3f9<x>" } });
  send({ type: "role", data: { is_founder: false, is_moderator: true } });
  assert.ok(els.identityBody.innerHTML.includes("a3f9&lt;x&gt;"));
  assert.ok(els.identityBody.innerHTML.includes("Moderatör"));
  assert.equal(els.modSection.style.display, "", "moderatöre moderasyon bölümü görünmeli");
  posted.length = 0;
  els.identityBody.lastQuery[0].click();
  assert.deepEqual(posted, [{ type: "copyText", text: "12D3KooABC", label: "Peer ID" }]);
});

test("panel: eş sayacı ve alıcı önerileri", () => {
  const { els, send } = boot();
  send({ type: "peers", data: [{ peer_id: "p1", addrs: ["/ip4/1.2.3.4/tcp/1"] }, { peer_id: "p2", addrs: [] }] });
  assert.equal(els.peerCount.textContent, "2");
  assert.ok(els.peerIdsList.innerHTML.includes('value="p2"'));
  send({ type: "peers", data: [] });
  assert.equal(els.peerCount.textContent, "", "eş yokken sayaç gizlenmeli");
});

test("panel: site listesi kaçırılır, sınırda Yeni sayfa kapanır", () => {
  const { els, posted, send } = boot();
  send({ type: "sitePages", data: { max: 20, maxBody: 8000, pages: [{ slug: "hakkimda", title: 'B "t" <b>', updated_at: "2026-10-18T10:00:00Z", bytes: 12, preview: "ö" }] } });
  const h = els.siteBody.innerHTML;
  assert.ok(h.includes("B &quot;t&quot; &lt;b&gt;"));
  assert.equal(els.siteCount.textContent, "1/20");
  for (const attr of ["data-site-open", "data-site-code", "data-site-delete"]) {
    assert.ok(h.includes(`${attr}="hakkimda"`), `${attr} eksik`);
  }
  const page = (slug: string) => ({ slug, title: slug, updated_at: "x", bytes: 1, preview: "" });
  send({ type: "sitePages", data: { max: 2, maxBody: 8000, pages: [page("a"), page("b")] } });
  assert.equal(els.siteNewBtn.disabled, true);
  send({ type: "siteUnavailable", message: "404 <x>" });
  assert.ok(els.siteBody.innerHTML.includes("404 &lt;x&gt;"));
  posted.length = 0;
  els.siteNewBtn.click();
  els.siteRefreshBtn.click();
  assert.deepEqual(posted, [{ type: "siteNew" }, { type: "siteRefresh" }]);
});

test("panel: sohbet geçmişi canlı akışla birleşir, yinelenmez, sıralanır", () => {
  const { el, send } = boot();
  const list = el("chatList");
  const texts = () => list.children.map((li) => /<div class="text">(.*?)<\/div>/.exec(li.innerHTML)?.[1]);
  send({ type: "identity", data: { name: "umut", peer_id: "p", pub_key: "k" } });
  send({ type: "chatMsg", data: { from: "ali", text: "canlı-yeni", sent_at: "2026-10-18T10:05:00+03:00" } });
  send({ type: "chatHistory", data: [
    { id: "1", from: "ali", text: "eski-1", sent_at: "2026-10-18T06:00:00Z" },
    { id: "2", from: "veli", text: "eski-2", sent_at: "2026-10-18T06:01:00.123456789Z" },
  ] });
  assert.deepEqual(texts(), ["eski-1", "eski-2", "canlı-yeni"]);
  send({ type: "chatMsg", data: { from: "veli", text: "eski-2", sent_at: "2026-10-18T09:01:00.123+03:00" } });
  assert.equal(list.children.length, 3, "aynı an farklı ofsetle gelirse yinelenmemeli");
  assert.equal(list.children[0].props["--h"], list.children[2].props["--h"], "aynı ad aynı renk tonunda olmalı");
  send({ type: "chatMsg", data: { from: "umut", text: "<script>", sent_at: "2026-10-18T11:00:00Z" } });
  const last = list.children[list.children.length - 1];
  assert.equal(last.className, "msg own");
  assert.ok(last.innerHTML.includes("&lt;script&gt;"));
});

test("panel: Enter gönderir; boş mesaj ve alıcısız DM gönderilmez", () => {
  const { els, el, posted, enter } = boot();
  posted.length = 0;
  el("chatInput").value = "merhaba";
  enter("chatInput");
  assert.deepEqual(posted, [{ type: "sendChat", message: "merhaba" }]);
  assert.equal(els.chatInput.value, "");
  enter("chatInput");
  assert.equal(posted.length, 1);
  el("dmInput").value = "selam";
  enter("dmInput");
  assert.equal(posted.length, 1);
  assert.match(els.banner.className, /err/);
  el("dmPeer").value = " p1 ";
  enter("dmInput");
  assert.deepEqual(posted[1], { type: "sendDM", peerId: "p1", message: "selam" });
});

test("panel: konu ayrıntısı ve yanıt", () => {
  const { els, el, posted, send } = boot();
  send({ type: "postDetail", data: { id: "k1", title: "T <i>", category: "Go", author_name: "z", author_key: "5d", body: "g", replies: [{ author_name: "a", author_key: "a1", body: "y" }] } });
  assert.ok(els.postDetailBody.innerHTML.includes("T &lt;i&gt;"));
  assert.ok(els.postDetailBody.innerHTML.includes("1 yanıt"));
  posted.length = 0;
  el("replyBody").value = "katılıyorum";
  els.replySend.click();
  assert.deepEqual(posted, [{ type: "reply", postId: "k1", body: "katılıyorum" }]);
});

test("panel: tema değişkenleri uygulanır, boş temada varsayılana dönülür", () => {
  const { els, root, send } = boot();
  send({ type: "theme", data: { themeName: "Turkuaz", accent: "#5fd7d7", accentFg: "#000000", selBg: "#005f5f", selFg: "#eeeeee" } });
  assert.equal(root.props["--and-accent"], "#5fd7d7");
  assert.equal(root.props["--and-accent-fg"], "#000000");
  assert.equal(els.themeName.textContent, "Turkuaz teması");
  send({ type: "theme", data: {} });
  assert.equal(root.props["--and-accent"], undefined);
  assert.equal(els.themeName.textContent, "");
  send({ type: "connectionState", connected: true });
  assert.equal(els.connBadge.className, "conn ok");
});

test("resolveThemeColors: xterm-256 kodları CSS'e, vurgu yazısı okunur renge", () => {
  const c = resolveThemeColors({ theme_name: "Turkuaz", accent: "80", sel_bg: "23", sel_fg: "255", badge: "141" });
  assert.deepEqual(
    { accent: c.accent, accentFg: c.accentFg, selBg: c.selBg, selFg: c.selFg, badge: c.badge },
    { accent: "#5fd7d7", accentFg: "#000000", selBg: "#005f5f", selFg: "#eeeeee", badge: "#af87ff" }
  );
  assert.equal(resolveThemeColors({ accent: "#000080" }).accentFg, "#ffffff");
  // AND'ın varsayılan Klasik teması: mavi-mor vurguda beyaz yazı.
  const klasik = resolveThemeColors({ theme_name: "Klasik", accent: "63", sel_bg: "57", sel_fg: "255", badge: "33" });
  assert.deepEqual(
    { accent: klasik.accent, accentFg: klasik.accentFg, selBg: klasik.selBg, badge: klasik.badge },
    { accent: "#5f5fff", accentFg: "#ffffff", selBg: "#5f00ff", badge: "#0087ff" }
  );
  assert.equal(resolveThemeColors({}).accentFg, undefined);
});
