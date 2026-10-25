// Uzantının arayüz dilleri — AND'ın Go "dil" paketinin (AND/dil) TypeScript
// karşılığı. Kurallar aynıdır: kaynak metinler Türkçedir ve aynı zamanda
// anahtardır; çevirisi olmayan metin kaynak haliyle görünür; tekil/çoğul
// biçimler çeviride "|" ile ayrılır ve sayıya göre seçilir. "vscode" modülüne
// BAĞIMLI DEĞİLDİR: panelin HTML'i ve testler bu modülü doğrudan kullanır.
//
// Komut başlıkları ve ayar açıklamaları ise package.nls.*.json dosyalarından
// gelir ve yalnızca VS Code'un görüntüleme dilini izleyebilir (VS Code kısıtı).

import * as fs from "fs";
import * as path from "path";
import ceviriler from "./ceviriler.json";
import kategoriler from "./kategoriler.json";

/** Kaynak metin → dil kodu → çeviri. AND'ın ceviriler/*.json dosyalarıyla aynı biçim. */
export type Katalog = Record<string, Record<string, string>>;
export type Arguman = string | number;

/** Metinlerin yazıldığı dil; çevirisi aranmaz. */
export const KAYNAK = "tr";

/** Desteklenen diller; ilki kaynak dildir. AND'daki dil.Diller ile aynı sıra. */
export const DILLER: readonly { kod: string; ad: string }[] = [
  { kod: "tr", ad: "Türkçe" },
  { kod: "en", ad: "English" },
  { kod: "de", ad: "Deutsch" },
  { kod: "es", ad: "Español" },
  { kod: "fr", ad: "Français" },
  { kod: "ru", ad: "Русский" },
];

/** AND'ın başlattığı süreçlere etkin dili ileten ortam değişkeni (Go: dil.OrtamDegiskeni). */
export const ORTAM_DEGISKENI = "AND_DIL";

let etkinKod = KAYNAK;
const katalog: Katalog = {};
ekle(ceviriler as Katalog);
ekle(kategoriler as Katalog);

/** Kod desteklenen bir dil mi? */
export function gecerli(kod: string): boolean {
  return DILLER.some((d) => d.kod === kod);
}

/** "pt-BR", " EN " gibi bir kodu desteklenen bir dile indirger; yoksa boş. */
export function dilKodu(ham: string | undefined): string {
  const kod = (ham ?? "").trim().toLowerCase().split(/[-_]/)[0];
  return gecerli(kod) ? kod : "";
}

/** Etkin dili değiştirir; bilinmeyen bir kodda değişiklik yapmaz ve false döner. */
export function ayarla(kod: string): boolean {
  const k = dilKodu(kod);
  if (!k) {
    return false;
  }
  etkinKod = k;
  return true;
}

/** Etkin dilin kodu. */
export function etkin(): string {
  return etkinKod;
}

/** Çeviri dosyası ekler; aynı kaynak metnin önceki çevirilerinin üzerine yazar. */
export function ekle(k: Katalog): void {
  for (const [kaynak, diller] of Object.entries(k)) {
    katalog[kaynak] = { ...katalog[kaynak], ...diller };
  }
}

/** Uzantının kendi çeviri dosyaları (testler için). kategoriler.json, AND'ın dil/ceviriler/kategoriler.json dosyasının kopyasıdır. */
export function gomulu(): Katalog {
  return { ...(ceviriler as Katalog), ...(kategoriler as Katalog) };
}

function cevir(kaynak: string): string {
  if (etkinKod === KAYNAK) {
    return kaynak;
  }
  const c = katalog[kaynak]?.[etkinKod];
  return c ? c : kaynak;
}

/**
 * Türkçe kaynak metnin etkin dildeki karşılığı. Argüman verilirse metin
 * biçimlendirilir (%s, %d, %[2]s, %%); çevirideki tekil/çoğul biçimlerinden
 * ilk sayı argümanına uyanı seçilir (Go: dil.T + dil.F).
 */
export function t(kaynak: string, ...args: Arguman[]): string {
  let c = cevir(kaynak);
  if (c.includes("|")) {
    const sayi = args.find((a): a is number => typeof a === "number");
    c = cogulSec(c, sayi ?? 0);
  }
  return args.length ? bicimle(c, args) : c;
}

/**
 * Türkçe kaynak metnin etkin dildeki karşılığı, biçimlendirilmeden ve tekil/çoğul
 * biçimleri "|" ile ayrılmış haliyle (Go: dil.T). Biçimi sonradan seçen yerler —
 * ör. webview betikleri — içindir.
 */
export function ham(kaynak: string): string {
  return cevir(kaynak);
}

