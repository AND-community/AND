package main

// and.exe'nin Windows'taki simgesi AND'ın logosudur (küre). Simge
// rsrc_windows_amd64.syso ve rsrc_windows_arm64.syso dosyalarındadır; go build
// bu dosyaları yalnızca o mimarilerdeki Windows derlemelerine kendiliğinden
// katar, başka bir adım gerekmez. winres/icon.ico (16–256 piksel) web sitesi
// deposundaki tools/simge-uret.js ile logodan üretildi; simge ya da manifest
// değişirse "go generate ./cmd/and ./cmd/andmod ./cmd/and-masaustu" nesneleri
// yeniler.
//
// Aynı nesnelerde and.exe'nin manifest'i (winres/and.manifest) de vardır. Tek
// ayarı consoleAllocationPolicy "detached"tır: and.exe Explorer'dan (çift
// tıklama, kısayol, Başlat menüsü) açılınca Windows ona konsol penceresi
// açmaz, AND doğrudan kendi penceresinde açılır. Bir terminalden (cmd,
// PowerShell, Windows Terminal) açılınca o terminalin konsolunu yine
// devralır: komut satırı ve terminal arayüzü eskisi gibi çalışır. Windows 11
// 24H2'den (26100) eski sürümler bu ayarı yok sayar; orada AND konsol
// penceresini açılır açılmaz gizler (bkz. konsol_windows.go).
//
//go:generate go run ./winres/uret -ico winres/icon.ico -manifest winres/and.manifest
