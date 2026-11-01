package main

// and.exe'nin Windows'taki simgesi AND'ın logosudur (küre). Simge
// rsrc_windows_amd64.syso ve rsrc_windows_arm64.syso dosyalarındadır; go build
// bu dosyaları yalnızca o mimarilerdeki Windows derlemelerine kendiliğinden
// katar, başka bir adım gerekmez. winres/icon.ico (16–256 piksel) web sitesi
// deposundaki tools/simge-uret.js ile logodan üretildi; simge değişirse
// "go generate ./cmd/and ./cmd/andmod" nesneleri yeniler.
//
//go:generate go run ./winres/uret -ico winres/icon.ico
