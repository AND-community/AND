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
	"github.com/lucian95511/and/protokol"
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

	// history, son özel mesajlardır (gelen ve giden; sınırlar için bkz.
	// sinirla). Deliver yalnızca o anda dinleyen eklentilere iletir; geçmiş
	// olmadan Özel Chat ya da DeltaX kapalıyken gelen her DM sessizce
	// kaybolurdu. KaliciAc çağrıldıysa şifreli olarak diske de yazılır (bkz.
	// kalici.go).
	history []pluginapi.DMRecord

	// Kalıcılık, giden kutusu ve okundu bilgisi (bkz. kalici.go).
	kalici       string
	anahtar      []byte
	kayitMu      sync.Mutex
	giden        []bekleyenDM
	okundu       map[string]time.Time
	gonderiliyor map[peer.ID]bool
}

// historyLimit, geçmişte tutulan en fazla insanlar arası mesaj sayısıdır.
const historyLimit = 500

// makineGecmisLimiti, geçmişte tutulan en fazla makine mesajı (bkz.
// protokol.MakineIletisiMi) sayısıdır.
const makineGecmisLimiti = 300

// maxHistoryNameRunes, geçmişe yazılan (gönderenin beyan ettiği) adın en
// fazla uzunluğudur.
const maxHistoryNameRunes = 64

// record, m'yi geçmişe ekler (kimliği yoksa verir); sınır aşılırsa en eskisi
// düşer. Kalıcılık açıksa durum diske yazılır.
func (b *Broker) record(m pluginapi.DMRecord) {
	if m.ID == "" {
		m.ID = yeniID()
	}
	b.mu.Lock()
	b.history = sinirla(append(b.history, m))
	b.mu.Unlock()
	b.kaydet()
}

// sinirla, geçmişi türüne göre sınırlar: insanlar arası mesajlardan en fazla
// historyLimit, makine mesajlarından en fazla makineGecmisLimiti kalır;
// fazlası en eskiden başlayarak düşer. Ayrı sınırlar, DeltaX'in sık üye
// listesi yayınlarının sohbet mesajlarını geçmişten itmesini (ve çok
// sohbetin, DeltaX kapalıyken gelen grup mesajlarını silmesini) önler. h
// yerinde değiştirilir.
func sinirla(h []pluginapi.DMRecord) []pluginapi.DMRecord {
	insan, makine := 0, 0
	for _, r := range h {
		if protokol.MakineIletisiMi(r.Text) {
			makine++
		} else {
			insan++
		}
	}
	fazlaInsan, fazlaMakine := insan-historyLimit, makine-makineGecmisLimiti
	if fazlaInsan <= 0 && fazlaMakine <= 0 {
		return h
	}
	kalan := h[:0]
	for _, r := range h {
		if protokol.MakineIletisiMi(r.Text) {
			if fazlaMakine > 0 {
				fazlaMakine--
				continue
			}
		} else if fazlaInsan > 0 {
			fazlaInsan--
			continue
		}
		kalan = append(kalan, r)
	}
	return kalan
}

// History, bu oturumdaki özel mesajların bir kopyasını (en eskiden en
// yeniye) döner — pluginapi.DMHistoryBackend.
func (b *Broker) History() []pluginapi.DMRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]pluginapi.DMRecord(nil), b.history...)
}

func New(node *network.Node) *Broker {
	b := &Broker{node: node}
	node.Host.SetStreamHandler(pluginapi.DMProtocol, b.handleStream)
	// Bir eş bağlandığında giden kutusunda ona bekleyen mesajlar gönderilir.
	// ConnectedF ağ katmanının goroutine'inde çağrılır; baglandi bloklamaz.
	node.Host.Network().Notify(&libp2pnet.NotifyBundle{
		ConnectedF: func(_ libp2pnet.Network, c libp2pnet.Conn) { b.baglandi(c.RemotePeer()) },
	})
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
	at := time.Now().UTC()
	if err := b.gonder(ctx, pid, senderName, message, at); err != nil {
		return err
	}
	if message != "" { // alıcı boş metni zaten yok sayar
		b.record(pluginapi.DMRecord{PeerID: pid.String(), Text: message, At: at, Outgoing: true})
	}
	return nil
}

// gonder, tek bir DM paketini pid'e iletir.
func (b *Broker) gonder(ctx context.Context, pid peer.ID, senderName, message string, at time.Time) error {
	stream, err := b.node.Host.NewStream(ctx, pid, pluginapi.DMProtocol)
	if err != nil {
		return fmt.Errorf("peer'a bağlanılamadı: %w", err)
	}
	defer stream.Close()
	stream.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck
	return json.NewEncoder(stream).Encode(dmPacket{From: senderName, Text: message, At: at})
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
	verifiedPeerID := s.Conn().RemotePeer().String()
	from := combineIdentity(pkt.From, verifiedPeerID)
	name := strings.TrimSpace(pkt.From)
	if r := []rune(name); len(r) > maxHistoryNameRunes {
		name = string(r[:maxHistoryNameRunes])
	}
	// Zaman, gönderenin beyan ettiği değil bu düğümün saatidir — geçmişin
	// sırasını bir eş bozamasın.
	b.record(pluginapi.DMRecord{PeerID: verifiedPeerID, Name: name, Text: pkt.Text, At: time.Now().UTC()})
	b.Deliver(pluginapi.DMMsg{
		From:           from,
		VerifiedPeerID: verifiedPeerID,
		Text:           pkt.Text,
		ReceivedAt:     pkt.At,
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
