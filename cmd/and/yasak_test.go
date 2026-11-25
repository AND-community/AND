package main

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/forum"
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

func testForumAdapter(t *testing.T) *forumAdapter {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	f, err := forum.New(id, nil, filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return &forumAdapter{store: f, dir: t.TempDir(), myPubHex: hex.EncodeToString(id.PublicKey())}
}

func TestForumAdapter_OlmayanKonuyuSilmek404(t *testing.T) {
	a := testForumAdapter(t)
	if err := a.DeleteOwnPost(context.Background(), strings.Repeat("ab", 16)); !errors.Is(err, pluginapi.ErrBulunamadi) {
		t.Fatalf("ErrBulunamadi beklenirdi: %v", err)
	}
}

func TestForumAdapter_BilinmeyenProfil404(t *testing.T) {
	a := testForumAdapter(t)
	if _, err := a.Profile(strings.Repeat("cd", 32)); !errors.Is(err, pluginapi.ErrBulunamadi) {
		t.Fatalf("ErrBulunamadi beklenirdi: %v", err)
	}
}

func TestForumAdapter_ForumChangesZamanAsimi(t *testing.T) {
	a := testForumAdapter(t)
	ctx, iptal := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer iptal()
	d, err := a.ForumChanges(ctx, 0)
	if err != nil || d.Surum != 0 || len(d.Konular) != 0 || d.Hepsi {
		t.Fatalf("değişiklik yok beklenirdi: %+v %v", d, err)
	}
}

var (
	_ pluginapi.ForumChangesBackend = (*forumAdapter)(nil)
	_ pluginapi.OwnDeleteBackend    = (*forumAdapter)(nil)
	_ pluginapi.ProfileBackend      = (*forumAdapter)(nil)
)
