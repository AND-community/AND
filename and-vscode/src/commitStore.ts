import { readObject, writeObject } from "./objectStore";
import { Commit, LogGirdisi } from "./types";

export function readCommit(root: string, projeId: string, hash: string): Commit {
  const { tip, payload } = readObject(root, projeId, hash);
  if (tip !== "C".charCodeAt(0)) {
    throw new Error("beklenmeyen nesne tipi: commit bekleniyordu");
  }
  return JSON.parse(payload.toString("utf8")) as Commit;
}

export function createCommit(
  root: string,
  projeId: string,
  agacHash: string,
  ebeveynler: string[],
  yazar: string,
  mesaj: string
): string {
  const c: Commit = {
    agac: agacHash,
    ...(ebeveynler.length > 0 ? { ebeveynler } : {}),
    yazar,
    mesaj,
    ts: new Date().toISOString(),
  };
  const payload = Buffer.from(JSON.stringify(c), "utf8");
  return writeObject(root, projeId, "C", payload);
}

export function headTreeOf(root: string, projeId: string, head: string): string {
  if (!head) {
    return "";
  }
  try {
    return readCommit(root, projeId, head).agac;
  } catch {
    return "";
  }
}

/** gunlukAl — ilk-ebeveyn zincirini izleyerek commit geçmişini döndürür. */
export function readLog(root: string, projeId: string, head: string, limit: number): LogGirdisi[] {
  const out: LogGirdisi[] = [];
  const gorulen = new Set<string>();
  let h = head;
  while (h && out.length < limit && !gorulen.has(h)) {
    gorulen.add(h);
    let c: Commit;
    try {
      c = readCommit(root, projeId, h);
    } catch {
      break;
    }
    out.push({ hash: h, commit: c });
    if (!c.ebeveynler || c.ebeveynler.length === 0) {
      break;
    }
    h = c.ebeveynler[0];
  }
  return out;
}

export function filterLog(girdiler: LogGirdisi[], sorgu: string): LogGirdisi[] {
  const q = sorgu.trim().toLowerCase();
  if (!q) {
    return girdiler;
  }
  return girdiler.filter(
    (g) =>
      g.commit.mesaj.toLowerCase().includes(q) ||
      g.commit.yazar.toLowerCase().includes(q) ||
      g.hash.toLowerCase().includes(q)
  );
}

/** atalariTopla — bir commit'ten (kendisi dahil) ilk-ebeveyn zincirinin ötesinde tüm ebeveyn commit hash'lerini toplar. */
export function collectAncestors(root: string, projeId: string, baslangic: string): Set<string> {
  const gorulen = new Set<string>();
  if (!baslangic) {
    return gorulen;
  }
  const kuyruk = [baslangic];
  while (kuyruk.length > 0) {
    const h = kuyruk.shift()!;
    if (!h || gorulen.has(h)) {
      continue;
    }
    gorulen.add(h);
    try {
      const c = readCommit(root, projeId, h);
      kuyruk.push(...(c.ebeveynler ?? []));
    } catch {
      continue;
    }
  }
  return gorulen;
}

/** ortakAta — iki commit'in en yakın ortak atasını (birleştirme tabanı) bulur. */
export function commonAncestor(root: string, projeId: string, a: string, b: string): string {
  if (!a || !b) {
    return "";
  }
  if (a === b) {
    return a;
  }
  const atalarA = collectAncestors(root, projeId, a);
  const gorulen = new Set<string>();
  const kuyruk = [b];
  while (kuyruk.length > 0) {
    const h = kuyruk.shift()!;
    if (!h || gorulen.has(h)) {
      continue;
    }
    gorulen.add(h);
    if (atalarA.has(h)) {
      return h;
    }
    try {
      const c = readCommit(root, projeId, h);
      kuyruk.push(...(c.ebeveynler ?? []));
    } catch {
      continue;
    }
  }
  return "";
}

export function shortHash(h: string): string {
  return h.length > 10 ? h.slice(0, 10) : h;
}
