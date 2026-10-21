import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import {
  adParcasi,
  akisAdimi,
  durumDosyasi,
  gecerliPeerID,
  gelenYoklamadan,
  imleciOku,
  jsonYaz,
  konusmalar,
  makineIletisiMi,
  okunmamisSayisi,
  sonGelenZamani,
  sonraMi,
  yeniKayitlar,
} from "./dmCore";
import { DMRecord } from "./liveTypes";

const ALI = "12D3KooWAzizAbCdEfGhJkLmNpQrStUvWxYz123456789abcdefg";
const VELI = "12D3KooWVedatAbCdEfGhJkLmNpQrStUvWxYz123456789abcde";
const ZARF = "AND-PROJEAG/1|eyJ0aXAiOiJ1eWVfZ3VuY2VsbGUifQ==";

test("makineIletisiMi ve gecerliPeerID AND'daki denetimlerle aynı", () => {
  assert.ok(makineIletisiMi(ZARF));
  assert.ok(makineIletisiMi("AND-PROJEAG/2|"));
  assert.ok(!makineIletisiMi(" AND-PROJEAG/1|x"));
  assert.ok(!makineIletisiMi("merhaba"));
  assert.ok(gecerliPeerID(ALI));
  assert.ok(!gecerliPeerID("ali (12D3KooWAbCd…)"));
  assert.ok(!gecerliPeerID("0OIl0OIl0OIl0OIl0OIl0OIl0OIl0OIl0OIl0OIl0OIl0OIl"));
});

test("yoklamadan gelen mesajda gönderen doğrulanmış Peer ID'dir, görünen ad değil", () => {
  const simdi = new Date("2026-10-21T12:00:00Z");
  const dm = gelenYoklamadan({ from: "ali (12D3KooWAziz…)", verified_peer_id: ALI, text: "selam", received_at: "x" }, simdi);
  assert.equal(dm.peerId, ALI);
  assert.equal(dm.ad, "ali");
  assert.equal(dm.at, simdi.toISOString());
  // verified_peer_id göndermeyen çok eski AND: from ya kendi Peer ID'sidir ya da Peer ID biçiminde değildir.
  assert.equal(gelenYoklamadan({ from: ALI, text: "x", received_at: "" }, simdi).peerId, ALI);
  assert.ok(!gecerliPeerID(gelenYoklamadan({ from: "ali (12D3KooWAziz…)", text: "x", received_at: "" }, simdi).peerId));
  assert.equal(adParcasi(ALI), undefined);
});

test("sonraMi nanosaniyeyi ve saat dilimini dikkate alır", () => {
  assert.ok(sonraMi("2026-10-21T12:00:00.000000002Z", "2026-10-21T12:00:00.000000001Z"));
  assert.ok(!sonraMi("2026-10-21T12:00:00.0000001Z", "2026-10-21T12:00:00.0000001Z"));
  assert.ok(sonraMi("2026-10-21T15:00:01+03:00", "2026-10-21T12:00:00Z"));
  assert.ok(sonraMi("2026-10-21T12:00:00Z", undefined), "okundu işareti yoksa okunmamıştır");
  assert.ok(!sonraMi("bozuk", undefined));
});

function kayit(id: string, at: string, ek: Partial<DMRecord> = {}): DMRecord {
  return { id, peer_id: ALI, text: id, at, ...ek };
}

test("imleç: kayıt yerindeyse sıraya göre, düşmüşse zamana göre", () => {
  const k = [kayit("a", "2026-10-21T12:00:00Z"), kayit("b", "2026-10-21T12:00:01Z"), kayit("c", "2026-10-21T11:00:00Z")];
  const ids = (r: DMRecord[]) => r.map((x) => x.id).join(",");
  assert.equal(ids(yeniKayitlar({ id: "a", zaman: "2026-10-21T12:00:00Z" }, k)), "b,c", "saat geri alınsa da sıraya göre");
  assert.equal(ids(yeniKayitlar({ id: "c", zaman: "2026-10-21T11:00:00Z" }, k)), "");
  assert.equal(ids(yeniKayitlar({ id: "silinmis", zaman: "2026-10-21T12:00:00Z" }, k)), "b,c");
  assert.equal(ids(yeniKayitlar({ zaman: "2026-10-21T12:00:01Z" }, k)), "");
});

