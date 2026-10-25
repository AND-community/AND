import * as crypto from "crypto";
import * as vscode from "vscode";
import { readCommit, readLog, shortHash } from "./commitStore";
import { makeBlobUri } from "./contentProvider";
import { trackDisposable } from "./disposableBag";
import { diffTrees, treeMap } from "./treeStore";
import { ChangeKind, DeltaXRepository } from "./scmController";
import { betikDegeri, etkin, ham, t } from "./dil/dil";

interface DiffRequestMsg {
  type: "requestDiff";
  hash: string;
}

interface OpenFileMsg {
  type: "openFile";
  hash: string;
  yol: string;
  kind: ChangeKind;
  parentHash: string;
}

type InboundMsg = DiffRequestMsg | OpenFileMsg;

interface OpenLogPanel {
  panel: vscode.WebviewPanel;
  repo: DeltaXRepository;
}

const panels = new Map<string, OpenLogPanel>();

function renderLogInto(panel: vscode.WebviewPanel, repo: DeltaXRepository): void {
  const girdiler = readLog(repo.root, repo.proje.id, repo.currentHead(), 200);
  panel.webview.html = renderHtml(
    panel.webview,
    repo.proje.ad,
    girdiler.map((g) => ({
      hash: g.hash,
      kisaHash: shortHash(g.hash),
      mesaj: g.commit.mesaj,
      yazar: g.commit.yazar,
      ts: g.commit.ts,
      ebeveynSayisi: g.commit.ebeveynler?.length ?? 0,
    }))
  );
}

export function showLogPanel(repo: DeltaXRepository, context: vscode.ExtensionContext): void {
  const existing = panels.get(repo.proje.id);
  if (existing) {
    // Aynı proje için önceki DeltaXRepository dispose edilip (ör. workspace
    // klasörü başka bir projeye geçip geri döndüğünde) yenisiyle
    // değiştirilmiş olabilir — panel bunu fark etmeden eski (artık
    // izleyicileri ölü) repo'dan içerik göstermeye devam etmesin diye
    // referansı ve içeriği güncel repo ile tazele.
    existing.repo = repo;
    renderLogInto(existing.panel, repo);
    existing.panel.reveal();
    return;
  }

  const panel = vscode.window.createWebviewPanel(
    "deltaxLog",
    t("Günlük: %s", repo.proje.ad),
    vscode.ViewColumn.Beside,
    { enableScripts: true, retainContextWhenHidden: true }
  );
  panels.set(repo.proje.id, { panel, repo });
  const forgetPanel = trackDisposable(context, panel);
  panel.onDidDispose(() => {
    panels.delete(repo.proje.id);
    forgetPanel();
  });

  renderLogInto(panel, repo);

  panel.webview.onDidReceiveMessage(async (msg: InboundMsg) => {
    if (msg.type === "requestDiff") {
      try {
        const commit = readCommit(repo.root, repo.proje.id, msg.hash);
        const parentHash = commit.ebeveynler?.[0] ?? "";
        const parentTree = parentHash ? readCommit(repo.root, repo.proje.id, parentHash).agac : "";
        const durum = diffTrees(repo.root, repo.proje.id, parentTree, commit.agac);
        void panel.webview.postMessage({ type: "diffResult", hash: msg.hash, parentHash, durum });
      } catch (err) {
        void panel.webview.postMessage({
          type: "diffError",
          hash: msg.hash,
          message: err instanceof Error ? err.message : String(err),
        });
      }
      return;
    }
    if (msg.type === "openFile") {
      await openFileAtCommit(repo, msg.yol, msg.kind, msg.parentHash, msg.hash);
    }
  });
}

/**
 * Açık günlük panelinin içeriğini (kapatmadan) tazeler — ör. bir
 * commit/birleştirmeden sonra. `repo` her zaman ÇAĞIRANIN o an elindeki
 * güncel DeltaXRepository olmalıdır (ör. bir commit'i az önce
 * gerçekleştiren repo) — bu, panelin önbelleklediği repo referansını da
 * güncel tutar (bkz. showLogPanel'deki aynı örüntü).
 */
export function refreshLogPanel(repo: DeltaXRepository): void {
  const entry = panels.get(repo.proje.id);
  if (entry) {
    entry.repo = repo;
    renderLogInto(entry.panel, repo);
  }
}

