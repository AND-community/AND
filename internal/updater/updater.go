package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	Version    = "v0.1.0"
	GitHubRepo = "AND-community/AND"
)

type ReleaseInfo struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Check(ctx context.Context) (*ReleaseInfo, error) {
	if GitHubRepo == "" {
		return nil, nil
	}
	url := "https://api.github.com/repos/" + GitHubRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AND/"+Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github api: %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return nil, err
	}
	if info.TagName == "" || !isNewer(info.TagName, Version) {
		return nil, nil
	}
	return &info, nil
}

func AssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("and_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
}


func Apply(ctx context.Context, info *ReleaseInfo) error {
	name := AssetName()
	var dlURL string
	for _, a := range info.Assets {
		if a.Name == name {
			dlURL = a.BrowserDownloadURL
			break
		}
	}
	if dlURL == "" {
		return fmt.Errorf("bu platform için asset bulunamadı (%s)", name)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("exe yolu alınamadı: %w", err)
	}

	tmpPath := exe + ".new"
	if err := downloadFile(ctx, dlURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("indirme hatası: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	oldPath := exe + ".old"
	os.Remove(oldPath)
	if err := os.Rename(exe, oldPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("mevcut exe taşınamadı: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Rename(oldPath, exe)
		os.Remove(tmpPath)
		return fmt.Errorf("yeni exe yerleştirilemedi: %w", err)
	}
	go os.Remove(oldPath)
	return nil
}

func SelfRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("updater: exe path: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: restart: %w", err)
	}
	return nil
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AND/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	const maxBinaryBytes = 128 << 20
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxBinaryBytes))
	return err
}

func isNewer(a, b string) bool {
	return semverKey(a) > semverKey(b)
}

func semverKey(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	nums := [3]int{}
	for i, p := range parts {
		if i >= 3 {
			break
		}
		fmt.Sscanf(strings.TrimSpace(p), "%d", &nums[i])
	}
	return fmt.Sprintf("%010d%010d%010d", nums[0], nums[1], nums[2])
}
