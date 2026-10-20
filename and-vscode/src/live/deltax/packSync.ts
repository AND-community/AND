import * as crypto from "crypto";
import * as fs from "fs";
import * as path from "path";
import { ensureProjectSkeleton, readGroup } from "../../groupProjectStore";
import { objectPath, readObject, writeVerifiedRawObject } from "../../objectStore";
import { projeNesneDizini } from "../../paths";
import { Agac, Commit } from "../../types";
import { decrypt } from "./crypto";
import { isMember } from "./groupOps";
import { buildTarGz, parseTarGz, TarEntry } from "./tarball";

// sync.go'nun "Paket oluşturma/uygulama" ve "Gönder (push)"/"Gelen paket
// işleme" bölümlerinin (paketOlustur/paketUygula/paketDosyaAdi/paketAyristir/
// paketGonderenGuvenilirMi/pushPaketiIsle) TypeScript karşılığı.

const HEX64 = /^[0-9a-f]{64}$/;
const MANIFEST_ENTRY = "MANIFEST";
// vcs.go'daki maxDosyaBoyutu (200 MB) ile aynı sınır — daha küçük, bağımsız
// bir sabit kullanmak (eskiden 64 MB) Go tarafında anlık görüntüye dahil
// edilen ama bu sınırı aşan bir dosyanın burada sessizce atlanmasına
// (dangling blob referansı bırakarak) yol açardı.
const MAX_OBJECT_BYTES = 200 * 1024 * 1024;
// sync.go'daki reRastgeleID ile aynı biçim — rastgeleID()'nin ürettiği 16
// küçük harf hex karakteri. Bu kontrol olmadan, gönderenin seçtiği dosya
// adı doğrudan disk yollarına (path.join) eklenir.
const RANDOM_ID = /^[0-9a-f]{16}$/;

