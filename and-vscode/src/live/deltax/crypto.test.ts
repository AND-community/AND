import { test } from "node:test";
import assert from "node:assert/strict";
import { decrypt, encrypt, generateSecret } from "./crypto";

// crypto.go'daki "grup paketleri için AES-256-GCM şifreleme"nin TS karşılığının
// (bu oturumda elle doğrulanan Go<->TS uyumluluğunun) kalıcı regresyon testi.

test("generateSecret 32 baytlık anahtarı hex olarak üretir", () => {
  const secret = generateSecret();
  assert.match(secret, /^[0-9a-f]{64}$/);
  assert.notEqual(secret, generateSecret(), "iki üretim aynı anahtarı vermemeli");
});

test("encrypt/decrypt round-trip orijinal veriyi geri verir", () => {
  const key = generateSecret();
  const plain = Buffer.from("merhaba dünya — ğüşiöç", "utf8");
  const sealed = encrypt(key, plain);
  assert.ok(sealed.length > plain.length, "nonce+tag eklendiği için şifreli veri daha uzun olmalı");
  const opened = decrypt(key, sealed);
  assert.ok(opened.equals(plain));
});

test("her şifreleme farklı bir nonce kullanır (aynı açık metin farklı çıktı üretir)", () => {
  const key = generateSecret();
  const plain = Buffer.from("aynı mesaj", "utf8");
  const a = encrypt(key, plain);
  const b = encrypt(key, plain);
  assert.notEqual(a.toString("hex"), b.toString("hex"));
});

test("decrypt yanlış anahtarla başarısız olur", () => {
  const sealed = encrypt(generateSecret(), Buffer.from("gizli"));
  assert.throws(() => decrypt(generateSecret(), sealed));
});

test("decrypt kurcalanmış veriyle (GCM etiketi uyuşmaz) başarısız olur", () => {
  const key = generateSecret();
  const sealed = encrypt(key, Buffer.from("gizli içerik"));
  sealed[sealed.length - 1] ^= 0xff; // GCM etiketinin son baytını boz
  assert.throws(() => decrypt(key, sealed));
});

test("decrypt çok kısa girdide anlamlı hata verir", () => {
  assert.throws(() => decrypt(generateSecret(), Buffer.from("kısa")));
});
