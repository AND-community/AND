package forum

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestMain disables the new-identity throttle (see isNewIdentity) for this
// package's whole test run by default: every pre-existing test creates
// brand-new authors and asserts against the normal rlMaxPosts/rlMaxReplies/
// cooldown constants, with no notion of a stricter "first 24h" limit.
// Tests that specifically exercise the new-identity throttle re-enable it
// (via newIdentityGracePeriod) for their own duration — see
// TestRateLimiter_NewIdentity_* below.
func TestMain(m *testing.M) {
	newIdentityGracePeriod = 0
	os.Exit(m.Run())
}

// noCooldown zeroes rlPostCooldown/rlReplyCooldown for the duration of a
// test — tests that fire several allowPost/allowReply calls for the SAME
// author back-to-back (no real time gap) are testing the count-window
// limit in isolation, not the cooldown; without this they'd trip the
// cooldown after the very first call.
func noCooldown(t *testing.T) {
	t.Helper()
	oldPost, oldReply := rlPostCooldown, rlReplyCooldown
	rlPostCooldown, rlReplyCooldown = 0, 0
	t.Cleanup(func() { rlPostCooldown, rlReplyCooldown = oldPost, oldReply })
}

// withNewIdentityGrace re-enables the new-identity throttle (disabled
// package-wide by TestMain) for a single test's duration.
func withNewIdentityGrace(t *testing.T, d time.Duration) {
	t.Helper()
	old := newIdentityGracePeriod
	newIdentityGracePeriod = d
	t.Cleanup(func() { newIdentityGracePeriod = old })
}

func TestRateLimiter_AllowPost_UnderLimit(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		if !rl.allowPost("key1", true) {
			t.Fatalf("post %d izin verilmeli (limit %d)", i+1, rlMaxPosts)
		}
	}
}

func TestRateLimiter_AllowPost_ExceedsLimit(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1", true)
	}
	if rl.allowPost("key1", true) {
		t.Fatal("limit aşıldıktan sonra post reddedilmeli")
	}
}

func TestRateLimiter_AllowReply_UnderLimit(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	for i := 0; i < rlMaxReplies; i++ {
		if !rl.allowReply("key1", true) {
			t.Fatalf("yanıt %d izin verilmeli (limit %d)", i+1, rlMaxReplies)
		}
	}
}

func TestRateLimiter_AllowReply_ExceedsLimit(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	for i := 0; i < rlMaxReplies; i++ {
		rl.allowReply("key1", true)
	}
	if rl.allowReply("key1", true) {
		t.Fatal("yanıt limiti aşıldıktan sonra reddedilmeli")
	}
}

func TestRateLimiter_DifferentAuthors_Independent(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1", true)
	}
	// key2'nin limiti key1'den bağımsız olmalı
	if !rl.allowPost("key2", true) {
		t.Fatal("farklı yazar limitten etkilenmemeli")
	}
}

func TestRateLimiter_GlobalPostLimit_SybilProtection(t *testing.T) {
	rl := newRateLimiter("")
	// Her biri farklı anahtar, toplamda global limiti doldur
	for i := 0; i < rlGlobalMaxPosts; i++ {
		key := fmt.Sprintf("sybil_key_%d", i)
		if !rl.allowPost(key, true) {
			t.Fatalf("global limit dolmadan post %d reddedildi", i)
		}
	}
	// Yeni kimlik bile olsa global limit aşılınca reddedilmeli
	if rl.allowPost("brand_new_identity", true) {
		t.Fatal("global limit dolunca yeni kimlik de reddedilmeli (Sybil koruması)")
	}
}

func TestRateLimiter_GlobalLimit_IndependentOfPerAuthor(t *testing.T) {
	noCooldown(t)
	rl := newRateLimiter("")
	// Bir yazarın kişisel limitini dolduralım
	for i := 0; i < rlMaxPosts; i++ {
		rl.allowPost("key1", true)
	}
	// Global henüz dolmadı, başka yazar devam edebilmeli
	if !rl.allowPost("key2", true) {
		t.Fatal("global limit dolmamışken başka yazar post atabilmeli")
	}
}