export interface PackManifest {
  dal: string;
  head: string;
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// ─── Paket dosya adı ────────────────────────────────────────────────────────

export function packFileName(grupId: string, projeId: string): string {
  const unique = `${Date.now()}${crypto.randomBytes(4).toString("hex")}`;
  return `projeag-${grupId}-${projeId}-${unique}.pack`;
}

export function parsePackFileName(fileName: string): { grupId: string; projeId: string } | undefined {
  const base = path.basename(fileName).replace(/\.pack$/, "");
  const parts = base.split("-");
  if (parts.length !== 4 || parts[0] !== "projeag") {
    return undefined;
  }
  if (!RANDOM_ID.test(parts[1]) || !RANDOM_ID.test(parts[2])) {
    return undefined;
  }
  return { grupId: parts[1], projeId: parts[2] };
}

// ─── Paket oluşturma ────────────────────────────────────────────────────────

/**
 * paketOlustur — kokCommit'ten ulaşılabilen ve knownObjects kümesinde
 * olmayan tüm commit/ağaç/blob nesnelerini toplayıp gzip'li bir tar arşivi
 * döndürür. Go ile birebir aynı davranış için knownObjects yalnızca (pushYap
 * gibi) atalariTopla/collectAncestors'tan gelen ATA COMMIT hash'lerini
 * içermelidir — ağaç/blob seviyesinde "zaten bilinen" optimizasyonu Go
 * tarafında da yoktur.
 */
export function buildPack(
  root: string,
  projeId: string,
  dal: string,
  kokCommit: string,
  knownObjects: ReadonlySet<string>
): { data: Buffer; objectCount: number } {
  const collected = new Set<string>();
  const ordered: string[] = [];
  const queue: string[] = [kokCommit];
  while (queue.length > 0) {
    const h = queue.shift()!;
    if (!h || collected.has(h) || knownObjects.has(h)) {
      continue;
    }
    collected.add(h);
    ordered.push(h);
    let raw;
    try {
      raw = readObject(root, projeId, h);
    } catch {
      continue;
    }
    if (raw.tip === "C".charCodeAt(0)) {
      try {
        const c = JSON.parse(raw.payload.toString("utf8")) as Commit;
        queue.push(c.agac, ...(c.ebeveynler ?? []));
      } catch {
        // yok say
      }
    } else if (raw.tip === "T".charCodeAt(0)) {
      try {
        const t = JSON.parse(raw.payload.toString("utf8")) as Agac;
        for (const girdi of t.girdiler ?? []) {
          queue.push(girdi.hash);
        }
      } catch {
        // yok say
      }
    }
  }

  const entries: TarEntry[] = [
    { name: MANIFEST_ENTRY, data: Buffer.from(JSON.stringify({ dal, head: kokCommit } as PackManifest), "utf8") },
  ];
  for (const h of ordered) {
    try {
      entries.push({ name: h, data: fs.readFileSync(objectPath(root, projeId, h)) });
    } catch {
      continue;
    }
  }
  return { data: buildTarGz(entries), objectCount: ordered.length };
}

// ─── Paket uygulama ─────────────────────────────────────────────────────────

/**
 * paketUygula — şifresi zaten çözülmüş bir paketin içindeki nesneleri
 * doğrulayıp (her girdinin sha256'sı dosya adıyla eşleşmeli) yerel nesne
 * deposuna yazar. Bütünlük ihlali olan girdiler sessizce atlanır.
 */
export function applyPack(root: string, projeId: string, plain: Buffer): { manifest: PackManifest; objectCount: number } {
  fs.mkdirSync(projeNesneDizini(root, projeId), { recursive: true, mode: 0o700 });
  let manifest: PackManifest = { dal: "", head: "" };
  let objectCount = 0;
  for (const e of parseTarGz(plain)) {
    if (e.name === MANIFEST_ENTRY) {
      try {
        manifest = JSON.parse(e.data.toString("utf8")) as PackManifest;
      } catch {
        // yok say
      }
      continue;
    }
    if (e.data.length < 1 || e.data.length > MAX_OBJECT_BYTES || !HEX64.test(e.name)) {
      continue;
    }
    const sum = crypto.createHash("sha256").update(e.data).digest("hex");
    if (sum !== e.name) {
      continue; // bütünlük hatası — atla
    }
    writeVerifiedRawObject(root, projeId, e.name, e.data);
    objectCount++;
  }
  return { manifest, objectCount };
}

// ─── Gelen paket işleme ─────────────────────────────────────────────────────

/** paketGonderenGuvenilirMi */
export function packSenderTrusted(root: string, fileName: string, senderPeerId: string): boolean {
  const parsed = parsePackFileName(fileName);
  if (!parsed) {
    return false;
  }
  try {
    return isMember(readGroup(root, parsed.grupId), senderPeerId);
  } catch {
    return false;
  }
}

export interface AppliedPack {
  grupId: string;
  projeId: string;
  manifest: PackManifest;
  objectCount: number;
}

/** pushPaketiIsle — dosya aktarımıyla gelen bir .pack dosyasını doğrular, şifresini çözer ve uygular. */
export function applyIncomingPackFile(root: string, filePath: string, senderPeerId: string): AppliedPack {
  const parsed = parsePackFileName(filePath);
  if (!parsed) {
    throw new Error("proje ağı paketi değil");
  }
  const { grupId, projeId } = parsed;
  let g;
  try {
    g = readGroup(root, grupId);
  } catch (err) {
    throw new Error(`bilinmeyen grup: ${describeError(err)}`);
  }
  if (!isMember(g, senderPeerId)) {
    throw new Error(`gönderen ${g.ad} grubunun üyesi değil`);
  }
  const sealed = fs.readFileSync(filePath);
  let plain: Buffer;
  try {
    plain = decrypt(g.sir, sealed);
  } catch (err) {
    throw new Error(`paket çözülemedi (yanlış grup anahtarı olabilir): ${describeError(err)}`);
  }
  ensureProjectSkeleton(root, projeId, grupId);
  const { manifest, objectCount } = applyPack(root, projeId, plain);
  return { grupId, projeId, manifest, objectCount };
}
