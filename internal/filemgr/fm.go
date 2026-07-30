package filemgr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	libp2pnet "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/lucian95511/and/internal/network"
	"github.com/lucian95511/and/internal/pluginapi"
)

// Protocol /and/file/2.0.0 — chunked transfer with resume.
//
// Message flow:
//
//	Sender                   Receiver
//	  INIT  ──────────────►
//	        ◄──────────────  RESUME (nextChunk=0 or N)
//	  CHUNK ──────────────►
//	        ◄──────────────  ACK
//	  CHUNK ──────────────►  (repeats; receiver writes each chunk to disk)
//	        ◄──────────────  ACK
//	  DONE  ──────────────►  (receiver renames .part → final file)
//
// On reconnect, receiver loads its state file and sends RESUME with the
// next missing chunk; sender seeks to that offset and continues.
//
// Peak memory = one chunk buffer (512 KB) regardless of file size.

const (
	chunkBytes   = 512 * 1024      // 512 KB per chunk — low memory, fine-grained resume
	chunkTimeout = 60 * time.Second // per-operation deadline; extended before each read/write

	msgINIT   uint8 = 0x01
	msgRESUME uint8 = 0x02
	msgCHUNK  uint8 = 0x03
	msgACK    uint8 = 0x04
	msgDONE   uint8 = 0x05
)

type Broker struct {
	node     *network.Node
	saveDir  string
	rateKBPS uint64 // 0 = unlimited; set via AND_FILE_RATE_KBPS
	mu          sync.Mutex
	subs        []chan pluginapi.FileMsg
	consentSubs []chan pluginapi.FileConsentReq
	consentPend map[string]chan bool // transferID → response channel
}

// recvState persists receive progress across reconnects.
type recvState struct {
	TransferID  string `json:"transfer_id"`
	Filename    string `json:"filename"`
	TotalSize   int64  `json:"total_size"`
	ChunkSize   uint32 `json:"chunk_size"`
	TotalChunks uint32 `json:"total_chunks"`
	NextChunk   uint32 `json:"next_chunk"`
	PartPath    string `json:"part_path"`
	SenderName  string `json:"sender_name"`
	ReceivedAt  int64  `json:"received_at_nano"`
}

func New(node *network.Node, saveDir string) *Broker {
	var rate uint64
	if s := os.Getenv("AND_FILE_RATE_KBPS"); s != "" {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 {
			rate = v
		}
	}
	b := &Broker{node: node, saveDir: saveDir, rateKBPS: rate, consentPend: make(map[string]chan bool)}
	node.Host.SetStreamHandler(pluginapi.FileProtocol, b.handleStream)
	return b
}

