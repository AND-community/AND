//go:build !windows

package main

// konsoluGizle, yalnızca Windows'ta bir şey yapar (çift tıklamayla açılan
// konsol penceresi); öteki sistemlerde masaüstü kipi terminali olduğu gibi
// bırakır.
func konsoluGizle() bool { return false }

func hataKutusu(string) {}
