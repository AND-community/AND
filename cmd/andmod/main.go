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

	"github.com/lucian95511/and/internal/crypto"
	"github.com/lucian95511/and/internal/dosya"
	"github.com/lucian95511/and/internal/moderation"

	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "andmod:", err)
		os.Exit(1)
	}
}

// shortKey truncates a hex key for display in error messages. Unlike a bare
// k[:16], it's safe for untrusted input (e.g. a hand-edited or corrupt
// --cert file's ModeratorKey) shorter than 16 characters.
func shortKey(k string) string {
	if len(k) > 16 {
		return k[:16] + "..."
	}
	return k
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
	case "revoke":
		return cmdRevoke(args[1:])
	case "unban":
		return cmdUnban(args[1:])
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
        go build -ldflags "-X github.com/lucian95511/and/internal/moderation.FounderPubKeyHex=<hex>" ./cmd/and

  andmod grant <hedef_pubkey_hex> [--days 7] [--permanent]
      Belirtilen public key'e moderatörlük sertifikası verir.
      --permanent ile süresiz sertifika oluşturulur.
      Çıktı: bans/mod_<ilk8karakter>.json

  andmod ban <peer_id> <sebep> --cert <cert.json> [--duration 1h] [--permanent] [--days 30]
  andmod ban <peer_id> <sebep> --as-founder [--duration 1h] [--permanent] [--days 30]
      Bir peer'ı banlar — --cert ile moderatör sertifikasıyla, --as-founder
      ile doğrudan kurucu kimliğiyle (sertifika gerektirmez; yalnızca
      binary'ye gömülü FounderPubKeyHex'e karşılık gelen kimlikle
      çalıştırıldığında ağda etkilidir). Süre esnek:
        --duration 1h      1 saatlik chat/forum yasağı (30m, 2h30m, 24h de olur)
        --permanent        ömür boyu yasak (süre gerektirmez)
        --days 30          eski kullanım, gün cinsinden (varsayılan: 30 gün)
      --duration en az 1 dakika, en fazla 720h (30 gün) olabilir — daha
      uzunu için --permanent kullanın. --permanent verilirse --duration/
      --days yok sayılır.
      Çıktı: bans/ban_<peer_id_kısa>.json

  andmod revoke <moderatör_pubkey_hex>
      Bir moderatörün sertifikasını iptal eder (yalnızca KURUCU kimliğiyle
      çalıştırıldığında ağda etkilidir — iptal mesajı kurucunun public
      key'ine karşı doğrulanır). Süresiz (--permanent) sertifikaları geri
      almanın tek yolu budur.
      Çıktı: bans/revoke_<pubkey_kısa>.json

  andmod unban <peer_id> --cert <cert.json>
  andmod unban <peer_id> --as-founder
      Daha önce verilmiş bir banı geri alır (hatalı/haksız bir ban için).
      --cert ile herhangi bir geçerli moderatör sertifikasıyla çalışır —
      banı kimin verdiği önemli değil. --as-founder ile doğrudan kurucu
      kimliğiyle (sertifika gerektirmez).
      Çıktı: bans/unban_<peer_id_kısa>.json

  AND başlarken bans/ klasöründeki ban_*.json ve revoke_*.json dosyalarını
  otomatik olarak ağa yayınlar.`)
}

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
	fmt.Printf("  go build -ldflags \"-X github.com/lucian95511/and/internal/moderation.FounderPubKeyHex=%s\" ./cmd/and\n", pubHex)
	return nil
}

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
			if err != nil || n < 1 || n > moderation.MaxCertDays {
				return fmt.Errorf("--days 1-%d arasında olmalı (verifyCert bunun ötesindeki süreleri reddeder)", moderation.MaxCertDays)
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
		cert = moderation.SignModeratorCert(id.PrivateKey(), targetHex, time.Time{}, true)
		sureStr = "süresiz"
	} else {
		expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		cert = moderation.SignModeratorCert(id.PrivateKey(), targetHex, expires, false)
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
	if err := dosya.AtomikYaz(outPath, data, 0o600); err != nil {
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

func cmdBan(args []string) error {
	var peerID, reason, certPath, durationStr string
	permanent := false
	asFounder := false
	days := 0 // 0 = --days verilmedi

	if len(args) < 2 {
		return fmt.Errorf("kullanım: andmod ban <peer_id> <sebep> --cert <dosya> [--duration 1h] [--permanent] [--days 30]\n   veya: andmod ban <peer_id> <sebep> --as-founder [--duration 1h] [--permanent] [--days 30]")
	}
	peerID = args[0]
	reason = args[1]

	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--cert":
			i++
			if i >= len(args) {
				return fmt.Errorf("--cert değeri eksik")
			}
			certPath = args[i]
		case "--as-founder":
			asFounder = true
		case "--duration":
			i++
			if i >= len(args) {
				return fmt.Errorf("--duration değeri eksik")
			}
			durationStr = args[i]
		case "--days":
			i++
			if i >= len(args) {
				return fmt.Errorf("--days değeri eksik")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 || n > 30 {
				return fmt.Errorf("--days 1-30 arasında olmalı")
			}
			days = n
		case "--permanent":
			permanent = true
		}
	}
	if certPath == "" && !asFounder {
		return fmt.Errorf("--cert <dosya> ya da --as-founder gerekli")
	}
	if certPath != "" && asFounder {
		return fmt.Errorf("--cert ve --as-founder birlikte kullanılamaz")
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("peer_id ve sebep boş olamaz")
	}
	if permanent && (durationStr != "" || days != 0) {
		return fmt.Errorf("--permanent, --duration veya --days ile birlikte kullanılamaz")
	}

	var cert moderation.ModeratorCert
	if !asFounder {
		certData, err := os.ReadFile(certPath)
		if err != nil {
			return fmt.Errorf("cert dosyası okunamadı: %w", err)
		}
		if err := json.Unmarshal(certData, &cert); err != nil {
			return fmt.Errorf("cert JSON geçersiz: %w", err)
		}
		if !cert.Permanent && time.Now().After(cert.ExpiresAt) {
			return fmt.Errorf("moderatör sertifikası süresi dolmuş: %s", cert.ExpiresAt.Format("2006-01-02 15:04"))
		}
	}

	id, err := unlockIdentity()
	if err != nil {
		return err
	}

	myPubHex := hex.EncodeToString(id.PublicKey())
	if !asFounder && !strings.EqualFold(cert.ModeratorKey, myPubHex) {
		return fmt.Errorf("kimlik uyuşmazlığı: bu cert size ait değil\n  cert için: %s\n  sizin key: %s",
			shortKey(cert.ModeratorKey), shortKey(myPubHex))
	}
	if asFounder && !strings.EqualFold(myPubHex, moderation.FounderPubKeyHex) {
		return fmt.Errorf("--as-founder yalnızca kurucu kimliğiyle çalıştırıldığında ağda etkilidir\n  sizin key : %s\n  kurucu key: %s\n(binary'ye gömülü kurucu anahtarı FounderPubKeyHex ile eşleşmiyor)",
			shortKey(myPubHex), shortKey(moderation.FounderPubKeyHex))
	}

	var expires time.Time
	var sureStr string
	switch {
	case permanent:
		sureStr = "ömür boyu (kalıcı)"
	case durationStr != "":
		d, err := time.ParseDuration(durationStr)
		if err != nil {
			return fmt.Errorf("geçersiz --duration: %w (örnek: 1h, 30m, 2h30m, 24h)", err)
		}
		if d < time.Minute {
			return fmt.Errorf("--duration en az 1 dakika (1m) olmalı")
		}
		if d > 30*24*time.Hour {
			return fmt.Errorf("--duration en fazla 720h (30 gün) olabilir — daha uzunu için --permanent kullanın")
		}
		expires = time.Now().UTC().Add(d)
		sureStr = fmt.Sprintf("%s (%s'e kadar)", durationStr, expires.Format("2006-01-02 15:04"))
	default:
		d := 30
		if days > 0 {
			d = days
		}
		expires = time.Now().UTC().Add(time.Duration(d) * 24 * time.Hour)
		sureStr = fmt.Sprintf("%d gün (%s'e kadar)", d, expires.Format("2006-01-02 15:04"))
	}

	var ban moderation.BanMsg
	if asFounder {
		ban = moderation.SignFounderBan(id.PrivateKey(), peerID, reason, expires, permanent)
	} else {
		ban = moderation.SignModeratorBan(id.PrivateKey(), cert, peerID, reason, expires, permanent)
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
	if err := dosya.AtomikYaz(outPath, data, 0o600); err != nil {
		return fmt.Errorf("dosya yazılamadı: %w", err)
	}

	fmt.Printf("Ban mesajı oluşturuldu: %s\n", outPath)
	fmt.Printf("Hedef  : %s\n", peerID)
	fmt.Printf("Sebep  : %s\n", reason)
	fmt.Printf("Süre   : %s\n", sureStr)
	fmt.Println()
	fmt.Println("AND'i başlatın — bans/ klasörü otomatik taranır ve ban ağda yayınlanır.")
	return nil
}

func cmdUnban(args []string) error {
	var peerID, certPath string
	asFounder := false

	if len(args) < 1 {
		return fmt.Errorf("kullanım: andmod unban <peer_id> --cert <dosya>\n   veya: andmod unban <peer_id> --as-founder")
	}
	peerID = args[0]

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--cert":
			i++
			if i >= len(args) {
				return fmt.Errorf("--cert değeri eksik")
			}
			certPath = args[i]
		case "--as-founder":
			asFounder = true
		}
	}
	if certPath == "" && !asFounder {
		return fmt.Errorf("--cert <dosya> ya da --as-founder gerekli")
	}
	if certPath != "" && asFounder {
		return fmt.Errorf("--cert ve --as-founder birlikte kullanılamaz")
	}
	if strings.TrimSpace(peerID) == "" {
		return fmt.Errorf("peer_id boş olamaz")
	}

	var cert moderation.ModeratorCert
	if !asFounder {
		certData, err := os.ReadFile(certPath)
		if err != nil {
			return fmt.Errorf("cert dosyası okunamadı: %w", err)
		}
		if err := json.Unmarshal(certData, &cert); err != nil {
			return fmt.Errorf("cert JSON geçersiz: %w", err)
		}
		if !cert.Permanent && time.Now().After(cert.ExpiresAt) {
			return fmt.Errorf("moderatör sertifikası süresi dolmuş: %s", cert.ExpiresAt.Format("2006-01-02 15:04"))
		}
	}

	id, err := unlockIdentity()
	if err != nil {
		return err
	}

	myPubHex := hex.EncodeToString(id.PublicKey())
	if !asFounder && !strings.EqualFold(cert.ModeratorKey, myPubHex) {
		return fmt.Errorf("kimlik uyuşmazlığı: bu cert size ait değil\n  cert için: %s\n  sizin key: %s",
			shortKey(cert.ModeratorKey), shortKey(myPubHex))
	}
	if asFounder && !strings.EqualFold(myPubHex, moderation.FounderPubKeyHex) {
		return fmt.Errorf("--as-founder yalnızca kurucu kimliğiyle çalıştırıldığında ağda etkilidir\n  sizin key : %s\n  kurucu key: %s",
			shortKey(myPubHex), shortKey(moderation.FounderPubKeyHex))
	}

	var u moderation.UnbanMsg
	if asFounder {
		u = moderation.SignFounderUnban(id.PrivateKey(), peerID)
	} else {
		u = moderation.SignModeratorUnban(id.PrivateKey(), cert, peerID)
	}

	outDir := bansDir()
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("bans/ dizini oluşturulamadı: %w", err)
	}

	shortPeer := peerID
	if len(shortPeer) > 12 {
		shortPeer = shortPeer[len(shortPeer)-12:]
	}
	outPath := filepath.Join(outDir, "unban_"+shortPeer+".json")

	data, _ := json.MarshalIndent(u, "", "  ")
	if err := dosya.AtomikYaz(outPath, data, 0o600); err != nil {
		return fmt.Errorf("dosya yazılamadı: %w", err)
	}

	fmt.Printf("İptal (unban) mesajı oluşturuldu: %s\n", outPath)
	fmt.Printf("Hedef  : %s\n", peerID)
	fmt.Println()
	fmt.Println("AND'i başlatın — bans/ klasörü otomatik taranır ve unban ağda yayınlanır.")
	return nil
}

func cmdRevoke(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("kullanım: andmod revoke <moderatör_pubkey_hex>")
	}
	targetHex := args[0]
	targetKey, err := hex.DecodeString(targetHex)
	if err != nil || len(targetKey) != ed25519.PublicKeySize {
		return fmt.Errorf("geçersiz public key hex (32 byte / 64 hex karakter olmalı)")
	}

	id, err := unlockIdentity()
	if err != nil {
		return err
	}

	rev := moderation.SignRevoke(id.PrivateKey(), targetHex)

	outDir := bansDir()
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("bans/ dizini oluşturulamadı: %w", err)
	}

	shortKey := targetHex
	if len(shortKey) > 8 {
		shortKey = shortKey[:8]
	}
	outPath := filepath.Join(outDir, "revoke_"+shortKey+".json")

	data, _ := json.MarshalIndent(rev, "", "  ")
	if err := dosya.AtomikYaz(outPath, data, 0o600); err != nil {
		return fmt.Errorf("dosya yazılamadı: %w", err)
	}

	fmt.Printf("İptal (revoke) mesajı oluşturuldu: %s\n", outPath)
	fmt.Println()
	fmt.Println("AND'i başlatın — bans/ klasörü otomatik taranır ve iptal ağda yayınlanır.")
	fmt.Println("Not: bu yalnızca KURUCU kimliğiyle çalıştırıldığında ağda etkilidir;")
	fmt.Println("başka bir kimlikle üretilen iptal mesajı hiçbir düğüm tarafından kabul edilmez.")
	return nil
}

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
