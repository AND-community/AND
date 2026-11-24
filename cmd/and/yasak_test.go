package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lucian95511/and/pluginapi"
)

// Yetkisi olmayan (kurucu değil, moderatör sertifikası yok) ya da kendini
// yasaklamaya çalışan çağıran ErrYetkisiz alır; ağa hiçbir şey yayınlanmaz.
func TestModAdapter_BanYetkisiz(t *testing.T) {
	ben := strings.Repeat("ab", 32)
	a := &modAdapter{dir: t.TempDir(), myPubHex: ben}
	for _, hedef := range []string{strings.Repeat("cd", 32), ben} {
		err := a.Ban(context.Background(), hedef, time.Hour, false, "spam")
		if !errors.Is(err, pluginapi.ErrYetkisiz) {
			t.Errorf("%s: ErrYetkisiz beklenirdi: %v", hedef[:4], err)
		}
	}
}

var _ pluginapi.BanBackend = (*modAdapter)(nil)
