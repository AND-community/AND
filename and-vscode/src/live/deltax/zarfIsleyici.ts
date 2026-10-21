// model.go'daki zarfIsle'ın VS Code karşılığı: gelen her DeltaX zarfı burada
// işlenir. "vscode" modülüne bağımlı değildir (bildirim ve soru gibi kullanıcı
// etkileşimleri ZarfOrtami üzerinden yapılır); zarfIsleyici.test.ts ile
// sınanır.
//
// Yetki kararları YALNIZCA bağlantıdan doğrulanmış gönderene (GelenDM.peerId)
// dayanır. Payload'daki kimlikler (gonderen, uye, isteyen) göndericinin kendi
// beyanıdır; her işleyici bunların doğrulanmış gönderenle eşleştiğini de
// denetler, yoksa bir eş başka birinin (ör. kurucunun) yerine geçebilirdi.

import { deleteGroup, readGroup, readProject, saveGroup } from "../../groupProjectStore";
import { Grup, Proje, Uye } from "../../types";
import { GelenDM, gecerliPeerID, kisaPeerID } from "../dmCore";
import {
  BekleyenDavet,
  DAVET_GECERLILIK_MS,
  PULL_GECERLILIK_MS,
  bekleyenDavetEkle,
  bekleyenDavetSil,
  davetGecerliMi,
  davetKullanildi,
} from "./davetler";
import { isAuthorized, isMember, removeMember, setAdmin, upsertMember } from "./groupOps";
import {
  CloseGroupPayload,
  DeltaXIstemcisi,
  Envelope,
  InvitePayload,
  InviteResponsePayload,
  LeavePayload,
  PullRequestPayload,
  PushNoticePayload,
  broadcastRoster,
  decodeEnvelope,
  push,
  sendInviteResponse,
} from "./protocol";

