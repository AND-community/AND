package main

import "testing"

func TestYokSayilsinMi(t *testing.T) {
	desenler := []string{"node_modules", "*.log", "build/", "src/gizli.txt", "# yorum", ""}

	vakalar := []struct {
		yol      string
		beklenen bool
	}{
		{"node_modules/paket/index.js", true},
		{"src/app.log", true},
		{"build/cikti.bin", true},
		{"src/gizli.txt", true},
		{"src/app.go", false},
		{"README.md", false},
	}
	for _, v := range vakalar {
		if got := yokSayilsinMi(desenler, v.yol); got != v.beklenen {
			t.Errorf("yokSayilsinMi(%q) = %v, beklenen %v", v.yol, got, v.beklenen)
		}
	}
}

func TestProjeAgIgnoreCalismaSnapshotindanCikartir(t *testing.T) {
	_, _, calismaDir := yeniTestProjesi(t)
	yazDosya(t, calismaDir, "a.go", "paket ana")
	yazDosya(t, calismaDir, "node_modules/paket/index.js", "// üretilmiş")
	yazDosya(t, calismaDir, projeAgIgnoreDosyasi, "node_modules\n")

	snap, err := calismaSnapshotHesapla(calismaDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap["node_modules/paket/index.js"]; ok {
		t.Fatal("node_modules altındaki dosya yok sayılmalıydı")
	}
	if _, ok := snap["a.go"]; !ok {
		t.Fatal("a.go izlenmeliydi")
	}
	if _, ok := snap[projeAgIgnoreDosyasi]; !ok {
		t.Fatal(".projeagignore dosyasının kendisi izlenmeliydi")
	}
}
