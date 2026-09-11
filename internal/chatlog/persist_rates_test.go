package chatlog

// persist_rates_test.go — chat_rates.json'a susturma/ihlal/kopya-metin
// durumunun kalıcılığını kapsar. Bkz. internal/forum/ratelimit_test.go'daki
// eşdeğer testler (TestRateLimiter_MutedState_PersistsAcrossRestart vb.) —
// bu paket öncesinde bu durumun tamamı yalnızca bellekteydi; bir düğüm
// yeniden başladığında ısrarlı bir spammer anında "temiz" görünürdü.

import (
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/storage"
)

func newPersistableStore(t *testing.T, dataDir string) *Store {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(id, nil, db, dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestStore_MutedState_PersistsAcrossRestart(t *testing.T) {
	old := muteViolationThreshold
	muteViolationThreshold = 2
	t.Cleanup(func() { muteViolationThreshold = old })

	dir := t.TempDir()
	s := newPersistableStore(t, dir)
	for i := 0; i < muteViolationThreshold; i++ {
		s.recordViolation("key_persist")
	}
	if !s.muted("key_persist") {
		t.Fatal("test kurulumu: eşiğe ulaşınca susturulmuş olmalıydı")
	}
	s.saveRates()

	s2 := newPersistableStore(t, dir)
	if !s2.muted("key_persist") {
		t.Fatal("aktif susturma, diskten yeniden yüklendikten sonra da geçerli olmalı")
	}
}

func TestStore_ExpiredMutedState_NotPersistedAsActive(t *testing.T) {
	dir := t.TempDir()
	s := newPersistableStore(t, dir)
	s.mu.Lock()
	s.bucket("key_expired").MutedUntil = time.Now().Add(-time.Hour)
	s.mu.Unlock()
	s.saveRates()

	s2 := newPersistableStore(t, dir)
	if s2.muted("key_expired") {
		t.Fatal("süresi çoktan dolmuş bir susturma, yeniden yüklendikten sonra aktif görünmemeliydi")
	}
}

func TestStore_RecentTexts_PersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s := newPersistableStore(t, dir)
	for i := 0; i < dupMaxRepeats; i++ {
		if s.isDuplicate("key_persist", "aynı metin") {
			t.Fatalf("tekrar %d, dupMaxRepeats sınırına kadar kopya sayılmamalıydı", i+1)
		}
	}
	s.saveRates()

	s2 := newPersistableStore(t, dir)
	if !s2.isDuplicate("key_persist", "aynı metin") {
		t.Fatal("dupMaxRepeats'e ulaşmış sayaç, yeniden yüklendikten sonra da korunmalıydı")
	}
}
