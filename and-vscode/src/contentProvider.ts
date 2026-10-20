import * as vscode from "vscode";
import { readObject } from "./objectStore";

/**
 * "deltax" URI şeması: DeltaX'in içerik-adresli nesne deposundaki bir blob'u
 * (HEAD'teki ya da geçmişteki herhangi bir commit'teki dosya sürümü) VS
 * Code'un yerleşik diff editöründe salt-okunur bir belge olarak gösterir.
 *
 * URI biçimi: deltax:/<projeId>/<göreli-yol>?hash=<blobHash>&label=<etiket>
 */
export const DELTAX_SCHEME = "deltax";

export function makeBlobUri(projeId: string, rel: string, hash: string, label: string): vscode.Uri {
  const query = `hash=${encodeURIComponent(hash)}&label=${encodeURIComponent(label)}`;
  return vscode.Uri.from({
    scheme: DELTAX_SCHEME,
    path: `/${projeId}/${rel}`,
    query,
  });
}

function isBinary(buf: Buffer): boolean {
  const len = Math.min(buf.length, 8000);
  for (let i = 0; i < len; i++) {
    if (buf[i] === 0) {
      return true;
    }
  }
  return false;
}

export class DeltaXContentProvider implements vscode.TextDocumentContentProvider {
  private readonly changeEmitter = new vscode.EventEmitter<vscode.Uri>();
  readonly onDidChange = this.changeEmitter.event;

  constructor(private getRoot: () => string) {}

  provideTextDocumentContent(uri: vscode.Uri): string {
    const parts = uri.path.split("/").filter(Boolean);
    const projeId = parts[0];
    const params = new URLSearchParams(uri.query);
    const hash = params.get("hash") ?? "";
    if (!projeId || !hash) {
      return "";
    }
    try {
      const { payload } = readObject(this.getRoot(), projeId, hash);
      if (isBinary(payload)) {
        return "(ikili dosya — önizleme desteklenmiyor)";
      }
      return payload.toString("utf8");
    } catch (err) {
      return `(nesne okunamadı: ${err instanceof Error ? err.message : String(err)})`;
    }
  }
}
