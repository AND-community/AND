package main

import "testing"

func TestMasaustuSecilsin(t *testing.T) {
	ortam := func(degerler map[string]string) func(string) string {
		return func(k string) string { return degerler[k] }
	}
	bos := ortam(nil)
	cases := []struct {
		ad       string
		args     []string
		env      func(string) string
		goos     string
		beklenen bool
	}{
		{"varsayılan Windows'ta masaüstü", nil, bos, "windows", true},
		{"varsayılan macOS'ta masaüstü", nil, bos, "darwin", true},
		{"--terminal", []string{"--terminal"}, bos, "windows", false},
		{"--tui", []string{"--tui"}, bos, "windows", false},
		{"--masaustu SSH'ta bile", []string{"--masaustu"}, ortam(map[string]string{"SSH_TTY": "/dev/pts/0"}), "linux", true},
		{"AND_ARAYUZ=terminal", nil, ortam(map[string]string{"AND_ARAYUZ": "Terminal"}), "windows", false},
		{"AND_ARAYUZ=masaustu ekransız", nil, ortam(map[string]string{"AND_ARAYUZ": "masaustu"}), "linux", true},
		{"SSH oturumu", nil, ortam(map[string]string{"SSH_CONNECTION": "1.2.3.4 22 5.6.7.8 22"}), "windows", false},
		{"ekransız Linux", nil, bos, "linux", false},
		{"X11 Linux", nil, ortam(map[string]string{"DISPLAY": ":0"}), "linux", true},
		{"Wayland Linux", nil, ortam(map[string]string{"WAYLAND_DISPLAY": "wayland-0"}), "linux", true},
		{"bilinmeyen argüman varsayılana düşer", []string{"--bilinmeyen"}, bos, "windows", true},
	}
	for _, c := range cases {
		if got := masaustuSecilsin(c.args, c.env, c.goos); got != c.beklenen {
			t.Errorf("%s: %v, beklenen %v", c.ad, got, c.beklenen)
		}
	}
}
