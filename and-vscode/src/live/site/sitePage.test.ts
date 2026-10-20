import { test } from "node:test";
import assert from "node:assert/strict";
import {
  SITE_MAX_BODY_BYTES,
  SITE_MAX_SLUG_BYTES,
  SITE_MAX_TITLE_BYTES,
  SITE_PUBLISH_SPACING_MS,
  bodyOverflowOffset,
  isValidSlug,
  parsePage,
  pathForSlug,
  publishDelayMs,
  serializePage,
  shareCode,
  shareLink,
  slugFromPath,
  utf8Bytes,
  validateNewSlug,
  validatePage,
} from "./sitePage";

// AND'ın internal/site/site.go'su (sınırlar, slug kuralı) ve Arama Motoru'nun
// paylaşım kodu biçimiyle uyumun regresyon testleri.

test("isValidSlug: internal/site'ın TestValidSlug vakalarıyla birebir aynı sonuç", () => {
  for (const s of ["home", "a", "about-me", "proje-2026", "x1"]) {
    assert.equal(isValidSlug(s), true, s);
  }
  for (const s of ["", "-leading", "trailing-", "UPPER", "with space", "ünlü", "a/b", "-", "a".repeat(SITE_MAX_SLUG_BYTES + 1)]) {
    assert.equal(isValidSlug(s), false, s);
  }
  assert.equal(isValidSlug("a".repeat(SITE_MAX_SLUG_BYTES)), true);
});

test("utf8Bytes: Türkçe karakterler 2 bayt sayılır (Go'nun len(string)'i gibi)", () => {
  assert.equal(utf8Bytes("abc"), 3);
  assert.equal(utf8Bytes("ğüşıöç"), 12);
  assert.equal(utf8Bytes("😀"), 4);
});

test("serializePage/parsePage: gidiş-dönüş başlığı ve gövdeyi korur", () => {
  const cases = [
    { title: "Hakkımda", body: "Merhaba, ben bir AND kullanıcısıyım.\n\n- Go\n- Rust" },
    { title: "#etiket ile başlayan", body: "# Gövdedeki bir başlık\nmetin" },
    { title: "", body: "başlıksız sayfa" },
    { title: "Boşluklu   iç   başlık", body: "ilk satır\n\n\nüç boş satırdan sonra" },
  ];
  for (const page of cases) {
    assert.deepEqual(parsePage(serializePage(page)), page);
  }
});

test("serializePage: satır sonu içeren bir başlık tek satıra indirgenir", () => {
  const doc = serializePage({ title: "iki\nsatır", body: "gövde" });
  assert.equal(doc, "# iki satır\n\ngövde");
  assert.deepEqual(parsePage(doc), { title: "iki satır", body: "gövde" });
});

test("parsePage: hoşgörülü ayrıştırma — CRLF, eksik '#', eksik ayırıcı satır", () => {
  assert.deepEqual(parsePage("# Başlık\r\n\r\nsatır 1\r\nsatır 2\r\n"), { title: "Başlık", body: "satır 1\nsatır 2" });
  assert.deepEqual(parsePage("Düz başlık\ngövde"), { title: "Düz başlık", body: "gövde" });
  assert.deepEqual(parsePage("#Bitişik\n\ngövde"), { title: "Bitişik", body: "gövde" });
  assert.deepEqual(parsePage("# Yalnızca başlık"), { title: "Yalnızca başlık", body: "" });
  assert.deepEqual(parsePage(""), { title: "", body: "" });
});

test("parsePage: baş/son boşluklar kırpılır (Eklentiler/site'ın kaydet()'i gibi)", () => {
  assert.deepEqual(parsePage("#   Başlık  \n\n\n  gövde  \n\n"), { title: "Başlık", body: "gövde" });
});

test("validatePage: geçerli bir sayfa için sorun yok", () => {
  assert.equal(validatePage("home", { title: "t", body: "b" }), undefined);
  assert.equal(validatePage("home", { title: "", body: "başlıksız olabilir" }), undefined);
});

