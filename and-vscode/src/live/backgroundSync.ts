import * as vscode from "vscode";
import { LiveClient } from "./liveClient";
import { LiveConnection } from "./liveConnection";
import { ChatMsg, DMMsg, FileConsentReq, FileMsg } from "./liveTypes";

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * AND'a canlı bağlıyken, Canlı Panel açık olsun olmasın sürekli çalışan tek
 * bir DM/sohbet/dosya uzun-poll döngüsü seti. Önceden bu döngüler yalnızca
 * livePanel.ts içinde, panel açıkken çalışıyordu — panel kapalıyken gelen bir
 * DM ya da dosya isteği tamamen kaçırılıyordu. Artık panel bu olaylara
 * (onDMMsg vb.) abone olup webview'a iletiyor; extension.ts ise panel
 * kapalıyken bildirim/durum çubuğu rozeti göstermek için abone oluyor.
 */
export class BackgroundSync implements vscode.Disposable {
  private active = false;
  private readonly chatEmitter = new vscode.EventEmitter<ChatMsg>();
  private readonly dmEmitter = new vscode.EventEmitter<DMMsg>();
  private readonly fileEmitter = new vscode.EventEmitter<FileMsg>();
  private readonly consentEmitter = new vscode.EventEmitter<FileConsentReq>();

  readonly onChatMsg = this.chatEmitter.event;
  readonly onDMMsg = this.dmEmitter.event;
  readonly onFileMsg = this.fileEmitter.event;
  readonly onConsentReq = this.consentEmitter.event;

  constructor(private readonly conn: LiveConnection) {}

  start(): void {
    if (this.active) {
      return;
    }
    this.active = true;
    this.loop((c) => c.pollChat(), this.chatEmitter);
    this.loop((c) => c.pollDM(), this.dmEmitter);
    this.loop((c) => c.pollFile(), this.fileEmitter);
    this.loop((c) => c.pollConsent(), this.consentEmitter);
  }

  dispose(): void {
    this.active = false;
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
}
