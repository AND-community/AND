package main

// forum_test.go — özellikle imza doğrulamasının doğruluğuna odaklanır:
// postPayload/replyPayload/deleteMsgPayload burada AND'ın kaynak kodundan
// (internal/forum/forum.go) elle transkript edildi (bu ayrı modül onu
// import edemediğinden) — bu testler en azından KENDİ İÇİNDE tutarlı
// olduğunu (imzala→doğrula döngüsü çalışır, kurcalama tespit edilir)
// kanıtlıyor.

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func signedPost(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey) *Post {
	t.Helper()
	p := &Post{
		ID: "p1", Category: "genel", AuthorKey: hex.EncodeToString(pub),
		AuthorName: "ali", Title: "başlık", Body: "gövde", CreatedAt: time.Now().UTC(),
	}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv, postPayload(p)))
	return p
}

func signedReply(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey) *Reply {
	t.Helper()
	r := &Reply{
		ID: "r1", PostID: "p1", AuthorKey: hex.EncodeToString(pub),
		AuthorName: "veli", Body: "yanıt gövdesi", CreatedAt: time.Now().UTC(),
	}
	r.Sig = hex.EncodeToString(ed25519.Sign(priv, replyPayload(r)))
	return r
}

func TestVerifyPost_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	if !verifyPost(p) {
		t.Fatal("geçerli imza reddedildi")
	}
}

func TestVerifyPost_KurcalanmisGovdeReddedilir(t *testing.T) {
	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	p.Body = "değiştirilmiş gövde"
	if verifyPost(p) {
		t.Fatal("kurcalanmış gövde kabul edildi")
	}
}

func TestVerifyPost_BaskaAnahtarlaImzaReddedilir(t *testing.T) {
	pub1, _ := genKey(t)
	_, priv2 := genKey(t)
	p := &Post{ID: "p1", Category: "genel", AuthorKey: hex.EncodeToString(pub1), Title: "x", Body: "y", CreatedAt: time.Now().UTC()}
	p.Sig = hex.EncodeToString(ed25519.Sign(priv2, postPayload(p))) // yanlış private key
	if verifyPost(p) {
		t.Fatal("AuthorKey ile eşleşmeyen imza kabul edildi")
	}
}

func TestVerifyReply_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	r := signedReply(t, priv, pub)
	if !verifyReply(r) {
		t.Fatal("geçerli yanıt imzası reddedildi")
	}
}

func TestVerifyDelete_GecerliImzaKabulEdilir(t *testing.T) {
	pub, priv := genKey(t)
	d := &deleteMsg{PostID: "p1", AuthorKey: hex.EncodeToString(pub), DeletedAt: time.Now().UTC()}
	d.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(d)))
	if !verifyDelete(d) {
		t.Fatal("geçerli silme imzası reddedildi")
	}
}

func TestForumMesajIsle_GecerliPostIndekslenir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	data, _ := json.Marshal(forumZarf{Type: "post", Post: p})

	forumMesajIsle(d, data)

	sonuclar, err := d.Ara("başlık", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].KaynakID != "p1" {
		t.Fatalf("geçerli post indekslenmedi: %+v", sonuclar)
	}
}

func TestForumMesajIsle_GecersizImzaliPostReddedilir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	p.Body = "saldırgan tarafından değiştirildi" // imzadan SONRA kurcala
	data, _ := json.Marshal(forumZarf{Type: "post", Post: p})

	forumMesajIsle(d, data)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("geçersiz imzalı post indekslenmemeliydi, %d kayıt var", n)
	}
}

func TestForumMesajIsle_GecerliYanitIndekslenir(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	r := signedReply(t, priv, pub)
	data, _ := json.Marshal(forumZarf{Type: "reply", Reply: r})

	forumMesajIsle(d, data)

	sonuclar, err := d.Ara("yanıt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sonuclar) != 1 || sonuclar[0].UstID != "p1" {
		t.Fatalf("geçerli yanıt indekslenmedi: %+v", sonuclar)
	}
}

func TestForumMesajIsle_SilmeTombstoneUygular(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	pub, priv := genKey(t)
	p := signedPost(t, priv, pub)
	data, _ := json.Marshal(forumZarf{Type: "post", Post: p})
	forumMesajIsle(d, data)
	if n, _ := d.SayimAl(); n != 1 {
		t.Fatalf("kurulum: post indekslenmedi, sayı=%d", n)
	}

	del := &deleteMsg{PostID: "p1", AuthorKey: hex.EncodeToString(pub), DeletedAt: time.Now().UTC()}
	del.Sig = hex.EncodeToString(ed25519.Sign(priv, deleteMsgPayload(del)))
	delData, _ := json.Marshal(forumZarf{Type: "delete", Delete: del})
	forumMesajIsle(d, delData)

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("silme sonrası post indekste kalmamalıydı, sayı=%d", n)
	}
}

func TestForumMesajIsle_BozukJSONPanikAtmaz(t *testing.T) {
	d, err := depoAc(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Kapat() //nolint:errcheck

	forumMesajIsle(d, []byte("bu geçerli bir json değil"))
	forumMesajIsle(d, []byte(`{"type":"bilinmeyen_tip"}`))
	forumMesajIsle(d, []byte(`{"type":"post"}`)) // post alanı nil

	if n, _ := d.SayimAl(); n != 0 {
		t.Fatalf("geçersiz girdilerden hiçbiri indekslenmemeliydi, sayı=%d", n)
	}
}
