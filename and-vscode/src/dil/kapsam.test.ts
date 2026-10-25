// Çeviri kapsamı: kaynak kodu TypeScript'in sözdizimi ağacıyla tarar ve
// AND'daki dil/diltest denetimlerinin aynısını yapar — t()/n()/k() ile
// kullanılan her metnin her dilde çevirisi olmalı, biçim fiilleri
// korunmalı, katalogda kullanılmayan metin kalmamalı ve çevrilmeden
// bırakılmış Türkçe metin bulunmamalı.
//
// DIL_ISKELET=<dosya> verilirse eksik metinler çevirisi boş bir katalog
// iskeleti olarak o dosyaya yazılır.

import * as assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { test } from "node:test";
import * as ts from "typescript";
import { DILLER, KAYNAK, Katalog, gomulu } from "./dil";

const KOK = path.resolve(__dirname, "..", "..");
const SRC = path.join(KOK, "src");
const CEVIRI_DILLERI = DILLER.map((d) => d.kod).filter((k) => k !== KAYNAK);
const TURKCE_HARF = /[çğıöşüÇĞİÖŞÜ]/;

/**
 * Çevrilmeden kalabilen Türkçe metinler: ağ protokolünün, AND'ın dosya
 * biçimlerinin ya da kullanıcı verisinin parçası olanlar (değiştirilirlerse
 * AND ve Go eklentileriyle uyum bozulur), dil adları ve kod içi tanımlayıcılar.
 */
const CEVRILMEZ = new Set<string>([
  // Go DeltaX'la aynı: alınan bir projenin meta.json'a yazılan geçici adı (sync.go)
  "(alınan proje)",
  // dil adları kendi dillerinde yazılır
  "Türkçe",
  "Español",
  "Français",
]);

interface Kullanim {
  metin: string;
  yer: string;
}

interface Tarama {
  anahtarlar: Kullanim[];
  kalintilar: Kullanim[];
  duzMetinler: Kullanim[];
}

function kaynakDosyalari(dizin: string): string[] {
  const sonuc: string[] = [];
  for (const g of fs.readdirSync(dizin, { withFileTypes: true })) {
    const yol = path.join(dizin, g.name);
    if (g.isDirectory()) {
      sonuc.push(...kaynakDosyalari(yol));
    } else if (g.name.endsWith(".ts") && !g.name.endsWith(".test.ts") && !g.name.endsWith(".d.ts")) {
      sonuc.push(yol);
    }
  }
  return sonuc.sort();
}

/**
 * Çağrı bir çeviri çağrısıysa anahtarın argüman sırası: t/k/ham → 0, n → 1.
 * h ve j, panelHtml.ts'teki t sarmalayıcılarıdır (HTML'e ve betiğe yazılan çeviri).
 */
function anahtarSirasi(cagri: ts.CallExpression): number {
  const c = cagri.expression;
  let ad: string | undefined;
  if (ts.isIdentifier(c)) {
    ad = c.text;
  } else if (ts.isPropertyAccessExpression(c) && ts.isIdentifier(c.expression) && c.expression.text === "dil") {
    ad = c.name.text;
  }
  switch (ad) {
    case "t":
    case "k":
    case "ham":
    case "h":
    case "j":
      return 0;
    case "n":
      return 1;
  }
  return -1;
}

function metinMi(d: ts.Node): d is ts.StringLiteral | ts.NoSubstitutionTemplateLiteral {
  return ts.isStringLiteral(d) || ts.isNoSubstitutionTemplateLiteral(d);
}

function tara(): Tarama {
  const tarama: Tarama = { anahtarlar: [], kalintilar: [], duzMetinler: [] };
  for (const dosya of kaynakDosyalari(SRC)) {
    const kod = fs.readFileSync(dosya, "utf8");
    const kaynak = ts.createSourceFile(dosya, kod, ts.ScriptTarget.ES2022, true);
    const goreli = path.relative(KOK, dosya).replace(/\\/g, "/");
    const yer = (d: ts.Node): string => `${goreli}:${kaynak.getLineAndCharacterOfPosition(d.getStart()).line + 1}`;
    const anahtarDugumleri = new Set<ts.Node>();

    const gez = (d: ts.Node): void => {
      if (ts.isImportDeclaration(d) || ts.isExportDeclaration(d)) {
        return;
      }
      if (ts.isCallExpression(d)) {
        const sira = anahtarSirasi(d);
        const arg = sira >= 0 ? d.arguments[sira] : undefined;
        if (arg && metinMi(arg)) {
          anahtarDugumleri.add(arg);
          tarama.anahtarlar.push({ metin: arg.text, yer: yer(arg) });
        }
      }
      if (metinMi(d) && !anahtarDugumleri.has(d)) {
        if (TURKCE_HARF.test(d.text) && !CEVRILMEZ.has(d.text)) {
          tarama.kalintilar.push({ metin: d.text, yer: yer(d) });
        } else if (ts.isStringLiteral(d)) {
          tarama.duzMetinler.push({ metin: d.text, yer: yer(d) });
        }
      } else if (ts.isTemplateExpression(d)) {
        // Webview betiklerindeki yorumlar (şablonun içinde kalan // ve /* */) gösterilmez.
        const yorumsuz = (p: string): string => p.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|\s)\/\/.*$/gm, "$1");
        const parcalar = [d.head.text, ...d.templateSpans.map((s) => s.literal.text)].map(yorumsuz);
        for (const p of parcalar) {
          if (TURKCE_HARF.test(p)) {
            tarama.kalintilar.push({ metin: p.trim().slice(0, 120), yer: yer(d) });
          }
        }
      }
      ts.forEachChild(d, gez);
    };
    gez(kaynak);
  }
  return tarama;
}