func (b *Broker) Subscribe() chan pluginapi.FileMsg {
	ch := make(chan pluginapi.FileMsg, 16)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(ch chan pluginapi.FileMsg) {
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

func (b *Broker) deliver(msg pluginapi.FileMsg) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Broker) SubscribeConsent() chan pluginapi.FileConsentReq {
	ch := make(chan pluginapi.FileConsentReq, 4)
	b.mu.Lock()
	b.consentSubs = append(b.consentSubs, ch)
	b.mu.Unlock()
	return ch
}

func (b *Broker) UnsubscribeConsent(ch chan pluginapi.FileConsentReq) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.consentSubs {
		if c == ch {
			b.consentSubs = append(b.consentSubs[:i], b.consentSubs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *Broker) RespondConsent(transferID string, accept bool) {
	b.mu.Lock()
	ch, ok := b.consentPend[transferID]
	if ok {
		delete(b.consentPend, transferID)
	}
	b.mu.Unlock()
	if ok {
		select {
		case ch <- accept:
		default:
		}
	}
}

func (b *Broker) deliverConsent(req pluginapi.FileConsentReq) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.consentSubs {
		select {
		case ch <- req:
		default:
		}
	}
}

// SendFile streams localPath to peerIDStr in 512 KB chunks.
// Each chunk is confirmed with an ACK before the next is sent — this
// prevents overwhelming slow receivers and limits memory to one chunk.
// If the transfer was interrupted, the receiver reports how many chunks
// it already has and the sender resumes from that offset automatically.
func (b *Broker) SendFile(ctx context.Context, peerIDStr, senderName, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("dosya açılamadı: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("dosya bilgisi alınamadı: %w", err)
	}
	size := info.Size()
	if size == 0 {
		return fmt.Errorf("dosya boş")
	}
	if size > int64(pluginapi.MaxFileBytes) {
		return fmt.Errorf("dosya çok büyük: %.1f GB (limit %.0f GB)",
			float64(size)/float64(1<<30), float64(pluginapi.MaxFileBytes)/float64(1<<30))
	}

	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("geçersiz peer ID: %w", err)
	}

	filename := filepath.Base(localPath)
	tid := makeTransferID(senderName, filename, size)
	totalChunks := uint32((size + int64(chunkBytes) - 1) / int64(chunkBytes))

	var stream libp2pnet.Stream
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(4 * time.Second):
			}
		}
		stream, err = b.node.Host.NewStream(ctx, pid, pluginapi.FileProtocol)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("peer'a bağlanılamadı: %w", err)
	}
	defer stream.Close()

	// ── INIT ──────────────────────────────────────────────────────────────
	touch(stream)
	if err := writeINIT(stream, tid, filename, size, chunkBytes, totalChunks, time.Now().UTC()); err != nil {
		return fmt.Errorf("INIT yazılamadı: %w", err)
	}

	// ── RESUME ────────────────────────────────────────────────────────────
	touch(stream)
	nextChunk, err := readRESUME(stream)
	if err != nil {
		return fmt.Errorf("RESUME okunamadı: %w", err)
	}
	if nextChunk >= totalChunks {
		return nil // karşı tarafda dosya zaten tam
	}

	if nextChunk > 0 {
		if _, err := f.Seek(int64(nextChunk)*int64(chunkBytes), io.SeekStart); err != nil {
			return fmt.Errorf("dosya konumu ayarlanamadı: %w", err)
		}
	}

	// ── CHUNKs ────────────────────────────────────────────────────────────
	buf := make([]byte, chunkBytes)
	for i := nextChunk; i < totalChunks; i++ {
		chunkStart := time.Now()

		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("dosya okunamadı (parça %d): %w", i, err)
		}

		touch(stream)
		if err := writeCHUNK(stream, i, buf[:n]); err != nil {
			return fmt.Errorf("CHUNK yazılamadı (parça %d): %w", i, err)
		}

		touch(stream)
		acked, err := readACK(stream)
		if err != nil {
			return fmt.Errorf("ACK okunamadı (parça %d): %w", i, err)
		}
		if acked != i {
			return fmt.Errorf("yanlış ACK: beklenen %d, alınan %d", i, acked)
		}

		b.throttle(stream, n, chunkStart)
	}

	// ── DONE ──────────────────────────────────────────────────────────────
	touch(stream)
	return writeDONE(stream)
}

