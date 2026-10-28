package pluginmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lucian95511/and/internal/pluginapi"
)

// ─── Plugin.Name / Label / Launch ─────────────────────────────────────────────

func TestPlugin_NameLabel(t *testing.T) {
	p := Plugin{
		Manifest: pluginapi.Manifest{
			Name:    "admin",
			Label:   "Yönetici Paneli",
			Version: "2.0.0",
			Author:  "AND",
		},
		ExePath: "/usr/local/bin/and-plugin-admin",
	}
	if p.Name() != "admin" {
		t.Errorf("Name: %q", p.Name())
	}
	if p.Label() != "Yönetici Paneli" {
		t.Errorf("Label: %q", p.Label())
	}
}

func TestPlugin_Launch_EnvVars(t *testing.T) {
	p := Plugin{
		Manifest: pluginapi.Manifest{Name: "test"},
		ExePath:  "/bin/echo",
	}
	cmd := p.Launch("127.0.0.1:12345", "test-token-abc", "/tmp/data", "AND_CATEGORY=genel")

	envMap := make(map[string]string)
	for _, e := range cmd.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if got := envMap["AND_API_ADDR"]; got != "127.0.0.1:12345" {
		t.Errorf("AND_API_ADDR: %q", got)
	}
	if got := envMap["AND_API_TOKEN"]; got != "test-token-abc" {
		t.Errorf("AND_API_TOKEN: %q", got)
	}
	if got := envMap["AND_DATA_DIR"]; got != "/tmp/data" {
		t.Errorf("AND_DATA_DIR: %q", got)
	}
	if got := envMap["AND_CATEGORY"]; got != "genel" {
		t.Errorf("AND_CATEGORY: %q", got)
	}
}

// ─── getManifest ──────────────────────────────────────────────────────────────

