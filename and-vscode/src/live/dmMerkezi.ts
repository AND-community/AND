// Canlı Panel'in Doğrudan mesaj sekmesi ve durum çubuğu rozeti için DM
// durumu. AND'ın DM geçmişi ve okundu bilgisi esastır: TUI'nin ana menü
// rozeti, Özel Chat ve VS Code aynı okunmamış sayısını görür. Geçmişi ya da
// okundu bilgisi olmayan eski bir AND'da canlı gelen mesajlar ve bu
// pencerenin kendi okundu bilgisi kullanılır. "vscode" modülüne bağımlı
// değildir; dmMerkezi.test.ts ile sınanır.

import {
  GelenDM,
  Konusma,
  konusmalar,
  makineIletisiMi,
  okunmamisSayisi,
  sonGelenZamani,
  sonraMi,
} from "./dmCore";
import { LiveApiError } from "./liveClient";
import { DMRecord, SendDMReq } from "./liveTypes";

/** DM'ler için kullanılan AND uçları (LiveClient bunu karşılar). */
export interface DMIstemcisi {
  getDMHistory(from?: string): Promise<DMRecord[]>;
  getDMReadMarks(): Promise<Record<string, string>>;
  markDMRead(peerId: string, at: string): Promise<void>;
  sendDM(req: SendDMReq): Promise<void>;
}

export interface DMDurumu {
  konusmalar: Konusma[];
  okunmamis: number;
  /** Giden kutusunda bekleyen mesaj var: durumu (iletildi/gönderilemedi) değişebilir. */
  bekleyenVar: boolean;
  /** AND geçmiş tutmuyor: yalnızca bu oturumda canlı gelenler görünür. */
  gecmisYok: boolean;
}

/** Geçmişi olmayan AND'da bellekte tutulan en fazla mesaj. */
const CANLI_SINIR = 500;

function bulunamadi(err: unknown): boolean {
  return err instanceof LiveApiError && err.status === 404;
}

export class DmMerkezi {
  private gecmisYok = false;
  private okunduYok = false;
  private canli: DMRecord[] = [];
  private readonly yerelOkundu: Record<string, string> = {};

  async durum(c: DMIstemcisi): Promise<DMDurumu> {
    const kayitlar = await this.kayitlar(c);
    const okundu = await this.okundu(c);
    return {
      konusmalar: konusmalar(kayitlar, okundu),
      okunmamis: okunmamisSayisi(kayitlar, okundu),
      bekleyenVar: kayitlar.some((r) => r.outgoing && r.pending),
      gecmisYok: this.gecmisYok,
    };
  }

  /** peerId ile konuşmayı, ondan gelen son mesaja kadar okundu işaretler. İşaret ilerlediyse true. */
  async okunduIsaretle(c: DMIstemcisi, peerId: string): Promise<boolean> {
    const at = sonGelenZamani(await this.kayitlar(c), peerId);
    if (!at) {
      return false;
    }
    const okundu = await this.okundu(c);
    if (!sonraMi(at, okundu[peerId])) {
      return false;
    }
    this.yerelOkundu[peerId] = at;
    if (!this.okunduYok) {
      try {
        await c.markDMRead(peerId, at);
      } catch (err) {
        if (!bulunamadi(err)) {
          throw err;
        }
        this.okunduYok = true;
      }
    }
    return true;
  }

  /** Mesaj giden kutusuyla gider: alıcı çevrimdışıysa AND bekletir, bağlandığında iletir. */
  async gonder(c: DMIstemcisi, peerId: string, metin: string): Promise<void> {
    await c.sendDM({ peer_id: peerId, message: metin, queue_if_offline: true });
    if (this.gecmisYok) {
      this.canliEkle([{ peer_id: peerId, text: metin, at: new Date().toISOString(), outgoing: true }]);
    }
  }

  /** Canlı gelen mesajlar: yalnızca geçmişi olmayan bir AND'da saklanır (geçmiş varken AND tutar). */
  canliGeldi(dmler: GelenDM[]): void {
    if (this.gecmisYok) {
      this.canliEkle(dmler.map((d) => ({ peer_id: d.peerId, name: d.ad, text: d.text, at: d.at })));
    }
  }

  private canliEkle(kayitlar: DMRecord[]): void {
    this.canli.push(...kayitlar.filter((r) => !makineIletisiMi(r.text)));
    if (this.canli.length > CANLI_SINIR) {
      this.canli = this.canli.slice(-CANLI_SINIR);
    }
  }

  private async kayitlar(c: DMIstemcisi): Promise<DMRecord[]> {
    if (!this.gecmisYok) {
      try {
        return await c.getDMHistory();
      } catch (err) {
        if (!bulunamadi(err)) {
          throw err;
        }
        this.gecmisYok = true;
      }
    }
    return this.canli;
  }

  private async okundu(c: DMIstemcisi): Promise<Record<string, string>> {
    if (!this.okunduYok) {
      try {
        return (await c.getDMReadMarks()) ?? {};
      } catch (err) {
        if (!bulunamadi(err)) {
          throw err;
        }
        this.okunduYok = true;
      }
    }
    return this.yerelOkundu;
  }
}
