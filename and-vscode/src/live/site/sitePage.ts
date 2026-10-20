// Kişisel site sayfaları için SAF yardımcılar — vscode'a bağımlı değildir,
// node:test ile doğrudan test edilir (bkz. sitePage.test.ts). AND'ın
// internal/site/site.go'sundaki sınırların ve slug kuralının karşılığı,
// editörde gösterilen belge biçimi (serializePage/parsePage), paylaşım
// kodu/bağlantısı ve yayın aralığı hesabı burada.
//
// Gerçek/otoriter denetim her zaman AND'dadır (internal/site.Store.
// PublishPage) — buradaki kontroller yalnızca ağa hiç çıkmadan anında,
// anlaşılır geri bildirim içindir (Eklentiler/site'ın kaydet()'iyle aynı
// ilke).

/** internal/site/site.go'daki MaxSlugBytes / MaxTitleBytes / MaxBodyBytes / MaxPagesPerAuthor. */
export const SITE_MAX_SLUG_BYTES = 40;
export const SITE_MAX_TITLE_BYTES = 100;
export const SITE_MAX_BODY_BYTES = 8000;
export const SITE_MAX_PAGES = 20;

/**
 * internal/site.minPageEditInterval (5 sn): AND eşleri AYNI (yazar, slug)
 * için canlı gossip'ten gelen bir güncellemeyi kabul ettikten sonraki 5 sn
 * içinde gelen bir sonrakini DÜŞÜRÜR. Kendi yayınlarımızı bundan biraz daha
 * seyrek tutuyoruz ki ağ gecikmesindeki küçük farklar bile bir düzenlemenin
 * eşlerde düşmesine yol açmasın.
 */
export const SITE_PUBLISH_SPACING_MS = 6000;

/** Editörde açılan sayfa belgelerinin uzantısı — package.json'daki "and-site" diline bağlıdır. */
export const SITE_DOC_EXTENSION = ".andsite";

// site.go'daki reSlug'ın birebir aynısı: küçük harf, rakam, tire; baş/son
// karakter tire olamaz.
const SLUG_RE = new RegExp(`^[a-z0-9]([a-z0-9-]{0,${SITE_MAX_SLUG_BYTES - 2}}[a-z0-9])?$`);

export function isValidSlug(slug: string): boolean {
  return SLUG_RE.test(slug);
}

/** Go'nun len(string)'i gibi UTF-8 BAYT uzunluğu — "ğ", "ş" gibi Türkçe karakterler 2 bayttır. */
export function utf8Bytes(s: string): number {
  return Buffer.byteLength(s, "utf8");
}

export interface SitePageContent {
  title: string;
  body: string;
}

/**
 * Bir sayfayı editörde gösterilen belgeye çevirir: ilk satır "# Başlık",
 * ardından bir boş satır, ardından gövde. Başlık tek satırlıktır (TUI'deki
 * başlık alanı gibi); API üzerinden satır sonu içeren bir başlık
 * yayınlanmışsa burada boşluğa çevrilir — yoksa parsePage'in geri dönüşü
 * başlığın bir kısmını gövdeye kaydırırdı.
 */
export function serializePage(page: SitePageContent): string {
  return `# ${page.title.replace(/\r\n|\r|\n/g, " ")}\n\n${page.body}`;
}

/**
 * serializePage'in tersi; hoşgörülüdür: ilk satırdaki "# " (ya da yalnızca
 * "#") öneki yoksa satırın tamamı başlıktır, ayırıcı boş satır yoksa gövde
 * doğrudan ikinci satırdan başlar. CRLF satır sonları LF'ye çevrilir (AND'ın
 * TUI'si de LF kullanır) ve — Eklentiler/site'ın kaydet()'iyle aynı şekilde —
 * başlığın ve gövdenin baş/son boşlukları kırpılır; yani dönen değer tam
 * olarak AND'a gönderilecek olandır.
 */
export function parsePage(text: string): SitePageContent {
  const { titleLine, bodyStart } = splitDocument(text);
  return {
    title: titleFromLine(titleLine).trim(),
    body: text.slice(bodyStart).replace(/\r\n/g, "\n").trim(),
  };
}

function splitDocument(text: string): { titleLine: string; bodyStart: number } {
  const nl = text.indexOf("\n");
  if (nl < 0) {
    return { titleLine: text, bodyStart: text.length };
  }
  let bodyStart = nl + 1;
  if (text.startsWith("\r\n", bodyStart)) {
    bodyStart += 2;
  } else if (text.startsWith("\n", bodyStart)) {
    bodyStart += 1;
  }
  return { titleLine: text.slice(0, nl).replace(/\r$/, ""), bodyStart };
}

function titleFromLine(line: string): string {
  if (line.startsWith("# ")) {
    return line.slice(2);
  }
  return line.startsWith("#") ? line.slice(1) : line;
}