test("validatePage: geçersiz slug, boş gövde ve bayt sınırları (karakter değil) yakalanır", () => {
  assert.match(validatePage("Kötü Slug", { title: "t", body: "b" }) ?? "", /Geçersiz slug/);
  assert.match(validatePage("home", { title: "t", body: "" }) ?? "", /boş olamaz/);

  // 51 karakter ama 102 bayt — karakter sayan bir kontrol bunu kaçırırdı.
  const title = "ğ".repeat(51);
  assert.equal(title.length <= SITE_MAX_TITLE_BYTES, true);
  assert.match(validatePage("home", { title, body: "b" }) ?? "", /Başlık en fazla 100 bayt olabilir \(şu an 102 bayt\)/);

  assert.equal(validatePage("home", { title: "t", body: "a".repeat(SITE_MAX_BODY_BYTES) }), undefined);
  assert.match(validatePage("home", { title: "t", body: "a".repeat(SITE_MAX_BODY_BYTES + 1) }) ?? "", /8000 bayt/);
});

test("validateNewSlug: boş, geçersiz ve zaten var olan slug reddedilir", () => {
  assert.match(validateNewSlug("", []) ?? "", /boş olamaz/);
  assert.match(validateNewSlug("Ünlü", []) ?? "", /küçük harf/);
  assert.match(validateNewSlug("home", ["home", "x"]) ?? "", /zaten var/);
  assert.equal(validateNewSlug("projeler", ["home"]), undefined);
});

test("bodyOverflowOffset: sınır aşılmıyorsa undefined", () => {
  assert.equal(bodyOverflowOffset(serializePage({ title: "t", body: "kısa" })), undefined);
  // Sondaki boşluklar kırpılır — sınırı yalnızca onlar aşıyorsa taşma yok.
  assert.equal(bodyOverflowOffset(`# t\n\n${"a".repeat(10)}${" ".repeat(50)}`, 10), undefined);
});

test("bodyOverflowOffset: ham belgede sınırı aşan ilk karakteri gösterir", () => {
  const doc = `# t\n\n${"a".repeat(10)}TAŞAN`;
  assert.equal(bodyOverflowOffset(doc, 10), doc.indexOf("TAŞAN"));
});

test("bodyOverflowOffset: CRLF tek bayt sayılır, baştaki boşluk ve çok baytlı karakterler doğru hesaplanır", () => {
  // Gövde: "ab\r\nğc" → yayınlanan "ab\nğc" = 2 + 1 + 2 + 1 = 6 bayt.
  const doc = "# t\r\n\r\n   ab\r\nğc";
  assert.equal(bodyOverflowOffset(doc, 6), undefined);
  assert.equal(bodyOverflowOffset(doc, 5), doc.indexOf("c"));
  assert.equal(bodyOverflowOffset(doc, 4), doc.indexOf("ğ")); // "ğ" sınırı ortasından aşar
  assert.equal(bodyOverflowOffset(doc, 2), doc.indexOf("\r\nğ"));
});

test("shareCode/shareLink: Arama Motoru'nun 'Kodla Git' ve and-site:// biçimi", () => {
  const key = "a3f9".repeat(16);
  assert.equal(shareCode(key, "hakkimda"), `${key}/hakkimda`);
  assert.equal(shareLink(key, "hakkimda"), `and-site://${key}/hakkimda`);
});

test("slugFromPath/pathForSlug: gidiş-dönüş ve geçersiz yolların reddi", () => {
  assert.equal(pathForSlug("hakkimda"), "/hakkimda.andsite");
  assert.equal(slugFromPath(pathForSlug("proje-2026")), "proje-2026");
  for (const p of ["/", "/hakkimda", "/hakkimda.md", "/alt/hakkimda.andsite", "/Kotu.andsite", "hakkimda.andsite"]) {
    assert.equal(slugFromPath(p), undefined, p);
  }
});

test("publishDelayMs: ilk yayın beklemez, yakın tarihli yayın aradaki farkı bekler, bekleme üstten sınırlıdır", () => {
  const now = 1_000_000;
  assert.equal(publishDelayMs(undefined, now), 0);
  assert.equal(publishDelayMs(Number.NaN, now), 0);
  assert.equal(publishDelayMs(now - 2000, now), SITE_PUBLISH_SPACING_MS - 2000);
  assert.equal(publishDelayMs(now - SITE_PUBLISH_SPACING_MS - 1, now), 0);
  assert.equal(publishDelayMs(now + 60_000, now), SITE_PUBLISH_SPACING_MS); // ileri tarihli damga
});