func TestRateLimiter_SaveAndLoad(t *testing.T) {
	noCooldown(t)
	dir := t.TempDir()
	rl := newRateLimiter(dir)

	// Bir post ekle ve kaydet
	if !rl.allowPost("key_persist", true) {
		t.Fatal("ilk post izin verilmeli")
	}
	rl.save()

	// Yeni rate limiter aynı dizinden yüklenmeli
	rl2 := newRateLimiter(dir)
	// key_persist zaten 1 post kullandı, 9 daha kalan
	count := 0
	for rl2.allowPost("key_persist", true) {
		count++
	}
	if count >= rlMaxPosts {
		t.Fatal("yüklenen durum önceki postu saymıyor (limit aşıldı)")
	}
}

func TestRateLimiter_MutedState_PersistsAcrossRestart(t *testing.T) {
	old := muteViolationThreshold
	muteViolationThreshold = 2
	t.Cleanup(func() { muteViolationThreshold = old })

	dir := t.TempDir()
	rl := newRateLimiter(dir)
	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key_persist")
	}
	if !rl.muted("key_persist") {
		t.Fatal("test kurulumu: eşiğe ulaşınca susturulmuş olmalıydı")
	}
	rl.save()

	// "Yeniden başlatma" simülasyonu: aynı dataDir ile yeni bir rateLimiter.
	rl2 := newRateLimiter(dir)
	if !rl2.muted("key_persist") {
		t.Fatal("aktif susturma, diskten yeniden yüklendikten sonra da geçerli olmalı — yoksa restart ısrarlı bir spammer'a anında temiz sayfa verir")
	}
}

func TestRateLimiter_ExpiredMutedState_NotPersistedAsActive(t *testing.T) {
	dir := t.TempDir()
	rl := newRateLimiter(dir)
	rl.mu.Lock()
	rl.bucket("key_expired").MutedUntil = time.Now().Add(-time.Hour) // çoktan dolmuş
	rl.mu.Unlock()
	rl.save()

	rl2 := newRateLimiter(dir)
	if rl2.muted("key_expired") {
		t.Fatal("süresi çoktan dolmuş bir susturma, yeniden yüklendikten sonra aktif görünmemeliydi")
	}
}

func TestRateLimiter_RecentTexts_PersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	rl := newRateLimiter(dir)
	for i := 0; i < dupMaxRepeats; i++ {
		if rl.isDuplicate("key_persist", "aynı metin") {
			t.Fatalf("tekrar %d, dupMaxRepeats sınırına kadar kopya sayılmamalıydı", i+1)
		}
	}
	rl.save()

	rl2 := newRateLimiter(dir)
	if !rl2.isDuplicate("key_persist", "aynı metin") {
		t.Fatal("dupMaxRepeats'e ulaşmış sayaç, yeniden yüklendikten sonra da korunmalıydı")
	}
}

// ─── Ardışık hızlı gönderim soğuması (cooldown) ────────────────────────────

func TestRateLimiter_AllowPost_CooldownRejectsRapidRepeat(t *testing.T) {
	rl := newRateLimiter("")
	if !rl.allowPost("key1", true) {
		t.Fatal("ilk post izin verilmeli")
	}
	if rl.allowPost("key1", true) {
		t.Fatal("rlPostCooldown dolmadan aynı yazardan ikinci bir post reddedilmeliydi")
	}
}

