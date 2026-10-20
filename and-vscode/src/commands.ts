import * as path from "path";
import * as vscode from "vscode";
import { mergeRemoteBranch, newBranch, switchBranch } from "./branchActions";
import { shortHash } from "./commitStore";
import type { DeltaXExtension } from "./extension";
import { defaultExportFolder, exportProject } from "./exportZip";
import { createProject, listGroups } from "./groupProjectStore";
import { GroupItem, ProjectItem } from "./groupsTreeView";
import { showLivePanel } from "./live/livePanel";
import { refreshLogPanel, showLogPanel } from "./logPanel";
import { projeCalismaDizini } from "./paths";
import { listAllProjects } from "./projectLocator";
import { ChangeKind, DeltaXRepository } from "./scmController";
import { Grup } from "./types";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function resolveRepo(ext: DeltaXExtension, maybe: unknown): DeltaXRepository | undefined {
  return maybe instanceof DeltaXRepository ? maybe : ext.repo;
}

async function requireRepo(ext: DeltaXExtension, maybe: unknown): Promise<DeltaXRepository | undefined> {
  const repo = resolveRepo(ext, maybe);
  if (!repo) {
    void vscode.window.showErrorMessage(
      "Aktif bir DeltaX projesi yok. Önce Gruplar ve Projeler görünümünden bir proje açın."
    );
    return undefined;
  }
  return repo;
}

export async function pickGroup(root: string): Promise<Grup | undefined> {
  const gruplar = listGroups(root);
  if (gruplar.length === 0) {
    void vscode.window.showInformationMessage(
      "Henüz bir DeltaX grubun yok. Etkinlik çubuğundaki DeltaX görünümünün başlığından \"Yeni Grup\" ile oluşturabilirsin."
    );
    return undefined;
  }
  const secim = await vscode.window.showQuickPick(
    gruplar.map((g) => ({ label: g.ad, description: `${g.uyeler.length} üye`, grup: g })),
    { placeHolder: "Grup seçin" }
  );
  return secim?.grup;
}

async function openProjectInWorkspace(root: string, projeId: string): Promise<void> {
  const calismaDir = projeCalismaDizini(root, projeId);
  await vscode.commands.executeCommand("vscode.openFolder", vscode.Uri.file(calismaDir), { forceNewWindow: false });
}

