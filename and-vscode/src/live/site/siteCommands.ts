import * as vscode from "vscode";
import { requireClient } from "../editorActions";
import { LiveConnection } from "../liveConnection";
import { SitePageInfo } from "../liveTypes";
import { SITE_EDIT_SCHEME, SiteFileSystem, siteUri } from "./siteFileSystem";
import {
  SITE_MAX_BODY_BYTES,
  SITE_MAX_PAGES,
  SITE_MAX_TITLE_BYTES,
  isValidSlug,
  shareCode,
  shareLink,
  slugFromPath,
  utf8Bytes,
  validateNewSlug,
} from "./sitePage";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

async function loadPages(siteFs: SiteFileSystem): Promise<SitePageInfo[] | undefined> {
  try {
    return await siteFs.listPages(true);
  } catch (err) {
    void vscode.window.showErrorMessage(`Kişisel site sayfaları alınamadı: ${describeError(err)}`);
    return undefined;
  }
}

/**
 * Komut argümanından slug çıkarır: Canlı Panel bir slug dizesi, editör
 * başlığındaki düğme ise belgenin Uri'sini geçirir. useActiveEditor, komut
 * paletinden çağrıldığında o an açık site sayfasının kullanılmasına izin verir.
 */
function slugFromArg(arg: unknown, useActiveEditor: boolean): string | undefined {
  if (typeof arg === "string" && isValidSlug(arg)) {
    return arg;
  }
  if (arg instanceof vscode.Uri && arg.scheme === SITE_EDIT_SCHEME) {
    return slugFromPath(arg.path);
  }
  const active = vscode.window.activeTextEditor?.document.uri;
  if (useActiveEditor && active?.scheme === SITE_EDIT_SCHEME) {
    return slugFromPath(active.path);
  }
  return undefined;
}

async function pickPage(pages: SitePageInfo[], placeHolder: string): Promise<string | undefined> {
  if (pages.length === 0) {
    const choice = await vscode.window.showInformationMessage("Henüz bir site sayfan yok.", "Yeni Sayfa");
    if (choice === "Yeni Sayfa") {
      void vscode.commands.executeCommand("and.site.newPage");
    }
    return undefined;
  }
  const pick = await vscode.window.showQuickPick(
    pages.map((p) => ({
      label: p.title || p.slug,
      description: p.slug,
      detail: `${new Date(p.updated_at).toLocaleString("tr-TR")} · ${utf8Bytes(p.body).toLocaleString("tr-TR")}/${SITE_MAX_BODY_BYTES.toLocaleString("tr-TR")} bayt`,
      slug: p.slug,
    })),
    { placeHolder, matchOnDescription: true }
  );
  return pick?.slug;
}

async function openSitePage(slug: string): Promise<void> {
  const doc = await vscode.workspace.openTextDocument(siteUri(slug));
  await vscode.window.showTextDocument(doc, { preview: false });
}

