import * as os from "os";
import * as vscode from "vscode";

/** Commit'lerde "yazar" olarak kullanılacak ad. AND'in kendi nickname'inden
 * bağımsızdır (bu uzantı AND'in canlı P2P/HTTP API'sine bağlanmaz) —
 * `deltax.authorName` ile override edilebilir, öntanımlı olarak işletim
 * sistemi kullanıcı adıdır. */
export function authorName(): string {
  const configured = vscode.workspace.getConfiguration("deltax").get<string>("authorName", "").trim();
  if (configured) {
    return configured;
  }
  try {
    return os.userInfo().username;
  } catch {
    return "bilinmeyen";
  }
}