test("akış: ilk çalıştırmada eski mesajlar yeni sayılmaz, sonra yalnızca yeniler gelir", () => {
  const istendi = new Date("2026-10-21T12:00:05Z");
  const eski = [kayit("a", "2026-10-21T12:00:00Z")];
  const ilk = akisAdimi({}, eski, istendi);
  assert.deepEqual(ilk.yeniler, []);
  assert.deepEqual(ilk.imlec, { id: "a", zaman: "2026-10-21T12:00:00Z" });
  assert.ok(ilk.degisti);
  assert.deepEqual(akisAdimi({}, [], istendi).imlec, { zaman: istendi.toISOString() }, "geçmiş boşsa isteğin anı");

  // AND ?from= ile imlecin kaydından itibaren döner: kayıt başta, yeniler arkasında.
  const ikinci = akisAdimi(ilk.imlec, [...eski, kayit("b", "2026-10-21T12:00:06Z")], istendi);
  assert.deepEqual(ikinci.yeniler.map((r) => r.id), ["b"]);
  assert.equal(ikinci.imlec.id, "b");
  const ucuncu = akisAdimi(ikinci.imlec, [kayit("b", "2026-10-21T12:00:06Z")], istendi);
  assert.deepEqual(ucuncu, { yeniler: [], imlec: ikinci.imlec, degisti: false });
});

test("imleç dosyası: yazılır, okunur; bozuk dosya boş imleç verir", () => {
  const dizin = fs.mkdtempSync(path.join(os.tmpdir(), "dmcore-"));
  try {
    const dosya = durumDosyasi(dizin, "/veri/and", "dm-imleci");
    assert.notEqual(dosya, durumDosyasi(dizin, "/baska/and", "dm-imleci"), "her AND veri dizininin ayrı durumu olmalı");
    assert.deepEqual(imleciOku(dosya), { id: undefined, zaman: undefined });
    jsonYaz(dosya, { id: "x", zaman: "2026-10-21T12:00:00Z" });
    assert.deepEqual(imleciOku(dosya), { id: "x", zaman: "2026-10-21T12:00:00Z" });
    fs.writeFileSync(dosya, "{bozuk");
    assert.deepEqual(imleciOku(dosya), { id: undefined, zaman: undefined });
  } finally {
    fs.rmSync(dizin, { recursive: true, force: true });
  }
});

test("konuşmalar: makine mesajları ayıklanır, okunmamışlar AND'ın okundu bilgisiyle sayılır", () => {
  const k: DMRecord[] = [
    { peer_id: ALI, name: "ali", text: "selam", at: "2026-10-21T10:00:00Z" },
    { peer_id: VELI, text: "naber", at: "2026-10-21T10:01:00Z", outgoing: true, pending: true },
    { peer_id: ALI, name: "ali", text: ZARF, at: "2026-10-21T10:02:00Z" },
    { peer_id: ALI, name: "ali", text: "orada mısın", at: "2026-10-21T10:03:00Z" },
    { peer_id: VELI, text: ZARF, at: "2026-10-21T10:04:00Z" },
  ];
  const okundu = { [ALI]: "2026-10-21T10:00:00Z" };
  const liste = konusmalar(k, okundu);
  assert.deepEqual(
    liste.map((x) => [x.peerId, x.ad, x.okunmamis, x.mesajlar.map((m) => m.metin)]),
    [
      [ALI, "ali", 1, ["selam", "orada mısın"]],
      [VELI, "", 0, ["naber"]],
    ],
    "en son insan mesajı olan konuşma üstte; makine mesajı sıralamayı da etkilemez"
  );
  assert.equal(liste[1].mesajlar[0].bekliyor, true);
  assert.equal(okunmamisSayisi(k, okundu), 1);
  assert.equal(okunmamisSayisi(k, {}), 2);
  assert.equal(sonGelenZamani(k, ALI), "2026-10-21T10:03:00Z");
  assert.equal(sonGelenZamani(k, VELI), undefined, "giden ve makine mesajları okundu işaretinde sayılmaz");
});
