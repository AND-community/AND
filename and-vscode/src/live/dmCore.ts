// DM akışının VS Code'dan bağımsız çekirdeği ("vscode" modülüne bağımlı
// değildir; dmCore.test.ts ile sınanır):
//
// - İmleç: AND'ın DM geçmişinde nereye kadar işlendiği
//   (Eklentiler/DeltaX/gecmis.go'daki dmImleci'nin karşılığı). Böylece ne
//   iki yoklama arasında ne de VS Code kapalıyken gelen bir DM kaybolur.
// - Makine mesajları: DM ile taşınan eklenti protokolleri (DeltaX'in grup
//   mesajları). İnsanlara gösterilmez, okunmamış sayılmaz.
// - Konuşmalar: Canlı Panel'in Doğrudan mesaj sekmesi için eşe göre
//   gruplanmış görünüm ve AND'ın okundu bilgisine göre okunmamış sayıları.

import * as crypto from "crypto";
import * as fs from "fs";
import * as path from "path";
import { DMMsg, DMRecord } from "./liveTypes";

/** protokol.DeltaXOnEki: DeltaX grup protokolü mesajlarının öneki (ardından sürüm ve "|"). */
export const MAKINE_ONEKI = "AND-PROJEAG/";

/** protokol.MakineIletisiMi: metin eklentiler arası bir protokol mesajı mı (insanlar için değil). */
export function makineIletisiMi(metin: string): boolean {
  return metin.startsWith(MAKINE_ONEKI);
}

/** pluginapi.GecerliPeerID: AND'ın kendi uyguladığı biçim denetimi (base58: 0, O, I ve l yok). */
export function gecerliPeerID(s: string): boolean {
  return /^[1-9A-HJ-NP-Za-km-z]{46,64}$/.test(s);
}

export function kisaPeerID(p: string): string {
  return p.length > 14 ? `${p.slice(0, 12)}…` : p;
}

/** Bu düğüme gelen bir DM. */
export interface GelenDM {
  /** Gönderen: bağlantıdan doğrulanmış Peer ID. Yetki kararları yalnızca buna dayanır. */
  peerId: string;
  /** Gönderenin KENDİ beyan ettiği ad (doğrulanmamış); yalnızca gösterim için. */
  ad?: string;
  text: string;
  /** Bu düğüme ulaştığı an (AND'ın saatiyle; gönderen belirleyemez). */
  at: string;
}

export function gelenKayittan(r: DMRecord): GelenDM {
  return { peerId: r.peer_id, ad: r.name, text: r.text, at: r.at };
}

/**
 * Geçmişi olmayan eski bir AND'da canlı yoklamadan gelen mesaj. Gönderen
 * verified_peer_id'dir. Onu da göndermeyen çok eski bir AND'da from
 * kullanılır: from ya gönderenin kendi Peer ID'sidir ya da "ad (12D3Koo…)"
 * biçimindedir (Peer ID biçiminde değildir), başka birinin Peer ID'si olamaz.
 */
export function gelenYoklamadan(m: DMMsg, simdi: Date): GelenDM {
  return { peerId: m.verified_peer_id || m.from, ad: adParcasi(m.from), text: m.text, at: simdi.toISOString() };
}

/** "ad (12D3KooW…)" → "ad"; yalnızca Peer ID'yse ad yoktur. */
export function adParcasi(from: string): string | undefined {
  const i = from.lastIndexOf(" (");
  if (i > 0 && from.endsWith("…)")) {
    return from.slice(0, i);
  }
  return gecerliPeerID(from) ? undefined : from;
}

/** Bildirimde gösterilecek ad: beyan edilen ad doğrulanmadığı için Peer ID'nin başı da yazılır. */
export function gorunenAd(dm: { peerId: string; ad?: string }): string {
  return dm.ad ? `${dm.ad} (${kisaPeerID(dm.peerId)})` : kisaPeerID(dm.peerId);
}

// ─── Zaman ────────────────────────────────────────────────────────────────────

