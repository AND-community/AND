import * as fs from "fs";
import * as path from "path";
import { readIgnorePatterns, isIgnored } from "./ignoreRules";
import { readObject, writeObject, writeBlobWithKnownHash, blobHashOf, objectExists } from "./objectStore";
import { Agac, AgacGirdi, DurumSonucu } from "./types";

// vcs.go'daki "Çalışma dizini ↔ ağaç" bölümünün TypeScript karşılığı.

const MAX_DOSYA_BOYUTU = 200 * 1024 * 1024; // 200 MB

export function readTree(root: string, projeId: string, hash: string): Agac {
  if (!hash) {
    return { girdiler: [] };
  }
  const { tip, payload } = readObject(root, projeId, hash);
  if (tip !== "T".charCodeAt(0)) {
    throw new Error("beklenmeyen nesne tipi: ağaç bekleniyordu");
  }
  return JSON.parse(payload.toString("utf8")) as Agac;
}

export function treeMap(root: string, projeId: string, hash: string): Map<string, AgacGirdi> {
  const agac = readTree(root, projeId, hash);
  const m = new Map<string, AgacGirdi>();
  for (const g of agac.girdiler ?? []) {
    m.set(g.yol, g);
  }
  return m;
}

interface WalkEntry {
  rel: string;
  full: string;
  stat: fs.Stats;
}

/** calismaDir'i .projeagignore kurallarına uyarak gezer; 200MB üstü dosyalar atlanır. */
function* walkWorkdir(calismaDir: string, desenler: string[], atlanan: string[]): Generator<WalkEntry> {
  function* recurse(dir: string): Generator<WalkEntry> {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        yield* recurse(full);
        continue;
      }
      if (!entry.isFile()) {
        continue; // sembolik bağlar vb. — Go tarafı da dizin olarak izlemez
      }
      const rel = path.relative(calismaDir, full).split(path.sep).join("/");
      if (isIgnored(desenler, rel)) {
        continue;
      }
      const stat = fs.statSync(full);
      if (stat.size > MAX_DOSYA_BOYUTU) {
        atlanan.push(rel);
        continue;
      }
      yield { rel, full, stat };
    }
  }
  yield* recurse(calismaDir);
}

interface CacheEntry {
  mtimeMs: number;
  size: number;
  hash: string;
}

/**
 * Proje/çalışma dizini başına, dosya yolu → (mtime, boyut, hash) eşlemesi
 * tutan bir önbellek. Her tam tarama (durum hesaplama, commit, birleştirme)
 * içeriği değişmemiş dosyaları yeniden OKUYUP HASH'LEMEK zorunda kalmasın
 * diye DeltaXRepository tarafından proje ömrü boyunca saklanır ve
 * paylaşılır (bkz. scmController.ts).
 */
export type FileHashCache = Map<string, CacheEntry>;

// "Racily clean" koruması: git'in kendi stat-cache mekanizmasıyla aynı
// mantık — bir dosyanın mtime'ı çok yakın zamanlıysa (dosya sistemi zaman
// çözünürlüğü penceresi içindeyse), o dosya bu taramadan SONRA da aynı
// milisaniye içinde tekrar değişmiş olabilir; böyle bir mtime'a ASLA
// güvenilmez, her zaman yeniden okunup hash'lenir.
//
// Bilinen ödünleşim: bu, buildTreeFromWorkdir'in AZ ÖNCE kendi yazdığı bir
// dosyanın hash'ini de (taze mtime'a sahip olduğu için) güvenilmez sayar —
// ör. bir commit/birleştirmeden hemen sonra çağrılan refresh(), değişen
// dosyaları ikinci kez okuyup hash'ler. Bu, dış bir aktörün (başka bir
// süreç/AND TUI'si) araya girebileceği GERÇEK bir yarış durumuna karşı
// korumanın maliyetidir; pencereyi daraltmak ya da "bu süreç içinde biz
// yazdık" bilgisini ayrıca izlemek mümkün olsa da, bunun getirdiği
// karmaşıklık/hata riski, kazanılacak performanstan (yalnızca değişen
// dosyalar için, yalnızca commit/birleştirme anında) daha yüksek görüldü.
const RACILY_CLEAN_WINDOW_MS = 2000;

