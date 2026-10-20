import * as os from "os";
import * as path from "path";
import * as vscode from "vscode";

/**
 * AND ana uygulamasının veri klasörünü (AND_DATA_DIR) tespit eder.
 *
 * ÖNEMLİ: `AND_DATA_DIR` ortam değişkeni buradan KASITLI olarak OKUNMAZ.
 * cmd/and/main.go içindeki appDir() bu değişkeni kendi veri dizinini
 * belirlerken hiçbir zaman okumaz — bu değişken yalnızca AND'ın kendisi
 * tarafından, başlattığı eklenti alt süreçlerine (bkz.
 * internal/pluginmgr/manager.go) YAZILIR. Burada da okunacak olsaydı,
 * geliştiricinin kabuğunda (ör. bir eklenti binary'sini elle test ederken)
 * bu değişken tesadüfen tanımlıysa, uzantı AND'ın gerçekte kullandığından
 * FARKLI bir dizini kök alır ve named pipe/socket adı hiç eşleşmeyeceği
 * için canlı panel sessizce, açıklamasız şekilde hep "bağlı değil" kalırdı.
 *
 * Öncelik sırası:
 *   1. `deltax.dataDir` ayarı (kullanıcı elle override etmişse)
 *   2. İşletim sistemine özgü varsayılan yapılandırma klasörü + "and"
 *      (Go'nun os.UserConfigDir() davranışının birebir karşılığı —
 *      appDir()'ın gerçekte kullandığı tek kaynak budur)
 */
export function resolveAndDataDir(): string {
  const configured = vscode.workspace.getConfiguration("deltax").get<string>("dataDir", "").trim();
  if (configured) {
    return configured;
  }

  return path.join(userConfigDir(), "and");
}

function userConfigDir(): string {
  switch (process.platform) {
    case "win32": {
      const appData = process.env.APPDATA;
      if (appData && appData.trim()) {
        return appData;
      }
      return path.join(os.homedir(), "AppData", "Roaming");
    }
    case "darwin":
      return path.join(os.homedir(), "Library", "Application Support");
    default: {
      const xdg = process.env.XDG_CONFIG_HOME;
      if (xdg && xdg.trim()) {
        return xdg;
      }
      return path.join(os.homedir(), ".config");
    }
  }
}

/** AND_DATA_DIR/DeltaX kök klasörü. */
export function deltaXRoot(andDataDir: string): string {
  return path.join(andDataDir, "DeltaX");
}