func TestRateLimiter_AllowPost_AllowedAfterCooldownElapses(t *testing.T) {
	rl := newRateLimiter("")
	if !rl.allowPost("key1", true) {
		t.Fatal("ilk post izin verilmeli")
	}
	// Soğuma süresinin çoktan geçtiğini simüle et (gerçek bir sleep yerine
	// son zaman damgasını geriye al).
	b := rl.bucket("key1")
	b.PostTimes[len(b.PostTimes)-1] = time.Now().Add(-rlPostCooldown - time.Second)

	if !rl.allowPost("key1", true) {
		t.Fatal("soğuma süresi geçtikten sonra aynı yazardan bir post kabul edilmeliydi")
	}
}

func TestRateLimiter_AllowPost_CooldownSkippedWhenSyncing(t *testing.T) {
	rl := newRateLimiter("")
	if !rl.allowPost("key1", true) {
		t.Fatal("ilk post izin verilmeli")
	}
	// checkCooldown=false — SyncWithPeer'ın eski/geçmiş postları toplu
	// işlerken kullandığı yol; aynı yazarın art arda gelen eski postları
	// yalnızca BİZİM işleme hızımız yüzünden reddedilmemeli.
	if !rl.allowPost("key1", false) {
		t.Fatal("checkCooldown=false iken soğuma uygulanmamalıydı")
	}
}

func TestRateLimiter_AllowReply_CooldownRejectsRapidRepeat(t *testing.T) {
	rl := newRateLimiter("")
	if !rl.allowReply("key1", true) {
		t.Fatal("ilk yanıt izin verilmeli")
	}
	if rl.allowReply("key1", true) {
		t.Fatal("rlReplyCooldown dolmadan aynı yazardan ikinci bir yanıt reddedilmeliydi")
	}
}

func TestRateLimiter_AllowReply_AllowedAfterCooldownElapses(t *testing.T) {
	rl := newRateLimiter("")
	if !rl.allowReply("key1", true) {
		t.Fatal("ilk yanıt izin verilmeli")
	}
	b := rl.bucket("key1")
	b.ReplyTimes[len(b.ReplyTimes)-1] = time.Now().Add(-rlReplyCooldown - time.Second)

	if !rl.allowReply("key1", true) {
		t.Fatal("soğuma süresi geçtikten sonra aynı yazardan bir yanıt kabul edilmeliydi")
	}
}

// ─── Otomatik geçici susturma (auto-mute) ────────────────────────────────────

func TestRateLimiter_RecordViolation_MutesAfterThreshold(t *testing.T) {
	old := muteViolationThreshold
	muteViolationThreshold = 3
	t.Cleanup(func() { muteViolationThreshold = old })

	rl := newRateLimiter("")
	if rl.muted("key1") {
		t.Fatal("hiç ihlal yokken susturulmuş olmamalı")
	}
	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key1")
	}
	if !rl.muted("key1") {
		t.Fatal("eşiğe ulaşan bir yazar otomatik susturulmalıydı")
	}
}

func TestRateLimiter_Muted_ExpiresAfterMuteDuration(t *testing.T) {
	rl := newRateLimiter("")
	rl.mu.Lock()
	rl.bucket("key1").MutedUntil = time.Now().Add(-time.Second) // çoktan dolmuş
	rl.mu.Unlock()

	if rl.muted("key1") {
		t.Fatal("süresi dolmuş bir susturma artık geçerli olmamalı")
	}
}

func TestRateLimiter_RecordViolation_DifferentAuthorsIndependent(t *testing.T) {
	old := muteViolationThreshold
	muteViolationThreshold = 2
	t.Cleanup(func() { muteViolationThreshold = old })

	rl := newRateLimiter("")
	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("spammer")
	}
	if !rl.muted("spammer") {
		t.Fatal("eşiğe ulaşan yazar susturulmalıydı")
	}
	if rl.muted("innocent") {
		t.Fatal("farklı bir yazar diğerinin ihlallerinden etkilenmemeli")
	}
}

