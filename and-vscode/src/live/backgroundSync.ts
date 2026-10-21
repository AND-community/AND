import * as vscode from "vscode";
import { GelenDM, akisAdimi, gelenKayittan, gelenYoklamadan, imleciOku, jsonYaz } from "./dmCore";
import { PencereLideri } from "./lider";
import { LiveApiError, LiveClient } from "./liveClient";
import { LiveConnection } from "./liveConnection";
import { ChatMsg, FileConsentReq, FileMsg } from "./liveTypes";

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Açıkken AND'ın DM geçmişinin yeniden okunma aralığı. */
const DM_ARALIGI_MS = 2000;

/**
 * AND'a canlı bağlıyken, Canlı Panel açık olsun olmasın sürekli çalışan
 * sohbet/dosya uzun-poll döngüleri ve DM akışı. Panel bu olaylara abone olur;
 * extension.ts ise bildirim/durum çubuğu rozeti ve DeltaX için.
 *
 * DM'ler AND'ın DM geçmişinden okunur (/dm/poll yalnızca istek açıkken
 * gelenleri görür: iki yoklama arasında ya da VS Code kapalıyken gelen DM
 * kaybolurdu). Nereye kadar işlendiği bir imleçte (bkz. dmCore.ts) tutulur.
 * Birden çok pencere açıksa geçmişi yalnızca lider pencere işler (bkz.
 * lider.ts). Geçmişi olmayan eski bir AND'da canlı dinlemeye düşülür.
 */
export class BackgroundSync implements vscode.Disposable {
  private active = false;
  private readonly chatEmitter = new vscode.EventEmitter<ChatMsg>();
  private readonly dmEmitter = new vscode.EventEmitter<GelenDM[]>();
  private readonly fileEmitter = new vscode.EventEmitter<FileMsg>();
  private readonly consentEmitter = new vscode.EventEmitter<FileConsentReq>();

  readonly onChatMsg = this.chatEmitter.event;
  /** Yeni gelen DM'ler, geliş sırasıyla (DeltaX zarfları dahil). */
  readonly onDMBatch = this.dmEmitter.event;
  readonly onFileMsg = this.fileEmitter.event;
  readonly onConsentReq = this.consentEmitter.event;

  constructor(
    private readonly conn: LiveConnection,
    private readonly imlecDosyasi: () => string,
    private readonly lider: PencereLideri
  ) {}

  start(): void {
    if (this.active) {
      return;
    }
    this.active = true;
    this.loop((c) => c.pollChat(), this.chatEmitter);
    this.loop((c) => c.pollFile(), this.fileEmitter);
    this.loop((c) => c.pollConsent(), this.consentEmitter);
    void this.dmLoop();
  }

  dispose(): void {
    this.active = false;
    this.lider.birak();
  }

  private loop<T>(poll: (c: LiveClient) => Promise<T[]>, emitter: vscode.EventEmitter<T>): void {
    void (async () => {
      while (this.active) {
        const client = this.conn.getClient();
        if (!client) {
          await sleep(3000);
          continue;
        }
        try {
          const msgs = await poll(client);
          for (const m of msgs) {
            emitter.fire(m);
          }
        } catch {
          await sleep(3000);
        }
      }
    })();
  }

  private async dmLoop(): Promise<void> {
    let gecmisVar = true;
    while (this.active) {
      const client = this.conn.getClient();
      if (!client) {
        await sleep(3000);
        continue;
      }
      if (!gecmisVar) {
        await this.canliDinle(client);
        continue;
      }
      if (!this.lider.al()) {
        await sleep(DM_ARALIGI_MS); // başka bir pencere işliyor
        continue;
      }
      try {
        // İmleç her turda dosyadan okunur: lider başka bir pencereye geçmiş olabilir.
        const imlec = imleciOku(this.imlecDosyasi());
        const istendi = new Date();
        const adim = akisAdimi(imlec, await client.getDMHistory(imlec.id), istendi);
        if (adim.degisti) {
          jsonYaz(this.imlecDosyasi(), adim.imlec);
        }
        const gelenler = adim.yeniler.filter((r) => !r.outgoing).map(gelenKayittan);
        if (gelenler.length > 0) {
          this.dmEmitter.fire(gelenler);
        }
      } catch (err) {
        if (err instanceof LiveApiError && err.status === 404) {
          gecmisVar = false;
          continue;
        }
      }
      await sleep(DM_ARALIGI_MS);
    }
  }

  /** Geçmişi olmayan eski AND: yalnızca istek açıkken gelenler görülür. */
  private async canliDinle(client: LiveClient): Promise<void> {
    try {
      const msgs = await client.pollDM();
      if (msgs.length > 0) {
        const simdi = new Date();
        this.dmEmitter.fire(msgs.map((m) => gelenYoklamadan(m, simdi)));
      }
    } catch {
      await sleep(3000);
    }
  }
}