export interface ZarfOrtami {
  /** DeltaX kök dizini (gruplar/, projeler/). */
  root(): string;
  /** Gönderilen davetlerin defteri (bkz. davetler.ts). */
  defterDosyasi(): string;
  /** Bekleyen (yanıtlanmamış) gelen davetler. */
  bekleyenDosyasi(): string;
  istemci(): DeltaXIstemcisi | undefined;
  ben(): Promise<Uye | undefined>;
  bildir(mesaj: string, hata?: boolean): void;
  /** Bekleyenlere yeni bir davet eklendi: kullanıcıya sorulabilir. */
  davetGeldi(d: BekleyenDavet): void;
  /** Gruplar ya da bekleyen davetler değişti. */
  gruplarDegisti(): void;
  simdi(): Date;
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function grupOku(o: ZarfOrtami, id: string): Grup | undefined {
  try {
    return readGroup(o.root(), id);
  } catch {
    return undefined;
  }
}

/** dm bir DeltaX zarfıysa işler ve true döner: genel DM akışına (bildirim, Canlı Panel) düşmemeli. */
export async function zarfIsle(o: ZarfOrtami, dm: GelenDM): Promise<boolean> {
  const z = decodeEnvelope(dm.text);
  if (!z) {
    return false;
  }
  if (!gecerliPeerID(dm.peerId)) {
    return true; // gönderen doğrulanamadı
  }
  const t = Date.parse(dm.at);
  const alindi = Number.isNaN(t) ? o.simdi() : new Date(t);
  try {
    await isle(o, dm.peerId, z, alindi);
  } catch {
    // zarfIsle de bozuk/beklenmeyen payload'ları sessizce yutar
  }
  return true;
}

async function isle(o: ZarfOrtami, gonderen: string, z: Envelope, alindi: Date): Promise<void> {
  switch (z.tip) {
    case "davet":
    case "uye_guncelle":
      return davetYaDaUyeListesi(o, gonderen, z, alindi);
    case "davet_yanit":
      return davetYaniti(o, gonderen, z, alindi);
    case "push_bildir":
      return pushBildirimi(o, z);
    case "pull_istek":
      return cekIstegi(o, gonderen, z, alindi);
    case "ayril":
      return ayrilma(o, gonderen, z);
    case "grup_kapat":
      return grupKapatma(o, gonderen, z);
  }
}

async function davetYaDaUyeListesi(o: ZarfOrtami, gonderen: string, z: Envelope, alindi: Date): Promise<void> {
  const dp = z.payload as InvitePayload;
  if (!z.grup_id || !Array.isArray(dp?.uyeler) || dp.gonderen?.peer_id !== gonderen) {
    return;
  }
  const mevcut = grupOku(o, z.grup_id);
  if (mevcut) {
    // Bildiğimiz bir grup için gelen güncelleme yalnızca zaten güvendiğimiz
    // bir yetkiliden (kurucu ya da yönetici) gelirse uygulanır.
    if (!isAuthorized(mevcut, gonderen)) {
      return;
    }
    mevcut.uyeler = dp.uyeler;
    mevcut.yoneticiler = dp.yoneticiler;
    mevcut.ad = dp.grup_ad;
    saveGroup(o.root(), mevcut);
    o.bildir(`"${dp.grup_ad}" grubunun üye listesi güncellendi.`);
    o.gruplarDegisti();
    return;
  }
  if (z.tip === "uye_guncelle") {
    return; // bilinmeyen grup için üye listesi yok sayılır
  }
  // Davet eden, bu kadar eski bir davetin kabulünü artık tanımaz.
  if (o.simdi().getTime() - alindi.getTime() > DAVET_GECERLILIK_MS) {
    return;
  }
  const d: BekleyenDavet = { grup_id: z.grup_id, gonderen_peer: gonderen, payload: dp, alindi: alindi.toISOString() };
  if (bekleyenDavetEkle(o.bekleyenDosyasi(), d, o.simdi())) {
    o.gruplarDegisti();
    o.davetGeldi(d);
  }
}

async function davetYaniti(o: ZarfOrtami, gonderen: string, z: Envelope, alindi: Date): Promise<void> {
  const yp = z.payload as InviteResponsePayload;
  if (!z.grup_id || yp?.uye?.peer_id !== gonderen) {
    return;
  }
  // Bu grup için gerçekten bu eşe davet gönderdiğimizi doğrula (bkz. davetGecerliMi).
  if (!davetGecerliMi(o.defterDosyasi(), z.grup_id, gonderen, alindi)) {
    return;
  }
  const g = grupOku(o, z.grup_id);
  const ben = await o.ben();
  if (!g || !ben || !isAuthorized(g, ben.peer_id)) {
    return;
  }
  // Yanıt ne olursa olsun davet kullanılmıştır: reddeden biri sonradan aynı davetle "kabul" edemez.
  davetKullanildi(o.defterDosyasi(), z.grup_id, gonderen);
  if (!yp.kabul) {
    o.bildir(`${yp.uye.ad} "${g.ad}" grubuna davetini reddetti.`);
    return;
  }
  upsertMember(g, yp.uye);
  saveGroup(o.root(), g);
  o.bildir(`${yp.uye.ad} "${g.ad}" grubuna katıldı.`);
  o.gruplarDegisti();
  const client = o.istemci();
  if (client) {
    await broadcastRoster(client, g, ben);
  }
}

function pushBildirimi(o: ZarfOrtami, z: Envelope): void {
  const bp = z.payload as PushNoticePayload;
  if (typeof bp?.dal !== "string" || typeof bp.nesne_sayisi !== "number") {
    return;
  }
  o.bildir(`Bir üye değişiklik gönderiyor (${bp.dal}, ${bp.nesne_sayisi} nesne); kabul edilince Uzak Dallar'da görünecek.`);
}

async function cekIstegi(o: ZarfOrtami, gonderen: string, z: Envelope, alindi: Date): Promise<void> {
  const pp = z.payload as PullRequestPayload;
  if (!z.grup_id || !z.proje_id || pp?.isteyen?.peer_id !== gonderen) {
    return;
  }
  // Paket gönderimi isteyenin şu an çevrimiçi olmasını gerektirir: VS Code
  // kapalıyken gelmiş eski bir istek yanıtlanmaz.
  if (o.simdi().getTime() - alindi.getTime() > PULL_GECERLILIK_MS) {
    return;
  }
  const g = grupOku(o, z.grup_id);
  if (!g || !isMember(g, gonderen)) {
    return;
  }
  let p: Proje;
  try {
    p = readProject(o.root(), z.proje_id);
  } catch {
    return;
  }
  const client = o.istemci();
  if (!client) {
    return;
  }
  try {
    await push(client, o.root(), g, p, pp.isteyen, pp.bilinen_hash ?? "");
  } catch (err) {
    o.bildir(`Çek isteği karşılanamadı: ${describeError(err)}`, true);
  }
}

async function ayrilma(o: ZarfOrtami, gonderen: string, z: Envelope): Promise<void> {
  const ap = z.payload as LeavePayload;
  // ayril yalnızca kendi ayrılışını bildirmek içindir: başka bir üyeyi "ayrılmış" gösterip attıramazsın.
  if (!z.grup_id || ap?.uye?.peer_id !== gonderen) {
    return;
  }
  const g = grupOku(o, z.grup_id);
  const ben = await o.ben();
  if (!g || !ben || !isAuthorized(g, ben.peer_id) || !isMember(g, gonderen)) {
    return;
  }
  removeMember(g, gonderen);
  setAdmin(g, ap.uye, false);
  saveGroup(o.root(), g);
  o.bildir(`${ap.uye.ad} "${g.ad}" grubundan ayrıldı.`);
  o.gruplarDegisti();
  const client = o.istemci();
  if (client) {
    await broadcastRoster(client, g, ben);
  }
}

function grupKapatma(o: ZarfOrtami, gonderen: string, z: Envelope): void {
  const kp = z.payload as CloseGroupPayload;
  if (!z.grup_id || kp?.gonderen?.peer_id !== gonderen) {
    return;
  }
  const g = grupOku(o, z.grup_id);
  if (!g || !isAuthorized(g, gonderen)) {
    return;
  }
  deleteGroup(o.root(), g.id);
  o.bildir(`"${g.ad}" grubu kurucusu tarafından kapatıldı.`, true);
  o.gruplarDegisti();
}

/**
 * Bekleyen bir daveti kabul eder ya da reddeder (kabulCmd). Yanıt giden
 * kutusuyla gider; gönderilemezse davet bekleyenlerde kalır ve yeniden
 * denenebilir.
 */
export async function davetiYanitla(o: ZarfOrtami, d: BekleyenDavet, kabul: boolean): Promise<Grup | undefined> {
  const client = o.istemci();
  const ben = await o.ben();
  if (!client || !ben) {
    throw new Error("AND'a canlı bağlantı yok.");
  }
  let g: Grup | undefined;
  if (kabul) {
    g = {
      id: d.grup_id,
      ad: d.payload.grup_ad,
      sir: d.payload.sir,
      kurucu_ad: d.payload.kurucu.ad,
      kurucu_peer: d.payload.kurucu.peer_id,
      yoneticiler: d.payload.yoneticiler,
      uyeler: d.payload.uyeler,
      olusturma_ts: o.simdi().toISOString(),
    };
    upsertMember(g, ben);
    saveGroup(o.root(), g);
  }
  await sendInviteResponse(client, d.gonderen_peer, d.grup_id, kabul, ben);
  bekleyenDavetSil(o.bekleyenDosyasi(), d.grup_id);
  o.gruplarDegisti();
  return g;
}

/** Bildirim ve listelerde davet edenin adı: beyan edilen ad doğrulanmadığı için Peer ID'nin başı da yazılır. */
export function davetEden(d: BekleyenDavet): string {
  const ad = d.payload.gonderen?.ad;
  return ad ? `${ad} (${kisaPeerID(d.gonderen_peer)})` : kisaPeerID(d.gonderen_peer);
}
