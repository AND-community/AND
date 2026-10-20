import * as vscode from "vscode";
import { externalSocketPath } from "./liveAddr";
import { LiveClient } from "./liveClient";

const POLL_INTERVAL_MS = 10_000;

/**
 * AND'ın canlı yerel API'sine (named pipe / Unix socket) bağlantı durumunu
 * yönetir. AND kapalıyken ya da AND_ENABLE_EXTERNAL_API=1 ayarlanmamışken
 * bağlantı basitçe başarısız olur — bu normaldir, DeltaX'in kendi yerel
 * dosya tabanlı özellikleri bundan etkilenmez.
 */
export class LiveConnection implements vscode.Disposable {
  private client: LiveClient | undefined;
  private timer: ReturnType<typeof setInterval> | undefined;
  private connected = false;
  private readonly stateEmitter = new vscode.EventEmitter<boolean>();
  readonly onDidChangeConnection = this.stateEmitter.event;

  constructor(private readonly andDataDir: () => string) {}

  start(): void {
    void this.attemptConnect();
    this.timer = setInterval(() => void this.attemptConnect(), POLL_INTERVAL_MS);
  }

  dispose(): void {
    if (this.timer) {
      clearInterval(this.timer);
    }
  }

  isConnected(): boolean {
    return this.connected;
  }

  getClient(): LiveClient | undefined {
    return this.connected ? this.client : undefined;
  }

  /** Şu anda bağlı değilse hemen bir kez daha dener — komutlar için kullanışlı. */
  async ensureConnected(): Promise<LiveClient | undefined> {
    if (this.connected && this.client) {
      return this.client;
    }
    await this.attemptConnect();
    return this.getClient();
  }

  private async attemptConnect(): Promise<void> {
    const socketPath = externalSocketPath(this.andDataDir());
    const candidate = new LiveClient(socketPath);
    try {
      await candidate.getIdentity();
      this.client = candidate;
      this.setConnected(true);
    } catch {
      this.client = undefined;
      this.setConnected(false);
    }
  }

  private setConnected(value: boolean): void {
    if (this.connected !== value) {
      this.connected = value;
      this.stateEmitter.fire(value);
    }
  }
}
