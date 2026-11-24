//go:build !windows

package main

// konsoluGizle, yalnızca Windows'ta bir şey yapar (çift tıklamayla açılan
// konsol penceresi); öteki sistemlerde masaüstü kipi terminali olduğu gibi
// bırakır.
func konsoluGizle() bool { return false }

// konsoldaYenidenBaslat yalnızca Windows'ta gereklidir: orada and.exe
// Explorer'dan konsolsuz açılır. Öteki sistemlerde terminal arayüzü AND'ı
// açan terminalde çalışır.
func konsoldaYenidenBaslat() (int, bool) { return 0, false }

func hataKutusu(string) {}
