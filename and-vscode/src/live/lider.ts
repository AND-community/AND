// Birden çok VS Code penceresi açıkken AND'ın DM geçmişini yalnızca BİRİ
// işler; aksi halde her pencere aynı DM için ayrı bildirim gösterir ve
// DeltaX'in bir çek isteğine iki kez paket gönderilir. Lider, ortak bir
// dosyaya düzenli olarak "buradayım" yazar; kayıt bayatlarsa (pencere
// kapandı ya da dondu) başka bir pencere devralır. "vscode" modülüne
// bağımlı değildir; lider.test.ts ile sınanır.

import * as crypto from "crypto";
import * as fs from "fs";
import { jsonOku, jsonYaz } from "./dmCore";

/** Liderin kaydını tazelemediği bu süreden sonra başka bir pencere devralır. */
export const LIDER_SURESI_MS = 15_000;

interface LiderKaydi {
  sahip?: string;
  zaman?: number;
}

export class PencereLideri {
  readonly kimlik = crypto.randomBytes(8).toString("hex");

  constructor(
    private readonly dosya: () => string,
    private readonly saat: () => number = Date.now,
    private readonly sure = LIDER_SURESI_MS
  ) {}

  /**
   * Bu pencere işi üstlenebilir mi? Üstlenirse kaydı tazeler. İki pencere
   * aynı anda üstlenirse bir sonraki turda dosyaya son yazan kalır.
   */
  al(): boolean {
    const k = jsonOku<LiderKaydi>(this.dosya(), {});
    const simdi = this.saat();
    const baskasi = !!k?.sahip && k.sahip !== this.kimlik;
    if (baskasi && typeof k.zaman === "number" && simdi - k.zaman < this.sure) {
      return false;
    }
    jsonYaz(this.dosya(), { sahip: this.kimlik, zaman: simdi });
    return true;
  }

  /** Pencere kapanırken: lider bu pencereyse başka bir pencere beklemeden devralsın. */
  birak(): void {
    if (jsonOku<LiderKaydi>(this.dosya(), {})?.sahip === this.kimlik) {
      try {
        fs.rmSync(this.dosya(), { force: true });
      } catch {
        // yok say
      }
    }
  }
}
