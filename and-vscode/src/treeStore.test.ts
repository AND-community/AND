import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { blobHashOf } from "./objectStore";
import {
  buildTreeFromWorkdir,
  checkoutTree,
  computeStatus,
  diffTrees,
  FileHashCache,
  readTree,
} from "./treeStore";

// vcs.go'daki "Çalışma dizini <-> ağaç" bölümünün TS karşılığı için round-trip,
// durum hesaplama, checkout ve diff regresyon testleri — gerçek bir geçici
// dizin ve gerçek nesne deposu köküyle çalışır (mock yok).

function tmpDir(t: { after: (fn: () => void) => void }): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "and-vscode-tree-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

function writeFile(dir: string, rel: string, content: string): void {
  const full = path.join(dir, ...rel.split("/"));
  fs.mkdirSync(path.dirname(full), { recursive: true });
  fs.writeFileSync(full, content, "utf8");
}

test("buildTreeFromWorkdir/readTree round-trip: girdiler yol sırasına göre sıralı ve içerik doğru", (t) => {
  const root = tmpDir(t);
  const workdir = tmpDir(t);
  const projeId = "projA";

  writeFile(workdir, "b.txt", "ikinci");
  writeFile(workdir, "a.txt", "ilk");
  writeFile(workdir, "sub/c.txt", "üçüncü — ğüşiöç");

  const built = buildTreeFromWorkdir(root, projeId, workdir);
  assert.equal(built.girdiler.length, 3);
  assert.deepEqual(
    built.girdiler.map((g) => g.yol),
    ["a.txt", "b.txt", "sub/c.txt"]
  );
  assert.equal(built.atlanan.length, 0);

  const read = readTree(root, projeId, built.hash);
  assert.deepEqual(read.girdiler, built.girdiler);

  const aEntry = built.girdiler.find((g) => g.yol === "a.txt")!;
  assert.equal(aEntry.hash, blobHashOf(Buffer.from("ilk", "utf8")));
});

test("buildTreeFromWorkdir: .projeagignore ile hariç tutulan dosyalar ağaca girmez", (t) => {
  const root = tmpDir(t);
  const workdir = tmpDir(t);
  const projeId = "projB";

  writeFile(workdir, ".projeagignore", "*.log\nnode_modules\n");
  writeFile(workdir, "main.ts", "kod");
  writeFile(workdir, "debug.log", "izlenmemeli");
  writeFile(workdir, "node_modules/pkg/index.js", "izlenmemeli");

  const built = buildTreeFromWorkdir(root, projeId, workdir);
  const yollar = built.girdiler.map((g) => g.yol).sort();
  // .projeagignore'un kendisi (git'teki .gitignore gibi) kendini otomatik
  // yok saymaz — yalnızca kendi listelediği desenlere uyan dosyalar hariç
  // tutulur; bu yüzden ağaçta izlenen dosyalarla birlikte kalır.
  assert.deepEqual(yollar, [".projeagignore", "main.ts"]);
});

test("computeStatus: eklenen/değişen/silinen doğru tespit edilir", (t) => {
  const root = tmpDir(t);
  const workdir = tmpDir(t);
  const projeId = "projC";

  writeFile(workdir, "kalici.txt", "değişmeyecek");
  writeFile(workdir, "silinecek.txt", "bu silinecek");
  writeFile(workdir, "degisecek.txt", "eski içerik");
  const head = buildTreeFromWorkdir(root, projeId, workdir);

  fs.rmSync(path.join(workdir, "silinecek.txt"));
  writeFile(workdir, "degisecek.txt", "yeni içerik");
  writeFile(workdir, "yeni.txt", "yeni dosya");

  const durum = computeStatus(root, projeId, workdir, head.hash);
  assert.deepEqual(durum.eklenen, ["yeni.txt"]);
  assert.deepEqual(durum.degisen, ["degisecek.txt"]);
  assert.deepEqual(durum.silinen, ["silinecek.txt"]);
});

test("computeStatus: hiçbir değişiklik yoksa durum tamamen boş", (t) => {
  const root = tmpDir(t);
  const workdir = tmpDir(t);
  const projeId = "projD";

  writeFile(workdir, "sabit.txt", "hiç değişmiyor");
  const head = buildTreeFromWorkdir(root, projeId, workdir);

  const durum = computeStatus(root, projeId, workdir, head.hash);
  assert.deepEqual(durum, { eklenen: [], degisen: [], silinen: [] });
});

test("computeStatus: FileHashCache doldurulur ve doğru hash'leri tutar", (t) => {
  const root = tmpDir(t);
  const workdir = tmpDir(t);
  const projeId = "projE";
  writeFile(workdir, "a.txt", "icerik");

  const cache: FileHashCache = new Map();
  const built = buildTreeFromWorkdir(root, projeId, workdir, cache);

  assert.equal(cache.size, 1);
  const entry = cache.get("a.txt");
  assert.ok(entry);
  assert.equal(entry!.hash, built.girdiler[0].hash);
});

test("checkoutTree: hedef ağacı yazar ve çalışma dizinindeki HER ŞEYİ (izlenmeyenler dahil) önce siler", (t) => {
  const root = tmpDir(t);
  const srcDir = tmpDir(t);
  const dstDir = tmpDir(t);
  const projeId = "projF";

  writeFile(srcDir, "a.txt", "A içeriği");
  writeFile(srcDir, "sub/b.txt", "B içeriği");
  const built = buildTreeFromWorkdir(root, projeId, srcDir);

  // dstDir'de hedef ağaçta hiç olmayan, kasıtlı bir "yabancı" dosya var.
  writeFile(dstDir, "yabanci.txt", "bu silinmeli");

  checkoutTree(root, projeId, dstDir, built.hash);

  assert.equal(fs.readFileSync(path.join(dstDir, "a.txt"), "utf8"), "A içeriği");
  assert.equal(fs.readFileSync(path.join(dstDir, "sub", "b.txt"), "utf8"), "B içeriği");
  assert.equal(fs.existsSync(path.join(dstDir, "yabanci.txt")), false);
});

test("checkoutTree: cache verilirse checkout sonunda tamamen temizlenir", (t) => {
  const root = tmpDir(t);
  const srcDir = tmpDir(t);
  const dstDir = tmpDir(t);
  const projeId = "projG";
  writeFile(srcDir, "a.txt", "icerik");
  const built = buildTreeFromWorkdir(root, projeId, srcDir);

  const cache: FileHashCache = new Map([["eski.txt", { mtimeMs: 1, size: 1, hash: "x".repeat(64) }]]);
  checkoutTree(root, projeId, dstDir, built.hash, cache);
  assert.equal(cache.size, 0);
});

test("diffTrees: iki ağaç arasındaki eklenen/değişen/silinen doğru hesaplanır", (t) => {
  const root = tmpDir(t);
  const dirV1 = tmpDir(t);
  const dirV2 = tmpDir(t);
  const projeId = "projH";

  writeFile(dirV1, "kalici.txt", "aynı");
  writeFile(dirV1, "eski.txt", "v1'de var");
  const v1 = buildTreeFromWorkdir(root, projeId, dirV1);

  writeFile(dirV2, "kalici.txt", "aynı");
  writeFile(dirV2, "yeni.txt", "v2'de eklendi");
  const v2 = buildTreeFromWorkdir(root, projeId, dirV2);

  const diff = diffTrees(root, projeId, v1.hash, v2.hash);
  assert.deepEqual(diff.eklenen, ["yeni.txt"]);
  assert.deepEqual(diff.degisen, []);
  assert.deepEqual(diff.silinen, ["eski.txt"]);
});
