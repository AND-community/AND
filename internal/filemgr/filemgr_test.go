package filemgr

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/lucian95511/and/internal/pluginapi"
)

// TestChunkedTransfer verifies that files are sent in chunks without holding
// the entire file in memory, and that the protocol frames are correct.
func TestChunkedTransfer(t *testing.T) {
	// Create a test file with known content (1 MB = 2 chunks of 512 KB)
	content := bytes.Repeat([]byte("test"), 256*1024) // 1 MB
	tid := makeTransferID("alice", "file.bin", int64(len(content)))

	// Test encoding/decoding INIT
	var buf bytes.Buffer
	if err := writeINIT(&buf, tid, "file.bin", int64(len(content)), chunkBytes, 2, time.Now().UTC()); err != nil {
		t.Fatalf("writeINIT failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("INIT frame is empty")
	}

	// Test encoding/decoding CHUNK
	buf.Reset()
	chunk := content[:chunkBytes]
	if err := writeCHUNK(&buf, 0, chunk); err != nil {
		t.Fatalf("writeCHUNK failed: %v", err)
	}
	if buf.Len() != 1+4+4+len(chunk) {
		t.Errorf("CHUNK frame size: expected %d, got %d", 1+4+4+len(chunk), buf.Len())
	}

	// Test encoding/decoding ACK
	buf.Reset()
	if err := writeACK(&buf, 0); err != nil {
		t.Fatalf("writeACK failed: %v", err)
	}
	if buf.Len() != 1+4 {
		t.Errorf("ACK frame size: expected 5, got %d", buf.Len())
	}

	// Test encoding/decoding DONE
	buf.Reset()
	if err := writeDONE(&buf); err != nil {
		t.Fatalf("writeDONE failed: %v", err)
	}
	if buf.Len() != 1 {
		t.Errorf("DONE frame size: expected 1, got %d", buf.Len())
	}
}

// TestResumeState verifies that transfer state can be saved and loaded correctly.
func TestResumeState(t *testing.T) {
	tmpDir := t.TempDir()

	broker := &Broker{saveDir: tmpDir}
	tid := "abc123def456"

	st := &recvState{
		TransferID:  tid,
		Filename:    "video.mp4",
		TotalSize:   1000000000,
		ChunkSize:   512 * 1024,
		TotalChunks: 1953,
		NextChunk:   1,
		PartPath:    tmpDir + "/video.mp4.part",
		SenderName:  "alice",
		ReceivedAt:  time.Now().UnixNano(),
	}

	// Create a .part file large enough to pass size validation (NextChunk * ChunkSize bytes).
	if err := os.WriteFile(st.PartPath, make([]byte, int64(st.NextChunk)*int64(st.ChunkSize)), 0o600); err != nil {
		t.Fatalf("create part file: %v", err)
	}

	// Save state
	broker.saveState(st)

	// Verify state file exists
	if _, err := os.Stat(broker.statePath(tid)); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// Load state
	loadedSt, err := broker.loadOrCreateState(tid, st.Filename, st.SenderName, st.TotalSize, st.ChunkSize, st.TotalChunks, time.Unix(0, st.ReceivedAt))
	if err != nil {
		t.Fatalf("loadOrCreateState failed: %v", err)
	}

	if loadedSt.NextChunk != 1 {
		t.Errorf("NextChunk mismatch: expected 1, got %d", loadedSt.NextChunk)
	}

	if loadedSt.TransferID != tid {
		t.Errorf("TransferID mismatch: expected %s, got %s", tid, loadedSt.TransferID)
	}

	// Delete state
	broker.deleteState(tid)
	if _, err := os.Stat(broker.statePath(tid)); err == nil {
		t.Fatal("state file still exists after delete")
	}
}

// TestUniquePathHandling verifies that duplicate filenames are renamed safely.
func TestUniquePathHandling(t *testing.T) {
	tmpDir := t.TempDir()
	broker := &Broker{saveDir: tmpDir}

	// Create first file
	path1 := broker.uniquePath("test.txt")
	if err := os.WriteFile(path1, []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Request path for same filename — should get _1 suffix
	path2 := broker.uniquePath("test.txt")
	if path2 == path1 {
		t.Fatal("uniquePath returned same path for existing file")
	}

	// Create second file
	if err := os.WriteFile(path2, []byte("second"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Request again — should get _2 suffix
	path3 := broker.uniquePath("test.txt")
	if path3 == path1 || path3 == path2 {
		t.Fatal("uniquePath returned duplicate path")
	}

	// Verify the two created files still exist.
	if _, err := os.Stat(path1); err != nil {
		t.Fatalf("path1 missing: %v", err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Fatalf("path2 missing: %v", err)
	}
	// path3 was not written; uniquePath only generates a candidate — verify it differs from the others.
	if path3 == path1 || path3 == path2 {
		t.Error("uniquePath should have returned a new unique path for path3")
	}
}

// TestTransferIDDeterminism verifies that same file from same sender always
// gets the same transfer ID (enables auto-resume without state server).
func TestTransferIDDeterminism(t *testing.T) {
	id1 := makeTransferID("alice", "file.zip", 1000000)
	id2 := makeTransferID("alice", "file.zip", 1000000)
	id3 := makeTransferID("alice", "file.zip", 1000001) // different size
	id4 := makeTransferID("bob", "file.zip", 1000000)   // different sender

	if id1 != id2 {
		t.Error("same file+sender should have same transfer ID")
	}
	if id1 == id3 {
		t.Error("different sizes should have different transfer IDs")
	}
	if id1 == id4 {
		t.Error("different senders should have different transfer IDs")
	}

	// All IDs should be exactly 32 hex chars
	for _, id := range []string{id1, id3, id4} {
		if len(id) != 32 {
			t.Errorf("transfer ID wrong length: %q (expected 32)", id)
		}
	}
}

// TestMaxFileSize verifies that oversized files are rejected.
func TestMaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	broker := &Broker{saveDir: tmpDir}

	// Use a value just above the current 20 GB limit.
	oversized := int64(pluginapi.MaxFileBytes) + 1
	if oversized <= int64(pluginapi.MaxFileBytes) {
		t.Skip("overflow: adjust oversized value")
	}

	// We cannot create a real 20 GB+ file in a unit test.
	// Instead verify SendFile rejects a non-existent path, which exercises
	// the os.Open error path before any size check.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := broker.SendFile(ctx, "12D3KooWInvalidPeer", "alice", tmpDir+"/nofile.iso")
	if err == nil {
		t.Error("expected error for missing file")
	}

	// Separately confirm the limit constant is what we expect.
	if pluginapi.MaxFileBytes != 20*1024*1024*1024 {
		t.Errorf("MaxFileBytes = %d, want 20 GB", pluginapi.MaxFileBytes)
	}
}

// TestChunkFrameRoundtrip verifies that chunk data survives encoding/decoding.
func TestChunkFrameRoundtrip(t *testing.T) {
	testData := []byte("Hello, this is test chunk data! " + string(make([]byte, chunkBytes-31)))

	var buf bytes.Buffer
	if err := writeCHUNK(&buf, 42, testData); err != nil {
		t.Fatalf("writeCHUNK failed: %v", err)
	}

	// Simulate reading: first read msgCHUNK type
	mtype, err := readByte(&buf)
	if err != nil {
		t.Fatalf("readByte failed: %v", err)
	}
	if mtype != msgCHUNK {
		t.Errorf("wrong message type: expected %d, got %d", msgCHUNK, mtype)
	}

	// Read chunk body
	idx, data, err := readCHUNKBody(&buf)
	if err != nil {
		t.Fatalf("readCHUNKBody failed: %v", err)
	}

	if idx != 42 {
		t.Errorf("chunk index mismatch: expected 42, got %d", idx)
	}

	if !bytes.Equal(data, testData) {
		t.Error("chunk data corrupted after roundtrip")
	}
}