export function registerCommands(context: vscode.ExtensionContext, ext: DeltaXExtension): void {
  const reg = (id: string, handler: (...args: unknown[]) => unknown): void => {
    context.subscriptions.push(vscode.commands.registerCommand(id, handler));
  };

  reg("deltax.refresh", () => ext.refreshAll());

  reg("deltax.openProject", async (item?: unknown) => {
    let projeId: string | undefined;
    if (item instanceof ProjectItem) {
      projeId = item.proje.id;
    } else {
      const secim = await vscode.window.showQuickPick(
        listAllProjects(ext.root).map((gp) => ({
          label: gp.proje.ad,
          description: `${gp.grup.ad} · ${gp.proje.aktif_dal}`,
          projeId: gp.proje.id,
        })),
        { placeHolder: "Açılacak projeyi seçin" }
      );
      projeId = secim?.projeId;
    }
    if (!projeId) {
      return;
    }
    await openProjectInWorkspace(ext.root, projeId);
  });

  reg("deltax.newProject", async (item?: unknown) => {
    const grup = item instanceof GroupItem ? item.grup : await pickGroup(ext.root);
    if (!grup) {
      return;
    }
    const ad = await vscode.window.showInputBox({
      prompt: `"${grup.ad}" grubunda yeni proje adı`,
      validateInput: (v) => (v.trim() ? undefined : "Proje adı boş olamaz"),
    });
    if (!ad) {
      return;
    }
    const proje = createProject(ext.root, grup.id, ad.trim());
    ext.treeProvider.refresh();
    const secim = await vscode.window.showInformationMessage(`Proje oluşturuldu: ${proje.ad}`, "Aç");
    if (secim === "Aç") {
      await openProjectInWorkspace(ext.root, proje.id);
    }
  });

  reg("deltax.commit", async (repoArg?: unknown) => {
    const repo = await requireRepo(ext, repoArg);
    if (!repo) {
      return;
    }
    const message = repo.sourceControl.inputBox.value;
    try {
      const { hash, atlanan } = await repo.commit(message);
      const not = atlanan > 0 ? ` (${atlanan} dosya boyut sınırı nedeniyle atlandı)` : "";
      void vscode.window.showInformationMessage(`Kaydedildi: ${shortHash(hash)}${not}`);
      refreshLogPanel(repo);
    } catch (err) {
      void vscode.window.showErrorMessage(describeError(err));
    }
  });

  reg("deltax.showLog", async (repoArg?: unknown) => {
    const repo = await requireRepo(ext, repoArg);
    if (!repo) {
      return;
    }
    showLogPanel(repo, context);
  });

  reg("deltax.switchBranch", async (repoArg?: unknown) => {
    const repo = await requireRepo(ext, repoArg);
    if (!repo) {
      return;
    }
    await switchBranch(repo);
  });

  reg("deltax.newBranch", async (repoArg?: unknown) => {
    const repo = await requireRepo(ext, repoArg);
    if (!repo) {
      return;
    }
    await newBranch(repo);
  });

  reg("deltax.merge", async (repoArg?: unknown) => {
    const repo = await requireRepo(ext, repoArg);
    if (!repo) {
      return;
    }
    await mergeRemoteBranch(repo);
    refreshLogPanel(repo);
  });

  reg("deltax.exportZip", async (item?: unknown) => {
    let projeId: string | undefined;
    let projeAd: string | undefined;
    if (item instanceof ProjectItem) {
      projeId = item.proje.id;
      projeAd = item.proje.ad;
    } else if (ext.repo) {
      projeId = ext.repo.proje.id;
      projeAd = ext.repo.proje.ad;
    }
    if (!projeId || !projeAd) {
      void vscode.window.showErrorMessage("Dışa aktarılacak bir proje seçilmedi.");
      return;
    }
    const hedef = await vscode.window.showOpenDialog({
      canSelectFiles: false,
      canSelectFolders: true,
      canSelectMany: false,
      defaultUri: vscode.Uri.file(defaultExportFolder()),
      openLabel: "Bu klasöre dışa aktar",
    });
    if (!hedef || hedef.length === 0) {
      return;
    }
    try {
      const yol = await exportProject(ext.root, projeId, projeAd, hedef[0].fsPath);
      const secim = await vscode.window.showInformationMessage(`Dışa aktarıldı: ${yol}`, "Klasörde Göster");
      if (secim === "Klasörde Göster") {
        await vscode.commands.executeCommand("revealFileInOS", vscode.Uri.file(yol));
      }
    } catch (err) {
      void vscode.window.showErrorMessage(`Dışa aktarma başarısız: ${describeError(err)}`);
    }
  });

  reg("deltax.openIgnoreFile", async () => {
    const repo = await requireRepo(ext, undefined);
    if (!repo) {
      return;
    }
    const uri = vscode.Uri.file(path.join(repo.calismaDir, ".projeagignore"));
    try {
      await vscode.workspace.fs.stat(uri);
    } catch {
      await vscode.workspace.fs.writeFile(
        uri,
        Buffer.from("# Her satıra bir desen (ör. node_modules, *.log)\n", "utf8")
      );
    }
    const doc = await vscode.workspace.openTextDocument(uri);
    await vscode.window.showTextDocument(doc);
  });

  reg("deltax.openChange", async (repo: unknown, yol: unknown, kind: unknown) => {
    if (repo instanceof DeltaXRepository && typeof yol === "string") {
      await repo.openChange(yol, kind as ChangeKind);
    }
  });

  reg("and.openLivePanel", () => {
    showLivePanel(ext.liveConnection, ext.backgroundSync, ext.siteFs, context);
    ext.clearUnread();
  });
}
