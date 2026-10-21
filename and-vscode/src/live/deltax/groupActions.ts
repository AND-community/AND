import * as vscode from "vscode";
import { pickGroup } from "../../commands";
import type { DeltaXExtension } from "../../extension";
import { createGroup, deleteGroup, readGroup, saveGroup } from "../../groupProjectStore";
import { GroupItem, ProjectItem } from "../../groupsTreeView";
import { listAllProjects } from "../../projectLocator";
import { Grup, Proje, Uye } from "../../types";
import { gecerliPeerID } from "../dmCore";
import { requireClient } from "../editorActions";
import { LiveClient } from "../liveClient";
import { PeerInfo } from "../liveTypes";
import { inviteCandidates, isAdmin, isAuthorized, isFounder, isMember, setAdmin } from "./groupOps";
import { fetchSelfUye } from "./identity";
import { broadcastRoster, push, requestPull, sendCloseNotice, sendInvite, sendLeaveNotice } from "./protocol";
import { davetEden } from "./zarfIsleyici";

// keys.go'daki grup paneli komutlarının (i üye davet et / y yönetici ata /
// x gruptan çık-kapat / a-enter Gönder / Çek İste) VS Code komut karşılığı.

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/** Davet edilecek Peer ID'deki sorun; sorun yoksa undefined. */
function inviteInputError(peerId: string, grup: Grup, selfPeer: string): string | undefined {
  if (!peerId) {
    return "Peer ID boş olamaz";
  }
  if (!gecerliPeerID(peerId)) {
    return "Bu bir Peer ID değil (12D3KooW… ile başlar)";
  }
  if (peerId === selfPeer) {
    return "Kendini davet edemezsin";
  }
  if (isMember(grup, peerId)) {
    return "Bu kişi zaten grubun üyesi";
  }
  return undefined;
}

interface MemberPick extends vscode.QuickPickItem {
  uye?: Uye;
}

async function resolveProjectAndGroup(ext: DeltaXExtension, item?: unknown): Promise<{ grup: Grup; proje: Proje } | undefined> {
  if (item instanceof ProjectItem) {
    try {
      return { grup: readGroup(ext.root, item.proje.grup_id), proje: item.proje };
    } catch {
      void vscode.window.showErrorMessage("Bu projenin grubu okunamadı.");
      return undefined;
    }
  }
  if (ext.repo) {
    try {
      return { grup: readGroup(ext.root, ext.repo.proje.grup_id), proje: ext.repo.proje };
    } catch {
      // düşün ve aşağıdaki seçiciye geç
    }
  }
  const secim = await vscode.window.showQuickPick(
    listAllProjects(ext.root).map((gp) => ({ label: gp.proje.ad, description: gp.grup.ad, gp })),
    { placeHolder: "Proje seçin" }
  );
  return secim ? { grup: secim.gp.grup, proje: secim.gp.proje } : undefined;
}

async function onlineGroupMembers(client: LiveClient, grup: Grup, excludePeerId: string | undefined): Promise<Uye[]> {
  let online: Set<string>;
  try {
    online = new Set((await client.getPeers()).map((p) => p.peer_id));
  } catch {
    online = new Set();
  }
  return grup.uyeler.filter((u) => u.peer_id !== excludePeerId && online.has(u.peer_id));
}