// writeSidecarPlugin ana-dizin (discoverAndDir) tarzı bir eklenti için bir
// and-plugin-<isim>[.exe] yer tutucu dosyası ve yanına bir and-plugin-<isim>.json
// sidecar manifest dosyası yazar. getManifest artık HİÇBİR ZAMAN binary'yi
// çalıştırmadığından (bkz. manager.go'daki güvenlik notu), yer tutucu dosya
// gerçek bir binary olmak zorunda değildir — bu da testleri gerçek bir `go
// build` alt sürecine ihtiyaç duymadan hızlandırır.
func writeSidecarPlugin(t *testing.T, dir string, manifest pluginapi.Manifest) string {
	t.Helper()

	binName := "and-plugin-" + manifest.Name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(dir, binName)
	if err := os.WriteFile(binPath, []byte("yer-tutucu"), 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	jsonPath := strings.TrimSuffix(binPath, ".exe") + ".json"
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return binPath
}

func TestGetManifest_Valid(t *testing.T) {
	want := pluginapi.Manifest{
		Name:    "testfake",
		Label:   "Test Eklentisi",
		Version: "1.0.0",
		Author:  "Test",
	}
	bin := writeSidecarPlugin(t, t.TempDir(), want)

	got, err := getManifest(bin)
	if err != nil {
		t.Fatalf("getManifest: %v", err)
	}
	if got.Name != want.Name {
		t.Errorf("Name: got %q, want %q", got.Name, want.Name)
	}
	if got.Label != want.Label {
		t.Errorf("Label: got %q, want %q", got.Label, want.Label)
	}
	if got.Version != want.Version {
		t.Errorf("Version: got %q, want %q", got.Version, want.Version)
	}
}

func TestGetManifest_NotFound(t *testing.T) {
	_, err := getManifest(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

// Güvenlik regresyon testi: getManifest ÖNCEDEN, sidecar JSON yoksa
// binary'yi "--manifest" argümanıyla ÇALIŞTIRIRDI — bu, and-plugin-*
// adındaki HERHANGİ BİR dosyanın (kullanıcı onayı olmadan) AND'ın
// yetkileriyle çalıştırılmasına yol açıyordu. Artık sidecar yoksa hata
// dönmeli ve binary asla exec edilmemeli.
func TestGetManifest_NoSidecar_NeverExecutesBinary(t *testing.T) {
	dir := t.TempDir()
	binName := "and-plugin-tehlikeli"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(dir, binName)

	marker := filepath.Join(dir, "calisti.marker")
	src := fmt.Sprintf(`package main
import "os"
func main() {
	os.WriteFile(%q, []byte("calisti"), 0o644)
}
`, marker)
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", binPath, srcFile).CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	if _, err := getManifest(binPath); err == nil {
		t.Fatal("expected error when no sidecar manifest exists")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("getManifest must never execute the plugin binary")
	}
}

// ─── Discover ─────────────────────────────────────────────────────────────────

func TestDiscover_FindsPlugins(t *testing.T) {
	want := pluginapi.Manifest{
		Name:    "myplugin",
		Label:   "Benim Eklentim",
		Version: "3.0.0",
	}
	dir := t.TempDir()
	writeSidecarPlugin(t, dir, want)

	// We test Discover indirectly by calling discoverAndDir.
	plugins := discoverAndDir(dir)
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name() != "myplugin" {
		t.Errorf("Name: %q", plugins[0].Name())
	}
	if plugins[0].Label() != "Benim Eklentim" {
		t.Errorf("Label: %q", plugins[0].Label())
	}
}

func TestDiscover_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins := discoverAndDir(dir)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscover_SkipsNonPluginFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a file that doesn't match the "and-plugin-" prefix.
	_ = os.WriteFile(filepath.Join(dir, "and-core"), []byte{}, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "README.txt"), []byte{}, 0o644)

	plugins := discoverAndDir(dir)
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

// ─── Sidecar JSON manifest ────────────────────────────────────────────────────

// TestGetManifest_SidecarJSON checks that a plugin.json sidecar is preferred
// over running the binary with --manifest (the XenForo-like static manifest path).
func TestGetManifest_SidecarJSON(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "and-plugin-sidecar")

	// Write a sidecar JSON file (no real binary needed).
	want := pluginapi.Manifest{Name: "sidecar", Label: "Sidecar Eklenti", Version: "9.0.0", Author: "Test"}
	data, _ := json.Marshal(want)
	jsonPath := filepath.Join(dir, "and-plugin-sidecar.json")
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	// Also write a dummy (non-executable) file as the binary so getManifest doesn't
	// attempt to execute it (it should return after reading the JSON).
	if err := os.WriteFile(binPath, []byte("not-a-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := getManifest(binPath)
	if err != nil {
		t.Fatalf("getManifest sidecar: %v", err)
	}
	if got.Name != want.Name || got.Label != want.Label || got.Version != want.Version {
		t.Errorf("sidecar manifest mismatch: got %+v, want %+v", got, want)
	}
}

// Güvenlik regresyon testi: bir manifest'in Label/Description alanındaki ESC/
// ANSI kaçış dizileri, dosya varlığıyla bile (eklenti hiç çalıştırılmadan)
// AND'ın kendi menüsünü manipüle edebilirdi (bkz. sanitizeManifest'in doc
// yorumu) — artık ayıklanmalı.
func TestGetManifest_SanitizesControlCharacters(t *testing.T) {
	dir := t.TempDir()
	want := pluginapi.Manifest{
		Name:        "kirli",
		Label:       "Zararsız\x1b[2J\x1b]0;Sahte Baslik\x07 Görünen",
		Description: "Aciklama\x00ile\x1bkontrol\x7fkarakterleri",
		Version:     "1.0.0",
		Author:      "Test",
	}
	binPath := writeSidecarPlugin(t, dir, want)

	got, err := getManifest(binPath)
	if err != nil {
		t.Fatalf("getManifest: %v", err)
	}
	if strings.ContainsAny(got.Label, "\x1b\x00\x07") {
		t.Errorf("Label kontrol karakteri içeriyor: %q", got.Label)
	}
	if strings.ContainsAny(got.Description, "\x1b\x00\x7f") {
		t.Errorf("Description kontrol karakteri içeriyor: %q", got.Description)
	}
}

// ─── Toggle ───────────────────────────────────────────────────────────────────

func TestToggle_FlipsEnabled(t *testing.T) {
	plugins := []Plugin{
		{Manifest: pluginapi.Manifest{Name: "a"}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "b"}, Enabled: false},
	}
	Toggle(plugins, "a")
	if plugins[0].Enabled {
		t.Error("expected a to be disabled after toggle")
	}
	Toggle(plugins, "b")
	if !plugins[1].Enabled {
		t.Error("expected b to be enabled after toggle")
	}
}

func TestToggle_UnknownName_NoOp(t *testing.T) {
	plugins := []Plugin{
		{Manifest: pluginapi.Manifest{Name: "a"}, Enabled: true},
	}
	Toggle(plugins, "nonexistent")
	if !plugins[0].Enabled {
		t.Error("toggle of unknown name should be a no-op")
	}
}

// ─── SaveState / loadState ────────────────────────────────────────────────────

func TestSaveState_LoadState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	plugins := []Plugin{
		{Manifest: pluginapi.Manifest{Name: "admin"}, Enabled: true},
		{Manifest: pluginapi.Manifest{Name: "mod"}, Enabled: false},
		{Manifest: pluginapi.Manifest{Name: "chat"}, Enabled: false},
	}
	if err := SaveState(plugins, dir); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	st := loadState(dir)
	disabled := make(map[string]bool)
	for _, n := range st.Disabled {
		disabled[n] = true
	}
	if disabled["admin"] {
		t.Error("admin should not be disabled")
	}
	if !disabled["mod"] {
		t.Error("mod should be disabled")
	}
	if !disabled["chat"] {
		t.Error("chat should be disabled")
	}
}