/** Bir metindeki biçim fiilleri, argüman sırasına göre: "%s %d" → ["1s", "2d"]. */
function fiiller(metin: string): string[] {
  const sonuc: string[] = [];
  let sira = 0;
  for (const m of metin.matchAll(/%(?:\[(\d+)\])?([sd%])/g)) {
    if (m[2] === "%") {
      continue;
    }
    sonuc.push(`${m[1] ? Number(m[1]) : ++sira}${m[2]}`);
  }
  return sonuc.sort();
}

const tarama = tara();
const katalog: Katalog = gomulu();

test("kaynakta çevrilmeden kalmış Türkçe metin yok", () => {
  const liste = tarama.kalintilar.map((k) => `${k.yer}: ${JSON.stringify(k.metin)}`);
  assert.deepEqual(liste, [], `t() ile sarılmamış ${liste.length} Türkçe metin:\n${liste.join("\n")}`);
});

test("katalogdaki kaynak metinler çeviri çağrısı dışında düz metin olarak kullanılmıyor", () => {
  const liste = tarama.duzMetinler
    .filter((d) => Object.hasOwn(katalog, d.metin) && !CEVRILMEZ.has(d.metin))
    .map((d) => `${d.yer}: ${JSON.stringify(d.metin)}`);
  assert.deepEqual(liste, [], `çeviri çağrısı dışında kalan katalog metinleri:\n${liste.join("\n")}`);
});

test("kullanılan her metnin her dilde çevirisi var", () => {
  const eksik = new Map<string, string>();
  for (const a of tarama.anahtarlar) {
    const c = katalog[a.metin];
    const dilleri = CEVIRI_DILLERI.filter((d) => !c?.[d]);
    if (dilleri.length && !eksik.has(a.metin)) {
      eksik.set(a.metin, `${a.yer}: ${JSON.stringify(a.metin)} (${dilleri.join(", ")})`);
    }
  }
  const iskelet = process.env.DIL_ISKELET;
  if (iskelet && eksik.size) {
    const bos: Katalog = {};
    for (const m of eksik.keys()) {
      bos[m] = Object.fromEntries(CEVIRI_DILLERI.map((d) => [d, katalog[m]?.[d] ?? ""]));
    }
    fs.writeFileSync(iskelet, JSON.stringify(bos, null, 2) + "\n");
  }
  assert.deepEqual([...eksik.values()], [], `çevirisi eksik ${eksik.size} metin`);
});

test("katalogda kullanılmayan metin yok", () => {
  const kullanilan = new Set(tarama.anahtarlar.map((a) => a.metin));
  const artik = Object.keys(katalog).filter((m) => !kullanilan.has(m));
  assert.deepEqual(artik, [], `hiçbir yerde kullanılmayan ${artik.length} metin`);
});

test("çeviriler biçim fiillerini ve çoğul kurallarını koruyor", () => {
  const hatalar: string[] = [];
  for (const [kaynak, diller] of Object.entries(katalog)) {
    const beklenen = fiiller(kaynak).join(" ");
    for (const [kod, ceviri] of Object.entries(diller)) {
      if (!CEVIRI_DILLERI.includes(kod)) {
        hatalar.push(`${JSON.stringify(kaynak)}: bilinmeyen dil ${kod}`);
        continue;
      }
      const bicimler = ceviri.split("|");
      const enFazla = kod === "ru" ? 3 : 2;
      if (bicimler.length > enFazla) {
        hatalar.push(`${JSON.stringify(kaynak)} [${kod}]: ${bicimler.length} çoğul biçimi; en fazla ${enFazla}`);
      }
      for (const b of bicimler) {
        if (fiiller(b).join(" ") !== beklenen) {
          hatalar.push(`${JSON.stringify(kaynak)} [${kod}]: fiiller ${JSON.stringify(b)} içinde ${beklenen || "yok"} olmalı`);
        }
      }
      if (ceviri.trim() !== ceviri && kaynak.trim() === kaynak) {
        hatalar.push(`${JSON.stringify(kaynak)} [${kod}]: baştaki/sondaki boşluk kaynakta yok`);
      }
      if (TURKCE_HARF.test(kaynak) && ceviri === kaynak) {
        hatalar.push(`${JSON.stringify(kaynak)} [${kod}]: çeviri kaynakla aynı`);
      }
    }
  }
  assert.deepEqual(hatalar, []);
});
