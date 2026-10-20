import * as zlib from "zlib";

// Bağımlılıksız, minimal USTAR tar okuyucu/yazıcı + gzip (Node'un yerleşik
// zlib'i Go'nun compress/gzip'iyle tamamen uyumludur). Yalnızca sync.go'nun
// ürettiği/beklediği basit girdileri (kısa ad, düz dosya) hedefler — dizin,
// sembolik bağ, uzun ad (>100 bayt) ya da GNU sparse desteği YOKTUR; bunlara
// paketOlustur/paketUygula da hiç ihtiyaç duymaz.

const BLOCK = 512;

export interface TarEntry {
  name: string;
  data: Buffer;
}

function padOctal(value: number, len: number): Buffer {
  const digits = value.toString(8).padStart(len - 1, "0");
  if (digits.length > len - 1) {
    throw new Error(`tar sayısal alanı taştı: ${value}`);
  }
  const buf = Buffer.alloc(len, 0);
  buf.write(digits, 0, "ascii");
  buf[len - 1] = 0;
  return buf;
}

function formatChecksum(sum: number): Buffer {
  const buf = Buffer.alloc(8, 0);
  buf.write(sum.toString(8).padStart(6, "0"), 0, "ascii");
  buf[6] = 0;
  buf[7] = 0x20;
  return buf;
}

function buildHeader(name: string, size: number, mtimeSec: number): Buffer {
  const nameBytes = Buffer.from(name, "utf8");
  if (nameBytes.length > 100) {
    throw new Error(`tar girdi adı çok uzun (>100 bayt): ${name}`);
  }
  const header = Buffer.alloc(BLOCK, 0);
  nameBytes.copy(header, 0);
  padOctal(0o600, 8).copy(header, 100); // mode
  padOctal(0, 8).copy(header, 108); // uid
  padOctal(0, 8).copy(header, 116); // gid
  padOctal(size, 12).copy(header, 124); // size
  padOctal(mtimeSec, 12).copy(header, 136); // mtime
  header.fill(0x20, 148, 156); // chksum alanı, hesap için geçici olarak boşluk
  header[156] = 0x30; // typeflag '0' — düz dosya
  header.write("ustar\0", 257, "ascii"); // magic
  header.write("00", 263, "ascii"); // version
  let sum = 0;
  for (let i = 0; i < BLOCK; i++) {
    sum += header[i];
  }
  formatChecksum(sum).copy(header, 148);
  return header;
}

/** entries'i USTAR tar arşivi olarak paketleyip gzip'ler. */
export function buildTarGz(entries: TarEntry[]): Buffer {
  const parts: Buffer[] = [];
  const mtimeSec = Math.floor(Date.now() / 1000);
  for (const e of entries) {
    parts.push(buildHeader(e.name, e.data.length, mtimeSec));
    parts.push(e.data);
    const pad = (BLOCK - (e.data.length % BLOCK)) % BLOCK;
    if (pad > 0) {
      parts.push(Buffer.alloc(pad, 0));
    }
  }
  parts.push(Buffer.alloc(BLOCK * 2, 0)); // arşiv sonu işareti (iki sıfır blok)
  return zlib.gzipSync(Buffer.concat(parts));
}

function readOctalField(header: Buffer, start: number, len: number): number {
  const raw = header.subarray(start, start + len);
  if (raw.length > 0 && (raw[0] & 0x80) !== 0) {
    throw new Error("desteklenmeyen tar sayısal biçimi (base-256)");
  }
  let s = raw.toString("ascii");
  const nulIdx = s.indexOf("\0");
  if (nulIdx !== -1) {
    s = s.slice(0, nulIdx);
  }
  s = s.trim();
  return s === "" ? 0 : parseInt(s, 8);
}

function readName(header: Buffer): string {
  const nameRaw = header.subarray(0, 100).toString("utf8");
  const name = nameRaw.slice(0, Math.max(0, nameRaw.indexOf("\0") === -1 ? nameRaw.length : nameRaw.indexOf("\0")));
  const prefixRaw = header.subarray(345, 500).toString("utf8");
  const prefix = prefixRaw.slice(0, Math.max(0, prefixRaw.indexOf("\0") === -1 ? prefixRaw.length : prefixRaw.indexOf("\0")));
  return prefix ? `${prefix}/${name}` : name;
}

/** gzip'i açıp USTAR/GNU/v7 (ad/boyut/typeflag alanları ortak) girdilerini çözer. */
export function parseTarGz(buf: Buffer): TarEntry[] {
  const tar = zlib.gunzipSync(buf);
  const entries: TarEntry[] = [];
  let offset = 0;
  while (offset + BLOCK <= tar.length) {
    const header = tar.subarray(offset, offset + BLOCK);
    if (header.every((b) => b === 0)) {
      break; // arşiv sonu
    }
    const storedChecksum = readOctalField(header, 148, 8);
    let sum = 0;
    for (let i = 0; i < BLOCK; i++) {
      sum += i >= 148 && i < 156 ? 0x20 : header[i];
    }
    if (sum !== storedChecksum) {
      throw new Error("bozuk tar başlığı (checksum uyuşmuyor)");
    }
    const name = readName(header);
    const size = readOctalField(header, 124, 12);
    const typeflag = header[156];
    offset += BLOCK;
    const data = tar.subarray(offset, offset + size);
    if (data.length !== size) {
      throw new Error("beklenmeyen tar sonu");
    }
    offset += Math.ceil(size / BLOCK) * BLOCK;
    if (typeflag === 0x30 || typeflag === 0) {
      entries.push({ name, data: Buffer.from(data) });
    }
  }
  return entries;
}