func (b *Broker) handleStream(s libp2pnet.Stream) {
	defer s.Close()

	// ── INIT ──────────────────────────────────────────────────────────────
	touch(s)
	tid, filename, totalSize, cs, totalChunks, at, err := readINIT(s)
	if err != nil {
		return
	}
	name := filepath.Base(filename)
	if name == "" || name == "." {
		return
	}
	senderName := s.Conn().RemotePeer().String()

	if err := os.MkdirAll(b.saveDir, 0o700); err != nil {
		return
	}

	// Reject the transfer if the disk cannot fit the file plus a 512 MB safety buffer.
	// freeBytesAt failure (e.g. unsupported FS) is ignored — we proceed optimistically.
	const diskBuffer = 512 * 1024 * 1024
	if free, err := freeBytesAt(b.saveDir); err == nil {
		if free < uint64(totalSize)+diskBuffer {
			return
		}
	}

	// ── Kullanıcı onayı ──────────────────────────────────────────────────────
	// Eklenti subscribe ediyorsa onay isteği gönder ve yanıt bekle (30 sn).
	b.mu.Lock()
	hasSubs := len(b.consentSubs) > 0
	b.mu.Unlock()
	if hasSubs {
		ch := make(chan bool, 1)
		b.mu.Lock()
		b.consentPend[tid] = ch
		b.mu.Unlock()

		b.deliverConsent(pluginapi.FileConsentReq{
			TransferID: tid,
			SenderID:   senderName,
			Filename:   name,
			Size:       totalSize,
		})

		accepted := false
		select {
		case accepted = <-ch:
		case <-time.After(30 * time.Second):
		}
		if !accepted {
			b.mu.Lock()
			delete(b.consentPend, tid)
			b.mu.Unlock()
			return
		}
	}

	state, err := b.loadOrCreateState(tid, name, senderName, totalSize, cs, totalChunks, at)
	if err != nil {
		return
	}

	// ── RESUME ────────────────────────────────────────────────────────────
	touch(s)
	if err := writeRESUME(s, state.NextChunk); err != nil {
		return
	}

	// ── Receive chunks ────────────────────────────────────────────────────
	partFile, err := os.OpenFile(state.PartPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}

	finalPath, complete := b.receiveChunks(s, state, partFile)
	partFile.Close()

	if !complete {
		return // bağlantı kesildi; state + .part dosyası kaldı, sonraki bağlantıda devam eder
	}

	// ── Finalize ──────────────────────────────────────────────────────────
	if err := os.Rename(state.PartPath, finalPath); err != nil {
		return
	}
	b.deleteState(state.TransferID)
	b.deliver(pluginapi.FileMsg{
		From:       state.SenderName,
		Filename:   name,
		Size:       totalSize,
		SavePath:   finalPath,
		ReceivedAt: time.Unix(0, state.ReceivedAt).UTC(),
	})
}

// receiveChunks handles the CHUNK/ACK loop until DONE.
// Returns (finalPath, true) on success, ("", false) on interrupted transfer.
func (b *Broker) receiveChunks(s libp2pnet.Stream, state *recvState, f *os.File) (string, bool) {
	cs := state.ChunkSize
	for {
		touch(s)
		mtype, err := readByte(s)
		if err != nil {
			return "", false
		}

		switch mtype {
		case msgDONE:
			return b.uniquePath(state.Filename), true

		case msgCHUNK:
			idx, data, err := readCHUNKBody(s)
			if err != nil || idx != state.NextChunk {
				return "", false
			}

			offset := int64(idx) * int64(cs)
			if _, err := f.WriteAt(data, offset); err != nil {
				return "", false
			}

			state.NextChunk = idx + 1
			b.saveState(state)

			touch(s)
			if err := writeACK(s, idx); err != nil {
				return "", false
			}

		default:
			return "", false
		}
	}
}

// ── State management ──────────────────────────────────────────────────────────

func (b *Broker) statePath(tid string) string {
	return filepath.Join(b.saveDir, tid+".state")
}

func (b *Broker) loadOrCreateState(tid, filename, senderName string, totalSize int64, cs, totalChunks uint32, at time.Time) (*recvState, error) {
	path := b.statePath(tid)
	if raw, err := os.ReadFile(path); err == nil {
		var st recvState
		if json.Unmarshal(raw, &st) == nil && st.NextChunk > 0 {
			// Validate: .part dosyası en az yazılmış kadar büyük olmalı
			if info, err := os.Stat(st.PartPath); err == nil {
				expected := int64(st.NextChunk) * int64(st.ChunkSize)
				if info.Size() >= expected {
					return &st, nil // geçerli devam noktası
				}
			}
			// Tutarsız durum — sıfırdan başla
			_ = os.Remove(st.PartPath)
		}
	}

	st := &recvState{
		TransferID:  tid,
		Filename:    filename,
		TotalSize:   totalSize,
		ChunkSize:   cs,
		TotalChunks: totalChunks,
		NextChunk:   0,
		PartPath:    filepath.Join(b.saveDir, tid+".part"),
		SenderName:  senderName,
		ReceivedAt:  at.UnixNano(),
	}
	b.saveState(st)
	return st, nil
}

