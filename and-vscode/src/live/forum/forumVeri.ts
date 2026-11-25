// Forum sekmesinin veri tarafı: konu listesini yükleme, sıralama ve kategori
// sayıları. "vscode" modülüne bağımlı değildir (birim testli).

import { LiveApiError } from "../liveClient";
import { FORUM_CATEGORIES, PostSummary } from "../liveTypes";

/** Bir konunun sıralamada kullanılan zamanı (ms); bilinmiyorsa undefined. */
function konuZamani(k: PostSummary): number | undefined {
  for (const z of [k.last_activity, k.created_at]) {
    const ms = z ? Date.parse(z) : NaN;
    if (!Number.isNaN(ms)) {
      return ms;
    }
  }
  return undefined;
}

/**
 * Konuları son etkinliğe (yoksa oluşturulma zamanına) göre yeniden eskiye
 * sıralar; zamanı bilinmeyenler (eski AND) sona, kendi sıralarıyla gider.
 * Girdiyi değiştirmez.
 */
export function siralaKonular(konular: readonly PostSummary[]): PostSummary[] {
  return konular
    .map((k, i) => ({ k, i, z: konuZamani(k) }))
    .sort((a, b) => {
      if (a.z === undefined || b.z === undefined) {
        return a.z === b.z ? a.i - b.i : a.z === undefined ? 1 : -1;
      }
      return b.z - a.z || a.i - b.i;
    })
    .map((x) => x.k);
}

export interface KategoriSayisi {
  kategori: string;
  sayi: number;
}

/**
 * Konusu olan kategoriler ve konu sayıları, AND'ın kategori sırasıyla
 * (protokol/kategori.go). Listede olmayan (daha yeni bir AND'ın eklediği)
 * kategoriler ilk görüldükleri sırayla sona eklenir.
 */
export function kategoriSayilari(konular: readonly PostSummary[]): KategoriSayisi[] {
  const sayilar = new Map<string, number>();
  for (const k of konular) {
    sayilar.set(k.category, (sayilar.get(k.category) ?? 0) + 1);
  }
  const bilinen = FORUM_CATEGORIES.filter((c) => sayilar.has(c));
  const bilinmeyen = [...sayilar.keys()].filter((c) => !FORUM_CATEGORIES.includes(c));
  return [...bilinen, ...bilinmeyen].map((kategori) => ({ kategori, sayi: sayilar.get(kategori)! }));
}

export interface KonuListesi {
  konular: PostSummary[];
  /** false: bu AND konu listesini vermiyor (eski sürüm, moderatör değilsin). */
  liste: boolean;
}

export interface ForumListeIstemcisi {
  listPosts(): Promise<PostSummary[]>;
  getAllPosts(): Promise<PostSummary[]>;
}

/**
 * Konu listesini yükler. Konu listesi ucu olmayan eski bir AND'da (404)
 * moderatör eski "tüm konular" ucuna düşer; üyeye liste yoktur.
 */
export async function konuListesiYukle(c: ForumListeIstemcisi, moderator: boolean): Promise<KonuListesi> {
  try {
    return { konular: siralaKonular(await c.listPosts()), liste: true };
  } catch (err) {
    if (!(err instanceof LiveApiError) || err.status !== 404) {
      throw err;
    }
  }
  if (!moderator) {
    return { konular: [], liste: false };
  }
  return { konular: siralaKonular(await c.getAllPosts()), liste: true };
}
