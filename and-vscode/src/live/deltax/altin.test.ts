import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "fs";
import * as path from "path";
import { decrypt, encrypt } from "./crypto";
import { decodeEnvelope, encodeEnvelope, InvitePayload } from "./protocol";

// DeltaX grup protokolünün Go (Eklentiler/DeltaX) ve TypeScript gerçeklemeleri
// AND deposundaki ortak örneklerle (protokol/testdata/deltax) birbirine karşı
// sınanır: burada Go'nun şifrelediği paket ve kodladığı zarf çözülür; burada
// üretilenleri de Go'nun testleri çözer (bkz. Eklentiler/DeltaX/altin_test.go).
// Örnekleri yeniden üretmek için AND_ALTIN_GUNCELLE=1 ile çalıştır.

const KOK = path.resolve(__dirname, "../../../../protokol/testdata/deltax");
const guncelle = process.env.AND_ALTIN_GUNCELLE === "1";
const oku = (ad: string): Buffer => fs.readFileSync(path.join(KOK, ad));
const sir = oku("sir.txt").toString("utf8").trim();
const acik = oku("acik.txt");

function davet(): InvitePayload {
  return {
    grup_ad: "Altın <grup> & ekip",
    sir,
    kurucu: { ad: "kurucu", peer_id: "12D3KooWKurucuAltin" },
    uyeler: [
      { ad: "kurucu", peer_id: "12D3KooWKurucuAltin" },
      { ad: "üye", peer_id: "12D3KooWUyeAltin", pub_key: "abcd" },
    ],
    gonderen: { ad: "kurucu", peer_id: "12D3KooWKurucuAltin" },
  };
}

test("TypeScript, Go'nun şifrelediği DeltaX paketini çözer", () => {
  assert.deepEqual(decrypt(sir, oku("sifreli_go.bin")), acik);
});

test("TypeScript, Go'nun kodladığı davet zarfını çözer", () => {
  const z = decodeEnvelope(oku("zarf_go.txt").toString("utf8"));
  assert.ok(z, "zarf çözülmeli");
  assert.equal(z.tip, "davet");
  assert.equal(z.grup_id, "grup-altin");
  assert.deepEqual(z.payload, davet());
});

test("TypeScript'in ürettiği örnekler kendi içinde tutarlı (Go testleri de çözer)", () => {
  if (guncelle) {
    fs.writeFileSync(path.join(KOK, "zarf_ts.txt"), encodeEnvelope({ tip: "davet", grup_id: "grup-altin", payload: davet() }));
    fs.writeFileSync(path.join(KOK, "sifreli_ts.bin"), encrypt(sir, acik));
  }
  assert.deepEqual(decrypt(sir, oku("sifreli_ts.bin")), acik);
  const z = decodeEnvelope(oku("zarf_ts.txt").toString("utf8"));
  assert.ok(z);
  assert.deepEqual(z.payload, davet());
});
