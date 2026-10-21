import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { LIDER_SURESI_MS, PencereLideri } from "./lider";

test("lider: tek pencere işler; lider susarsa ya da bırakırsa diğeri devralır", () => {
  const dizin = fs.mkdtempSync(path.join(os.tmpdir(), "lider-"));
  try {
    const dosya = path.join(dizin, "dm-lider.json");
    let saat = 1_000_000;
    const a = new PencereLideri(() => dosya, () => saat);
    const b = new PencereLideri(() => dosya, () => saat);

    assert.ok(a.al(), "ilk pencere lider olur");
    assert.ok(!b.al(), "lider tazeyken ikinci pencere beklemeli");
    saat += LIDER_SURESI_MS - 1;
    assert.ok(a.al(), "lider kaydını tazeler");
    saat += LIDER_SURESI_MS - 1;
    assert.ok(!b.al());

    saat += LIDER_SURESI_MS + 1; // a dondu ya da kapandı
    assert.ok(b.al(), "bayat kayıt devralınır");
    assert.ok(!a.al(), "eski lider geri dönünce yeni lideri ezmez");

    b.birak();
    assert.ok(a.al(), "bırakılan liderlik beklemeden devralınır");
    b.birak(); // lider değilken bırakmak başkasının kaydını silmez
    assert.ok(!b.al());
  } finally {
    fs.rmSync(dizin, { recursive: true, force: true });
  }
});
