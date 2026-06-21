// Command andmod is the AND moderation CLI.
//
// Kullanım:
//
//	andmod pubkey                              -- AND kimliğinin public key'ini gösterir
//	andmod grant <hedef_pubkey_hex> [--days N] -- Moderatör sertifikası oluşturur (kurucu olarak)
//	andmod ban <peer_id> <sebep> --cert <dosya> [--days N] -- Ban mesajı oluşturur (moderatör olarak)
//
// grant ve ban çıktı dosyaları bans/ klasörüne kaydedilir.
// AND başlarken bu klasördeki tüm .json dosyalarını ağda yayınlar.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"and/internal/crypto"
	"and/internal/moderation"

	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "andmod:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "pubkey":
		return cmdPubkey()
	case "grant":
		return cmdGrant(args[1:])
	case "ban":
		return cmdBan(args[1:])
	default:
		printUsage()
		return fmt.Errorf("bilinmeyen komut: %s", args[0])
	}
}

func printUsage() {
	fmt.Println(`AND Moderasyon Aracı

Komutlar:
  andmod pubkey
      AND kimliğinin public key'ini gösterir.
      Bu değeri binary'ye gömmek için kullanın:
        go build -ldflags "-X and/internal/moderation.FounderPubKeyHex=<hex>" ./cmd/and

  andmod grant <hedef_pubkey_hex> [--days 7] [--permanent]
      Belirtilen public key'e moderatörlük sertifikası verir.
      --permanent ile süresiz sertifika oluşturulur.
      Çıktı: bans/mod_<ilk8karakter>.json

  andmod ban <peer_id> <sebep> --cert <cert.json> [--days 30]
      Moderatör sertifikası ile bir peer'ı banlar.
      Çıktı: bans/ban_<peer_id_kısa>.json
      AND başlarken bans/ klasöründeki tüm .json dosyalarını otomatik yayınlar.`)
}

// ── pubkey ────────────────────────────────────────────────────────────────────

func cmdPubkey() error {
	id, err := unlockIdentity()
	if err != nil {
		return err
	}
	pubHex := hex.EncodeToString(id.PublicKey())
	fmt.Println("Public Key (hex):")
	fmt.Println(pubHex)
	fmt.Println()
	fmt.Println("Binary'ye gömmek için:")
	fmt.Printf("  go build -ldflags \"-X and/internal/moderation.FounderPubKeyHex=%s\" ./cmd/and\n", pubHex)
	return nil
}

// ── grant ─────────────────────────────────────────────────────────────────────

func cmdGrant(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("kullanım: andmod grant <hedef_pubkey_hex> [--days 7] [--permanent]")
	}
	targetHex := args[0]
	days := 7
	permanent := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--permanent":
			permanent = true
		case "--days":
			if i+1 >= len(args) {
				return fmt.Errorf("--days değeri eksik")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 || n > 7 {
				return fmt.Errorf("--days 1-7 arasında olmalı")
			}
			days = n
			i++
		}
	}

	targetKey, err := hex.DecodeString(targetHex)
	if err != nil || len(targetKey) != ed25519.PublicKeySize {
		return fmt.Errorf("geçersiz public key hex (32 byte / 64 hex karakter olmalı)")
	}

	id, err := unlockIdentity()
	if err != nil {
		return err
	}

	var cert moderation.ModeratorCert
	var sureStr string

	if permanent {
		payload := fmt.Sprintf("%s|permanent", targetHex)
		sig := ed25519.Sign(id.PrivateKey(), []byte(payload))
		cert = moderation.ModeratorCert{
			ModeratorKey: targetHex,
			Permanent:    true,
			Sig:          hex.EncodeToString(sig),
		}
		sureStr = "süresiz"
	} else {
		expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		payload := fmt.Sprintf("%s|%d", targetHex, expires.Unix())
		sig := ed25519.Sign(id.PrivateKey(), []byte(payload))
		cert = moderation.ModeratorCert{
			ModeratorKey: targetHex,
			ExpiresAt:    expires,
			Sig:          hex.EncodeToString(sig),
		}
		sureStr = fmt.Sprintf("%d gün (%s'e kadar)", days, expires.Format("2006-01-02 15:04"))
	}

	outDir := bansDir()
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("bans/ dizini oluşturulamadı: %w", err)
	}

	shortKey := targetHex
	if len(shortKey) > 8 {
		shortKey = shortKey[:8]
	}
	outPath := filepath.Join(outDir, "mod_"+shortKey+".json")

	data, _ := json.MarshalIndent(cert, "", "  ")
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return fmt.Errorf("dosya yazılamadı: %w", err)
	}

	fmt.Printf("Moderatör sertifikası oluşturuldu: %s\n", outPath)
	fmt.Printf("Süre: %s\n", sureStr)
	fmt.Println()
	fmt.Println("Bu dosyayı moderatöre Özel Chat ile gönderin.")
	fmt.Println("Moderatör bunu --cert parametresiyle kullanacak:")
	fmt.Printf("  andmod ban <peer_id> <sebep> --cert %s\n", outPath)
	return nil
}

