package main

// chat.go — AND'ın sohbet protokolünün bağımsız bir yeniden uygulaması
// (bkz. forum.go'nun doc yorumu — aynı yaklaşım). Mesajlar chatTopicAdi'ne
// doğrudan (bir zarfa sarmalanmadan) ham Message JSON'ı olarak yayınlanır
// — bkz. AND/internal/chatlog/chatlog.go.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	libp2phost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const chatSyncProtokolu = "/and/chatsync/1.0.0"
const chatSyncZamanAsimi = 30 * time.Second
const maxChatSyncYanitBayt = 16 * 1024 * 1024

// json etiketleri (n/t/a) AND'ın kendi Message'ıyla bire bir aynı olmalı —
// bkz. AND/internal/chatlog/chatlog.go'nun Message struct'ı.
type ChatMessage struct {
	ID         string    `json:"id,omitempty"`
	AuthorKey  string    `json:"author_key,omitempty"`
	AuthorName string    `json:"n"`
	Text       string    `json:"t"`
	SentAt     time.Time `json:"a"`
	Sig        string    `json:"sig,omitempty"`
}

func (m *ChatMessage) imzali() bool { return m.AuthorKey != "" && m.Sig != "" }

type chatSignablePayload struct {
	ID        string    `json:"id"`
	AuthorKey string    `json:"author_key"`
	Text      string    `json:"text"`
	SentAt    time.Time `json:"sent_at"`
}

func chatPayload(m *ChatMessage) []byte {
	data, _ := json.Marshal(chatSignablePayload{ID: m.ID, AuthorKey: m.AuthorKey, Text: m.Text, SentAt: m.SentAt})
	return data
}

func verifyChat(m *ChatMessage) bool {
	pub, err := hex.DecodeString(m.AuthorKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(m.Sig)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), chatPayload(m), sig)
}

// chatMesajIsle, gossip'ten gelen bir sohbet mesajını doğrulayıp depoya
// uygular. İmzasız mesajlar (ör. bir eklenti API'sinden gelen, kimliği
// kanıtlanamayan mesajlar) kasıtlı olarak indekslenmez — AND'ın kendisi de
// bunları kalıcılaştırmaz, yalnızca canlı gösterir (bkz. chatlog.go'nun
// doc yorumu).
func chatMesajIsle(depo *Depo, data []byte) {
	var m ChatMessage
	if err := json.Unmarshal(data, &m); err != nil || m.Text == "" || !m.imzali() {
		return
	}
	if !verifyChat(&m) {
		gunlukUyari("geçersiz imzalı sohbet mesajı reddedildi:", m.ID)
		return
	}
	id := m.ID
	if id == "" {
		return
	}
	if err := depo.Ekle(AramaSonucu{
		Tip: TipSohbet, KaynakID: id, Govde: m.Text, YazarAd: m.AuthorName, YazarKey: m.AuthorKey,
		OlusturmaTS: m.SentAt,
		Sig: m.Sig,
	}); err != nil {
		gunlukHata("sohbet mesajı indekslenemedi:", id, err)
	}
}

type chatSyncIstek struct {
	MessageIDs []string `json:"message_ids"`
}

type chatSyncYanit struct {
	Messages []*ChatMessage `json:"messages"`
}

func chatIleSenkronizeOl(ctx context.Context, h libp2phost.Host, peerID peer.ID, depo *Depo) error {
	ctx, cancel := context.WithTimeout(ctx, chatSyncZamanAsimi)
	defer cancel()

	st, err := h.NewStream(ctx, peerID, chatSyncProtokolu)
	if err != nil {
		return fmt.Errorf("chat sync: stream açılamadı: %w", err)
	}
	defer st.Close()
	st.SetDeadline(time.Now().Add(chatSyncZamanAsimi)) //nolint:errcheck

	bilinenIDler, err := depo.BilinenIDler(TipSohbet)
	if err != nil {
		return err
	}

	if err := json.NewEncoder(st).Encode(chatSyncIstek{MessageIDs: bilinenIDler}); err != nil {
		return fmt.Errorf("chat sync: istek gönderilemedi: %w", err)
	}
	st.CloseWrite() //nolint:errcheck

	var resp chatSyncYanit
	if err := json.NewDecoder(io.LimitReader(st, maxChatSyncYanitBayt)).Decode(&resp); err != nil {
		return fmt.Errorf("chat sync: yanıt okunamadı: %w", err)
	}

	for _, m := range resp.Messages {
		if m == nil || m.ID == "" || !m.imzali() || !verifyChat(m) {
			continue
		}
		_ = depo.Ekle(AramaSonucu{
			Tip: TipSohbet, KaynakID: m.ID, Govde: m.Text, YazarAd: m.AuthorName, YazarKey: m.AuthorKey,
			OlusturmaTS: m.SentAt,
			Sig: m.Sig,
		})
	}
	return nil
}
