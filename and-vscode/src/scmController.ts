import * as path from "path";
import * as vscode from "vscode";
import { createCommit, headTreeOf, readCommit } from "./commitStore";
import { makeBlobUri } from "./contentProvider";
import { clearMergeHead, mergeHeadPending, readProject, saveProject } from "./groupProjectStore";
import { projeKokDizini } from "./paths";
import { authorName } from "./settings";
import { buildTreeFromWorkdir, checkoutTree, computeStatus, FileHashCache, treeMap } from "./treeStore";
import { AgacGirdi, DurumSonucu, Proje, durumTemizMi } from "./types";
import { t } from "./dil/dil";

export type ChangeKind = "eklenen" | "degisen" | "silinen";

function decorations(kind: ChangeKind): vscode.SourceControlResourceDecorations {
  switch (kind) {
    case "eklenen":
      return { strikeThrough: false, tooltip: t("Eklendi"), iconPath: new vscode.ThemeIcon("diff-added") };
    case "degisen":
      return { strikeThrough: false, tooltip: t("Değişti"), iconPath: new vscode.ThemeIcon("diff-modified") };
    case "silinen":
      return { strikeThrough: true, tooltip: t("Silindi"), iconPath: new vscode.ThemeIcon("diff-removed") };
  }
}

export class DeltaXRepository implements vscode.Disposable {
  readonly sourceControl: vscode.SourceControl;
  private readonly changesGroup: vscode.SourceControlResourceGroup;
  private readonly watcher: vscode.FileSystemWatcher;
  private readonly disposables: vscode.Disposable[] = [];
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;
  private mergingWith: string | undefined;
  /** computeStatus() son refresh()'te başarısız olduysa true — bkz. isClean(). */
  private statusError = false;
  /**
   * Proje ömrü boyunca paylaşılan mtime/boyut → hash önbelleği (bkz.
   * treeStore.ts). Özel (private) tutulur — dışarıdan yalnızca
   * buildWorkdirTree()/checkout() üzerinden, encapsüle şekilde kullanılır;
   * hiçbir dış modül bu Map'i doğrudan silip/değiştiremez.
   */
  private readonly fileHashCache: FileHashCache = new Map();

  proje: Proje;
  headTree = "";
  status: DurumSonucu = { eklenen: [], degisen: [], silinen: [] };

  constructor(
    readonly root: string,
    proje: Proje,
    readonly calismaDir: string,
    private readonly onDidRefresh?: () => void
  ) {
    this.proje = proje;
    this.sourceControl = vscode.scm.createSourceControl(
      "deltax",
      `DeltaX: ${proje.ad}`,
      vscode.Uri.file(calismaDir)
    );
    this.changesGroup = this.sourceControl.createResourceGroup("changes", t("Değişiklikler"));
    this.writeLabels();
    this.changesGroup.hideWhenEmpty = false;

    this.watcher = vscode.workspace.createFileSystemWatcher(new vscode.RelativePattern(calismaDir, "**/*"));
    this.disposables.push(
      this.watcher.onDidChange(() => this.scheduleRefresh()),
      this.watcher.onDidCreate(() => this.scheduleRefresh()),
      this.watcher.onDidDelete(() => this.scheduleRefresh()),
      this.watcher
    );

    // meta.json (dallar/aktif_dal) ve MERGE_HEAD, calisma/ dışında,
    // proje kökünde yaşar — AND TUI'si aynı anda açıkken orada yapılan bir
    // commit/dal değişikliği/birleştirmeyi de yakalamak için ayrıca izlenir.
    const metaWatcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(projeKokDizini(root, proje.id), "{meta.json,MERGE_HEAD}")
    );
    this.disposables.push(
      metaWatcher.onDidChange(() => this.scheduleRefresh()),
      metaWatcher.onDidCreate(() => this.scheduleRefresh()),
      metaWatcher.onDidDelete(() => this.scheduleRefresh()),
      metaWatcher
    );

