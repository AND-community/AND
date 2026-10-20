import * as path from "path";
import { listGroups, listProjects } from "./groupProjectStore";
import { projeCalismaDizini } from "./paths";
import { Grup, Proje } from "./types";

export interface GroupedProject {
  grup: Grup;
  proje: Proje;
}

export function listAllProjects(root: string): GroupedProject[] {
  const out: GroupedProject[] = [];
  for (const grup of listGroups(root)) {
    for (const proje of listProjects(root, grup.id)) {
      out.push({ grup, proje });
    }
  }
  return out;
}

function normalize(p: string): string {
  const resolved = path.resolve(p);
  return process.platform === "win32" ? resolved.toLowerCase() : resolved;
}

/** Verilen klasör yolunun bilinen bir DeltaX projesinin çalışma klasörü olup olmadığını bulur. */
export function findProjectForFolder(root: string, folderFsPath: string): GroupedProject | undefined {
  const target = normalize(folderFsPath);
  for (const gp of listAllProjects(root)) {
    if (normalize(projeCalismaDizini(root, gp.proje.id)) === target) {
      return gp;
    }
  }
  return undefined;
}
