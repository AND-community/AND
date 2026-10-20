import * as crypto from "crypto";
import * as path from "path";

// internal/pluginapi/externalpipe_windows.go / externalpipe_unix.go ile
// BİREBİR aynı türetme mantığı. Bağlantı noktası bir SIR değildir — erişim
// kararını OS'in kendisi (named pipe security descriptor / Unix socket
// dosya izinleri) verir; burada yalnızca AYNI adı hesaplıyoruz ki
// bağlanmayı deneyebilelim.
//
// andDataDir, DeltaX alt klasörü DEĞİL, AND'ın kendi kök veri klasörüdür
// (ör. Windows'ta %APPDATA%\and) — dataDir.ts'deki resolveAndDataDir()'in
// döndürdüğü değerin ta kendisi.

export function pipeNameFor(andDataDir: string): string {
  const hash = crypto.createHash("sha256").update(andDataDir, "utf8").digest("hex");
  return `\\\\.\\pipe\\and-plugin-api-${hash.slice(0, 16)}`;
}

export function socketPathFor(andDataDir: string): string {
  return path.join(andDataDir, "api.sock");
}

export function externalSocketPath(andDataDir: string): string {
  return process.platform === "win32" ? pipeNameFor(andDataDir) : socketPathFor(andDataDir);
}
