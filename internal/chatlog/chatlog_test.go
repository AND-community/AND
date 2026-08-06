package chatlog

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	stdcrypto "github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/storage"
)

// TestMain disables the new-identity throttle (see isNewIdentity) for this
// package's whole test run by default — mirrors internal/forum/
// ratelimit_test.go's identical TestMain/rationale. Tests that specifically
// exercise the new-identity throttle re-enable it via withNewIdentityGrace.
func TestMain(m *testing.M) {
	newIdentityGracePeriod = 0
	os.Exit(m.Run())
}

// withNewIdentityGrace re-enables the new-identity throttle (disabled
// package-wide by TestMain) for a single test's duration.
func withNewIdentityGrace(t *testing.T, d time.Duration) {
	t.Helper()
	old := newIdentityGracePeriod
	newIdentityGracePeriod = d
	t.Cleanup(func() { newIdentityGracePeriod = old })
}

func newTestStore(t *testing.T) (*Store, *stdcrypto.Identity) {
	t.Helper()
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	id.SetName("alice")
	db, err := storage.Open(filepath.Join(t.TempDir(), "forum.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := New(id, nil, db, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, id
}

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func signedMessage(priv ed25519.PrivateKey, pub ed25519.PublicKey, text string, sentAt time.Time) *Message {
	m := &Message{
		AuthorKey: hex.EncodeToString(pub), AuthorName: "eve",
		Text: text, SentAt: sentAt.UTC(),
	}
	m.ID = deriveID(m)
	m.Sig = hex.EncodeToString(ed25519.Sign(priv, payload(m)))
	return m
}

func mustJSON(t *testing.T, m *Message) []byte {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}

// ─── ByAuthor ───────────────────────────────────────────────────────────────

func TestByAuthor_FiltersAndCapsToLimit(t *testing.T) {
	s, _ := newTestStore(t)
	pubA, privA := genKey(t)
	pubB, privB := genKey(t)
	now := time.Now()

	for i := 0; i < 3; i++ {
		s.persist(signedMessage(privA, pubA, fmt.Sprintf("a%d", i), now.Add(time.Duration(i)*time.Second)))
	}
	s.persist(signedMessage(privB, pubB, "b0", now))

	authorA := hex.EncodeToString(pubA)
	got := s.ByAuthor(authorA, 2)
	if len(got) != 2 {
		t.Fatalf("limit=2 iken len(got) = %d, want 2", len(got))
	}
	// En yeni 2'si (a1, a2) dönmeli — a0 sınırın dışında kalmalı.
	if got[0].Text != "a1" || got[1].Text != "a2" {
		t.Fatalf("ByAuthor en yeni mesajları döndürmeliydi, got %+v", got)
	}
	for _, m := range got {
		if m.AuthorKey != authorA {
			t.Errorf("ByAuthor başka bir yazarın mesajını döndürdü: %+v", m)
		}
	}
}

func TestByAuthor_UnknownAuthorReturnsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	if got := s.ByAuthor("hiç-mesajı-olmayan-anahtar", 5); len(got) != 0 {
		t.Fatalf("hiç mesajı olmayan bir yazar için boş dilim beklenir, got %+v", got)
	}
}

// ─── Send ──────────────────────────────────────────────────────────────────

func TestSend_StoresLocallyWithNilTopic(t *testing.T) {
	s, id := newTestStore(t)
	m, err := s.Send(context.Background(), "merhaba")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if m.AuthorKey != hex.EncodeToString(id.PublicKey()) {
		t.Errorf("AuthorKey yanlış")
	}
	if !verify(m) {
		t.Fatal("Send'in ürettiği mesaj kendi imzasıyla doğrulanmalı")
	}
	all := s.All()
	if len(all) != 1 || all[0].Text != "merhaba" {
		t.Fatalf("All() = %+v", all)
	}
}

func TestSend_EmptyTextRejected(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Send(context.Background(), ""); err == nil {
		t.Fatal("boş mesaj reddedilmeliydi")
	}
}

func TestSend_OversizedTextRejected(t *testing.T) {
	s, _ := newTestStore(t)
	big := make([]byte, MaxTextBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if _, err := s.Send(context.Background(), string(big)); err == nil {
		t.Fatal("MaxTextBytes'ı aşan bir mesaj reddedilmeliydi")
	}
}

func TestSend_PersistsAcrossRestart(t *testing.T) {
	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "forum.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sA, err := New(id, nil, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sA.Send(context.Background(), "kalıcı mesaj"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	sB, err := New(id, nil, db, "")
	if err != nil {
		t.Fatal(err)
	}
	all := sB.All()
	if len(all) != 1 || all[0].Text != "kalıcı mesaj" {
		t.Fatalf("mesaj diskten yeniden yüklenmeliydi, got %+v", all)
	}
}

// ─── handleMessage ──────────────────────────────────────────────────────────

func TestHandleMessage_ValidSignedMessageStoredAndForwarded(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "merhaba dünya", time.Now())

	s.handleMessage(mustJSON(t, m))

	all := s.All()
	if len(all) != 1 || all[0].Text != "merhaba dünya" {
		t.Fatalf("geçerli imzalı mesaj kalıcılaştırılmalıydı, got %+v", all)
	}
	select {
	case got := <-s.NewMessages():
		if got.Text != "merhaba dünya" {
			t.Errorf("NewMessages() = %+v", got)
		}
	default:
		t.Fatal("NewMessages() kanalına da iletilmeliydi")
	}
}

func TestHandleMessage_TamperedTextRejected(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "orijinal", time.Now())
	m.Text = "değiştirilmiş"

	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("imza payload'ıyla eşleşmeyen bir mesaj kabul edilmemeliydi, got %+v", all)
	}
}

