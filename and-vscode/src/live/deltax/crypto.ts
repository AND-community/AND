import * as crypto from "crypto";

// Go crypto.go'nun ("grup paketleri için AES-256-GCM şifreleme") birebir
// karşılığı. Go: gcm.Seal(nonce, nonce, data, nil) -> nonce(12) || ciphertext || tag(16).
// Node crypto GCM tag'i ayrı döndürür (getAuthTag()); aynı düzeni elde etmek
// için burada elle birleştiriyoruz.

const KEY_BYTES = 32;
const NONCE_BYTES = 12;
const TAG_BYTES = 16;

export function generateSecret(): string {
  return crypto.randomBytes(KEY_BYTES).toString("hex");
}

function keyFrom(secretHex: string): Buffer {
  const key = Buffer.from(secretHex, "hex");
  if (key.length !== KEY_BYTES) {
    throw new Error("geçersiz grup anahtarı");
  }
  return key;
}

export function encrypt(secretHex: string, plain: Buffer): Buffer {
  const key = keyFrom(secretHex);
  const nonce = crypto.randomBytes(NONCE_BYTES);
  const cipher = crypto.createCipheriv("aes-256-gcm", key, nonce);
  const ciphertext = Buffer.concat([cipher.update(plain), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([nonce, ciphertext, tag]);
}

export function decrypt(secretHex: string, sealed: Buffer): Buffer {
  const key = keyFrom(secretHex);
  if (sealed.length < NONCE_BYTES + TAG_BYTES) {
    throw new Error("şifreli veri çok kısa");
  }
  const nonce = sealed.subarray(0, NONCE_BYTES);
  const tag = sealed.subarray(sealed.length - TAG_BYTES);
  const ciphertext = sealed.subarray(NONCE_BYTES, sealed.length - TAG_BYTES);
  const decipher = crypto.createDecipheriv("aes-256-gcm", key, nonce);
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(ciphertext), decipher.final()]);
}
