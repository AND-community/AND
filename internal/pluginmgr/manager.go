package pluginmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lucian95511/and/internal/pluginapi"
)

type Plugin struct {
	Manifest pluginapi.Manifest
	ExePath  string
	Enabled  bool
}

func (p *Plugin) Name() string  { return p.Manifest.Name }
func (p *Plugin) Label() string { return p.Manifest.Label }

func (p *Plugin) Launch(apiAddr, apiToken, dataDir string, extraEnv ...string) *exec.Cmd {
	cmd := exec.Command(p.ExePath)
	cmd.Dir = filepath.Dir(p.ExePath) // her eklenti kendi dizininde çalışır
	cmd.Env = append(os.Environ(),
		"AND_API_ADDR="+apiAddr,
		"AND_API_TOKEN="+apiToken,
		"AND_DATA_DIR="+dataDir,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	return cmd
}

type pluginsState struct {
	Disabled []string `json:"disabled"`
}

const stateFile = "plugins_state.json"

func Discover() []Plugin {
	exePath, err := os.Executable()
	if err != nil {
		return nil
	}
	andDir := filepath.Dir(exePath)

	// Önce Eklentiler/ alt dizinlerini tara (her eklenti kendi klasöründe)
	eklentilerDir := filepath.Join(andDir, "Eklentiler")
	plugins := discoverEklentiler(eklentilerDir)

	// Geriye dönük uyumluluk: AND dizininde de ara; zaten bulunanları atla
	for _, p := range discoverAndDir(andDir) {
		found := false
		for _, existing := range plugins {
			if existing.Name() == p.Name() {
				found = true
				break
			}
		}
		if !found {
			plugins = append(plugins, p)
		}
	}

	return plugins
}

func DiscoverWithState(dataDir string) []Plugin {
	plugins := Discover()
	if len(plugins) == 0 {
		return plugins
	}
	applyState(plugins, loadState(dataDir))
	return plugins
}

func Toggle(plugins []Plugin, name string) {
	for i := range plugins {
		if plugins[i].Name() == name {
			plugins[i].Enabled = !plugins[i].Enabled
			return
		}
	}
}

func SaveState(plugins []Plugin, dataDir string) error {
	if dataDir == "" {
		return nil
	}
	var disabled []string
	for _, p := range plugins {
		if !p.Enabled {
			disabled = append(disabled, p.Name())
		}
	}
	st := pluginsState{Disabled: disabled}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, stateFile), data, 0o600)
}

// discoverEklentiler her alt dizinde plugin.json + and-plugin-<isim>[.exe] arar.
func discoverEklentiler(eklentilerDir string) []Plugin {
	entries, err := os.ReadDir(eklentilerDir)
	if err != nil {
		return nil
	}

	var plugins []Plugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(eklentilerDir, e.Name())

		// plugin.json manifest dosyasını oku
		data, err := os.ReadFile(filepath.Join(subDir, "plugin.json"))
		if err != nil {
			continue
		}
		var m pluginapi.Manifest
		if err := json.Unmarshal(data, &m); err != nil || m.Name == "" {
			continue
		}
		sanitizeManifest(&m)

		// Binary'yi bul: and-plugin-<isim> ya da and-plugin-<isim>.exe
		base := "and-plugin-" + m.Name
		exePath := filepath.Join(subDir, base)
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			exePath += ".exe"
			if _, err := os.Stat(exePath); os.IsNotExist(err) {
				continue // binary yok; bu eklenti çalıştırılamaz
			}
		}

		plugins = append(plugins, Plugin{Manifest: m, ExePath: exePath, Enabled: true})
	}
	return plugins
}

// discoverAndDir AND binary'si ile aynı dizindeki eski tarz and-plugin-* dosyalarını arar.
func discoverAndDir(dir string) []Plugin {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var plugins []Plugin
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		baseName := strings.TrimSuffix(name, ".exe")
		if !strings.HasPrefix(baseName, "and-plugin-") {
			continue
		}
		exeFile := filepath.Join(dir, name)
		m, err := getManifest(exeFile)
		if err != nil {
			continue
		}
		plugins = append(plugins, Plugin{Manifest: m, ExePath: exeFile, Enabled: true})
	}
	return plugins
}