func TestHandleMessage_WrongAuthorKeyRejected(t *testing.T) {
	s, _ := newTestStore(t)
	_, priv := genKey(t)     // gerçek imzalayan
	otherPub, _ := genKey(t) // iddia edilen ama imzalamayan yazar
	m := signedMessage(priv, otherPub, "sahte", time.Now())

	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("AuthorKey ile imzalayan uyuşmuyorsa kabul edilmemeliydi, got %+v", all)
	}
}

func TestHandleMessage_OversizedTextRejected(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	big := make([]byte, MaxTextBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	m := signedMessage(priv, pub, string(big), time.Now())

	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("boyut sınırını aşan bir mesaj kabul edilmemeliydi, got %+v", all)
	}
}

func TestHandleMessage_DuplicateIDIgnored(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "bir kere", time.Now())

	s.handleMessage(mustJSON(t, m))
	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 1 {
		t.Fatalf("aynı ID'li mesaj tekrar eklenmemeliydi, got %d kayıt", len(all))
	}
}

// TestHandleMessage_ReplayDoesNotConsumeRateLimitOrDupQuota, tam olarak
// aynı (geçerli imzalı) mesajın tekrar tekrar "replay" edilmesinin —
// özel anahtara ihtiyaç duymadan, libp2p'nin kendi "seen" önbelleği
// dolduktan sonra biri yakaladığı eski bir mesajı aynen yeniden
// yayınlayabilir — kurbanın hız sınırı/kopya-mesaj kotasını TÜKETMEDİĞİNİ
// doğrular. ID tekrarı kontrolü artık allow()/isDuplicate()'ten önce
// çalıştığından, replay'ler bu sayaçlara hiç dokunmadan erken reddedilir.
func TestHandleMessage_ReplayDoesNotConsumeRateLimitOrDupQuota(t *testing.T) {
	noRLCooldown(t)
	oldMax := rlMaxPerAuthor
	rlMaxPerAuthor = 2
	t.Cleanup(func() { rlMaxPerAuthor = oldMax })

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "orijinal mesaj", time.Now())
	data := mustJSON(t, m)

	// Aynı mesajı 10 kez "replay" et — rlMaxPerAuthor=2 olsa bile bunların
	// hiçbiri yazarın kotasını tüketmemeli, çünkü hepsi allow()'a
	// ulaşmadan ID-tekrarı kontrolünde reddediliyor.
	for i := 0; i < 10; i++ {
		s.handleMessage(data)
	}
	if all := s.All(); len(all) != 1 {
		t.Fatalf("replay'ler kalıcılaştırılmamalıydı, got %d kayıt", len(all))
	}

	// Yazarın kotası hâlâ dolu olmamalı — gerçek YENİ bir mesaj kabul
	// edilebilmeli.
	fresh := signedMessage(priv, pub, "gerçek ikinci mesaj", time.Now().Add(time.Second))
	s.handleMessage(mustJSON(t, fresh))
	if all := s.All(); len(all) != 2 {
		t.Fatalf("replay'ler kotayı tüketmiş olmalıydı ki bu yeni mesaj reddedildi, got %d kayıt", len(all))
	}
}

