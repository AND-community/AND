import { test } from "node:test";
import assert from "node:assert/strict";
import { ansi256ToHex, ansiOrHexToCss, readableTextOn } from "./themeColor";

// AND'ın tema renklerinin (xterm-256 / hex) CSS'e çevrilmesi ve vurgu
// renginde düğmeler için okunur yazı renginin seçimi.

test("ansi256ToHex: küp ve gri rampa doğru çevrilir", () => {
  assert.equal(ansi256ToHex(80), "#5fd7d7"); // AND'ın varsayılan Turkuaz vurgusu
  assert.equal(ansi256ToHex(23), "#005f5f"); // varsayılan seçim zemini
  assert.equal(ansi256ToHex(232), "#080808");
  assert.equal(ansi256ToHex(255), "#eeeeee");
  assert.equal(ansi256ToHex(300), "#888888");
});

test("ansiOrHexToCss: kod noktası, hex ve geçersiz değer", () => {
  assert.equal(ansiOrHexToCss("205"), "#ff5faf");
  assert.equal(ansiOrHexToCss("#ff6600"), "#ff6600");
  assert.equal(ansiOrHexToCss("mor"), undefined);
  assert.equal(ansiOrHexToCss(undefined), undefined);
});

test("readableTextOn: açık zeminde siyah, koyu zeminde beyaz yazı", () => {
  assert.equal(readableTextOn("#5fd7d7"), "#000000"); // Turkuaz vurgu
  assert.equal(readableTextOn("#ff5faf"), "#000000"); // Pembiş vurgu
  assert.equal(readableTextOn("#005f5f"), "#ffffff"); // Turkuaz seçim zemini
  assert.equal(readableTextOn("#5f00af"), "#ffffff"); // Gece Moru seçim zemini
  assert.equal(readableTextOn("geçersiz"), undefined);
  assert.equal(readableTextOn(undefined), undefined);
});