    this.refresh();
  }

  dispose(): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }
    this.disposables.forEach((d) => d.dispose());
    this.sourceControl.dispose();
  }

  private scheduleRefresh(): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }
    this.refreshTimer = setTimeout(() => this.refresh(), 400);
  }

  currentHead(): string {
    return this.proje.dallar[this.proje.aktif_dal] ?? "";
  }

  refresh(): void {
    try {
      this.proje = readProject(this.root, this.proje.id);
    } catch {
      // meta.json okunamadı — bir önceki bilinen durumla devam edilir
    }
    this.headTree = headTreeOf(this.root, this.proje.id, this.currentHead());
    this.mergingWith = mergeHeadPending(this.root, this.proje.id);
    try {
      this.status = computeStatus(this.root, this.proje.id, this.calismaDir, this.headTree, this.fileHashCache);
      this.statusError = false;
    } catch (err) {
      // DİKKAT: durumu boş/temiz diziye SIFIRLAMIYORUZ — bu, isClean()'in
      // yanlışlıkla "temiz" dönüp dal değiştirme/birleştirme gibi yıkıcı
      // işlemlerin durumu hiç bilinmeden (ör. geçici bir dosya kilidi,
      // AND TUI'sinin aynı anda nesneler/'i yazması ya da senkronize bir
      // ağ sürücüsünün anlık erişilemez olması yüzünden) devam etmesine yol
      // açabilirdi. Son bilinen durum olduğu gibi bırakılır; isClean() ayrıca
      // statusError bayrağını kontrol ederek bu durumda HER ZAMAN "kirli"
      // (false) döner — güvenli taraf, hata durumunda işlemi engellemektir.
      this.statusError = true;
      void vscode.window.showErrorMessage(t("DeltaX durum hesaplanamadı: %s", describeError(err)));
    }

    const resources: vscode.SourceControlResourceState[] = [];
    for (const kind of ["eklenen", "degisen", "silinen"] as ChangeKind[]) {
      for (const yol of this.status[kind]) {
        resources.push(this.makeResourceState(kind, yol));
      }
    }
    this.changesGroup.resourceStates = resources;

    const total = resources.length;
    this.sourceControl.count = total;
    this.writeLabels();

    this.onDidRefresh?.();
  }

  /** Etkin dilde yazılan metinler: dil değişince bir sonraki refresh() bunları da günceller. */
  private writeLabels(): void {
    this.sourceControl.inputBox.placeholder = t("Commit mesajı (Ctrl+Enter ile Kaydet)");
    this.sourceControl.acceptInputCommand = {
      command: "deltax.commit",
      title: t("Kaydet"),
      arguments: [this],
    };
    const dalEtiketi = this.mergingWith ? `${this.proje.aktif_dal} ${t("(birleştiriliyor)")}` : this.proje.aktif_dal;
    this.changesGroup.label = t("Değişiklikler — dal: %s", dalEtiketi);
  }

  isMerging(): boolean {
    return this.mergingWith !== undefined;
  }

  /**
   * Yalnızca son durum hesaplaması BAŞARILI olduysa VE hiçbir değişiklik
   * yoksa true döner. Son computeStatus() çağrısı hata verdiyse (bkz.
   * refresh()) — durum bilinmiyor demektir — burada KASITLI olarak false
   * dönülür: dal değiştirme/birleştirme gibi yıkıcı işlemler, gerçek durum
   * doğrulanamadığında asla "temiz" varsayılıp devam etmemelidir.
   */
  isClean(): boolean {
    return !this.statusError && durumTemizMi(this.status);
  }

  private makeResourceState(kind: ChangeKind, yol: string): vscode.SourceControlResourceState {
    const resourceUri = vscode.Uri.file(path.join(this.calismaDir, ...yol.split("/")));
    return {
      resourceUri,
      decorations: decorations(kind),
      command: {
        command: "deltax.openChange",
        title: t("Değişikliği Görüntüle"),
        arguments: [this, yol, kind],
      },
    };
  }

  /** deltax.openChange tarafından çağrılır: HEAD sürümü ile çalışma dosyasını native diff'te açar. */
  async openChange(yol: string, kind: ChangeKind): Promise<void> {
    const workingUri = vscode.Uri.file(path.join(this.calismaDir, ...yol.split("/")));
    if (kind === "eklenen") {
      await vscode.window.showTextDocument(workingUri);
      return;
    }

    const headMap: Map<string, AgacGirdi> = this.headTree
      ? treeMap(this.root, this.proje.id, this.headTree)
      : new Map();
    const headEntry = headMap.get(yol);
    if (!headEntry) {
      await vscode.window.showTextDocument(workingUri);
      return;
    }
    const headUri = makeBlobUri(this.proje.id, yol, headEntry.hash, `${yol} (HEAD)`);

    if (kind === "silinen") {
      await vscode.commands.executeCommand("vscode.open", headUri, { preview: true }, `${yol} (${t("silinen — HEAD sürümü")})`);
      return;
    }
    await vscode.commands.executeCommand(
      "vscode.diff",
      headUri,
      workingUri,
      `${yol} (${t("HEAD ↔ Çalışma Klasörü")})`,
      { preview: true }
    );
  }

  /**
   * agacOlustur'un bu proje/çalışma dizini için, proje ömrü boyunca
   * paylaşılan önbelleği kullanan sarmalayıcısı. branchActions.ts'teki
   * birleştirme akışı, `fileHashCache`'e doğrudan erişmek yerine bunu
   * çağırır.
   */
  buildWorkdirTree(): ReturnType<typeof buildTreeFromWorkdir> {
    return buildTreeFromWorkdir(this.root, this.proje.id, this.calismaDir, this.fileHashCache);
  }

  /**
   * dalaGec'in bu proje/çalışma dizini için, proje ömrü boyunca paylaşılan
   * önbelleği kullanan (ve işlem sonunda temizleyen) sarmalayıcısı.
   */
  checkout(hedefTree: string): void {
    checkoutTree(this.root, this.proje.id, this.calismaDir, hedefTree, this.fileHashCache);
  }

  /** commitCmd'nin karşılığı. */
  async commit(message: string): Promise<{ hash: string; atlanan: number }> {
    const trimmed = message.trim();
    if (!trimmed) {
      throw new Error(t("Commit mesajı boş olamaz"));
    }
    const { hash: yeniTree, atlanan } = this.buildWorkdirTree();
    const head = this.currentHead();
    const ebeveynler: string[] = head ? [head] : [];

    const merging = mergeHeadPending(this.root, this.proje.id);
    if (merging) {
      ebeveynler.push(merging);
    } else if (head) {
      const hc = readCommit(this.root, this.proje.id, head);
      if (hc.agac === yeniTree) {
        throw new Error(t("Kaydedilecek değişiklik yok"));
      }
    }

    const hash = createCommit(this.root, this.proje.id, yeniTree, ebeveynler, authorName(), trimmed);
    this.proje.dallar[this.proje.aktif_dal] = hash;
    saveProject(this.root, this.proje);
    clearMergeHead(this.root, this.proje.id);
    this.sourceControl.inputBox.value = "";
    this.refresh();
    return { hash, atlanan: atlanan.length };
  }
}

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
