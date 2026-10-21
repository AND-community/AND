import * as vscode from "vscode";
import { registerCommands } from "./commands";
import { DELTAX_SCHEME, DeltaXContentProvider } from "./contentProvider";
import { deltaXRoot, resolveAndDataDir } from "./dataDir";
import { trackDisposable } from "./disposableBag";
import { GroupsTreeProvider } from "./groupsTreeView";
import { BackgroundSync } from "./live/backgroundSync";
import { registerGroupActions } from "./live/deltax/groupActions";
import { DeltaXSyncService } from "./live/deltax/syncService";
import { GelenDM, durumDosyasi, gorunenAd, makineIletisiMi } from "./live/dmCore";
import { DmMerkezi } from "./live/dmMerkezi";
import { registerEditorActions } from "./live/editorActions";
import { PencereLideri } from "./live/lider";
import { LiveConnection } from "./live/liveConnection";
import { isLivePanelVisible } from "./live/livePanel";
import { FileConsentReq } from "./live/liveTypes";
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
/** Okunmamış DM rozetinin AND'dan yeniden okunma aralığı (Özel Chat'te okunanlar da yansısın). */
const ROZET_POLL_MS = 60_000;
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
  readonly dm = new DmMerkezi();
  private readonly dmChangedEmitter = new vscode.EventEmitter<void>();
  /** DM'ler değişti (yeni mesaj geldi, gönderildi ya da okundu işaretlendi). */
  readonly onDidChangeDM = this.dmChangedEmitter.event;
  private unreadDM = 0;
  private unreadConsent = 0;
  /** Gelen DM grupları sırayla işlenir: DeltaX'in üye listesi güncellemeleri geliş sırasıyla uygulanmalı. */
  private dmQueue: Promise<void> = Promise.resolve();

  constructor(private readonly context: vscode.ExtensionContext) {
    this.andDataDir = resolveAndDataDir();
    this.root = deltaXRoot(this.andDataDir);
    const stateFile = (name: string): string => durumDosyasi(context.globalStorageUri.fsPath, this.andDataDir, name);
    this.contentProvider = new DeltaXContentProvider(() => this.root);
    this.statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    this.statusBar.command = "deltax.switchBranch";

    this.liveConnection = new LiveConnection(() => this.andDataDir);
    this.backgroundSync = new BackgroundSync(
      this.liveConnection,
      () => stateFile("dm-imleci"),
      new PencereLideri(() => stateFile("dm-lider"))
    );
    this.deltaxSync = new DeltaXSyncService(() => this.root, this.liveConnection, stateFile);
    this.treeProvider = new GroupsTreeProvider(
      () => this.root,
      () => this.repo?.proje.id,
      () => this.deltaxSync.bekleyenDavetler().length
    );
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
      this.dmChangedEmitter,
      this.backgroundSync.onDMBatch((batch) => {
        this.dmQueue = this.dmQueue.then(() => this.onDMBatch(batch)).catch(() => undefined);
      }),
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
          void this.refreshUnreadDM();
        }
      }),
      // VS Code'a dönünce rozet güncellensin: mesaj bu arada AND'da ya da Özel Chat'te okunmuş olabilir.
      vscode.window.onDidChangeWindowState((s) => {
        if (s.focused) {
          void this.refreshUnreadDM();
        }
      })
    );
    const unreadPoll = setInterval(() => void this.refreshUnreadDM(), ROZET_POLL_MS);
    this.context.subscriptions.push(new vscode.Disposable(() => clearInterval(unreadPoll)));

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

  /**
   * Kullanıcı Canlı Paneli açtığında dosya isteği rozetini sıfırlar. DM'ler
   * AND'ın okundu bilgisine göre sayılır: konuşma panelde açılınca düşer.
   */
  clearUnread(): void {
    this.unreadConsent = 0;
    this.updateLiveStatusBar(this.liveConnection.isConnected());
  }

  /** DM'ler değişti: panel yenilensin, rozet AND'dan yeniden okunsun. */
  dmChanged(): void {
    this.dmChangedEmitter.fire();
    void this.refreshUnreadDM();
  }

  /** Durum çubuğu rozeti: AND'ın ana menüsündeki okunmamış DM sayısıyla aynı. */
  async refreshUnreadDM(): Promise<void> {
    const client = this.liveConnection.getClient();
    if (!client) {
      return;
    }
    try {
      this.unreadDM = (await this.dm.durum(client)).okunmamis;
      this.updateLiveStatusBar(true);
    } catch {
      // bağlantı koptu; bir sonraki turda
    }
  }

  /**
   * AND'ın DM geçmişinden gelen yeni mesajlar. DeltaX'in grup protokolü
   * mesajları burada tüketilir, asla DM olarak gösterilmez; insanların
   * yazdıkları bildirim olarak düşer.
   */
  private async onDMBatch(batch: GelenDM[]): Promise<void> {
    const human: GelenDM[] = [];
    for (const dm of batch) {
      if ((await this.deltaxSync.handleDM(dm)) || makineIletisiMi(dm.text)) {
        continue;
      }
      human.push(dm);
    }
    if (human.length === 0) {
      return;
    }
    this.dm.canliGeldi(human);
    this.dmChanged();
    this.notifyDMs(human);
  }

  private notifyDMs(dms: GelenDM[]): void {
    if (isLivePanelVisible()) {
      return; // panel zaten canlı gösteriyor, ayrıca bildirime gerek yok
    }
    const first = dms[0];
    const preview = first.text.length > 80 ? `${first.text.slice(0, 80)}…` : first.text;
    const senders = [...new Set(dms.map(gorunenAd))];
    const text =
      dms.length === 1
        ? `AND DM — ${gorunenAd(first)}: ${preview}`
        : `AND — ${dms.length} yeni doğrudan mesaj (${senders.slice(0, 3).join(", ")}${senders.length > 3 ? "…" : ""})`;
    void vscode.window.showInformationMessage(text, "Görüntüle").then((choice) => {
      if (choice === "Görüntüle") {
        void vscode.commands.executeCommand("and.openLivePanel");
      }
    });
  }

  private async onConsentReceived(req: FileConsentReq): Promise<void> {
    if (await this.deltaxSync.handleConsent(req)) {
      return; // bilinen bir grup üyesinden gelen proje ağı paketi — sessizce kabul edildi
    }
    this.onBackgroundConsent(req);
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
