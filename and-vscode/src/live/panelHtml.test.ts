import { test } from "node:test";
import assert from "node:assert/strict";
import { KAYNAK, ayarla, gomulu, t } from "../dil/dil";
import { renderHtml, resolveThemeColors } from "./panelHtml";

// Canlı Panel'in webview betiği, küçük bir sahte DOM üzerinde çalıştırılır:
// gerçek tarayıcı olmadan mesaj işleyicileri, kaçırma (XSS), sohbet birleştirme,
// Enter ile gönderme, kopyalama ve tema renkleri sınanır.

type Listener = (e?: unknown) => void;

const camel = (a: string): string => a.replace(/-([a-z])/g, (_, ch: string) => ch.toUpperCase());

class FakeEl {
  /**
   * innerHTML yazılınca içindeki id'li öğeler gerçek tarayıcıdaki gibi yeni
   * öğelerdir: boot() bu kancayla onları kayıttan siler, sonraki
   * getElementById taze (değersiz, dinleyicisiz) bir öğe verir.
   */
  static yenilendi: (html: string) => void = () => {};
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
  /** Seçiciye göre son querySelectorAll sonucu. */
  queries: Record<string, FakeEl[]> = {};
  classList = { add(): void {}, remove(): void {}, toggle(): void {} };
  private html = "";

