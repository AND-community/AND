import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import JSZip from "jszip";
import { projeKokDizini } from "./paths";

// export.go'daki projeDisaAktar'ın TypeScript karşılığı: proje kök dizinini
// (meta.json, nesneler/, calisma/) "<proje adı>-<zaman>.zip" olarak paketler.
// Tamamen yerel bir işlemdir, ağ trafiği üretmez.

function sanitizeFileName(ad: string): string {
  let out = "";
  for (const ch of ad) {
    out += "/\\:*?\"<>|".includes(ch) ? "_" : ch;
  }
  out = out.trim();
  return out || "proje";
}

function timestamp(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-` +
    `${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
  );
}

function addDirToZip(zip: JSZip, dir: string, base: string): void {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      addDirToZip(zip, full, base);
      continue;
    }
    if (!entry.isFile()) {
      continue;
    }
    const rel = path.relative(base, full).split(path.sep).join("/");
    zip.file(rel, fs.readFileSync(full));
  }
}

export function defaultExportFolder(): string {
  const home = os.homedir();
  const desktop = path.join(home, "Desktop");
  try {
    if (fs.statSync(desktop).isDirectory()) {
      return desktop;
    }
  } catch {
    // Desktop yok — ev dizinine düş
  }
  return home;
}

/** projeDisaAktar — proje kökünü hedefKlasor altında bir .zip'e paketler ve dosya yolunu döndürür. */
export async function exportProject(
  root: string,
  projeId: string,
  projeAd: string,
  hedefKlasor: string
): Promise<string> {
  const kok = projeKokDizini(root, projeId);
  if (!fs.existsSync(kok)) {
    throw new Error(`proje bulunamadı: ${kok}`);
  }
  if (!hedefKlasor.trim()) {
    throw new Error("hedef klasör boş olamaz");
  }
  fs.mkdirSync(hedefKlasor, { recursive: true, mode: 0o700 });

  const hedefDosya = path.join(hedefKlasor, `${sanitizeFileName(projeAd)}-${timestamp()}.zip`);
  const zip = new JSZip();
  addDirToZip(zip, kok, kok);
  const content = await zip.generateAsync({ type: "nodebuffer", compression: "DEFLATE" });
  fs.writeFileSync(hedefDosya, content);
  return hedefDosya;
}
