// Forumun canlı değişimlerini AND'ın uzun yoklama ucundan
// (/api/v1/forum/changes) izler. "vscode" modülüne bağımlı değildir.

import { LiveApiError } from "../liveClient";
import { ForumChanges } from "../liveTypes";

export interface DegisimIstemcisi {
  forumChanges(since: number): Promise<ForumChanges>;
}

/** Değişen konuların kimlikleri; hepsi doğruysa liste baştan yüklenmelidir. */
export interface ForumDegisimi {
  konular: string[];
  hepsi: boolean;
}

/** Bir hatadan sonra yeniden denemeden önce beklenen süre. */
const HATA_BEKLEMESI_MS = 5000;

const uyu = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));

/**
 * ForumIzleyici, panel açıkken forumdaki değişiklikleri bildirir. AND'ın
 * sayacı 0'dan başlar ve since=0 ilk değişikliği bekler; bu yüzden ilk yanıt
 * da bir değişiklik sayılır (açılışta en fazla bir fazladan yenileme, ama
 * hiçbir değişiklik kaçmaz). Değişim ucu olmayan eski bir AND'da (404/501)
 * durur ve eskiAnd'ı çağırır; başka hatalarda bekleyip yeniden dener.
 */
export class ForumIzleyici {
  private nesil = 0;
  private _calisiyor = false;

  constructor(
    private readonly istemci: DegisimIstemcisi,
    private readonly degisti: (d: ForumDegisimi) => void,
    private readonly bekle: (ms: number) => Promise<void> = uyu,
    private readonly eskiAnd?: () => void
  ) {}

  get calisiyor(): boolean {
    return this._calisiyor;
  }

  baslat(): void {
    if (this._calisiyor) {
      return;
    }
    this._calisiyor = true;
    void this.dongu(++this.nesil);
  }

  durdur(): void {
    this._calisiyor = false;
    this.nesil++;
  }

  private async dongu(nesil: number): Promise<void> {
    let since = 0;
    while (nesil === this.nesil) {
      let d: ForumChanges;
      try {
        d = await this.istemci.forumChanges(since);
      } catch (err) {
        if (nesil !== this.nesil) {
          return;
        }
        if (err instanceof LiveApiError && (err.status === 404 || err.status === 501)) {
          this._calisiyor = false;
          this.nesil++;
          this.eskiAnd?.();
          return;
        }
        await this.bekle(HATA_BEKLEMESI_MS);
        continue;
      }
      if (nesil !== this.nesil) {
        return;
      }
      const konular = d.konular ?? [];
      if (d.hepsi || konular.length > 0) {
        this.degisti({ konular, hepsi: !!d.hepsi });
      }
      since = d.surum;
    }
  }
}