/**
 * Ağa çıkmadan önce Eklentiler/site'ın kaydet()'iyle aynı kurallar: slug
 * biçimi, boş olmayan gövde ve bayt sınırları. İlk sorunun Türkçe
 * açıklamasını, sorun yoksa undefined döner.
 */
export function validatePage(slug: string, page: SitePageContent): string | undefined {
  if (!isValidSlug(slug)) {
    return `Geçersiz slug "${slug}" — yalnızca küçük harf, rakam ve tire (baş/son tire olamaz), en fazla ${SITE_MAX_SLUG_BYTES} karakter.`;
  }
  if (!page.body) {
    return "Sayfa içeriği boş olamaz — başlık satırının altına bir şeyler yaz.";
  }
  const titleBytes = utf8Bytes(page.title);
  if (titleBytes > SITE_MAX_TITLE_BYTES) {
    return `Başlık en fazla ${SITE_MAX_TITLE_BYTES} bayt olabilir (şu an ${titleBytes} bayt).`;
  }
  const bodyBytes = utf8Bytes(page.body);
  if (bodyBytes > SITE_MAX_BODY_BYTES) {
    return `Sayfa en fazla ${SITE_MAX_BODY_BYTES} bayt olabilir (şu an ${bodyBytes} bayt).`;
  }
  return undefined;
}

/** "Yeni Sayfa" slug girişinin doğrulaması (showInputBox'ın validateInput'u için). */
export function validateNewSlug(slug: string, existingSlugs: readonly string[]): string | undefined {
  if (!slug) {
    return "Slug boş olamaz.";
  }
  if (!isValidSlug(slug)) {
    return `Yalnızca küçük harf, rakam ve tire (baş/son tire olamaz), en fazla ${SITE_MAX_SLUG_BYTES} karakter.`;
  }
  if (existingSlugs.includes(slug)) {
    return `"${slug}" adlı bir sayfan zaten var — onu düzenlemek için "Site Sayfası Aç" komutunu kullan.`;
  }
  return undefined;
}

/**
 * Yayınlanacak gövde sınırı aşıyorsa, HAM belgede sınırın aşıldığı ilk
 * karakterin ofsetini döner (editörde o noktadan itibaren işaretlemek
 * için); aşmıyorsa undefined. parsePage'in dönüşümlerini (CRLF→LF, baştaki
 * ve sondaki boşlukların kırpılması) hesaba katar — yani tam olarak AND'a
 * gönderilecek baytları sayar.
 */
export function bodyOverflowOffset(text: string, limit = SITE_MAX_BODY_BYTES): number | undefined {
  if (utf8Bytes(parsePage(text).body) <= limit) {
    return undefined;
  }
  let i = splitDocument(text).bodyStart;
  while (i < text.length && /\s/.test(text[i])) {
    i++; // kırpılan baştaki boşluklar sayılmaz
  }
  let bytes = 0;
  while (i < text.length) {
    if (text.startsWith("\r\n", i)) {
      bytes += 1; // LF'ye çevrilir
      if (bytes > limit) {
        return i;
      }
      i += 2;
      continue;
    }
    const cp = text.codePointAt(i) ?? 0;
    bytes += cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
    if (bytes > limit) {
      return i;
    }
    i += cp >= 0x10000 ? 2 : 1;
  }
  return undefined;
}

/** AND Arama Motoru'nun "Kodla Git" ekranının ve -site bayrağının kabul ettiği paylaşım kodu. */
export function shareCode(authorKey: string, slug: string): string {
  return `${authorKey}/${slug}`;
}

/** Arama Motoru'nun protokol.go'sundaki siteURI ile aynı tıklanabilir bağlantı. */
export function shareLink(authorKey: string, slug: string): string {
  return `and-site://${shareCode(authorKey, slug)}`;
}

/** "/hakkimda.andsite" → "hakkimda"; biçime uymayan bir yol için undefined. */
export function slugFromPath(path: string): string | undefined {
  if (!path.startsWith("/") || !path.endsWith(SITE_DOC_EXTENSION)) {
    return undefined;
  }
  const slug = path.slice(1, -SITE_DOC_EXTENSION.length);
  return isValidSlug(slug) ? slug : undefined;
}

export function pathForSlug(slug: string): string {
  return `/${slug}${SITE_DOC_EXTENSION}`;
}

/**
 * Bir sonraki yayından önce ne kadar beklenmesi gerektiği (ms) — son
 * yayından bu yana SITE_PUBLISH_SPACING_MS geçmediyse aradaki fark. Bekleme
 * hiçbir zaman spacingMs'i aşmaz: ileri tarihli bir zaman damgası (ör. saat
 * düzeltmesi) kaydetmeyi sınırsızca geciktiremez.
 */
export function publishDelayMs(lastPublishMs: number | undefined, nowMs: number, spacingMs = SITE_PUBLISH_SPACING_MS): number {
  if (lastPublishMs === undefined || !Number.isFinite(lastPublishMs)) {
    return 0;
  }
  return Math.min(spacingMs, Math.max(0, lastPublishMs + spacingMs - nowMs));
}
