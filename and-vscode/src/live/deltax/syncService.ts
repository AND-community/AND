import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";
import { deleteGroup, readGroup, readProject, saveGroup, saveProject } from "../../groupProjectStore";
import { Grup, Uye } from "../../types";
import { LiveConnection } from "../liveConnection";
import { DMMsg, FileConsentReq, FileMsg } from "../liveTypes";
import { isAuthorized, isMember, removeMember, setAdmin, upsertMember } from "./groupOps";
import { fetchSelfUye } from "./identity";
import { applyIncomingPackFile, packSenderTrusted } from "./packSync";
import {
  CloseGroupPayload,
  decodeEnvelope,
  Envelope,
  InvitePayload,
  InviteResponsePayload,
  LeavePayload,
  PullRequestPayload,
  PushNoticePayload,
  broadcastRoster,
  push,
  sendInviteResponse,
} from "./protocol";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * model.go'daki zarfIsle + filePollResultMsg/consentPollResultMsg
 * işleyicilerinin karşılığı. extension.ts, BackgroundSync'ten aldığı her
 * DM/dosya/onay olayını ÖNCE buraya iletir; handleDM/handleConsent bir
 * protokol mesajını tükettiyse `true` döner ve extension.ts genel
 * bildirim/Canlı Panel akışını atlar (davet/roster/push gibi iç mesajlar
 * kullanıcıya ham DM olarak hiç görünmez).
 */
export class DeltaXSyncService implements vscode.Disposable {
  private readonly groupsChangedEmitter = new vscode.EventEmitter<void>();
  readonly onGroupsChanged = this.groupsChangedEmitter.event;

  // model.go'daki gonderilenDavetler'in (grupId -> davet gönderdiğimiz
  // peerId kümesi) birebir karşılığı — bkz. onInviteResponse'un doc yorumu:
  // davet edilmemiş biri, kendi gerçek kimliğiyle "kabul ediyorum" diyerek
  // asla gruba giremesin diye, yalnızca GERÇEKTEN davet gönderdiğimiz bir
  // (grup, peer) çifti için gelen bir "kabul" işlenir.
  private readonly sentInvites = new Map<string, Set<string>>();

  constructor(private readonly getRoot: () => string, private readonly conn: LiveConnection) {}

  /** groupActions.ts'teki sendInvite çağrısından hemen sonra çağrılmalı. */
  recordSentInvite(grupId: string, peerId: string): void {
    let set = this.sentInvites.get(grupId);
    if (!set) {
      set = new Set();
      this.sentInvites.set(grupId, set);
    }
    set.add(peerId);
  }

  dispose(): void {
    this.groupsChangedEmitter.dispose();
  }

  private notify(message: string, isError = false): void {
    if (isError) {
      void vscode.window.showErrorMessage(`AND — ${message}`);
    } else {
      void vscode.window.showInformationMessage(`AND — ${message}`);
    }
  }

  private async getSelfUye(): Promise<Uye | undefined> {
    const client = this.conn.getClient();
    return client ? fetchSelfUye(client) : undefined;
  }

  /** true dönerse mesaj tüketildi (genel DM bildirimi/Canlı Panel'e hiç düşmemeli). */
  async handleDM(msg: DMMsg): Promise<boolean> {
    const z = decodeEnvelope(msg.text);
    if (!z) {
      return false;
    }
    try {
      await this.processEnvelope(msg.from, z);
    } catch {
      // sync.go'daki zarfIsle de bozuk/beklenmeyen payload'ları sessizce yutar
    }
    return true;
  }

  /** projeag- önekli dosyalar dışındaki her şeyi olduğu gibi bırakır (Canlı Panel'in Dosyalar sekmesi kendi başına yönetir). */
  async handleFile(msg: FileMsg): Promise<void> {
    if (!path.basename(msg.filename).startsWith("projeag-")) {
      return;
    }
    const root = this.getRoot();
    try {
      const applied = applyIncomingPackFile(root, msg.save_path, msg.from);
      try {
        const p = readProject(root, applied.projeId);
        if (!p.uzaklar) {
          p.uzaklar = {};
        }
        if (applied.manifest.head) {
          p.uzaklar[`${msg.from}/${applied.manifest.dal}`] = applied.manifest.head;
        }
        saveProject(root, p);
      } catch {
        // proje okunamadı — es geç, nesneler yine de yazıldı
      }
      this.notify(`${applied.objectCount} yeni nesne alındı (${applied.manifest.dal}) — Uzak Dallar'dan birleştirebilirsin.`);
      this.groupsChangedEmitter.fire();
    } catch (err) {
      this.notify(`Paket işlenemedi: ${describeError(err)}`, true);
    } finally {
      fs.rmSync(msg.save_path, { force: true });
    }
  }

