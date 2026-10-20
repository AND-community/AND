// AND'ın tema renkleri (bkz. ThemeReq, Eklentiler/pembis) lipgloss/xterm-256
// palet numaraları olarak taşınır (ör. "205"), bazen doğrudan hex kod olarak
// da gelebilir ("#ff6600" — bkz. internal/pluginapi/api.go'daki ThemeReq
// yorumu). Bu dosya ikisini de CSS'in anlayacağı bir hex koda çevirir.

const ANSI_16: readonly string[] = [
  "#000000", "#800000", "#008000", "#808000", "#000080", "#800080", "#008080", "#c0c0c0",
  "#808080", "#ff0000", "#00ff00", "#ffff00", "#0000ff", "#ff00ff", "#00ffff", "#ffffff",
];

const CUBE_STEPS = [0, 95, 135, 175, 215, 255];

function toHex2(n: number): string {
  return n.toString(16).padStart(2, "0");
}

/** xterm-256 palet indeksini (0-255) "#rrggbb" hex koduna çevirir. */
export function ansi256ToHex(code: number): string {
  if (code < 0 || code > 255 || !Number.isInteger(code)) {
    return "#888888";
  }
  if (code < 16) {
    return ANSI_16[code];
  }
  if (code < 232) {
    const n = code - 16;
    const r = CUBE_STEPS[Math.floor(n / 36)];
    const g = CUBE_STEPS[Math.floor((n % 36) / 6)];
    const b = CUBE_STEPS[n % 6];
    return `#${toHex2(r)}${toHex2(g)}${toHex2(b)}`;
  }
  const gray = 8 + (code - 232) * 10;
  return `#${toHex2(gray)}${toHex2(gray)}${toHex2(gray)}`;
}

function relativeLuminance(hex: string): number | undefined {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex.trim());
  if (!m) {
    return undefined;
  }
  const lin = (h: string): number => {
    const c = parseInt(h, 16) / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * lin(m[1]) + 0.7152 * lin(m[2]) + 0.0722 * lin(m[3]);
}

/**
 * hex zemin üzerinde daha okunur olan metin rengini ("#000000" ya da
 * "#ffffff") WCAG kontrast oranına göre seçer. AND'ın varsayılan vurgusu
 * (#5fd7d7) gibi açık bir zeminde beyaz yazı ~1.6:1 ile okunmaz kalırdı —
 * vurgu renginde düğmelerin yazısı bu yüzden temaya göre hesaplanır.
 */
export function readableTextOn(hex: string | undefined): string | undefined {
  const l = hex ? relativeLuminance(hex) : undefined;
  if (l === undefined) {
    return undefined;
  }
  const onBlack = (l + 0.05) / 0.05;
  const onWhite = 1.05 / (l + 0.05);
  return onBlack >= onWhite ? "#000000" : "#ffffff";
}

/** ThemeReq alanlarında gelen bir rengi ("63" ya da "#ff6600") CSS hex koduna çevirir. */
export function ansiOrHexToCss(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  const trimmed = value.trim();
  if (trimmed.startsWith("#")) {
    return trimmed;
  }
  const n = Number(trimmed);
  if (!Number.isFinite(n)) {
    return undefined;
  }
  return ansi256ToHex(n);
}
