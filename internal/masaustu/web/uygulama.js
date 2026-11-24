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
    kod: "M8 8l-4 4 4 4M16 8l4 4-4 4M14 5l-4 14",
    yanitla: "M9 7l-5 5 5 5M4 12h10a6 6 0 0 1 6 6v1",
    yenile: "M20 12a8 8 0 1 1-2.34-5.66M20 4v5h-5",
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
    eklentiAd: "",
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
    r.setProperty("--vurgu-yazi", tema.vurguYazi || "#ffffff");
    terminalTemalariniGuncelle();
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
    // Açılışta arka planda başlayan eklentiler (kenar çubuğunda yeşil nokta).
    k.addEventListener("terminaller", () => { if (D.asama === "uygulama") oturumlariYukle(); });
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
    await oturumlariYukle();
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

  // bolumle, kategorisine göre dizili eklentileri ardışık bölümlere ayırır.
  function bolumle(liste) {
    const gruplar = [];
    for (const e of liste) {
      const son = gruplar[gruplar.length - 1];
      if (son && son.kategori === e.kategori) son.ogeler.push(e);
      else gruplar.push({ kategori: e.kategori, ogeler: [e] });
    }
    return gruplar;
  }

  // kategoriIkonu, bir eklenti kategorisinin simgesidir.
  function kategoriIkonu(kod) {
    return { communication: "sohbet", content: "belge", moderation: "kalkan", development: "kod", tools: "ayarlar", theme: "palet" }[kod] || "eklenti";
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
      // Eklentiler tek bir başlık altında değil, kategorilerine göre ayrı
      // bölümlerde (İletişim, İçerik, Yönetim…; sunucu bölüm sırasına dizer).
      (o.menu && o.menu.length)
        ? bolumle(o.menu).map((g) => [
          el("div", { class: "grup-basligi" }, g.kategori),
          g.ogeler.map((e) => {
            const etkin = D.gorunum === "eklenti" && D.eklentiAd === e.ad;
            return el("button", { class: "nav" + (etkin ? " etkin" : ""), "aria-current": etkin ? "page" : undefined, title: e.etiket, on: { click: () => eklentiGoster(e.ad) } },
              ikon(kategoriIkonu(e.kategoriKodu)), el("span", null, e.etiket),
              eklentiCalisiyor(e.ad) ? el("i", { class: "calisiyor", title: t("Çalışıyor"), "aria-label": t("Çalışıyor") }) : null);
          })])
        : [el("div", { class: "grup-basligi" }, t("Eklentiler")), el("div", { class: "gezinme-bos" }, t("Etkin eklenti yok"))],
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
  const GORUNUMLER = ["forum", "sohbet", "ayarlar", "hakkinda", "eklenti"];
  function adresiYaz() {
    const alt = D.gorunum === "forum" ? (D.forum.secili ? "/" + D.forum.secili : "") : D.gorunum === "ayarlar" ? "/" + D.ayarSekmesi
      : D.gorunum === "hakkinda" ? "/" + D.hakkindaSekmesi : D.gorunum === "eklenti" ? "/" + encodeURIComponent(D.eklentiAd) : "";
    history.replaceState(null, "", "#" + D.gorunum + alt);
  }
  function adrestenOku() {
    const [g, alt] = decodeURIComponent(location.hash.slice(1)).split("/");
    if (!GORUNUMLER.includes(g)) return;
    if (g === "eklenti" && !alt) return;
    D.gorunum = g;
    if (g === "eklenti") D.eklentiAd = alt;
    if (g === "ayarlar" && AYAR_SEKMELERI.some((s) => s[0] === alt)) D.ayarSekmesi = alt;
    if (g === "hakkinda" && HAKKINDA_SEKMELERI.some((s) => s[0] === alt)) D.hakkindaSekmesi = alt;
    if (g === "forum" && alt) D.forum.secili = alt;
  }

  function gorunumCiz() {
    arayuzGorunumDegisti();
    // Ekran değişirken kısa bir geçiş (hareket azaltılmışsa yok).
    gorunumAlani.classList.remove("gecis");
    void gorunumAlani.offsetWidth;
    gorunumAlani.classList.add("gecis");
    switch (D.gorunum) {
      case "sohbet": return sohbetCiz();
      case "ayarlar": return ayarlarCiz();
      case "hakkinda": return hakkindaCiz();
      case "eklenti": return eklentiCiz();
      default: return forumCiz();
    }
  }

  // ustCubuk, her ekranın başındaki araç çubuğudur: simge, başlık, kısa
  // açıklama ve sağda o ekranın işlemleri.
  function ustCubuk(ikonAd, baslik, altBaslik, ...sag) {
    return el("header", { class: "ust-cubuk" },
      ikon(ikonAd, "baslik-ikon"),
      el("div", { class: "baslik-grubu" }, el("h1", null, baslik), altBaslik ? el("span", { class: "alt-baslik" }, altBaslik) : null),
      el("div", { class: "bosluk" }), sag);
  }

  // bosHarita, giriş ekranındaki ağ haritasının küçük bir parçasıdır: iki
  // hat, birkaç durak ve ortada ekranın simgesini taşıyan durak.
  function bosHarita(ikonAd) {
    return svgEl("svg", { class: "bos-harita", viewBox: "0 0 176 64", "aria-hidden": "true" },
      svgEl("path", { class: "hat hat-yan", d: "M4 14H172" }),
      svgEl("path", { class: "hat hat-ana", d: "M4 54H40L62 32H172" }),
      [[30, 14], [146, 14], [24, 54], [140, 32]].map(([x, y]) => svgEl("circle", { class: "durak", cx: x, cy: y, r: 3.5 })),
      svgEl("circle", { class: "merkez", cx: 92, cy: 32, r: 21 }),
      svgEl("svg", { class: "simge", x: 80, y: 20, width: 24, height: 24, viewBox: "0 0 24 24" }, svgEl("path", { d: IKONLAR[ikonAd] || "" })));
  }

  function bosDurum(ikonAd, baslik, metin) {
    return el("div", { class: "bos-durum" }, bosHarita(ikonAd), el("h3", null, baslik), metin ? el("p", null, metin) : null);
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
    const sayac = el("span", { class: "sayac" });
    const gonder = el("button", { class: "dugme birincil", on: { click: () => yanitGonder(k.konu.id, yazma, gonder) } }, ikon("gonder"), tb("gönder"));
    const sayacGuncelle = () => {
      const bayt = new TextEncoder().encode(yazma.value).length;
      sayac.textContent = bayt + " / " + k.sinir;
      sayac.classList.toggle("asim", bayt > k.sinir);
      D.forum.taslaklar[k.konu.id] = yazma.value;
    };
    yazma.addEventListener("input", sayacGuncelle);
    yazma.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); gonder.click(); }
    });
    sayacGuncelle();

    // Araç çubuğu: kategori (tıklayınca o kategorinin konuları), başlık
    // yukarı kaydırılınca onun kısa hâli, yanıt sayısı ve Yanıtla.
    const aracKonu = el("span", { class: "arac-konu", "aria-hidden": "true" }, ikon("asagi", "arac-ayrac"), el("span", null, k.konu.baslik));
    const arac = el("div", { class: "detay-arac" },
      el("button", { class: "kirinti", title: k.konu.kategoriAd, on: { click: () => kategoriSec(k.konu.kategori) } }, ikon("forum"), k.konu.kategoriAd),
      aracKonu,
      el("span", { class: "yanit-sayisi" + (k.yanitlar.length ? "" : " sifir"), title: t("Yanıtlar") }, ikon("sohbet"), String(k.yanitlar.length)),
      el("button", { class: "dugme kucuk", on: { click: () => { yazma.focus(); yazma.scrollIntoView({ block: "nearest" }); } } }, ikon("yanitla"), tb("yanıtla")));

    // Konu bir makale gibi okunur; yanıtlar sol kenardaki bir izlek
    // çizgisiyle konuya ve birbirine bağlanır.
    const baslik = el("h1", null, k.konu.baslik);
    const govde = el("div", { class: "detay-govde" },
      el("div", { class: "okuma" },
        el("header", { class: "detay-ust" },
          baslik,
          el("div", { class: "konu-yazar" }, avatar(k.konu.yazar, k.konu.yazarKisa, "kucuk"), yazarSatiri(k.konu))),
        el("article", { class: "ana-metin" }, govdeCiz(k.govde)),
        el("h2", { class: "yanitlar-basligi" }, k.yanitlar.length ? tf("Yanıtlar (%d)", k.yanitlar.length) : t("Henüz yanıt yok")),
        k.yanitlar.length ? el("ol", { class: "izlek" }, k.yanitlar.map((y) => el("li", { class: "yanit" + (y.kendi ? " kendi" : "") },
          avatar(y.yazar, y.yazarKisa, "kucuk"),
          el("div", { class: "icerik" }, yazarSatiri(y), govdeCiz(y.govde))))) : null));

    bosalt(D.forum.detayAlani,
      arac,
      govde,
      el("div", { class: "yazma" }, yazma,
        el("div", { class: "yazma-alt" }, yazmaIpuclari(), sayac, gonder)));
    // Başlık görünür alandan çıkınca araç çubuğunda kısa hâli belirir.
    if (D.forum.gozlemci) D.forum.gozlemci.disconnect();
    D.forum.gozlemci = new IntersectionObserver(([g]) => aracKonu.classList.toggle("gorunur", !g.isIntersecting), { root: govde });
    D.forum.gozlemci.observe(baslik);
    if (ayniKonu) govde.scrollTop = altta ? govde.scrollHeight : eskiKonum;
    D.forum.cizilen = k.konu.id;
  }

  // yazmaIpuclari, konu ve yanıt yazarken gösterilen kısayol ipuçlarıdır.
  function yazmaIpuclari() {
    return el("div", { class: "ipuclari" },
      el("span", { class: "ipucu" }, el("kbd", null, "Ctrl"), el("kbd", null, "Enter"), t("gönderir")),
      el("span", { class: "ipucu" }, el("code", null, "[code]…[/code]"), t("kod bloğu")));
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
        alan(t("İçerik"), metin, null, yazmaIpuclari())],
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
      ustCubuk("sohbet", t("Sohbet"), t("Herkese açık genel sohbet; iletiler 48 saat sonra silinir.")),
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
      ustCubuk("ayarlar", t("Ayarlar")),
      sekmeler(AYAR_SEKMELERI, D.ayarSekmesi, (ad) => { D.ayarSekmesi = ad; adresiYaz(); ayarlarCiz(); }),
      sayfaAlani);
    sayfaCiz();
  }

  async function sayfaCiz() {
    clearInterval(agZamanlayici);
    if (D.gorunum !== "ayarlar" || !sayfaAlani) return;
    const ic = el("div", { class: "sayfa-ic" });
    bosalt(sayfaAlani, ic);
    // Yükleme uzarsa (150 ms'den çok) dönen bir gösterge; kısa yüklemelerde
    // ekran kırpışmaz.
    const yukleniyor = setTimeout(() => {
      if (!ic.childNodes.length) bosalt(ic, el("div", { class: "sayfa-yukleniyor" }, el("div", { class: "donen" })));
    }, 150);
    try {
      switch (D.ayarSekmesi) {
        case "gorunum": return await gorunumSekmesi(ic);
        case "dil": return dilSekmesi(ic);
        case "eklentiler": return await eklentilerSekmesi(ic);
        case "ag": {
          await agSekmesi(ic);
          // Her 5 saniyede yenilenir; yeni içerik o an ekrandakinin yerine
          // geçer (ilk içeriğin değil: o ilk yenilemede ekrandan çıkar).
          let ekrandaki = ic;
          agZamanlayici = setInterval(() => {
            if (D.gorunum !== "ayarlar" || D.ayarSekmesi !== "ag" || !ekrandaki.isConnected) { clearInterval(agZamanlayici); return; }
            const yeni = el("div", { class: "sayfa-ic" });
            agSekmesi(yeni).then(() => {
              if (!ekrandaki.isConnected) return;
              ekrandaki.replaceWith(yeni);
              ekrandaki = yeni;
            }).catch(() => {});
          }, 5000);
          return;
        }
        default: return await profilSekmesi(ic);
      }
    } catch (e) {
      bosalt(ic, bosDurum("uyari", t("Yüklenemedi."), e.message));
    } finally {
      clearTimeout(yukleniyor);
    }
  }

  // Profil sekmesi: solda düzenleme formu, sağda profilin eşlere nasıl
  // göründüğünün canlı önizlemesi ve kimlik bilgileri; değişiklik varken
  // altta yapışkan bir kaydetme çubuğu.
  async function profilSekmesi(ic) {
    const p = await api("/api/profil");
    ic.classList.add("genis");
    const kayitli = { bio: p.bio || "", durum: p.durum || "", baglanti: p.baglanti || "", etiketler: [...(p.etiketler || [])] };
    const bio = el("textarea", { class: "metin-alani", rows: "4", placeholder: t("kendinden kısaca bahset…"), value: kayitli.bio });
    const durum = el("input", { class: "girdi", placeholder: t("şu an ne yapıyorsun?…"), value: kayitli.durum });
    const baglanti = el("input", { class: "girdi", placeholder: t("github.com/kullanici-adi"), value: kayitli.baglanti });
    const secili = new Set(kayitli.etiketler);
    const etiketAdi = (ad) => (p.kategoriler.find((k) => k.ad === ad) || { etiket: ad }).etiket;

    const sayac = (girdi, sinir) => {
      const s = el("span", { class: "alan-durum" });
      const g = () => {
        const n = new TextEncoder().encode(girdi.value).length;
        s.textContent = n + " / " + sinir;
        s.classList.toggle("asim", n > sinir);
      };
      girdi.addEventListener("input", g);
      g();
      return s;
    };

    // Canlı önizleme.
    const onDurum = el("div", { class: "on-durum" });
    const onBio = el("div", { class: "on-bio metin" });
    const onBaglanti = el("div", { class: "on-baglanti" });
    const onEtiketler = el("div", { class: "on-etiketler" });
    const onizlemeGuncelle = () => {
      bosalt(onDurum, durum.value.trim());
      bosalt(onBio, bio.value.trim() ? metinDugumleri(bio.value.trim()) : null);
      bosalt(onBaglanti, baglanti.value.trim() ? [ikon("dis"), baglanti.value.trim()] : null);
      bosalt(onEtiketler, [...secili].map((ad) => el("span", { class: "rozet kategori" }, etiketAdi(ad))));
      for (const e of [onDurum, onBio, onBaglanti, onEtiketler]) e.classList.toggle("gizli", !e.childNodes.length);
    };

    // İlgi alanları.
    const etiketSayaci = el("span", { class: "alan-durum" });
    const cipler = el("div", { class: "cipler" });
    const ciplerCiz = () => {
      const dolu = secili.size >= p.sinir.etiket;
      etiketSayaci.textContent = secili.size + " / " + p.sinir.etiket;
      bosalt(cipler, p.kategoriler.map((k) => {
        const s = secili.has(k.ad);
        return el("button", {
          class: "cip" + (s ? " secili" : ""), type: "button", "aria-pressed": s ? "true" : "false",
          disabled: !s && dolu, title: !s && dolu ? tf("en çok %d", p.sinir.etiket) : undefined,
          on: { click: () => { if (s) secili.delete(k.ad); else secili.add(k.ad); ciplerCiz(); degisti(); } },
        }, s ? ikon("tamam") : null, k.etiket);
      }));
    };

    // Kaydetme çubuğu: yalnızca kaydedilmemiş değişiklik varken etkin.
    const durumYazisi = el("span", { class: "kaydet-durumu" });
    const vazgec = el("button", { class: "dugme hayalet" }, tb("vazgeç"));
    const kaydet = el("button", { class: "dugme birincil" }, ikon("tamam"), t("Profili yayınla"));
    const cubuk = el("div", { class: "kaydet-cubugu" }, durumYazisi, el("span", { class: "bosluk" }), vazgec, kaydet);
    const simdiki = () => ({ bio: bio.value, durum: durum.value, baglanti: baglanti.value, etiketler: [...secili] });
    const kirli = () => {
      const s = simdiki();
      return s.bio !== kayitli.bio || s.durum !== kayitli.durum || s.baglanti !== kayitli.baglanti ||
        s.etiketler.length !== kayitli.etiketler.length || s.etiketler.some((e) => !kayitli.etiketler.includes(e));
    };
    const degisti = () => {
      const k = kirli();
      cubuk.classList.toggle("kirli", k);
      durumYazisi.textContent = k ? t("Kaydedilmemiş değişiklikler") : "";
      kaydet.disabled = !k;
      vazgec.disabled = !k;
      onizlemeGuncelle();
    };
    for (const g of [bio, durum, baglanti]) g.addEventListener("input", degisti);
    vazgec.addEventListener("click", () => {
      bio.value = kayitli.bio;
      durum.value = kayitli.durum;
      baglanti.value = kayitli.baglanti;
      secili.clear();
      kayitli.etiketler.forEach((e) => secili.add(e));
      for (const g of [bio, durum, baglanti]) g.dispatchEvent(new Event("input"));
      ciplerCiz();
      degisti();
    });
    kaydet.addEventListener("click", async () => {
      kaydet.disabled = true;
      try {
        const s = simdiki();
        await api("/api/profil", s);
        Object.assign(kayitli, { bio: s.bio, durum: s.durum, baglanti: s.baglanti, etiketler: [...s.etiketler] });
        bildir(t("Profilin yayınlandı."), "iyi");
      } catch (e) {
        bildir(e.message, "hata");
      } finally {
        degisti();
      }
    });

    const kimlikSatiri = (etiket, deger) => el("div", { class: "kimlik-satiri" },
      el("div", { class: "kimlik-ust" }, el("span", null, etiket),
        deger ? el("button", { class: "ikon-dugme", title: t("Kopyala"), "aria-label": t("Kopyala"), on: { click: () => kopyala(deger) } }, ikon("kopyala")) : null),
      el("code", null, deger || "—"));

    ciplerCiz();
    bosalt(ic, el("div", { class: "profil-duzen" },
      el("div", { class: "profil-form" },
        el("div", { class: "bolum" }, el("h3", null, t("Herkese açık profil")),
          el("p", { class: "paragraf soluk" }, t("Profilin eşlere yayılır ve herkes görebilir.")),
          el("div", { class: "kutu" }, el("div", { class: "kutu-ic" },
            alan(t("Hakkında"), bio, sayac(bio, p.sinir.bio)),
            alan(t("Durum"), durum, sayac(durum, p.sinir.durum)),
            alan(t("Bağlantı"), baglanti, sayac(baglanti, p.sinir.baglanti)),
            el("div", { class: "alan son" }, el("div", { class: "alan-ust" }, el("span", { class: "alan-etiket" }, t("İlgi alanları")), etiketSayaci), cipler)))),
        cubuk),
      el("aside", { class: "profil-yan" },
        el("div", { class: "yan-baslik" }, t("Profilin eşlere böyle görünür")),
        el("div", { class: "kutu on-kart" },
          el("div", { class: "on-ust" }, avatar(p.ad, p.anahtar, "buyuk"),
            el("div", null, el("div", { class: "on-ad" }, p.ad || t("adsız")),
              el("div", { class: "on-rozetler" }, p.rol ? el("span", { class: "rozet rol" }, p.rol) : null,
                p.github ? el("span", { class: "rozet kategori" }, "GitHub: " + p.github) : null))),
          onDurum, onBio, onBaglanti, onEtiketler,
          el("div", { class: "istatistikler" },
            el("div", { class: "istatistik" }, el("b", null, String(p.konuSayisi || 0)), el("span", null, t("Konu"))),
            el("div", { class: "istatistik" }, el("b", null, String(p.yanitSayisi || 0)), el("span", null, t("Yanıt"))),
            el("div", { class: "istatistik" }, el("b", null, p.katilim ? new Date(p.katilim).toLocaleDateString(DIL) : "—"), el("span", null, t("İlk görülme"))))),
        el("div", { class: "yan-baslik" }, t("Kimlik")),
        el("div", { class: "kutu kimlik-kutusu" }, kimlikSatiri(t("Peer ID"), p.peerID), kimlikSatiri(t("Genel anahtar"), p.anahtar)))));
    degisti();
  }

  // temaOnizleme, giriş ekranındaki ağ haritasının bir temanın renkleriyle
  // küçük bir çizimidir; zemini de temanın tonunu taşır.
  function temaOnizleme(tm) {
    const kutu = el("div", { class: "tema-onizleme" },
      svgEl("svg", { class: "tema-harita", viewBox: "0 0 160 64", "aria-hidden": "true" },
        svgEl("path", { d: "M8 14H152", stroke: tm.isim }),
        svgEl("path", { d: "M90 6V58", stroke: tm.secimZemin }),
        svgEl("path", { d: "M8 50H50L74 26H152", stroke: tm.vurgu }),
        svgEl("circle", { class: "aktarma", cx: 90, cy: 14, r: 5 }),
        svgEl("circle", { class: "durak", cx: 130, cy: 26, r: 4, fill: tm.rozet }),
        svgEl("circle", { class: "sen", cx: 90, cy: 26, r: 7, fill: tm.vurgu })));
    kutu.style.setProperty("--onizleme-vurgu", tm.vurgu);
    return kutu;
  }

  async function gorunumSekmesi(ic) {
    const r = await api("/api/temalar");
    const ozelEtkin = r.etkin.ad === r.ozelAd;

    // Özel renk: renk seçilirken türetilen tema önizlenir, Uygula ile kaydedilir.
    const renk = el("input", { type: "color", class: "renk-secici", value: r.etkin.vurgu, "aria-label": t("Özel renk") });
    const kod = el("span", { class: "renk-kodu" }, r.etkin.vurgu);
    const onizlemeYeri = el("div", { class: "ozel-onizleme" }, temaOnizleme(r.etkin));
    let zaman;
    renk.addEventListener("input", () => {
      kod.textContent = renk.value;
      clearTimeout(zaman);
      zaman = setTimeout(async () => {
        try {
          bosalt(onizlemeYeri, temaOnizleme(await api("/api/tema/ozel?renk=" + encodeURIComponent(renk.value))));
        } catch { /* önizleme isteğe bağlı */ }
      }, 80);
    });
    const uygula = el("button", { class: "dugme birincil" }, ikon("palet"), tb("uygula"));
    uygula.addEventListener("click", async () => {
      try {
        temaUygula(await api("/api/tema/ozel", { renk: renk.value }));
        sayfaCiz();
      } catch (e) {
        bildir(e.message, "hata");
      }
    });

    bosalt(ic,
      el("div", { class: "bolum" }, el("h3", null, t("Tema")),
        el("p", { class: "paragraf soluk" }, t("Tema, terminal arayüzünde ve eklentilerde de geçerlidir.")),
        el("div", { class: "tema-izgarasi" }, r.temalar.map((tm) => {
          const etkin = tm.ad === r.etkin.ad;
          return el("button", { class: "tema-karti" + (etkin ? " etkin" : ""), "aria-pressed": etkin ? "true" : "false", on: { click: () => temaSec(tm.ad) } },
            temaOnizleme(tm), el("div", { class: "tema-adi" }, tm.etiket, etkin ? ikon("tamam") : null));
        }))),
      el("div", { class: "bolum" }, el("h3", null, t("Özel renk")),
        el("p", { class: "paragraf soluk" }, t("Kendi vurgu rengini seç; AND öteki renkleri okunur kalacak biçimde ondan türetir.")),
        el("div", { class: "ozel-tema" + (ozelEtkin ? " etkin" : "") },
          onizlemeYeri,
          el("div", { class: "ozel-denetim" },
            el("label", { class: "renk-alani" }, renk, kod),
            uygula,
            ozelEtkin ? el("span", { class: "ozel-etkin" }, ikon("tamam"), t("Özel")) : null))));
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
          // Kapatılan eklenti AND'ın içinde çalışıyorsa o da durur: menüden
          // kalkınca ekranına ulaşılamaz.
          if (!anahtar.checked) await terminaliAt(e.ad);
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
        el("div", { class: "eklenti-simgesi" }, ikon(kategoriIkonu(e.kategoriKodu))),
        el("div", { class: "bilgi" },
          el("div", { class: "ad" }, e.etiket, e.harici ? el("span", { class: "rozet uyari" }, t("dışarıdan")) : null),
          el("div", { class: "ust-bilgi" }, [e.surum && "v" + e.surum, e.yazar].filter(Boolean).join(" · ")),
          e.aciklama ? el("p", { class: "aciklama" }, e.aciklama) : null,
          e.izinler && e.izinler.length ? el("div", { class: "izinler" }, e.izinler.map((iz) => el("span", { class: "rozet kategori", title: iz.aciklama }, iz.baslik))) : null),
        el("div", { class: "islemler" },
          el("label", { class: "anahtar" }, anahtar, el("span")),
          e.etkin && !e.tema ? el("button", { class: "dugme kucuk", on: { click: () => eklentiGoster(e.ad) } }, ikon(kategoriIkonu(e.kategoriKodu)), tb("aç")) : null));
    };
    bosalt(ic, el("div", { class: "bolum" }, el("h3", null, t("Kurulu eklentiler")),
      el("p", { class: "paragraf soluk" }, t("Eklentiler AND'ın içinde açılır. Kapattığın eklenti menüde görünmez.")),
      r.eklentiler.length
        ? bolumle(r.eklentiler).map((g) => el("div", { class: "eklenti-bolumu" },
          el("h4", null, ikon(kategoriIkonu(g.ogeler[0].kategoriKodu)), g.kategori, el("span", { class: "sayi" }, String(g.ogeler.length))),
          el("div", { class: "kutu" }, g.ogeler.map(kart))))
        : bosDurum("eklenti", t("Eklenti yok"))));
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

  // izinSor, eklentinin istediği izinleri gösterir; kullanıcı izin verirse
  // kabul, vazgeçerse vazgec çağrılır.
  function izinSor(r, kabul, vazgec) {
    let verildi = false;
    const kapat = pencereAc({
      baslik: r.baslik,
      govde: [
        r.uyari ? el("div", { class: "uyari-kutusu" }, ikon("uyari"), el("div", null, r.uyari)) : null,
        el("div", { class: "izin-listesi" }, r.izinler.map((iz) => el("div", { class: "izin" }, ikon("kalkan"), el("div", null, el("b", null, iz.baslik), el("span", null, iz.aciklama))))),
        el("p", { class: "paragraf cok-soluk" }, r.not),
      ],
      dugmeler: [
        el("button", { class: "dugme", on: { click: () => kapat() } }, tb("vazgeç")),
        el("button", { class: "dugme birincil", on: { click: () => { verildi = true; kapat(); } } }, tb("izin ver ve aç")),
      ],
      kapaninca: () => (verildi ? kabul() : vazgec && vazgec()),
    });
  }

  // eklentiAc, eklentiyi kendi penceresinde açar (AND'ın içinde açılamadığında
  // ya da kullanıcı "Ayrı pencerede aç" dediğinde).
  async function eklentiAc(ad, onay) {
    try {
      const r = await api("/api/eklenti/ac", { ad, onay: !!onay });
      if (r.izinGerekli) { izinSor(r, () => eklentiAc(ad, true)); return; }
      const e = eklentiBilgisi(ad);
      bildir(tf("%s kendi penceresinde açıldı.", e ? e.etiket : ad), "iyi");
    } catch (e) {
      bildir(e.message, "hata");
    }
  }

  // ── Eklentiler AND'ın içinde ──────────────────────────────────────────
  //
  // Eklentinin terminal arayüzü AND'ın penceresindeki bir terminalde
  // (xterm.js) çalışır: ekranı /api/terminal/akis'ten gelir, tuşları
  // /api/terminal/yaz ile gider. Başka ekrana geçince eklenti kapanmaz;
  // dönünce kaldığı yerden görünür. xterm.js ilk eklenti açılırken yüklenir.

  const T = {}; // eklenti adı → terminal durumu
  let terminalAkisi = null;
  let xtermSozu = null;

  const eklentiBilgisi = (ad) => ((D.oturum && D.oturum.menu) || []).find((m) => m.ad === ad);
  const calisiyor = (ad) => !!(T[ad] && T[ad].oturum && !T[ad].bitti);

  function terminalDurumu(ad) {
    if (!T[ad]) T[ad] = { ad, oturum: null, term: null, fit: null, kutu: null, acik: false, ofs: 0, kuyruk: "", gonderiliyor: false, senkron: false, bekleyen: [], bitti: false, kod: 0, ayri: false, vazgecildi: false, aciliyor: false };
    return T[ad];
  }

  function xtermYukle() {
    if (!xtermSozu) {
      const betik = (src) => new Promise((tamam, hata) =>
        document.head.append(el("script", { src, on: { load: tamam, error: () => hata(new Error(t("Bir şeyler ters gitti."))) } })));
      document.head.append(el("link", { rel: "stylesheet", href: "xterm/xterm.css" }));
      xtermSozu = betik("xterm/xterm.js").then(() => betik("xterm-fit/addon-fit.js"));
      xtermSozu.catch(() => { xtermSozu = null; });
    }
    return xtermSozu;
  }

  // xterm.js, temasını ve hücre ölçülerini sayfaya <style> öğeleriyle ekler;
  // içerik güvenlik politikası (style-src 'self') satır içi stil öğelerini
  // uygulamaz. Bu öğelerin metni oluşturulabilir stil sayfalarına taşınır,
  // öğe değiştikçe güncellenir, öğe kaldırılınca sayfa da kalkar.
  const tasinanStiller = new Map();
  function stilTasi(s) {
    let sayfa = tasinanStiller.get(s);
    if (!sayfa) {
      sayfa = new CSSStyleSheet();
      tasinanStiller.set(s, sayfa);
      document.adoptedStyleSheets = [...document.adoptedStyleSheets, sayfa];
    }
    sayfa.replaceSync(s.textContent || "");
  }
  function stilBirak(s) {
    const sayfa = tasinanStiller.get(s);
    if (!sayfa) return;
    tasinanStiller.delete(s);
    document.adoptedStyleSheets = document.adoptedStyleSheets.filter((x) => x !== sayfa);
  }
  function stilleriIzle(kok) {
    const stiller = (d) => (d.nodeName === "STYLE" ? [d] : d.querySelectorAll ? [...d.querySelectorAll("style")] : []);
    new MutationObserver((kayitlar) => {
      for (const k of kayitlar) {
        const hedef = k.type === "characterData" ? k.target.parentNode : k.target;
        if (hedef && hedef.nodeName === "STYLE") stilTasi(hedef);
        for (const d of k.addedNodes) stiller(d).forEach(stilTasi);
        for (const d of k.removedNodes) stiller(d).forEach(stilBirak);
      }
    }).observe(kok, { childList: true, subtree: true, characterData: true });
  }

  // cozulmusRenk, bir CSS rengini (değişken, color-mix…) #rrggbb'ye çevirir:
  // terminalin teması uygulamanın etkin temasından türetilir.
  function cozulmusRenk(css) {
    const olcer = el("span");
    olcer.style.color = css;
    document.body.append(olcer);
    const c = getComputedStyle(olcer).color;
    olcer.remove();
    const k = document.createElement("canvas").getContext("2d", { willReadFrequently: true });
    k.fillStyle = c;
    k.fillRect(0, 0, 1, 1);
    const [r, g, b] = k.getImageData(0, 0, 1, 1).data;
    return "#" + [r, g, b].map((x) => x.toString(16).padStart(2, "0")).join("");
  }

  function terminalTemasi() {
    const zemin = cozulmusRenk("var(--zemin)");
    const vurgu = cozulmusRenk("var(--vurgu)");
    return {
      background: zemin, foreground: cozulmusRenk("var(--yazi)"), cursor: vurgu, cursorAccent: zemin,
      selectionBackground: vurgu + "66", selectionForeground: "#ffffff",
      black: "#18181b", red: "#ff6b7d", green: "#3ecf8e", yellow: "#f2b94f", blue: "#6b8cff", magenta: "#c084fc", cyan: "#5fd7e6", white: "#d4d4d8",
      brightBlack: "#52525b", brightRed: "#ff8a98", brightGreen: "#6ee7b0", brightYellow: "#ffd27a", brightBlue: "#93a8ff", brightMagenta: "#d8b4fe", brightCyan: "#8be9f5", brightWhite: "#fafafa",
    };
  }

  function terminalTemalariniGuncelle() {
    for (const s of Object.values(T)) if (s.term) s.term.options.theme = terminalTemasi();
  }

  function base64Cozul(veri) {
    const s = atob(veri || "");
    const b = new Uint8Array(s.length);
    for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
    return b;
  }

  // Bütün terminallerin çıktısı tek bir olay akışından gelir; her parça
  // akıştaki konumunu (ofs) taşır. Kaçırılan parça olursa (yavaş arayüz)
  // eksik kalan geçmişten çekilir.
  function terminalAkisiniAc() {
    if (terminalAkisi) return;
    terminalAkisi = new EventSource("/api/terminal/akis");
    let koptu = false;
    const bul = (id) => Object.values(T).find((s) => s.oturum === id);
    terminalAkisi.addEventListener("error", () => { koptu = true; });
    terminalAkisi.addEventListener("open", () => {
      if (koptu) {
        koptu = false;
        for (const s of Object.values(T)) if (s.oturum && s.term) senkronla(s);
      }
      // Akış kurulmadan önce arka planda kapanan eklentinin "bitti" olayı
      // kaçmış olabilir (ekranı açık olanları senkronla zaten günceller).
      api("/api/terminal/liste").then((r) => {
        let degisti = false;
        for (const x of r.terminaller) {
          const s = T[x.ad];
          if (s && s.oturum === x.id && x.bitti && !s.bitti) { Object.assign(s, { bitti: true, kod: x.kod }); degisti = true; }
        }
        if (degisti) gezinmeCiz();
      }).catch(() => {});
    });
    terminalAkisi.addEventListener("cikti", (e) => {
      const o = JSON.parse(e.data);
      const s = bul(o.id);
      if (!s || !s.term) return; // ekranı henüz kurulmadı: gösterilince geçmişten okunur
      if (s.senkron) s.bekleyen.push(o);
      else parcaYaz(s, o);
    });
    terminalAkisi.addEventListener("bitti", (e) => {
      const o = JSON.parse(e.data);
      const s = bul(o.id);
      if (!s) return;
      s.bitti = true;
      s.kod = o.kod;
      if (s.term) s.term.write("\x1b[?25l"); // imleç gizlenir
      gezinmeCiz();
      eklentiDurumuCiz(s);
    });
  }

  function parcaYaz(s, o) {
    const veri = base64Cozul(o.veri);
    const son = o.ofs + veri.length;
    if (son <= s.ofs) return; // zaten yazıldı
    if (o.ofs > s.ofs) { senkronla(s); return; } // bir parça kaçtı
    s.term.write(veri.subarray(s.ofs - o.ofs));
    s.ofs = son;
  }

  // senkronla, terminale s.ofs'tan sonraki çıktıyı geçmişten yazar. Geçmiş o
  // kadar eskiye gitmiyorsa terminal sıfırlanır ve eklentinin ekranı sözde
  // konsoldan yeniden boyanır.
  async function senkronla(s) {
    if (s.senkron || !s.oturum || !s.term) return;
    s.senkron = true;
    try {
      const g = await api("/api/terminal/gecmis?id=" + encodeURIComponent(s.oturum) + "&ofs=" + s.ofs);
      if (g.kesik) s.term.reset();
      s.term.write(base64Cozul(g.veri));
      s.ofs = g.son;
      if (g.bitti && !s.bitti) { s.bitti = true; s.kod = g.kod; gezinmeCiz(); eklentiDurumuCiz(s); }
      if (g.kesik && !g.bitti) api("/api/terminal/boyut", { id: s.oturum, sutun: s.term.cols, satir: s.term.rows, yenile: true }).catch(() => {});
    } catch (e) {
      if (e.durum === 404) { s.oturum = null; s.bitti = true; gezinmeCiz(); eklentiDurumuCiz(s); }
    } finally {
      s.senkron = false;
      const bekleyen = s.bekleyen;
      s.bekleyen = [];
      for (const o of bekleyen) parcaYaz(s, o);
    }
  }

  // Tuşlar sırayla gider: bir istek sürerken gelenler birikir ve bir sonraki
  // istekte birlikte gönderilir.
  async function terminaleYaz(s, veri) {
    if (!s.oturum || s.bitti) return;
    s.kuyruk += veri;
    if (s.gonderiliyor) return;
    s.gonderiliyor = true;
    try {
      while (s.kuyruk && s.oturum) {
        const v = s.kuyruk;
        s.kuyruk = "";
        await api("/api/terminal/yaz", { id: s.oturum, veri: v });
      }
    } catch { s.kuyruk = ""; } finally { s.gonderiliyor = false; }
  }

  let boyutZamani = null;
  function olcuGuncelle(s) {
    if (!s.kutu || !s.kutu.isConnected || !s.kutu.clientWidth) return;
    s.fit.fit();
    clearTimeout(boyutZamani);
    boyutZamani = setTimeout(() => {
      if (s.oturum && !s.bitti) api("/api/terminal/boyut", { id: s.oturum, sutun: s.term.cols, satir: s.term.rows }).catch(() => {});
    }, 80);
  }

  async function terminalKur(s) {
    await xtermYukle();
    if (s.term) return;
    s.kutu = el("div", { class: "terminal-kutusu" });
    stilleriIzle(s.kutu);
    s.term = new window.Terminal({
      fontFamily: '"Cascadia Mono", "Cascadia Code", Consolas, monospace', fontSize: 13.5, lineHeight: 1.12,
      cursorBlink: true, scrollback: 2000, theme: terminalTemasi(), windowsPty: { backend: "conpty" },
    });
    s.fit = new window.FitAddon.FitAddon();
    s.term.loadAddon(s.fit);
    s.term.onData((v) => terminaleYaz(s, v));
  }

  function eklentiGoster(ad) {
    D.eklentiAd = ad;
    git("eklenti");
  }

  async function eklentiCiz() {
    const ad = D.eklentiAd;
    const e = eklentiBilgisi(ad);
    if (!e) { git("forum"); return; } // kapatılmış ya da kaldırılmış
    if (e.arayuz) { arayuzEklentisiCiz(e); return; } // AND'ın bileşenleriyle (bkz. Eklenti arayüzü)
    const s = terminalDurumu(ad);
    const ekran = el("div", { class: "eklenti-ekrani" });
    D.eklentiDurumAlani = el("div", { class: "eklenti-durum" });
    bosalt(gorunumAlani,
      ustCubuk(kategoriIkonu(e.kategoriKodu), e.etiket, e.kategori,
        el("button", { class: "dugme kucuk hayalet", on: { click: () => ayriPenceredeAc(s) } }, ikon("dis"), t("Ayrı pencerede aç")),
        el("button", { class: "ikon-dugme", title: t("Eklentiyi kapat"), "aria-label": t("Eklentiyi kapat"), on: { click: () => eklentiyiKapat(s) } }, ikon("kapat"))),
      ekran);
    try {
      await terminalKur(s);
    } catch (err) {
      bosalt(ekran, bosDurum("uyari", t("Yüklenemedi."), err.message));
      return;
    }
    if (D.gorunum !== "eklenti" || D.eklentiAd !== ad) return;
    ekran.append(s.kutu, D.eklentiDurumAlani);
    if (!s.acik) {
      s.term.open(s.kutu);
      s.acik = true;
      new ResizeObserver(() => olcuGuncelle(s)).observe(s.kutu);
    }
    s.fit.fit();
    s.term.refresh(0, s.term.rows - 1);
    eklentiDurumuCiz(s);
    if (s.oturum) {
      terminalAkisiniAc();
      senkronla(s);
    } else if (!s.bitti && !s.ayri && !s.vazgecildi) {
      eklentiyiBaslat(s);
    }
    s.term.focus();
  }

  async function eklentiyiBaslat(s, onay) {
    if (s.aciliyor) return;
    s.aciliyor = true;
    s.vazgecildi = false;
    eklentiDurumuCiz(s);
    try {
      const boyut = (s.fit && s.fit.proposeDimensions()) || { cols: 100, rows: 30 };
      const r = await api("/api/eklenti/ac", { ad: s.ad, onay: !!onay, gomulu: true, sutun: boyut.cols, satir: boyut.rows });
      if (r.izinGerekli) {
        s.aciliyor = false;
        izinSor(r, () => eklentiyiBaslat(s, true), () => { s.vazgecildi = true; eklentiDurumuCiz(s); });
        return;
      }
      if (!r.oturum) {
        // Bu sistemde AND'ın içinde açılamıyor: kendi penceresinde açıldı.
        s.ayri = true;
        return;
      }
      terminalAkisiniAc();
      if (s.oturum !== r.oturum) {
        s.term.reset();
        Object.assign(s, { oturum: r.oturum, ofs: 0, bitti: false, kod: 0, ayri: false });
      }
      await senkronla(s);
      gezinmeCiz();
      if (D.gorunum === "eklenti" && D.eklentiAd === s.ad) s.term.focus();
    } catch (e) {
      bildir(e.message, "hata");
    } finally {
      s.aciliyor = false;
      eklentiDurumuCiz(s);
    }
  }

  // eklentiDurumuCiz, terminalin üstündeki durum katmanıdır: açılırken,
  // eklenti kapanınca, izin verilmeyince ya da kendi penceresinde açıkken.
  function eklentiDurumuCiz(s) {
    const alan = D.eklentiDurumAlani;
    if (!alan || D.gorunum !== "eklenti" || D.eklentiAd !== s.ad) return;
    const e = eklentiBilgisi(s.ad);
    const etiket = e ? e.etiket : s.ad;
    const simge = kategoriIkonu(e && e.kategoriKodu);
    const dugmeler = (...d) => el("div", { class: "durum-dugmeleri" }, d);
    const yeniden = () => {
      if (s.oturum) api("/api/terminal/kapat", { id: s.oturum }).catch(() => {});
      Object.assign(s, { oturum: null, bitti: false, ayri: false, ofs: 0 });
      s.term.reset();
      eklentiyiBaslat(s);
    };
    if (s.aciliyor) bosalt(alan, el("div", { class: "donen" }));
    else if (s.bitti) {
      bosalt(alan, bosDurum(simge, tf("%s kapandı.", etiket), s.kod ? tf("Çıkış kodu: %d", s.kod) : null),
        dugmeler(el("button", { class: "dugme birincil", on: { click: yeniden } }, ikon("yenile"), t("Yeniden aç"))));
    } else if (s.ayri) {
      bosalt(alan, bosDurum(simge, tf("%s kendi penceresinde açıldı.", etiket)),
        dugmeler(el("button", { class: "dugme", on: { click: () => eklentiAc(s.ad) } }, ikon("dis"), t("Ayrı pencerede aç"))));
    } else if (s.vazgecildi) {
      bosalt(alan, bosDurum("kalkan", t("Eklenti açılmadı")),
        dugmeler(el("button", { class: "dugme birincil", on: { click: () => eklentiyiBaslat(s) } }, tb("aç"))));
    } else bosalt(alan);
  }

  async function ayriPenceredeAc(s) {
    if (calisiyor(s.ad)) {
      const e = eklentiBilgisi(s.ad);
      if (!(await onayla(t("Ayrı pencerede aç"), tf("%s burada kapanır ve kendi penceresinde yeniden açılır.", e ? e.etiket : s.ad), t("Ayrı pencerede aç")))) return;
      await api("/api/terminal/kapat", { id: s.oturum }).catch(() => {});
      Object.assign(s, { oturum: null, bitti: false });
      s.term.reset();
    }
    s.ayri = true;
    eklentiDurumuCiz(s);
    gezinmeCiz();
    eklentiAc(s.ad);
  }

  async function eklentiyiKapat(s) {
    if (calisiyor(s.ad)) {
      const e = eklentiBilgisi(s.ad);
      if (!(await onayla(t("Eklentiyi kapat"), tf("%s hemen kapanır; kaydetmediğin işler kaybolabilir.", e ? e.etiket : s.ad), t("Eklentiyi kapat"), true))) return;
    }
    await terminaliAt(s.ad);
    git("forum");
  }

  // terminaliAt, eklentiyi (AND'ın içinde çalışıyorsa) durdurur ve
  // terminalini bırakır.
  async function terminaliAt(ad) {
    const s = T[ad];
    if (!s) return;
    delete T[ad];
    if (s.oturum) await api("/api/terminal/kapat", { id: s.oturum }).catch(() => {});
    if (s.term) s.term.dispose();
  }

  // ── Eklenti arayüzü ───────────────────────────────────────────────────
  //
  // Arayüz kipindeki eklenti (manifest'te "ui": 1) ekranını bir bileşen ağacı
  // olarak gönderir (bkz. paket arayuz); burada AND'ın kendi bileşenleriyle
  // çizilir. Eklentinin metni hiçbir zaman HTML olarak yorumlanmaz. Tıklamalar,
  // form gönderimleri ve sekme seçimleri /api/arayuz/olay ile eklentiye gider.
  // Kullanıcının yazdığı değer, eklenti o alanın değerini değiştirmedikçe
  // yeni ekranlarda korunur; odak, imleç ve kaydırma da korunur.

  const A = {}; // eklenti adı → arayüz oturumu durumu
  let arayuzAkisi = null;
  let gorunurOturum = null; // eklentiye "görünür" bildirilen oturum

  function arayuzDurumu(ad) {
    if (!A[ad]) A[ad] = { ad, oturum: null, ekran: null, onceki: null, surum: 0, bitti: false, kod: 0, hata: null, yazilan: {}, aciliyor: false, vazgecildi: false, diyalog: null };
    return A[ad];
  }

  const eklentiCalisiyor = (ad) => calisiyor(ad) || !!(A[ad] && A[ad].oturum && !A[ad].bitti);
  const arayuzGosteriliyor = (s) => D.gorunum === "eklenti" && D.eklentiAd === s.ad;

  function arayuzAkisiniAc() {
    if (arayuzAkisi) return;
    arayuzAkisi = new EventSource("/api/arayuz/akis");
    let koptu = false;
    const bul = (id) => Object.values(A).find((s) => s.oturum === id);
    const oku = (e) => JSON.parse(e.data);
    arayuzAkisi.addEventListener("error", () => { koptu = true; });
    arayuzAkisi.addEventListener("open", () => {
      if (!koptu) return;
      koptu = false;
      for (const s of Object.values(A)) if (s.oturum) arayuzEkraniCek(s);
    });
    arayuzAkisi.addEventListener("ekran", (e) => {
      const o = oku(e);
      const s = bul(o.id);
      if (!s || o.surum <= s.surum) return;
      ekranAl(s, o.ekran, o.surum);
    });
    arayuzAkisi.addEventListener("bildirim", (e) => {
      const o = oku(e);
      if (bul(o.id)) bildir(o.metin, o.durum);
    });
    arayuzAkisi.addEventListener("hata", (e) => {
      const o = oku(e);
      const s = bul(o.id);
      if (!s) return;
      s.hata = o.metin;
      if (arayuzGosteriliyor(s)) arayuzCiz(s);
    });
    arayuzAkisi.addEventListener("bitti", (e) => {
      const o = oku(e);
      const s = bul(o.id);
      if (!s) return;
      Object.assign(s, { bitti: true, kod: o.kod });
      gezinmeCiz();
      if (arayuzGosteriliyor(s)) arayuzCiz(s);
    });
  }

  function ekranAl(s, ekran, surum) {
    s.onceki = s.ekran;
    s.ekran = ekran;
    s.surum = surum;
    s.hata = null;
    if (arayuzGosteriliyor(s)) arayuzCiz(s);
  }

  async function arayuzEkraniCek(s) {
    try {
      const g = await api("/api/arayuz/ekran?id=" + encodeURIComponent(s.oturum));
      if (g.bitti && !s.bitti) { Object.assign(s, { bitti: true, kod: g.kod }); gezinmeCiz(); }
      if (g.ekran && g.surum > s.surum) ekranAl(s, g.ekran, g.surum);
      else if (arayuzGosteriliyor(s)) arayuzCiz(s);
    } catch (e) {
      if (e.durum === 404) { Object.assign(s, { oturum: null, bitti: true }); gezinmeCiz(); }
    }
  }

  // gorunurlukGuncelle, eklentiye ekranının kullanıcıya görünüp görünmediğini
  // bildirir (ör. mesajları ancak görünürken okundu sayabilsin).
  function gorunurlukGuncelle() {
    const s = D.gorunum === "eklenti" ? A[D.eklentiAd] : null;
    const simdiki = s && s.oturum && !s.bitti && !document.hidden ? s.oturum : null;
    if (simdiki === gorunurOturum) return;
    if (gorunurOturum) api("/api/arayuz/gorunur", { id: gorunurOturum, gorunur: false }).catch(() => {});
    if (simdiki) api("/api/arayuz/gorunur", { id: simdiki, gorunur: true }).catch(() => {});
    gorunurOturum = simdiki;
  }
  document.addEventListener("visibilitychange", gorunurlukGuncelle);

  // arayuzGorunumDegisti, ekran değişirken çağrılır: gizlenen eklentilerin
  // diyalogları kapanır, eklentilere görünürlükleri bildirilir.
  function arayuzGorunumDegisti() {
    for (const s of Object.values(A)) if (!arayuzGosteriliyor(s)) diyalogKapat(s);
    gorunurlukGuncelle();
  }

  function arayuzEklentisiCiz(e) {
    const s = arayuzDurumu(e.ad);
    D.arayuzAlani = el("div", { class: "eklenti-arayuzu" });
    bosalt(gorunumAlani, D.arayuzAlani);
    arayuzCiz(s);
    if (!s.oturum && !s.bitti && !s.vazgecildi) arayuzBaslat(s);
    else if (s.oturum && !s.ekran) arayuzEkraniCek(s);
  }

  async function arayuzBaslat(s, onay) {
    if (s.aciliyor) return;
    s.aciliyor = true;
    s.vazgecildi = false;
    try {
      const r = await api("/api/eklenti/ac", { ad: s.ad, onay: !!onay, gomulu: true });
      if (r.izinGerekli) {
        s.aciliyor = false;
        izinSor(r, () => arayuzBaslat(s, true), () => { s.vazgecildi = true; arayuzCiz(s); });
        return;
      }
      if (s.oturum !== r.oturum) Object.assign(s, { oturum: r.oturum, ekran: null, onceki: null, surum: 0, bitti: false, kod: 0, hata: null, yazilan: {} });
      arayuzAkisiniAc();
      await arayuzEkraniCek(s);
      gezinmeCiz();
      gorunurlukGuncelle();
    } catch (e) {
      bildir(e.message, "hata");
    } finally {
      s.aciliyor = false;
      if (arayuzGosteriliyor(s)) arayuzCiz(s);
    }
  }

  function arayuzOlayi(s, olay, oid, deger, degerler) {
    if (!s.oturum || s.bitti) return;
    api("/api/arayuz/olay", { id: s.oturum, olay, oid: oid || "", deger: deger || "", degerler: degerler || undefined })
      .catch((e) => bildir(e.message, "hata"));
  }

  async function arayuzKapat(s) {
    if (eklentiCalisiyor(s.ad)) {
      const e = eklentiBilgisi(s.ad);
      if (!(await onayla(t("Eklentiyi kapat"), tf("%s hemen kapanır; kaydetmediğin işler kaybolabilir.", e ? e.etiket : s.ad), t("Eklentiyi kapat"), true))) return;
    }
    if (s.oturum) await api("/api/arayuz/kapat", { id: s.oturum }).catch(() => {});
    diyalogKapat(s);
    delete A[s.ad];
    gorunurlukGuncelle();
    git("forum");
  }

  // ── Çizim ──

  function arayuzCiz(s) {
    const alan = D.arayuzAlani;
    if (!alan || !arayuzGosteriliyor(s)) return;
    const bilgi = eklentiBilgisi(s.ad) || { etiket: s.ad };
    const e = s.ekran || {};
    const durum = durumuYakala(alan);
    const kapatDugmesi = el("button", { class: "ikon-dugme", title: t("Eklentiyi kapat"), "aria-label": t("Eklentiyi kapat"), on: { click: () => arayuzKapat(s) } }, ikon("kapat"));
    const araclar = s.bitti ? [] : (e.araclar || []).map((o, i) => ogeCiz(s, o, "a" + i));
    const baslik = e.baslik || bilgi.etiket;
    const ust = (e.yol && e.yol.length)
      ? el("header", { class: "ust-cubuk" }, ikon(e.ikon || kategoriIkonu(bilgi.kategoriKodu), "baslik-ikon"),
        el("nav", { class: "baslik-grubu a-yol", "aria-label": baslik }, e.yol.map((k, i) => [
          i ? el("span", { class: "a-yol-ayrac", "aria-hidden": "true" }, "›") : null,
          k.id && i < e.yol.length - 1
            ? el("button", { class: "a-yol-oge", on: { click: () => arayuzOlayi(s, "tikla", k.id) } }, k.metin)
            : el(i === e.yol.length - 1 ? "h1" : "span", { class: "a-yol-oge son" }, k.metin)])),
        el("div", { class: "bosluk" }), araclar, kapatDugmesi)
      : ustCubuk(e.ikon || kategoriIkonu(bilgi.kategoriKodu), baslik, e.altBaslik || bilgi.kategori, araclar, kapatDugmesi);

    let govde;
    if (s.bitti) {
      govde = el("div", { class: "a-bitti" }, bosDurum(kategoriIkonu(bilgi.kategoriKodu), tf("%s kapandı.", bilgi.etiket), s.kod ? tf("Çıkış kodu: %d", s.kod) : null),
        el("div", { class: "durum-dugmeleri" }, el("button", { class: "dugme birincil", on: { click: () => { Object.assign(s, { oturum: null, bitti: false }); arayuzBaslat(s); } } }, ikon("yenile"), t("Yeniden aç"))));
    } else if (s.vazgecildi) {
      govde = el("div", { class: "a-bitti" }, bosDurum("kalkan", t("Eklenti açılmadı")),
        el("div", { class: "durum-dugmeleri" }, el("button", { class: "dugme birincil", on: { click: () => arayuzBaslat(s) } }, tb("aç"))));
    } else if (!s.ekran) {
      govde = el("div", { class: "a-bekle" }, el("div", { class: "donen" }), el("p", { class: "soluk" }, t("Ekran bekleniyor…")));
    } else {
      govde = el("div", { class: "a-govde", "data-kaydir": "govde" },
        s.hata ? el("div", { class: "hata-kutusu" }, t("Eklentinin ekranı gösterilemedi."), " ", s.hata) : null,
        e.govde ? ogeCiz(s, e.govde, "g") : null);
    }
    bosalt(alan, ust, e.mesgul && !s.bitti ? el("div", { class: "a-mesgul", role: "progressbar" }) : null, govde,
      e.durum && !s.bitti ? el("div", { class: "a-durum" }, e.durum) : null);
    durumuGeriYukle(alan, durum);
    arayuzDiyalogu(s, s.bitti ? null : e.diyalog);
    const odak = !document.activeElement || document.activeElement === document.body ? alan.querySelector("[data-odak]") : null;
    if (odak) odak.focus();
  }

  // durumuYakala / durumuGeriYukle: yeniden çizimde odak, imleç ve kaydırma
  // konumları korunur (anahtar: öğe kimliği, yoksa ağaçtaki yolu).
  function durumuYakala(kok) {
    const d = { kaydir: {}, odak: null };
    for (const k of kok.querySelectorAll("[data-kaydir]")) {
      d.kaydir[k.getAttribute("data-kaydir")] = [k.scrollTop, k.scrollLeft, k.scrollHeight - k.scrollTop - k.clientHeight < 24];
    }
    const a = document.activeElement;
    if (a && kok.contains(a) && a.hasAttribute("data-anahtar")) {
      d.odak = { anahtar: a.getAttribute("data-anahtar") };
      try { d.odak.bas = a.selectionStart; d.odak.son = a.selectionEnd; } catch { /* seçimi olmayan öğe */ }
    }
    return d;
  }

  function durumuGeriYukle(kok, d) {
    for (const k of kok.querySelectorAll("[data-kaydir]")) {
      const v = d.kaydir[k.getAttribute("data-kaydir")];
      // Mesaj akışı ilk çizimde ve en alttayken yeni mesajla en altta kalır.
      if (k.classList.contains("a-mesajlar") && (!v || v[2])) k.scrollTop = k.scrollHeight;
      else if (v) { k.scrollTop = v[0]; k.scrollLeft = v[1]; }
    }
    if (!d.odak) return;
    const a = [...kok.querySelectorAll("[data-anahtar]")].find((x) => x.getAttribute("data-anahtar") === d.odak.anahtar);
    if (!a) return;
    a.focus({ preventScroll: true });
    if (d.odak.bas != null) try { a.setSelectionRange(d.odak.bas, d.odak.son); } catch { /* seçimi olmayan öğe */ }
  }

  // oncekiDeger, alanın bir önceki ekrandaki değeridir; eklenti değeri
  // değiştirmediyse kullanıcının yazdığı korunur.
  function oncekiDeger(s, id) {
    let bulunan;
    const ara = (o) => {
      if (!o || bulunan !== undefined) return;
      if (o.id === id && ["girdi", "metinAlani", "secim", "onayKutusu"].includes(o.tur)) { bulunan = o.tur === "onayKutusu" ? !!o.isaretli : (o.deger || ""); return; }
      for (const c of o.cocuklar || []) ara(c);
      ara(o.sol); ara(o.sag); ara(o.dugme);
      for (const c of o.araclar || []) ara(c);
    };
    const e = s.onceki || {};
    for (const c of e.araclar || []) ara(c);
    ara(e.govde);
    if (e.diyalog) { ara(e.diyalog.govde); for (const c of e.diyalog.dugmeler || []) ara(c); }
    return bulunan;
  }

  function alanDegeri(s, o) {
    const yeni = o.tur === "onayKutusu" ? !!o.isaretli : (o.deger || "");
    if (o.id in s.yazilan) {
      if (oncekiDeger(s, o.id) === yeni) return s.yazilan[o.id];
      delete s.yazilan[o.id]; // eklenti değeri değiştirdi (ör. gönderimden sonra boşalttı)
    }
    return yeni;
  }

  const STILLER = ["soluk", "kalin", "kod", "iyi", "uyari", "hata", "vurgu"];
  const stilSinifi = (stil) => (STILLER.includes(stil) ? " s-" + stil : "");

  function parcalarCiz(o) {
    if (!o.parcalar || !o.parcalar.length) return o.metin || "";
    return o.parcalar.map((p) => el("span", { class: stilSinifi(p.stil).trim() || undefined }, p.metin));
  }

  function ogeCiz(s, o, yol) {
    if (!o) return null;
    const anahtar = o.id || yol;
    const cocuklar = (liste) => (liste || []).map((c, i) => ogeCiz(s, c, yol + "." + i));
    switch (o.tur) {
      case "yigin": return el("div", { class: "a-yigin a-aralik-" + (o.aralik || "orta") }, cocuklar(o.cocuklar));
      case "satir": return el("div", { class: "a-satir" + (o.sarmala ? " sarmal" : "") + (o.hizala ? " a-hizala-" + o.hizala : "") }, cocuklar(o.cocuklar));
      case "bolunmus": return el("div", { class: "a-bolunmus" },
        el("div", { class: "a-sol", "data-kaydir": anahtar + "/sol" }, ogeCiz(s, o.sol, yol + ".sol")),
        el("div", { class: "a-sag", "data-kaydir": anahtar + "/sag" }, ogeCiz(s, o.sag, yol + ".sag")));
      case "bolum": return el("section", { class: "a-bolum" },
        o.baslik || (o.araclar && o.araclar.length) ? el("div", { class: "a-bolum-ust" },
          el("div", null, o.baslik ? el("h3", null, o.baslik) : null, o.aciklama ? el("p", { class: "soluk" }, o.aciklama) : null),
          el("div", { class: "a-bolum-arac" }, cocuklar(o.araclar))) : null,
        cocuklar(o.cocuklar));
      case "kart": return el("div", { class: "a-kart" }, cocuklar(o.cocuklar));
      case "sekmeler": return el("div", { class: "sekmeler a-sekmeler", role: "tablist" }, (o.sekmeler || []).map((k) =>
        el("button", { class: "sekme" + (o.secili === k.id ? " etkin" : ""), role: "tab", "aria-selected": o.secili === k.id ? "true" : "false",
          on: { click: () => { if (o.secili !== k.id) arayuzOlayi(s, "sekme", o.id, k.id); } } },
        k.ikon ? ikon(k.ikon) : null, k.etiket, k.sayac ? el("b", { class: "sayac" }, String(k.sayac)) : null)));
      case "ayrac": return el("hr", { class: "a-ayrac" });
      case "metin": return el("p", { class: "a-metin" + stilSinifi(o.stil) + (o.boyut ? " b-" + o.boyut : "") }, parcalarCiz(o));
      case "baslik": return el(o.duzey === 3 ? "h3" : "h2", { class: "a-baslik" }, o.metin);
      case "rozet": return rozetCiz(o.metin, o.renk);
      case "anahtarDeger": return el("div", { class: "a-kv" }, (o.ciftler || []).map((c) =>
        el("div", { class: "satir-kv" }, el("div", { class: "k" }, c.anahtar),
          el("div", { class: "v" + (c.kod ? " es-aralikli" : "") }, c.deger,
            c.kopyala ? el("button", { class: "ikon-dugme kucuk", title: t("Kopyala"), "aria-label": t("Kopyala"), on: { click: () => kopyala(c.deger) } }, ikon("kopyala")) : null))));
      case "bos": return el("div", { class: "a-bos" }, bosDurum(o.ikon || "eklenti", o.baslik, o.metin), o.dugme ? el("div", { class: "durum-dugmeleri" }, ogeCiz(s, o.dugme, yol + ".d")) : null);
      case "kod": return el("div", { class: "kod-blogu" }, el("pre", { "data-kaydir": anahtar }, o.satirNo
        ? o.metin.split("\n").map((m, i) => [el("span", { class: "a-satir-no", "aria-hidden": "true" }, String(i + 1)), m, "\n"])
        : o.metin));
      case "fark": return farkCiz(o, anahtar);
      case "ilerleme": {
        const cubuk = el("div", { class: "a-ilerleme" + (o.oran == null ? " belirsiz" : ""), role: "progressbar" }, el("div"));
        if (o.oran != null) cubuk.firstChild.style.width = (o.oran * 100) + "%";
        return cubuk;
      }
      case "baglanti": return el("button", { class: "a-baglanti", on: { click: () => (o.url ? disariAc(o.url) : arayuzOlayi(s, "tikla", o.id)) } }, o.metin);
      case "dugme": return dugmeCiz(s, o, null);
      case "liste": return listeCiz(s, o, anahtar);
      case "tablo": return tabloCiz(s, o);
      case "form": return el("form", { class: "a-form", "data-form": o.id, on: { submit: (ev) => { ev.preventDefault(); formGonder(s, ev.currentTarget); } } }, cocuklar(o.cocuklar));
      case "girdi": case "metinAlani": case "secim": case "onayKutusu": return alanCiz(s, o, anahtar, yol);
      case "mesajAkisi": return el("div", { class: "a-mesajlar", "data-kaydir": anahtar }, (o.mesajlar || []).map((m) =>
        el("div", { class: "a-mesaj" + (m.giden ? " giden" : "") + (m.durum ? " " + m.durum : "") },
          m.kimden && !m.giden ? el("div", { class: "a-mesaj-kimden" }, m.kimden) : null,
          el("div", { class: "a-mesaj-metin" }, m.metin), m.zaman ? el("time", { class: "a-mesaj-zaman", datetime: m.zaman }, saat(m.zaman)) : null)));
      default: return el("p", { class: "a-metin s-soluk" }, t("Bu öğe bu AND sürümünde gösterilemiyor."));
    }
  }

  function rozetCiz(metin, renk) {
    return el("span", { class: "rozet a-rozet" + (["vurgu", "iyi", "uyari", "hata"].includes(renk) ? " r-" + renk : "") }, metin);
  }

  function dugmeCiz(s, o, satir) {
    const varyant = { birincil: " birincil", tehlikeli: " tehlike", hayalet: " hayalet" }[o.varyant] || "";
    const tikla = async (ev) => {
      if (o.kopyala) { kopyala(o.kopyala); return; }
      if (o.onay && !(await onayla(o.onay.baslik || t("Emin misin?"), o.onay.metin || "", o.onay.dugme || o.metin, o.varyant === "tehlikeli"))) return;
      if (o.gonder) {
        const kap = ev.currentTarget.closest(".pencere") || ev.currentTarget.closest(".eklenti-arayuzu");
        const form = ev.currentTarget.closest("form") || (kap && kap.querySelector("form"));
        if (form) { formGonder(s, form); return; }
      }
      arayuzOlayi(s, "tikla", o.id, satir);
    };
    return el("button", {
      class: "dugme" + varyant + (satir != null ? " kucuk" : ""), type: "button", disabled: o.devreDisi,
      title: o.kisayol ? o.metin + " (" + o.kisayol + ")" : undefined, "data-kisayol": o.kisayol || undefined, "data-anahtar": o.id || undefined,
      on: { click: (ev) => { ev.stopPropagation(); tikla(ev); } },
    }, o.ikon ? ikon(o.ikon) : null, o.metin);
  }

  // formGonder, formun bütün alanlarını gönderir. Gönderilen alanların
  // değerini artık eklenti belirler (ör. mesaj gönderilince kutuyu boşaltır;
  // hata olursa değeri geri verir): kullanıcının yazdığı olarak tutulmazlar.
  function formGonder(s, form) {
    const degerler = {};
    for (const a of form.querySelectorAll("[data-alan]")) {
      const id = a.getAttribute("data-alan");
      degerler[id] = a.type === "checkbox" ? (a.checked ? "1" : "0") : a.value;
      delete s.yazilan[id];
    }
    arayuzOlayi(s, "gonder", form.getAttribute("data-form"), "", degerler);
  }

  function alanCiz(s, o, anahtar, yol) {
    const deger = alanDegeri(s, o);
    const kaydet = (v) => { s.yazilan[o.id] = v; };
    const ortak = { "data-alan": o.id, "data-anahtar": anahtar, "data-odak": o.odak ? "" : undefined, disabled: o.devreDisi, id: "a-" + yol };
    if (o.tur === "onayKutusu") {
      return el("label", { class: "a-onay" }, el("input", { ...ortak, type: "checkbox", checked: deger, on: { change: (ev) => kaydet(ev.target.checked) } }), el("span", null, o.etiket));
    }
    let girdi;
    if (o.tur === "secim") {
      girdi = el("select", { ...ortak, class: "secim", on: { change: (ev) => {
          // Anında seçimde (ör. dal) değeri eklenti belirler: işlem iptal
          // edilirse yeni ekran eski değeri gösterir.
          if (o.aninda) arayuzOlayi(s, "degisti", o.id, ev.target.value);
          else kaydet(ev.target.value);
        } } },
        (o.secenekler || []).map((c) => el("option", { value: c.deger }, c.etiket)));
      girdi.value = deger;
    } else if (o.tur === "metinAlani") {
      girdi = el("textarea", { ...ortak, class: "metin-alani" + (o.kod ? " es-aralikli a-kod" : ""), rows: o.satir || (o.kod ? 18 : 5), maxlength: o.enUzun || undefined,
        placeholder: o.yerTutucu || undefined, spellcheck: o.kod ? "false" : undefined, value: deger,
        on: { input: (ev) => kaydet(ev.target.value), keydown: (ev) => metinAlaniTusu(ev, o) } });
    } else {
      let zaman = null;
      girdi = el("input", { ...ortak, class: "girdi", type: o.tip === "sifre" ? "password" : "text", maxlength: o.enUzun || undefined, placeholder: o.yerTutucu || undefined,
        required: o.zorunlu, autocomplete: "off", value: deger, list: o.oneriler && o.oneriler.length ? "a-oneri-" + yol : undefined,
        on: {
          input: (ev) => {
            kaydet(ev.target.value);
            if (o.canli) { clearTimeout(zaman); zaman = setTimeout(() => arayuzOlayi(s, "degisti", o.id, ev.target.value), 250); }
          },
          keydown: (ev) => {
            if (ev.key !== "Enter" || ev.isComposing) return;
            const form = ev.target.closest("form");
            if (form) { ev.preventDefault(); formGonder(s, form); }
          },
        } });
    }
    return el("div", { class: "alan a-alan" },
      o.etiket ? el("label", { for: "a-" + yol }, o.etiket) : null,
      girdi,
      o.oneriler && o.oneriler.length ? el("datalist", { id: "a-oneri-" + yol }, o.oneriler.map((n) => el("option", { value: n.deger }, n.etiket || n.deger))) : null,
      o.aciklama ? el("div", { class: "alan-not" }, o.aciklama) : null,
      o.hata ? el("div", { class: "alan-durum asim", role: "alert" }, o.hata) : null);
  }

  function metinAlaniTusu(ev, o) {
    if (ev.key === "Enter" && (ev.ctrlKey || ev.metaKey)) {
      const form = ev.target.closest("form");
      if (form) { ev.preventDefault(); form.requestSubmit(); }
      return;
    }
    if (o.kod && ev.key === "Tab" && !ev.shiftKey && !ev.ctrlKey && !ev.altKey) {
      ev.preventDefault();
      ev.target.setRangeText("\t", ev.target.selectionStart, ev.target.selectionEnd, "end");
      ev.target.dispatchEvent(new Event("input"));
    }
  }

  function listeCiz(s, o, anahtar) {
    const ogeler = o.ogeler || [];
    let suzgec = "";
    const satirlar = el("div", { class: "a-liste", role: "listbox", tabindex: "0", "aria-activedescendant": undefined, "data-anahtar": anahtar,
      on: { keydown: (ev) => listeTusu(ev, s, o, satirlar) } });
    const ciz = () => {
      const g = ogeler.filter((x) => !suzgec || (x.baslik + " " + (x.alt || "")).toLocaleLowerCase(DIL).includes(suzgec));
      bosalt(satirlar, g.length ? g.map((x) => el("div", {
        class: "a-oge" + (o.secili === x.id ? " etkin" : "") + (x.soluk ? " soluk" : ""), role: "option", "aria-selected": o.secili === x.id ? "true" : "false", "data-oge": x.id,
        on: { click: () => { if (o.secili !== x.id) arayuzOlayi(s, "sec", o.id, x.id); }, dblclick: () => arayuzOlayi(s, "ac", o.id, x.id) },
      }, x.ikon ? ikon(x.ikon, "a-oge-ikon") : null,
      el("div", { class: "a-oge-metin" }, el("div", { class: "a-oge-baslik" }, el("span", { class: "a-oge-baslik-metin" }, x.baslik), (x.rozetler || []).map((r) => rozetCiz(r.metin, r.renk))),
        x.alt ? el("div", { class: "a-oge-alt" }, x.alt) : null),
      x.sag ? el("span", { class: "a-oge-sag" }, x.sag) : null,
      x.dugmeler && x.dugmeler.length ? el("div", { class: "a-oge-dugmeler" }, x.dugmeler.map((d) => dugmeCiz(s, d, x.id))) : null))
        : el("p", { class: "a-bos-metin soluk" }, o.bos || ""));
    };
    ciz();
    const arama = o.arama ? el("input", { class: "girdi a-suz", type: "search", placeholder: t("Süz…"), "data-anahtar": anahtar + "/suz",
      on: { input: (ev) => { suzgec = ev.target.value.trim().toLocaleLowerCase(DIL); ciz(); } } }) : null;
    const kap = el("div", { class: "a-liste-kap", "data-kaydir": anahtar }, arama, satirlar);
    setTimeout(() => { const e = satirlar.querySelector(".a-oge.etkin"); if (e) e.scrollIntoView({ block: "nearest" }); }, 0);
    return kap;
  }

  function listeTusu(ev, s, o, satirlar) {
    if (ev.target !== satirlar) return;
    const ids = [...satirlar.querySelectorAll("[data-oge]")].map((x) => x.getAttribute("data-oge"));
    const i = ids.indexOf(o.secili);
    if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
      ev.preventDefault();
      const j = Math.max(0, Math.min(ids.length - 1, i + (ev.key === "ArrowDown" ? 1 : -1)));
      if (ids[j] !== undefined && ids[j] !== o.secili) arayuzOlayi(s, "sec", o.id, ids[j]);
    } else if (ev.key === "Enter" && o.secili) {
      ev.preventDefault();
      arayuzOlayi(s, "ac", o.id, o.secili);
    }
  }

  function tabloCiz(s, o) {
    const hizala = (h) => (h === "orta" || h === "son" ? " h-" + h : "");
    return el("div", { class: "a-tablo-kap" }, el("table", { class: "a-tablo" },
      el("thead", null, el("tr", null, (o.sutunlar || []).map((c) => el("th", { class: hizala(c.hizala).trim() || undefined }, c.baslik)), (o.satirlar || []).some((r) => r.dugmeler && r.dugmeler.length) ? el("th") : null)),
      el("tbody", null, (o.satirlar || []).map((r) => el("tr", { class: o.secili === r.id ? "etkin" : undefined, on: { click: () => arayuzOlayi(s, "sec", o.id, r.id) } },
        (r.hucreler || []).map((h, i) => el("td", { class: hizala((o.sutunlar[i] || {}).hizala).trim() || undefined }, (h || []).map((p) => el("span", { class: stilSinifi(p.stil).trim() || undefined }, p.metin)))),
        r.dugmeler && r.dugmeler.length ? el("td", { class: "a-tablo-dugmeler" }, r.dugmeler.map((d) => dugmeCiz(s, d, r.id))) : null))),
      (o.satirlar || []).length ? null : el("caption", { class: "soluk" }, o.bos || "")));
  }

  function farkCiz(o, anahtar) {
    const tur = { "+": "ekle", "-": "sil", "@": "baslik" };
    return el("div", { class: "a-fark" }, (o.dosyalar || []).map((f, i) =>
      el("details", { class: "a-fark-dosya", open: true },
        el("summary", null, f.durum ? rozetCiz(f.durum, { A: "iyi", D: "hata", M: "vurgu" }[f.durum]) : null,
          el("span", { class: "a-fark-yol es-aralikli" }, f.yol),
          f.eklenen ? el("span", { class: "s-iyi" }, "+" + f.eklenen) : null, f.silinen ? el("span", { class: "s-hata" }, "−" + f.silinen) : null),
        el("pre", { class: "a-fark-satirlar", "data-kaydir": anahtar + "/" + i }, (f.satirlar || []).map((l) =>
          el("div", { class: "a-fark-satir " + (tur[l.tur] || "bag") },
            el("span", { class: "a-satir-no", "aria-hidden": "true" }, l.eski ? String(l.eski) : ""),
            el("span", { class: "a-satir-no", "aria-hidden": "true" }, l.yeni ? String(l.yeni) : ""),
            el("span", { class: "a-fark-isaret", "aria-hidden": "true" }, l.tur === "@" ? "" : l.tur), l.metin))))));
  }

  // ── Diyalog ──

  function diyalogKapat(s) {
    if (!s.diyalog) return;
    s.diyalog.perde.remove();
    s.diyalog = null;
  }

  function arayuzDiyalogu(s, d) {
    if (!d || !arayuzGosteriliyor(s)) { diyalogKapat(s); return; }
    const kimlik = d.id || d.baslik || "";
    if (s.diyalog && s.diyalog.kimlik !== kimlik) diyalogKapat(s);
    if (!s.diyalog) {
      const govde = el("div", { class: "pencere-govde" });
      const alt = el("div", { class: "pencere-alt" });
      const baslik = el("h2");
      const vazgec = () => arayuzOlayi(s, "diyalogKapat");
      const pencere = el("div", { class: "pencere genis a-diyalog", role: "dialog", "aria-modal": "true" },
        el("div", { class: "pencere-ust" }, baslik, el("button", { class: "ikon-dugme", "aria-label": t("Kapat"), on: { click: vazgec } }, ikon("kapat"))),
        govde, alt);
      const perde = el("div", { class: "perde", on: {
        mousedown: (ev) => { if (ev.target === perde) vazgec(); },
        keydown: (ev) => { if (ev.key === "Escape") { ev.preventDefault(); ev.stopPropagation(); vazgec(); } },
      } }, pencere);
      document.getElementById("katman").append(perde);
      s.diyalog = { kimlik, perde, pencere, govde, alt, baslik, yeni: true };
    }
    const g = s.diyalog;
    const durum = durumuYakala(g.pencere);
    g.baslik.textContent = d.baslik || "";
    g.pencere.setAttribute("aria-label", d.baslik || "");
    bosalt(g.govde, d.govde ? ogeCiz(s, d.govde, "d") : null);
    bosalt(g.alt, (d.dugmeler || []).map((o, i) => ogeCiz(s, o, "dd" + i)));
    durumuGeriYukle(g.pencere, durum);
    if (g.yeni) {
      g.yeni = false;
      setTimeout(() => {
        const ilk = g.pencere.querySelector("[data-odak]") || g.pencere.querySelector("input, textarea, select") || g.alt.querySelector(".birincil");
        if (ilk) ilk.focus();
      }, 0);
    }
  }

  // ── Klavye ──

  function tusAdi(ev) {
    const k = ev.key.length === 1 ? ev.key.toLowerCase() : ev.key.toLowerCase().replace("arrow", "");
    return (ev.ctrlKey || ev.metaKey ? "ctrl+" : "") + (ev.altKey ? "alt+" : "") + (ev.shiftKey && ev.key.length > 1 ? "shift+" : "") + k;
  }

  // arayuzTusu, eklentinin ekranı açıkken tuşları işler: eklentinin
  // kısayolları, Esc ve Alt+← ("geri"). İşlediyse true döner.
  function arayuzTusu(ev) {
    if (D.gorunum !== "eklenti") return false;
    const s = A[D.eklentiAd];
    if (!s || !s.oturum || s.bitti || !D.arayuzAlani) return false;
    const katman = document.getElementById("katman");
    const kendiDiyalogu = s.diyalog && katman.lastElementChild === s.diyalog.perde;
    if (katman.children.length && !kendiDiyalogu) return false; // AND'ın kendi penceresi açık
    const ad = tusAdi(ev);
    if (kendiDiyalogu && ad === "escape") {
      ev.preventDefault();
      arayuzOlayi(s, "diyalogKapat");
      return true;
    }
    const yaziyor = ev.target && ev.target.matches && ev.target.matches("input, textarea, select");
    if (!kendiDiyalogu && (ad === "escape" || ad === "alt+left") && !(yaziyor && ad === "escape")) {
      ev.preventDefault();
      arayuzOlayi(s, "geri");
      return true;
    }
    if (yaziyor && !ev.ctrlKey && !ev.metaKey && !ev.altKey) return false;
    const kok = kendiDiyalogu ? s.diyalog.pencere : D.arayuzAlani;
    const d = [...kok.querySelectorAll("[data-kisayol]")].find((b) => b.getAttribute("data-kisayol") === ad && !b.disabled);
    if (!d) return false;
    ev.preventDefault();
    d.click();
    return true;
  }

  // oturumlariYukle, AND'ın içinde açık olan eklentileri bağlar: açılışta
  // arka planda başlayanları, arayüz yenilenince (ya da dil değişince) hâlâ
  // çalışanları. Çalışan varsa olay akışları açılır; eklenti arka planda
  // kapanırsa kenar çubuğundaki nokta da kalkar.
  async function oturumlariYukle() {
    let r;
    try { r = await api("/api/eklenti/oturumlar"); } catch { return; }
    for (const x of r.oturumlar) {
      if (x.tur === "arayuz") {
        const s = arayuzDurumu(x.ad);
        if (s.oturum !== x.id && (!s.oturum || s.bitti)) Object.assign(s, { oturum: x.id, bitti: x.bitti, kod: x.kod, ekran: null, onceki: null, surum: 0, hata: null, yazilan: {} });
        if (!x.bitti) { arayuzAkisiniAc(); arayuzEkraniCek(s); }
      } else {
        const s = terminalDurumu(x.ad);
        if (!s.oturum) Object.assign(s, { oturum: x.id, bitti: x.bitti, kod: x.kod, ofs: 0 });
        if (!x.bitti) terminalAkisiniAc();
      }
    }
    if (r.oturumlar.length) gezinmeCiz();
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
    bosalt(gorunumAlani, ustCubuk("bilgi", t("AND hakkında"), "AND " + ((D.oturum && D.oturum.surum) || D.surum)),
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
    for (const e of (D.oturum && D.oturum.menu) || []) ekle(e.kategori, kategoriIkonu(e.kategoriKodu), e.etiket, () => eklentiGoster(e.ad));
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
    // Eklentinin terminali odaktayken bütün tuşlar eklentinindir.
    if (e.target && e.target.closest && e.target.closest(".terminal-kutusu")) return;
    if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "k") {
      e.preventDefault();
      hizliErisim();
      return;
    }
    if (arayuzTusu(e)) return;
    if (!e.altKey || e.ctrlKey || e.metaKey) return;
    const kisayol = { 1: "forum", 2: "sohbet", 3: "ayarlar", 4: "hakkinda" }[e.key];
    if (kisayol) { e.preventDefault(); git(kisayol); }
  });

  basla();
})();