func TestRateLimiter_RecordViolation_EscalatesMuteDurationOnRepeatOffense(t *testing.T) {
	oldThreshold, oldDuration, oldCap := muteViolationThreshold, muteDuration, muteDurationCap
	muteViolationThreshold = 2
	muteDuration = time.Minute
	muteDurationCap = time.Hour
	t.Cleanup(func() {
		muteViolationThreshold, muteDuration, muteDurationCap = oldThreshold, oldDuration, oldCap
	})

	rl := newRateLimiter("")
	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key1")
	}
	rl.mu.Lock()
	b := rl.bucket("key1")
	if b.MuteCount != 1 {
		t.Fatalf("ilk susturmadan sonra MuteCount = %d, want 1", b.MuteCount)
	}
	firstRemaining := time.Until(b.MutedUntil)
	// İlk susturmanın süresinin dolduğunu simüle et, ikinci ihlal döngüsünü tetikle.
	b.MutedUntil = time.Now().Add(-time.Second)
	rl.mu.Unlock()

	rl.recordViolation("key1")

	rl.mu.Lock()
	secondCount := b.MuteCount
	secondRemaining := time.Until(b.MutedUntil)
	rl.mu.Unlock()

	if secondCount != 2 {
		t.Fatalf("ikinci susturmadan sonra MuteCount = %d, want 2", secondCount)
	}
	if secondRemaining <= firstRemaining {
		t.Fatalf("ikinci susturma ilkinden daha uzun sürmeliydi: ilk=%v ikinci=%v", firstRemaining, secondRemaining)
	}
	if secondRemaining < muteDuration+30*time.Second {
		t.Fatalf("ikinci susturma ~2x muteDuration civarında olmalıydı, got %v", secondRemaining)
	}
}

func TestRateLimiter_RecordViolation_MuteCountDecaysAfterCleanPeriod(t *testing.T) {
	oldThreshold, oldDuration := muteViolationThreshold, muteDuration
	muteViolationThreshold = 2
	muteDuration = time.Minute
	t.Cleanup(func() { muteViolationThreshold, muteDuration = oldThreshold, oldDuration })

	rl := newRateLimiter("")
	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key1")
	}
	rl.mu.Lock()
	b := rl.bucket("key1")
	if b.MuteCount != 1 {
		t.Fatalf("kurulum: MuteCount = %d, want 1", b.MuteCount)
	}
	// Uzun süredir temiz olduğunu simüle et: susturma ve ihlal geçmişi
	// çoktan sona ermiş, son susturma da muteCountDecay'in çok ötesinde.
	b.MutedUntil = time.Now().Add(-time.Hour)
	b.ViolationTimes = nil
	b.LastMuteAt = time.Now().Add(-muteCountDecay - time.Hour)
	rl.mu.Unlock()

	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key1")
	}

	rl.mu.Lock()
	gotCount := b.MuteCount
	rl.mu.Unlock()
	if gotCount != 1 {
		t.Fatalf("uzun süredir temiz kalan bir yazar için sayaç sıfırlanıp yeniden 1'den başlamalıydı, got %d", gotCount)
	}
}

func TestRateLimiter_RecordViolation_MuteDurationCapped(t *testing.T) {
	oldThreshold, oldDuration, oldCap := muteViolationThreshold, muteDuration, muteDurationCap
	muteViolationThreshold = 2
	muteDuration = time.Hour
	muteDurationCap = 90 * time.Minute
	t.Cleanup(func() {
		muteViolationThreshold, muteDuration, muteDurationCap = oldThreshold, oldDuration, oldCap
	})

	rl := newRateLimiter("")
	rl.mu.Lock()
	b := rl.bucket("key1")
	b.MuteCount = 5 // tavan olmasaydı 5 × 1 saat = 5 saat sürerdi
	rl.mu.Unlock()

	for i := 0; i < muteViolationThreshold; i++ {
		rl.recordViolation("key1")
	}

	rl.mu.Lock()
	remaining := time.Until(b.MutedUntil)
	rl.mu.Unlock()
	if remaining > muteDurationCap+time.Minute {
		t.Fatalf("susturma süresi muteDurationCap'i aşmamalıydı, got %v (cap %v)", remaining, muteDurationCap)
	}
}

