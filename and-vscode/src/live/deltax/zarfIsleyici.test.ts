import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { createProject, readGroup, saveGroup } from "../../groupProjectStore";
import { Grup, Uye } from "../../types";
import { GelenDM } from "../dmCore";
import { SendDMReq, SendFileReq } from "../liveTypes";
import { BekleyenDavet, DAVET_GECERLILIK_MS, bekleyenDavetler, davetGecerliMi, davetGonderildi } from "./davetler";
import { DeltaXIstemcisi, decodeEnvelope, encodeEnvelope, broadcastRoster, sendCloseNotice, sendLeaveNotice, requestPull } from "./protocol";
import { ZarfOrtami, davetiYanitla, zarfIsle } from "./zarfIsleyici";

const KURUCU = "12D3KooWKurucuAbCdEfGhJkLmNpQrStUvWxYz123456789abcd";
const DAVETLI = "12D3KooWDavetAbCdEfGhJkLmNpQrStUvWxYz123456789abcde";
const YONETICI = "12D3KooWYonetAbCdEfGhJkLmNpQrStUvWxYz123456789abcde";

class SahteIstemci implements DeltaXIstemcisi {
  dm: SendDMReq[] = [];
  dosya: SendFileReq[] = [];
  async sendDM(req: SendDMReq): Promise<void> {
    this.dm.push(req);
  }
  async sendFile(req: SendFileReq): Promise<void> {
    this.dosya.push(req);
  }
}

interface Kurulum {
  o: ZarfOrtami;
  root: string;
  istemci: SahteIstemci;
  bildirimler: string[];
  sorulan: BekleyenDavet[];
  kapat(): void;
}

function kur(ben: Uye, simdi = new Date("2026-10-21T12:00:00Z")): Kurulum {
  const dizin = fs.mkdtempSync(path.join(os.tmpdir(), "zarf-"));
  const root = path.join(dizin, "DeltaX");
  const istemci = new SahteIstemci();
  const bildirimler: string[] = [];
  const sorulan: BekleyenDavet[] = [];
  const o: ZarfOrtami = {
    root: () => root,
    defterDosyasi: () => path.join(dizin, "durum", "defter.json"),
    bekleyenDosyasi: () => path.join(dizin, "durum", "bekleyen.json"),
    istemci: () => istemci,
    ben: async () => ben,
    bildir: (m) => bildirimler.push(m),
    davetGeldi: (d) => sorulan.push(d),
    gruplarDegisti: () => undefined,
    simdi: () => simdi,
  };
  return { o, root, istemci, bildirimler, sorulan, kapat: () => fs.rmSync(dizin, { recursive: true, force: true }) };
}

function grup(root: string, uyeler: Uye[] = []): Grup {
  const g: Grup = {
    id: "0123456789abcdef",
    ad: "Proje Ekibi",
    sir: "a".repeat(64),
    kurucu_ad: "kurucu",
    kurucu_peer: KURUCU,
    uyeler: [{ ad: "kurucu", peer_id: KURUCU }, ...uyeler],
    olusturma_ts: "2026-10-01T00:00:00Z",
  };
  saveGroup(root, g);
  return g;
}

function dm(peerId: string, tip: string, payload: unknown, at = "2026-10-21T11:59:00Z", ek: object = {}): GelenDM {
  return { peerId, ad: "biri", at, text: encodeEnvelope({ tip: tip as never, grup_id: "0123456789abcdef", payload, ...ek }) };
}

const kurucu: Uye = { ad: "kurucu", peer_id: KURUCU };
const davetli: Uye = { ad: "davetli", peer_id: DAVETLI };
const davetPayload = { grup_ad: "Proje Ekibi", sir: "a".repeat(64), kurucu, uyeler: [kurucu], gonderen: kurucu };

