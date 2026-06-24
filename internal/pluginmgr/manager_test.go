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
	cmd := p.Launch("127.0.0.1:12345", "/tmp/data", "AND_CATEGORY=genel")

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
	if got := envMap["AND_DATA_DIR"]; got != "/tmp/data" {
		t.Errorf("AND_DATA_DIR: %q", got)
	}
	if got := envMap["AND_CATEGORY"]; got != "genel" {
		t.Errorf("AND_CATEGORY: %q", got)
	}
}

// ─── getManifest ──────────────────────────────────────────────────────────────

// buildFakePlugin compiles a tiny Go program that prints a manifest on
// --manifest and exits. Returns the path to the compiled binary.
func buildFakePlugin(t *testing.T, manifest pluginapi.Manifest) string {
	t.Helper()

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	src := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		fmt.Println(%q)
		return
	}
	os.Exit(1)
}
`, string(data))

	dir := t.TempDir()
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	binName := "and-plugin-testfake"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", binPath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake plugin: %v\n%s", err, out)
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
	bin := buildFakePlugin(t, want)

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

// ─── Discover ─────────────────────────────────────────────────────────────────

func TestDiscover_FindsPlugins(t *testing.T) {
	want := pluginapi.Manifest{
		Name:    "myplugin",
		Label:   "Benim Eklentim",
		Version: "3.0.0",
	}
	// Build the fake plugin into a temp dir named "and-plugin-myplugin[.exe]".
	dir := t.TempDir()
	binName := "and-plugin-myplugin"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	data, _ := json.Marshal(want)
	src := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		fmt.Println(%q)
		return
	}
	os.Exit(1)
}
`, string(data))

	srcFile := filepath.Join(dir, "main.go")
	_ = os.WriteFile(srcFile, []byte(src), 0o644)
	binPath := filepath.Join(dir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Run a child process that scans the temp dir (not the test's exe dir).
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

	// Build a real plugin binary.
	want := pluginapi.Manifest{Name: "mywired", Label: "Wired", Version: "1.0.0"}
	data, _ := json.Marshal(want)
	src := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		fmt.Println(%q)
		return
	}
	os.Exit(1)
}
`, string(data))

	binName := "and-plugin-mywired"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	srcFile := filepath.Join(dir, "main.go")
	_ = os.WriteFile(srcFile, []byte(src), 0o644)
	binPath := filepath.Join(dir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

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