/** RFC 3339 zamanını nanosaniye çözünürlüğünde sayıya çevirir: AND kesirli saniyeyi 9 haneye kadar yazar, Date.parse milisaniyede keser. */
export function zamanNs(s: string | undefined): bigint | undefined {
  if (!s) {
    return undefined;
  }
  const m = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(s);
  if (!m) {
    const t = Date.parse(s);
    return Number.isNaN(t) ? undefined : BigInt(t) * 1_000_000n;
  }
  const t = Date.parse(m[1] + m[3]);
  return Number.isNaN(t) ? undefined : BigInt(t) * 1_000_000n + BigInt((m[2] ?? "").padEnd(9, "0"));
}

/** a, b'den sonra mı; b yoksa a her zaman sonradır. */
export function sonraMi(a: string, b: string | undefined): boolean {
  const ta = zamanNs(a);
  if (ta === undefined) {
    return false;
  }
  const tb = zamanNs(b);
  return tb === undefined || ta > tb;
}

// ─── İmleç ────────────────────────────────────────────────────────────────────

/** En son işlenen kaydın kimliği ve zamanı. İkisi de yoksa hiç çalışılmamıştır. */
export interface DmImleci {
  id?: string;
  zaman?: string;
}

export function imlecBos(im: DmImleci): boolean {
  return !im.id && !im.zaman;
}

/**
 * kayitlar içinde imleçten sonra gelenler. Geçmiş geliş sırasıyla tutulur:
 * imlecin kaydı yanıttaysa ondan sonrakiler yenidir (saat geri alınsa bile).
 * Kayıt geçmişten düşmüşse ya da AND kimlik göndermiyorsa imlecin zamanından
 * sonra gelen ilk kayıttan itibaren alınır.
 */
export function yeniKayitlar(im: DmImleci, kayitlar: DMRecord[]): DMRecord[] {
  if (im.id) {
    for (let i = kayitlar.length - 1; i >= 0; i--) {
      if (kayitlar[i].id === im.id) {
        return kayitlar.slice(i + 1);
      }
    }
  }
  const i = kayitlar.findIndex((r) => sonraMi(r.at, im.zaman));
  return i === -1 ? [] : kayitlar.slice(i);
}

export interface AkisAdimi {
  yeniler: DMRecord[];
  imlec: DmImleci;
  /** İmleç ilerledi; diske yazılmalı. */
  degisti: boolean;
}

/**
 * Bir geçmiş yanıtını imlece göre işler. İlk çalıştırmada (imleç boş) hiçbir
 * kayıt yeni sayılmaz, imleç geçmişin sonuna (geçmiş boşsa isteğin anına)
 * konur: eski mesajlar için bildirim yağmaz, eski DeltaX mesajları yeniden
 * işlenmez.
 */
export function akisAdimi(im: DmImleci, kayitlar: DMRecord[], istendi: Date): AkisAdimi {
  if (imlecBos(im)) {
    return { yeniler: [], imlec: sonKayit(kayitlar) ?? { zaman: istendi.toISOString() }, degisti: true };
  }
  const yeniler = yeniKayitlar(im, kayitlar);
  const son = sonKayit(yeniler);
  return son ? { yeniler, imlec: son, degisti: true } : { yeniler, imlec: im, degisti: false };
}

function sonKayit(k: DMRecord[]): DmImleci | undefined {
  const r = k[k.length - 1];
  return r ? { id: r.id, zaman: r.at } : undefined;
}

export function imleciOku(dosya: string): DmImleci {
  const v = jsonOku<Record<string, unknown>>(dosya, {});
  return {
    id: typeof v?.id === "string" ? v.id : undefined,
    zaman: typeof v?.zaman === "string" ? v.zaman : undefined,
  };
}

// ─── Durum dosyaları ──────────────────────────────────────────────────────────

/**
 * VS Code'un bu uzantıya ayırdığı depolama dizininde, AND veri dizinine özgü
 * bir durum dosyası: deltax.dataDir değişirse ya da birden çok AND kimliği
 * kullanılırsa durumlar karışmaz.
 */