/** Dil değişti: açık günlük panelleri etkin dilde yeniden çizilir. */
export function redrawLogPanels(): void {
  for (const entry of panels.values()) {
    entry.panel.title = t("Günlük: %s", entry.repo.proje.ad);
    renderLogInto(entry.panel, entry.repo);
  }
}

async function openFileAtCommit(
  repo: DeltaXRepository,
  yol: string,
  kind: ChangeKind,
  parentHash: string,
  hash: string
): Promise<void> {
  const parentTreeHash = parentHash ? readCommit(repo.root, repo.proje.id, parentHash).agac : "";
  const childTreeHash = readCommit(repo.root, repo.proje.id, hash).agac;
  const parentEntry = parentTreeHash ? treeMap(repo.root, repo.proje.id, parentTreeHash).get(yol) : undefined;
  const childEntry = treeMap(repo.root, repo.proje.id, childTreeHash).get(yol);

  if (kind === "degisen" && parentEntry && childEntry) {
    const before = makeBlobUri(repo.proje.id, yol, parentEntry.hash, `${yol} (${shortHash(parentHash)})`);
    const after = makeBlobUri(repo.proje.id, yol, childEntry.hash, `${yol} (${shortHash(hash)})`);
    await vscode.commands.executeCommand("vscode.diff", before, after, `${yol}: ${shortHash(parentHash)} ↔ ${shortHash(hash)}`);
    return;
  }
  if (kind === "eklenen" && childEntry) {
    const after = makeBlobUri(repo.proje.id, yol, childEntry.hash, `${yol} (${shortHash(hash)})`);
    await vscode.commands.executeCommand("vscode.open", after, { preview: true });
    return;
  }
  if (kind === "silinen" && parentEntry) {
    const before = makeBlobUri(repo.proje.id, yol, parentEntry.hash, `${yol} (${t("silinen — %s", shortHash(parentHash))})`);
    await vscode.commands.executeCommand("vscode.open", before, { preview: true });
  }
}

