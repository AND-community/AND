import * as crypto from "crypto";
import * as fs from "fs";
import * as path from "path";
import {
  grupYolu,
  gruplarDizini,
  projeBirlesmeYolu,
  projeCalismaDizini,
  projeKokDizini,
  projeMetaYolu,
  projeNesneDizini,
  projelerDizini,
} from "./paths";
import { generateSecret } from "./live/deltax/crypto";
import { Grup, Proje } from "./types";

// store.go'daki "Üye / Grup" ve "Proje" bölümlerinin TypeScript karşılığı.
// Grup oluşturma/davet/roster güncelleme/kapatma AND'in canlı P2P bağlantısını
// gerektirir (bkz. live/deltax/protocol.ts, live/deltax/syncService.ts —
// sync.go'nun karşılığı) — buradaki yazma fonksiyonları yalnızca yerel JSON
// kaydını günceller, ağ tarafı ayrı katmandadır.

export function readGroup(root: string, id: string): Grup {
  const data = fs.readFileSync(grupYolu(root, id), "utf8");
  return JSON.parse(data) as Grup;
}

export function saveGroup(root: string, g: Grup): void {
  fs.mkdirSync(gruplarDizini(root), { recursive: true, mode: 0o700 });
  fs.writeFileSync(grupYolu(root, g.id), JSON.stringify(g, null, 2), { mode: 0o600 });
}

/** grupOlustur — yeni bir grup (kendisi tek üye, kurucu) oluşturur ve kaydeder. */
export function createGroup(root: string, ad: string, kendi: { ad: string; peer_id: string }): Grup {
  const g: Grup = {
    id: randomId(),
    ad,
    sir: generateSecret(),
    kurucu_ad: kendi.ad,
    kurucu_peer: kendi.peer_id,
    uyeler: [{ ad: kendi.ad, peer_id: kendi.peer_id }],
    olusturma_ts: new Date().toISOString(),
  };
  saveGroup(root, g);
  return g;
}

/** grupSil — grubun yerel kaydını siler; projelerin nesne/çalışma verisi kasıtlı olarak diskte bırakılır. */
export function deleteGroup(root: string, id: string): void {
  fs.rmSync(grupYolu(root, id), { force: true });
}

export function listGroups(root: string): Grup[] {
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(gruplarDizini(root), { withFileTypes: true });
  } catch {
    return [];
  }
  const out: Grup[] = [];
  for (const e of entries) {
    if (e.isDirectory() || path.extname(e.name) !== ".json") {
      continue;
    }
    const id = e.name.slice(0, -".json".length);
    try {
      out.push(readGroup(root, id));
    } catch {
      // bozuk/okunamayan grup dosyası — sessizce atla
    }
  }
  out.sort((a, b) => (a.olusturma_ts < b.olusturma_ts ? 1 : a.olusturma_ts > b.olusturma_ts ? -1 : 0));
  return out;
}

export function readProject(root: string, id: string): Proje {
  const data = fs.readFileSync(projeMetaYolu(root, id), "utf8");
  const p = JSON.parse(data) as Proje;
  if (!p.dallar) {
    p.dallar = {};
  }
  return p;
}

export function saveProject(root: string, p: Proje): void {
  fs.mkdirSync(projeKokDizini(root, p.id), { recursive: true, mode: 0o700 });
  fs.writeFileSync(projeMetaYolu(root, p.id), JSON.stringify(p, null, 2), { mode: 0o600 });
}

export function listProjects(root: string, grupId: string): Proje[] {
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(projelerDizini(root), { withFileTypes: true });
  } catch {
    return [];
  }
  const out: Proje[] = [];
  for (const e of entries) {
    if (!e.isDirectory()) {
      continue;
    }
    try {
      const p = readProject(root, e.name);
      if (p.grup_id === grupId) {
        out.push(p);
      }
    } catch {
      // bozuk/okunamayan proje meta.json — sessizce atla
    }
  }
  out.sort((a, b) => (a.olusturma_ts < b.olusturma_ts ? 1 : a.olusturma_ts > b.olusturma_ts ? -1 : 0));
  return out;
}

function randomId(): string {
  return crypto.randomBytes(8).toString("hex");
}

/** projeOlustur — mevcut bir grup içinde yeni, boş bir proje oluşturur (tamamen yerel işlem). */
export function createProject(root: string, grupId: string, ad: string): Proje {
  const p: Proje = {
    id: randomId(),
    grup_id: grupId,
    ad,
    olusturma_ts: new Date().toISOString(),
    dallar: { ana: "" },
    aktif_dal: "ana",
  };
  fs.mkdirSync(projeNesneDizini(root, p.id), { recursive: true, mode: 0o700 });
  fs.mkdirSync(projeCalismaDizini(root, p.id), { recursive: true, mode: 0o700 });
  saveProject(root, p);
  return p;
}

/**
 * Bir üyeden gelen ilk push'ta (bu düğümde daha önce hiç var olmayan bir
 * proje için) iskelet proje kaydını oluşturur — Go pushPaketiIsle'deki
 * "proje hiç yoksa iskeletini oluştur" bloğunun karşılığı. Proje zaten
 * varsa hiçbir şey değiştirmeden okuyup döndürür (createProject'ten farklı
 * olarak ID'yi çağıran belirler — göndericiyle aynı proje ID'si olmalı).
 */
export function ensureProjectSkeleton(root: string, projeId: string, grupId: string): Proje {
  try {
    return readProject(root, projeId);
  } catch {
    // yok — aşağıda oluşturulacak
  }
  const p: Proje = {
    id: projeId,
    grup_id: grupId,
    ad: "(alınan proje)",
    olusturma_ts: new Date().toISOString(),
    dallar: {},
    aktif_dal: "ana",
  };
  fs.mkdirSync(projeNesneDizini(root, p.id), { recursive: true, mode: 0o700 });
  fs.mkdirSync(projeCalismaDizini(root, p.id), { recursive: true, mode: 0o700 });
  saveProject(root, p);
  return p;
}

export function mergeHeadPending(root: string, projeId: string): string | undefined {
  try {
    const h = fs.readFileSync(projeBirlesmeYolu(root, projeId), "utf8").trim();
    return h || undefined;
  } catch {
    return undefined;
  }
}

export function markMergeHead(root: string, projeId: string, ebeveynHash: string): void {
  fs.writeFileSync(projeBirlesmeYolu(root, projeId), ebeveynHash, { mode: 0o600 });
}

export function clearMergeHead(root: string, projeId: string): void {
  fs.rmSync(projeBirlesmeYolu(root, projeId), { force: true });
}
