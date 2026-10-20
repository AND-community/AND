import * as vscode from "vscode";
import { registerCommands } from "./commands";
import { DELTAX_SCHEME, DeltaXContentProvider } from "./contentProvider";
import { deltaXRoot, resolveAndDataDir } from "./dataDir";
import { trackDisposable } from "./disposableBag";
import { GroupsTreeProvider } from "./groupsTreeView";
import { BackgroundSync } from "./live/backgroundSync";
import { registerGroupActions } from "./live/deltax/groupActions";
import { DeltaXSyncService } from "./live/deltax/syncService";
import { registerEditorActions } from "./live/editorActions";
import { LiveConnection } from "./live/liveConnection";
import { isLivePanelVisible } from "./live/livePanel";
import { DMMsg, FileConsentReq } from "./live/liveTypes";
import { registerSiteCommands } from "./live/site/siteCommands";
import { SiteEditorStatus } from "./live/site/siteEditorStatus";
import { SITE_EDIT_SCHEME, SiteFileSystem } from "./live/site/siteFileSystem";

/**
 * Açık bir site sayfası editörü varken sayfaların AND'dan yeniden okunma
 * aralığı — sayfa başka bir istemciden (ör. AND TUI) değiştirilirse, VS Code
 * değişmemiş editörü yenilesin, kaydedilmemiş olanda ise kaydederken
 * "dosya daha yeni" çakışmasını gösterebilsin. Yerel pipe üzerinden tek bir
 * hafif istek; hiçbir site editörü açık değilse hiç yapılmaz.
 */
const SITE_POLL_MS = 15_000;
import { findProjectForFolder } from "./projectLocator";
import { DeltaXRepository } from "./scmController";

export class DeltaXExtension {
  andDataDir: string;
  root: string;
  repo: DeltaXRepository | undefined;
  private forgetRepo: (() => void) | undefined;
  readonly treeProvider: GroupsTreeProvider;
  readonly contentProvider: DeltaXContentProvider;
  readonly statusBar: vscode.StatusBarItem;
  readonly liveConnection: LiveConnection;
  readonly liveStatusBar: vscode.StatusBarItem;
  readonly backgroundSync: BackgroundSync;
  readonly deltaxSync: DeltaXSyncService;
  readonly siteFs: SiteFileSystem;
  private unreadDM = 0;
  private unreadConsent = 0;

