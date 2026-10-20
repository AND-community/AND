import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { writeObject } from "../../objectStore";
import { Agac, Commit } from "../../types";
import { applyPack, buildPack, packFileName, parsePackFileName } from "./packSync";
import { buildTarGz } from "./tarball";

// sync.go'nun paketOlustur/paketUygula çiftinin TS karşılığı için round-trip
// ve bütünlük-doğrulama regresyon testleri.

function tmpRoot(t: { after: (fn: () => void) => void }): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "and-vscode-packsync-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

function seedCommit(root: string, projeId: string): string {
  const blobData = Buffer.from("dosya içeriği", "utf8");
  const blobHash = writeObject(root, projeId, "B", blobData);
  const tree: Agac = { girdiler: [{ yol: "a.txt", hash: blobHash, boyut: blobData.length }] };
  const treeHash = writeObject(root, projeId, "T", Buffer.from(JSON.stringify(tree), "utf8"));
  const commit: Commit = { agac: treeHash, yazar: "test", mesaj: "ilk commit", ts: new Date().toISOString() };
  return writeObject(root, projeId, "C", Buffer.from(JSON.stringify(commit), "utf8"));
}

test("buildPack/applyPack round-trip commit/tree/blob zincirini korur", (t) => {
  const srcRoot = tmpRoot(t);
  const projeId = "projKaynak";
  const commitHash = seedCommit(srcRoot, projeId);

  const { data, objectCount } = buildPack(srcRoot, projeId, "ana", commitHash, new Set());
  assert.equal(objectCount, 3, "commit + ağaç + blob = 3 nesne");

  const dstRoot = tmpRoot(t);
  const applied = applyPack(dstRoot, "projHedef", data);
  assert.equal(applied.objectCount, 3);
  assert.equal(applied.manifest.dal, "ana");
  assert.equal(applied.manifest.head, commitHash);
});

test("buildPack knownObjects kümesindeki ataları paketlemez", (t) => {
  const root = tmpRoot(t);
  const projeId = "proj1";
  const commitHash = seedCommit(root, projeId);

  const { objectCount } = buildPack(root, projeId, "ana", commitHash, new Set([commitHash]));
  assert.equal(objectCount, 0, "kök commit zaten biliniyorsa hiçbir şey toplanmamalı");
});

test("applyPack hash'i içeriğiyle eşleşmeyen nesneyi sessizce atlar", (t) => {
  const root = tmpRoot(t);
  const fakeHash = "0".repeat(64); // gerçek sha256("0"...+veri) değil — bütünlük ihlali
  const gz = buildTarGz([
    { name: "MANIFEST", data: Buffer.from(JSON.stringify({ dal: "ana", head: "x" }), "utf8") },
    { name: fakeHash, data: Buffer.from("bu veri hash'e karşılık gelmiyor", "utf8") },
  ]);

  const applied = applyPack(root, "projX", gz);
  assert.equal(applied.objectCount, 0, "hash uyuşmayan nesne uygulanmamalı");
});

test("packFileName/parsePackFileName round-trip grup ve proje kimliğini korur", () => {
  // rastgeleID()'nin ürettiği biçimle (16 küçük harf hex karakteri) aynı — bkz.
  // sync.go'daki reRastgeleID ve packSync.ts'teki RANDOM_ID.
  const grupId = "0123456789abcdef";
  const projeId = "fedcba9876543210";
  const name = packFileName(grupId, projeId);
  assert.match(name, new RegExp(`^projeag-${grupId}-${projeId}-.+\\.pack$`));
  assert.deepEqual(parsePackFileName(name), { grupId, projeId });
});

test("parsePackFileName biçime uymayan dosya adında undefined döner", () => {
  assert.equal(parsePackFileName("rastgele-dosya.txt"), undefined);
});

test("parsePackFileName grup/proje kimliği geçerli hex biçiminde değilse undefined döner (path traversal koruması)", () => {
  assert.equal(parsePackFileName("projeag-grup1-proje1-123abc.pack"), undefined);
  assert.equal(parsePackFileName("projeag-..%2f..%2fetc-0123456789abcdef-123abc.pack"), undefined);
});