// ─── Yeni kimlik kısıtlaması (Sybil maliyetini artırma) ─────────────────────

func TestRateLimiter_NewIdentity_StricterPostLimitWithinGracePeriod(t *testing.T) {
	noCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	rl := newRateLimiter("")
	for i := 0; i < newIdentityMaxPosts; i++ {
		if !rl.allowPost("yeni-kimlik", true) {
			t.Fatalf("post %d izin verilmeliydi (yeni kimlik limiti %d)", i+1, newIdentityMaxPosts)
		}
	}
	if rl.allowPost("yeni-kimlik", true) {
		t.Fatal("yeni kimlik limiti aşılınca reddedilmeliydi")
	}
}

func TestRateLimiter_NewIdentity_NormalLimitAfterGracePeriodElapses(t *testing.T) {
	noCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	rl := newRateLimiter("")
	rl.mu.Lock()
	b := rl.bucket("eski-kimlik")
	b.FirstSeen = time.Now().Add(-25 * time.Hour) // grace period çoktan geçmiş
	rl.mu.Unlock()

	for i := 0; i < rlMaxPosts; i++ {
		if !rl.allowPost("eski-kimlik", true) {
			t.Fatalf("post %d izin verilmeliydi (grace period geçmiş, normal limit %d)", i+1, rlMaxPosts)
		}
	}
}

func TestRateLimiter_NewIdentity_ZeroFirstSeenTreatedAsEstablished(t *testing.T) {
	// Bu alan eklenmeden önce yazılmış eski bir forum_rates.json'dan
	// yüklenmiş bir bucket'ı simüle eder — FirstSeen sıfır değerinde.
	noCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	rl := newRateLimiter("")
	rl.mu.Lock()
	rl.authors["eski-kayit"] = &authorBucket{}
	rl.mu.Unlock()

	for i := 0; i < rlMaxPosts; i++ {
		if !rl.allowPost("eski-kayit", true) {
			t.Fatalf("post %d izin verilmeliydi (FirstSeen sıfırsa yeni kimlik sayılmamalı)", i+1)
		}
	}
}

func TestRateLimiter_NewIdentity_SyncBackfillNotThrottled(t *testing.T) {
	// Yeni kimlik kısıtlaması yalnızca canlı alımda (checkCooldown=true)
	// uygulanmalı — aksi halde ağa yeni katılan bir düğüm, senkronizasyon
	// sırasında çoktan var olan üretken bir yazarın geçmiş gönderilerinin
	// çoğunu "yeni kimlik" sayıp gereksiz yere reddederdi.
	withNewIdentityGrace(t, 24*time.Hour)

	rl := newRateLimiter("")
	n := newIdentityMaxPosts + 5 // yeni-kimlik limitini aşan bir miktar
	for i := 0; i < n; i++ {
		if !rl.allowPost("senkron-yazar", false) {
			t.Fatalf("post %d, sync backfill'de (checkCooldown=false) yeni-kimlik limitine takılmamalıydı", i+1)
		}
	}
}

func TestRateLimiter_NewIdentity_StricterReplyLimitWithinGracePeriod(t *testing.T) {
	noCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	rl := newRateLimiter("")
	for i := 0; i < newIdentityMaxReplies; i++ {
		if !rl.allowReply("yeni-kimlik", true) {
			t.Fatalf("yanıt %d izin verilmeliydi (yeni kimlik limiti %d)", i+1, newIdentityMaxReplies)
		}
	}
	if rl.allowReply("yeni-kimlik", true) {
		t.Fatal("yeni kimlik yanıt limiti aşılınca reddedilmeliydi")
	}
}

