import * as assert from "node:assert/strict";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { afterEach, test } from "node:test";
import {
  DILLER,
  KAYNAK,
  andDosyaDili,
  ayarla,
  betikDegeri,
  bicimle,
  binlik,
  cogulBicimi,
  dilKodu,
  dilSec,
  ekle,
  etkin,
  gomulu,
  ham,
  k,
  n,
  t,
} from "./dil";

afterEach(() => {
  ayarla(KAYNAK);
});

test("t: kaynak dilde metin olduğu gibi, çevirisi olmayan metin kaynak haliyle döner", () => {
  ekle({ "test: Gruplarım": { en: "My groups", de: "" } });
  assert.equal(t("test: Gruplarım"), "test: Gruplarım");
  ayarla("en");
  assert.equal(t("test: Gruplarım"), "My groups");
  assert.equal(t("test: çevirisi yok"), "test: çevirisi yok");
  ayarla("de");
  assert.equal(t("test: Gruplarım"), "test: Gruplarım", "boş çeviri kaynağa düşmeli");
});

test("t: tekil/çoğul biçimini ilk sayı argümanı seçer", () => {
  ekle({ "test: %s, %d dosya": { en: "%s, %d file|%s, %d files", ru: "%s, %d файл|%s, %d файла|%s, %d файлов" } });
  assert.equal(t("test: %s, %d dosya", "a", 3), "test: a, 3 dosya");
  ayarla("en");
  assert.equal(t("test: %s, %d dosya", "a", 1), "a, 1 file");
  assert.equal(t("test: %s, %d dosya", "a", 2), "a, 2 files");
  ayarla("ru");
  const beklenen: Record<number, string> = { 1: "a, 1 файл", 3: "a, 3 файла", 5: "a, 5 файлов", 21: "a, 21 файл", 12: "a, 12 файлов" };
  for (const [sayi, metin] of Object.entries(beklenen)) {
    assert.equal(t("test: %s, %d dosya", "a", Number(sayi)), metin);
  }
});

test("ham: çeviriyi biçimlendirmeden ve bütün çoğul biçimleriyle döndürür", () => {
  ekle({ "test: %d yanıt": { en: "%d reply|%d replies", fr: "%d réponse|%d réponses" } });
  ayarla("fr");
  assert.equal(ham("test: %d yanıt"), "%d réponse|%d réponses");
  assert.equal(t("test: %d yanıt", 2), "2 réponses");
  ayarla(KAYNAK);
  assert.equal(ham("test: %d yanıt"), "test: %d yanıt");
});

test("n: sayı argümanlarda değilken de biçimi seçer", () => {
  ekle({ "test: %s konu": { en: "%s topic|%s topics" }, "test: %d eş": { en: "%d peer|%d peers" } });
  ayarla("en");
  assert.equal(n(1200, "test: %s konu", "1,200"), "1,200 topics");
  assert.equal(n(1, "test: %s konu", "1"), "1 topic");
  assert.equal(n(2, "test: %d eş"), "2 peers");
});

test("k yalnızca işarettir", () => {
  ayarla("en");
  assert.equal(k("Gönder"), "Gönder");
});

test("bicimle: sıralı ve numaralı fiiller, %% ve eksik argüman", () => {
  assert.equal(bicimle("%s ve %s", ["a", "b"]), "a ve b");
  assert.equal(bicimle("%[2]s, %[1]s", ["a", "b"]), "b, a");
  assert.equal(bicimle("%d%% tamam", [42.7]), "42% tamam");
  assert.equal(bicimle("%s ve %s", ["a"]), "a ve %s", "eksik argüman fiili olduğu gibi bırakır");
  assert.equal(bicimle("$& %s", ["$1"]), "$& $1", "argümandaki $ kalıpları yorumlanmaz");
});

test("cogulBicimi Go'daki dil.cogulBicimi ile aynı tabloyu verir", () => {
  const tablo: [string, number, number][] = [
    ["tr", 1, 0], ["tr", 5, 0],
    ["en", 1, 0], ["en", 0, 1], ["en", 2, 1], ["en", -1, 0],
    ["de", 1, 0], ["de", 3, 1],
    ["fr", 0, 0], ["fr", 1, 0], ["fr", 2, 1],
    ["ru", 1, 0], ["ru", 21, 0], ["ru", 2, 1], ["ru", 24, 1],
    ["ru", 5, 2], ["ru", 11, 2], ["ru", 12, 2], ["ru", 111, 2], ["ru", 0, 2],
  ];
  for (const [kod, sayi, sira] of tablo) {
    assert.equal(cogulBicimi(kod, sayi), sira, `cogulBicimi(${kod}, ${sayi})`);
  }
});

