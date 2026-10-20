import * as fs from "fs";
import * as path from "path";

// ignore.go'daki .projeagignore mantığının karşılığı:
//   - "/" içermeyen desenler ("node_modules", "*.log") yolun HERHANGİ bir
//     segmentiyle eşleşirse o yolu (ve bir dizinse altındakileri) yok sayar.
//   - "/" içeren desenler proje köküne göre tam yol olarak eşleştirilir.
// Desen eşleştirme kuralı Go'nun filepath.Match'iyle aynıdır: '*', '?' ve
// '[...]' karakter sınıfları desteklenir (segment içinde, '/' bir joker
// tarafından asla yutulmaz).

const IGNORE_FILE = ".projeagignore";

export function readIgnorePatterns(calismaDir: string): string[] {
  let data: string;
  try {
    data = fs.readFileSync(path.join(calismaDir, IGNORE_FILE), "utf8");
  } catch {
    return [];
  }
  const desenler: string[] = [];
  for (const rawLine of data.split("\n")) {
    const satir = rawLine.trim();
    if (satir === "" || satir.startsWith("#")) {
      continue;
    }
    desenler.push(satir);
  }
  return desenler;
}

export function isIgnored(desenler: string[], rel: string): boolean {
  if (desenler.length === 0) {
    return false;
  }
  const parcalar = rel.split("/");
  for (const ham of desenler) {
    const desen = ham.endsWith("/") ? ham.slice(0, -1) : ham;
    if (desen === "") {
      continue;
    }
    if (desen.includes("/")) {
      if (globMatch(desen, rel)) {
        return true;
      }
      continue;
    }
    for (const parca of parcalar) {
      if (globMatch(desen, parca)) {
        return true;
      }
    }
  }
  return false;
}

/**
 * Go'nun path/filepath.Match fonksiyonuna denk, tek bir yol segmenti (ya da
 * '/' içeren desenler için tam göreli yol) üzerinde çalışan sınırlı bir
 * glob eşleştirici. Desteklenenler: '*' (sıfır+ karakter), '?' (tek
 * karakter), '[abc]' / '[a-z]' / '[^abc]' karakter sınıfları.
 *
 * Go'nun kendi belgelediği gibi: '*' ve '?' hiçbir zaman bir '/' ayracını
 * TÜKETMEZ (eşleşmez) — yalnızca ayraç içermeyen tek bir segment içinde
 * genişler. Bu, '/' içeren desenlerin (ör. "build/*.log") alt dizinlere
 * sızmadan yalnızca doğrudan o dizindeki dosyalarla eşleşmesini sağlar;
 * atlanırsa DeltaX TUI'siyle (Go) farklı dosyalar yok sayılır/izlenir.
 */
export function globMatch(pattern: string, value: string): boolean {
  const memo = new Map<string, boolean>();

  function match(pi: number, vi: number): boolean {
    const key = `${pi}:${vi}`;
    const cached = memo.get(key);
    if (cached !== undefined) {
      return cached;
    }
    const result = matchUncached(pi, vi);
    memo.set(key, result);
    return result;
  }

  function matchUncached(pi: number, vi: number): boolean {
    if (pi === pattern.length) {
      return vi === value.length;
    }
    const pc = pattern[pi];
    if (pc === "*") {
      for (let skip = 0; vi + skip <= value.length; skip++) {
        if (match(pi + 1, vi + skip)) {
          return true;
        }
        if (value[vi + skip] === "/") {
          break; // '*' bir '/' yutamaz (Go'nun filepath.Match'iyle aynı)
        }
      }
      return false;
    }
    if (vi === value.length) {
      return false;
    }
    if (pc === "?") {
      return value[vi] !== "/" && match(pi + 1, vi + 1);
    }
    if (pc === "[") {
      const close = pattern.indexOf("]", pi + 1);
      if (close === -1) {
        return pc === value[vi] && match(pi + 1, vi + 1);
      }
      if (value[vi] === "/") {
        return false; // karakter sınıfı bir '/' ayracıyla asla eşleşmez
      }
      let cls = pattern.slice(pi + 1, close);
      let negate = false;
      if (cls.startsWith("^")) {
        negate = true;
        cls = cls.slice(1);
      }
      const inClass = classContains(cls, value[vi]);
      if (inClass === negate) {
        return false;
      }
      return match(close + 1, vi + 1);
    }
    return pc === value[vi] && match(pi + 1, vi + 1);
  }

  return match(0, 0);
}

function classContains(cls: string, ch: string): boolean {
  for (let i = 0; i < cls.length; i++) {
    if (cls[i + 1] === "-" && i + 2 < cls.length) {
      if (ch >= cls[i] && ch <= cls[i + 2]) {
        return true;
      }
      i += 2;
      continue;
    }
    if (cls[i] === ch) {
      return true;
    }
  }
  return false;
}