func (b *Broker) saveState(st *recvState) {
	raw, _ := json.Marshal(st)
	_ = os.WriteFile(b.statePath(st.TransferID), raw, 0o600)
}

func (b *Broker) deleteState(tid string) {
	_ = os.Remove(b.statePath(tid))
}

func (b *Broker) uniquePath(filename string) string {
	dest := filepath.Join(b.saveDir, filename)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(b.saveDir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(b.saveDir, fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext))
}

// ── Protocol helpers ──────────────────────────────────────────────────────────

// touch extends the per-operation deadline; called before each send or receive
// so a slow connection never triggers a false timeout between chunks.
func touch(s libp2pnet.Stream) {
	s.SetDeadline(time.Now().Add(chunkTimeout)) //nolint:errcheck
}

// throttle sleeps after a chunk so the transfer stays within rateKBPS.
// Sleep is capped at 55 s so the receiver's 60 s deadline is never exceeded;
// touch is called after the sleep to extend the deadline for the next chunk.
func (b *Broker) throttle(s libp2pnet.Stream, n int, chunkStart time.Time) {
	if b.rateKBPS == 0 {
		return
	}
	expected := time.Duration(float64(n) / float64(b.rateKBPS*1024) * float64(time.Second))
	if sleep := expected - time.Since(chunkStart); sleep > 0 {
		if sleep > 55*time.Second {
			sleep = 55 * time.Second
		}
		time.Sleep(sleep)
		touch(s)
	}
}

// makeTransferID produces a deterministic 32-char hex ID from sender+filename+size.
// The same file sent by the same sender always gets the same ID, which lets
// the receiver automatically resume without out-of-band coordination.
func makeTransferID(senderName, filename string, size int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", senderName, filename, size)))
	return hex.EncodeToString(h[:16])
}

