package pluginapi

import (
	"testing"
	"time"
)

func TestLocalActionLimiter_AllowsUpToMaxThenBlocks(t *testing.T) {
	l := newLocalActionLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow() {
			t.Fatalf("istek %d izin verilmeliydi (sınır 3)", i+1)
		}
	}
	if l.allow() {
		t.Fatal("sınır (3) aşıldıktan sonra izin verilmemeliydi")
	}
}

func TestLocalActionLimiter_RecoversAfterWindowPasses(t *testing.T) {
	l := newLocalActionLimiter(1, 30*time.Millisecond)
	if !l.allow() {
		t.Fatal("ilk istek izin verilmeliydi")
	}
	if l.allow() {
		t.Fatal("pencere dolmadan ikinci istek reddedilmeliydi")
	}
	time.Sleep(40 * time.Millisecond)
	if !l.allow() {
		t.Fatal("pencere geçtikten sonra yeniden izin verilmeliydi")
	}
}

func TestLocalActionLimiter_IndependentInstancesDoNotShareBudget(t *testing.T) {
	a := newLocalActionLimiter(1, time.Minute)
	b := newLocalActionLimiter(1, time.Minute)
	if !a.allow() || !b.allow() {
		t.Fatal("bağımsız iki sınırlayıcı birbirini etkilememeli")
	}
}
