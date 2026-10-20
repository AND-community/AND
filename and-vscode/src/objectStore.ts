import * as crypto from "crypto";
import * as fs from "fs";
import * as path from "path";
import { projeNesneDizini } from "./paths";

// vcs.go'daki nesne deposunun TypeScript karşılığı: nesneler tip baytı +
// payload'ın sha256'sıyla adreslenir ve nesneler/<hash[:2]>/<hash[2:]>
// altında saklanır. Aynı içerik iki kez yazılmaz.

const HEX64 = /^[0-9a-f]{64}$/;

export type ObjectType = "B" | "T" | "C"; // Blob | Ağaç | Commit

function typeCode(tip: ObjectType): number {
  return tip.charCodeAt(0);
}

/** Nesnenin tip baytıyla etiketlenmiş ham gövdesi — hash da bu baytlar üzerinden hesaplanır. */
function taggedBuffer(tip: ObjectType, payload: Buffer): Buffer {
  return Buffer.concat([Buffer.from([typeCode(tip)]), payload]);
}

export function objectHash(tip: ObjectType, payload: Buffer): string {
  return crypto.createHash("sha256").update(taggedBuffer(tip, payload)).digest("hex");
}

function writeRaw(root: string, projeId: string, hash: string, full: Buffer): void {
  const dir = path.join(projeNesneDizini(root, projeId), hash.slice(0, 2));
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  const file = path.join(dir, hash.slice(2));
  if (fs.existsSync(file)) {
    return; // içerik zaten var — tekilleştir
  }
  fs.writeFileSync(file, full, { mode: 0o600 });
}

export function writeObject(root: string, projeId: string, tip: ObjectType, payload: Buffer): string {
  const hash = objectHash(tip, payload);
  writeRaw(root, projeId, hash, taggedBuffer(tip, payload));
  return hash;
}

/**
 * writeObject ile aynı işi yapar (yalnızca 'B' blob tipi için), ancak
 * hash'i YENİDEN HESAPLAMAZ — çağıran tarafın `hash`in gerçekten
 * `objectHash('B', data)` olduğundan emin olması gerekir. treeStore.ts'teki
 * hashWithCache, bir dosyayı yeni okuyup hash'lediği anda bu hash'i zaten
 * elinde tutar; writeObject'i tekrar çağırmak aynı içeriği ikinci kez
 * hash'lemek anlamına gelirdi. Yanlış bir hash verilirse nesne deposu
 * bozulur — bu yüzden yalnızca hash'in doğruluğundan emin olunan dahili
 * kullanım içindir.
 */
export function writeBlobWithKnownHash(root: string, projeId: string, hash: string, data: Buffer): void {
  writeRaw(root, projeId, hash, taggedBuffer("B", data));
}

/**
 * Zaten tip baytıyla etiketlenmiş VE hash'i çağıran tarafça doğrulanmış ham
 * nesne baytlarını doğrudan diske yazar (yeniden hash'lemeden/etiketlemeden).
 * paketUygula'nın (packSync.ts) gelen bir pack'teki nesneleri uygularken
 * kullanması içindir — nesne zaten `sha256(fullTaggedBytes) === hash` olarak
 * doğrulanmış olmalıdır.
 */
export function writeVerifiedRawObject(root: string, projeId: string, hash: string, fullTaggedBytes: Buffer): void {
  writeRaw(root, projeId, hash, fullTaggedBytes);
}

export function objectPath(root: string, projeId: string, hash: string): string {
  if (!HEX64.test(hash)) {
    throw new Error(`geçersiz nesne hash'i: ${hash}`);
  }
  return path.join(projeNesneDizini(root, projeId), hash.slice(0, 2), hash.slice(2));
}

export interface RawObject {
  tip: number;
  payload: Buffer;
}

export function readObject(root: string, projeId: string, hash: string): RawObject {
  const file = objectPath(root, projeId, hash);
  const full = fs.readFileSync(file);
  if (full.length < 1) {
    throw new Error(`bozuk nesne: ${hash}`);
  }
  return { tip: full[0], payload: full.subarray(1) };
}

export function objectExists(root: string, projeId: string, hash: string): boolean {
  try {
    return fs.existsSync(objectPath(root, projeId, hash));
  } catch {
    return false;
  }
}

/** Ham dosya içeriğinin blob hash'i — sha256('B' + data). calismaSnapshotHesapla ile aynı formül. */
export function blobHashOf(data: Buffer): string {
  return objectHash("B", data);
}