  constructor(private readonly context: vscode.ExtensionContext) {
    this.andDataDir = resolveAndDataDir();
    this.root = deltaXRoot(this.andDataDir);
    this.contentProvider = new DeltaXContentProvider(() => this.root);
    this.treeProvider = new GroupsTreeProvider(
      () => this.root,
      () => this.repo?.proje.id
    );
    this.statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    this.statusBar.command = "deltax.switchBranch";

    this.liveConnection = new LiveConnection(() => this.andDataDir);
    this.backgroundSync = new BackgroundSync(this.liveConnection);
    this.deltaxSync = new DeltaXSyncService(() => this.root, this.liveConnection);
    this.siteFs = new SiteFileSystem(this.liveConnection);
    this.liveStatusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 99);
    this.liveStatusBar.command = "and.openLivePanel";
  }

  activate(): void {
    this.context.subscriptions.push(
      vscode.workspace.registerTextDocumentContentProvider(DELTAX_SCHEME, this.contentProvider),
      vscode.window.registerTreeDataProvider("deltaxGroups", this.treeProvider),
      this.statusBar,
      this.liveStatusBar,
      this.liveConnection,
      this.backgroundSync,
      this.deltaxSync,
      this.deltaxSync.onGroupsChanged(() => this.treeProvider.refresh()),
      this.backgroundSync.onDMMsg((msg) => void this.onDMReceived(msg)),
      this.backgroundSync.onFileMsg((msg) => void this.deltaxSync.handleFile(msg)),
      this.backgroundSync.onConsentReq((req) => void this.onConsentReceived(req)),
      vscode.workspace.registerFileSystemProvider(SITE_EDIT_SCHEME, this.siteFs, { isCaseSensitive: true }),
      this.siteFs,
      new SiteEditorStatus()
    );

    registerCommands(this.context, this);
    registerEditorActions(this.context, this.liveConnection);
    registerGroupActions(this.context, this);
    registerSiteCommands(this.context, this.liveConnection, this.siteFs);

    this.context.subscriptions.push(
      vscode.workspace.onDidChangeWorkspaceFolders(() => this.attachCurrentWorkspace()),
      vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration("deltax.dataDir")) {
          this.andDataDir = resolveAndDataDir();
          this.root = deltaXRoot(this.andDataDir);
          this.treeProvider.refresh();
          this.attachCurrentWorkspace();
        }
      }),
      this.liveConnection.onDidChangeConnection((connected) => {
        this.updateLiveStatusBar(connected);
        if (connected) {
          // Bağlantı yokken başka bir istemciden (ör. AND TUI) değişen sayfalar, açık site editörlerine bildirilsin.
          void this.siteFs.listPages(true).catch(() => undefined);
        }
      })
    );

    const sitePoll = setInterval(() => {
      const siteDocOpen = vscode.workspace.textDocuments.some((d) => d.uri.scheme === SITE_EDIT_SCHEME);
      if (siteDocOpen && this.liveConnection.isConnected()) {
        void this.siteFs.listPages(true).catch(() => undefined);
      }
    }, SITE_POLL_MS);
    this.context.subscriptions.push(new vscode.Disposable(() => clearInterval(sitePoll)));

    this.attachCurrentWorkspace();
    this.updateLiveStatusBar(false);
    this.liveConnection.start();
    this.backgroundSync.start();
  }

  /** Kullanıcı Canlı Paneli açtığında okunmamış DM/dosya isteği rozetini sıfırlar. */
  clearUnread(): void {
    this.unreadDM = 0;
    this.unreadConsent = 0;
    this.updateLiveStatusBar(this.liveConnection.isConnected());
  }

  /** DeltaX'in AND-PROJEAG protokol mesajları (davet/roster/push/pull) burada tüketilir — asla ham DM olarak gösterilmez. */
  private async onDMReceived(msg: DMMsg): Promise<void> {
    if (await this.deltaxSync.handleDM(msg)) {
      return;
    }
    this.onBackgroundDM(msg);
  }

  private async onConsentReceived(req: FileConsentReq): Promise<void> {
    if (await this.deltaxSync.handleConsent(req)) {
      return; // bilinen bir grup üyesinden gelen proje ağı paketi — sessizce kabul edildi
    }
    this.onBackgroundConsent(req);
  }

  private onBackgroundDM(msg: DMMsg): void {
    if (isLivePanelVisible()) {
      return; // panel zaten canlı gösteriyor, ayrıca bildirime gerek yok
    }
    this.unreadDM++;
    this.updateLiveStatusBar(this.liveConnection.isConnected());
    const preview = msg.text.length > 80 ? `${msg.text.slice(0, 80)}…` : msg.text;
    void vscode.window.showInformationMessage(`AND DM — ${msg.from}: ${preview}`, "Görüntüle").then((choice) => {
      if (choice === "Görüntüle") {
        void vscode.commands.executeCommand("and.openLivePanel");
      }
    });
  }

  private onBackgroundConsent(req: FileConsentReq): void {
    if (isLivePanelVisible()) {
      return;
    }
    this.unreadConsent++;
    this.updateLiveStatusBar(this.liveConnection.isConnected());
    const sizeKb = (req.size / 1024).toFixed(1);
    void vscode.window
      .showInformationMessage(`AND — ${req.sender_id} bir dosya göndermek istiyor: ${req.filename} (${sizeKb} KB)`, "Kabul Et", "Reddet")
      .then(async (choice) => {
        if (choice !== "Kabul Et" && choice !== "Reddet") {
          return;
        }
        const client = await this.liveConnection.ensureConnected();
        await client?.respondConsent({ transfer_id: req.transfer_id, accept: choice === "Kabul Et" });
      });
  }

  private updateLiveStatusBar(connected: boolean): void {
    const unread = this.unreadDM + this.unreadConsent;
    const badge = unread > 0 ? ` ($(bell) ${unread})` : "";
    this.liveStatusBar.text = connected ? `$(broadcast) AND: bağlı${badge}` : "$(circle-slash) AND: bağlı değil";
    this.liveStatusBar.tooltip = connected
      ? "AND'a canlı bağlısınız — Forum/DM/Sohbet/Ağ paneli için tıklayın"
      : "AND kapalı ya da AND_ENABLE_EXTERNAL_API=1 ayarlanmamış — canlı panel için tıklayın";
    this.liveStatusBar.show();
  }

  refreshAll(): void {
    this.treeProvider.refresh();
    this.repo?.refresh();
    this.updateStatusBar();
  }

  /** Açık workspace klasörü bilinen bir DeltaX proje çalışma klasörüyse SCM sağlayıcısını ona bağlar. */
  attachCurrentWorkspace(): void {
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!folder) {
      this.detachRepo();
      return;
    }
    const found = findProjectForFolder(this.root, folder.uri.fsPath);
    if (!found) {
      this.detachRepo();
      return;
    }
    if (this.repo?.proje.id === found.proje.id) {
      return; // zaten bağlı
    }
    this.detachRepo();
    this.repo = new DeltaXRepository(this.root, found.proje, folder.uri.fsPath, () => this.onRepoRefresh());
    this.forgetRepo = trackDisposable(this.context, this.repo);
    this.treeProvider.refresh();
    this.updateStatusBar();
  }

  private detachRepo(): void {
    if (this.repo) {
      this.forgetRepo?.();
      this.forgetRepo = undefined;
      this.repo.dispose();
      this.repo = undefined;
      this.treeProvider.refresh();
    }
    this.updateStatusBar();
  }

  private onRepoRefresh(): void {
    this.treeProvider.refresh();
    this.updateStatusBar();
  }

  private updateStatusBar(): void {
    if (!this.repo) {
      this.statusBar.hide();
      return;
    }
    const toplam = this.repo.status.eklenen.length + this.repo.status.degisen.length + this.repo.status.silinen.length;
    const kirli = toplam > 0 ? ` ${toplam}●` : "";
    const merging = this.repo.isMerging() ? " (birleştiriliyor)" : "";
    this.statusBar.text = `$(git-branch) ${this.repo.proje.aktif_dal}${kirli}${merging}`;
    this.statusBar.tooltip = `DeltaX proje: ${this.repo.proje.ad} — dal değiştirmek için tıklayın`;
    this.statusBar.show();
  }
}

let extension: DeltaXExtension | undefined;

export function activate(context: vscode.ExtensionContext): void {
  extension = new DeltaXExtension(context);
  extension.activate();
}

export function deactivate(): void {
  extension?.repo?.dispose();
  extension = undefined;
}