/** Kişisel site komutları — sayfaların kendisi siteFileSystem.ts'deki sanal dosya sisteminde düzenlenir. */
export function registerSiteCommands(context: vscode.ExtensionContext, conn: LiveConnection, siteFs: SiteFileSystem): void {
  context.subscriptions.push(
    vscode.commands.registerCommand("and.site.newPage", async () => {
      const pages = await loadPages(siteFs);
      if (!pages) {
        return;
      }
      if (pages.length >= SITE_MAX_PAGES) {
        void vscode.window.showErrorMessage(`En fazla ${SITE_MAX_PAGES} sayfan olabilir — yenisi için önce birini sil.`);
        return;
      }
      const existing = pages.map((p) => p.slug);
      const slugInput = await vscode.window.showInputBox({
        title: "Yeni Site Sayfası (1/2) — slug",
        prompt: "Sayfanın kalıcı adresi: küçük harf, rakam ve tire (ör. hakkimda, projeler). Sonradan değiştirilemez.",
        placeHolder: "hakkimda",
        validateInput: (v) => validateNewSlug(v.trim(), existing),
      });
      if (slugInput === undefined) {
        return;
      }
      const slug = slugInput.trim();
      const title = await vscode.window.showInputBox({
        title: "Yeni Site Sayfası (2/2) — başlık",
        prompt: "Sayfa başlığı — sonradan editördeki ilk satırdan da değiştirebilirsin.",
        value: slug,
        validateInput: (v) => {
          const n = utf8Bytes(v.trim());
          return n > SITE_MAX_TITLE_BYTES ? `Başlık en fazla ${SITE_MAX_TITLE_BYTES} bayt olabilir (şu an ${n} bayt).` : undefined;
        },
      });
      if (title === undefined) {
        return;
      }
      const uri = siteFs.createDraft(slug, title.trim());
      const editor = await vscode.window.showTextDocument(await vscode.workspace.openTextDocument(uri), { preview: false });
      const end = editor.document.positionAt(editor.document.getText().length);
      editor.selection = new vscode.Selection(end, end);
      void vscode.window.showInformationMessage(
        `"${slug}" taslağı açıldı. İlk satır başlıktır; içeriği altına yaz. Hazır olduğunda kaydet (Ctrl+S) — kaydetmek sayfayı ağa yayınlar.`
      );
    }),

    vscode.commands.registerCommand("and.site.openPage", async (arg?: unknown) => {
      let slug = slugFromArg(arg, false);
      if (!slug) {
        const pages = await loadPages(siteFs);
        if (!pages) {
          return;
        }
        slug = await pickPage(pages, "Düzenlenecek sayfa");
      }
      if (!slug) {
        return;
      }
      try {
        await openSitePage(slug);
      } catch (err) {
        void vscode.window.showErrorMessage(`"${slug}" açılamadı: ${describeError(err)}`);
      }
    }),

    vscode.commands.registerCommand("and.site.deletePage", async (arg?: unknown) => {
      const pages = await loadPages(siteFs);
      if (!pages) {
        return;
      }
      const slug = slugFromArg(arg, false) ?? (await pickPage(pages, "Silinecek sayfa"));
      if (!slug) {
        return;
      }
      const page = pages.find((p) => p.slug === slug);
      if (!page) {
        void vscode.window.showErrorMessage(`"${slug}" adlı yayınlanmış bir sayfan yok.`);
        return;
      }
      const confirm = await vscode.window.showWarningMessage(
        `"${page.title || slug}" (${slug}) sayfası silinsin mi?`,
        {
          modal: true,
          detail:
            "Ağa bir silme kaydı yayınlanır; diğer eşlerdeki ve AND Arama Motoru indekslerindeki kopyalar da kaldırılır. Silinen bir slug bir daha kullanılamaz.",
        },
        "Sil"
      );
      if (confirm !== "Sil") {
        return;
      }
      try {
        await siteFs.deletePage(slug);
        void vscode.window.showInformationMessage(`"${slug}" silindi.`);
      } catch (err) {
        void vscode.window.showErrorMessage(`"${slug}" silinemedi: ${describeError(err)}`);
      }
    }),

    vscode.commands.registerCommand("and.site.copyShareCode", async (arg?: unknown) => {
      const pages = await loadPages(siteFs);
      if (!pages) {
        return;
      }
      const slug = slugFromArg(arg, true) ?? (await pickPage(pages, "Paylaşım kodu kopyalanacak sayfa"));
      if (!slug) {
        return;
      }
      if (!pages.some((p) => p.slug === slug)) {
        void vscode.window.showErrorMessage(`"${slug}" henüz yayınlanmadı — önce kaydet (Ctrl+S), sonra paylaş.`);
        return;
      }
      const client = await requireClient(conn);
      if (!client) {
        return;
      }
      try {
        const { pub_key } = await client.getIdentity();
        await vscode.env.clipboard.writeText(shareCode(pub_key, slug));
        const choice = await vscode.window.showInformationMessage(
          `"${slug}" paylaşım kodu panoya kopyalandı — alan kişi AND Arama Motoru'nda "Kodla Git" ekranına yapıştırarak (ya da -site ile) sayfayı açabilir.`,
          "and-site:// Bağlantısını Kopyala"
        );
        if (choice) {
          await vscode.env.clipboard.writeText(shareLink(pub_key, slug));
        }
      } catch (err) {
        void vscode.window.showErrorMessage(`Paylaşım kodu alınamadı: ${describeError(err)}`);
      }
    })
  );
}
