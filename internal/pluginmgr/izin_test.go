package pluginmgr

import (
	"strings"
	"testing"

	"github.com/lucian95511/and/internal/pluginapi"
)

func TestIzinOnayi_KaydedilirVeYeniIzindeYenidenSorulur(t *testing.T) {
	dir := t.TempDir()
	istenen := []string{pluginapi.IzinDM, pluginapi.IzinDosya}
	if IzinOnayli(dir, "ozel_chat", istenen) {
		t.Fatal("hiç onay yokken onaylı sayılmamalı")
	}
	if !IzinOnayli(dir, "ozel_chat", nil) {
		t.Fatal("izin istemeyen eklenti için onay gerekmez")
	}
	if err := IzinOnayla(dir, "ozel_chat", istenen); err != nil {
		t.Fatal(err)
	}
	if !IzinOnayli(dir, "ozel_chat", istenen) || !IzinOnayli(dir, "ozel_chat", istenen[:1]) {
		t.Fatal("onaylanan izinler (ve alt kümesi) onaylı sayılmalı")
	}
	if IzinOnayli(dir, "ozel_chat", append(istenen, pluginapi.IzinTema)) {
		t.Fatal("güncellemede eklenen yeni izin yeniden sorulmalı")
	}
	if IzinOnayli(dir, "baska", istenen) {
		t.Fatal("onay eklenti başınadır")
	}
}

func TestSaveState_IzinOnaylariniKorur(t *testing.T) {
	dir := t.TempDir()
	if err := IzinOnayla(dir, "site", []string{pluginapi.IzinSite}); err != nil {
		t.Fatal(err)
	}
	plugins := []Plugin{{Manifest: pluginapi.Manifest{Name: "site"}, Enabled: false}}
	if err := SaveState(plugins, dir); err != nil {
		t.Fatal(err)
	}
	if !IzinOnayli(dir, "site", []string{pluginapi.IzinSite}) {
		t.Fatal("eklentiyi kapatıp açmak izin onayını silmemeli")
	}
	if st := loadState(dir); len(st.Disabled) != 1 || st.Disabled[0] != "site" {
		t.Fatalf("kapalı eklenti listesi de yazılmalı: %+v", st)
	}
}

func TestIzinleriGeriAl(t *testing.T) {
	dir := t.TempDir()
	_ = IzinOnayla(dir, "admin", []string{pluginapi.IzinModerasyon})
	if err := IzinleriGeriAl(dir, "admin"); err != nil {
		t.Fatal(err)
	}
	if IzinOnayli(dir, "admin", []string{pluginapi.IzinModerasyon}) || len(OnayliIzinler(dir, "admin")) != 0 {
		t.Fatal("geri alınan onay silinmeli")
	}
	if err := IzinleriGeriAl(dir, "olmayan"); err != nil {
		t.Fatalf("onayı olmayan eklenti için geri alma hata vermemeli: %v", err)
	}
}

func TestUyumlu(t *testing.T) {
	p := Plugin{Manifest: pluginapi.Manifest{Name: "x", Label: "X", MinAPI: pluginapi.APIVersion}}
	if err := Uyumlu(p); err != nil {
		t.Fatalf("aynı sürüm uyumlu olmalı: %v", err)
	}
	p.Manifest.MinAPI = pluginapi.APIVersion + 1
	if err := Uyumlu(p); err == nil || !strings.Contains(err.Error(), "güncelle") {
		t.Fatalf("daha yeni API isteyen eklenti reddedilmeli: %v", err)
	}
}

func TestSanitizeManifest_BilinmeyenIzinleriAtar(t *testing.T) {
	m := pluginapi.Manifest{Name: "x", Permissions: []string{"dm", "kök", "\x1b[31mdm", "theme"}}
	sanitizeManifest(&m)
	if strings.Join(m.Permissions, ",") != "dm,theme" {
		t.Fatalf("yalnızca tanınan izinler kalmalı: %v", m.Permissions)
	}
}
