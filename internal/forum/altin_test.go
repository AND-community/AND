package forum

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lucian95511/and/protokol/altin"
)

// TestAltinOrnekler, forum iletilerinin ağdaki biçimini protokol/testdata'daki
// örneklere sabitler; arama motoru aynı örnekleri kendi koduyla doğrular.
func TestAltinOrnekler(t *testing.T) {
	p := &Post{
		Category:   "Go",
		AuthorName: "altın",
		AuthorKey:  altin.AnahtarHex(),
		Title:      "Protokol örneği",
		Body:       "Bu konu, AND ile arama motorunun aynı protokolü konuştuğunu sınar: <ç & ğ>.",
		CreatedAt:  altin.Zaman,
	}
	p.ID = derivePostID(p)
	p.Sig = altin.Imzala(postPayload(p))
	konu, _ := json.Marshal(envelope{Type: "post", Post: p})
	altin.Karsilastir(t, "forum_konu.json", konu)

	r := &Reply{
		PostID:     p.ID,
		AuthorName: "altın",
		AuthorKey:  altin.AnahtarHex(),
		Body:       "Yanıt da aynı biçimde imzalanır.",
		CreatedAt:  altin.Zaman.Add(time.Minute),
	}
	r.ID = deriveReplyID(r)
	r.Sig = altin.Imzala(replyPayload(r))
	yanit, _ := json.Marshal(envelope{Type: "reply", Reply: r})
	altin.Karsilastir(t, "forum_yanit.json", yanit)

	d := &DeleteMsg{PostID: p.ID, AuthorKey: altin.AnahtarHex(), DeletedAt: altin.Zaman.Add(time.Hour)}
	d.Sig = altin.Imzala(deleteMsgPayload(d))
	silme, _ := json.Marshal(envelope{Type: "delete", Delete: d})
	altin.Karsilastir(t, "forum_silme.json", silme)

	// AND, kayıtlı örnekleri ağdan gelmiş gibi kabul eder.
	var e envelope
	if err := json.Unmarshal(altin.Oku(t, "forum_konu.json"), &e); err != nil || e.Post == nil || !verifyPost(e.Post) || derivePostID(e.Post) != e.Post.ID {
		t.Fatalf("kayıtlı konu örneği doğrulanmalı: %v", err)
	}
	e = envelope{}
	if err := json.Unmarshal(altin.Oku(t, "forum_yanit.json"), &e); err != nil || e.Reply == nil || !verifyReply(e.Reply) {
		t.Fatalf("kayıtlı yanıt örneği doğrulanmalı: %v", err)
	}
	e = envelope{}
	if err := json.Unmarshal(altin.Oku(t, "forum_silme.json"), &e); err != nil || e.Delete == nil || !verifyDeleteMsg(e.Delete) {
		t.Fatalf("kayıtlı silme örneği doğrulanmalı: %v", err)
	}
}