func TestSaveState_EmptyDir_NoError(t *testing.T) {
	if err := SaveState(nil, ""); err != nil {
		t.Errorf("SaveState with empty dataDir: %v", err)
	}
}

func TestLoadState_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	st := loadState(dir)
	if len(st.Disabled) != 0 {
		t.Errorf("expected empty state, got: %v", st.Disabled)
	}
}

// ─── DiscoverWithState ────────────────────────────────────────────────────────

func TestDiscoverWithState_AppliesDisabledList(t *testing.T) {
	dir := t.TempDir()

	want := pluginapi.Manifest{Name: "mywired", Label: "Wired", Version: "1.0.0"}
	writeSidecarPlugin(t, dir, want)

	// Save a state file that disables "mywired".
	plugins := []Plugin{{Manifest: want, Enabled: false}}
	_ = SaveState(plugins, dir)

	// Discover + apply state.
	discovered := discoverAndDir(dir)
	applyState(discovered, loadState(dir))

	if len(discovered) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(discovered))
	}
	if discovered[0].Enabled {
		t.Error("mywired should be disabled after applyState")
	}
}

// TestLoadState_OkunamayanDosyaKenaraAlinir: çözülemeyen plugins_state.json
// bir sonraki kayıtta ezilmeden önce kenara alınır.
func TestLoadState_OkunamayanDosyaKenaraAlinir(t *testing.T) {
	dir := t.TempDir()
	yol := filepath.Join(dir, stateFile)
	if err := os.WriteFile(yol, []byte(`{"disabled": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	if st := loadState(dir); len(st.Disabled) != 0 {
		t.Fatalf("bozuk dosyadan durum okunmamalı: %+v", st)
	}
	kenar, _ := filepath.Glob(yol + ".bozuk-*")
	if len(kenar) != 1 {
		t.Fatalf("bozuk dosya kenara alınmalıydı, bulunan: %v", kenar)
	}
	if veri, _ := os.ReadFile(kenar[0]); string(veri) != `{"disabled": [` {
		t.Errorf("kenardaki içerik %q", veri)
	}
}
