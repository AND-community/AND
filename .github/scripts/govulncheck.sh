#!/usr/bin/env bash
# Bulunduğu dizindeki Go modülünü govulncheck ile tarar. Kodun eriştiği her
# açık .github/govulncheck-izinli.txt'de gerekçesiyle listelenmiş olmalıdır;
# listede olmayan yeni bir açık CI'ı durdurur. Tarama hiç çalışamazsa (ağ
# hatası gibi) da durdurur: sessizce "temiz" sayılmaz.
set -euo pipefail

kok="$(git rev-parse --show-toplevel)"
izinli_dosya="${kok}/.github/govulncheck-izinli.txt"
surum="${GOVULNCHECK_SURUM:-v1.7.0}"
cikti="$(mktemp)"
trap 'rm -f "$cikti"' EXIT

set +e
go run "golang.org/x/vuln/cmd/govulncheck@${surum}" ./... >"$cikti" 2>&1
set -e
cat "$cikti"

if ! grep -qE '^(Your code is affected|No vulnerabilities found)' "$cikti"; then
  echo "::error::govulncheck taramayı tamamlayamadı"
  exit 1
fi

bulunan="$(grep -oE '^Vulnerability #[0-9]+: GO-[0-9]+-[0-9]+' "$cikti" | grep -oE 'GO-[0-9]+-[0-9]+' | sort -u || true)"
izinli="$(grep -oE '^GO-[0-9]+-[0-9]+' "$izinli_dosya" | sort -u || true)"
yeni="$(comm -23 <(printf '%s\n' "$bulunan") <(printf '%s\n' "$izinli") | grep . || true)"

if [ -n "$yeni" ]; then
  for id in $yeni; do
    echo "::error::Yeni bilinen açık: ${id} (https://pkg.go.dev/vuln/${id}). Bağımlılığı güncelle ya da gerekçesiyle ${izinli_dosya#"$kok"/} dosyasına ekle."
  done
  exit 1
fi
echo "Kodun eriştiği açıkların hepsi izin listesinde."
