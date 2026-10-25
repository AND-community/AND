import * as vscode from "vscode";
import { BackgroundSync } from "./backgroundSync";
import { trackDisposable } from "../disposableBag";
import { gecerliPeerID } from "./dmCore";
import { DmMerkezi } from "./dmMerkezi";
import { LiveClient } from "./liveClient";
import { LiveConnection } from "./liveConnection";
import { ChatMsg, RoleInfo, SitePageInfo } from "./liveTypes";
import { renderHtml, resolveThemeColors } from "./panelHtml";
import { SiteFileSystem } from "./site/siteFileSystem";
import { SITE_MAX_BODY_BYTES, SITE_MAX_PAGES, utf8Bytes } from "./site/sitePage";
import { t } from "../dil/dil";

let panel: vscode.WebviewPanel | undefined;

/** Sohbet sekmesinde gösterilecek en fazla geçmiş mesaj — kalıcı geçmişin tamamı binlerce mesaj olabilir. */
const CHAT_HISTORY_LIMIT = 200;

/** Panel açıkken AND temasının ne sıklıkla yoklandığı — TUI'de seçilen tema panele bu kadar gecikmeyle yansır. */
const THEME_POLL_MS = 5000;

/** Panelin DM verisine erişimi (extension.ts sağlar). */
export interface PanelDM {
  merkez: DmMerkezi;
  /** DM'ler değişti: yeni mesaj, gönderim ya da okundu işareti. */
  onDidChange: vscode.Event<void>;
  /** Panel bir değişiklik yaptı (gönderim, okundu): rozet ve diğer dinleyiciler güncellensin. */
  changed(): void;
}

function recentChat(all: ChatMsg[]): ChatMsg[] {
  return [...all]
    .sort((a, b) => (Date.parse(a.sent_at) || 0) - (Date.parse(b.sent_at) || 0))
    .slice(-CHAT_HISTORY_LIMIT);
}

/** Site sekmesi için sayfa listesi — tam gövde yerine bayt sayısı ve kısa bir önizleme gönderilir. */
function sitePagesForWebview(pages: SitePageInfo[]): unknown {
  return {
    max: SITE_MAX_PAGES,
    maxBody: SITE_MAX_BODY_BYTES,
    pages: pages.map((p) => ({
      slug: p.slug,
      title: p.title,
      updated_at: p.updated_at,
      bytes: utf8Bytes(p.body),
      preview: p.body.length > 140 ? `${p.body.slice(0, 140)}…` : p.body,
    })),
  };
}

/** Dil değişti: açık panel etkin dilde yeniden kurulur (webview yeniden yüklenip verisini baştan ister). */
export function redrawLivePanel(): void {
  if (panel) {
    panel.title = t("AND Ekosistemi");
    panel.webview.html = renderHtml(panel.webview);
  }
}

/** Canlı Panel şu an açık ve görünür mü (arka planda bildirim/rozet mantığı için). */
export function isLivePanelVisible(): boolean {
  return !!panel && panel.visible;
}


