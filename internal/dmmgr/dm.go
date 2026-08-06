package dmmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/internal/moderation"
	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
)

const maxDMReadBytes = 16 * 1024

type dmPacket struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

type Broker struct {
	node *network.Node
	mu   sync.Mutex
	subs []chan pluginapi.DMMsg
	mod  atomic.Pointer[moderation.Moderator]
}

func New(node *network.Node) *Broker {
	b := &Broker{node: node}
	node.Host.SetStreamHandler(pluginapi.DMProtocol, b.handleStream)
	return b
}

// SetModerator bağlar (veya nil ile çözer) bir *moderation.Moderator'ı bu
// Broker'a — bağlıysa handleStream, banlı bir peer'dan gelen DM'leri
// bağlantı gater'ından (InterceptSecured) bağımsız olarak da reddeder:
// gater yalnızca YENİ bağlantıları engeller, ban'dan ÖNCE zaten kurulmuş
// bir bağlantı ban sonrasında da açık kalabilir. Ayarlanmazsa (nil kalırsa)
// davranış tamamen eskisiyle aynıdır.
func (b *Broker) SetModerator(mod *moderation.Moderator) {
	b.mod.Store(mod)
}

func (b *Broker) authorBanned(pid peer.ID) bool {
	mod := b.mod.Load()
	return mod != nil && mod.IsBanned(pid)
}

func (b *Broker) Subscribe() chan pluginapi.DMMsg {
	ch := make(chan pluginapi.DMMsg, 32)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(ch chan pluginapi.DMMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.subs {
		if c == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *Broker) Deliver(msg pluginapi.DMMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Broker) SendDM(ctx context.Context, peerIDStr, senderName, message string) error {
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("geçersiz peer ID: %w", err)
	}
	stream, err := b.node.Host.NewStream(ctx, pid, pluginapi.DMProtocol)
	if err != nil {
		return fmt.Errorf("peer'a bağlanılamadı: %w", err)
	}
	defer stream.Close()
	stream.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck

	pkt := dmPacket{From: senderName, Text: message, At: time.Now().UTC()}
	return json.NewEncoder(stream).Encode(pkt)
}

func (b *Broker) handleStream(s libp2pnet.Stream) {
	defer s.Close()

	if b.authorBanned(s.Conn().RemotePeer()) {
		return
	}

	s.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck

	var pkt dmPacket
	if err := json.NewDecoder(io.LimitReader(s, maxDMReadBytes)).Decode(&pkt); err != nil {
		return
	}
	if pkt.Text == "" {
		return
	}
	from := combineIdentity(pkt.From, s.Conn().RemotePeer().String())
	b.Deliver(pluginapi.DMMsg{
		From:       from,
		Text:       pkt.Text,
		ReceivedAt: pkt.At,
	})
}

// combineIdentity, DM göndericisinin öz-bildirdiği adını (pkt.From —
// doğrulanmamış, gönderenin kendi beyanı) her zaman libp2p tarafından
// doğrulanmış gerçek Peer ID'siyle birlikte döndürür. Yalnızca öz-bildirilen
// ada güvenmek, bir eşin başka bir kullanıcının görünen adını taklit
// etmesine izin verir; Peer ID'yi daima yanında tutmak, alıcının taklidi
// fark edebilmesini sağlar. claimedName boşsa ya da zaten verifiedPeerID'nin
// kendisiyse yalnızca doğrulanmış ID döner.
func combineIdentity(claimedName, verifiedPeerID string) string {
	claimedName = strings.TrimSpace(claimedName)
	if claimedName == "" || claimedName == verifiedPeerID {
		return verifiedPeerID
	}
	short := verifiedPeerID
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("%s (%s…)", claimedName, short)
}