func TestForum_HandleMessage_MutedAuthorPostSkippedEvenIfValid(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)

	f.rl.mu.Lock()
	f.rl.bucket(authorKey).MutedUntil = time.Now().Add(muteDuration)
	f.rl.mu.Unlock()

	p := newSignedPost(priv, pub, "Go", "Başlık", "geçerli ama susturulmuş")
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))

	if f.PostByID(p.ID) != nil {
		t.Fatal("susturulmuş bir yazardan gelen (imzası geçerli olsa bile) bir post kabul edilmemeliydi")
	}
}

// ─── Aynı/benzer içerik tekrarı tespiti (duplicate detection) ───────────────

func TestRateLimiter_IsDuplicate_RepeatsUpToThresholdAccepted(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < dupMaxRepeats; i++ {
		if rl.isDuplicate("key1", "merhaba dünya") {
			t.Fatalf("tekrar %d, dupMaxRepeats (%d) sınırına kadar kopya sayılmamalıydı", i+1, dupMaxRepeats)
		}
	}
}

func TestRateLimiter_IsDuplicate_RepeatsBeyondThresholdRejected(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < dupMaxRepeats; i++ {
		rl.isDuplicate("key1", "merhaba dünya")
	}
	if !rl.isDuplicate("key1", "merhaba dünya") {
		t.Fatal("dupMaxRepeats sonrası bir tekrar kopya sayılmalıydı")
	}
}

func TestRateLimiter_IsDuplicate_CaseAndWhitespaceCountedTogether(t *testing.T) {
	rl := newRateLimiter("")
	variants := []string{"Merhaba Dünya", "  merhaba dünya  ", "MERHABA DÜNYA"}
	for i := 0; i < dupMaxRepeats; i++ {
		rl.isDuplicate("key1", variants[i%len(variants)])
	}
	if !rl.isDuplicate("key1", "merhaba dünya") {
		t.Fatal("büyük/küçük harf ve boşluk dışında aynı olan varyantlar aynı sayaç altında birikip eşiği aşmalıydı")
	}
}

func TestRateLimiter_IsDuplicate_DifferentTextNotFlagged(t *testing.T) {
	rl := newRateLimiter("")
	rl.isDuplicate("key1", "merhaba")
	if rl.isDuplicate("key1", "nasılsın") {
		t.Fatal("farklı içerikli bir metin kopya sayılmamalıydı")
	}
}

func TestRateLimiter_IsDuplicate_DifferentAuthorsIndependent(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < dupMaxRepeats+2; i++ {
		rl.isDuplicate("key1", "aynı metin")
	}
	if rl.isDuplicate("key2", "aynı metin") {
		t.Fatal("başka bir yazarın aynı metni göndermesi, diğer yazarın sayacından etkilenmemeli")
	}
}

func TestRateLimiter_IsDuplicate_OutsideWindowResetsCounter(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < dupMaxRepeats; i++ {
		rl.isDuplicate("key1", "tekrar edecek")
	}

	rl.mu.Lock()
	entry := rl.bucket("key1").RecentTexts["tekrar edecek"]
	entry.At = time.Now().Add(-dupWindow - time.Second)
	rl.bucket("key1").RecentTexts["tekrar edecek"] = entry
	rl.mu.Unlock()

	if rl.isDuplicate("key1", "tekrar edecek") {
		t.Fatal("dupWindow dışına çıkınca sayaç sıfırlanıp yeni bir tekrara izin vermeliydi")
	}
}

