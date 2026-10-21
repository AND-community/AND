// Gönderilen ve gelen DeltaX davetlerinin kalıcı kaydı (Eklentiler/DeltaX/
// gecmis.go'daki davetDefteri ve bekleyen_davetler.json'un karşılığı).
// "vscode" modülüne bağımlı değildir; zarfIsleyici.test.ts ile sınanır.
//
// Dosyalar her işlemde yeniden okunur: bir davet bir VS Code penceresinden
// gönderilip yanıtı başka bir pencerede (DM'leri işleyen liderde) işlenebilir.

import { jsonOku, jsonYaz } from "../dmCore";
import type { InvitePayload } from "./protocol";

/** Bir davetin kabulü gönderildikten sonra bu süre içinde gelmeli (giden kutusu bir daveti 7 güne kadar bekletebilir). */
export const DAVET_GECERLILIK_MS = 30 * 24 * 60 * 60 * 1000;
/** Bundan eski bir çek isteği yanıtlanmaz: paket gönderimi isteyenin şu an çevrimiçi olmasını gerektirir. */
export const PULL_GECERLILIK_MS = 60 * 60 * 1000;

/** grupId → peerId → davetin gönderildiği an */
type Defter = Record<string, Record<string, string>>;

function defterOku(dosya: string): Defter {
  const d = jsonOku<unknown>(dosya, {});
  return d && typeof d === "object" && !Array.isArray(d) ? (d as Defter) : {};
}

/** Davet gönderilirken kaydedilir; süresi dolan eski kayıtlar bu sırada düşer. */
export function davetGonderildi(dosya: string, grupId: string, peerId: string, t: Date): void {
  const eski = defterOku(dosya);
  const d: Defter = {};
  for (const [g, kisiler] of Object.entries(eski)) {
    for (const [p, zaman] of Object.entries(kisiler ?? {})) {
      if (t.getTime() - Date.parse(String(zaman)) <= DAVET_GECERLILIK_MS) {
        (d[g] ??= {})[p] = String(zaman);
      }
    }
  }
  (d[grupId] ??= {})[peerId] = t.toISOString();
  jsonYaz(dosya, d);
}

/**
 * Bir "kabul" ancak o eşe o grup için gerçekten davet gönderildiyse ve yanıt
 * davetGecerlilik içinde geldiyse uygulanır: aksi halde hiç davet edilmemiş
 * biri bile "kabul ettim" diyerek gruba katılabilirdi. Süre yanıtın alındığı
 * ana göre ölçülür (VS Code sonradan açılsa da süresinde gelmiş yanıt tanınır).
 */
export function davetGecerliMi(dosya: string, grupId: string, peerId: string, alindi: Date): boolean {
  const t = Date.parse(String(defterOku(dosya)[grupId]?.[peerId]));
  return !Number.isNaN(t) && alindi.getTime() - t <= DAVET_GECERLILIK_MS;
}

/** Yanıt (kabul ya da ret) geldi: davet kullanılmıştır, ikinci bir yanıtı tanınmaz. */
export function davetKullanildi(dosya: string, grupId: string, peerId: string): void {
  const d = defterOku(dosya);
  if (d[grupId]?.[peerId] === undefined) {
    return;
  }
  delete d[grupId][peerId];
  if (Object.keys(d[grupId]).length === 0) {
    delete d[grupId];
  }
  jsonYaz(dosya, d);
}

/** Sana gelen, henüz yanıtlamadığın bir davet. */
export interface BekleyenDavet {
  grup_id: string;
  /** Daveti gönderen (bağlantıdan doğrulanmış Peer ID). */
  gonderen_peer: string;
  payload: InvitePayload;
  /** Davetin bu düğüme ulaştığı an. */
  alindi: string;
}

/** Bekleyen davetler; süresi dolanlar (davet eden artık tanımaz) listelenmez. */
export function bekleyenDavetler(dosya: string, simdi: Date): BekleyenDavet[] {
  const liste = jsonOku<unknown>(dosya, []);
  if (!Array.isArray(liste)) {
    return [];
  }
  return (liste as BekleyenDavet[]).filter(
    (d) => typeof d?.grup_id === "string" && simdi.getTime() - Date.parse(d.alindi) <= DAVET_GECERLILIK_MS
  );
}

/** Aynı grup için bekleyen bir davet varsa eklenmez; eklendiyse true. */
export function bekleyenDavetEkle(dosya: string, d: BekleyenDavet, simdi: Date): boolean {
  const liste = bekleyenDavetler(dosya, simdi);
  if (liste.some((x) => x.grup_id === d.grup_id)) {
    return false;
  }
  liste.push(d);
  jsonYaz(dosya, liste);
  return true;
}

export function bekleyenDavetSil(dosya: string, grupId: string): void {
  const liste = jsonOku<unknown>(dosya, []);
  if (!Array.isArray(liste)) {
    return;
  }
  const kalan = (liste as BekleyenDavet[]).filter((d) => d?.grup_id !== grupId);
  if (kalan.length !== liste.length) {
    jsonYaz(dosya, kalan);
  }
}
