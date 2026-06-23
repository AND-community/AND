package pluginmgr

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lucian95511/and/internal/pluginapi"
)

type Plugin struct {
	Manifest pluginapi.Manifest
	ExePath  string
	Enabled  bool
}

func (p *Plugin) Name() string  { return p.Manifest.Name }
func (p *Plugin) Label() string { return p.Manifest.Label }

func (p *Plugin) Launch(apiAddr, dataDir string, extraEnv ...string) *exec.Cmd {
	cmd := exec.Command(p.ExePath)
	cmd.Env = append(os.Environ(),
		"AND_API_ADDR="+apiAddr,
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
	return discoverDir(filepath.Dir(exePath))
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

func discoverDir(dir string) []Plugin {
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

func getManifest(exePath string) (pluginapi.Manifest, error) {
	jsonPath := strings.TrimSuffix(exePath, ".exe") + ".json"
	if data, err := os.ReadFile(jsonPath); err == nil {
		var m pluginapi.Manifest
		if err := json.Unmarshal(data, &m); err == nil && m.Name != "" {
			return m, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exePath, "--manifest").Output()
	if err != nil {
		return pluginapi.Manifest{}, err
	}
	var m pluginapi.Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		return pluginapi.Manifest{}, err
	}
	if m.Name == "" {
		return pluginapi.Manifest{}, nil
	}
	return m, nil
}