  /** true dönerse istek tüketildi (otomatik kabul edildi — genel onay bildirimi atlanmalı). */
  async handleConsent(req: FileConsentReq): Promise<boolean> {
    if (!req.filename.startsWith("projeag-")) {
      return false;
    }
    if (!packSenderTrusted(this.getRoot(), req.filename, req.sender_id)) {
      return false; // bilinmeyen gönderen — genel onay akışına düşsün
    }
    const client = this.conn.getClient();
    if (!client) {
      return false;
    }
    try {
      await client.respondConsent({ transfer_id: req.transfer_id, accept: true });
    } catch {
      // yok say
    }
    return true;
  }

  private async processEnvelope(senderPeer: string, z: Envelope): Promise<void> {
    switch (z.tip) {
      case "davet":
      case "uye_guncelle":
        await this.onInviteOrRoster(senderPeer, z);
        return;
      case "davet_yanit":
        await this.onInviteResponse(senderPeer, z);
        return;
      case "push_bildir":
        this.onPushNotice(z);
        return;
      case "pull_istek":
        await this.onPullRequest(senderPeer, z);
        return;
      case "ayril":
        await this.onLeave(senderPeer, z);
        return;
      case "grup_kapat":
        this.onCloseGroup(senderPeer, z);
        return;
    }
  }

  private async onInviteOrRoster(senderPeer: string, z: Envelope): Promise<void> {
    const dp = z.payload as InvitePayload;
    if (!z.grup_id || !Array.isArray(dp.uyeler) || !dp.gonderen?.peer_id) {
      return;
    }
    // DM API göndereni doğrulamadığından, payload'ın kendi içindeki
    // "gonderen" alanı herkes tarafından uydurulabilir — bu yüzden bu alan
    // yalnızca metadata/görünüm amaçlıdır ve GERÇEK yetki kararı her zaman
    // taşıyıcının (transport) verdiği senderPeer'a dayanmalıdır (model.go'daki
    // zarfIsle'ın "verifiedSender" kontrolünün birebir karşılığı). Bu kontrol
    // olmadan, bilinen bir yönetici/kurucunun peer ID'sini payload'a yazan
    // HERHANGİ BİR peer, mevcut bir grubun tüm üye listesini (ve dolaylı
    // olarak sonraki bir roster yayınında grup sırrını) devralabilirdi.
    if (dp.gonderen.peer_id !== senderPeer) {
      return;
    }
    const root = this.getRoot();
    let existing: Grup | undefined;
    try {
      existing = readGroup(root, z.grup_id);
    } catch {
      existing = undefined;
    }
    if (existing) {
      // Zaten bildiğimiz bir grup için gelen güncelleme yalnızca zaten
      // güvendiğimiz bir yetkilinin peer ID'siyle eşleşiyorsa kabul edilir —
      // DM API göndereni doğrulamadığından bu kontrol atlanamaz.
      if (!isAuthorized(existing, dp.gonderen.peer_id)) {
        return;
      }
      existing.uyeler = dp.uyeler;
      existing.yoneticiler = dp.yoneticiler;
      existing.ad = dp.grup_ad;
      saveGroup(root, existing);
      this.notify(`"${dp.grup_ad}" grubunun üye listesi güncellendi.`);
      this.groupsChangedEmitter.fire();
      return;
    }
    if (z.tip === "uye_guncelle") {
      return; // bilinmeyen grup için roster güncellemesi yok sayılır
    }
    this.promptInvite(z.grup_id, senderPeer, dp);
  }

  private promptInvite(grupId: string, senderPeer: string, dp: InvitePayload): void {
    void vscode.window
      .showInformationMessage(`Yeni grup daveti: ${dp.grup_ad} (davet eden: ${dp.gonderen.ad})`, "Kabul Et", "Reddet")
      .then(async (choice) => {
        if (choice !== "Kabul Et" && choice !== "Reddet") {
          return;
        }
        const client = this.conn.getClient();
        const self = await this.getSelfUye();
        if (!client || !self) {
          void vscode.window.showErrorMessage("AND'a canlı bağlantı yok.");
          return;
        }
        if (choice === "Reddet") {
          await sendInviteResponse(client, senderPeer, grupId, false, self).catch(() => {});
          return;
        }
        const root = this.getRoot();
        const g: Grup = {
          id: grupId,
          ad: dp.grup_ad,
          sir: dp.sir,
          kurucu_ad: dp.kurucu.ad,
          kurucu_peer: dp.kurucu.peer_id,
          yoneticiler: dp.yoneticiler,
          uyeler: dp.uyeler,
          olusturma_ts: new Date().toISOString(),
        };
        upsertMember(g, self);
        saveGroup(root, g);
        this.groupsChangedEmitter.fire();
        try {
          await sendInviteResponse(client, senderPeer, grupId, true, self);
        } catch (err) {
          void vscode.window.showWarningMessage(`Katılma bildirimi gönderilemedi: ${describeError(err)}`);
        }
      });
  }