// ── ban ───────────────────────────────────────────────────────────────────────

func cmdBan(args []string) error {
	var peerID, reason, certPath string
	days := 30

	if len(args) < 2 {
		return fmt.Errorf("kullanım: andmod ban <peer_id> <sebep> --cert <dosya> [--days 30]")
	}
	peerID = args[0]
	reason = args[1]

	for i := 2; i < len(args)-1; i++ {
		switch args[i] {
		case "--cert":
			certPath = args[i+1]
		case "--days":
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 || n > 30 {
				return fmt.Errorf("--days 1-30 arasında olmalı")
			}
			days = n
		}
	}
	if certPath == "" {
		return fmt.Errorf("--cert <dosya> gerekli")
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("peer_id ve sebep boş olamaz")
	}

	// Cert yükle ve doğrula.
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cert dosyası okunamadı: %w", err)
	}
	var cert moderation.ModeratorCert
	if err := json.Unmarshal(certData, &cert); err != nil {
		return fmt.Errorf("cert JSON geçersiz: %w", err)
	}
	if !cert.Permanent && time.Now().After(cert.ExpiresAt) {
		return fmt.Errorf("moderatör sertifikası süresi dolmuş: %s", cert.ExpiresAt.Format("2006-01-02 15:04"))
	}

	// Kimlik: moderatör kendi AND kimliğini kullanır.
	id, err := unlockIdentity()
	if err != nil {
		return err
	}

	// Cert'teki public key ile AND kimliği eşleşmeli.
	myPubHex := hex.EncodeToString(id.PublicKey())
	if !strings.EqualFold(cert.ModeratorKey, myPubHex) {
		return fmt.Errorf("kimlik uyuşmazlığı: bu cert size ait değil\n  cert için: %s\n  sizin key: %s",
			cert.ModeratorKey[:16]+"...", myPubHex[:16]+"...")
	}

	expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	payload := fmt.Sprintf("%s|%s|%d", peerID, reason, expires.Unix())
	sig := ed25519.Sign(id.PrivateKey(), []byte(payload))

	ban := moderation.BanMsg{
		BannedPeer: peerID,
		Reason:     reason,
		ExpiresAt:  expires,
		Cert:       cert,
		Sig:        hex.EncodeToString(sig),
	}

	outDir := bansDir()
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("bans/ dizini oluşturulamadı: %w", err)
	}

	shortPeer := peerID
	if len(shortPeer) > 12 {
		shortPeer = shortPeer[len(shortPeer)-12:]
	}
	outPath := filepath.Join(outDir, "ban_"+shortPeer+".json")

	data, _ := json.MarshalIndent(ban, "", "  ")
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		return fmt.Errorf("dosya yazılamadı: %w", err)
	}

	fmt.Printf("Ban mesajı oluşturuldu: %s\n", outPath)
	fmt.Printf("Hedef  : %s\n", peerID)
	fmt.Printf("Sebep  : %s\n", reason)
	fmt.Printf("Süre   : %d gün (%s'e kadar)\n", days, expires.Format("2006-01-02 15:04"))
	fmt.Println()
	fmt.Println("AND'i başlatın — bans/ klasörü otomatik taranır ve ban ağda yayınlanır.")
	return nil
}

// ── Yardımcılar ───────────────────────────────────────────────────────────────

func unlockIdentity() (*crypto.Identity, error) {
	identityFile := defaultIdentityFile()
	if _, err := os.Stat(identityFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("kimlik dosyası bulunamadı: %s\nÖnce AND'i çalıştırıp kimlik oluşturun", identityFile)
	}

	fmt.Print("AND şifreniz: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("şifre okunamadı: %w", err)
	}
	return crypto.LoadEncrypted(identityFile, string(passBytes))
}

func defaultIdentityFile() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "identity.dat"
	}
	return filepath.Join(base, "and", "identity.dat")
}

func bansDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "bans"
	}
	return filepath.Join(base, "and", "bans")
}
