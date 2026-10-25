import * as vscode from "vscode";
import { LiveClient } from "../liveClient";
import { LiveConnection } from "../liveConnection";
import { SitePageInfo } from "../liveTypes";
import {
  SitePageContent,
  parsePage,
  pathForSlug,
  publishDelayMs,
  serializePage,
  slugFromPath,
  utf8Bytes,
  validatePage,
} from "./sitePage";
import { t } from "../../dil/dil";

/**
 * Kişisel site sayfalarını GERÇEK VS Code editör belgeleri olarak açan sanal
 * dosya sistemi: and-site-edit:/<slug>.andsite. Okuma AND'ın
 * /api/v1/site/pages'inden, kaydetme /api/v1/site/publish'ten geçer — yani
 * Ctrl+S, AND TUI'sindeki site eklentisinin ctrl+s'i gibi "kaydet ve ağa
 * yayınla" demektir. Belge biçimi için bkz. sitePage.ts'deki serializePage.
 *
 * Şema adı Arama Motoru'nun and-site:// paylaşım bağlantılarından BİLEREK
 * farklıdır: VS Code kayıtlı bir dosya sistemi şemasındaki bağlantıları kendi
 * içinde açar — "and-site" kullanılsaydı, başkasının sayfasına giden ve
 * işletim sisteminde Arama Motoru'nun açması gereken her bağlantı burada
 * (yalnızca KENDİ sayfalarımızı okuyabilen) bu sağlayıcıya düşerdi.
 *
 * stat() her sayfanın mtime'ı olarak AND'daki updated_at'i döner; VS Code
 * kaydetmeden önce bunu karşılaştırdığından, sayfa bu editör açıkken başka
 * bir yerde (ör. AND TUI'sinde) değiştirilmişse yerleşik "dosya daha yeni —
 * Karşılaştır / Üzerine Yaz" akışı kendiliğinden çalışır.
 */
export const SITE_EDIT_SCHEME = "and-site-edit";

/** stat/readFile patlamalarını (açılış, kaydetme) tek bir API çağrısında toplayan kısa önbellek ömrü. */
const CACHE_TTL_MS = 1500;

