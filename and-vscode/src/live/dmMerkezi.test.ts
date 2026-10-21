import { test } from "node:test";
import assert from "node:assert/strict";
import { DMIstemcisi, DmMerkezi } from "./dmMerkezi";
import { LiveApiError } from "./liveClient";
import { DMRecord, SendDMReq } from "./liveTypes";

const ALI = "12D3KooWAzizAbCdEfGhJkLmNpQrStUvWxYz123456789abcdefg";

class SahteAND implements DMIstemcisi {
  gecmis: DMRecord[] | undefined = [];
  okundu: Record<string, string> | undefined = {};
  isaretler: [string, string][] = [];
  gonderilen: SendDMReq[] = [];

  async getDMHistory(): Promise<DMRecord[]> {
    if (!this.gecmis) {
      throw new LiveApiError("404 page not found", 404);
    }
    return this.gecmis;
  }
  async getDMReadMarks(): Promise<Record<string, string>> {
    if (!this.okundu) {
      throw new LiveApiError("404 page not found", 404);
    }
    return this.okundu;
  }
  async markDMRead(peerId: string, at: string): Promise<void> {
    if (!this.okundu) {
      throw new LiveApiError("404 page not found", 404);
    }
    this.isaretler.push([peerId, at]);
    this.okundu[peerId] = at;
  }
  async sendDM(req: SendDMReq): Promise<void> {
    this.gonderilen.push(req);
  }
}

test("DM merkezi: AND'ın geçmişi ve okundu bilgisi esastır", async () => {
  const and = new SahteAND();
  and.gecmis = [
    { peer_id: ALI, name: "ali", text: "selam", at: "2026-10-21T10:00:00Z" },
    { peer_id: ALI, name: "ali", text: "AND-PROJEAG/1|eyJ9", at: "2026-10-21T10:01:00Z" },
    { peer_id: ALI, text: "selam ali", at: "2026-10-21T10:02:00Z", outgoing: true, pending: true },
  ];
  const m = new DmMerkezi();
  let d = await m.durum(and);
  assert.equal(d.okunmamis, 1, "makine mesajı okunmamış sayılmaz");
  assert.equal(d.bekleyenVar, true);
  assert.equal(d.gecmisYok, false);

  assert.equal(await m.okunduIsaretle(and, ALI), true);
  assert.deepEqual(and.isaretler, [[ALI, "2026-10-21T10:00:00Z"]], "son İNSAN mesajına kadar işaretlenir");
  assert.equal(await m.okunduIsaretle(and, ALI), false, "ilerlemeyen işaret yeniden gönderilmez");
  d = await m.durum(and);
  assert.equal(d.okunmamis, 0);

  await m.gonder(and, ALI, "naber");
  assert.deepEqual(and.gonderilen, [{ peer_id: ALI, message: "naber", queue_if_offline: true }], "giden kutusuyla");
});

test("DM merkezi: geçmişi ve okundu bilgisi olmayan eski AND'da canlı gelenler ve yerel işaret", async () => {
  const and = new SahteAND();
  and.gecmis = undefined;
  and.okundu = undefined;
  const m = new DmMerkezi();
  assert.equal((await m.durum(and)).gecmisYok, true);
  m.canliGeldi([
    { peerId: ALI, ad: "ali", text: "selam", at: "2026-10-21T10:00:00Z" },
    { peerId: ALI, text: "AND-PROJEAG/1|eyJ9", at: "2026-10-21T10:00:01Z" },
  ]);
  await m.gonder(and, ALI, "naber");
  let d = await m.durum(and);
  assert.deepEqual(d.konusmalar[0].mesajlar.map((x) => x.metin), ["selam", "naber"]);
  assert.equal(d.okunmamis, 1);
  assert.equal(await m.okunduIsaretle(and, ALI), true);
  d = await m.durum(and);
  assert.equal(d.okunmamis, 0, "AND tutmuyorsa okundu bilgisi pencerede tutulur");
});

test("DM merkezi: 404 dışındaki hatalar yutulmaz (geçmiş kalıcı olarak kapatılmaz)", async () => {
  const m = new DmMerkezi();
  const bozuk = new SahteAND();
  bozuk.getDMHistory = async () => {
    throw new LiveApiError("bağlantı koptu", 500);
  };
  await assert.rejects(m.durum(bozuk));
  const iyi = new SahteAND();
  assert.equal((await m.durum(iyi)).gecmisYok, false);
});
