// AND masaüstü arayüzü.
//
// Güvenlik: ağdan gelen hiçbir metin HTML olarak yorumlanmaz. Bütün öğeler
// el() ile kurulur; metinler her zaman metin düğümü (textContent) olur.
// Metni HTML olarak işleyen DOM özellikleri hiç kullanılmaz (bkz.
// TestArayuzHTMLYorumlamaz). Simgeler bu dosyadaki sabit SVG yollarıdır.
//
// Metinler Türkçe kaynak metinlerdir; t() onları sunucunun gönderdiği etkin
// dildeki çevirileriyle değiştirir (bkz. internal/masaustu/metinler.go).

"use strict";

(() => {
  // ── Temel yardımcılar ─────────────────────────────────────────────────

  let M = {};
  let DIL = "tr";
  const t = (s) => M[s] || s;
  // tb, küçük harfle başlayan bir kaynak metni (terminal arayüzünün kısayol
  // metinleri) çevirir ve ilk harfini etkin dilin kurallarıyla büyütür.
  const tb = (s) => {
    const c = t(s);
    return c.charAt(0).toLocaleUpperCase(DIL) + c.slice(1);
  };
  // K, çizilirken çevrilecek bir metni işaretler (metinler.go onu da toplar).
  const K = (s) => s;
  const tf = (s, ...a) => {
    let i = 0;
    return t(s).replace(/%[ds]/g, () => String(a[i++] ?? ""));
  };

  function el(etiket, ozellik, ...cocuklar) {
    const e = document.createElement(etiket);
    if (ozellik) {
      for (const [k, v] of Object.entries(ozellik)) {
        if (v === undefined || v === null || v === false) continue;
        if (k === "class") e.className = v;
        else if (k === "on") for (const [olay, f] of Object.entries(v)) e.addEventListener(olay, f);
        else if (k === "value") e.value = v;
        else if (k === "checked" || k === "disabled") e[k] = !!v;
        else e.setAttribute(k, v === true ? "" : String(v));
      }
    }
    ekle(e, cocuklar);
    return e;
  }

  function ekle(e, cocuklar) {
    for (const c of cocuklar.flat(Infinity)) {
      if (c === null || c === undefined || c === false) continue;
      e.append(c instanceof Node ? c : document.createTextNode(String(c)));
    }
    return e;
  }

  function bosalt(e, ...cocuklar) {
    e.replaceChildren();
    return ekle(e, cocuklar);
  }

  const SVG = "http://www.w3.org/2000/svg";
  const IKONLAR = {
    forum: "M3 6.5A2.5 2.5 0 0 1 5.5 4h9A2.5 2.5 0 0 1 17 6.5v5a2.5 2.5 0 0 1-2.5 2.5H9l-4 3v-3.1a2.5 2.5 0 0 1-2-2.4zM20 9.5v6a2.5 2.5 0 0 1-2 2.45V21l-3.5-3H11",
    sohbet: "M21 12a8.5 8.5 0 0 1-12.4 7.55L4 21l1.45-4.6A8.5 8.5 0 1 1 21 12zM8.5 12h.01M12.5 12h.01M16.5 12h.01",
    eklenti: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z",
    ayarlar: "M4 6h9M19 6h1M4 12h3M11 12h9M4 18h11M19 18h1M13 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0M5 12a3 3 0 1 0 6 0a3 3 0 1 0-6 0M13 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0",
    bilgi: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM12 11v5M12 7.5h.01",
    guc: "M12 3v8M6.3 6.3a8 8 0 1 0 11.4 0",
    arti: "M12 5v14M5 12h14",
    gonder: "M4 12L20 4l-6 16-3-7-7-1zM11 13l9-9",
    kopyala: "M9 9h10v10H9zM5 15V5h10",
    ara: "M11 4a7 7 0 1 0 0 14a7 7 0 1 0 0-14zM20 20l-4-4",
    dis: "M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5",
    geri: "M15 5l-7 7 7 7",
    kilit: "M6 11h12v9H6zM8.5 11V8a3.5 3.5 0 0 1 7 0v3",
    kure: "M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM3 12h18M12 3c2.5 2.6 3.8 5.6 3.8 9s-1.3 6.4-3.8 9c-2.5-2.6-3.8-5.6-3.8-9S9.5 5.6 12 3z",
    ag: "M10 5a2 2 0 1 0 4 0a2 2 0 1 0-4 0M3 18a2 2 0 1 0 4 0a2 2 0 1 0-4 0M17 18a2 2 0 1 0 4 0a2 2 0 1 0-4 0M11 6.8l-5 9.4M13 6.8l5 9.4M7 18h10",
    profil: "M12 12a4 4 0 1 0 0-8a4 4 0 1 0 0 8zM4 21a8 8 0 0 1 16 0",
    palet: "M12 3a9 9 0 1 0 0 18c1.1 0 1.5-.8 1.5-1.6 0-1.4-1.2-1.4-1.2-2.6 0-.9.7-1.6 1.6-1.6H16a5 5 0 0 0 5-5C21 6.4 17 3 12 3zM7.5 11h.01M10 7.5h.01M15 7.5h.01",
    dil: "M4 5h8M8 3v2M10.5 5c-.8 3.5-3 6.3-6.5 8M6 9c1.2 1.8 3 3.2 5 4M13 21l4-9 4 9M14.5 18h5",
    kalkan: "M12 3l7 3v5c0 4.5-3 8.3-7 10-4-1.7-7-5.5-7-10V6z",
    belge: "M7 3h7l5 5v13H7zM14 3v5h5M10 13h6M10 17h6",
    tamam: "M5 12.5l4.5 4.5L19 7",
    uyari: "M12 4l9 16H3zM12 10v4M12 17h.01",
    kapat: "M6 6l12 12M18 6L6 18",
    asagi: "M6 9l6 6 6-6",
    imza: "M4 17c3-1 4-6 6-6s1 5 3 5 3-3 4-3 1 2 3 2M4 21h16",
    yildiz: "M12 3l2.6 5.6L20.5 9l-4.4 4 1.2 6L12 16.2 6.7 19l1.2-6L3.5 9l5.9-.4z",
    muhur: "M12 3l2.2 1.6 2.7-.2.9 2.6 2.2 1.6-.9 2.6.9 2.6-2.2 1.6-.9 2.6-2.7-.2L12 21l-2.2-1.6-2.7.2-.9-2.6-2.2-1.6.9-2.6-.9-2.6 2.2-1.6.9-2.6 2.7.2zM9 12l2 2 4-4",
  };

  function svgEl(etiket, ozellik, ...cocuklar) {
    const e = document.createElementNS(SVG, etiket);
    for (const [k, v] of Object.entries(ozellik || {})) e.setAttribute(k, String(v));
    for (const c of cocuklar.flat()) if (c) e.append(c);
    return e;
  }

  // agHaritasi, giriş ekranının tek gösterişli öğesidir: AND ağının bir
  // metro hattı şeması gibi çizimi. Sen bir duraksın, eşler öteki duraklar;
  // hatlar açılışta bir kez çizilir (hareket azaltılmışsa çizili gelir).
  const HATLAR = [
    "M30 300H150L230 220H390L470 140V60",
    "M230 40V330",
    "M60 110H330L410 190H490",
  ];
  const DURAKLAR = [[30, 300], [150, 300], [310, 220], [470, 60], [230, 40], [230, 330], [60, 110], [140, 110], [330, 110], [490, 190]];
  const AKTARMALAR = [[230, 110], [420, 190]];
  function agHaritasi() {
    return svgEl("svg", { class: "ag-haritasi", viewBox: "0 0 520 360", "aria-hidden": "true" },
      HATLAR.map((d, i) => svgEl("path", { class: "hat hat-" + (i + 1), d, pathLength: 1 })),
      DURAKLAR.map(([x, y]) => svgEl("circle", { class: "durak", cx: x, cy: y, r: 6 })),
      AKTARMALAR.map(([x, y]) => svgEl("circle", { class: "durak aktarma", cx: x, cy: y, r: 9 })),
      svgEl("circle", { class: "dalga", cx: 230, cy: 220, r: 12 }),
      svgEl("circle", { class: "sen", cx: 230, cy: 220, r: 12 }));
  }

  function ikon(ad, sinif) {
    const s = document.createElementNS(SVG, "svg");
    s.setAttribute("viewBox", "0 0 24 24");
    s.setAttribute("fill", "none");
    s.setAttribute("stroke", "currentColor");
    s.setAttribute("stroke-width", "1.8");
    s.setAttribute("stroke-linecap", "round");
    s.setAttribute("stroke-linejoin", "round");
    s.setAttribute("aria-hidden", "true");
    if (sinif) s.setAttribute("class", sinif);
    const p = document.createElementNS(SVG, "path");
    p.setAttribute("d", IKONLAR[ad] || "");
    s.append(p);
    return s;
  }

  async function api(yol, govde) {
    const ayar = govde === undefined
      ? { method: "GET", credentials: "same-origin" }
      : { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-AND": "1" }, body: JSON.stringify(govde) };
    let r;
    try {
      r = await fetch(yol, ayar);
    } catch {
      throw new Error(t("AND'a ulaşılamıyor."));
    }
    let veri = null;
    try { veri = await r.json(); } catch { /* boş yanıt */ }
    if (!r.ok) {
      const e = new Error((veri && veri.hata) || t("Bir şeyler ters gitti."));
      e.durum = r.status;
      throw e;
    }
    return veri;
  }

  // ── Bildirimler ve pencereler ─────────────────────────────────────────

  function bildir(metin, tur = "bilgi") {
    if (!metin) return;
    const kutu = document.getElementById("bildirimler");
    const b = el("div", { class: "bildirim " + tur }, ikon(tur === "iyi" ? "tamam" : tur === "hata" ? "uyari" : "bilgi"), el("div", null, metin));
    kutu.append(b);
    setTimeout(() => b.remove(), tur === "hata" ? 7000 : 4000);
  }

  function pencereAc({ baslik, govde, dugmeler, genis, kapanabilir = true, kapaninca }) {
    const katman = document.getElementById("katman");
    const onceki = document.activeElement;
    let kapandi = false;
    const kapat = () => {
      if (kapandi) return;
      kapandi = true;
      perde.remove();
      document.removeEventListener("keydown", tus);
      if (onceki && onceki.focus) onceki.focus();
      if (kapaninca) kapaninca();
    };
    const tus = (e) => { if (e.key === "Escape" && kapanabilir) kapat(); };
    const pencere = el("div", { class: "pencere" + (genis ? " genis" : ""), role: "dialog", "aria-modal": "true", "aria-label": baslik },
      el("div", { class: "pencere-ust" }, el("h2", null, baslik),
        kapanabilir ? el("button", { class: "ikon-dugme", "aria-label": t("Kapat"), on: { click: kapat } }, ikon("kapat")) : null),
      el("div", { class: "pencere-govde" }, govde),
      dugmeler && dugmeler.length ? el("div", { class: "pencere-alt" }, dugmeler) : null);
    const perde = el("div", { class: "perde", on: { mousedown: (e) => { if (e.target === perde && kapanabilir) kapat(); } } }, pencere);
    katman.append(perde);
    document.addEventListener("keydown", tus);
    setTimeout(() => {
      const ilk = pencere.querySelector("input, textarea, select, .pencere-alt .birincil");
      if (ilk) ilk.focus();
    }, 0);
    return kapat;
  }

  function onayla(baslik, govde, tamamMetni, tehlike) {
    return new Promise((coz) => {
      let sonuc = false;
      const kapat = pencereAc({
        baslik,
        govde: el("div", { class: "paragraf soluk" }, govde),
        dugmeler: [
          el("button", { class: "dugme", on: { click: () => kapat() } }, tb("vazgeç")),
          el("button", { class: "dugme " + (tehlike ? "tehlike" : "birincil"), on: { click: () => { sonuc = true; kapat(); } } }, tamamMetni),
        ],
        kapaninca: () => coz(sonuc),
      });
    });
  }

  async function kopyala(metin) {
    try {
      await navigator.clipboard.writeText(metin);
      bildir(t("Panoya kopyalandı."), "iyi");
    } catch {
      bildir(t("Panoya kopyalanamadı."), "hata");
    }
  }

  // ── Zaman ─────────────────────────────────────────────────────────────

  function neZaman(iso) {
    const d = new Date(iso);
    if (isNaN(d)) return "";
    const fark = (d.getTime() - Date.now()) / 1000;
    const rtf = new Intl.RelativeTimeFormat(DIL, { numeric: "auto" });
    const mutlak = Math.abs(fark);
    if (mutlak < 45) return rtf.format(0, "second");
    if (mutlak < 3600) return rtf.format(Math.round(fark / 60), "minute");
    if (mutlak < 86400) return rtf.format(Math.round(fark / 3600), "hour");
    if (mutlak < 7 * 86400) return rtf.format(Math.round(fark / 86400), "day");
    return d.toLocaleDateString(DIL, { day: "numeric", month: "short", year: d.getFullYear() === new Date().getFullYear() ? undefined : "numeric" });
  }
  const tamZaman = (iso) => new Date(iso).toLocaleString(DIL, { dateStyle: "long", timeStyle: "short" });
  const saat = (iso) => new Date(iso).toLocaleTimeString(DIL, { hour: "2-digit", minute: "2-digit" });
  const gun = (iso) => new Date(iso).toLocaleDateString(DIL, { weekday: "long", day: "numeric", month: "long" });

  function zamanEtiketi(iso, sinif) {
    return el("time", { class: sinif, datetime: iso, title: tamZaman(iso), "data-zaman": iso }, neZaman(iso));
  }
  setInterval(() => {
    for (const z of document.querySelectorAll("[data-zaman]")) z.textContent = neZaman(z.getAttribute("data-zaman"));
  }, 30000);

  // ── İçerik ────────────────────────────────────────────────────────────

  const RENKLER = ["#5f5fff", "#8b5cf6", "#ec4899", "#f97316", "#14b8a6", "#0ea5e9", "#22c55e", "#eab308", "#ef4444", "#6366f1"];
  function avatar(ad, anahtar, boyut) {
    let h = 0;
    for (const c of ((anahtar || "").slice(0, 8) || ad || "?")) h = (h * 31 + c.codePointAt(0)) >>> 0;
    const harf = [...(ad || "?").trim()][0] || "?";
    const a = el("div", { class: "avatar" + (boyut ? " " + boyut : ""), "aria-hidden": "true" }, harf);
    a.style.setProperty("background", RENKLER[h % RENKLER.length]);
    return a;
  }

  const BAGLANTI = /\bhttps?:\/\/[^\s<>"'`]+[^\s<>"'`.,;:!?)\]]/g;
  function metinDugumleri(metin) {
    const out = [];
    let son = 0;
    for (const m of metin.matchAll(BAGLANTI)) {
      if (m.index > son) out.push(metin.slice(son, m.index));
      const adres = m[0];
      out.push(el("a", { href: "#", title: adres, on: { click: (e) => { e.preventDefault(); disariAc(adres); } } }, adres));
      son = m.index + adres.length;
    }
    if (son < metin.length) out.push(metin.slice(son));
    return out;
  }

  async function disariAc(adres) {
    const govde = [el("p", { class: "paragraf" }, t("Bu bağlantı varsayılan tarayıcında açılsın mı?")),
      el("div", { class: "kimlik-ozeti es-aralikli" }, adres)];
    if (await onayla(t("Bağlantıyı aç"), govde, tb("aç"))) {
      try { await api("/api/disari", { adres }); } catch (e) { bildir(e.message, "hata"); }
    }
  }

  function govdeCiz(parcalar) {
    return (parcalar || []).map((p) => {
      if (!p.kod) return el("div", { class: "metin" }, metinDugumleri(p.metin.replace(/^\n+|\n+$/g, "")));
      return el("div", { class: "kod-blogu" },
        el("div", { class: "kod-ust" }, el("span", null, p.dil || t("kod")),
          el("button", { class: "ikon-dugme", title: t("Kopyala"), "aria-label": t("Kopyala"), on: { click: () => kopyala(p.metin) } }, ikon("kopyala"))),
        el("pre", null, el("code", null, p.metin)));
    });
  }

  // yazarSatiri: ad (genel anahtarın kısası üzerine gelince), rol, zaman ve
  // imza mührü. AND, ağdan gelen her konu ve yanıtın imzasını saklamadan önce
  // doğrular; mühür bunu söyler.
  function yazarSatiri(o, ek) {
    return el("div", { class: "yazar-satiri" },
      el("span", { class: "ad" + (o.kendi ? " kendi" : ""), title: o.yazarKisa ? t("Genel anahtar") + ": " + o.yazarKisa : undefined }, o.yazar || t("adsız")),
      o.rol ? el("span", { class: "rozet rol" }, o.rol) : null,
      zamanEtiketi(o.zaman, "zaman"),
      el("span", { class: "imza", title: t("Her konu ve mesaj Ed25519 ile imzalanır") }, ikon("muhur")),
      ek);
  }

  // ── Durum ─────────────────────────────────────────────────────────────

  const D = {
    asama: "",
    surum: "",
    diller: [],
    oturum: null,
    durum: {},
    gorunum: "forum",
    forum: { kategori: "", secili: null, veri: null, konu: null, arama: "", taslaklar: {} },
    sohbet: { iletiler: [], sinir: 500, yeniVar: false },
    ayarSekmesi: "profil",
    hakkindaSekmesi: "genel",
    kurallarGerekli: false,
  };
  const kok = () => document.getElementById("kok");

  function temaUygula(tema) {
    if (!tema) return;
    const r = document.documentElement.style;
    r.setProperty("--vurgu", tema.vurgu);
    r.setProperty("--vurgu-2", tema.secimZemin);
    r.setProperty("--secim-yazi", tema.secimYazi);
    r.setProperty("--isim", tema.isim);
    r.setProperty("--rozet", tema.rozet);
  }

  function metinleriUygula(veri) {
    M = veri.metinler || {};
    DIL = veri.dil || "tr";
    document.documentElement.lang = DIL;
  }

  // ── Açılış ────────────────────────────────────────────────────────────

  async function basla() {
    try {
      const b = await api("/api/baslangic");
      metinleriUygula(b);
      temaUygula(b.tema);
      D.asama = b.asama;
      D.surum = b.surum;
      D.diller = b.diller;
      olaylariDinle();
      if (D.asama === "uygulama") await uygulamayaGec();
      else ciz();
    } catch (e) {
      bosalt(kok(), el("div", { class: "baslatiliyor" }, el("div", null, el("h2", null, t("AND'a ulaşılamıyor.")), el("p", null, e.message))));
    }
  }

  function ciz() {
    switch (D.asama) {
      case "uygulama": return uygulamaCiz();
      case "basliyor": return baslatiliyorCiz();
      default: return girisCiz();
    }
  }

  function olaylariDinle() {
    const k = new EventSource("/api/olaylar");
    k.addEventListener("hazir", () => { if (D.asama !== "uygulama") uygulamayaGec(); });
    k.addEventListener("durum", (e) => { D.durum = JSON.parse(e.data); durumuGuncelle(); });
    k.addEventListener("tema", (e) => { temaUygula(JSON.parse(e.data)); if (D.gorunum === "ayarlar" && D.ayarSekmesi === "gorunum") sayfaCiz(); });
    k.addEventListener("sohbet", (e) => iletiGeldi(JSON.parse(e.data)));
    k.addEventListener("sohbet-gecmis", () => { if (D.gorunum === "sohbet") sohbetYukle(); });
    k.addEventListener("forum", (e) => forumOlayi(JSON.parse(e.data)));
  }

  async function uygulamayaGec() {
    try {
      D.oturum = await api("/api/oturum");
    } catch (e) {
      if (e.durum === 503) { D.asama = "basliyor"; ciz(); return; }
      bildir(e.message, "hata");
      return;
    }
    D.asama = "uygulama";
    D.kurallarGerekli = !D.oturum.kurallarOnayli;
    adrestenOku();
    ciz();
  }

  // ── Giriş ekranları ───────────────────────────────────────────────────

  function dilSecici() {
    const s = el("select", { class: "secim", "aria-label": t("Dil"), on: { change: () => diliDegistir(s.value) } },
      D.diller.map((d) => el("option", { value: d.kod, selected: d.kod === DIL }, d.ad)));
    s.value = DIL;
    return el("label", { class: "dil-secici" }, ikon("dil"), s);
  }

  async function diliDegistir(kod) {
    try {
      const r = await api("/api/dil", { kod });
      metinleriUygula(r);
      if (D.asama === "uygulama") D.oturum = await api("/api/oturum");
      ciz();
    } catch (e) {
      bildir(e.message, "hata");
    }
  }

  function tanitim() {
    return el("section", { class: "tanitim" },
      agHaritasi(),
      el("div", { class: "tanitim-metin" },
        el("div", { class: "amblem" }, el("img", { class: "logo", src: "logo.png", alt: "" }), el("h1", null, "AND")),
        el("p", { class: "slogan" }, t("Sunucusuz, eşten eşe topluluk")),
        el("ul", null,
          el("li", null, ikon("kilit"), t("Hesap ve sunucu yok; kimliğin bu cihazda üretilir")),
          el("li", null, ikon("muhur"), t("Her konu ve mesaj Ed25519 ile imzalanır")),
          el("li", null, ikon("ag"), t("İçerik eşler arasında doğrudan yayılır")))),
      el("div", { class: "surum" }, "AND " + D.surum));
  }

  function alan(etiket, girdi, sag, not) {
    const kimlik = "a" + Math.random().toString(36).slice(2, 9);
    girdi.id = kimlik;
    return el("div", { class: "alan" },
      el("div", { class: "alan-ust" }, el("label", { for: kimlik }, etiket), sag || null),
      girdi,
      not ? el("div", { class: "alan-not" }, not) : null);
  }

  function hataAlani() {
    const h = el("div", { class: "hata-kutusu gizli", role: "alert" });
    h.goster = (m) => { if (!m) { h.classList.add("gizli"); return; } bosalt(h, ikon("uyari"), el("div", null, m)); h.classList.remove("gizli"); };
    return h;
  }

  function adimlar(etkin) {
    const ad = [t("Bilgiler"), t("Kurtarma kodu"), t("Doğrulama")];
    const out = [];
    ad.forEach((a, i) => {
      if (i) out.push(el("span", { class: "adim-cizgi" }));
      out.push(el("span", { class: "adim" + (i < etkin ? " bitti" : i === etkin ? " etkin" : "") },
        el("b", null, i < etkin ? "✓" : String(i + 1)), a));
    });
    return el("div", { class: "adimlar" }, out);
  }

  function gonderilebilir(form, dugme, f) {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (dugme.disabled) return;
      dugme.disabled = true;
      try { await f(); } finally { dugme.disabled = false; }
    });
  }

  function girisSayfasi(kart) {
    bosalt(kok(), el("div", { class: "giris" }, tanitim(), el("main", { class: "giris-alani" }, dilSecici(), kart)));
    const ilk = kart.querySelector("input");
    if (ilk) ilk.focus();
  }

  function sifreGirdisi(yer) {
    return el("input", { class: "girdi", type: "password", autocomplete: "off", placeholder: yer, spellcheck: "false" });
  }

  function gucGostergesi(girdi) {
    const etiket = el("span", { class: "alan-durum" });
    const cubuk = el("div", { class: "guc-cubugu" }, el("span"), el("span"), el("span"), el("span"));
    let zaman;
    girdi.addEventListener("input", () => {
      clearTimeout(zaman);
      zaman = setTimeout(async () => {
        try {
          const r = await api("/api/sifre-gucu", { sifre: girdi.value });
          cubuk.className = "guc-cubugu guc-" + r.puan;
          etiket.textContent = r.etiket;
        } catch { /* gösterge isteğe bağlı */ }
      }, 120);
    });
    return { etiket, cubuk };
  }

  // sifreAlani, şifre girdisini güç çubuğuyla (ve isteğe bağlı notla) birlikte kurar.
  function sifreAlani(sifre, guc, not) {
    const a = alan(t("Şifre"), sifre, guc.etiket, not);
    a.insertBefore(guc.cubuk, sifre.nextSibling);
    return a;
  }

  async function asamaIste(yol, govde) {
    const r = await api(yol, govde);
    if (r && r.asama) D.asama = r.asama;
    return r;
  }

  function girisCiz() {
    switch (D.asama) {
      case "giris": return girisFormu();
      case "kayit": return kayitFormu();
      case "kod": return kurtarmaKoduEkrani();
      case "ad": return adFormu();
      case "geri-kod": return geriKodFormu();
      case "geri-form": return geriYuklemeFormu(D.geriBilgi || {});
      default: return baslatiliyorCiz();
    }
  }

  function girisFormu() {
    const sifre = sifreGirdisi(t("şifre"));
    const hata = hataAlani();
    const dugme = el("button", { class: "dugme birincil genis", type: "submit" }, ikon("kilit"), tb("kilidi aç"));
    const form = el("form", { class: "kart" },
      el("h2", null, t("Kimliğinin kilidini aç")),
      el("p", { class: "aciklama" }, t("Kimliğin bu cihazda, şifrenle şifrelenmiş olarak saklanıyor.")),
      hata, alan(t("Şifre"), sifre), dugme,
      el("div", { class: "kart-alt" }, el("button", { class: "baglanti-dugme", type: "button", on: { click: () => geriYuklemeyeGec() } }, tb("şifremi unuttum"))));
    gonderilebilir(form, dugme, async () => {
      try {
        await asamaIste("/api/giris", { sifre: sifre.value });
        ciz();
      } catch (e) {
        hata.goster(e.message);
        sifre.value = "";
        sifre.focus();
      }
    });
    girisSayfasi(form);
  }

  function kayitFormu() {
    const ad = el("input", { class: "girdi", maxlength: "32", placeholder: t("görünen adın"), autocomplete: "nickname" });
    const sifre = sifreGirdisi(t("şifre"));
    const tekrar = sifreGirdisi(t("şifre (tekrar)"));
    const guc = gucGostergesi(sifre);
    const eslesme = el("span", { class: "alan-durum" });
    tekrar.addEventListener("input", () => {
      eslesme.textContent = !tekrar.value ? "" : tekrar.value === sifre.value ? t("✓ eşleşiyor") : t("eşleşmiyor");
    });
    const hata = hataAlani();
    const dugme = el("button", { class: "dugme birincil genis", type: "submit" }, tb("devam"));
    const form = el("form", { class: "kart" },
      adimlar(0),
      el("h2", null, t("Kimliğini oluştur")),
      el("p", { class: "aciklama" }, t("Hesap ya da sunucu yok: kimliğin bu cihazda üretilir ve şifrenle korunur.")),
      hata,
      alan(t("Görünen ad"), ad, null, t("Adın sohbette ve forumda diğer kullanıcılara gösterilir; imzalarını etkilemez.")),
      sifreAlani(sifre, guc, t("Şifre en az 8 karakter olmalı ve yalnızca bu cihazda kullanılır.")),
      alan(t("Şifre (tekrar)"), tekrar, eslesme),
      dugme,
      el("div", { class: "kart-alt" }, t("Kurtarma kodun var mı?"),
        el("button", { class: "baglanti-dugme", type: "button", on: { click: () => geriYuklemeyeGec() } }, t("Kimliğini geri yükle"))));
    gonderilebilir(form, dugme, async () => {
      try {
        const r = await asamaIste("/api/kayit", { ad: ad.value, sifre: sifre.value, tekrar: tekrar.value });
        D.kurtarmaKodu = r.kod;
        D.kodAdimi = 1;
        ciz();
      } catch (e) {
        hata.goster(e.message);
      }
    });
    girisSayfasi(form);
  }

  async function kurtarmaKoduEkrani() {
    if (!D.kurtarmaKodu) {
      try { D.kurtarmaKodu = (await api("/api/kayit/kod")).kod; } catch (e) { bildir(e.message, "hata"); return; }
    }
    if (D.kodAdimi === 2) return dogrulamaFormu();
    const geri = el("button", { class: "dugme", type: "button", on: { click: async () => {
      await asamaIste("/api/kayit/geri", {});
      D.kurtarmaKodu = "";
      ciz();
    } } }, ikon("geri"), tb("geri"));
    const devam = el("button", { class: "dugme birincil", type: "button", on: { click: () => { D.kodAdimi = 2; ciz(); } } }, tb("kaydettim"));
    const kart = el("div", { class: "kart" },
      adimlar(1),
      el("h2", null, t("Kurtarma kodunu kaydet")),
      el("p", { class: "aciklama" }, t("Bu kod, kimliğini başka bir cihazda ya da şifreni unutursan geri getirmenin tek yoludur. Kâğıda yaz ya da güvenli bir yere kaydet.")),
      el("div", { class: "kurtarma-kodu", "aria-label": D.kurtarmaKodu }, D.kurtarmaKodu.split("-").map((p) => el("span", null, p))),
      el("div", { class: "dugme-satiri" },
        el("button", { class: "dugme", type: "button", on: { click: () => kopyala(D.kurtarmaKodu) } }, ikon("kopyala"), t("Kopyala"))),
      el("div", { class: "uyari-kutusu" }, ikon("uyari"), el("div", null, t("Bu kod bir daha gösterilmeyecek."))),
      el("div", { class: "dugme-satiri" }, geri, devam));
    girisSayfasi(kart);
    devam.focus();
  }

  function dogrulamaFormu() {
    const kod = el("input", { class: "girdi es-aralikli", placeholder: "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX", autocomplete: "off", spellcheck: "false" });
    const hata = hataAlani();
    const dugme = el("button", { class: "dugme birincil", type: "submit" }, tb("doğrula ve bitir"));
    const form = el("form", { class: "kart" },
      adimlar(2),
      el("h2", null, t("Kurtarma kodunu doğrula")),
      el("p", { class: "aciklama" }, t("Kodu gerçekten kaydettiğinden emin olmak için tamamını yaz. Tireleri yazmasan da olur.")),
      hata, alan(t("Kurtarma kodu"), kod),
      el("div", { class: "dugme-satiri" },
        el("button", { class: "dugme", type: "button", on: { click: () => { D.kodAdimi = 1; ciz(); } } }, ikon("geri"), tb("koda dön")),
        dugme));
    gonderilebilir(form, dugme, async () => {
      try {
        await asamaIste("/api/kayit/dogrula", { kod: kod.value });
        D.kurtarmaKodu = "";
        ciz();
      } catch (e) {
        hata.goster(e.message);
        kod.select();
      }
    });
    girisSayfasi(form);
  }

  function adFormu() {
    const ad = el("input", { class: "girdi", maxlength: "32", placeholder: t("görünen adın") });
    const hata = hataAlani();
    const dugme = el("button", { class: "dugme birincil", type: "submit" }, tb("kaydet"));
    const form = el("form", { class: "kart" },
      el("h2", null, t("Görünen adını belirle")),
      el("p", { class: "aciklama" }, t("Adın sohbette ve forumda diğer kullanıcılara gösterilir; imzalarını etkilemez.")),
      hata, alan(t("Görünen ad"), ad),
      el("div", { class: "dugme-satiri" },
        el("button", { class: "dugme", type: "button", on: { click: async () => { await asamaIste("/api/ad", { atla: true }); ciz(); } } }, tb("şimdilik atla")),
        dugme));
    gonderilebilir(form, dugme, async () => {
      try {
        await asamaIste("/api/ad", { ad: ad.value });
        ciz();
      } catch (e) {
        hata.goster(e.message);
      }
    });
    girisSayfasi(form);
  }

  async function geriYuklemeyeGec(geri) {
    try {
      await asamaIste("/api/geri-yukleme", { geri: !!geri });
      ciz();
    } catch (e) {
      bildir(e.message, "hata");
    }
  }

  function geriKodFormu() {
    const kod = el("textarea", { class: "metin-alani es-aralikli", rows: "3", placeholder: "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX", spellcheck: "false" });
    const hata = hataAlani();
    const dugme = el("button", { class: "dugme birincil", type: "submit" }, tb("devam"));
    const form = el("form", { class: "kart" },
      el("h2", null, t("Kimliğini geri yükle")),
      el("p", { class: "aciklama" }, t("Kimliğini oluştururken kaydettiğin kurtarma kodunu yaz. Tire ve boşluk önemsiz; eski sürümlerin 12 kelimelik kodu da olur.")),
      hata, alan(t("Kurtarma kodu"), kod),
      el("div", { class: "bilgi-kutusu" }, ikon("bilgi"), el("div", null, t("Bu cihazdaki kimlik dosyası silinmez; yanına yedek olarak alınır."))),
      el("div", { class: "dugme-satiri" },
        el("button", { class: "dugme", type: "button", on: { click: () => geriYuklemeyeGec(true) } }, ikon("geri"), tb("geri")),
        dugme));
    gonderilebilir(form, dugme, async () => {
      try {
        D.geriBilgi = await asamaIste("/api/geri-yukleme/kod", { kod: kod.value });
        ciz();
      } catch (e) {
        hata.goster(e.message);
      }
    });
    girisSayfasi(form);
  }

  function geriYuklemeFormu(bilgi) {
    const ad = el("input", { class: "girdi", maxlength: "32", placeholder: t("görünen adın"), value: bilgi.ad || "" });
    const sifre = sifreGirdisi(t("şifre"));
    const tekrar = sifreGirdisi(t("şifre (tekrar)"));
    const guc = gucGostergesi(sifre);
    const hata = hataAlani();
    const uyari = el("div", { class: "uyari-kutusu gizli" });
    const dugme = el("button", { class: "dugme birincil", type: "submit" }, t("Geri yükle"));
    const form = el("form", { class: "kart" },
      el("h2", null, t("Yeni şifre belirle")),
      el("p", { class: "aciklama" }, t("Kimliğin kurtarma kodundan yeniden üretildi; bu cihazda kullanacağın şifreyi belirle.")),
      el("div", { class: "kimlik-ozeti" }, ikon("profil"),
        el("div", null, el("div", { class: "es-aralikli" }, bilgi.kimlik || ""), el("div", { class: "cok-soluk" }, bilgi.eslesmeMetni || ""))),
      hata, uyari,
      alan(t("Görünen ad"), ad),
      sifreAlani(sifre, guc),
      alan(t("Şifre (tekrar)"), tekrar),
      el("div", { class: "dugme-satiri" },
        el("button", { class: "dugme", type: "button", on: { click: () => geriYuklemeyeGec(true) } }, ikon("geri"), tb("koda dön")),
        dugme));
    gonderilebilir(form, dugme, async () => {
      hata.goster("");
      try {
        const r = await asamaIste("/api/geri-yukleme/kaydet", { ad: ad.value, sifre: sifre.value, tekrar: tekrar.value });
        if (r.onayGerekli) {
          bosalt(uyari, ikon("uyari"), el("div", null, r.uyari));
          uyari.classList.remove("gizli");
          return;
        }
        D.geriBilgi = null;
        ciz();
      } catch (e) {
        hata.goster(e.message);
      }
    });
    girisSayfasi(form);
    if (bilgi.ad) sifre.focus();
  }

  function baslatiliyorCiz() {
    bosalt(kok(), el("div", { class: "baslatiliyor" },
      el("div", null, el("div", { class: "donen" }), el("p", null, t("AND başlatılıyor…")))));
    // Canlı olay kaçırılırsa diye arada bir sorulur.
    setTimeout(async () => {
      if (D.asama === "basliyor") {
        try {
          const b = await api("/api/baslangic");
          if (b.asama === "uygulama") uygulamayaGec();
          else if (D.asama === "basliyor") baslatiliyorCiz();
        } catch { /* yeniden denenir */ }
      }
    }, 1500);
  }

  // ── Uygulama iskeleti ─────────────────────────────────────────────────

  let durumAlani = null;
  let seritAlani = null;
  let gorunumAlani = null;
  let gezinmeAlani = null;

  function uygulamaCiz() {
    durumAlani = el("footer", { class: "durum-cubugu", role: "status" });
    seritAlani = el("div");
    gorunumAlani = el("div", { class: "gorunum" });
    gezinmeAlani = el("nav", { class: "gezinme", "aria-label": t("Gezinme") });
    const o = D.oturum;
    const profileGit = () => { D.ayarSekmesi = "profil"; git("ayarlar"); };
    bosalt(kok(), el("div", { class: "uygulama" },
      el("aside", { class: "kenar" },
        el("div", { class: "marka" }, el("img", { class: "logo", src: "logo.png", alt: "" }), el("span", { class: "marka-ad" }, "AND")),
        el("button", { class: "arama-dugmesi", on: { click: hizliErisim } },
          ikon("ara"), el("span", null, t("Ara ya da git…")), el("kbd", null, "Ctrl K")),
        gezinmeAlani,
        el("div", { class: "kullanici-karti" },
          el("button", { class: "kullanici", title: t("Profil"), on: { click: profileGit } },
            avatar(o.ad, o.anahtar, "kucuk"),
            el("span", { class: "bilgi" }, el("span", { class: "ad" }, o.ad || t("adsız")), el("span", { class: "rol" }, o.rol || t("Profil")))),
          el("button", { class: "ikon-dugme", title: t("Çıkış"), "aria-label": t("Çıkış"), on: { click: cikis } }, ikon("guc")))),
      el("main", { class: "ana" }, seritAlani, gorunumAlani, durumAlani)));
    gezinmeCiz();
    durumuGuncelle();
    gorunumCiz();
    if (D.kurallarGerekli) kurallarCiz();
  }

  function gezinmeCiz() {
    const o = D.oturum;
    const nav = (ad, ikonAd, etiket, sayac) => el("button", {
      class: "nav" + (D.gorunum === ad ? " etkin" : ""), "aria-current": D.gorunum === ad ? "page" : undefined, title: etiket,
      on: { click: () => { if (ad === "forum") D.forum.kategori = ""; git(ad); } },
    }, ikon(ikonAd), el("span", null, etiket), sayac ? el("b", { class: "sayac" }, String(sayac)) : null);
    // Konusu olan kategoriler Forum'un altında kanal gibi listelenir; geri
    // kalanı forumun başlığındaki seçimdedir.
    const kategoriler = D.forum.veri ? D.forum.veri.kategoriler.filter((k) => k.sayi > 0) : [];
    const kategoriAc = (ad) => { D.forum.kategori = ad; D.forum.secili = null; git("forum"); };
    bosalt(gezinmeAlani,
      el("div", { class: "grup-basligi" }, t("Topluluk")),
      nav("forum", "forum", t("Forum")),
      kategoriler.length ? el("div", { class: "alt-gezinme" }, kategoriler.map((k) => el("button", {
        class: "alt-nav" + (D.gorunum === "forum" && D.forum.kategori === k.ad ? " etkin" : ""), title: k.etiket,
        on: { click: () => kategoriAc(k.ad) },
      }, el("span", null, k.etiket), el("span", { class: "sayi" }, String(k.sayi))))) : null,
      nav("sohbet", "sohbet", t("Sohbet"), D.sohbet.yeniVar && D.gorunum !== "sohbet" ? "•" : null),
      el("div", { class: "grup-basligi" }, t("Eklentiler")),
      (o.menu && o.menu.length)
        ? o.menu.map((e) => el("button", { class: "nav", title: tf("%s kendi penceresinde açılır", e.etiket), on: { click: () => eklentiAc(e.ad) } },
          ikon("eklenti"), el("span", null, e.etiket), ikon("dis", "dis")))
        : el("div", { class: "gezinme-bos" }, t("Etkin eklenti yok")),
      el("div", { class: "gezinme-alt" },
        nav("ayarlar", "ayarlar", t("Ayarlar")),
        nav("hakkinda", "bilgi", t("AND hakkında"))));
  }

  function durumuGuncelle() {
    if (!durumAlani) return;
    const d = D.durum || {};
    const esler = d.esler || 0;
    // Durum çubuğu: ağın durumu, eş sayısı, son senkron ve sürüm.
    const parca = (ozellik, ...c) => el("span", { class: "parca", ...ozellik }, c);
    const nokta = esler === 0 ? "uyari" : d.erisimDurum === "iyi" ? "iyi" : d.erisimDurum === "uyari" ? "uyari" : "";
    bosalt(durumAlani,
      parca({ title: t("Erişilebilirlik") }, el("span", { class: "nokta " + nokta }), esler > 0 ? (d.erisim || t("Erişilebilirlik")) : t("ağa bağlanıyor…")),
      esler > 0 ? parca(null, ikon("ag"), tf("Eşler: %d", esler)) : null,
      d.senkron ? parca(null, ikon("yenile"), t("Son senkron"), " ", zamanEtiketi(d.senkron)) : null,
      el("span", { class: "bosluk" }),
      parca(null, "AND " + ((D.oturum && D.oturum.surum) || D.surum)));
    const seritler = [];
    if (d.guncelleme) seritler.push(el("div", { class: "serit bilgi" }, ikon("yildiz"), el("span", null, d.guncelleme)));
    if (D.oturum && D.oturum.uyari && !D.uyariKapandi) {
      seritler.push(el("div", { class: "serit uyari" }, ikon("uyari"), el("span", null, D.oturum.uyari),
        el("button", { class: "ikon-dugme", "aria-label": t("Kapat"), on: { click: () => { D.uyariKapandi = true; durumuGuncelle(); } } }, ikon("kapat"))));
    }
    bosalt(seritAlani, seritler);
  }

  function git(gorunum) {
    D.gorunum = gorunum;
    if (gorunum === "sohbet") D.sohbet.yeniVar = false;
    adresiYaz();
    gezinmeCiz();
    gorunumCiz();
  }

  // Açık ekran adreste tutulur (#sohbet, #ayarlar/ag): pencere yenilenince
  // aynı ekran açılır.
  const GORUNUMLER = ["forum", "sohbet", "ayarlar", "hakkinda"];
  function adresiYaz() {
    const alt = D.gorunum === "forum" ? (D.forum.secili ? "/" + D.forum.secili : "") : D.gorunum === "ayarlar" ? "/" + D.ayarSekmesi : D.gorunum === "hakkinda" ? "/" + D.hakkindaSekmesi : "";
    history.replaceState(null, "", "#" + D.gorunum + alt);
  }
  function adrestenOku() {
    const [g, alt] = decodeURIComponent(location.hash.slice(1)).split("/");
    if (!GORUNUMLER.includes(g)) return;
    D.gorunum = g;
    if (g === "ayarlar" && AYAR_SEKMELERI.some((s) => s[0] === alt)) D.ayarSekmesi = alt;
    if (g === "hakkinda" && HAKKINDA_SEKMELERI.some((s) => s[0] === alt)) D.hakkindaSekmesi = alt;
    if (g === "forum" && alt) D.forum.secili = alt;
  }

  function gorunumCiz() {
    // Ekran değişirken kısa bir geçiş (hareket azaltılmışsa yok).
    gorunumAlani.classList.remove("gecis");
    void gorunumAlani.offsetWidth;
    gorunumAlani.classList.add("gecis");
    switch (D.gorunum) {
      case "sohbet": return sohbetCiz();
      case "ayarlar": return ayarlarCiz();
      case "hakkinda": return hakkindaCiz();
      default: return forumCiz();
    }
  }

  function ustCubuk(baslik, altBaslik, ...sag) {
    return el("header", { class: "ust-cubuk" },
      el("div", null, el("h1", null, baslik), altBaslik ? el("div", { class: "alt-baslik" }, altBaslik) : null),
      el("div", { class: "bosluk" }), sag);
  }

  function bosDurum(ikonAd, baslik, metin) {
    return el("div", { class: "bos-durum" }, ikon(ikonAd), el("h3", null, baslik), metin ? el("p", null, metin) : null);
  }

  async function cikis() {
    try { await api("/api/kapat", {}); } catch { /* pencere zaten kapanıyor */ }
    bosalt(kok(), el("div", { class: "baslatiliyor" }, el("div", null, el("img", { class: "acilis-logo", src: "logo.png", alt: "" }), el("p", null, t("AND kapanıyor…")))));
  }

  // ── Kurallar ──────────────────────────────────────────────────────────

  async function kurallarCiz() {
    let k;
    try { k = await api("/api/kurallar"); } catch (e) { bildir(e.message, "hata"); return; }
    if (k.onayli) { D.kurallarGerekli = false; return; }
    const kabul = el("button", { class: "dugme birincil", disabled: true }, ikon("tamam"), tb("okudum, kabul ediyorum"));
    const ipucu = el("span", { class: "cok-soluk sola" }, t("Kabul etmeden önce metnin sonuna kadar kaydır."));
    const govde = el("div", { class: "pencere-govde" },
      el("p", { class: "paragraf" }, k.giris),
      el("h3", null, t("Gizlilik")), el("ul", { class: "maddeler" }, k.gizlilik.map((m) => el("li", null, m))),
      el("h3", null, t("Topluluk kuralları")), el("ul", { class: "maddeler" }, k.kurallar.map((m) => el("li", null, m))),
      el("p", { class: "paragraf soluk" }, k.not));
    const sonaGeldi = () => {
      if (govde.scrollTop + govde.clientHeight >= govde.scrollHeight - 8) {
        kabul.disabled = false;
        ipucu.classList.add("gizli");
      }
    };
    govde.addEventListener("scroll", sonaGeldi);
    const katman = el("div", { class: "kurallar" },
      el("div", { class: "pencere", role: "dialog", "aria-modal": "true", "aria-label": t("Başlamadan önce") },
        el("div", { class: "pencere-ust" }, el("h2", null, t("Başlamadan önce")), el("span", { class: "rozet kategori" }, tf("kurallar sürüm %d", k.surum))),
        govde,
        el("div", { class: "pencere-alt" }, ipucu,
          el("button", { class: "dugme", on: { click: cikis } }, tb("kabul etmeden çık")), kabul)));
    kabul.addEventListener("click", async () => {
      try {
        await api("/api/kurallar", {});
        D.kurallarGerekli = false;
        katman.remove();
      } catch (e) {
        bildir(e.message, "hata");
      }
    });
    for (const eski of document.querySelectorAll(".kurallar")) eski.remove();
    document.body.append(katman);
    setTimeout(sonaGeldi, 50);
  }

  // ── Forum ─────────────────────────────────────────────────────────────

  async function forumYukle() {
    const q = D.forum.kategori ? "?kategori=" + encodeURIComponent(D.forum.kategori) : "";
    D.forum.veri = await api("/api/forum" + q);
  }

  // iskelet, yükleme uzarsa (150 ms'den çok) gösterilen yer tutucudur; kısa
  // yüklemelerde ekran hiç kırpışmaz.
  function iskeletSatirlari(n) {
    return Array.from({ length: n }, () => el("div", { class: "iskelet-satir" },
      el("span", { class: "iskelet k1" }), el("span", { class: "iskelet k2" }), el("span", { class: "iskelet k3" })));
  }

  async function forumCiz() {
    const iskelet = setTimeout(() => bosalt(gorunumAlani, el("div", { class: "forum" },
      el("div", { class: "sutun" }, el("div", { class: "sutun-ust" }, el("span", { class: "iskelet baslik-iskelet" })), iskeletSatirlari(6)),
      el("section", { class: "detay" }))), 150);
    try {
      await forumYukle();
    } catch (e) {
      clearTimeout(iskelet);
      bosalt(gorunumAlani, bosDurum("uyari", t("Forum yüklenemedi."), e.message));
      return;
    } finally {
      clearTimeout(iskelet);
    }
    if (D.gorunum !== "forum") return;
    gezinmeCiz(); // kenar çubuğundaki kategori sayıları
    const v = D.forum.veri;

    // Sütunun başlığı aynı zamanda kategori seçimidir.
    const katSecimi = el("select", { class: "baslik-secimi", "aria-label": t("Kategori"), on: { change: () => kategoriSec(katSecimi.value) } },
      el("option", { value: "" }, t("Tüm konular")),
      v.kategoriler.map((k) => el("option", { value: k.ad }, k.etiket)));
    katSecimi.value = D.forum.kategori;

    const arama = el("input", { class: "girdi", type: "search", placeholder: t("Konularda ara"), value: D.forum.arama,
      on: { input: () => { D.forum.arama = arama.value; konuListesiCiz(); } } });

    const yeniKonu = v.konuAcabilir
      ? el("button", { class: "dugme birincil kucuk", on: { click: yeniKonuPenceresi } }, ikon("arti"), tb("yeni konu"))
      : null;

    D.forum.listeAlani = el("div", { class: "kaydirma", role: "list" });
    D.forum.detayAlani = el("section", { class: "detay" });

    bosalt(gorunumAlani, el("div", { class: "forum" },
      el("div", { class: "sutun" },
        el("div", { class: "sutun-ust" }, el("label", { class: "baslik-sarmal" }, katSecimi, ikon("asagi")), yeniKonu),
        el("div", { class: "arama" }, ikon("ara"), arama),
        D.forum.listeAlani),
      D.forum.detayAlani));
    konuListesiCiz();
    if (D.forum.secili) konuAc(D.forum.secili, true);
    else bosalt(D.forum.detayAlani, bosDurum("forum", t("Bir konu seç"), t("Konuları ve yanıtları burada okursun; yanıtın eşler arasında imzalı olarak yayılır.")));
  }

  function kategoriSec(ad) {
    D.forum.kategori = ad;
    D.forum.secili = null;
    adresiYaz();
    forumCiz();
  }

  function konuListesiCiz() {
    const v = D.forum.veri;
    if (!v || !D.forum.listeAlani) return;
    const aranan = D.forum.arama.trim().toLocaleLowerCase(DIL);
    const konular = aranan ? v.konular.filter((k) => (k.baslik + " " + k.yazar).toLocaleLowerCase(DIL).includes(aranan)) : v.konular;
    if (!konular.length) {
      bosalt(D.forum.listeAlani, aranan
        ? bosDurum("ara", t("Eşleşen konu yok"))
        : bosDurum("forum", t("Henüz konu yok"), t("Eşlerden gelen konular burada görünür. Forum konuları 5 gün sonra kendiliğinden silinir.")));
      return;
    }
    bosalt(D.forum.listeAlani, konular.map((k) => el("button", {
      class: "konu-satiri" + (D.forum.secili === k.id ? " etkin" : ""), role: "listitem", "data-id": k.id,
      on: { click: () => konuAc(k.id) },
    },
      D.forum.kategori ? null : el("div", { class: "konu-kategori" }, k.kategoriAd),
      el("div", { class: "baslik" }, k.baslik),
      el("div", { class: "meta" },
        el("span", { class: "yazar" + (k.kendi ? " kendi" : "") }, k.yazar || t("adsız")),
        k.rol ? el("span", { class: "rozet rol" }, k.rol) : null,
        zamanEtiketi(k.sonEtkinlik, "zaman"),
        el("span", { class: "yanit-sayisi" + (k.yanit ? "" : " sifir"), title: t("Yanıtlar") }, ikon("sohbet"), String(k.yanit))))));
  }

  async function konuAc(id, sessiz) {
    D.forum.secili = id;
    adresiYaz();
    for (const s of D.forum.listeAlani.querySelectorAll(".konu-satiri")) s.classList.toggle("etkin", s.getAttribute("data-id") === id);
    let k;
    try {
      k = await api("/api/forum/konu?id=" + encodeURIComponent(id));
    } catch (e) {
      if (!sessiz) bildir(e.message, "hata");
      D.forum.secili = null;
      bosalt(D.forum.detayAlani, bosDurum("forum", t("Bir konu seç")));
      return;
    }
    if (D.forum.secili !== id || D.gorunum !== "forum") return;
    D.forum.konu = k;
    konuDetayCiz(k);
  }

  function konuDetayCiz(k) {
    const kaydirma = D.forum.detayAlani.querySelector(".detay-govde");
    const altta = kaydirma && kaydirma.scrollTop + kaydirma.clientHeight >= kaydirma.scrollHeight - 40;
    const eskiKonum = kaydirma ? kaydirma.scrollTop : 0;
    const ayniKonu = D.forum.cizilen === k.konu.id;

    const yazma = el("textarea", { class: "metin-alani", placeholder: t("Yanıtını yaz…"), value: D.forum.taslaklar[k.konu.id] || "" });
    const sayac = el("span");
    const gonder = el("button", { class: "dugme birincil", on: { click: () => yanitGonder(k.konu.id, yazma, gonder) } }, ikon("gonder"), tb("gönder"));
    const sayacGuncelle = () => {
      const bayt = new TextEncoder().encode(yazma.value).length;
      sayac.textContent = bayt + " / " + k.sinir;
      sayac.className = bayt > k.sinir ? "asim" : "";
      D.forum.taslaklar[k.konu.id] = yazma.value;
    };
    yazma.addEventListener("input", sayacGuncelle);
    yazma.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); gonder.click(); }
    });
    sayacGuncelle();

    // Konu bir makale gibi okunur; yanıtlar sol kenardaki bir izlek
    // çizgisiyle konuya ve birbirine bağlanır.
    const govde = el("div", { class: "detay-govde" },
      el("div", { class: "okuma" },
        el("header", { class: "detay-ust" },
          el("div", { class: "konu-kategori" }, k.konu.kategoriAd),
          el("h1", null, k.konu.baslik),
          el("div", { class: "konu-yazar" }, avatar(k.konu.yazar, k.konu.yazarKisa, "kucuk"), yazarSatiri(k.konu))),
        el("article", { class: "ana-metin" }, govdeCiz(k.govde)),
        el("h2", { class: "yanitlar-basligi" }, k.yanitlar.length ? tf("Yanıtlar (%d)", k.yanitlar.length) : t("Henüz yanıt yok")),
        k.yanitlar.length ? el("ol", { class: "izlek" }, k.yanitlar.map((y) => el("li", { class: "yanit" + (y.kendi ? " kendi" : "") },
          avatar(y.yazar, y.yazarKisa, "kucuk"),
          el("div", { class: "icerik" }, yazarSatiri(y), govdeCiz(y.govde))))) : null));

    bosalt(D.forum.detayAlani,
      govde,
      el("div", { class: "yazma" }, yazma,
        el("div", { class: "yazma-alt" }, el("span", null, t("[code]…[/code] kod bloğu; Ctrl+Enter gönderir")), el("span", { class: "bosluk" }), sayac, gonder)));
    if (ayniKonu) govde.scrollTop = altta ? govde.scrollHeight : eskiKonum;
    D.forum.cizilen = k.konu.id;
  }

  async function yanitGonder(konu, yazma, dugme) {
    const metin = yazma.value.trim();
    if (!metin) { bildir(t("Yanıt boş olamaz"), "hata"); return; }
    dugme.disabled = true;
    try {
      await api("/api/forum/yanit", { konu, metin });
      D.forum.taslaklar[konu] = "";
      await konuAc(konu, true);
      const g = D.forum.detayAlani.querySelector(".detay-govde");
      if (g) g.scrollTop = g.scrollHeight;
      bildir(t("Yanıtın gönderildi."), "iyi");
    } catch (e) {
      bildir(e.message, "hata");
    } finally {
      dugme.disabled = false;
    }
  }

  let forumYenileme = null;
  function forumOlayi(o) {
    if (D.gorunum !== "forum" || !D.forum.veri) return;
    clearTimeout(forumYenileme);
    forumYenileme = setTimeout(async () => {
      try {
        await forumYukle();
        konuListesiCiz();
        if (o.konu && o.konu === D.forum.secili) {
          const k = await api("/api/forum/konu?id=" + encodeURIComponent(o.konu));
          if (D.forum.secili === o.konu) konuDetayCiz(k);
        }
      } catch { /* bir sonraki olayda yeniden denenir */ }
    }, 400);
  }

  function yeniKonuPenceresi() {
    const v = D.forum.veri;
    const kategori = el("select", { class: "secim" },
      el("option", { value: "" }, t("Kategori seç…")),
      v.kategoriler.map((k) => el("option", { value: k.ad }, k.etiket)));
    kategori.value = D.forum.kategori;
    const baslik = el("input", { class: "girdi", maxlength: "100", placeholder: t("Konunun başlığı") });
    const baslikSayaci = el("span", { class: "alan-durum" }, "0 / 100");
    baslik.addEventListener("input", () => { baslikSayaci.textContent = [...baslik.value].length + " / 100"; });
    const metin = el("textarea", { class: "metin-alani", rows: "9", placeholder: t("Ne paylaşmak istiyorsun?") });
    const hata = hataAlani();
    const ac = el("button", { class: "dugme birincil" }, ikon("gonder"), t("Konuyu aç"));
    const kapat = pencereAc({
      baslik: tb("yeni konu"), genis: true,
      govde: [hata, alan(t("Kategori"), kategori), alan(t("Başlık"), baslik, baslikSayaci),
        alan(t("İçerik"), metin, null, t("[code]…[/code] kod bloğu; Ctrl+Enter gönderir"))],
      dugmeler: [el("button", { class: "dugme", on: { click: () => kapat() } }, tb("vazgeç")), ac],
    });
    metin.addEventListener("keydown", (e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); ac.click(); } });
    ac.addEventListener("click", async () => {
      ac.disabled = true;
      try {
        const r = await api("/api/forum/konu", { kategori: kategori.value, baslik: baslik.value, metin: metin.value });
        kapat();
        bildir(t("Konun açıldı."), "iyi");
        D.forum.secili = r.id;
        forumCiz();
      } catch (e) {
        hata.goster(e.message);
      } finally {
        ac.disabled = false;
      }
    });
  }

  // ── Sohbet ────────────────────────────────────────────────────────────

  async function sohbetYukle() {
    const r = await api("/api/sohbet");
    D.sohbet.iletiler = r.iletiler || [];
    D.sohbet.sinir = r.sinir || 500;
    if (D.gorunum === "sohbet") iletileriCiz(true);
  }

  function sohbetCiz() {
    D.sohbet.liste = el("div", { class: "iletiler", role: "log", "aria-live": "polite" });
    const girdi = el("input", { class: "girdi", placeholder: t("Herkese açık bir ileti yaz…"), maxlength: String(D.sohbet.sinir || 500) });
    const sayac = el("span", { class: "sayac" });
    const gonder = el("button", { class: "dugme birincil", "aria-label": tb("gönder") }, ikon("gonder"), tb("gönder"));
    const sayacGuncelle = () => { sayac.textContent = [...girdi.value].length + " / " + (D.sohbet.sinir || 500); };
    girdi.addEventListener("input", sayacGuncelle);
    const gonderF = async () => {
      const metin = girdi.value.trim();
      if (!metin || gonder.disabled) return;
      gonder.disabled = true;
      try {
        const m = await api("/api/sohbet", { metin });
        girdi.value = "";
        sayacGuncelle();
        iletiGeldi(m, true);
      } catch (e) {
        bildir(e.message, "hata");
      } finally {
        gonder.disabled = false;
        girdi.focus();
      }
    };
    girdi.addEventListener("keydown", (e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); gonderF(); } });
    gonder.addEventListener("click", gonderF);
    D.sohbet.yeniDugmesi = el("button", { class: "dugme kucuk birincil yeni-iletiler gizli", on: { click: () => asagiKaydir() } }, ikon("asagi"), t("Yeni iletiler"));
    bosalt(gorunumAlani,
      ustCubuk(t("Sohbet"), t("Herkese açık genel sohbet; iletiler 48 saat sonra silinir.")),
      el("div", { class: "sohbet" }, D.sohbet.liste, D.sohbet.yeniDugmesi, el("div", { class: "sohbet-yaz" }, girdi, sayac, gonder)));
    D.sohbet.liste.addEventListener("scroll", () => { if (alttaMi()) D.sohbet.yeniDugmesi.classList.add("gizli"); });
    sayacGuncelle();
    iletileriCiz(true);
    sohbetYukle().catch((e) => bildir(e.message, "hata"));
    girdi.focus();
  }

  const alttaMi = () => {
    const l = D.sohbet.liste;
    return !l || l.scrollTop + l.clientHeight >= l.scrollHeight - 60;
  };
  const asagiKaydir = () => {
    if (D.sohbet.liste) D.sohbet.liste.scrollTop = D.sohbet.liste.scrollHeight;
    if (D.sohbet.yeniDugmesi) D.sohbet.yeniDugmesi.classList.add("gizli");
  };

  function iletiOgesi(m, onceki) {
    const ayniGun = onceki && new Date(onceki.zaman).toDateString() === new Date(m.zaman).toDateString();
    const devam = onceki && ayniGun && onceki.yazar === m.yazar && onceki.yazarKisa === m.yazarKisa &&
      new Date(m.zaman) - new Date(onceki.zaman) < 5 * 60 * 1000;
    const parcalar = [];
    if (!ayniGun) parcalar.push(el("div", { class: "gun-ayraci" }, gun(m.zaman)));
    parcalar.push(el("div", { class: "ileti" + (m.kendi ? " kendi" : "") + (devam ? " devam" : "") },
      avatar(m.yazar, m.yazarKisa),
      el("div", { class: "govde" },
        devam ? null : el("div", { class: "ust" },
          el("span", { class: "ad" }, m.yazar || t("adsız")),
          m.rol ? el("span", { class: "rozet rol" }, m.rol) : null,
          el("time", { class: "saat", datetime: m.zaman, title: tamZaman(m.zaman) }, saat(m.zaman)),
          m.imzasiz ? el("span", { class: "imzasiz", title: t("Bu ileti imzasız: kimden geldiği doğrulanamaz.") }, t("imzasız")) : null),
        el("div", { class: "metin", title: devam ? saat(m.zaman) : undefined }, metinDugumleri(m.metin)))));
    return parcalar;
  }

  function iletileriCiz(asagiyaIn) {
    const l = D.sohbet.liste;
    if (!l) return;
    const iletiler = D.sohbet.iletiler;
    if (!iletiler.length) {
      bosalt(l, bosDurum("sohbet", t("Henüz ileti yok"), t("İlk iletiyi sen yaz; bağlı eşlere hemen ulaşır.")));
      return;
    }
    bosalt(l, iletiler.map((m, i) => iletiOgesi(m, iletiler[i - 1])));
    if (asagiyaIn) asagiKaydir();
  }

  function iletiGeldi(m, kendi) {
    const iletiler = D.sohbet.iletiler;
    if (m.id && iletiler.some((x) => x.id === m.id)) return;
    iletiler.push(m);
    if (D.gorunum !== "sohbet") {
      if (!m.kendi) { D.sohbet.yeniVar = true; gezinmeCiz(); }
      return;
    }
    const l = D.sohbet.liste;
    if (!l) return;
    const altta = alttaMi();
    if (iletiler.length === 1) { iletileriCiz(true); return; }
    ekle(l, iletiOgesi(m, iletiler[iletiler.length - 2]));
    if (altta || kendi || m.kendi) asagiKaydir();
    else D.sohbet.yeniDugmesi.classList.remove("gizli");
  }

  // ── Ayarlar ───────────────────────────────────────────────────────────

  const AYAR_SEKMELERI = [
    ["profil", "profil", K("Profil")],
    ["gorunum", "palet", K("Görünüm")],
    ["dil", "dil", K("Dil")],
    ["eklentiler", "eklenti", K("Eklentiler")],
    ["ag", "ag", K("Ağ")],
  ];

  let sayfaAlani = null;
  let agZamanlayici = null;

  function sekmeler(liste, etkin, sec) {
    return el("div", { class: "sekmeler", role: "tablist" }, liste.map(([ad, ikonAd, etiket]) =>
      el("button", { class: "sekme" + (etkin === ad ? " etkin" : ""), role: "tab", "aria-selected": etkin === ad ? "true" : "false", on: { click: () => sec(ad) } },
        ikonAd ? ikon(ikonAd) : null, t(etiket))));
  }

  function ayarlarCiz() {
    sayfaAlani = el("div", { class: "sayfa" });
    bosalt(gorunumAlani,
      ustCubuk(t("Ayarlar")),
      sekmeler(AYAR_SEKMELERI, D.ayarSekmesi, (ad) => { D.ayarSekmesi = ad; adresiYaz(); ayarlarCiz(); }),
      sayfaAlani);
    sayfaCiz();
  }

  async function sayfaCiz() {
    clearInterval(agZamanlayici);
    if (D.gorunum !== "ayarlar" || !sayfaAlani) return;
    const ic = el("div", { class: "sayfa-ic" });
    bosalt(sayfaAlani, ic);
    try {
      switch (D.ayarSekmesi) {
        case "gorunum": return await gorunumSekmesi(ic);
        case "dil": return dilSekmesi(ic);
        case "eklentiler": return await eklentilerSekmesi(ic);
        case "ag":
          await agSekmesi(ic);
          agZamanlayici = setInterval(() => {
            if (D.gorunum !== "ayarlar" || D.ayarSekmesi !== "ag") { clearInterval(agZamanlayici); return; }
            const yeni = el("div", { class: "sayfa-ic" });
            agSekmesi(yeni).then(() => ic.replaceWith(yeni)).catch(() => {});
          }, 5000);
          return;
        default: return await profilSekmesi(ic);
      }
    } catch (e) {
      bosalt(ic, bosDurum("uyari", t("Yüklenemedi."), e.message));
    }
  }

  async function profilSekmesi(ic) {
    const p = await api("/api/profil");
    const bio = el("textarea", { class: "metin-alani", rows: "4", placeholder: t("kendinden kısaca bahset…"), value: p.bio || "" });
    const durum = el("input", { class: "girdi", placeholder: t("şu an ne yapıyorsun?…"), value: p.durum || "" });
    const baglanti = el("input", { class: "girdi", placeholder: t("github.com/kullanici-adi"), value: p.baglanti || "" });
    const sayacli = (girdi, sinir) => {
      const s = el("span", { class: "alan-durum" });
      const g = () => {
        const n = new TextEncoder().encode(girdi.value).length;
        s.textContent = n + " / " + sinir;
        s.style.setProperty("color", n > sinir ? "var(--hata)" : "");
      };
      girdi.addEventListener("input", g);
      g();
      return s;
    };
    const secili = new Set(p.etiketler || []);
    const cipler = el("div", { class: "cipler" });
    const ciplerCiz = () => bosalt(cipler, p.kategoriler.map((k) => el("button", {
      class: "cip" + (secili.has(k.ad) ? " secili" : ""), type: "button", "aria-pressed": secili.has(k.ad) ? "true" : "false",
      disabled: !secili.has(k.ad) && secili.size >= p.sinir.etiket,
      on: { click: () => { if (secili.has(k.ad)) secili.delete(k.ad); else secili.add(k.ad); ciplerCiz(); } },
    }, k.etiket)));
    ciplerCiz();
    const kaydet = el("button", { class: "dugme birincil" }, ikon("tamam"), t("Profili yayınla"));
    kaydet.addEventListener("click", async () => {
      kaydet.disabled = true;
      try {
        await api("/api/profil", { bio: bio.value, durum: durum.value, baglanti: baglanti.value, etiketler: [...secili] });
        bildir(t("Profilin yayınlandı."), "iyi");
      } catch (e) {
        bildir(e.message, "hata");
      } finally {
        kaydet.disabled = false;
      }
    });
    bosalt(ic,
      el("div", { class: "bolum" }, el("div", { class: "kutu" },
        el("div", { class: "kimlik-karti" }, avatar(p.ad, p.anahtar, "buyuk"),
          el("div", null,
            el("div", { class: "ad" }, p.ad || t("adsız"), " ", p.rol ? el("span", { class: "rozet rol" }, p.rol) : null),
            el("div", { class: "satir" }, el("span", null, t("Peer ID")), el("span", { class: "kopyalanabilir", title: p.peerID }, p.peerID || "—"),
              el("button", { class: "ikon-dugme", title: t("Kopyala"), "aria-label": t("Kopyala"), on: { click: () => kopyala(p.peerID) } }, ikon("kopyala"))),
            el("div", { class: "satir" }, el("span", null, t("Genel anahtar")), el("span", { class: "kopyalanabilir", title: p.anahtar }, p.anahtar),
              el("button", { class: "ikon-dugme", title: t("Kopyala"), "aria-label": t("Kopyala"), on: { click: () => kopyala(p.anahtar) } }, ikon("kopyala"))))),
        el("div", { class: "istatistikler" },
          el("div", { class: "istatistik" }, el("b", null, String(p.konuSayisi || 0)), el("span", null, t("Konu"))),
          el("div", { class: "istatistik" }, el("b", null, String(p.yanitSayisi || 0)), el("span", null, t("Yanıt"))),
          el("div", { class: "istatistik" }, el("b", null, p.katilim ? new Date(p.katilim).toLocaleDateString(DIL) : "—"), el("span", null, t("İlk görülme")))))),
      el("div", { class: "bolum" }, el("h3", null, t("Herkese açık profil")),
        el("div", { class: "kutu" }, el("div", { class: "kutu-ic" },
          el("p", { class: "paragraf soluk" }, t("Profilin eşlere yayılır ve herkes görebilir.")),
          alan(t("Hakkında"), bio, sayacli(bio, p.sinir.bio)),
          alan(t("Durum"), durum, sayacli(durum, p.sinir.durum)),
          alan(t("Bağlantı"), baglanti, sayacli(baglanti, p.sinir.baglanti)),
          el("div", { class: "alan" }, el("div", { class: "alan-ust" }, el("span", { class: "alan-etiket" }, t("İlgi alanları")),
            el("span", { class: "alan-durum" }, tf("en çok %d", p.sinir.etiket))), cipler),
          p.github ? el("div", { class: "alan-not" }, "GitHub: " + p.github) : null,
          el("div", { class: "sag-hizali" }, kaydet)))));
  }

  async function gorunumSekmesi(ic) {
    const r = await api("/api/temalar");
    bosalt(ic, el("div", { class: "bolum" }, el("h3", null, t("Tema")),
      el("p", { class: "paragraf soluk" }, t("Tema, terminal arayüzünde ve eklentilerde de geçerlidir.")),
      el("div", { class: "tema-izgarasi" }, r.temalar.map((tm) => {
        // Önizleme, giriş ekranındaki ağ haritasının o temanın renkleriyle
        // küçük bir çizimidir.
        const onizleme = el("div", { class: "tema-onizleme" },
          svgEl("svg", { class: "tema-harita", viewBox: "0 0 160 64", "aria-hidden": "true" },
            svgEl("path", { d: "M8 14H152", stroke: tm.isim }),
            svgEl("path", { d: "M90 6V58", stroke: tm.secimZemin }),
            svgEl("path", { d: "M8 50H50L74 26H152", stroke: tm.vurgu }),
            svgEl("circle", { class: "aktarma", cx: 90, cy: 14, r: 5 }),
            svgEl("circle", { class: "durak", cx: 130, cy: 26, r: 4, fill: tm.rozet }),
            svgEl("circle", { class: "sen", cx: 90, cy: 26, r: 7, fill: tm.vurgu })));
        const etkin = tm.ad === r.etkin.ad;
        return el("button", { class: "tema-karti" + (etkin ? " etkin" : ""), "aria-pressed": etkin ? "true" : "false", on: { click: () => temaSec(tm.ad) } },
          onizleme, el("div", { class: "tema-adi" }, tm.etiket, etkin ? ikon("tamam") : null));
      }))));
  }

  async function temaSec(ad) {
    try {
      temaUygula(await api("/api/tema", { ad }));
      sayfaCiz();
    } catch (e) {
      bildir(e.message, "hata");
    }
  }

  function dilSekmesi(ic) {
    bosalt(ic, el("div", { class: "bolum" }, el("h3", null, t("Arayüz dili")),
      el("p", { class: "paragraf soluk" }, t("Seçilen dil terminal arayüzünde ve eklentilerde de kullanılır.")),
      el("div", { class: "kutu", role: "radiogroup" }, D.diller.map((d) => el("button", {
        class: "liste-secenek" + (d.kod === DIL ? " etkin" : ""), role: "radio", "aria-checked": d.kod === DIL ? "true" : "false",
        on: { click: () => diliDegistir(d.kod) },
      }, el("span", { class: "daire" }), el("span", null, d.ad), el("span", { class: "kod" }, d.kod))))));
  }

  async function eklentilerSekmesi(ic) {
    const r = await api("/api/eklentiler");
    const kart = (e) => {
      const anahtar = el("input", { type: "checkbox", checked: e.etkin, "aria-label": t("Etkin") });
      anahtar.addEventListener("change", async () => {
        try {
          const s = await api("/api/eklenti/etkin", { ad: e.ad, etkin: anahtar.checked });
          D.oturum.menu = s.menu;
          D.oturum.konuAcabilir = s.konuAcabilir;
          gezinmeCiz();
          sayfaCiz();
        } catch (err) {
          anahtar.checked = !anahtar.checked;
          bildir(err.message, "hata");
        }
      });
      return el("div", { class: "eklenti-karti" },
        el("div", { class: "eklenti-simgesi" }, ikon(e.tema ? "palet" : "eklenti")),
        el("div", { class: "bilgi" },
          el("div", { class: "ad" }, e.etiket, e.harici ? el("span", { class: "rozet uyari" }, t("dışarıdan")) : null),
          el("div", { class: "ust-bilgi" }, [e.surum && "v" + e.surum, e.yazar].filter(Boolean).join(" · ")),
          e.aciklama ? el("p", { class: "aciklama" }, e.aciklama) : null,
          e.izinler && e.izinler.length ? el("div", { class: "izinler" }, e.izinler.map((iz) => el("span", { class: "rozet kategori", title: iz.aciklama }, iz.baslik))) : null),
        el("div", { class: "islemler" },
          el("label", { class: "anahtar" }, anahtar, el("span")),
          e.etkin && !e.tema ? el("button", { class: "dugme kucuk", on: { click: () => eklentiAc(e.ad) } }, ikon("dis"), tb("aç")) : null));
    };
    bosalt(ic, el("div", { class: "bolum" }, el("h3", null, t("Kurulu eklentiler")),
      el("p", { class: "paragraf soluk" }, t("Eklentiler kendi terminal penceresinde açılır. Kapattığın eklenti menüde görünmez.")),
      r.eklentiler.length ? el("div", { class: "kutu" }, r.eklentiler.map(kart)) : bosDurum("eklenti", t("Eklenti yok"))));
  }

  function kvSatiri(s) {
    return el("div", { class: "satir-kv" }, el("div", { class: "k" }, s.etiket),
      el("div", null, el("div", { class: "v " + (s.durum || "") }, s.deger), s.aciklama ? el("div", { class: "not" }, s.aciklama) : null));
  }

  async function agSekmesi(ic) {
    const a = await api("/api/ag");
    if (a.bos) { bosalt(ic, bosDurum("ag", a.bos)); return; }
    bosalt(ic,
      el("div", { class: "bolum" }, el("h3", null, a.dugum.baslik), el("div", { class: "kutu satirlar" }, a.dugum.satirlar.map(kvSatiri))),
      el("div", { class: "bolum" }, el("h3", null, a.esler.baslik, "  ", el("span", { class: "rozet kategori" }, a.esler.ozet)),
        a.esler.liste.length
          ? el("div", { class: "kutu" }, a.esler.liste.map((e) => el("div", { class: "es-satiri" },
            el("span", { class: "id", title: e.id }, e.id), el("span", { class: "soluk" }, e.adres), el("span", null, e.tasiyici), el("span", null, e.gecikme),
            e.etiketler && e.etiketler.length ? el("span", { class: "etiketler" }, e.etiketler.join("  ·  ")) : null)))
          : el("div", { class: "kutu kutu-ic soluk" }, a.esler.bos)),
      el("div", { class: "bolum" }, el("h3", null, a.kesif.baslik), el("div", { class: "kutu satirlar" }, a.kesif.satirlar.map(kvSatiri))));
  }

  // ── Eklenti açma ──────────────────────────────────────────────────────

  async function eklentiAc(ad, onay) {
    try {
      const r = await api("/api/eklenti/ac", { ad, onay: !!onay });
      if (r.izinGerekli) {
        const kapat = pencereAc({
          baslik: r.baslik,
          govde: [
            r.uyari ? el("div", { class: "uyari-kutusu" }, ikon("uyari"), el("div", null, r.uyari)) : null,
            el("div", { class: "izin-listesi" }, r.izinler.map((iz) => el("div", { class: "izin" }, ikon("kalkan"), el("div", null, el("b", null, iz.baslik), el("span", null, iz.aciklama))))),
            el("p", { class: "paragraf cok-soluk" }, r.not),
          ],
          dugmeler: [
            el("button", { class: "dugme", on: { click: () => kapat() } }, tb("vazgeç")),
            el("button", { class: "dugme birincil", on: { click: () => { kapat(); eklentiAc(ad, true); } } }, tb("izin ver ve aç")),
          ],
        });
        return;
      }
      const e = (D.oturum.menu || []).find((m) => m.ad === ad);
      bildir(tf("%s kendi penceresinde açıldı.", e ? e.etiket : ad), "iyi");
    } catch (e) {
      bildir(e.message, "hata");
    }
  }

  // ── AND hakkında ──────────────────────────────────────────────────────

  const HAKKINDA_SEKMELERI = [
    ["genel", "bilgi", K("Genel")],
    ["gizlilik", "kalkan", K("Gizlilik")],
    ["kurallar", "forum", K("Kurallar")],
    ["lisans", "belge", K("Lisans")],
    ["ucuncu", "eklenti", K("Üçüncü taraf")],
  ];

  async function hakkindaCiz() {
    const alanH = el("div", { class: "sayfa" });
    bosalt(gorunumAlani, ustCubuk(t("AND hakkında"), "AND " + ((D.oturum && D.oturum.surum) || D.surum)),
      sekmeler(HAKKINDA_SEKMELERI, D.hakkindaSekmesi, (ad) => { D.hakkindaSekmesi = ad; adresiYaz(); hakkindaCiz(); }), alanH);
    const ic = el("div", { class: "sayfa-ic" }, el("div", { class: "donen" }));
    alanH.append(ic);
    const sekme = D.hakkindaSekmesi;
    let v;
    try {
      v = await api("/api/hakkinda" + (sekme === "lisans" || sekme === "ucuncu" ? "?sekme=" + sekme : ""));
    } catch (e) {
      bosalt(ic, bosDurum("uyari", t("Yüklenemedi."), e.message));
      return;
    }
    if (D.gorunum !== "hakkinda" || D.hakkindaSekmesi !== sekme) return;
    switch (sekme) {
      case "gizlilik":
        bosalt(ic, el("p", { class: "paragraf" }, v.gizlilikGiris), el("ul", { class: "maddeler" }, v.gizlilik.map((m) => el("li", null, m))),
          el("p", { class: "paragraf soluk" }, " "), el("p", { class: "paragraf soluk" }, v.gizlilikNot));
        break;
      case "kurallar":
        bosalt(ic, el("p", { class: "paragraf" }, v.kurallarGiris), el("ul", { class: "maddeler" }, v.kurallar.map((m) => el("li", null, m))),
          el("p", { class: "paragraf soluk" }, " "), el("p", { class: "paragraf soluk" }, v.kurallarNot));
        break;
      case "lisans":
      case "ucuncu":
        bosalt(ic, el("p", { class: "paragraf soluk" }, v.giris), el("pre", { class: "lisans-metni" }, v.metin));
        break;
      default:
        bosalt(ic,
          el("div", { class: "bolum" }, el("div", { class: "kutu satirlar" }, v.bilgiler.map(([k, d]) => kvSatiri({ etiket: k, deger: d })))),
          el("div", { class: "bolum" }, el("h3", null, t("Yasal bildirim")), v.yasal.map((p) => el("p", { class: "paragraf" }, p))),
          el("div", { class: "bolum" }, el("h3", null, t("Üçüncü taraf bileşenler")), el("p", { class: "paragraf" }, v.ucuncu)),
          el("div", { class: "bolum" }, el("h3", null, t("Güvenlik açığı bildirimi")), el("p", { class: "paragraf" }, v.guvenlik)));
    }
  }

  // ── Hızlı erişim (Ctrl+K) ─────────────────────────────────────────────
  //
  // Ekranlara, ayar sekmelerine, konusu olan kategorilere, yüklenmiş
  // konulara ve eklentilere klavyeden ulaşılır: yaz, ↑↓ ile seç, Enter.

  function erisimKomutlari() {
    const k = [];
    const ekle = (grup, ikonAd, ad, is, alt) => k.push({ grup, ikonAd, ad, is, alt });
    ekle(t("Gezinme"), "forum", t("Forum"), () => { D.forum.kategori = ""; git("forum"); });
    ekle(t("Gezinme"), "sohbet", t("Sohbet"), () => git("sohbet"));
    for (const [ad, ikonAd, etiket] of AYAR_SEKMELERI) {
      ekle(t("Gezinme"), ikonAd, t("Ayarlar") + " › " + t(etiket), () => { D.ayarSekmesi = ad; git("ayarlar"); });
    }
    ekle(t("Gezinme"), "bilgi", t("AND hakkında"), () => git("hakkinda"));
    const v = D.forum.veri;
    if (v) {
      for (const kat of v.kategoriler.filter((x) => x.sayi > 0)) {
        ekle(t("Kategoriler"), "forum", kat.etiket, () => { D.forum.kategori = kat.ad; D.forum.secili = null; git("forum"); }, String(kat.sayi));
      }
      for (const konu of v.konular) {
        ekle(t("Konular"), "sohbet", konu.baslik, () => { D.forum.secili = konu.id; git("forum"); }, konu.yazar);
      }
    }
    for (const e of (D.oturum && D.oturum.menu) || []) ekle(t("Eklentiler"), "eklenti", e.etiket, () => eklentiAc(e.ad));
    return k;
  }

  function hizliErisim() {
    if (D.asama !== "uygulama" || document.querySelector(".palet")) return;
    const girdi = el("input", { class: "palet-girdi", placeholder: t("Ara ya da git…"), "aria-label": t("Hızlı erişim"), spellcheck: "false" });
    const liste = el("div", { class: "palet-liste", role: "listbox", "aria-label": t("Hızlı erişim") });
    const komutlar = erisimKomutlari();
    let sonuc = [];
    let secili = 0;
    const onceki = document.activeElement;
    const kapat = () => {
      perde.remove();
      if (onceki && onceki.focus) onceki.focus();
    };
    const calistir = (k) => { kapat(); k.is(); };
    const ciz = () => {
      const aranan = girdi.value.trim().toLocaleLowerCase(DIL);
      sonuc = komutlar.filter((k) => !aranan || (k.ad + " " + (k.alt || "")).toLocaleLowerCase(DIL).includes(aranan)).slice(0, 40);
      secili = Math.min(secili, Math.max(sonuc.length - 1, 0));
      if (!sonuc.length) {
        bosalt(liste, el("div", { class: "palet-bos" }, t("Sonuç yok")));
        return;
      }
      const ogeler = [];
      let grup = "";
      butonlar = sonuc.map((k, i) => el("button", {
        class: "palet-oge", role: "option", tabindex: "-1",
        on: { click: () => calistir(k), mousemove: () => { if (secili !== i) isaretle(i, false); } },
      }, ikon(k.ikonAd), el("span", { class: "ad" }, k.ad), k.alt ? el("span", { class: "alt" }, k.alt) : null));
      sonuc.forEach((k, i) => {
        if (k.grup !== grup) { grup = k.grup; ogeler.push(el("div", { class: "palet-grup" }, grup)); }
        ogeler.push(butonlar[i]);
      });
      bosalt(liste, ogeler);
      isaretle(secili, true);
    };
    let butonlar = [];
    const isaretle = (i, kaydir) => {
      secili = i;
      butonlar.forEach((b, j) => {
        b.classList.toggle("secili", j === i);
        b.setAttribute("aria-selected", j === i ? "true" : "false");
      });
      if (kaydir && butonlar[i]) butonlar[i].scrollIntoView({ block: "nearest" });
    };
    girdi.addEventListener("input", () => { secili = 0; ciz(); });
    girdi.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown") { e.preventDefault(); isaretle(Math.min(secili + 1, sonuc.length - 1), true); }
      else if (e.key === "ArrowUp") { e.preventDefault(); isaretle(Math.max(secili - 1, 0), true); }
      else if (e.key === "Enter") { e.preventDefault(); if (sonuc[secili]) calistir(sonuc[secili]); }
      else if (e.key === "Escape") { e.preventDefault(); kapat(); }
    });
    const perde = el("div", { class: "perde palet-perde", on: { mousedown: (e) => { if (e.target === perde) kapat(); } } },
      el("div", { class: "palet", role: "dialog", "aria-modal": "true", "aria-label": t("Hızlı erişim") },
        el("div", { class: "palet-ust" }, ikon("ara"), girdi, el("kbd", null, "Esc")), liste));
    document.getElementById("katman").append(perde);
    ciz();
    girdi.focus();
  }

  // ── Klavye ────────────────────────────────────────────────────────────

  document.addEventListener("keydown", (e) => {
    if (D.asama !== "uygulama") return;
    if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "k") {
      e.preventDefault();
      hizliErisim();
      return;
    }
    if (!e.altKey || e.ctrlKey || e.metaKey) return;
    const kisayol = { 1: "forum", 2: "sohbet", 3: "ayarlar", 4: "hakkinda" }[e.key];
    if (kisayol) { e.preventDefault(); git(kisayol); }
  });

  basla();
})();
