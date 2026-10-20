import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { applyThreeWayMerge } from "./mergeEngine";
import { writeObject } from "./objectStore";
import { Agac } from "./types";

// vcs.go'daki "Üçlü birleştirme" bölümünün TS karşılığı için, mergeEntry'nin
// her dalını (otomatik-birleşme, çakışma, silme, tek-taraflı-yeni) tek tek
// tetikleyen senaryo testleri.

function tmpDir(t: { after: (fn: () => void) => void }): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "and-vscode-merge-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

// files: yol -> içerik (undefined ise o yol bu ağaçta yok).
function writeTree(root: string, projeId: string, files: Record<string, string | undefined>): string {
  const agac: Agac = { girdiler: [] };
  for (const [yol, icerik] of Object.entries(files)) {
    if (icerik === undefined) {
      continue;
    }
    const data = Buffer.from(icerik, "utf8");
    const hash = writeObject(root, projeId, "B", data);
    agac.girdiler!.push({ yol, hash, boyut: data.length });
  }
  return writeObject(root, projeId, "T", Buffer.from(JSON.stringify(agac), "utf8"));
}

test("applyThreeWayMerge: yalnızca uzakta eklenen dosya çakışmasız yazılır", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj1";

  const temel = writeTree(root, projeId, {});
  const yerel = writeTree(root, projeId, {});
  const uzak = writeTree(root, projeId, { "a.txt": "uzaktan eklendi" });

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8"), "uzaktan eklendi");
});

test("applyThreeWayMerge: yalnızca yerel değiştiyse (uzak taban ile aynı) yerel korunur", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj2";

  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "yerelde değişti" });
  const uzak = writeTree(root, projeId, { "a.txt": "taban" }); // uzak değişmemiş

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8"), "yerelde değişti");
});

test("applyThreeWayMerge: yalnızca uzak değiştiyse (yerel taban ile aynı) uzak alınır", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj3";

  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "taban" }); // yerel değişmemiş
  const uzak = writeTree(root, projeId, { "a.txt": "uzakta değişti" });

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8"), "uzakta değişti");
});

test("applyThreeWayMerge: ikisi de farklı değiştirdiyse çakışma işaretleri yazılır", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj4";

  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "yerel sürüm" });
  const uzak = writeTree(root, projeId, { "a.txt": "uzak sürüm" });

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, ["a.txt"]);
  const icerik = fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8");
  assert.match(icerik, /^<<<<<<< YEREL\nyerel sürüm\n=======\nuzak sürüm\n>>>>>>> UZAK\n$/);
});

test("applyThreeWayMerge: uzakta silinen ama yerelde değişmeyen dosya kaldırılır", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj5";

  fs.writeFileSync(path.join(calismaDir, "a.txt"), "eski");
  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "taban" }); // değişmemiş
  const uzak = writeTree(root, projeId, { "a.txt": undefined }); // uzakta silinmiş

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.existsSync(path.join(calismaDir, "a.txt")), false);
});

test("applyThreeWayMerge: uzak sildi ama yerel değiştirdi -> silme/değişiklik çakışması, yerel içerik korunur", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj6";

  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "yerelde değiştirildi" });
  const uzak = writeTree(root, projeId, { "a.txt": undefined }); // uzakta silinmiş

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, ["a.txt"]);
  const icerik = fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8");
  assert.match(icerik, /^<<<<<<< YEREL\nyerelde değiştirildi\n=======\n\n>>>>>>> UZAK\n$/);
});

test("applyThreeWayMerge: yalnız yerelde yeni olan dosya (tabanda hiç yok) korunur", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj7";

  const temel = writeTree(root, projeId, {});
  const yerel = writeTree(root, projeId, { "yeni.txt": "yalnız yerelde" });
  const uzak = writeTree(root, projeId, {});

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.readFileSync(path.join(calismaDir, "yeni.txt"), "utf8"), "yalnız yerelde");
});

test("applyThreeWayMerge: hem yerel hem uzak aynı içeriğe değiştiyse çakışma sayılmaz", (t) => {
  const root = tmpDir(t);
  const calismaDir = tmpDir(t);
  const projeId = "proj8";

  const temel = writeTree(root, projeId, { "a.txt": "taban" });
  const yerel = writeTree(root, projeId, { "a.txt": "ikisi de aynı" });
  const uzak = writeTree(root, projeId, { "a.txt": "ikisi de aynı" });

  const sonuc = applyThreeWayMerge(root, projeId, calismaDir, temel, yerel, uzak);
  assert.deepEqual(sonuc.catismalar, []);
  assert.equal(fs.readFileSync(path.join(calismaDir, "a.txt"), "utf8"), "ikisi de aynı");
});