// TestHandleMessage_DuplicateRejectionDoesNotTriggerMute, dupMaxRepeats
// sınırını aşan tekrarların REDDEDİLMEYE devam ettiğini ama
// muteViolationThreshold'a ne kadar yaklaşırsa yaklaşsın otomatik
// susturmayı TETİKLEMEDİĞİNİ doğrular — sıradan/masum bir kullanıcının kısa
// bir ifadeyi çok kez tekrarlaması artık tüm trafiğinin susturulmasına yol
// açmıyor.
func TestHandleMessage_DuplicateRejectionDoesNotTriggerMute(t *testing.T) {
	noRLCooldown(t)
	oldThreshold := muteViolationThreshold
	muteViolationThreshold = 3 // çok düşük bir eşik — eskiden olsaydı kesin tetiklenirdi
	t.Cleanup(func() { muteViolationThreshold = oldThreshold })

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)
	now := time.Now()

	// dupMaxRepeats + bolca fazlası kadar aynı metni gönder — hepsi
	// dupMaxRepeats sonrası reddedilecek, ama recordViolation hiç
	// çağrılmamalı.
	for i := 0; i < dupMaxRepeats+10; i++ {
		m := signedMessage(priv, pub, "tekrar eden ifade", now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
	}

	if s.muted(authorKey) {
		t.Fatal("yalnızca kopya-mesaj reddi otomatik susturmayı tetiklememeliydi")
	}
	if all := s.All(); len(all) != dupMaxRepeats {
		t.Fatalf("dupMaxRepeats kadar mesaj kabul edilmeliydi, got %d", len(all))
	}
}

// TestHandleMessage_UnsignedPluginMessageStaysLiveOnly, eklenti API'sinin
// (cmd/and'deki chatAdapter) gönderdiği eski/imzasız {n,t,a} paketinin hâlâ
// canlı gösterim için NewMessages'a iletildiğini ama kalıcılaştırılmadığını
// doğrular — bkz. paket belgesi.
func TestHandleMessage_UnsignedPluginMessageStaysLiveOnly(t *testing.T) {
	s, _ := newTestStore(t)
	pkt := struct {
		N string    `json:"n"`
		T string    `json:"t"`
		A time.Time `json:"a"`
	}{N: "bob", T: "eklentiden mesaj", A: time.Now().UTC()}
	data, err := json.Marshal(pkt)
	if err != nil {
		t.Fatal(err)
	}

	s.handleMessage(data)

	if all := s.All(); len(all) != 0 {
		t.Fatalf("imzasız bir mesaj kalıcılaştırılmamalıydı, got %+v", all)
	}
	select {
	case got := <-s.NewMessages():
		if got.Text != "eklentiden mesaj" || got.AuthorName != "bob" {
			t.Errorf("NewMessages() = %+v", got)
		}
	default:
		t.Fatal("imzasız mesaj yine de canlı gösterim için iletilmeliydi")
	}
}

func TestHandleMessage_EmptyTextRejected(t *testing.T) {
	s, _ := newTestStore(t)
	s.handleMessage([]byte(`{"n":"bob","t":"","a":"2024-01-01T00:00:00Z"}`))
	select {
	case got := <-s.NewMessages():
		t.Fatalf("boş metinli bir paket hiç iletilmemeliydi, got %+v", got)
	default:
	}
}

// ─── TTL temizliği ────────────────────────────────────────────────────────────

func TestCleanup_PurgesExpiredMessages(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	fresh := signedMessage(priv, pub, "taze", now)
	fresh.ExpiresAt = now.Add(time.Hour)
	s.persist(fresh)

	expired := signedMessage(priv, pub, "eski", now.Add(-time.Hour))
	expired.ExpiresAt = now.Add(-time.Minute)
	s.persist(expired)

	if len(s.All()) != 2 {
		t.Fatalf("cleanup öncesi 2 mesaj bekleniyor, got %d", len(s.All()))
	}

	s.cleanup()

	all := s.All()
	if len(all) != 1 || all[0].Text != "taze" {
		t.Fatalf("yalnızca süresi dolmayan mesaj kalmalıydı, got %+v", all)
	}
}

// ─── Signed() ───────────────────────────────────────────────────────────────

