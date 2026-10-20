import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { collectAncestors } from "../../commitStore";
import { Grup, Proje, Uye } from "../../types";
import { encrypt } from "./crypto";
import { LiveClient } from "../liveClient";
import { buildPack, packFileName } from "./packSync";

// sync.go'nun "grup üyeleri arasında merkezi ağdan bağımsız özel
// senkronizasyon" protokolünün (zarf + davet/roster/push/pull mesajları)
// TypeScript karşılığı. DM'ler AND'ın /api/v1/dm/* üzerinden, paketler
// /api/v1/file/* üzerinden gönderilir (bkz. LiveClient).

export const PROTOCOL_PREFIX = "AND-PROJEAG/1|";

export type EnvelopeType = "davet" | "uye_guncelle" | "davet_yanit" | "push_bildir" | "pull_istek" | "ayril" | "grup_kapat";

export interface Envelope {
  tip: EnvelopeType;
  grup_id?: string;
  proje_id?: string;
  payload?: unknown;
}

export interface InvitePayload {
  grup_ad: string;
  sir: string;
  kurucu: Uye;
  yoneticiler?: Uye[];
  uyeler: Uye[];
  gonderen: Uye;
}

export interface InviteResponsePayload {
  kabul: boolean;
  uye: Uye;
}

export interface PushNoticePayload {
  dal: string;
  head_hash: string;
  nesne_sayisi: number;
  dosya_adi: string;
}

export interface PullRequestPayload {
  dal: string;
  bilinen_hash: string;
  isteyen: Uye;
}

export interface LeavePayload {
  uye: Uye;
}