/** Bir dosyanın hash'ini döndürür; mümkünse önbellekten, değilse okuyup hesaplayarak. */
function hashWithCache(
  cache: FileHashCache | undefined,
  now: number,
  entry: WalkEntry
): { hash: string; data?: Buffer } {
  if (cache) {
    const cached = cache.get(entry.rel);
    const safelyOld = now - entry.stat.mtimeMs > RACILY_CLEAN_WINDOW_MS;
    if (cached && safelyOld && cached.mtimeMs === entry.stat.mtimeMs && cached.size === entry.stat.size) {
      return { hash: cached.hash };
    }
  }
  const data = fs.readFileSync(entry.full);
  const hash = blobHashOf(data);
  cache?.set(entry.rel, { mtimeMs: entry.stat.mtimeMs, size: entry.stat.size, hash });
  return { hash, data };
}

export interface BuildTreeResult {
  hash: string;
  girdiler: AgacGirdi[];
  atlanan: string[];
}

/**
 * agacOlustur — çalışma dizininin anlık görüntüsünü yeni bir ağaç nesnesi
 * olarak yazar. `cache` verilirse (mtime, boyut) değişmemiş dosyalar için
 * içerik yeniden okunmaz/hash'lenmez — yalnızca değişen ya da önbellekte
 * hiç olmayan dosyalar diskten okunur.
 */
export function buildTreeFromWorkdir(
  root: string,
  projeId: string,
  calismaDir: string,
  cache?: FileHashCache
): BuildTreeResult {
  const desenler = readIgnorePatterns(calismaDir);
  const atlanan: string[] = [];
  const girdiler: AgacGirdi[] = [];
  const now = Date.now();
  for (const entry of walkWorkdir(calismaDir, desenler, atlanan)) {
    let { hash, data } = hashWithCache(cache, now, entry);
    if (data !== undefined) {
      writeBlobWithKnownHash(root, projeId, hash, data);
    } else if (!objectExists(root, projeId, hash)) {
      // Bu SIK karşılaşılan, gerekli bir kontroldür — yalnızca nadir bir
      // "nesneler/ elle silindi" ihtimaline karşı bir yedek değil: aynı
      // `cache`, computeStatus() tarafından da (bkz. calismaSnapshotHesapla)
      // kullanılır ve o yol yalnızca HASH hesaplar, hiçbir nesne YAZMAZ.
      // Yani bir dosyanın hash'i az önce bir durum yenilemesinden
      // (refresh/watcher) önbelleğe girmiş ama karşılığındaki blob nesnesi
      // hiç diske yazılmamış olabilir — bu durumda `data === undefined`
      // olur (önbellek isabeti) ama nesne gerçekten yok. Bu kontrol
      // olmadan ağaca, deposunda hiç karşılığı olmayan bir hash referansı
      // eklenir ve proje sessizce bozulur. Bulunca içeriği yeniden okuyup
      // GERÇEKTEN yazılan hash'i (writeObject'in döndürdüğü) kullanırız;
      // önbellekteki hash'e körü körüne güvenip ağaca tutarsız bir
      // referans eklemeyiz.
      const freshData = fs.readFileSync(entry.full);
      hash = writeObject(root, projeId, "B", freshData);
      cache?.set(entry.rel, { mtimeMs: entry.stat.mtimeMs, size: entry.stat.size, hash });
    }
    girdiler.push({ yol: entry.rel, hash, boyut: entry.stat.size });
  }
  girdiler.sort((a, b) => (a.yol < b.yol ? -1 : a.yol > b.yol ? 1 : 0));
  const payload = Buffer.from(JSON.stringify({ girdiler }), "utf8");
  const hash = writeObject(root, projeId, "T", payload);
  return { hash, girdiler, atlanan };
}

interface WorkDosya {
  hash: string;
  boyut: number;
}

function calismaSnapshotHesapla(calismaDir: string, cache?: FileHashCache): Map<string, WorkDosya> {
  const desenler = readIgnorePatterns(calismaDir);
  const sonuc = new Map<string, WorkDosya>();
  const atlanan: string[] = []; // dosya boyutu aşımı burada sessizce atlanır (Go tarafıyla aynı)
  const now = Date.now();
  for (const entry of walkWorkdir(calismaDir, desenler, atlanan)) {
    const { hash } = hashWithCache(cache, now, entry);
    sonuc.set(entry.rel, { hash, boyut: entry.stat.size });
  }
  return sonuc;
}

