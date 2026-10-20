import * as vscode from "vscode";
import { SITE_EDIT_SCHEME } from "./siteFileSystem";
import { SITE_MAX_BODY_BYTES, SITE_MAX_TITLE_BYTES, bodyOverflowOffset, parsePage, utf8Bytes } from "./sitePage";

function formatCount(n: number): string {
  return n.toLocaleString("tr-TR");
}

/**
 * Açık bir site sayfası editöründe, Eklentiler/site'ın düzenleme formundaki
 * canlı bayt sayaçlarının karşılığı: durum çubuğunda "başlık 12/100 · gövde
 * 1.234/8.000 bayt" ve sınırı aşan kısım için — tam aşıldığı karakterden
 * itibaren — editörde hata işareti (Sorunlar panelinde de görünür). Böylece
 * AND'ın reddedeceği bir sayfa ağa hiç gönderilmeden fark edilir.
 */
export class SiteEditorStatus implements vscode.Disposable {
  private readonly item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
  private readonly diagnostics = vscode.languages.createDiagnosticCollection("and-site");
  private readonly subscriptions: vscode.Disposable[];

  constructor() {
    this.subscriptions = [
      vscode.window.onDidChangeActiveTextEditor(() => this.updateStatus()),
      vscode.workspace.onDidChangeTextDocument((e) => this.onDocument(e.document)),
      vscode.workspace.onDidOpenTextDocument((doc) => this.onDocument(doc)),
      vscode.workspace.onDidCloseTextDocument((doc) => this.diagnostics.delete(doc.uri)),
    ];
    vscode.workspace.textDocuments.forEach((doc) => this.check(doc));
    this.updateStatus();
  }

  dispose(): void {
    this.subscriptions.forEach((s) => s.dispose());
    this.item.dispose();
    this.diagnostics.dispose();
  }

  private onDocument(doc: vscode.TextDocument): void {
    if (doc.uri.scheme !== SITE_EDIT_SCHEME) {
      return;
    }
    this.check(doc);
    if (doc === vscode.window.activeTextEditor?.document) {
      this.updateStatus();
    }
  }

  private check(doc: vscode.TextDocument): void {
    if (doc.uri.scheme !== SITE_EDIT_SCHEME) {
      return;
    }
    const text = doc.getText();
    const page = parsePage(text);
    const found: vscode.Diagnostic[] = [];

    const titleBytes = utf8Bytes(page.title);
    if (titleBytes > SITE_MAX_TITLE_BYTES) {
      found.push(
        new vscode.Diagnostic(
          doc.lineAt(0).range,
          `Başlık en fazla ${SITE_MAX_TITLE_BYTES} bayt olabilir (şu an ${titleBytes} bayt) — AND bu sayfayı kabul etmez.`,
          vscode.DiagnosticSeverity.Error
        )
      );
    }
    const overflow = bodyOverflowOffset(text);
    if (overflow !== undefined) {
      found.push(
        new vscode.Diagnostic(
          new vscode.Range(doc.positionAt(overflow), doc.positionAt(text.length)),
          `Sayfa en fazla ${SITE_MAX_BODY_BYTES} bayt olabilir (şu an ${utf8Bytes(page.body)} bayt) — işaretli kısım sığmıyor.`,
          vscode.DiagnosticSeverity.Error
        )
      );
    }
    for (const d of found) {
      d.source = "AND Site";
    }
    this.diagnostics.set(doc.uri, found);
  }

  private updateStatus(): void {
    const doc = vscode.window.activeTextEditor?.document;
    if (!doc || doc.uri.scheme !== SITE_EDIT_SCHEME) {
      this.item.hide();
      return;
    }
    const page = parsePage(doc.getText());
    const titleBytes = utf8Bytes(page.title);
    const bodyBytes = utf8Bytes(page.body);
    const over = titleBytes > SITE_MAX_TITLE_BYTES || bodyBytes > SITE_MAX_BODY_BYTES;
    this.item.text =
      `${over ? "$(error)" : "$(globe)"} başlık ${formatCount(titleBytes)}/${formatCount(SITE_MAX_TITLE_BYTES)}` +
      ` · gövde ${formatCount(bodyBytes)}/${formatCount(SITE_MAX_BODY_BYTES)} bayt`;
    this.item.tooltip =
      "AND kişisel site sayfası — kaydetmek (Ctrl+S) sayfayı ağa yayınlar. Sınırlar AND'ın kendisiyle aynıdır " +
      "ve karakter değil BAYT sayar (ğ, ş gibi Türkçe karakterler 2 bayttır).";
    this.item.backgroundColor = over ? new vscode.ThemeColor("statusBarItem.errorBackground") : undefined;
    this.item.show();
  }
}