test("davet: gönderen doğrulanmış Peer ID'yle eşleşir (görünen ad değil) ve bekleyenlere eklenir", async () => {
  const k = kur(davetli);
  try {
    assert.equal(await zarfIsle(k.o, dm(KURUCU, "davet", davetPayload)), true);
    assert.equal(k.sorulan.length, 1, "adı olan bir gönderenin daveti kaybolmamalı");
    assert.equal(bekleyenDavetler(k.o.bekleyenDosyasi(), new Date("2026-10-21T12:00:00Z")).length, 1);
    await zarfIsle(k.o, dm(KURUCU, "davet", davetPayload));
    assert.equal(k.sorulan.length, 1, "aynı grubun daveti iki kez sorulmaz");

    // Payload'da başkasının kimliğini taşıyan zarf (taklit) yok sayılır.
    const k2 = kur(davetli);
    await zarfIsle(k2.o, dm(YONETICI, "davet", davetPayload));
    assert.equal(k2.sorulan.length, 0);
    k2.kapat();
  } finally {
    k.kapat();
  }
});

test("davet: 30 günden eski davet işlenmez; zarf olmayan metin genel akışa düşer", async () => {
  const k = kur(davetli);
  try {
    const eski = new Date(Date.parse("2026-10-21T12:00:00Z") - DAVET_GECERLILIK_MS - 60_000).toISOString();
    await zarfIsle(k.o, dm(KURUCU, "davet", davetPayload, eski));
    assert.equal(k.sorulan.length, 0);
    assert.equal(await zarfIsle(k.o, { peerId: KURUCU, text: "selam", at: "2026-10-21T11:00:00Z" }), false);
  } finally {
    k.kapat();
  }
});

test("davet kabulü: yalnızca gerçekten davet edilen tanınır; ret daveti tüketir ve bildirilir", async () => {
  const k = kur(kurucu);
  try {
    grup(k.root);
    const kabul = { kabul: true, uye: davetli };
    await zarfIsle(k.o, dm(DAVETLI, "davet_yanit", kabul));
    assert.ok(!readGroup(k.root, "0123456789abcdef").uyeler.some((u) => u.peer_id === DAVETLI), "davetsiz katılamaz");

    davetGonderildi(k.o.defterDosyasi(), "0123456789abcdef", DAVETLI, new Date("2026-10-21T11:00:00Z"));
    await zarfIsle(k.o, dm(DAVETLI, "davet_yanit", { kabul: false, uye: davetli }));
    assert.ok(k.bildirimler.some((b) => b.includes("reddetti")));
    await zarfIsle(k.o, dm(DAVETLI, "davet_yanit", kabul));
    assert.ok(!readGroup(k.root, "0123456789abcdef").uyeler.some((u) => u.peer_id === DAVETLI), "reddedilen davet sonradan kabul edilemez");
  } finally {
    k.kapat();
  }
});

test("davet kabulü: VS Code kapalıyken gelen kabul tanınır, üye listesi giden kutusuyla yayınlanır", async () => {
  const k = kur(kurucu);
  try {
    grup(k.root, [{ ad: "yonetici", peer_id: YONETICI }]);
    davetGonderildi(k.o.defterDosyasi(), "0123456789abcdef", DAVETLI, new Date("2026-10-15T11:00:00Z"));
    await zarfIsle(k.o, dm(DAVETLI, "davet_yanit", { kabul: true, uye: davetli }, "2026-10-16T09:00:00Z"));
    const g = readGroup(k.root, "0123456789abcdef");
    assert.ok(g.uyeler.some((u) => u.peer_id === DAVETLI));
    assert.ok(!davetGecerliMi(k.o.defterDosyasi(), g.id, DAVETLI, new Date()), "kullanılan davet düşer");
    assert.deepEqual(k.istemci.dm.map((r) => [r.peer_id, r.queue_if_offline]).sort(), [
      [DAVETLI, true],
      [YONETICI, true],
    ]);
    assert.equal(decodeEnvelope(k.istemci.dm[0].message)?.tip, "uye_guncelle");
  } finally {
    k.kapat();
  }
});

test("çek isteği: bir saatten eskisi yanıtlanmaz, yenisi karşılanmaya çalışılır", async () => {
  const k = kur(kurucu);
  try {
    grup(k.root, [davetli]);
    const p = createProject(k.root, "0123456789abcdef", "proje");
    const istek = { dal: "ana", bilinen_hash: "", isteyen: davetli };
    await zarfIsle(k.o, dm(DAVETLI, "pull_istek", istek, "2026-10-21T10:30:00Z", { proje_id: p.id }));
    assert.equal(k.bildirimler.length, 0, "eski istek sessizce atlanmalı");
    await zarfIsle(k.o, dm(DAVETLI, "pull_istek", istek, "2026-10-21T11:59:00Z", { proje_id: p.id }));
    assert.ok(k.bildirimler.some((b) => b.startsWith("Çek isteği karşılanamadı")), "henüz kayıt yokken push denenip hata bildirilmeli");
  } finally {
    k.kapat();
  }
});