func readByte(r io.Reader) (uint8, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

// Wire layout — INIT:
// [1 msgINIT] [16 tidBytes] [2 nameLen] [N name] [8 totalSize] [4 chunkSize] [4 totalChunks] [8 unixNano]
func writeINIT(w io.Writer, tid, filename string, totalSize int64, cs, totalChunks uint32, at time.Time) error {
	tidB, _ := hex.DecodeString(tid) // always 16 bytes
	nameB := []byte(filename)
	if len(nameB) > 512 {
		return fmt.Errorf("dosya adı çok uzun (max 512 bayt)")
	}
	var buf bytes.Buffer
	buf.WriteByte(msgINIT)
	buf.Write(tidB)
	binary.Write(&buf, binary.BigEndian, uint16(len(nameB))) //nolint:errcheck
	buf.Write(nameB)
	binary.Write(&buf, binary.BigEndian, totalSize)          //nolint:errcheck
	binary.Write(&buf, binary.BigEndian, cs)                 //nolint:errcheck
	binary.Write(&buf, binary.BigEndian, totalChunks)        //nolint:errcheck
	binary.Write(&buf, binary.BigEndian, at.UnixNano())      //nolint:errcheck
	_, err := w.Write(buf.Bytes())
	return err
}

func readINIT(r io.Reader) (tid, filename string, totalSize int64, cs, totalChunks uint32, at time.Time, err error) {
	var mtype [1]byte
	if _, err = io.ReadFull(r, mtype[:]); err != nil {
		return
	}
	if mtype[0] != msgINIT {
		err = fmt.Errorf("INIT beklendi, tip=%d alındı", mtype[0])
		return
	}

	tidB := make([]byte, 16)
	if _, err = io.ReadFull(r, tidB); err != nil {
		return
	}
	tid = hex.EncodeToString(tidB)

	var nameLen uint16
	if err = binary.Read(r, binary.BigEndian, &nameLen); err != nil {
		return
	}
	if nameLen > 512 {
		err = fmt.Errorf("dosya adı çok uzun")
		return
	}
	nameB := make([]byte, nameLen)
	if _, err = io.ReadFull(r, nameB); err != nil {
		return
	}
	filename = string(nameB)

	if err = binary.Read(r, binary.BigEndian, &totalSize); err != nil {
		return
	}
	if totalSize <= 0 || totalSize > int64(pluginapi.MaxFileBytes) {
		err = fmt.Errorf("geçersiz dosya boyutu: %d", totalSize)
		return
	}
	if err = binary.Read(r, binary.BigEndian, &cs); err != nil {
		return
	}
	if err = binary.Read(r, binary.BigEndian, &totalChunks); err != nil {
		return
	}
	var tsNano int64
	if err = binary.Read(r, binary.BigEndian, &tsNano); err != nil {
		return
	}
	at = time.Unix(0, tsNano).UTC()
	return
}

// Wire layout — RESUME: [1 msgRESUME] [4 nextChunk]
func writeRESUME(w io.Writer, nextChunk uint32) error {
	var buf bytes.Buffer
	buf.WriteByte(msgRESUME)
	binary.Write(&buf, binary.BigEndian, nextChunk) //nolint:errcheck
	_, err := w.Write(buf.Bytes())
	return err
}

func readRESUME(r io.Reader) (uint32, error) {
	mtype, err := readByte(r)
	if err != nil {
		return 0, err
	}
	if mtype != msgRESUME {
		return 0, fmt.Errorf("RESUME beklendi, tip=%d alındı", mtype)
	}
	var next uint32
	if err := binary.Read(r, binary.BigEndian, &next); err != nil {
		return 0, err
	}
	return next, nil
}

// Wire layout — CHUNK: [1 msgCHUNK] [4 idx] [4 dataLen] [N data]
func writeCHUNK(w io.Writer, idx uint32, data []byte) error {
	var buf bytes.Buffer
	buf.WriteByte(msgCHUNK)
	binary.Write(&buf, binary.BigEndian, idx)               //nolint:errcheck
	binary.Write(&buf, binary.BigEndian, uint32(len(data))) //nolint:errcheck
	buf.Write(data)
	_, err := w.Write(buf.Bytes())
	return err
}

func readCHUNKBody(r io.Reader) (idx uint32, data []byte, err error) {
	if err = binary.Read(r, binary.BigEndian, &idx); err != nil {
		return
	}
	var dataLen uint32
	if err = binary.Read(r, binary.BigEndian, &dataLen); err != nil {
		return
	}
	if dataLen > uint32(chunkBytes)*2 {
		err = fmt.Errorf("parça çok büyük: %d bayt", dataLen)
		return
	}
	data = make([]byte, dataLen)
	_, err = io.ReadFull(r, data)
	return
}

// Wire layout — ACK: [1 msgACK] [4 idx]
func writeACK(w io.Writer, idx uint32) error {
	var buf bytes.Buffer
	buf.WriteByte(msgACK)
	binary.Write(&buf, binary.BigEndian, idx) //nolint:errcheck
	_, err := w.Write(buf.Bytes())
	return err
}

func readACK(r io.Reader) (uint32, error) {
	mtype, err := readByte(r)
	if err != nil {
		return 0, err
	}
	if mtype != msgACK {
		return 0, fmt.Errorf("ACK beklendi, tip=%d alındı", mtype)
	}
	var idx uint32
	if err := binary.Read(r, binary.BigEndian, &idx); err != nil {
		return 0, err
	}
	return idx, nil
}

// Wire layout — DONE: [1 msgDONE]
func writeDONE(w io.Writer) error {
	_, err := w.Write([]byte{msgDONE})
	return err
}
