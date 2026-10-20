import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { globMatch, isIgnored, readIgnorePatterns } from "./ignoreRules";

// ignore.go'daki .projeagignore eşleştirme mantığının TS karşılığı için
// round-trip ve glob-semantiği regresyon testleri.

function tmpDir(t: { after: (fn: () => void) => void }): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "and-vscode-ignore-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  return dir;
}

test("globMatch: '*' hiçbir zaman '/' yutmaz", () => {
  assert.equal(globMatch("*.log", "app.log"), true);
  assert.equal(globMatch("*.log", "dir/app.log"), false); // '*' segment sınırını aşamaz
  assert.equal(globMatch("build/*.log", "build/app.log"), true);
  assert.equal(globMatch("build/*.log", "build/sub/app.log"), false);
});

test("globMatch: '?' tek karakterle eşleşir ama '/' ile eşleşmez", () => {
  assert.equal(globMatch("a?c", "abc"), true);
  assert.equal(globMatch("a?c", "ac"), false);
  assert.equal(globMatch("a?c", "a/c"), false);
});

test("globMatch: karakter sınıfları ([abc], [a-z], [^abc])", () => {
  assert.equal(globMatch("[abc].txt", "a.txt"), true);
  assert.equal(globMatch("[abc].txt", "d.txt"), false);
  assert.equal(globMatch("[a-z].txt", "m.txt"), true);
  assert.equal(globMatch("[a-z].txt", "M.txt"), false);
  assert.equal(globMatch("[^abc].txt", "d.txt"), true);
  assert.equal(globMatch("[^abc].txt", "a.txt"), false);
});

test("isIgnored: '/' içermeyen desen yolun herhangi bir segmentiyle eşleşir", () => {
  const desenler = ["node_modules", "*.tmp"];
  assert.equal(isIgnored(desenler, "node_modules/pkg/index.js"), true);
  assert.equal(isIgnored(desenler, "src/deep/node_modules/x.js"), true);
  assert.equal(isIgnored(desenler, "cache/file.tmp"), true);
  assert.equal(isIgnored(desenler, "src/main.ts"), false);
});

test("isIgnored: '/' içeren desen yalnızca tam göreli yolla eşleşir", () => {
  const desenler = ["build/output.log"];
  assert.equal(isIgnored(desenler, "build/output.log"), true);
  assert.equal(isIgnored(desenler, "other/build/output.log"), false);
  assert.equal(isIgnored(desenler, "build/sub/output.log"), false);
});

test("isIgnored: desen listesi boşsa hiçbir şey yok sayılmaz", () => {
  assert.equal(isIgnored([], "anything.txt"), false);
});

test("readIgnorePatterns: boş satırlar ve # yorumları atlanır, sondaki '/' korunur (isIgnored'da kırpılır)", (t) => {
  const dir = tmpDir(t);
  fs.writeFileSync(
    path.join(dir, ".projeagignore"),
    "\n# yorum satırı\nnode_modules\n\n*.log\n  \nbuild/\n"
  );
  const desenler = readIgnorePatterns(dir);
  assert.deepEqual(desenler, ["node_modules", "*.log", "build/"]);
});

test("readIgnorePatterns: dosya yoksa boş dizi döner", (t) => {
  const dir = tmpDir(t);
  assert.deepEqual(readIgnorePatterns(dir), []);
});

test("isIgnored: sondaki '/' kırpılır; '/' içermeyen sonuç segment eşleşmesiyle tüm alt ağacı yok sayar", () => {
  assert.equal(isIgnored(["build/"], "build"), true);
  // "build/" kırpılınca "build" olur — '/' İÇERMEYEN bir desen, herhangi bir
  // segmentle eşleşirse tüm yolu yok sayar; bu yüzden "build/" altındaki her
  // şey de (segment eşleşmesi yoluyla) yok sayılır.
  assert.equal(isIgnored(["build/"], "build/output.log"), true);
});