func loadState(dataDir string) pluginsState {
	if dataDir == "" {
		return pluginsState{}
	}
	data, err := os.ReadFile(filepath.Join(dataDir, stateFile))
	if err != nil {
		return pluginsState{}
	}
	var st pluginsState
	json.Unmarshal(data, &st) //nolint:errcheck
	return st
}

func applyState(plugins []Plugin, st pluginsState) {
	disabled := make(map[string]bool, len(st.Disabled))
	for _, name := range st.Disabled {
		disabled[name] = true
	}
	for i := range plugins {
		plugins[i].Enabled = !disabled[plugins[i].Name()]
	}
}

// getManifest, exePath'in yanındaki <isim>.json sidecar manifest dosyasını
// okur.
//
// ÖNCEDEN, sidecar bulunamazsa exePath "--manifest" argümanıyla
// ÇALIŞTIRILARAK bir manifest istenirdi — bu, AND'ın kendi binary dizinine
// (ör. bir arşivden çıkarılmış kötü amaçlı bir dosya, ya da o dizine
// yazabilen başka bir yerel süreç tarafından bırakılmış bir dosya)
// "and-plugin-" önekiyle YERLEŞTİRİLEN herhangi bir dosyanın, kullanıcı onayı
// ya da imza/checksum kontrolü olmadan, AND'ın kendi yetkileriyle
// ÇALIŞTIRILMASINA yol açıyordu — yalnızca AND'ı başlatmak ya da eklenti
// listesini yenilemek yeterliydi. README'nin belgelediği iş akışı zaten
// sidecar JSON'ı yalnızca BİR KEZ, geliştirme sırasında üretmeyi öngörüyor
// (`./and-plugin-x --manifest > and-plugin-x.json`) — bu yüzden çalışma
// zamanında tekrar çalıştırmaya hiç gerek yok. Artık sidecar yoksa bu
// "eklenti" sessizce atlanır (discoverAndDir), asla çalıştırılmaz.
func getManifest(exePath string) (pluginapi.Manifest, error) {
	jsonPath := strings.TrimSuffix(exePath, ".exe") + ".json"
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return pluginapi.Manifest{}, fmt.Errorf("pluginmgr: sidecar manifest yok (%s): %w", jsonPath, err)
	}
	var m pluginapi.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return pluginapi.Manifest{}, err
	}
	if m.Name == "" {
		return pluginapi.Manifest{}, fmt.Errorf("pluginmgr: manifest name alanı boş")
	}
	sanitizeManifest(&m)
	return m, nil
}

// sanitizeManifest, bir manifest'in TUI'de doğrudan (hiç eklenti
// çalıştırılmadan, yalnızca dosya varlığıyla) görüntülenen alanlarından
// yazdırılamayan kontrol karakterlerini (ESC/ANSI/OSC kaçış dizileri dahil)
// temizler. Bunlar temizlenmezse bir plugin.json — imleç hareketi, ekran
// temizleme, terminal başlığı/pano manipülasyonu (OSC) içeren ya da başka bir
// eklentinin adını taklit eden bir Label ile — AND'ın kendi menüsünü
// yönlendirebilirdi.
func sanitizeManifest(m *pluginapi.Manifest) {
	m.Name = sanitizeManifestText(m.Name)
	m.Label = sanitizeManifestText(m.Label)
	m.Version = sanitizeManifestText(m.Version)
	m.Description = sanitizeManifestText(m.Description)
	m.Author = sanitizeManifestText(m.Author)
	m.Category = sanitizeManifestText(m.Category)
}

func sanitizeManifestText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x1b: // ESC — ANSI/OSC kaçış dizilerinin başlangıcı
			continue
		case r < 0x20: // C0 kontrol karakterleri (yeni satır/tab dahil — tek satırlık alanlar)
			continue
		case r >= 0x7f && r <= 0x9f: // DEL + C1 kontrol karakterleri
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