export function registerGroupActions(context: vscode.ExtensionContext, ext: DeltaXExtension): void {
  const reg = (id: string, handler: (...args: unknown[]) => unknown): void => {
    context.subscriptions.push(vscode.commands.registerCommand(id, handler));
  };

  reg("deltax.createGroup", async () => {
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    if (!self) {
      return;
    }
    const ad = await vscode.window.showInputBox({
      prompt: "Yeni grup adı",
      validateInput: (v) => (v.trim() ? undefined : "Grup adı boş olamaz"),
    });
    if (!ad) {
      return;
    }
    const grup = createGroup(ext.root, ad.trim(), self);
    ext.treeProvider.refresh();
    void vscode.window.showInformationMessage(`Grup oluşturuldu: ${grup.ad}`);
  });

  reg("deltax.inviteMember", async (item?: unknown) => {
    const grup = item instanceof GroupItem ? item.grup : await pickGroup(ext.root);
    if (!grup) {
      return;
    }
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    if (!self) {
      return;
    }
    if (!isAuthorized(grup, self.peer_id)) {
      void vscode.window.showErrorMessage("Yalnızca kurucu ya da bir yönetici üye davet edebilir.");
      return;
    }
    let peers: PeerInfo[] = [];
    try {
      peers = await client.getPeers();
    } catch {
      // eş listesi alınamadı — elle giriş isteriz
    }
    const candidates = inviteCandidates(peers, grup);
    const MANUAL = "$(edit) Peer ID'yi elle gir…";
    let peerId: string | undefined;
    if (candidates.length > 0) {
      const pick = await vscode.window.showQuickPick(
        [...candidates.map((p) => ({ label: p.peer_id, description: p.addrs[0] ?? "" })), { label: MANUAL, description: "" }],
        { placeHolder: "Davet edilecek eş" }
      );
      if (!pick) {
        return;
      }
      if (pick.label !== MANUAL) {
        peerId = pick.label;
      }
    }
    if (!peerId) {
      peerId = await vscode.window.showInputBox({
        prompt: "Davet edilecek Peer ID (kişi kendi ID'sini AND'ın Kimlik ekranından paylaşabilir)",
        validateInput: (v) => inviteInputError(v.trim(), grup, self.peer_id),
      });
    }
    peerId = peerId?.trim();
    if (!peerId) {
      return;
    }
    const invalid = inviteInputError(peerId, grup, self.peer_id);
    if (invalid) {
      void vscode.window.showErrorMessage(invalid);
      return;
    }
    // Kabul bu kayda göre tanınır; gönderim bitmeden yazılır (bkz. davetGonderiliyor).
    ext.deltaxSync.davetGonderiliyor(grup.id, peerId);
    try {
      await sendInvite(client, peerId, grup, self);
      void vscode.window.showInformationMessage("Davet gönderildi. Kişi çevrimdışıysa AND daveti bekletir, bağlandığında iletir.");
    } catch (err) {
      void vscode.window.showErrorMessage(`Davet gönderilemedi: ${describeError(err)}`);
    }
  });

  reg("deltax.showInvites", async () => {
    const pending = ext.deltaxSync.bekleyenDavetler();
    if (pending.length === 0) {
      void vscode.window.showInformationMessage("Bekleyen davet yok.");
      ext.treeProvider.refresh();
      return;
    }
    const invite = await vscode.window.showQuickPick(
      pending.map((d) => ({
        label: d.payload.grup_ad,
        description: `davet eden: ${davetEden(d)}`,
        detail: `${d.payload.uyeler.length} üye, ${new Date(d.alindi).toLocaleString()} geldi`,
        d,
      })),
      { placeHolder: "Yanıtlanacak davet" }
    );
    if (!invite) {
      return;
    }
    const choice = await vscode.window.showQuickPick(["Kabul et", "Reddet"], {
      placeHolder: `"${invite.d.payload.grup_ad}" daveti`,
    });
    if (choice) {
      await ext.deltaxSync.davetiYanitla(invite.d, choice === "Kabul et");
    }
  });

  reg("deltax.manageAdmins", async (item?: unknown) => {
    const grup = item instanceof GroupItem ? item.grup : await pickGroup(ext.root);
    if (!grup) {
      return;
    }
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    if (!self) {
      return;
    }
    if (!isFounder(grup, self.peer_id)) {
      void vscode.window.showErrorMessage("Yalnızca grubun kurucusu yönetici atayabilir.");
      return;
    }
    const others = grup.uyeler.filter((u) => u.peer_id !== self.peer_id);
    if (others.length === 0) {
      void vscode.window.showInformationMessage("Grupta başka üye yok.");
      return;
    }
    const pick = await vscode.window.showQuickPick(
      others.map((u) => ({
        label: u.ad,
        description: isAdmin(grup, u.peer_id) ? "yönetici — kaldırmak için seç" : "üye — yönetici yapmak için seç",
        uye: u,
      })),
      { placeHolder: "Yönetici durumu değiştirilecek üye" }
    );
    if (!pick) {
      return;
    }
    const makeAdmin = !isAdmin(grup, pick.uye.peer_id);
    setAdmin(grup, pick.uye, makeAdmin);
    saveGroup(ext.root, grup);
    ext.treeProvider.refresh();
    void vscode.window.showInformationMessage(
      makeAdmin ? `${pick.uye.ad} yönetici yapıldı.` : `${pick.uye.ad} yöneticilikten çıkarıldı.`
    );
    const errors = await broadcastRoster(client, grup, self);
    if (errors.length > 0) {
      void vscode.window.showWarningMessage(`${errors.length} üyeye ulaşılamadı.`);
    }
  });

  reg("deltax.leaveOrCloseGroup", async (item?: unknown) => {
    const grup = item instanceof GroupItem ? item.grup : await pickGroup(ext.root);
    if (!grup) {
      return;
    }
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    if (!self) {
      return;
    }
    // DeltaX'teki gibi: kurucu ya da bir yönetici grubu kapatır, sıradan üye ayrılır.
    const closes = isAuthorized(grup, self.peer_id);
    const warning = closes
      ? `"${grup.ad}" grubunu kapatmak üzeresin. Tüm üyelere bildirilir ve grup kalıcı olarak kapanır.`
      : `"${grup.ad}" grubundan ayrılmak üzeresin. Kurucuya ve yöneticilere bildirilir, üye listesinden çıkarılırsın.`;
    const confirmLabel = closes ? "Grubu Kapat" : "Gruptan Çık";
    const choice = await vscode.window.showWarningMessage(warning, { modal: true }, confirmLabel);
    if (choice !== confirmLabel) {
      return;
    }
    const errors = closes ? await sendCloseNotice(client, grup, self) : await sendLeaveNotice(client, grup, self);
    deleteGroup(ext.root, grup.id);
    ext.treeProvider.refresh();
    if (errors.length > 0) {
      void vscode.window.showWarningMessage(
        `${closes ? "Grup kapatıldı" : "Gruptan ayrıldın"}, ancak ${errors.length} üyeye bildirim ulaşmadı.`
      );
    } else {
      void vscode.window.showInformationMessage(closes ? "Grup kapatıldı." : "Gruptan ayrıldın.");
    }
  });

  reg("deltax.pushProject", async (item?: unknown) => {
    const resolved = await resolveProjectAndGroup(ext, item);
    if (!resolved) {
      return;
    }
    const { grup, proje } = resolved;
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    const candidates = await onlineGroupMembers(client, grup, self?.peer_id);
    if (candidates.length === 0) {
      void vscode.window.showWarningMessage("Çevrimiçi üye yok.");
      return;
    }
    const ALL = "$(broadcast) Çevrimiçi herkese gönder";
    const items: MemberPick[] = [
      { label: ALL, description: `${candidates.length} üye` },
      ...candidates.map((u) => ({ label: u.ad, description: u.peer_id, uye: u })),
    ];
    const pick = await vscode.window.showQuickPick(items, { placeHolder: "Hedef üye" });
    if (!pick) {
      return;
    }
    const targets = pick.uye ? [pick.uye] : candidates;
    let ok = 0;
    const errors: string[] = [];
    for (const target of targets) {
      const known = proje.uzaklar?.[`${target.peer_id}/${proje.aktif_dal}`] ?? "";
      try {
        await push(client, ext.root, grup, proje, target, known);
        ok++;
      } catch (err) {
        errors.push(`${target.ad}: ${describeError(err)}`);
      }
    }
    if (ok > 0) {
      void vscode.window.showInformationMessage(`${ok} üyeye gönderildi.`);
    }
    if (errors.length > 0) {
      void vscode.window.showErrorMessage(`Gönderilemedi: ${errors.join("; ")}`);
    }
  });

  reg("deltax.pullFromMember", async (item?: unknown) => {
    const resolved = await resolveProjectAndGroup(ext, item);
    if (!resolved) {
      return;
    }
    const { grup, proje } = resolved;
    const client = await requireClient(ext.liveConnection);
    if (!client) {
      return;
    }
    const self = await fetchSelfUye(client);
    if (!self) {
      return;
    }
    const candidates = await onlineGroupMembers(client, grup, self.peer_id);
    if (candidates.length === 0) {
      void vscode.window.showWarningMessage("Çevrimiçi üye yok.");
      return;
    }
    const pick = await vscode.window.showQuickPick(
      candidates.map((u) => ({ label: u.ad, description: u.peer_id, uye: u })),
      { placeHolder: "Kimden çekilsin?" }
    );
    if (!pick) {
      return;
    }
    try {
      await requestPull(client, pick.uye, grup, proje, self);
      void vscode.window.showInformationMessage(`Çek isteği gönderildi: ${pick.uye.ad}`);
    } catch (err) {
      void vscode.window.showErrorMessage(`İstek gönderilemedi: ${describeError(err)}`);
    }
  });
}
