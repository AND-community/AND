import { test } from "node:test";
import assert from "node:assert/strict";
import { LiveApiError } from "../liveClient";
import { ForumChanges } from "../liveTypes";
import { ForumDegisimi, ForumIzleyici } from "./forumIzleyici";

/**
 * Sahte değişim ucu: her çağrıda sıradaki yanıtı (ya da hatayı) verir;
 * yanıtlar bitince durdurulana dek bekler.
 */
function sahteUc(yanitlar: (ForumChanges | Error)[]) {
  const istekler: number[] = [];
  let bitince: () => void = () => {};
  const bitti = new Promise<void>((r) => (bitince = r));
  return {
    istekler,
    bitti,
    forumChanges(since: number): Promise<ForumChanges> {
      istekler.push(since);
      const y = yanitlar.shift();
      if (y === undefined) {
        bitince();
        return new Promise(() => {});
      }
      return y instanceof Error ? Promise.reject(y) : Promise.resolve(y);
    },
  };
}

test("izleyici: her değişiklik bildirilir, since sunucunun sürümüyle ilerler", async () => {
  const uc = sahteUc([
    { surum: 3, konular: ["a"] },
    { surum: 3 }, // bekleme süresi doldu, değişiklik yok
    { surum: 5, konular: ["b", "a"] },
    { surum: 1, hepsi: true }, // AND yeniden başladı
  ]);
  const gelen: ForumDegisimi[] = [];
  const iz = new ForumIzleyici(uc, (d) => gelen.push(d), async () => {});
  iz.baslat();
  await uc.bitti;
  iz.durdur();
  assert.deepEqual(uc.istekler, [0, 3, 3, 5, 1]);
  assert.deepEqual(gelen, [
    { konular: ["a"], hepsi: false },
    { konular: ["b", "a"], hepsi: false },
    { konular: [], hepsi: true },
  ]);
});

test("izleyici: eski AND'da (404) durur ve bunu bildirir", async () => {
  const uc = sahteUc([new LiveApiError("HTTP 404", 404)]);
  let eski = 0;
  const iz = new ForumIzleyici(uc, () => assert.fail("değişiklik bildirilmemeli"), async () => {}, () => eski++);
  iz.baslat();
  await new Promise((r) => setImmediate(r));
  assert.equal(eski, 1);
  assert.deepEqual(uc.istekler, [0]);
  assert.equal(iz.calisiyor, false);
});

test("izleyici: başka bir hatada bekleyip yeniden dener", async () => {
  const uc = sahteUc([new Error("bağlantı koptu"), { surum: 2, konular: ["x"] }]);
  const beklemeler: number[] = [];
  const gelen: ForumDegisimi[] = [];
  const iz = new ForumIzleyici(uc, (d) => gelen.push(d), async (ms) => {
    beklemeler.push(ms);
  });
  iz.baslat();
  await uc.bitti;
  iz.durdur();
  assert.deepEqual(beklemeler, [5000]);
  assert.deepEqual(gelen, [{ konular: ["x"], hepsi: false }]);
});

test("izleyici: durdurulduktan sonra gelen yanıt bildirilmez; ikinci baslat tek döngü açar", async () => {
  let coz: (v: ForumChanges) => void = () => {};
  let cagri = 0;
  const uc = {
    forumChanges(): Promise<ForumChanges> {
      cagri++;
      return new Promise((r) => (coz = r));
    },
  };
  const iz = new ForumIzleyici(uc, () => assert.fail("durdurulmuş izleyici bildirmemeli"), async () => {});
  iz.baslat();
  iz.baslat();
  assert.equal(cagri, 1);
  iz.durdur();
  coz({ surum: 9, konular: ["a"] });
  await new Promise((r) => setImmediate(r));
  assert.equal(cagri, 1);
});
