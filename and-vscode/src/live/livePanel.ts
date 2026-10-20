import * as vscode from "vscode";
import { BackgroundSync } from "./backgroundSync";
import { trackDisposable } from "../disposableBag";
import { LiveClient } from "./liveClient";
import { LiveConnection } from "./liveConnection";
import { ChatMsg, RoleInfo, SitePageInfo } from "./liveTypes";
import { renderHtml, resolveThemeColors } from "./panelHtml";
import { SiteFileSystem } from "./site/siteFileSystem";
import { SITE_MAX_BODY_BYTES, SITE_MAX_PAGES, utf8Bytes } from "./site/sitePage";

let panel: vscode.WebviewPanel | undefined;

/** Sohbet sekmesinde gösterilecek en fazla geçmiş mesaj — kalıcı geçmişin tamamı binlerce mesaj olabilir. */
const CHAT_HISTORY_LIMIT = 200;

/** Panel açıkken AND temasının ne sıklıkla yoklandığı — TUI'de seçilen tema panele bu kadar gecikmeyle yansır. */
const THEME_POLL_MS = 5000;

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
  context: vscode.ExtensionContext
): void {
  if (panel) {
    panel.reveal();
    return;
  }

  panel = vscode.window.createWebviewPanel("andLive", "AND Ekosistemi", vscode.ViewColumn.Beside, {
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

  const syncSubs = [
    sync.onChatMsg((m) => void thePanel.webview.postMessage({ type: "chatMsg", data: m })),
    sync.onDMMsg((m) => void thePanel.webview.postMessage({ type: "dmMsg", data: m })),
    sync.onFileMsg((m) => void thePanel.webview.postMessage({ type: "fileMsg", data: m })),
    sync.onConsentReq((m) => void thePanel.webview.postMessage({ type: "consentReq", data: m })),
    siteFs.onDidChangePages((pages) => void thePanel.webview.postMessage({ type: "sitePages", data: sitePagesForWebview(pages) })),
  ];

  thePanel.webview.onDidReceiveMessage(async (msg: { type: string; [k: string]: unknown }) => {
    if (msg.type === "copyText") {
      // AND bağlantısı gerektirmez — kimlik/eş bilgisi panelde zaten görünüyor.
      await vscode.env.clipboard.writeText(String(msg.text ?? ""));
      void vscode.window.showInformationMessage(`${String(msg.label ?? "Metin")} panoya kopyalandı.`);
      return;
    }
    const client = conn.getClient();
    if (!client && msg.type !== "refreshAll") {
      void thePanel.webview.postMessage({ type: "error", message: "AND'a canlı bağlantı yok." });
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
          await client!.sendDM({ peer_id: String(msg.peerId), message: String(msg.message) });
          return;
        }
        case "pickFileToSend": {
          const peerId = String(msg.peerId ?? "").trim();
          if (!peerId) {
            void thePanel.webview.postMessage({ type: "error", message: "Önce hedef Peer ID girin." });
            return;
          }
          const picked = await vscode.window.showOpenDialog({
            canSelectFiles: true,
            canSelectFolders: false,
            canSelectMany: false,
            openLabel: "Gönder",
          });
          if (!picked || picked.length === 0) {
            return;
          }
          await client!.sendFile({ peer_id: peerId, local_path: picked[0].fsPath });
          void thePanel.webview.postMessage({ type: "info", message: `Gönderiliyor: ${picked[0].fsPath}` });
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
          void thePanel.webview.postMessage({ type: "info", message: "Gönderi oluşturuldu ve ağa yayınlandı." });
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
          void thePanel.webview.postMessage({ type: "info", message: "İşlem tamamlandı." });
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
    disposeSub.dispose();
    syncSubs.forEach((s) => s.dispose());
    forgetPanel();
    panel = undefined;
  });

  void thePanel.webview.postMessage({ type: "connectionState", connected: conn.isConnected() });
}