test("ayarla ve dilKodu: bölge kodu ve büyük harf indirgenir, bilinmeyen dil reddedilir", () => {
  assert.equal(dilKodu(" EN "), "en");
  assert.equal(dilKodu("pt-BR"), "");
  assert.equal(dilKodu("de-AT"), "de");
  assert.equal(dilKodu("zh_CN"), "");
  assert.equal(ayarla("FR"), true);
  assert.equal(etkin(), "fr");
  assert.equal(ayarla("xx"), false);
  assert.equal(etkin(), "fr");
  assert.deepEqual(
    DILLER.map((d) => d.kod),
    ["tr", "en", "de", "es", "fr", "ru"],
    "AND'daki dil.Diller ile aynı sıra"
  );
});

test("dilSec: ayar > AND_DIL > AND'ın dil.json'u > VS Code dili > İngilizce", () => {
  assert.equal(dilSec({ ayar: "de", ortam: "fr", andDosyasi: "ru", vscode: "es" }), "de");
  assert.equal(dilSec({ ayar: "auto", ortam: "fr", andDosyasi: "ru", vscode: "es" }), "fr");
  assert.equal(dilSec({ ayar: "auto", andDosyasi: "ru", vscode: "es" }), "ru");
  assert.equal(dilSec({ ayar: "auto", andDosyasi: "", vscode: "es-ES" }), "es");
  assert.equal(dilSec({ ayar: "auto", vscode: "ja" }), "en", "desteklenmeyen VS Code dili");
  assert.equal(dilSec({ ayar: "xx", ortam: "xx", vscode: "tr" }), "tr", "geçersiz değerler atlanır");
});

test("andDosyaDili: AND'ın dil.json'unu okur; yoksa, bozuksa ya da dil bilinmiyorsa boş", () => {
  const dizin = fs.mkdtempSync(path.join(os.tmpdir(), "and-dil-"));
  try {
    assert.equal(andDosyaDili(dizin), "");
    fs.writeFileSync(path.join(dizin, "dil.json"), '{"dil":"ru"}');
    assert.equal(andDosyaDili(dizin), "ru");
    fs.writeFileSync(path.join(dizin, "dil.json"), '{"dil":"xx"}');
    assert.equal(andDosyaDili(dizin), "");
    fs.writeFileSync(path.join(dizin, "dil.json"), "{bozuk");
    assert.equal(andDosyaDili(dizin), "");
  } finally {
    fs.rmSync(dizin, { recursive: true, force: true });
  }
});

test("binlik etkin dilin ayırıcısını kullanır", () => {
  ayarla("en");
  assert.equal(binlik(1234567), "1,234,567");
  ayarla("de");
  assert.equal(binlik(1234567), "1.234.567");
});

test("betikDegeri betiği kapatamaz ve geri okunduğunda aynı değeri verir", () => {
  const deger = { metin: "</script><script>alert(1)</script>", sayi: 3 };
  const js = betikDegeri(deger);
  assert.ok(!js.includes("<"), js);
  assert.deepEqual(JSON.parse(js), deger);
});

test("kategoriler.json AND'daki dil/ceviriler/kategoriler.json ile aynı", (b) => {
  const andKopyasi = path.join(process.cwd(), "..", "dil", "ceviriler", "kategoriler.json");
  if (!fs.existsSync(andKopyasi)) {
    b.skip("AND kaynağı yanında değil");
    return;
  }
  const uzanti = JSON.parse(fs.readFileSync(path.join(process.cwd(), "src", "dil", "kategoriler.json"), "utf8"));
  assert.deepEqual(uzanti, JSON.parse(fs.readFileSync(andKopyasi, "utf8")));
  for (const kategori of Object.keys(uzanti)) {
    assert.ok(gomulu()[kategori], kategori);
  }
});