function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function showLivePanel(
  conn: LiveConnection,
  sync: BackgroundSync,
  siteFs: SiteFileSystem,
  context: vscode.ExtensionContext,
  dm: PanelDM
): void {
  if (panel) {
    panel.reveal();
    return;
  }

  panel = vscode.window.createWebviewPanel("andLive", t("AND Ekosistemi"), vscode.ViewColumn.Beside, {
    enableScripts: true,
    retainContextWhenHidden: true,
  });
  const thePanel = panel;
  const forgetPanel = trackDisposable(context, thePanel);
  let currentRole: RoleInfo | undefined;

  /** AND'ın forum.go'sunda onay kuyruğu yoktur — gönderiler oluşturulduğu anda
   * yayınlanır; kurucu/moderatörün tek yetkisi mevcut bir konuyu silmektir
   * (bkz. AND README "Moderasyon"). Bu yüzden yalnızca getAllPosts vardır. */
  async function refreshForumModIfAuthorized(): Promise<void> {
    if (!currentRole?.is_founder && !currentRole?.is_moderator) {
      return;
    }
    const c = conn.getClient();
    if (!c) {
      return;
    }
    const all = await c.getAllPosts();
    void thePanel.webview.postMessage({ type: "forumMod", all });
  }

  thePanel.webview.html = renderHtml(thePanel.webview);

  const disposeSub = conn.onDidChangeConnection((connected) => {
    void thePanel.webview.postMessage({ type: "connectionState", connected });
  });
  // AND'da tema değişirse (TUI'de Ayarlar → Temalar ya da bir tema eklentisi)
  // panel de Yenile beklemeden değişsin: panel görünürken tema birkaç saniyede
  // bir yoklanır ve yalnızca değiştiyse webview'a gönderilir.
  let lastThemeKey = "";
  async function pushTheme(c: LiveClient, force = false): Promise<void> {
    const colors = resolveThemeColors(await c.getTheme());
    const key = JSON.stringify(colors);
    if (!force && key === lastThemeKey) {
      return;
    }
    lastThemeKey = key;
    void thePanel.webview.postMessage({ type: "theme", data: colors });
  }
  const themeTimer = setInterval(() => {
    const c = conn.getClient();
    if (c && thePanel.visible) {
      pushTheme(c).catch(() => {
        // Eski bir AND'da /theme yoksa ya da bağlantı koptuysa bir sonraki turda denenir.
      });
    }
  }, THEME_POLL_MS);

  // Doğrudan mesaj sekmesi AND'ın DM geçmişinden kurulur (TUI ve Özel Chat ile
  // aynı konuşmalar ve okunmamış sayıları). Giden kutusunda bekleyen mesaj
  // varken panel görünürse durumu (iletildi/gönderilemedi) birkaç saniyede bir
  // yenilenir.
  let dmPending = false;
  async function pushDM(): Promise<void> {
    const c = conn.getClient();
    if (!c) {
      return;
    }
    const d = await dm.merkez.durum(c);
    dmPending = d.bekleyenVar;
    void thePanel.webview.postMessage({ type: "dmData", data: { konusmalar: d.konusmalar, gecmisYok: d.gecmisYok } });
  }
  const dmTimer = setInterval(() => {
    if (dmPending && thePanel.visible) {
      pushDM().catch(() => undefined);
    }
  }, THEME_POLL_MS);

  const syncSubs = [
    sync.onChatMsg((m) => void thePanel.webview.postMessage({ type: "chatMsg", data: m })),
    dm.onDidChange(() => void pushDM().catch(() => undefined)),
    sync.onFileMsg((m) => void thePanel.webview.postMessage({ type: "fileMsg", data: m })),
    sync.onConsentReq((m) => void thePanel.webview.postMessage({ type: "consentReq", data: m })),
    siteFs.onDidChangePages((pages) => void thePanel.webview.postMessage({ type: "sitePages", data: sitePagesForWebview(pages) })),
  ];

  thePanel.webview.onDidReceiveMessage(async (msg: { type: string; [k: string]: unknown }) => {
    if (msg.type === "copyText") {
      // AND bağlantısı gerektirmez — kimlik/eş bilgisi panelde zaten görünüyor.
      await vscode.env.clipboard.writeText(String(msg.text ?? ""));
      void vscode.window.showInformationMessage(t("%s panoya kopyalandı.", String(msg.label ?? t("Metin"))));
      return;
    }
    const client = conn.getClient();
    if (!client && msg.type !== "refreshAll") {
      void thePanel.webview.postMessage({ type: "error", message: t("AND'a canlı bağlantı yok.") });
      return;
    }
    try {
      switch (msg.type) {
        case "refreshAll": {
          const c = await conn.ensureConnected();
          void thePanel.webview.postMessage({ type: "connectionState", connected: !!c });
          if (!c) {
            return;
          }
          const [identity, role, peers] = await Promise.all([c.getIdentity(), c.getRole(), c.getPeers()]);
          currentRole = role;
          void thePanel.webview.postMessage({ type: "identity", data: identity });
          void thePanel.webview.postMessage({ type: "role", data: role });
          void thePanel.webview.postMessage({ type: "peers", data: peers });
          if (role.is_founder || role.is_moderator) {
            const all = await c.getAllPosts();
            void thePanel.webview.postMessage({ type: "forumMod", all });
          }
          try {
            await pushTheme(c, true);
          } catch {
            // AND teması okunamadıysa VS Code'un kendi temasıyla devam edilir.
          }
          try {
            const history = await c.getAllChat();
            void thePanel.webview.postMessage({ type: "chatHistory", data: recentChat(history) });
          } catch {
            // /chat/all'ı olmayan eski bir AND'da sohbet yalnızca canlı akışla dolar.
          }
          try {
            await siteFs.listPages(true); // sonuç onDidChangePages → "sitePages" ile gelir
          } catch (err) {
            void thePanel.webview.postMessage({ type: "siteUnavailable", message: describeError(err) });
          }
          try {
            await pushDM();
          } catch (err) {
            void thePanel.webview.postMessage({ type: "error", message: t("Mesajlar alınamadı: %s", describeError(err)) });
          }
          return;
        }
        case "siteRefresh": {
          await siteFs.listPages(true);
          return;
        }
        case "siteNew": {
          await vscode.commands.executeCommand("and.site.newPage");
          return;
        }
        case "siteOpen": {
          await vscode.commands.executeCommand("and.site.openPage", String(msg.slug));
          return;
        }
        case "siteCopyCode": {
          await vscode.commands.executeCommand("and.site.copyShareCode", String(msg.slug));
          return;
        }
        case "siteDelete": {
          await vscode.commands.executeCommand("and.site.deletePage", String(msg.slug));
          return;
        }
        case "sendChat": {
          await client!.sendChat(String(msg.message));
          return;
        }
        case "sendDM": {
          const peerId = String(msg.peerId ?? "").trim();
          if (!gecerliPeerID(peerId)) {
            void thePanel.webview.postMessage({ type: "error", message: t("Bu bir Peer ID değil (12D3KooW… ile başlar).") });
            return;
          }
          await dm.merkez.gonder(client!, peerId, String(msg.message));
          dm.changed();
          return;
        }
        case "dmOpen": {
          // Yalnızca kullanıcı konuşmayı gerçekten görüyorsa okundu sayılır.
          if (thePanel.visible && (await dm.merkez.okunduIsaretle(client!, String(msg.peerId)))) {
            dm.changed();
          }
          return;
        }
        case "pickFileToSend": {
          const peerId = String(msg.peerId ?? "").trim();
          if (!peerId) {
            void thePanel.webview.postMessage({ type: "error", message: t("Önce hedef Peer ID girin.") });
            return;
          }
          const picked = await vscode.window.showOpenDialog({
            canSelectFiles: true,
            canSelectFolders: false,
            canSelectMany: false,
            openLabel: t("Gönder"),
          });
          if (!picked || picked.length === 0) {
            return;
          }
          await client!.sendFile({ peer_id: peerId, local_path: picked[0].fsPath });
          void thePanel.webview.postMessage({ type: "info", message: t("Gönderiliyor: %s", picked[0].fsPath) });
          return;
        }
        case "respondConsent": {
          const transferId = String(msg.transferId);
          await client!.respondConsent({ transfer_id: transferId, accept: Boolean(msg.accept) });
          void thePanel.webview.postMessage({ type: "consentResolved", transferId });
          return;
        }
        case "revealFile": {
          await vscode.commands.executeCommand("revealFileInOS", vscode.Uri.file(String(msg.path)));
          return;
        }
        case "createPost": {
          await client!.createPost({
            category: String(msg.category),
            title: String(msg.title),
            body: String(msg.body),
          });
          void thePanel.webview.postMessage({ type: "info", message: t("Gönderi oluşturuldu ve ağa yayınlandı.") });
          await refreshForumModIfAuthorized();
          return;
        }
        case "viewPost": {
          const detail = await client!.getPost(String(msg.id));
          void thePanel.webview.postMessage({ type: "postDetail", data: detail });
          return;
        }
        case "reply": {
          await client!.createReply({ post_id: String(msg.postId), body: String(msg.body) });
          const detail = await client!.getPost(String(msg.postId));
          void thePanel.webview.postMessage({ type: "postDetail", data: detail });
          await refreshForumModIfAuthorized();
          return;
        }
        case "deletePost": {
          await client!.deletePost(String(msg.postId));
          void thePanel.webview.postMessage({ type: "info", message: t("İşlem tamamlandı.") });
          await refreshForumModIfAuthorized();
          return;
        }
        default:
          return;
      }
    } catch (err) {
      void thePanel.webview.postMessage({ type: "error", message: describeError(err) });
    }
  });

  thePanel.onDidDispose(() => {
    clearInterval(themeTimer);
    clearInterval(dmTimer);
    disposeSub.dispose();
    syncSubs.forEach((s) => s.dispose());
    forgetPanel();
    panel = undefined;
  });

  void thePanel.webview.postMessage({ type: "connectionState", connected: conn.isConnected() });
}