func TestMessage_Signed(t *testing.T) {
	m := &Message{}
	if m.Signed() {
		t.Error("boş AuthorKey/Sig ile Signed() true dönmemeli")
	}
	m.AuthorKey, m.Sig = "aa", "bb"
	if !m.Signed() {
		t.Error("AuthorKey ve Sig doluyken Signed() true dönmeli")
	}
}

// ─── Saat kayması ─────────────────────────────────────────────────────────────

func TestHandleMessage_FutureTimestampRejected(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "gelecekten geliyorum", time.Now().Add(time.Hour))

	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("maxClockSkew'i aşan gelecek tarihli bir mesaj kabul edilmemeliydi, got %+v", all)
	}
}

func TestHandleMessage_FutureTimestampDoesNotBlockLaterHonestMessage(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	future := signedMessage(priv, pub, "sahte gelecek", now.Add(24*time.Hour))
	s.handleMessage(mustJSON(t, future))
	if len(s.All()) != 0 {
		t.Fatalf("gelecek tarihli mesaj hiç kabul edilmemeliydi (önkoşul)")
	}

	honest := signedMessage(priv, pub, "gerçek mesaj", now)
	s.handleMessage(mustJSON(t, honest))

	all := s.All()
	if len(all) != 1 || all[0].Text != "gerçek mesaj" {
		t.Fatalf("reddedilen gelecek tarihli mesaj, aynı yazarın dürüst zamanlı sonraki mesajını engellememeliydi, got %+v", all)
	}
}

// ─── Hız sınırı (rate limit) ────────────────────────────────────────────────

func TestHandleMessage_PerAuthorRateLimitRejectsBeyondLimit(t *testing.T) {
	old := rlMaxPerAuthor
	rlMaxPerAuthor = 3
	t.Cleanup(func() { rlMaxPerAuthor = old })
	// Bu test, aynı yazardan gelen art arda çağrıları gerçek bir zaman
	// aralığı olmadan tetikliyor — rlCooldown'ı sıfırlamazsak sayaç
	// sınırından ÖNCE soğuma devreye girer ve bu testin ölçmek istediği
	// şeyi (saatlik sayaç sınırı) test edemez hale gelir.
	oldCooldown := rlCooldown
	rlCooldown = 0
	t.Cleanup(func() { rlCooldown = oldCooldown })

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	var accepted int
	for i := 0; i < rlMaxPerAuthor+2; i++ {
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
		accepted = len(s.All())
	}
	if accepted != rlMaxPerAuthor {
		t.Fatalf("yazar başına saatlik sınırı aşan mesajlar reddedilmeliydi: %d kabul edildi, want %d", accepted, rlMaxPerAuthor)
	}
}

func TestHandleMessage_GlobalRateLimitRejectsBeyondLimit(t *testing.T) {
	oldGlobal := rlGlobalMax
	oldPerAuthor := rlMaxPerAuthor
	rlGlobalMax = 3
	rlMaxPerAuthor = 1000 // bu testte yazar başına sınır değil, küresel tavan test ediliyor
	t.Cleanup(func() {
		rlGlobalMax = oldGlobal
		rlMaxPerAuthor = oldPerAuthor
	})

	s, _ := newTestStore(t)
	now := time.Now()

	var accepted int
	for i := 0; i < rlGlobalMax+2; i++ {
		pub, priv := genKey(t) // her seferinde farklı yazar — Sybil senaryosu
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now)
		s.handleMessage(mustJSON(t, m))
	}
	accepted = len(s.All())
	if accepted != rlGlobalMax {
		t.Fatalf("küresel saatlik tavanı aşan mesajlar (farklı yazarlardan olsa bile) reddedilmeliydi: %d kabul edildi, want %d", accepted, rlGlobalMax)
	}
}

// ─── Ardışık hızlı gönderim soğuması (cooldown) ────────────────────────────

func TestHandleMessage_CooldownRejectsRapidRepeat(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedMessage(priv, pub, "ilk", now)
	s.handleMessage(mustJSON(t, first))
	if len(s.All()) != 1 {
		t.Fatal("ilk mesaj kabul edilmeliydi")
	}

	second := signedMessage(priv, pub, "hemen ardından", now.Add(time.Millisecond))
	s.handleMessage(mustJSON(t, second))
	if all := s.All(); len(all) != 1 {
		t.Fatalf("rlCooldown dolmadan aynı yazardan gelen ikinci mesaj reddedilmeliydi, got %d kayıt", len(all))
	}
}