export interface CloseGroupPayload {
  gonderen: Uye;
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function encodeEnvelope(z: Envelope): string {
  return PROTOCOL_PREFIX + Buffer.from(JSON.stringify(z), "utf8").toString("base64");
}

export function decodeEnvelope(text: string): Envelope | undefined {
  if (!text.startsWith(PROTOCOL_PREFIX)) {
    return undefined;
  }
  try {
    const raw = Buffer.from(text.slice(PROTOCOL_PREFIX.length), "base64");
    return JSON.parse(raw.toString("utf8")) as Envelope;
  } catch {
    return undefined;
  }
}

function invitePayloadFrom(g: Grup, gonderen: Uye): InvitePayload {
  return {
    grup_ad: g.ad,
    sir: g.sir,
    kurucu: { ad: g.kurucu_ad, peer_id: g.kurucu_peer },
    yoneticiler: g.yoneticiler,
    uyeler: g.uyeler,
    gonderen,
  };
}

// ─── Davetler ───────────────────────────────────────────────────────────────

/** davetGonder */
export async function sendInvite(client: LiveClient, targetPeer: string, g: Grup, self: Uye): Promise<void> {
  const text = encodeEnvelope({ tip: "davet", grup_id: g.id, payload: invitePayloadFrom(g, self) });
  await client.sendDM({ peer_id: targetPeer, message: text });
}

/** davetYanitGonder */
export async function sendInviteResponse(client: LiveClient, targetPeer: string, grupId: string, accept: boolean, self: Uye): Promise<void> {
  const payload: InviteResponsePayload = { kabul: accept, uye: self };
  const text = encodeEnvelope({ tip: "davet_yanit", grup_id: grupId, payload });
  await client.sendDM({ peer_id: targetPeer, message: text });
}

/** uyeGuncellemesiYayinla — güncel üye/yönetici listesini kendisi hariç tüm üyelere yeniden gönderir. */
export async function broadcastRoster(client: LiveClient, g: Grup, self: Uye): Promise<Error[]> {
  const text = encodeEnvelope({ tip: "uye_guncelle", grup_id: g.id, payload: invitePayloadFrom(g, self) });
  const errors: Error[] = [];
  for (const u of g.uyeler) {
    if (u.peer_id === self.peer_id) {
      continue;
    }
    try {
      await client.sendDM({ peer_id: u.peer_id, message: text });
    } catch (err) {
      errors.push(new Error(`${u.ad}: ${describeError(err)}`));
    }
  }
  return errors;
}

/** ayrilBildir — üyenin ayrıldığını kurucuya ve tüm yöneticilere bildirir. */
export async function sendLeaveNotice(client: LiveClient, g: Grup, self: Uye): Promise<Error[]> {
  const payload: LeavePayload = { uye: self };
  const text = encodeEnvelope({ tip: "ayril", grup_id: g.id, payload });
  const targets: Uye[] = [{ ad: g.kurucu_ad, peer_id: g.kurucu_peer }, ...(g.yoneticiler ?? [])];
  const errors: Error[] = [];
  for (const h of targets) {
    if (!h.peer_id || h.peer_id === self.peer_id) {
      continue;
    }
    try {
      await client.sendDM({ peer_id: h.peer_id, message: text });
    } catch (err) {
      errors.push(new Error(`${h.ad}: ${describeError(err)}`));
    }
  }
  return errors;
}

/** grupKapatBildir — grubun kapatıldığını kendisi hariç tüm üyelere bildirir. */
export async function sendCloseNotice(client: LiveClient, g: Grup, self: Uye): Promise<Error[]> {
  const payload: CloseGroupPayload = { gonderen: self };
  const text = encodeEnvelope({ tip: "grup_kapat", grup_id: g.id, payload });
  const errors: Error[] = [];
  for (const u of g.uyeler) {
    if (u.peer_id === self.peer_id) {
      continue;
    }
    try {
      await client.sendDM({ peer_id: u.peer_id, message: text });
    } catch (err) {
      errors.push(new Error(`${u.ad}: ${describeError(err)}`));
    }
  }
  return errors;
}

// ─── Gönder (push) / Çek (pull) ─────────────────────────────────────────────

/**
 * pushYap — aktif daldaki yeni nesneleri hedef üyeye şifreli paket olarak
 * gönderir ve ardından bir bildirim DM'i yollar. knownHeadAtTarget boşsa tam
 * paket, doluysa yalnızca o commit'ten sonraki (ata zincirindeki) nesneler
 * hariç tutulur.
 */
export async function push(
  client: LiveClient,
  root: string,
  g: Grup,
  p: Proje,
  targetMember: Uye,
  knownHeadAtTarget: string
): Promise<number> {
  const head = p.dallar[p.aktif_dal];
  if (!head) {
    throw new Error("önce en az bir kayıt (commit) oluşturmalısın");
  }
  const known = new Set<string>();
  if (knownHeadAtTarget) {
    for (const h of collectAncestors(root, p.id, knownHeadAtTarget)) {
      known.add(h);
    }
    known.add(knownHeadAtTarget);
  }
  const { data, objectCount } = buildPack(root, p.id, p.aktif_dal, head, known);
  if (objectCount === 0) {
    throw new Error(`${targetMember.ad} zaten güncel`);
  }
  const sealed = encrypt(g.sir, data);

  const tmpDir = path.join(os.tmpdir(), "and-deltax");
  fs.mkdirSync(tmpDir, { recursive: true, mode: 0o700 });
  const fileName = packFileName(g.id, p.id);
  const filePath = path.join(tmpDir, fileName);
  fs.writeFileSync(filePath, sealed, { mode: 0o600 });
  try {
    await client.sendFile({ peer_id: targetMember.peer_id, local_path: filePath });
  } finally {
    fs.rmSync(filePath, { force: true });
  }

  const noticePayload: PushNoticePayload = { dal: p.aktif_dal, head_hash: head, nesne_sayisi: objectCount, dosya_adi: fileName };
  const text = encodeEnvelope({ tip: "push_bildir", grup_id: g.id, proje_id: p.id, payload: noticePayload });
  await client.sendDM({ peer_id: targetMember.peer_id, message: text });
  return objectCount;
}

/** pullIste */
export async function requestPull(client: LiveClient, target: Uye, g: Grup, p: Proje, self: Uye): Promise<void> {
  const payload: PullRequestPayload = { dal: p.aktif_dal, bilinen_hash: p.dallar[p.aktif_dal] ?? "", isteyen: self };
  const text = encodeEnvelope({ tip: "pull_istek", grup_id: g.id, proje_id: p.id, payload });
  await client.sendDM({ peer_id: target.peer_id, message: text });
}