interface LogRow {
  hash: string;
  kisaHash: string;
  mesaj: string;
  yazar: string;
  ts: string;
  ebeveynSayisi: number;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function renderHtml(webview: vscode.Webview, projeAd: string, rows: LogRow[]): string {
  const nonce = crypto.randomBytes(16).toString("hex");
  const rowsHtml = rows
    .map(
      (r) => `<tr class="commit-row" data-hash="${r.hash}" data-search="${escapeHtml(
        (r.mesaj + " " + r.yazar + " " + r.hash).toLowerCase()
      )}">
        <td class="hash">${r.kisaHash}${r.ebeveynSayisi > 1 ? ' <span class="merge-badge">merge</span>' : ""}</td>
        <td class="msg">${escapeHtml(r.mesaj)}</td>
        <td class="author">${escapeHtml(r.yazar)}</td>
        <td class="ts">${escapeHtml(formatTs(r.ts))}</td>
      </tr>
      <tr class="detail-row" data-hash="${r.hash}" style="display:none">
        <td colspan="4" class="detail-cell"><div class="detail-body">${escapeHtml(t("Yükleniyor…"))}</div></td>
      </tr>`
    )
    .join("\n");

  return `<!doctype html>
<html lang="${etkin()}">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
<style>
  body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); padding: 0 12px; }
  h2 { font-weight: 600; }
  #search { width: 100%; box-sizing: border-box; padding: 6px 8px; margin: 8px 0; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border, transparent); border-radius: 4px; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  td { padding: 4px 8px; border-bottom: 1px solid var(--vscode-widget-border, rgba(128,128,128,0.2)); vertical-align: top; }
  .commit-row { cursor: pointer; }
  .commit-row:hover { background: var(--vscode-list-hoverBackground); }
  .hash { font-family: var(--vscode-editor-font-family, monospace); color: var(--vscode-textLink-foreground); white-space: nowrap; }
  .merge-badge { font-size: 10px; padding: 1px 5px; border-radius: 8px; background: var(--vscode-badge-background); color: var(--vscode-badge-foreground); }
  .author, .ts { white-space: nowrap; opacity: 0.85; }
  .detail-cell { padding: 0 8px 10px 28px; }
  .detail-body ul { margin: 4px 0; padding-left: 16px; }
  .detail-body li { cursor: pointer; padding: 2px 0; }
  .detail-body li:hover { text-decoration: underline; }
  .kind-eklenen { color: var(--vscode-gitDecoration-addedResourceForeground, #4caf50); }
  .kind-degisen { color: var(--vscode-gitDecoration-modifiedResourceForeground, #e2a03f); }
  .kind-silinen { color: var(--vscode-gitDecoration-deletedResourceForeground, #f44336); text-decoration: line-through; }
  .empty { opacity: 0.7; padding: 8px 0; }
</style>
</head>
<body>
  <h2>${escapeHtml(t("%s — Commit Günlüğü", projeAd))}</h2>
  <input id="search" type="text" placeholder="${escapeHtml(t("Mesaj, yazar ya da hash'e göre ara…"))}" />
  <table>
    <thead><tr><td><b>${escapeHtml(t("Hash"))}</b></td><td><b>${escapeHtml(t("Mesaj"))}</b></td><td><b>${escapeHtml(t("Yazar"))}</b></td><td><b>${escapeHtml(t("Zaman"))}</b></td></tr></thead>
    <tbody id="rows">
      ${rowsHtml || `<tr><td colspan="4" class="empty">${escapeHtml(t("Henüz commit yok."))}</td></tr>`}
    </tbody>
  </table>
<script nonce="${nonce}">
  const vscode = acquireVsCodeApi();
  const search = document.getElementById('search');
  search.addEventListener('input', () => {
    const q = search.value.trim().toLowerCase();
    document.querySelectorAll('.commit-row').forEach(row => {
      const hit = !q || row.dataset.search.includes(q);
      row.style.display = hit ? '' : 'none';
      const detail = document.querySelector('.detail-row[data-hash="' + row.dataset.hash + '"]');
      if (detail && !hit) { detail.style.display = 'none'; }
    });
  });

  document.querySelectorAll('.commit-row').forEach(row => {
    row.addEventListener('click', () => {
      const hash = row.dataset.hash;
      const detail = document.querySelector('.detail-row[data-hash="' + hash + '"]');
      if (!detail) return;
      const open = detail.style.display !== 'none';
      if (open) { detail.style.display = 'none'; return; }
      detail.style.display = '';
      const body = detail.querySelector('.detail-body');
      if (body.dataset.loaded === '1') return;
      vscode.postMessage({ type: 'requestDiff', hash });
    });
  });

  window.addEventListener('message', (event) => {
    const msg = event.data;
    const detail = document.querySelector('.detail-row[data-hash="' + msg.hash + '"]');
    if (!detail) return;
    const body = detail.querySelector('.detail-body');
    if (msg.type === 'diffError') {
      body.textContent = ${betikDegeri(ham("Fark hesaplanamadı: %s"))}.replace('%s', () => msg.message);
      return;
    }
    if (msg.type === 'diffResult') {
      body.dataset.loaded = '1';
      body.dataset.parent = msg.parentHash || '';
      const list = [];
      for (const yol of msg.durum.eklenen) list.push({ yol, kind: 'eklenen', label: '+ ' + yol });
      for (const yol of msg.durum.degisen) list.push({ yol, kind: 'degisen', label: '~ ' + yol });
      for (const yol of msg.durum.silinen) list.push({ yol, kind: 'silinen', label: '- ' + yol });
      if (list.length === 0) {
        body.innerHTML = '<span class="empty">' + ${betikDegeri(escapeHtml(t("Değişiklik yok.")))} + '</span>';
        return;
      }
      const ul = document.createElement('ul');
      for (const item of list) {
        const li = document.createElement('li');
        li.className = 'kind-' + item.kind;
        li.textContent = item.label;
        li.addEventListener('click', (ev) => {
          ev.stopPropagation();
          vscode.postMessage({ type: 'openFile', hash: msg.hash, parentHash: body.dataset.parent, yol: item.yol, kind: item.kind });
        });
        ul.appendChild(li);
      }
      body.innerHTML = '';
      body.appendChild(ul);
    }
  });
</script>
</body>
</html>`;
}

function formatTs(ts: string): string {
  const d = new Date(ts);
  if (isNaN(d.getTime())) {
    return ts;
  }
  return d.toLocaleString(etkin());
}