export function siteUri(slug: string): vscode.Uri {
  return vscode.Uri.from({ scheme: SITE_EDIT_SCHEME, path: pathForSlug(slug) });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function encode(text: string): Uint8Array {
  return Buffer.from(text, "utf8");
}

export class SiteFileSystem implements vscode.FileSystemProvider, vscode.Disposable {
  private readonly fileEmitter = new vscode.EventEmitter<vscode.FileChangeEvent[]>();
  readonly onDidChangeFile = this.fileEmitter.event;
  private readonly pagesEmitter = new vscode.EventEmitter<SitePageInfo[]>();
  /** Sayfa listesi AND'dan her yeniden okunduğunda (yayın/silme sonrası dahil) — Canlı Panel'in Site sekmesi için. */
  readonly onDidChangePages = this.pagesEmitter.event;

  private pages = new Map<string, SitePageInfo>();
  private fetchedAt = 0;
  private inflight: Promise<SitePageInfo[]> | undefined;
  /** Her okumaya başlarken artan sıra numarası ve en son UYGULANAN okumanınki — bkz. fetchPages. */
  private fetchSeq = 0;
  private appliedSeq = 0;
  /** "Yeni Sayfa" ile açılmış ama henüz hiç yayınlanmamış sayfalar — ilk kayıtta yayınlanır. */
  private readonly drafts = new Map<string, SitePageContent>();
  /** Bu oturumda slug başına son başarılı yayın anı (ms) — bkz. publishDelayMs. */
  private readonly lastPublish = new Map<string, number>();
  /** onWillSaveTextDocument'tan yakalanan son kaydetme nedeni — writeFile'da otomatik kaydetmeyi ayırt etmek için. */
  private readonly saveReasons = new Map<string, vscode.TextDocumentSaveReason>();
  private readonly willSaveSub: vscode.Disposable;

  constructor(private readonly conn: LiveConnection) {
    this.willSaveSub = vscode.workspace.onWillSaveTextDocument((e) => {
      if (e.document.uri.scheme === SITE_EDIT_SCHEME) {
        this.saveReasons.set(e.document.uri.toString(), e.reason);
      }
    });
  }

  dispose(): void {
    this.willSaveSub.dispose();
    this.fileEmitter.dispose();
    this.pagesEmitter.dispose();
  }

  // ─── Komutlar ve Canlı Panel için ─────────────────────────────────────────

  /** AND'daki güncel sayfalar (slug'a göre sıralı). force=false iken kısa ömürlü önbellekten döner. */
  listPages(force = false): Promise<SitePageInfo[]> {
    if (!force && this.fetchedAt > 0 && Date.now() - this.fetchedAt < CACHE_TTL_MS) {
      return Promise.resolve(this.sortedPages());
    }
    this.inflight ??= this.fetchPages().finally(() => {
      this.inflight = undefined;
    });
    return this.inflight;
  }

  /** Henüz yayınlanmamış yeni bir sayfa taslağı açar; editörde ilk kaydedildiğinde yayınlanır. */
  createDraft(slug: string, title: string): vscode.Uri {
    this.drafts.set(slug, { title, body: "" });
    const uri = siteUri(slug);
    this.fileEmitter.fire([{ type: vscode.FileChangeType.Created, uri }]);
    return uri;
  }

  deletePage(slug: string): Promise<void> {
    return this.delete(siteUri(slug));
  }

  // ─── vscode.FileSystemProvider ────────────────────────────────────────────

  watch(): vscode.Disposable {
    // Değişiklikleri zaten kendimiz bildiriyoruz (listPages'in her yenilemesi).
    return new vscode.Disposable(() => undefined);
  }

  async stat(uri: vscode.Uri): Promise<vscode.FileStat> {
    if (uri.path === "/" || uri.path === "") {
      return { type: vscode.FileType.Directory, ctime: 0, mtime: 0, size: 0 };
    }
    const slug = this.slugOf(uri);
    const draft = this.drafts.get(slug);
    if (draft) {
      return { type: vscode.FileType.File, ctime: 0, mtime: 0, size: utf8Bytes(serializePage(draft)) };
    }
    const page = await this.findPage(uri, slug);
    const mtime = Date.parse(page.updated_at) || 0;
    return { type: vscode.FileType.File, ctime: mtime, mtime, size: utf8Bytes(serializePage(page)) };
  }

  async readDirectory(uri: vscode.Uri): Promise<[string, vscode.FileType][]> {
    if (uri.path !== "/" && uri.path !== "") {
      throw vscode.FileSystemError.FileNotADirectory(uri);
    }
    const slugs = new Set([...(await this.listPages()).map((p) => p.slug), ...this.drafts.keys()]);
    return [...slugs].sort().map((slug): [string, vscode.FileType] => [pathForSlug(slug).slice(1), vscode.FileType.File]);
  }

  createDirectory(): void {
    throw vscode.FileSystemError.NoPermissions(t("Kişisel sitede klasör yoktur — her sayfa tek bir slug'dır."));
  }

  async readFile(uri: vscode.Uri): Promise<Uint8Array> {
    const slug = this.slugOf(uri);
    const draft = this.drafts.get(slug);
    return encode(serializePage(draft ?? (await this.findPage(uri, slug))));
  }

  async writeFile(uri: vscode.Uri, content: Uint8Array): Promise<void> {
    const slug = this.slugOf(uri);
    const key = uri.toString();
    const reason = this.saveReasons.get(key);
    this.saveReasons.delete(key);
    if (reason === vscode.TextDocumentSaveReason.AfterDelay || reason === vscode.TextDocumentSaveReason.FocusOut) {
      // package.json bu dil için otomatik kaydetmeyi zaten kapatır; kullanıcı
      // yine de açtıysa yarım bir taslağın ağa yayılmasına izin verilmez.
      throw new Error(t("Otomatik kaydetme bir site sayfasını ağa YAYINLAMAZ — sayfa hazır olduğunda Ctrl+S ile kaydet."));
    }
    const page = parsePage(Buffer.from(content).toString("utf8"));
    const problem = validatePage(slug, page);
    if (problem) {
      throw new Error(problem);
    }
    await this.publish(slug, page);
  }

  async delete(uri: vscode.Uri): Promise<void> {
    const slug = this.slugOf(uri);
    if (this.drafts.delete(slug)) {
      this.fileEmitter.fire([{ type: vscode.FileChangeType.Deleted, uri }]);
      return;
    }
    const client = await this.requireClient();
    await client.deleteSitePage(slug);
    this.lastPublish.delete(slug);
    await this.fetchPages();
  }

  rename(): void {
    throw vscode.FileSystemError.NoPermissions(
      t("Bir sayfanın slug'ı değiştirilemez — yeni bir sayfa oluşturup içeriği taşıyabilir, eskisini silebilirsin.")
    );
  }

  // ─── İç yardımcılar ───────────────────────────────────────────────────────

  private async publish(slug: string, page: SitePageContent): Promise<void> {
    const wait = publishDelayMs(this.lastPublishedAt(slug), Date.now());
    if (wait > 0) {
      await vscode.window.withProgress(
        { location: vscode.ProgressLocation.Window, title: t("AND: \"%s\" %d sn içinde yayınlanacak…", slug, Math.ceil(wait / 1000)) },
        () => sleep(wait)
      );
    }
    const client = await this.requireClient();
    await client.publishSitePage({ slug, title: page.title, body: page.body });
    this.lastPublish.set(slug, Date.now());

    const pages = await this.fetchPages(slug);
    if (!pages.some((p) => p.slug === slug)) {
      // Eski AND sürümleri, silinmiş (tombstone'lu) bir slug'ın yayınını hata
      // vermeden sessizce yok sayar — "yayınlandı" demeden önce doğrula.
      throw new Error(
        t(
          "AND \"%s\" sayfasını kaydetmedi — bu slug daha önce silinmiş olabilir (silinen bir slug yeniden kullanılamaz). Farklı bir slug ile yeni bir sayfa oluştur.",
          slug
        )
      );
    }
    this.drafts.delete(slug);
  }

  /**
   * Son yayın anı: bu oturumdaki kendi yayınımız ile AND'daki updated_at'ten
   * (sayfa başka bir istemciden, ör. TUI'den az önce yayınlanmış olabilir)
   * hangisi daha yeniyse.
   */
  private lastPublishedAt(slug: string): number | undefined {
    const local = this.lastPublish.get(slug);
    const page = this.pages.get(slug);
    const remote = page ? Date.parse(page.updated_at) : Number.NaN;
    if (!Number.isFinite(remote)) {
      return local;
    }
    return local === undefined ? remote : Math.max(local, remote);
  }

  /**
   * Sayfa listesini AND'dan okur ve önceki listeye göre değişen sayfalar için
   * dosya olayları yayar — açık bir editör, sayfa başka bir yerden
   * değiştirilince/silinince bunu böyle öğrenir. quietSlug için olay
   * yayılmaz: o sayfayı az önce bu sağlayıcının kendisi yazdı, VS Code zaten
   * biliyor.
   *
   * Okumalar eşzamanlı olabilir (ör. yoklama + kayıt sonrası okuma). Daha
   * sonra başlamış bir okuma zaten uygulandıysa geç gelen eski yanıt atılır —
   * yoksa önbellek yayın ÖNCESİ listeye döner ve VS Code'a giden sahte bir
   * "değişti" olayı, az önce kaydedilmiş editörü eski içerikle yeniden
   * yükletirdi.
   */
  private async fetchPages(quietSlug?: string): Promise<SitePageInfo[]> {
    const seq = ++this.fetchSeq;
    const client = await this.requireClient();
    const list = await client.getSitePages();
    if (seq < this.appliedSeq) {
      return this.sortedPages();
    }
    this.appliedSeq = seq;
    const next = new Map(list.map((p) => [p.slug, p] as const));
    const events: vscode.FileChangeEvent[] = [];
    if (this.fetchedAt > 0) {
      for (const [slug, page] of next) {
        const prev = this.pages.get(slug);
        if (slug === quietSlug || (prev && prev.updated_at === page.updated_at)) {
          continue;
        }
        events.push({ type: prev ? vscode.FileChangeType.Changed : vscode.FileChangeType.Created, uri: siteUri(slug) });
      }
      for (const slug of this.pages.keys()) {
        if (!next.has(slug) && slug !== quietSlug) {
          events.push({ type: vscode.FileChangeType.Deleted, uri: siteUri(slug) });
        }
      }
    }
    this.pages = next;
    this.fetchedAt = Date.now();
    if (events.length > 0) {
      this.fileEmitter.fire(events);
    }
    const sorted = this.sortedPages();
    this.pagesEmitter.fire(sorted);
    return sorted;
  }

  private async findPage(uri: vscode.Uri, slug: string): Promise<SitePageInfo> {
    const page = (await this.listPages()).find((p) => p.slug === slug);
    if (!page) {
      throw vscode.FileSystemError.FileNotFound(uri);
    }
    return page;
  }

  private sortedPages(): SitePageInfo[] {
    return [...this.pages.values()].sort((a, b) => a.slug.localeCompare(b.slug));
  }

  private slugOf(uri: vscode.Uri): string {
    const slug = slugFromPath(uri.path);
    if (!slug) {
      throw vscode.FileSystemError.FileNotFound(uri);
    }
    return slug;
  }

  private async requireClient(): Promise<LiveClient> {
    const client = await this.conn.ensureConnected();
    if (!client) {
      throw vscode.FileSystemError.Unavailable(
        t("AND'a canlı bağlantı yok — AND açık ve AND_ENABLE_EXTERNAL_API=1 ile başlatılmış olmalı.")
      );
    }
    return client;
  }
}