func TestHandleMessage_AllowedAfterCooldownElapses(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedMessage(priv, pub, "ilk", now)
	s.handleMessage(mustJSON(t, first))

	// Soğuma süresinin çoktan geçtiğini simüle et (gerçek bir sleep yerine
	// son zaman damgasını geriye al).
	s.mu.Lock()
	times := s.bucket(first.AuthorKey).MessageTimes
	times[len(times)-1] = time.Now().Add(-rlCooldown - time.Second)
	s.mu.Unlock()

	second := signedMessage(priv, pub, "sonra", now.Add(time.Second))
	s.handleMessage(mustJSON(t, second))

	if all := s.All(); len(all) != 2 {
		t.Fatalf("soğuma süresi geçtikten sonra ikinci mesaj kabul edilmeliydi, got %d kayıt", len(all))
	}
}

func TestAllow_CooldownSkippedWhenCheckCooldownFalse(t *testing.T) {
	s, _ := newTestStore(t)
	if !s.allow("author1", true) {
		t.Fatal("ilk çağrı izin verilmeli")
	}
	// checkCooldown=false — SyncWithPeer'ın eski/geçmiş mesajları toplu
	// işlerken kullandığı yol; aynı yazarın art arda gelen eski mesajları
	// yalnızca BİZİM işleme hızımız yüzünden reddedilmemeli.
	if !s.allow("author1", false) {
		t.Fatal("checkCooldown=false iken soğuma uygulanmamalıydı")
	}
}

// ─── Otomatik geçici susturma (auto-mute) ────────────────────────────────────

func TestHandleMessage_AutoMutesAfterRepeatedViolations(t *testing.T) {
	oldCooldown := rlCooldown
	oldThreshold := muteViolationThreshold
	rlCooldown = time.Hour // her mesajdan sonra hemen ihlal üretmek için büyük bir soğuma
	muteViolationThreshold = 3
	t.Cleanup(func() {
		rlCooldown = oldCooldown
		muteViolationThreshold = oldThreshold
	})

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)
	now := time.Now()

	// İlk mesaj kabul edilir; sonraki her mesaj rlCooldown yüzünden
	// reddedilip birer ihlal sayılır — muteViolationThreshold'a ulaşınca
	// yazar otomatik susturulmalı.
	for i := 0; i < muteViolationThreshold+2; i++ {
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now.Add(time.Duration(i)*time.Millisecond))
		s.handleMessage(mustJSON(t, m))
	}

	if !s.muted(authorKey) {
		t.Fatal("ısrarla ihlal eden bir yazar otomatik susturulmalıydı")
	}
}

func TestHandleMessage_MutedAuthorMessagesSkippedEvenIfValid(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)

	s.mu.Lock()
	s.bucket(authorKey).MutedUntil = time.Now().Add(muteDuration)
	s.mu.Unlock()

	m := signedMessage(priv, pub, "geçerli ama susturulmuş", time.Now())
	s.handleMessage(mustJSON(t, m))

	if all := s.All(); len(all) != 0 {
		t.Fatalf("susturulmuş bir yazardan gelen (imzası geçerli olsa bile) bir mesaj kabul edilmemeliydi, got %+v", all)
	}
}

func TestMuted_ExpiresAfterMuteDuration(t *testing.T) {
	s, _ := newTestStore(t)
	authorKey := "aabbcc"

	s.mu.Lock()
	s.bucket(authorKey).MutedUntil = time.Now().Add(-time.Second) // çoktan dolmuş
	s.mu.Unlock()

	if s.muted(authorKey) {
		t.Fatal("süresi dolmuş bir susturma artık geçerli olmamalı")
	}
}

