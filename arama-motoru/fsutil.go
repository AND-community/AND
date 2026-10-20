package main

// fsutil.go — dosya sistemi yardımcıları. atomicYaz, identity.go (kimlik.json
// — kaybı geri döndürülemez, ağ kimliğinin kendisidir) ve peers.go
// (bilinen_peerler.json) tarafından paylaşılır.

import (
	"os"
	"path/filepath"
)

// atomicYaz, data'yı yol'a ATOMİK olarak yazar: önce aynı dizinde bir geçici
// dosyaya yazıp senkronize eder, ardından yeniden adlandırır. Düz bir
// os.WriteFile, işlem tam yazma sırasında kesintiye uğrarsa (kilitlenme,
// güç kesintisi, ctrl+c) hedef dosyayı YARIM/BOZUK bırakabilir; rename
// (hem POSIX'te hem Windows'ta) ise ya TAMAMEN eskisiyle ya da TAMAMEN
// yenisiyle sonuçlanır, ara bir durum asla gözlemlenemez.
func atomicYaz(yol string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(yol)
	tmp, err := os.CreateTemp(dir, filepath.Base(yol)+".tmp-*")
	if err != nil {
		return err
	}
	tmpYol := tmp.Name()
	// Hata durumunda geçici dosyayı temizle — başarı yolunda Rename zaten
	// onu hedef ada taşıdığından bu Remove no-op (dosya artık orada değil).
	defer os.Remove(tmpYol) //nolint:errcheck

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close() //nolint:errcheck
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpYol, perm); err != nil {
		return err
	}
	return os.Rename(tmpYol, yol)
}