  private async onInviteResponse(senderPeer: string, z: Envelope): Promise<void> {
    const yp = z.payload as InviteResponsePayload;
    if (!z.grup_id || !yp.uye?.peer_id || !yp.kabul) {
      return;
    }
    if (yp.uye.peer_id !== senderPeer) {
      return;
    }
    // Bu grup için gerçekten bu peer'a bir davet gönderdiğimizi doğrula —
    // aksi halde davet edilmemiş biri bile, kendi gerçek kimliğiyle "kabul
    // ediyorum" diyerek gruba katılabilirdi (hiçbir zaman gönderilmemiş bir
    // daveti "kabul etmek"). bkz. sentInvites'in doc yorumu.
    if (!this.sentInvites.get(z.grup_id)?.has(senderPeer)) {
      return;
    }
    const root = this.getRoot();
    let g: Grup;
    try {
      g = readGroup(root, z.grup_id);
    } catch {
      return;
    }
    const self = await this.getSelfUye();
    if (!self || !isAuthorized(g, self.peer_id)) {
      return;
    }
    this.sentInvites.get(z.grup_id)?.delete(senderPeer);
    upsertMember(g, yp.uye);
    saveGroup(root, g);
    this.notify(`${yp.uye.ad} "${g.ad}" grubuna katıldı.`);
    this.groupsChangedEmitter.fire();
    const client = this.conn.getClient();
    if (client) {
      await broadcastRoster(client, g, self).catch(() => {});
    }
  }

  private onPushNotice(z: Envelope): void {
    const bp = z.payload as PushNoticePayload;
    if (typeof bp.dal !== "string" || typeof bp.nesne_sayisi !== "number") {
      return;
    }
    this.notify(`Bir üye değişiklik gönderiyor (${bp.dal}, ${bp.nesne_sayisi} nesne) — kabul edilince Uzak Dallar'da görünecek.`);
  }

  private async onPullRequest(senderPeer: string, z: Envelope): Promise<void> {
    const pp = z.payload as PullRequestPayload;
    if (!z.grup_id || !z.proje_id || !pp.isteyen?.peer_id) {
      return;
    }
    if (pp.isteyen.peer_id !== senderPeer) {
      return;
    }
    const root = this.getRoot();
    let g: Grup;
    try {
      g = readGroup(root, z.grup_id);
    } catch {
      return;
    }
    if (!isMember(g, pp.isteyen.peer_id)) {
      return;
    }
    let p;
    try {
      p = readProject(root, z.proje_id);
    } catch {
      return;
    }
    const client = this.conn.getClient();
    if (!client) {
      return;
    }
    try {
      await push(client, root, g, p, pp.isteyen, pp.bilinen_hash ?? "");
    } catch (err) {
      this.notify(`Çek isteği karşılanamadı: ${describeError(err)}`, true);
    }
  }

  private async onLeave(senderPeer: string, z: Envelope): Promise<void> {
    const ap = z.payload as LeavePayload;
    if (!z.grup_id || !ap.uye?.peer_id) {
      return;
    }
    // ayril yalnızca kendi ayrılışını bildirmek için kullanılır — bu yüzden
    // ap.uye, DM'i GERÇEKTEN gönderen peer'la eşleşmelidir; aksi halde
    // herhangi bir peer, başka bir üyeyi keyfi olarak "ayrılmış" gösterip
    // gruptan attırabilirdi.
    if (ap.uye.peer_id !== senderPeer) {
      return;
    }
    const root = this.getRoot();
    let g: Grup;
    try {
      g = readGroup(root, z.grup_id);
    } catch {
      return;
    }
    const self = await this.getSelfUye();
    if (!self || !isAuthorized(g, self.peer_id) || !isMember(g, ap.uye.peer_id)) {
      return;
    }
    removeMember(g, ap.uye.peer_id);
    setAdmin(g, ap.uye, false);
    saveGroup(root, g);
    this.notify(`${ap.uye.ad} "${g.ad}" grubundan ayrıldı.`);
    this.groupsChangedEmitter.fire();
    const client = this.conn.getClient();
    if (client) {
      await broadcastRoster(client, g, self).catch(() => {});
    }
  }

  private onCloseGroup(senderPeer: string, z: Envelope): void {
    const kp = z.payload as CloseGroupPayload;
    if (!z.grup_id || !kp.gonderen?.peer_id) {
      return;
    }
    if (kp.gonderen.peer_id !== senderPeer) {
      return;
    }
    const root = this.getRoot();
    let g: Grup;
    try {
      g = readGroup(root, z.grup_id);
    } catch {
      return;
    }
    if (!isAuthorized(g, kp.gonderen.peer_id)) {
      return;
    }
    deleteGroup(root, g.id);
    this.notify(`"${g.ad}" grubu kurucusu tarafından kapatıldı.`, true);
    this.groupsChangedEmitter.fire();
  }
}
