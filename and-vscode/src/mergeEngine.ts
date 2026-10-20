import * as fs from "fs";
import * as path from "path";
import { readObject } from "./objectStore";
import { treeMap } from "./treeStore";
import { AgacGirdi } from "./types";

// vcs.go'daki "Üçlü birleştirme" bölümünün TypeScript karşılığı.

interface MergeEntryResult {
  girdi?: AgacGirdi;
  varMi: boolean;
  catisma: boolean;
}

/** birlestirGirdi — tek bir dosya yolu için temel/yerel/uzak durumlarını karşılaştırır. */
function mergeEntry(
  base: AgacGirdi | undefined,
  local: AgacGirdi | undefined,
  remote: AgacGirdi | undefined
): MergeEntryResult {
  if (local && remote && local.hash === remote.hash) {
    return { girdi: local, varMi: true, catisma: false };
  }
  if (!local && !remote) {
    return { varMi: false, catisma: false };
  }
  if (local && !remote) {
    if (base && base.hash === local.hash) {
      return { varMi: false, catisma: false }; // uzakta silinmiş, yerel değişmemiş → sil
    }
    if (!base) {
      return { girdi: local, varMi: true, catisma: false }; // yalnız yerelde yeni
    }
    return { girdi: local, varMi: true, catisma: true }; // yerel değişti, uzak sildi → çakışma
  }
  if (!local && remote) {
    if (base && base.hash === remote.hash) {
      return { varMi: false, catisma: false };
    }
    if (!base) {
      return { girdi: remote, varMi: true, catisma: false };
    }
    return { girdi: remote, varMi: true, catisma: true };
  }
  // local && remote, hash'ler farklı
  if (base && local && base.hash === local!.hash) {
    return { girdi: remote, varMi: true, catisma: false }; // yalnız uzak değişti
  }
  if (base && remote && base.hash === remote!.hash) {
    return { girdi: local, varMi: true, catisma: false }; // yalnız yerel değişti
  }
  return { girdi: local, varMi: true, catisma: true }; // ikisi de değişti → çakışma
}

function unionPaths(...maps: Map<string, AgacGirdi>[]): string[] {
  const gorulen = new Set<string>();
  for (const m of maps) {
    for (const yol of m.keys()) {
      gorulen.add(yol);
    }
  }
  return [...gorulen].sort();
}

export interface MergeApplyResult {
  catismalar: string[];
}

/**
 * birlestirmeUygula — temel/yerel/uzak ağaçları üçlü birleştirir ve sonucu
 * doğrudan çalışma dizinine yazar. Çakışan dosyalara <<<<<<< / ======= /
 * >>>>>>> işaretleri eklenir; kullanıcı bunları elle çözüp tekrar kaydetmelidir.
 */
export function applyThreeWayMerge(
  root: string,
  projeId: string,
  calismaDir: string,
  temelTree: string,
  yerelTree: string,
  uzakTree: string
): MergeApplyResult {
  const temel = treeMap(root, projeId, temelTree);
  const yerel = treeMap(root, projeId, yerelTree);
  const uzak = treeMap(root, projeId, uzakTree);

  const catismalar: string[] = [];

  for (const yol of unionPaths(temel, yerel, uzak)) {
    const b = temel.get(yol);
    const l = yerel.get(yol);
    const r = uzak.get(yol);
    const { girdi, varMi, catisma } = mergeEntry(b, l, r);

    const full = path.join(calismaDir, ...yol.split("/"));
    if (!varMi) {
      fs.rmSync(full, { force: true });
      continue;
    }
    if (catisma) {
      catismalar.push(yol);
      let lData: Buffer = Buffer.alloc(0);
      let rData: Buffer = Buffer.alloc(0);
      if (l) {
        lData = readObject(root, projeId, l.hash).payload;
      }
      if (r) {
        rData = readObject(root, projeId, r.hash).payload;
      }
      const icerik = `<<<<<<< YEREL\n${lData.toString("utf8")}\n=======\n${rData.toString("utf8")}\n>>>>>>> UZAK\n`;
      fs.mkdirSync(path.dirname(full), { recursive: true, mode: 0o700 });
      fs.writeFileSync(full, icerik, { mode: 0o600 });
      continue;
    }
    if (girdi) {
      const { payload } = readObject(root, projeId, girdi.hash);
      fs.mkdirSync(path.dirname(full), { recursive: true, mode: 0o700 });
      fs.writeFileSync(full, payload, { mode: 0o600 });
    }
  }

  return { catismalar };
}