func TestForum_HandleMessage_DuplicatePostBodyRejected(t *testing.T) {
	// rlPostCooldown burada test edilmiyor — dupMaxRepeats+1 (7) kadar post
	// göndermek gerçek soğumayla çakışmasın diye sıfırla. rlMaxPosts (10,
	// const) zaten dupMaxRepeats+1'in üzerinde olduğundan saatlik sayaç
	// sınırıyla bir çakışma yok.
	noCooldown(t)

	f := newTestForum(t)
	pub, priv := genPostKey(t)

	for i := 0; i < dupMaxRepeats; i++ {
		p := newSignedPost(priv, pub, "Go", fmt.Sprintf("Başlık %d", i), "tekrar eden içerik")
		f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))
		if f.PostByID(p.ID) == nil {
			t.Fatalf("tekrar %d, dupMaxRepeats (%d) sınırına kadar kabul edilmeliydi", i+1, dupMaxRepeats)
		}
	}

	extra := newSignedPost(priv, pub, "Go", "Son Başlık", "tekrar eden içerik")
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: extra}))

	if f.PostByID(extra.ID) != nil {
		t.Fatal("dupMaxRepeats sonrası aynı gövdeye sahip bir post reddedilmeliydi")
	}
}

// ─── Replay korumasının hız-sınırı/kopya-mesaj kotasını tüketmemesi ─────────

// TestForum_HandleMessage_ReplayDoesNotConsumeRateLimitOrDupQuota, tam
// olarak aynı (geçerli imzalı) postun tekrar tekrar "replay" edilmesinin —
// özel anahtara ihtiyaç duymadan, libp2p'nin kendi "seen" önbelleği
// dolduktan sonra biri yakaladığı eski bir postu aynen yeniden
// yayınlayabilir — kurbanın hız sınırı kotasını TÜKETMEDİĞİNİ doğrular. ID
// tekrarı kontrolü artık allowPost()/isDuplicate()'ten önce çalıştığından,
// replay'ler bu sayaçlara hiç dokunmadan erken reddedilir.
func TestForum_HandleMessage_ReplayDoesNotConsumeRateLimitOrDupQuota(t *testing.T) {
	noCooldown(t)
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	p := newSignedPost(priv, pub, "Go", "Başlık", "İçerik")
	data := envJSON(t, envelope{Type: "post", Post: p})

	// rlMaxPosts (10) sınırının çok üzerinde replay gönder — hiçbiri
	// yazarın kotasını tüketmemeli.
	for i := 0; i < rlMaxPosts+5; i++ {
		f.handleMessage(data)
	}
	if f.PostCount("Go") != 1 {
		t.Fatalf("replay'ler kalıcılaştırılmamalıydı, got %d post", f.PostCount("Go"))
	}

	// Yazarın kotası hâlâ dolu olmamalı — gerçek YENİ bir post kabul
	// edilebilmeli.
	fresh := newSignedPost(priv, pub, "Go", "Farklı Başlık", "Farklı İçerik")
	f.handleMessage(envJSON(t, envelope{Type: "post", Post: fresh}))
	if f.PostByID(fresh.ID) == nil {
		t.Fatal("replay'ler kotayı tüketmiş olmalıydı ki bu yeni post reddedildi")
	}
}

// ─── "delete" mesaj tipi: hız sınırı + susturma ──────────────────────────────

func newSignedDelete(t *testing.T, priv ed25519.PrivateKey, authorKey, postID string) *DeleteMsg {
	t.Helper()
	d := &DeleteMsg{PostID: postID, AuthorKey: authorKey, DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(d)))
	return d
}

func TestRateLimiter_AllowDelete_UnderLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxDeletes; i++ {
		if !rl.allowDelete("key1") {
			t.Fatalf("silme %d izin verilmeli (limit %d)", i+1, rlMaxDeletes)
		}
	}
}

func TestRateLimiter_AllowDelete_ExceedsLimit(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlMaxDeletes; i++ {
		rl.allowDelete("key1")
	}
	if rl.allowDelete("key1") {
		t.Fatal("limit aşıldıktan sonra silme reddedilmeli")
	}
}