export function durumDosyasi(durumDizini: string, andDataDir: string, ad: string): string {
  let kok = path.resolve(andDataDir);
  if (process.platform === "win32") {
    kok = kok.toLowerCase();
  }
  const anahtar = crypto.createHash("sha256").update(kok).digest("hex").slice(0, 12);
  return path.join(durumDizini, `${ad}-${anahtar}.json`);
}

export function jsonOku<T>(dosya: string, varsayilan: T): T {
  try {
    return JSON.parse(fs.readFileSync(dosya, "utf8")) as T;
  } catch {
    return varsayilan;
  }
}

/**
 * Önce geçici bir dosyaya yazıp yerine taşır: aynı dosyayı okuyan başka bir
 * VS Code penceresi yarım yazılmış bir dosya görmez. Hata yutulur: bu
 * dosyalar kaybolursa en kötü ihtimalle bir mesaj yeniden işlenir.
 */
export function jsonYaz(dosya: string, deger: unknown): void {
  try {
    fs.mkdirSync(path.dirname(dosya), { recursive: true });
    const gecici = `${dosya}.${process.pid}.yeni`;
    fs.writeFileSync(gecici, JSON.stringify(deger), { mode: 0o600 });
    fs.renameSync(gecici, dosya);
  } catch {
    // yok say
  }
}

// ─── Konuşmalar ───────────────────────────────────────────────────────────────

export interface DMMesaj {
  metin: string;
  at: string;
  giden: boolean;
  /** Giden kutusunda, alıcının bağlanmasını bekliyor. */
  bekliyor?: boolean;
  /** Bir hafta içinde iletilemedi. */
  basarisiz?: boolean;
}

export interface Konusma {
  peerId: string;
  /** Eşin son beyan ettiği ad (doğrulanmamış); yoksa boş. */
  ad: string;
  okunmamis: number;
  mesajlar: DMMesaj[];
}

/** Konuşma başına panele gönderilen en fazla mesaj. */
export const KONUSMA_MESAJ_SINIRI = 200;

/** İnsanlar arası mesajları eşe göre gruplar; en son hareket eden konuşma en üstte. */
export function konusmalar(kayitlar: DMRecord[], okundu: Record<string, string>): Konusma[] {
  const harita = new Map<string, { k: Konusma; sira: number }>();
  kayitlar.forEach((r, sira) => {
    if (!r.peer_id || makineIletisiMi(r.text)) {
      return;
    }
    let girdi = harita.get(r.peer_id);
    if (!girdi) {
      girdi = { k: { peerId: r.peer_id, ad: "", okunmamis: 0, mesajlar: [] }, sira };
      harita.set(r.peer_id, girdi);
    }
    girdi.sira = sira;
    const k = girdi.k;
    if (!r.outgoing && r.name) {
      k.ad = r.name;
    }
    const m: DMMesaj = { metin: r.text, at: r.at, giden: !!r.outgoing };
    if (r.pending) {
      m.bekliyor = true;
    }
    if (r.failed) {
      m.basarisiz = true;
    }
    k.mesajlar.push(m);
    if (!r.outgoing && sonraMi(r.at, okundu[r.peer_id])) {
      k.okunmamis++;
    }
  });
  return [...harita.values()]
    .sort((a, b) => b.sira - a.sira)
    .map(({ k }) => ({ ...k, mesajlar: k.mesajlar.slice(-KONUSMA_MESAJ_SINIRI) }));
}

/** AND'ın ana menü rozetiyle aynı sayım: okundu işaretinden sonra gelen, insanların yazdığı mesajlar. */
export function okunmamisSayisi(kayitlar: DMRecord[], okundu: Record<string, string>): number {
  return kayitlar.filter((r) => !r.outgoing && !makineIletisiMi(r.text) && sonraMi(r.at, okundu[r.peer_id])).length;
}

/** peerId'den gelen son (insanın yazdığı) mesajın anı: konuşmayı okundu işaretlemek için. */
export function sonGelenZamani(kayitlar: DMRecord[], peerId: string): string | undefined {
  for (let i = kayitlar.length - 1; i >= 0; i--) {
    const r = kayitlar[i];
    if (r.peer_id === peerId && !r.outgoing && !makineIletisiMi(r.text)) {
      return r.at;
    }
  }
  return undefined;
}
