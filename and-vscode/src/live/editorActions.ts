import * as vscode from "vscode";
import { LiveClient } from "./liveClient";
import { LiveConnection } from "./liveConnection";
import { FORUM_CATEGORIES } from "./liveTypes";
import { t } from "../dil/dil";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function baseName(fsPath: string): string {
  return fsPath.split(/[\\/]/).pop() ?? fsPath;
}

export async function requireClient(conn: LiveConnection): Promise<LiveClient | undefined> {
  const client = await conn.ensureConnected();
  if (!client) {
    void vscode.window.showErrorMessage(
      t("AND'a canlı bağlantı yok. AND açık ve AND_ENABLE_EXTERNAL_API=1 ayarlanmış olmalı.")
    );
  }
  return client;
}

/** Editör/gezgin içinden AND ekosistemine (forum, dosya aktarımı) doğrudan erişen kısayol komutları. */
export function registerEditorActions(context: vscode.ExtensionContext, conn: LiveConnection): void {
  context.subscriptions.push(
    vscode.commands.registerCommand("and.shareSelectionToForum", async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        void vscode.window.showErrorMessage(t("Paylaşılacak açık bir dosya yok."));
        return;
      }
      const text = editor.selection.isEmpty ? editor.document.getText() : editor.document.getText(editor.selection);
      if (!text.trim()) {
        void vscode.window.showErrorMessage(t("Paylaşılacak bir içerik yok."));
        return;
      }
      const client = await requireClient(conn);
      if (!client) {
        return;
      }
      const secilen = await vscode.window.showQuickPick(
        FORUM_CATEGORIES.map((c) => ({ label: t(c), kategori: c })),
        { placeHolder: t("Kategori seçin") }
      );
      if (!secilen) {
        return;
      }
      const category = secilen.kategori;
      const title = await vscode.window.showInputBox({
        prompt: t("Konu başlığı"),
        value: baseName(editor.document.fileName),
        validateInput: (v) => (v.trim() ? undefined : t("Başlık boş olamaz")),
      });
      if (!title) {
        return;
      }
      const body = `\`\`\`${editor.document.languageId}\n${text}\n\`\`\``;
      try {
        await client.createPost({ category, title: title.trim(), body });
        const panelAc = t("Canlı Paneli Aç");
        const choice = await vscode.window.showInformationMessage(t("Gönderi oluşturuldu ve ağa yayınlandı."), panelAc);
        if (choice === panelAc) {
          void vscode.commands.executeCommand("and.openLivePanel");
        }
      } catch (err) {
        void vscode.window.showErrorMessage(t("Paylaşılamadı: %s", describeError(err)));
      }
    }),

    vscode.commands.registerCommand("and.sendFileToPeer", async (uri?: vscode.Uri) => {
      const target = uri ?? vscode.window.activeTextEditor?.document.uri;
      if (!target || target.scheme !== "file") {
        void vscode.window.showErrorMessage(t("Gönderilecek bir dosya seçilmedi."));
        return;
      }
      const client = await requireClient(conn);
      if (!client) {
        return;
      }
      let peers: { peer_id: string; addrs: string[] }[] = [];
      try {
        peers = await client.getPeers();
      } catch {
        // Eş listesi alınamadıysa elle giriş isteriz.
      }
      const manualEntry = `$(edit) ${t("Peer ID'yi elle gir…")}`;
      let peerId: string | undefined;
      if (peers.length > 0) {
        const pick = await vscode.window.showQuickPick(
          [...peers.map((p) => ({ label: p.peer_id, description: p.addrs[0] ?? "" })), { label: manualEntry, description: "" }],
          { placeHolder: t("Hedef eş") }
        );
        if (!pick) {
          return;
        }
        if (pick.label !== manualEntry) {
          peerId = pick.label;
        }
      }
      if (!peerId) {
        peerId = await vscode.window.showInputBox({ prompt: t("Hedef Peer ID") });
      }
      if (!peerId) {
        return;
      }
      try {
        await client.sendFile({ peer_id: peerId, local_path: target.fsPath });
        void vscode.window.showInformationMessage(t("Gönderiliyor: %s", baseName(target.fsPath)));
      } catch (err) {
        void vscode.window.showErrorMessage(t("Gönderilemedi: %s", describeError(err)));
      }
    })
  );
}
