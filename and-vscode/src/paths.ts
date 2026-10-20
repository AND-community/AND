import * as path from "path";

// store.go'daki dizin yerleşimiyle bire bir aynı yollar.

export function gruplarDizini(root: string): string {
  return path.join(root, "gruplar");
}

export function projelerDizini(root: string): string {
  return path.join(root, "projeler");
}

export function grupYolu(root: string, id: string): string {
  return path.join(gruplarDizini(root), `${id}.json`);
}

export function projeKokDizini(root: string, id: string): string {
  return path.join(projelerDizini(root), id);
}

export function projeMetaYolu(root: string, id: string): string {
  return path.join(projeKokDizini(root, id), "meta.json");
}

export function projeNesneDizini(root: string, id: string): string {
  return path.join(projeKokDizini(root, id), "nesneler");
}

export function projeCalismaDizini(root: string, id: string): string {
  return path.join(projeKokDizini(root, id), "calisma");
}

export function projeBirlesmeYolu(root: string, id: string): string {
  return path.join(projeKokDizini(root, id), "MERGE_HEAD");
}
