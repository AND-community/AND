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
import { binlik, etkin, t } from "../../dil/dil";

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

async function loadPages(siteFs: SiteFileSystem): Promise<SitePageInfo[] | undefined> {
  try {
    return await siteFs.listPages(true);
  } catch (err) {
    void vscode.window.showErrorMessage(t("Kişisel site sayfaları alınamadı: %s", describeError(err)));
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
    const yeni = t("Yeni Sayfa");
    const choice = await vscode.window.showInformationMessage(t("Henüz bir site sayfan yok."), yeni);
    if (choice === yeni) {
      void vscode.commands.executeCommand("and.site.newPage");
    }
    return undefined;
  }
  const pick = await vscode.window.showQuickPick(
    pages.map((p) => ({
      label: p.title || p.slug,
      description: p.slug,
      detail: `${new Date(p.updated_at).toLocaleString(etkin())} · ${t("%s/%s bayt", binlik(utf8Bytes(p.body)), binlik(SITE_MAX_BODY_BYTES))}`,
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
        void vscode.window.showErrorMessage(t("En fazla %d sayfan olabilir — yenisi için önce birini sil.", SITE_MAX_PAGES));
        return;
      }
      const existing = pages.map((p) => p.slug);
      const slugInput = await vscode.window.showInputBox({
        title: t("Yeni Site Sayfası (1/2) — slug"),
        prompt: t("Sayfanın kalıcı adresi: küçük harf, rakam ve tire (ör. hakkimda, projeler). Sonradan değiştirilemez."),
        placeHolder: t("hakkimda"),
        validateInput: (v) => validateNewSlug(v.trim(), existing),
      });
      if (slugInput === undefined) {
        return;
      }
      const slug = slugInput.trim();
      const title = await vscode.window.showInputBox({
        title: t("Yeni Site Sayfası (2/2) — başlık"),
        prompt: t("Sayfa başlığı — sonradan editördeki ilk satırdan da değiştirebilirsin."),
        value: slug,
        validateInput: (v) => {
          const n = utf8Bytes(v.trim());
          return n > SITE_MAX_TITLE_BYTES ? t("Başlık en fazla %d bayt olabilir (şu an %d bayt).", SITE_MAX_TITLE_BYTES, n) : undefined;
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
        t("\"%s\" taslağı açıldı. İlk satır başlıktır; içeriği altına yaz. Hazır olduğunda kaydet (Ctrl+S) — kaydetmek sayfayı ağa yayınlar.", slug)
      );
    }),

    vscode.commands.registerCommand("and.site.openPage", async (arg?: unknown) => {
      let slug = slugFromArg(arg, false);
      if (!slug) {
        const pages = await loadPages(siteFs);
        if (!pages) {
          return;
        }
        slug = await pickPage(pages, t("Düzenlenecek sayfa"));
      }
      if (!slug) {
        return;
      }
      try {
        await openSitePage(slug);
      } catch (err) {
        void vscode.window.showErrorMessage(t("\"%s\" açılamadı: %s", slug, describeError(err)));
      }
    }),

    vscode.commands.registerCommand("and.site.deletePage", async (arg?: unknown) => {
      const pages = await loadPages(siteFs);
      if (!pages) {
        return;
      }
      const slug = slugFromArg(arg, false) ?? (await pickPage(pages, t("Silinecek sayfa")));
      if (!slug) {
        return;
      }
      const page = pages.find((p) => p.slug === slug);
      if (!page) {
        void vscode.window.showErrorMessage(t("\"%s\" adlı yayınlanmış bir sayfan yok.", slug));
        return;
      }
      const sil = t("Sil");
      const confirm = await vscode.window.showWarningMessage(
        t("\"%s\" (%s) sayfası silinsin mi?", page.title || slug, slug),
        {
          modal: true,
          detail: t(
            "Ağa bir silme kaydı yayınlanır; diğer eşlerdeki ve AND Arama Motoru indekslerindeki kopyalar da kaldırılır. Silinen bir slug bir daha kullanılamaz."
          ),
        },
        sil
      );
      if (confirm !== sil) {
        return;
      }
      try {
        await siteFs.deletePage(slug);
        void vscode.window.showInformationMessage(t("\"%s\" silindi.", slug));
      } catch (err) {
        void vscode.window.showErrorMessage(t("\"%s\" silinemedi: %s", slug, describeError(err)));
      }
    }),

    vscode.commands.registerCommand("and.site.copyShareCode", async (arg?: unknown) => {
      const pages = await loadPages(siteFs);
      if (!pages) {
        return;
      }
      const slug = slugFromArg(arg, true) ?? (await pickPage(pages, t("Paylaşım kodu kopyalanacak sayfa")));
      if (!slug) {
        return;
      }
      if (!pages.some((p) => p.slug === slug)) {
        void vscode.window.showErrorMessage(t("\"%s\" henüz yayınlanmadı — önce kaydet (Ctrl+S), sonra paylaş.", slug));
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
          t(
            "\"%s\" paylaşım kodu panoya kopyalandı — alan kişi AND Arama Motoru'nda \"Kodla Git\" ekranına yapıştırarak (ya da -site ile) sayfayı açabilir.",
            slug
          ),
          t("and-site:// Bağlantısını Kopyala")
        );
        if (choice) {
          await vscode.env.clipboard.writeText(shareLink(pub_key, slug));
        }
      } catch (err) {
        void vscode.window.showErrorMessage(t("Paylaşım kodu alınamadı: %s", describeError(err)));
      }
    })
  );
}