/** durumHesapla — çalışma dizinini HEAD ağacıyla karşılaştırır (Eklenen/Değişen/Silinen). */
export function computeStatus(
  root: string,
  projeId: string,
  calismaDir: string,
  headTree: string,
  cache?: FileHashCache
): DurumSonucu {
  const work = calismaSnapshotHesapla(calismaDir, cache);
  const head = treeMap(root, projeId, headTree);

  const eklenen: string[] = [];
  const degisen: string[] = [];
  const silinen: string[] = [];

  for (const [yol, wf] of work) {
    const hg = head.get(yol);
    if (!hg) {
      eklenen.push(yol);
    } else if (hg.hash !== wf.hash) {
      degisen.push(yol);
    }
  }
  for (const yol of head.keys()) {
    if (!work.has(yol)) {
      silinen.push(yol);
    }
  }

  eklenen.sort();
  degisen.sort();
  silinen.sort();
  return { eklenen, degisen, silinen };
}

function writeFileToWorkdir(root: string, projeId: string, calismaDir: string, g: AgacGirdi): void {
  const { payload } = readObject(root, projeId, g.hash);
  const full = path.join(calismaDir, ...g.yol.split("/"));
  fs.mkdirSync(path.dirname(full), { recursive: true, mode: 0o700 });
  fs.writeFileSync(full, payload, { mode: 0o600 });
}

/**
 * dalaGec — çalışma dizinini TAMAMEN siler ve hedef ağacın içeriğini yeniden
 * yazar. Go tarafındaki davranışla birebir aynı: yalnızca izlenen dosyalar
 * değil, .projeagignore ile hariç tutulan / hiç izlenmeyen dosyalar da dahil
 * olmak üzere çalışma klasöründeki HER ŞEY önce silinir. Çağıran taraf, veri
 * kaybını önlemek için önce computeStatus ile çalışma dizininin temiz
 * olduğunu doğrulamalı ve kullanıcıyı bu konuda açıkça uyarmalıdır.
 *
 * `cache` verilirse, işlem sonunda TAMAMEN TEMİZLENİR: dizindeki her dosya
 * silinip yeniden yazıldığı için önbellekteki eski (mtime, boyut) kayıtları
 * artık geçersizdir. Bu temizlik, yalnızca `hashWithCache`'in "racily
 * clean" zaman penceresine (bkz. RACILY_CLEAN_WINDOW_MS) güvenmek yerine,
 * yavaş bir diskte/kaba mtime çözünürlüğüne sahip bir dosya sisteminde bile
 * eski bir sürümün hash'inin yanlışlıkla yeniden kullanılmasını yapısal
 * olarak imkânsız kılar.
 */
export function checkoutTree(
  root: string,
  projeId: string,
  calismaDir: string,
  hedefTree: string,
  cache?: FileHashCache
): void {
  const entries = fs.readdirSync(calismaDir, { withFileTypes: true });
  for (const e of entries) {
    fs.rmSync(path.join(calismaDir, e.name), { recursive: true, force: true });
  }
  const agac = readTree(root, projeId, hedefTree);
  for (const g of agac.girdiler ?? []) {
    writeFileToWorkdir(root, projeId, calismaDir, g);
  }
  cache?.clear();
}

/** İki ağaç arasındaki farkı (commit geçmişi görünümü için) hesaplar. */
export function diffTrees(
  root: string,
  projeId: string,
  eskiTree: string,
  yeniTree: string
): DurumSonucu {
  const eski = treeMap(root, projeId, eskiTree);
  const yeni = treeMap(root, projeId, yeniTree);

  const eklenen: string[] = [];
  const degisen: string[] = [];
  const silinen: string[] = [];

  for (const [yol, yf] of yeni) {
    const ef = eski.get(yol);
    if (!ef) {
      eklenen.push(yol);
    } else if (ef.hash !== yf.hash) {
      degisen.push(yol);
    }
  }
  for (const yol of eski.keys()) {
    if (!yeni.has(yol)) {
      silinen.push(yol);
    }
  }

  eklenen.sort();
  degisen.sort();
  silinen.sort();
  return { eklenen, degisen, silinen };
}