  set innerHTML(v: string) {
    this.html = v;
    this.children = [];
    FakeEl.yenilendi(v);
  }
  focus(): void {}
  /** Olayı bütün dinleyicilere iletir (input, change, keydown…). */
  fire(tur: string, e?: unknown): void {
    (this.listeners[tur] ?? []).forEach((f) => f(e));
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
    this.queries[sel] = this.lastQuery;
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
  FakeEl.yenilendi = (yeni) => {
    for (const [, id] of yeni.matchAll(/\sid="([^"]+)"/g)) {
      delete els[id];
    }
  };
  const root = new FakeEl();
  const posted: unknown[] = [];
  let onMessage: Listener = () => {};
  // Gezinme rayındaki düğmeler: betik sekme değişimini bunlara tıklanınca öğrenir.
  const nav = [...html.matchAll(/<button data-tab="([a-z]+)"/g)].map(([, tab]) => {
    const b = new FakeEl();
    b.dataset.tab = tab;
    return b;
  });
  const document = {
    getElementById: el,
    querySelectorAll: (sel: string) => (sel === "#nav button" ? nav : []),
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
  const openTab = (tab: string) => nav.find((b) => b.dataset.tab === tab)!.click();
  return { html, els, el, root, posted, send, enter, openTab };
}

test("panel: açılışta yenileme ister, CSP dışarıdan betik/stil yüklemez", () => {
  const { html, posted } = boot();
  assert.deepEqual(posted, [{ type: "refreshAll" }]);
  assert.match(html, /default-src 'none'/);
  assert.doesNotMatch(html, /<script src=|<link /);
  for (const id of ["connBadge", "refreshBtn", "identityBody", "peersBody", "chatList", "chatInput", "chatSend", "dmNewPeer", "dmNewBtn", "dmConvs", "dmCount", "dmHead", "dmInput", "dmSend", "forumKategori", "forumYeniBtn", "forumKonular", "forumEski", "forumSag", "viewPostBtn", "siteNewBtn", "siteBody", "fileSendBtn", "consentBody", "receivedFilesBody", "themeName"]) {
    assert.ok(html.includes(`id="${id}"`), `${id} eksik`);
  }
});

test("panel: kimlik kaçırılır, kopyala düğmesi copyText gönderir", () => {
  const { els, posted, send } = boot();
  send({ type: "identity", data: { name: "umut", peer_id: "12D3KooABC", pub_key: "a3f9<x>" } });
  send({ type: "role", data: { is_founder: false, is_moderator: true } });
  assert.ok(els.identityBody.innerHTML.includes("a3f9&lt;x&gt;"));
  assert.ok(els.identityBody.innerHTML.includes("Moderatör"));
  posted.length = 0;
  els.identityBody.lastQuery[0].click();
  assert.deepEqual(posted, [{ type: "copyText", text: "12D3KooABC", label: "Peer ID" }]);
});

test("panel: bağlantı yoksa kimlik bölümü VS Code bağlantısının nerede açılacağını söyler", () => {
  const { els, send } = boot();
  send({ type: "connectionState", connected: false });
  assert.match(els.identityBody.innerHTML, /Ayarlar › Eklentiler/);
  assert.match(els.identityBody.innerHTML, /VS Code bağlantısı/);
  send({ type: "identity", data: { name: "umut", peer_id: "12D3", pub_key: "a3" } });
  send({ type: "role", data: { is_founder: false, is_moderator: false } });
  send({ type: "connectionState", connected: false });
  assert.ok(els.identityBody.innerHTML.includes("umut"), "bilinen kimlik bağlantı kopunca silinmemeli");
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

test("panel: Enter gönderir; boş mesaj ve konuşma seçilmeden DM gönderilmez", () => {
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
  el("dmNewPeer").value = " p1 ";
  enter("dmNewPeer");
  assert.ok(els.dmHead.innerHTML.includes("Yeni konuşma"), "yeni konuşma başlığı görünmeli");
  enter("dmInput");
  assert.deepEqual(posted[1], { type: "sendDM", peerId: "p1", message: "selam" });
});

const ALI = "12D3KooWAzizAbCdEfGhJkLmNpQrStUvWxYz123456789abcdefg";
const VELI = "12D3KooWVedatAbCdEfGhJkLmNpQrStUvWxYz123456789abcde";

function dmData(okunmamisAli = 2) {
  return {
    type: "dmData",
    data: {
      konusmalar: [
        {
          peerId: ALI,
          ad: "ali <b>",
          okunmamis: okunmamisAli,
          mesajlar: [
            { metin: "selam", at: "2026-10-21T10:00:00Z", giden: false },
            { metin: "AND-PROJEAG/1|eyJ0aXAiOiJkYXZldCJ9", at: "2026-10-21T10:00:01Z", giden: false },
            { metin: "naber <i>", at: "2026-10-21T10:01:00Z", giden: false },
          ],
        },
        {
          peerId: VELI,
          ad: "",
          okunmamis: 0,
          mesajlar: [
            { metin: "eski", at: "2026-10-14T09:00:00Z", giden: true, basarisiz: true },
            { metin: "yarın?", at: "2026-10-21T09:00:00Z", giden: true, bekliyor: true },
          ],
        },
      ],
    },
  };
}

test("panel: DM konuşmaları ve okunmamış sayıları; makine mesajı gösterilmez", () => {
  const { els, send } = boot();
  send(dmData());
  const list = els.dmConvs.innerHTML;
  assert.equal(els.dmCount.textContent, "2", "gezinmede toplam okunmamış");
  assert.ok(list.includes("ali &lt;b&gt;") && !list.includes("ali <b>"), "ad kaçırılmalı");
  assert.ok(list.includes("12D3KooWVeda…"), "adı olmayan eş kısaltılmış Peer ID ile görünmeli");
  assert.ok(list.includes("naber &lt;i&gt;"), "son mesaj önizlemesi");
  assert.ok(list.includes("Sen: yarın?"), "kendi son mesajın önizlemesi");
  assert.doesNotMatch(list, /AND-PROJEAG/);
});

test("panel: konuşma seçilince akış gösterilir; okundu yalnızca DM sekmesi açıkken bildirilir", () => {
  const { els, el, posted, send, openTab } = boot();
  send(dmData());
  posted.length = 0;
  els.dmConvs.lastQuery[0].click(); // ali
  const texts = el("dmList").children.map((li) => li.innerHTML);
  assert.equal(texts.length, 2, "makine mesajı akışta yer almamalı");
  assert.ok(texts[1].includes("naber &lt;i&gt;"));
  assert.deepEqual(posted, [], "DM sekmesi açık değilken okundu bildirilmemeli");
  openTab("dm");
  assert.deepEqual(posted, [{ type: "dmOpen", peerId: ALI }]);
  send(dmData(0));
  assert.equal(posted.length, 1, "okunmamış kalmayınca yeniden bildirilmemeli");
  assert.equal(els.dmCount.textContent, "");

  els.dmConvs.lastQuery[1].click(); // veli: giden kutusu durumları
  const veli = el("dmList").children.map((li) => li.innerHTML);
  assert.ok(veli[0].includes('<span class="state err">gönderilemedi</span>'));
  assert.ok(veli[1].includes('<span class="state">bekliyor</span>'));
  assert.equal(el("dmList").children[0].className, "msg own");

  el("dmInput").value = "tamam";
  els.dmSend.click();
  assert.deepEqual(posted[posted.length - 1], { type: "sendDM", peerId: VELI, message: "tamam" });
});

test("panel: geçmiş tutmayan AND'da açıklama değişir", () => {
  const { els, send } = boot();
  send({ type: "dmData", data: { konusmalar: [], gecmisYok: true } });
  assert.match(els.dmLede.textContent, /geçmişi tutmuyor/);
});

const birSaatOnce = () => new Date(Date.now() - 3600_000).toISOString();
const ozet = (id: string, ek: Record<string, unknown> = {}) => ({
  id,
  category: "Go",
  title: "Başlık " + id,
  author_name: "ali",
  author_key: "a1",
  created_at: birSaatOnce(),
  reply_count: 2,
  ...ek,
});
const ayrinti = (id: string, ek: Record<string, unknown> = {}) => ({
  id,
  title: "Başlık " + id,
  category: "Go",
  author_name: "ali",
  author_key: "a1",
  body: "gövde " + id,
  created_at: birSaatOnce(),
  replies: [],
  ...ek,
});

test("panel forum: konu listesi kaçırılır, kategori süzgeci sayılarıyla, tıklayınca forumOpen", () => {
  const { els, posted, send } = boot();
  send({
    type: "forumPosts",
    liste: true,
    konular: [ozet("k1", { title: "<b>x</b>" }), ozet("k2", { category: "Python", author_role: "moderator", reply_count: 1 })],
    kategoriler: [{ kategori: "Python", sayi: 1 }, { kategori: "Go", sayi: 1 }],
  });
  // AND yanıtsız konuda reply_count'u hiç göndermez (omitempty): yeni AND'da (zamanı olan konu) 0 yanıt yazılır.
  send({ type: "forumPosts", liste: true, konular: [ozet("s", { reply_count: undefined })], kategoriler: [{ kategori: "Go", sayi: 1 }] });
  assert.ok(els.forumKonular.innerHTML.includes("0 yanıt"), els.forumKonular.innerHTML);
  send({ type: "forumPosts", liste: true, konular: [{ id: "e", category: "Go", title: "eski", author_name: "a", author_key: "a" }], kategoriler: [{ kategori: "Go", sayi: 1 }] });
  assert.ok(!els.forumKonular.innerHTML.includes("yanıt"), "eski AND'ın konusunda yanıt sayısı bilinmez");
  send({
    type: "forumPosts",
    liste: true,
    konular: [ozet("k1", { title: "<b>x</b>" }), ozet("k2", { category: "Python", author_role: "moderator", reply_count: 1 })],
    kategoriler: [{ kategori: "Python", sayi: 1 }, { kategori: "Go", sayi: 1 }],
  });
  const liste = els.forumKonular.innerHTML;
  assert.ok(liste.includes("&lt;b&gt;x&lt;/b&gt;") && !liste.includes("<b>x</b>"), liste);
  assert.ok(liste.includes("2 yanıt") && liste.includes("1 yanıt"), liste);
  assert.ok(liste.includes("Moderatör"), liste);
  assert.ok(liste.includes("1 saat önce"), liste);
  const secenekler = els.forumKategori.innerHTML;
  assert.ok(secenekler.includes("Tüm kategoriler (2)"), secenekler);
  assert.ok(secenekler.includes(">Go (1)<") && secenekler.includes(">Python (1)<"), secenekler);

  els.forumKategori.value = "Python";
  els.forumKategori.fire("change");
  assert.ok(!els.forumKonular.innerHTML.includes('data-konu-ac="k1"'));
  assert.ok(els.forumKonular.innerHTML.includes('data-konu-ac="k2"'));
  posted.length = 0;
  els.forumKonular.queries["[data-konu-ac]"][0].click();
  assert.deepEqual(posted, [{ type: "forumOpen", id: "k2" }]);
});

test("panel forum: ayrıntı, kod bloğu kaçırılır; Sil yalnız kendi konunda ya da moderatöre; yazar profili açar", () => {
  const { els, posted, send } = boot();
  send({ type: "identity", data: { name: "ben", peer_id: "12D3", pub_key: "B0" } });
  send({ type: "role", data: { is_founder: false, is_moderator: false } });
  send({
    type: "postDetail",
    ac: true,
    data: ayrinti("k1", {
      body: "önce [code=go]x := <1>[/code] sonra",
      replies: [{ id: "y1", author_name: "veli", author_key: "v1", body: "y", created_at: birSaatOnce(), author_role: "founder" }],
    }),
  });
  let sag = els.forumSag.innerHTML;
  assert.ok(!sag.includes("data-sil"), "başkasının konusunda üyeye Sil yok");
  assert.ok(sag.includes('<pre class="kod">') && sag.includes("x := &lt;1&gt;") && sag.includes(">go<"), sag);
  assert.ok(sag.includes("Kurucu") && sag.includes("1 yanıt"), sag);
  // Blok etiketlerinin çevresindeki satır sonu boş satır bırakmaz (pre zaten blok).
  send({ type: "postDetail", ac: true, data: ayrinti("k1", { body: "a\n[code]\nx\n[/code]\nb\r\n[code]y[/code]\r\nc" }) });
  sag = els.forumSag.innerHTML;
  assert.ok(sag.includes("a<pre") && sag.includes("</pre>b") && sag.includes("</pre>c") && sag.includes("<code>x\n</code>"), sag);
  send({ type: "postDetail", ac: true, data: ayrinti("k1", { body: "önce [code=go]x := <1>[/code] sonra" }) });

  posted.length = 0;
  els.forumSag.queries["[data-profil]"][0].click();
  assert.deepEqual(posted, [{ type: "profile", key: "a1" }]);

  send({ type: "postDetail", ac: true, data: ayrinti("k1", { author_key: "b0" }) });
  posted.length = 0;
  els.forumSag.queries["[data-sil]"][0].click();
  assert.deepEqual(posted, [{ type: "deletePost", postId: "k1", own: true, title: "Başlık k1" }]);

  send({ type: "role", data: { is_founder: false, is_moderator: true } });
  send({ type: "postDetail", ac: true, data: ayrinti("k2") });
  sag = els.forumSag.innerHTML;
  posted.length = 0;
  els.forumSag.queries["[data-sil]"][0].click();
  assert.deepEqual(posted, [{ type: "deletePost", postId: "k2", own: false, title: "Başlık k2" }]);
});

test("panel forum: yanıt taslağı canlı yenilemede ve konu değişiminde korunur, gönderilince temizlenir", () => {
  const { els, posted, send } = boot();
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  els.replyBody.value = "yarım";
  els.replyBody.fire("input");
  send({ type: "postDetail", data: ayrinti("k1", { replies: [{ id: "y", author_name: "a", author_key: "a", body: "yeni", created_at: birSaatOnce() }] }) });
  assert.ok(els.forumSag.innerHTML.includes("yeni"), "canlı yenileme çizilmeli");
  assert.equal(els.replyBody.value, "yarım", "canlı yenileme yazılanı silmemeli");
  send({ type: "postDetail", ac: true, data: ayrinti("k2") });
  assert.equal(els.replyBody.value, "");
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  assert.equal(els.replyBody.value, "yarım");

  posted.length = 0;
  els.replySend.click();
  assert.deepEqual(posted, [{ type: "reply", postId: "k1", body: "yarım" }]);
  send({ type: "replySent", postId: "k1" });
  assert.equal(els.replyBody.value, "");
  send({ type: "postDetail", data: ayrinti("k1") });
  assert.equal(els.replyBody.value, "");
});

test("panel forum: canlı yenileme yeni konu formundan atmaz; başka konunun yenilemesi açık konuyu değiştirmez", () => {
  const { els, send } = boot();
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  els.forumYeniBtn.click();
  assert.ok(els.forumSag.innerHTML.includes('id="postTitle"'));
  send({ type: "postDetail", data: ayrinti("k1") });
  assert.ok(els.forumSag.innerHTML.includes('id="postTitle"'), "form açık kalmalı");
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  send({ type: "postDetail", data: ayrinti("k9") });
  assert.ok(els.forumSag.innerHTML.includes("Başlık k1") && !els.forumSag.innerHTML.includes("Başlık k9"));
});

test("panel forum: yeni konu formu doğrular, taslağı tutar, createPost gönderir; Vazgeç taslağı atar", () => {
  const { els, posted, send } = boot();
  els.forumYeniBtn.click();
  posted.length = 0;
  els.postTitle.value = "Başlık";
  els.postTitle.fire("input");
  els.postCreate.click();
  assert.deepEqual(posted, []);
  assert.match(els.banner.textContent, /boş olamaz/);

  els.postCategory.value = "Python";
  els.postCategory.fire("change");
  els.postBody.value = "gövde";
  els.postBody.fire("input");
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  els.forumYeniBtn.click();
  assert.equal(els.postTitle.value, "Başlık", "taslak korunmalı");
  assert.equal(els.postCategory.value, "Python");
  els.postCreate.click();
  assert.deepEqual(posted, [{ type: "createPost", category: "Python", title: "Başlık", body: "gövde" }]);

  send({ type: "forumCreated" });
  els.forumYeniBtn.click();
  assert.equal(els.postTitle.value, "", "yayınlanınca taslak silinmeli");
  els.postTitle.value = "atılacak";
  els.postTitle.fire("input");
  els.postCancel.click();
  els.forumYeniBtn.click();
  assert.equal(els.postTitle.value, "", "Vazgeç taslağı atmalı");
});

test("panel forum: konu yoksa ya da silindiyse ilgili not görünür", () => {
  const { els, send } = boot();
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  send({ type: "postGone", id: "k2" });
  assert.ok(els.forumSag.innerHTML.includes("Başlık k1"), "başka konunun gidişi açık konuyu etkilemez");
  send({ type: "postGone", id: "k1" });
  assert.match(els.forumSag.innerHTML, /artık yok/);
  send({ type: "postGone", id: "zz", ac: true });
  assert.match(els.forumSag.innerHTML, /artık yok/);
  send({ type: "postDetail", ac: true, data: ayrinti("k3") });
  send({ type: "postDeleted", id: "k3" });
  assert.match(els.forumSag.innerHTML, /soldan bir konu seç/);
});

test("panel forum: liste vermeyen eski AND'da ID kutusu görünür", () => {
  const { els, posted, send } = boot();
  send({ type: "forumPosts", liste: false, konular: [], kategoriler: [] });
  assert.equal(els.forumEski.style.display, "");
  assert.equal(els.forumKonular.style.display, "none");
  posted.length = 0;
  els.viewPostId.value = " abc ";
  els.viewPostBtn.click();
  assert.deepEqual(posted, [{ type: "forumOpen", id: "abc" }]);
  send({ type: "forumPosts", liste: true, konular: [ozet("k1")], kategoriler: [{ kategori: "Go", sayi: 1 }] });
  assert.equal(els.forumEski.style.display, "none");
  assert.equal(els.forumKonular.style.display, "");
});

test("panel forum: profil kaçırılır; moderatöre yasak formu, kendine ve üyeye yok", () => {
  const { els, el, posted, send } = boot();
  send({ type: "identity", data: { name: "ben", peer_id: "12D3", pub_key: "b0" } });
  send({ type: "role", data: { is_founder: false, is_moderator: true } });
  send({ type: "postDetail", ac: true, data: ayrinti("k1") });
  send({
    type: "profile",
    data: {
      key: "a1",
      name: "<ali>",
      role: "moderator",
      post_count: 2,
      reply_count: 5,
      first_seen: "2026-11-01T10:00:00Z",
      status: "meşgul",
      bio: "merhaba",
      tags: ["Go"],
      link: "https://ornek.dev",
      github_user: "ali",
      ban: { permanent: true, reason: "spam" },
      site_pages: [{ slug: "cv", title: "Özgeçmiş", body: "", updated_at: "2026-11-02T10:00:00Z" }],
    },
  });
  const sag = els.forumSag.innerHTML;
  for (const parca of ["&lt;ali&gt;", "Moderatör", "2 konu", "5 yanıt", "meşgul", "merhaba", "https://ornek.dev", "ali", "Özgeçmiş", "/cv", "spam", "Konuya dön"]) {
    assert.ok(sag.includes(parca), `${parca} yok: ${sag}`);
  }
  assert.ok(!sag.includes("<ali>"));
  assert.ok(sag.includes('id="banGonder"'));

  posted.length = 0;
  el("banSebep").value = "  ";
  els.banGonder.click();
  assert.deepEqual(posted, []);
  assert.match(els.banner.textContent, /Sebep boş olamaz/);
  el("banSure").value = "604800";
  el("banSebep").value = " reklam ";
  els.banGonder.click();
  el("banSure").value = "kalici";
  els.banGonder.click();
  assert.deepEqual(posted, [
    { type: "ban", key: "a1", name: "<ali>", seconds: 604800, reason: "reklam" },
    { type: "ban", key: "a1", name: "<ali>", permanent: true, reason: "reklam" },
  ]);

  els.profilGeri.click();
  assert.ok(els.forumSag.innerHTML.includes("Başlık k1"), "Konuya dön konuyu göstermeli");

  send({ type: "profile", data: { key: "B0", name: "ben", post_count: 0, reply_count: 0 } });
  assert.ok(!els.forumSag.innerHTML.includes('id="banGonder"'), "kendini yasaklama formu olmamalı");
  send({ type: "role", data: { is_founder: false, is_moderator: false } });
  send({ type: "profile", data: { key: "a1", name: "ali", post_count: 0, reply_count: 0 } });
  assert.ok(!els.forumSag.innerHTML.includes('id="banGonder"'), "üyeye yasak formu olmamalı");
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

/** HTML'in kullanıcıya görünen metni: etiket dışı metin ve okunan öznitelikler (stil ve betik hariç). */
function gorunenMetin(html: string): string {
  const govde = html.replace(/<style[\s\S]*?<\/style>/g, " ").replace(/<script[\s\S]*?<\/script>/g, " ");
  const oznitelikler = [...govde.matchAll(/\s(?:placeholder|aria-label|title|data-empty)="([^"]*)"/g)].map((m) => m[1]);
  return [govde.replace(/<[^>]*>/g, " "), ...oznitelikler].join("\n");
}

/** Çevirisi kaynaktan farklı olan bir katalog metni bu dilin ekranında kelime olarak geçiyorsa kalıntıdır. */
function kalintilar(kod: string, metin: string): string[] {
  // Yalnızca Türkçeye özgü harfler: ö/ü Almancada, ç Fransızcada da vardır; onları katalog denetimi yakalar.
  const bulunan: string[] = [...(metin.match(/\S*[ğışĞİŞ]\S*/g) ?? [])];
  const katalog = gomulu();
  // Bu dilin kendi çevirileri: kaynak metin orada da kelime olarak geçiyorsa (fr "Site") o dilin kelimesidir.
  const dilinMetinleri = Object.values(katalog)
    .map((d) => d[kod] ?? "")
    .join("\n");
  for (const [kaynak, diller] of Object.entries(katalog)) {
    const sade = kaynak.replace(/%(\[\d+\])?[sd]/g, "").trim();
    if (sade.length < 4 || !diller[kod] || diller[kod] === kaynak) {
      continue;
    }
    const kacik = sade.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const kelime = new RegExp(`(^|[^\\p{L}])${kacik}($|[^\\p{L}])`, "u");
    if (kelime.test(metin) && !kelime.test(dilinMetinleri)) {
      bulunan.push(kaynak);
    }
  }
  return bulunan;
}

test("panel: her dilde HTML, betik çıktısı, kategori ve tema adları çevrilir", () => {
  try {
    for (const kod of ["en", "de", "es", "fr", "ru"]) {
      ayarla(kod);
      const { html, els, send } = boot();
      assert.ok(html.includes(`<html lang="${kod}">`), kod);
      assert.deepEqual(kalintilar(kod, gorunenMetin(html)), [], `${kod}: HTML'de çevrilmemiş metin`);

      send({ type: "connectionState", connected: false });
      send({ type: "identity", data: { name: "", peer_id: "12D3KooABC", pub_key: "a3f9" } });
      send({ type: "role", data: { is_founder: true, is_moderator: false } });
      send({
        type: "forumPosts",
        liste: true,
        konular: [{ id: "k1", title: "T", category: "Yapay Zeka", author_name: "z", author_key: "z1", reply_count: 2, author_role: "founder", created_at: new Date(Date.now() - 7200_000).toISOString() }],
        kategoriler: [{ kategori: "Yapay Zeka", sayi: 1 }],
      });
      const liste = [els.forumKonular.innerHTML, els.forumKategori.innerHTML].join("\n");
      send({
        type: "postDetail",
        ac: true,
        data: { id: "k1", title: "T", category: "Yapay Zeka", author_name: "z", author_key: "a3f9", body: "g", replies: [{ author_name: "a", body: "y" }, { author_name: "b", body: "x" }] },
      });
      const konu = els.forumSag.innerHTML;
      els.forumYeniBtn.click();
      const yeniKonu = els.forumSag.innerHTML;
      send({
        type: "profile",
        data: { key: "z1", name: "z", role: "moderator", post_count: 1, reply_count: 3, first_seen: "2026-11-01T10:00:00Z", tags: ["Yapay Zeka"], ban: { expires_at: "2026-12-01T10:00:00Z", reason: "r" }, site_pages: [{ slug: "s", title: "S" }] },
      });
      const profil = els.forumSag.innerHTML;
      send({ type: "postGone", id: "x", ac: true });
      const gitti = els.forumSag.innerHTML;
      send({ type: "theme", data: { themeName: "Gece Moru" } });
      send({ type: "sitePages", data: { max: 20, maxBody: 8000, pages: [] } });
      send({ type: "dmData", data: { konusmalar: [], gecmisYok: true } });
      const cikti = [
        els.connBadge.textContent,
        els.identityBody.innerHTML,
        liste,
        konu,
        yeniKonu,
        profil,
        gitti,
        els.themeName.textContent,
        els.siteBody.innerHTML,
        els.dmLede.textContent,
      ].join("\n");
      assert.deepEqual(kalintilar(kod, gorunenMetin(cikti)), [], `${kod}: betik çıktısında çevrilmemiş metin`);
      assert.ok(konu.includes(`<h3>${t("%d yanıt", 2)}</h3>`), `${kod}: çoğul biçim`);
      assert.ok(konu.includes(t("Yapay Zeka")) && liste.includes(t("Yapay Zeka")), `${kod}: kategori adı`);
      assert.equal(els.themeName.textContent, t("%s teması", t("Gece Moru")), `${kod}: tema adı`);
      // AND'ın bütün hazır temaları ve özel renk (ui/theme.go, ui/ozel.go).
      for (const ad of ["Klasik", "Turkuaz", "Gece Moru", "Kehribar", "Orman", "Gül", "Okyanus", "Gün Batımı", "Lavanta", "Mono", "Özel"]) {
        send({ type: "theme", data: { themeName: ad } });
        assert.equal(els.themeName.textContent, t("%s teması", t(ad)), `${kod}: ${ad} teması`);
        assert.notEqual(t(ad), ad === "Mono" ? "" : ad, `${kod}: ${ad} çevrilmeli`);
      }
    }
    // Tekil/çoğul biçimi betikte sayıya göre seçilir: İngilizcede 1 tekil, Fransızcada 0 tekil, Rusçada üç biçim.
    const yanitlar = (n: number) => Array.from({ length: n }, () => ({ author_name: "a", body: "y" }));
    const durumlar: [string, number, string][] = [
      ["en", 1, "1 reply"], ["en", 2, "2 replies"],
      ["fr", 1, "1 réponse"], ["fr", 2, "2 réponses"],
      ["ru", 1, "1 ответ"], ["ru", 3, "3 ответа"], ["ru", 5, "5 ответов"], ["ru", 21, "21 ответ"],
    ];
    for (const [kod, n, beklenen] of durumlar) {
      ayarla(kod);
      const { els, send } = boot();
      send({ type: "postDetail", ac: true, data: { id: "k", title: "T", category: "Go", author_name: "z", body: "g", replies: yanitlar(n) } });
      assert.ok(els.forumSag.innerHTML.includes(`<h3>${beklenen}</h3>`), `${kod} ${n}: ${els.forumSag.innerHTML}`);
    }
  } finally {
    ayarla(KAYNAK);
  }
});