func TestStore_RecordViolation_EscalatesMuteDurationOnRepeatOffense(t *testing.T) {
	oldThreshold, oldDuration, oldCap := muteViolationThreshold, muteDuration, muteDurationCap
	muteViolationThreshold = 2
	muteDuration = time.Minute
	muteDurationCap = time.Hour
	t.Cleanup(func() {
		muteViolationThreshold, muteDuration, muteDurationCap = oldThreshold, oldDuration, oldCap
	})

	s, _ := newTestStore(t)
	for i := 0; i < muteViolationThreshold; i++ {
		s.recordViolation("key1")
	}
	s.mu.Lock()
	b := s.bucket("key1")
	if b.MuteCount != 1 {
		t.Fatalf("ilk susturmadan sonra MuteCount = %d, want 1", b.MuteCount)
	}
	firstRemaining := time.Until(b.MutedUntil)
	b.MutedUntil = time.Now().Add(-time.Second) // ilk susturmanın süresi dolmuş gibi
	s.mu.Unlock()

	s.recordViolation("key1")

	s.mu.Lock()
	secondCount := b.MuteCount
	secondRemaining := time.Until(b.MutedUntil)
	s.mu.Unlock()

	if secondCount != 2 {
		t.Fatalf("ikinci susturmadan sonra MuteCount = %d, want 2", secondCount)
	}
	if secondRemaining <= firstRemaining {
		t.Fatalf("ikinci susturma ilkinden daha uzun sürmeliydi: ilk=%v ikinci=%v", firstRemaining, secondRemaining)
	}
}

func TestStore_RecordViolation_MuteDurationCapped(t *testing.T) {
	oldThreshold, oldDuration, oldCap := muteViolationThreshold, muteDuration, muteDurationCap
	muteViolationThreshold = 2
	muteDuration = time.Hour
	muteDurationCap = 90 * time.Minute
	t.Cleanup(func() {
		muteViolationThreshold, muteDuration, muteDurationCap = oldThreshold, oldDuration, oldCap
	})

	s, _ := newTestStore(t)
	s.mu.Lock()
	b := s.bucket("key1")
	b.MuteCount = 5 // tavan olmasaydı 5 saat sürerdi
	s.mu.Unlock()

	for i := 0; i < muteViolationThreshold; i++ {
		s.recordViolation("key1")
	}

	s.mu.Lock()
	remaining := time.Until(b.MutedUntil)
	s.mu.Unlock()
	if remaining > muteDurationCap+time.Minute {
		t.Fatalf("susturma süresi muteDurationCap'i aşmamalıydı, got %v (cap %v)", remaining, muteDurationCap)
	}
}

// ─── Yeni kimlik kısıtlaması (Sybil maliyetini artırma) ─────────────────────

func TestStore_NewIdentity_StricterLimitWithinGracePeriod(t *testing.T) {
	noRLCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	s, _ := newTestStore(t)
	for i := 0; i < newIdentityMaxPerAuthor; i++ {
		if !s.allow("yeni-kimlik", true) {
			t.Fatalf("mesaj %d izin verilmeliydi (yeni kimlik limiti %d)", i+1, newIdentityMaxPerAuthor)
		}
	}
	if s.allow("yeni-kimlik", true) {
		t.Fatal("yeni kimlik limiti aşılınca reddedilmeliydi")
	}
}

func TestStore_NewIdentity_NormalLimitAfterGracePeriodElapses(t *testing.T) {
	noRLCooldown(t)
	withNewIdentityGrace(t, 24*time.Hour)

	s, _ := newTestStore(t)
	s.mu.Lock()
	b := s.bucket("eski-kimlik")
	b.FirstSeen = time.Now().Add(-25 * time.Hour)
	s.mu.Unlock()

	for i := 0; i < rlMaxPerAuthor; i++ {
		if !s.allow("eski-kimlik", true) {
			t.Fatalf("mesaj %d izin verilmeliydi (grace period geçmiş, normal limit %d)", i+1, rlMaxPerAuthor)
		}
	}
}

func TestStore_NewIdentity_SyncBackfillNotThrottled(t *testing.T) {
	// bkz. internal/forum/ratelimit_test.go'daki aynı isimli testin doc
	// yorumu — yeni kimlik kısıtlaması yalnızca canlı alımda uygulanmalı.
	withNewIdentityGrace(t, 24*time.Hour)

	s, _ := newTestStore(t)
	n := newIdentityMaxPerAuthor + 5
	for i := 0; i < n; i++ {
		if !s.allow("senkron-yazar", false) {
			t.Fatalf("mesaj %d, sync backfill'de (checkCooldown=false) yeni-kimlik limitine takılmamalıydı", i+1)
		}
	}
}

// ─── Aynı/benzer mesaj tekrarı tespiti (duplicate detection) ─────────────────