test("üye listesi: yalnızca yetkiliden; ayrılma yalnızca kendisi için; kapatma yetkiliden", async () => {
  const k = kur(kurucu);
  try {
    grup(k.root, [davetli]);
    const sahteListe = { ...davetPayload, uyeler: [davetli], gonderen: davetli };
    await zarfIsle(k.o, dm(DAVETLI, "uye_guncelle", sahteListe));
    assert.equal(readGroup(k.root, "0123456789abcdef").uyeler.length, 2, "yetkisiz üye listesi uygulanmaz");

    await zarfIsle(k.o, dm(YONETICI, "ayril", { uye: davetli }));
    assert.equal(readGroup(k.root, "0123456789abcdef").uyeler.length, 2, "başkası adına ayrılma yok sayılır");
    await zarfIsle(k.o, dm(DAVETLI, "ayril", { uye: davetli }));
    assert.equal(readGroup(k.root, "0123456789abcdef").uyeler.length, 1);

    const k2 = kur(davetli);
    grup(k2.root, [davetli]);
    await zarfIsle(k2.o, dm(YONETICI, "grup_kapat", { gonderen: { ad: "y", peer_id: YONETICI } }));
    assert.doesNotThrow(() => readGroup(k2.root, "0123456789abcdef"), "yetkisiz kapatma yok sayılır");
    await zarfIsle(k2.o, dm(KURUCU, "grup_kapat", { gonderen: kurucu }));
    assert.throws(() => readGroup(k2.root, "0123456789abcdef"));
    k2.kapat();
  } finally {
    k.kapat();
  }
});

test("bekleyen davet: kabul edilince grup kurulur, yanıt giden kutusuyla gider, davet listeden düşer", async () => {
  const k = kur(davetli);
  try {
    await zarfIsle(k.o, dm(KURUCU, "davet", davetPayload));
    const g = await davetiYanitla(k.o, k.sorulan[0], true);
    assert.ok(g && g.uyeler.some((u) => u.peer_id === DAVETLI));
    assert.equal(readGroup(k.root, "0123456789abcdef").ad, "Proje Ekibi");
    assert.equal(k.istemci.dm[0].peer_id, KURUCU);
    assert.equal(k.istemci.dm[0].queue_if_offline, true);
    assert.deepEqual(decodeEnvelope(k.istemci.dm[0].message)?.payload, { kabul: true, uye: davetli });
    assert.equal(bekleyenDavetler(k.o.bekleyenDosyasi(), new Date("2026-10-21T12:00:00Z")).length, 0);
  } finally {
    k.kapat();
  }
});

test("gönderimler: grup durumu giden kutusuyla ve tekrarsız, çek isteği doğrudan", async () => {
  const istemci = new SahteIstemci();
  const g: Grup = {
    id: "0123456789abcdef",
    ad: "g",
    sir: "s",
    kurucu_ad: "kurucu",
    kurucu_peer: KURUCU,
    yoneticiler: [{ ad: "y", peer_id: YONETICI }],
    uyeler: [kurucu, davetli, { ad: "y", peer_id: YONETICI }, { ad: "tekrar", peer_id: DAVETLI }],
    olusturma_ts: "",
  };
  assert.deepEqual(await broadcastRoster(istemci, g, kurucu), []);
  assert.deepEqual(await sendCloseNotice(istemci, g, kurucu), []);
  assert.deepEqual(await sendLeaveNotice(istemci, g, davetli), []);
  assert.equal(istemci.dm.length, 6, "her yayında kendisi ve tekrarlar hariç 2 alıcı");
  assert.ok(istemci.dm.every((r) => r.queue_if_offline === true));
  await requestPull(istemci, davetli, g, { id: "p", grup_id: g.id, ad: "p", olusturma_ts: "", dallar: { ana: "" }, aktif_dal: "ana" }, kurucu);
  assert.equal(istemci.dm[6].queue_if_offline, undefined, "çek isteği iki tarafın çevrimiçi olmasını gerektirir");
});
