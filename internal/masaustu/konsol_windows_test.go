//go:build windows

package masaustu

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestOrtamBlogu(t *testing.T) {
	blok, err := ortamBlogu([]string{"B=2", "a=1", "=C:=C:\\yol", "", "A=son", "BOS"})
	if err != nil {
		t.Fatal(err)
	}
	if len(blok) < 2 || blok[len(blok)-1] != 0 || blok[len(blok)-2] != 0 {
		t.Fatal("blok çift NUL ile bitmeli")
	}
	var girdiler []string
	for _, p := range strings.Split(string(utf16.Decode(blok[:len(blok)-2])), "\x00") {
		girdiler = append(girdiler, p)
	}
	// Adlar büyük/küçük harf duyarsız sıralı; aynı addan sonuncusu geçerli;
	// '=' ile başlayan gizli değişkenler korunur, '='siz girdi atlanır.
	beklenen := []string{"=C:=C:\\yol", "A=son", "B=2"}
	if !slices.Equal(girdiler, beklenen) {
		t.Fatalf("girdiler = %q, beklenen %q", girdiler, beklenen)
	}
	if _, err := ortamBlogu([]string{"A=x\x00y"}); err == nil {
		t.Error("NUL içeren değişken reddedilmeli")
	}
	if b, _ := ortamBlogu(nil); len(b) != 2 {
		t.Errorf("boş ortam bloğu = %v", b)
	}
}