// noRLCooldown zeroes rlCooldown for a test — allow()'s cooldown compares
// real wall-clock arrival time, not a message's claimed SentAt, so two
// handleMessage calls fired back-to-back (as these tests do) would
// otherwise get rejected by cooldown regardless of duplicate-detection,
// making it impossible to test duplicate-detection in isolation.
func noRLCooldown(t *testing.T) {
	t.Helper()
	old := rlCooldown
	rlCooldown = 0
	t.Cleanup(func() { rlCooldown = old })
}

func TestHandleMessage_RepeatsUpToThresholdAccepted(t *testing.T) {
	noRLCooldown(t)
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	for i := 0; i < dupMaxRepeats; i++ {
		m := signedMessage(priv, pub, "merhaba dünya", now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
	}

	if all := s.All(); len(all) != dupMaxRepeats {
		t.Fatalf("dupMaxRepeats (%d) sınırına kadar aynı metnin her tekrarı kabul edilmeliydi, got %d kayıt", dupMaxRepeats, len(all))
	}
}

func TestHandleMessage_RepeatsBeyondThresholdRejected(t *testing.T) {
	noRLCooldown(t)
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	for i := 0; i < dupMaxRepeats+3; i++ {
		m := signedMessage(priv, pub, "merhaba dünya", now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
	}

	if all := s.All(); len(all) != dupMaxRepeats {
		t.Fatalf("dupMaxRepeats (%d) sonrası tekrarlar reddedilmeliydi, got %d kayıt", dupMaxRepeats, len(all))
	}
}

func TestHandleMessage_NearDuplicateCaseAndWhitespaceCountedTogether(t *testing.T) {
	noRLCooldown(t)
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	variants := []string{"Merhaba Dünya", "  merhaba dünya  ", "MERHABA DÜNYA"}
	for i := 0; i < dupMaxRepeats+2; i++ {
		text := variants[i%len(variants)]
		m := signedMessage(priv, pub, text, now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
	}

	if all := s.All(); len(all) != dupMaxRepeats {
		t.Fatalf("büyük/küçük harf ve boşluk dışında aynı olan varyantlar tek bir sayaç altında birikmeliydi, got %d kayıt (want %d)", len(all), dupMaxRepeats)
	}
}

func TestHandleMessage_DifferentTextNotRejectedAsDuplicate(t *testing.T) {
	noRLCooldown(t)
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	first := signedMessage(priv, pub, "merhaba", now)
	s.handleMessage(mustJSON(t, first))

	second := signedMessage(priv, pub, "nasılsın", now.Add(time.Second))
	s.handleMessage(mustJSON(t, second))

	if all := s.All(); len(all) != 2 {
		t.Fatalf("farklı içerikli bir mesaj kopya sayılmamalıydı, got %d kayıt", len(all))
	}
}

func TestHandleMessage_DuplicateOutsideWindowResetsCounter(t *testing.T) {
	noRLCooldown(t)
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	authorKey := hex.EncodeToString(pub)
	now := time.Now()

	// Sayacı eşiğe kadar doldur.
	for i := 0; i < dupMaxRepeats; i++ {
		m := signedMessage(priv, pub, "tekrar edecek", now.Add(time.Duration(i)*time.Second))
		s.handleMessage(mustJSON(t, m))
	}
	if len(s.All()) != dupMaxRepeats {
		t.Fatalf("önkoşul: %d mesaj kabul edilmiş olmalıydı", dupMaxRepeats)
	}

	// dupWindow'un çoktan geçtiğini simüle et (gerçek bir sleep yerine
	// izlenen zaman damgasını doğrudan geriye alıyoruz — makine hızına bağlı
	// kararsız/flaky bir teste yol açmamak için).
	s.mu.Lock()
	entry := s.bucket(authorKey).RecentTexts["tekrar edecek"]
	entry.At = time.Now().Add(-dupWindow - time.Second)
	s.bucket(authorKey).RecentTexts["tekrar edecek"] = entry
	s.mu.Unlock()

	extra := signedMessage(priv, pub, "tekrar edecek", now.Add(time.Duration(dupMaxRepeats+5)*time.Second))
	s.handleMessage(mustJSON(t, extra))

	if all := s.All(); len(all) != dupMaxRepeats+1 {
		t.Fatalf("dupWindow dışına çıkınca sayaç sıfırlanıp yeni bir tekrara izin vermeliydi, got %d kayıt (want %d)", len(all), dupMaxRepeats+1)
	}
}

// ─── persist() dönüş değeri / HistoryChanged ─────────────────────────────────

// TestPersist_ReturnsWhetherNewlyAdded, SyncWithPeer'ın HistoryChanged'i ne
// zaman sinyalleyeceğine karar vermek için kullandığı persist()'in dönüş
// değerinin doğru olduğunu doğrular (bkz. sync.go'daki "applied" mantığı).
func TestPersist_ReturnsWhetherNewlyAdded(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "mesaj", time.Now())

	if !s.persist(m) {
		t.Fatal("yeni bir mesaj persist() ile ilk kez eklendiğinde true dönmeli")
	}
	if s.persist(m) {
		t.Fatal("aynı mesajı ikinci kez persist() etmek false dönmeli (zaten var)")
	}
}

func TestHistoryChanged_SignaledOnlyOnNewPersist(t *testing.T) {
	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	m := signedMessage(priv, pub, "geçmiş mesajı", time.Now())

	select {
	case <-s.HistoryChanged():
		t.Fatal("hiçbir şey persist edilmeden HistoryChanged sinyali gelmemeliydi")
	default:
	}

	// Normal bir persist() (tahliyeye yol açmayan) HistoryChanged'i
	// SİNYALLEMEZ — canlı akış (NewMessages) zaten sıralı ekleme için
	// yeterli; HistoryChanged yalnızca sync.go'nun geçmiş doldurması ya da
	// MaxStoredMessages tahliyesi gibi "aradan/baştan mesaj çıkarma"
	// durumları için var (bkz. TestPersist_EvictionSignalsHistoryChanged).
	s.persist(m)
	select {
	case <-s.HistoryChanged():
		t.Fatal("tahliyeye yol açmayan sıradan bir persist() HistoryChanged sinyali göndermemeliydi")
	default:
	}
}

// ─── MaxStoredMessages tahliyesi ─────────────────────────────────────────────

func TestPersist_EvictsOldestBeyondMaxStoredMessages(t *testing.T) {
	old := MaxStoredMessages
	MaxStoredMessages = 3
	t.Cleanup(func() { MaxStoredMessages = old })

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()

	var ids []string
	for i := 0; i < 5; i++ {
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now.Add(time.Duration(i)*time.Second))
		s.persist(m)
		ids = append(ids, m.ID)
	}

	all := s.All()
	if len(all) != MaxStoredMessages {
		t.Fatalf("len(All()) = %d, want %d (tavanı aşan en eskiler tahliye edilmeliydi)", len(all), MaxStoredMessages)
	}
	// En yeni 3 mesaj (2,3,4) kalmalı; en eski 2'si (0,1) tahliye edilmeliydi.
	if all[0].Text != "mesaj 2" || all[len(all)-1].Text != "mesaj 4" {
		t.Fatalf("tahliye en ESKİ mesajları kaldırmalıydı, kalanlar: %+v", all)
	}
}

func TestPersist_EvictionRemovesFromDB(t *testing.T) {
	old := MaxStoredMessages
	MaxStoredMessages = 2
	t.Cleanup(func() { MaxStoredMessages = old })

	id, err := stdcrypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "forum.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sA, err := New(id, nil, db, "")
	if err != nil {
		t.Fatal(err)
	}
	pub, priv := genKey(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now.Add(time.Duration(i)*time.Second))
		sA.persist(m)
	}

	// Yeniden başlatma simülasyonu: tahliye DB'den de silmemiş olsaydı,
	// yeniden yüklemede tahliye edilmiş mesaj geri gelirdi.
	sB, err := New(id, nil, db, "")
	if err != nil {
		t.Fatal(err)
	}
	all := sB.All()
	if len(all) != MaxStoredMessages {
		t.Fatalf("tahliye DB'ye de yansımalıydı, yeniden yüklemede %d mesaj var, want %d", len(all), MaxStoredMessages)
	}
}

func TestPersist_EvictionSignalsHistoryChanged(t *testing.T) {
	old := MaxStoredMessages
	MaxStoredMessages = 2
	t.Cleanup(func() { MaxStoredMessages = old })

	s, _ := newTestStore(t)
	pub, priv := genKey(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		m := signedMessage(priv, pub, fmt.Sprintf("mesaj %d", i), now.Add(time.Duration(i)*time.Second))
		s.persist(m)
	}

	select {
	case <-s.HistoryChanged():
	default:
		t.Fatal("tahliyeye yol açan bir persist() HistoryChanged'i sinyallemeliydi")
	}
}
