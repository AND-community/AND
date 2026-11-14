#!/usr/bin/env bash
# derle.sh — and-ag'ı derler: ./derle.sh [cargo argümanları], ör. ./derle.sh build --release
#
# Windows'ta Rust'ın GNU araç zinciriyle, C derleyicisi ve Visual Studio C++
# araçları olmadan derlenir; gerekenler rustup bileşenleridir:
#   rustup component add llvm-tools
# Linux ve macOS'ta doğrudan "cargo build" yeterlidir.
set -euo pipefail
cd "$(dirname "$0")"
if [[ "$(rustc -vV | sed -n 's/^host: //p')" == *windows-gnu ]]; then
    sysroot="$(rustc --print sysroot)"
    host="$(rustc -vV | sed -n 's/^host: //p')"
    llvm_ar="$sysroot/lib/rustlib/$host/bin/llvm-ar.exe"
    if [[ ! -f "$llvm_ar" ]]; then
        echo "derle.sh: llvm-tools eksik; kur: rustup component add llvm-tools" >&2
        exit 1
    fi
    # llvm-ar, dlltool adıyla çağrılınca llvm-dlltool gibi çalışır.
    araclar="$PWD/target/araclar"
    mkdir -p "$araclar"
    if [[ ! -f "$araclar/dlltool.exe" || "$llvm_ar" -nt "$araclar/dlltool.exe" ]]; then
        cp "$llvm_ar" "$araclar/dlltool.exe"
    fi
    export PATH="$araclar:$PATH"
fi
exec cargo "$@"