func TestRateLimiter_AllowDelete_GlobalSybilProtection(t *testing.T) {
	rl := newRateLimiter("")
	for i := 0; i < rlGlobalMaxDeletes; i++ {
		key := fmt.Sprintf("sybil_key_%d", i)
		if !rl.allowDelete(key) {
			t.Fatalf("global limit dolmadan silme %d reddedildi", i)
		}
	}
	if rl.allowDelete("brand_new_identity") {
		t.Fatal("global limit dolunca yeni kimlik de reddedilmeli (Sybil koruması)")
	}
}

func TestForum_HandleMessage_DeleteRateLimited(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)

	// rlMaxDeletes sınırına kadar sahte (var olmayan postID'ler için) silme
	// mesajı gönder — hepsi kabul edilmeli. Postlar yerel olarak hiç
	// bilinmediğinden bunlar kalıcı tombstone değil, bekleyen/doğrulanmamış
	// talep olarak saklanır (bkz. applyDelete/storePost) — rate limiting
	// bu ayrımdan bağımsız, allowDelete zaten handleMessage'da önceden
	// kontrol ediliyor.
	for i := 0; i < rlMaxDeletes; i++ {
		d := newSignedDelete(t, priv, authorKey, fmt.Sprintf("post-%d", i))
		f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))
	}
	if len(f.pendingDeletes) != rlMaxDeletes {
		t.Fatalf("rlMaxDeletes kadar bekleyen silme talebi oluşmalıydı, got %d", len(f.pendingDeletes))
	}

	// Sınırı aşan bir silme daha reddedilmeli.
	extra := newSignedDelete(t, priv, authorKey, "post-extra")
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: extra}))
	if _, ok := f.pendingDeletes["post-extra"]; ok {
		t.Fatal("rlMaxDeletes sonrası bir silme mesajı reddedilmeliydi")
	}
}

func TestForum_HandleMessage_MutedAuthorDeleteSkipped(t *testing.T) {
	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)

	f.rl.mu.Lock()
	f.rl.bucket(authorKey).MutedUntil = time.Now().Add(muteDuration)
	f.rl.mu.Unlock()

	d := newSignedDelete(t, priv, authorKey, "some-post")
	f.handleMessage(envJSON(t, envelope{Type: "delete", Delete: d}))

	if _, ok := f.pendingDeletes["some-post"]; ok {
		t.Fatal("susturulmuş bir yazardan gelen (imzası geçerli olsa bile) bir silme mesajı işlenmemeliydi")
	}
}

// ─── Kopya-içerik reddi otomatik susturmayı tetiklememeli ───────────────────

func TestForum_HandleMessage_DuplicateRejectionDoesNotTriggerMute(t *testing.T) {
	noCooldown(t)
	oldThreshold := muteViolationThreshold
	muteViolationThreshold = 3 // çok düşük bir eşik — eskiden olsaydı kesin tetiklenirdi
	t.Cleanup(func() { muteViolationThreshold = oldThreshold })

	f := newTestForum(t)
	pub, priv := genPostKey(t)
	authorKey := hex.EncodeToString(pub)

	// rlMaxPosts (10, const) sınırının İÇİNDE kal — burada test edilen
	// yalnızca kopya-içerik reddinin kendisi, saatlik sayaç sınırının
	// tüketilmesi değil (o ayrı ve doğru şekilde recordViolation'ı
	// tetiklemeye devam ediyor — bkz. TestRateLimiter_AllowPost_ExceedsLimit).
	n := rlMaxPosts - 1
	for i := 0; i < n; i++ {
		p := newSignedPost(priv, pub, "Go", fmt.Sprintf("Başlık %d", i), "tekrar eden içerik")
		f.handleMessage(envJSON(t, envelope{Type: "post", Post: p}))
	}

	if f.rl.muted(authorKey) {
		t.Fatal("yalnızca kopya-içerik reddi otomatik susturmayı tetiklememeliydi")
	}
	if f.PostCount("Go") != dupMaxRepeats {
		t.Fatalf("dupMaxRepeats kadar post kabul edilmeliydi, got %d", f.PostCount("Go"))
	}
}