/** Sayıya göre tekil/çoğul biçimi seçer; argüman yoksa sayının kendisi yazılır (Go: dil.N). */
export function n(sayi: number, kaynak: string, ...args: Arguman[]): string {
  return bicimle(cogulSec(cevir(kaynak), sayi), args.length ? args : [sayi]);
}

/** Metni olduğu gibi döndürür; "bu metin çevrilir" işaretidir (Go: dil.K). Gösterilirken t() ile çevrilir. */
export function k(kaynak: string): string {
  return kaynak;
}

function cogulSec(metin: string, sayi: number): string {
  const bicimler = metin.split("|");
  return bicimler[Math.min(cogulBicimi(etkinKod, sayi), bicimler.length - 1)];
}

/**
 * Dilin sayı için kullandığı biçimin sırası: 0 tekil, 1 çoğul; Rusçada
 * 0 (1, 21…), 1 (2–4, 22–24…), 2 (5–20, 0…). Go'daki cogulBicimi'nin aynısıdır.
 * Panelin betiğine de kaynak koduyla gömülür, bu yüzden dışarıdaki hiçbir
 * şeye başvurmaz.
 */
export function cogulBicimi(kod: string, sayi: number): number {
  const m = Math.abs(Math.trunc(sayi));
  if (kod === "tr") {
    return 0;
  }
  if (kod === "fr") {
    return m <= 1 ? 0 : 1;
  }
  if (kod === "ru") {
    if (m % 10 === 1 && m % 100 !== 11) {
      return 0;
    }
    if (m % 10 >= 2 && m % 10 <= 4 && (m % 100 < 12 || m % 100 > 14)) {
      return 1;
    }
    return 2;
  }
  return m === 1 ? 0 : 1;
}

/**
 * Biçim fiillerini argümanlarla doldurur: %s ve %d sırayla, %[n]s / %[n]d
 * n'inci argümanla (çeviride sözcük sırası değişebilsin diye), %% yüzde
 * işaretiyle. Panelin betiğine de kaynak koduyla gömülür.
 */
export function bicimle(kalip: string, args: readonly Arguman[]): string {
  let sira = 0;
  return kalip.replace(/%(?:\[(\d+)\])?([sd%])/g, (tum: string, no: string | undefined, fiil: string) => {
    if (fiil === "%") {
      return "%";
    }
    const i = no ? Number(no) - 1 : sira++;
    if (i < 0 || i >= args.length) {
      return tum;
    }
    const a = args[i];
    return fiil === "d" && typeof a === "number" ? String(Math.trunc(a)) : String(a);
  });
}

/** Sayıyı etkin dilin binlik ayırıcısıyla yazar. */
export function binlik(sayi: number): string {
  return sayi.toLocaleString(etkinKod);
}

/** AND'ın veri dizininde sakladığı dil seçimi (dil.json); yoksa ya da geçersizse boş. */
export function andDosyaDili(dataDir: string): string {
  try {
    const veri = JSON.parse(fs.readFileSync(dilDosyasi(dataDir), "utf8")) as { dil?: unknown };
    return typeof veri.dil === "string" ? dilKodu(veri.dil) : "";
  } catch {
    return "";
  }
}

/** AND'ın dil seçimini tuttuğu dosya. */
export function dilDosyasi(dataDir: string): string {
  return path.join(dataDir, "dil.json");
}

/** Uzantının dil ayarının (and.language) "kendiliğinden" değeri. */
export const KENDILIGINDEN = "auto";

/**
 * Uzantının kullanacağı dil. Öncelik: ayar ("auto" değilse) > AND_DIL >
 * AND'ın dil.json'u > VS Code'un görüntüleme dili > İngilizce (VS Code'un
 * desteklenmeyen bir dilinde komut başlıkları da İngilizce görünür).
 */
export function dilSec(secenekler: { ayar?: string; ortam?: string; andDosyasi?: string; vscode?: string }): string {
  if (secenekler.ayar && secenekler.ayar !== KENDILIGINDEN) {
    const k = dilKodu(secenekler.ayar);
    if (k) {
      return k;
    }
  }
  return dilKodu(secenekler.ortam) || dilKodu(secenekler.andDosyasi) || dilKodu(secenekler.vscode) || "en";
}

/**
 * Değeri (metin, sayı, nesne) bir webview betiğine gömülecek JavaScript
 * değişmezine çevirir. "<" kaçırılır: hiçbir değer betiği "</script>" ile
 * kapatamaz.
 */
export function betikDegeri(deger: unknown): string {
  return JSON.stringify(deger).replace(/</g, "\\u003c");
}
