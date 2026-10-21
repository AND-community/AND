import { test } from "node:test";
import assert from "node:assert/strict";
import * as fs from "fs";
import * as path from "path";
import { FORUM_CATEGORIES } from "./liveTypes";

// AND bu listede olmayan kategoriyle gelen konuyu reddeder: liste
// protokol/kategori.go'daki Kategoriler ile aynı sırada, birebir aynı olmalı.
// Uzantı AND deposunda (AND/and-vscode) durur; testler and-vscode'dan çalışır.
test("forum kategorileri AND'ın kapalı listesiyle aynı", () => {
  const kaynak = fs.readFileSync(path.join(process.cwd(), "..", "protokol", "kategori.go"), "utf8");
  const blok = /var Kategoriler = \[\]string\{([\s\S]*?)\n\}/.exec(kaynak);
  assert.ok(blok, "protokol/kategori.go'da Kategoriler bulunamadı");
  const goListesi = [...blok[1].matchAll(/"((?:[^"\\]|\\.)*)"/g)].map((m) => m[1]);
  assert.deepEqual(FORUM_CATEGORIES, goListesi);
});
