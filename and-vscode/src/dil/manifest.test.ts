// Manifestin (package.json) dilleri: komut başlıkları, ayarlar ve görünüm adları
// package.nls.<dil>.json dosyalarından gelir. Her yer tutucunun her dilde
// karşılığı olmalı ve and.language ayarı uzantının dil listesiyle aynı kalmalı.

import * as assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { test } from "node:test";
import { DILLER, KENDILIGINDEN } from "./dil";

const KOK = path.resolve(__dirname, "..", "..");
const oku = (ad: string): Record<string, unknown> => JSON.parse(fs.readFileSync(path.join(KOK, ad), "utf8"));

/** package.json'daki bütün "%anahtar%" yer tutucuları. */
function yerTutucular(deger: unknown, sonuc = new Set<string>()): Set<string> {
  if (typeof deger === "string") {
    const m = /^%([\w.-]+)%$/.exec(deger);
    if (m) {
      sonuc.add(m[1]);
    }
  } else if (deger && typeof deger === "object") {
    for (const v of Object.values(deger)) {
      yerTutucular(v, sonuc);
    }
  }
  return sonuc;
}

const manifest = oku("package.json");
const anahtarlar = [...yerTutucular(manifest)].sort();
// package.nls.json VS Code'un varsayılanıdır (İngilizce); Türkçe ayrı dosyadadır.
const dosyalar = ["package.nls.json", ...DILLER.filter((d) => d.kod !== "en").map((d) => `package.nls.${d.kod}.json`)];

test("manifestin her yer tutucusunun her dilde karşılığı var, fazlası yok", () => {
  assert.ok(anahtarlar.length > 30, `yalnızca ${anahtarlar.length} yer tutucu bulundu`);
  for (const ad of dosyalar) {
    const nls = oku(ad);
    assert.deepEqual(Object.keys(nls).sort(), anahtarlar, ad);
    for (const [k, v] of Object.entries(nls)) {
      assert.ok(typeof v === "string" && v.trim() !== "", `${ad}: ${k} boş`);
    }
  }
});

test("karşılama metnindeki komut bağlantıları her dilde korunuyor", () => {
  const bag = (s: string): string[] => [...s.matchAll(/\]\((command:[\w.]+)\)/g)].map((m) => m[1]);
  const beklenen = bag(String(oku("package.nls.tr.json")["viewsWelcome.deltaxGroups"]));
  assert.deepEqual(beklenen, ["command:deltax.createGroup", "command:deltax.refresh"]);
  for (const ad of dosyalar) {
    assert.deepEqual(bag(String(oku(ad)["viewsWelcome.deltaxGroups"])), beklenen, ad);
  }
});

test("varsayılan dil dosyası İngilizce: Türkçeye özgü harf yok", () => {
  for (const [k, v] of Object.entries(oku("package.nls.json"))) {
    assert.doesNotMatch(String(v), /[ğışĞİŞ]/, k);
  }
});

test("and.language seçenekleri uzantının dil listesiyle aynı", () => {
  const ayarlar = (manifest.contributes as { configuration: { properties: Record<string, Record<string, unknown>> }[] }).configuration;
  const dil = ayarlar.map((a) => a.properties["and.language"]).find(Boolean);
  assert.ok(dil, "and.language ayarı yok");
  assert.equal(dil.default, KENDILIGINDEN);
  assert.deepEqual(dil.enum, [KENDILIGINDEN, ...DILLER.map((d) => d.kod)]);
  assert.deepEqual((dil.enumItemLabels as string[]).slice(1), DILLER.map((d) => d.ad));
});
