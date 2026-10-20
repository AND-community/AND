import { test } from "node:test";
import assert from "node:assert/strict";
import * as zlib from "node:zlib";
import { buildTarGz, parseTarGz } from "./tarball";

// sync.go'nun tar+gzip paket biçimiyle bire bir uyumlu olması gereken minimal
// USTAR yazıcı/okuyucunun round-trip ve bütünlük-kontrolü regresyon testleri.

test("buildTarGz/parseTarGz round-trip girdileri sırayla ve bozulmadan korur", () => {
  const entries = [
    { name: "MANIFEST", data: Buffer.from(JSON.stringify({ dal: "ana", head: "abc123" }), "utf8") },
    { name: "a".repeat(64), data: Buffer.from("blob içeriği — ğüşiöç", "utf8") },
    { name: "b".repeat(64), data: Buffer.alloc(0) },
  ];
  const gz = buildTarGz(entries);
  const parsed = parseTarGz(gz);
  assert.equal(parsed.length, entries.length);
  for (let i = 0; i < entries.length; i++) {
    assert.equal(parsed[i].name, entries[i].name);
    assert.ok(parsed[i].data.equals(entries[i].data), `girdi ${i} verisi eşleşmiyor`);
  }
});

test("blok sınırını aşan büyük veri de doğru paketlenir/çözülür", () => {
  const big = Buffer.alloc(512 * 3 + 17, 0x41); // 512'nin katı olmayan boyut -> dolgu yolunu dener
  const gz = buildTarGz([{ name: "c".repeat(64), data: big }]);
  const parsed = parseTarGz(gz);
  assert.equal(parsed.length, 1);
  assert.ok(parsed[0].data.equals(big));
});

test("boş arşiv sıfır girdiye çözülür", () => {
  assert.deepEqual(parseTarGz(buildTarGz([])), []);
});

test("100 bayttan uzun girdi adı reddedilir", () => {
  assert.throws(() => buildTarGz([{ name: "x".repeat(101), data: Buffer.from("y") }]));
});

test("bozuk başlık checksum'ı (kurcalanmış veri) reddedilir", () => {
  const gz = buildTarGz([{ name: "d".repeat(64), data: Buffer.from("orijinal içerik") }]);
  const tar = zlib.gunzipSync(gz);
  tar[10] ^= 0xff; // başlığın ad alanı içinde bir baytı boz -> checksum uyuşmaz
  const corrupted = zlib.gzipSync(tar);
  assert.throws(() => parseTarGz(corrupted), /checksum/);
});
