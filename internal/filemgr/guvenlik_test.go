package filemgr

import (
	"strings"
	"testing"
)

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"rapor.pdf":                  "rapor.pdf",
		"../../etc/passwd":           "passwd",
		`..\..\Windows\win.ini`:      "win.ini",
		"rapor.pdf:gizli":            "rapor.pdf_gizli", // NTFS alternatif veri akışı
		"CON":                        "_CON",
		"nul.txt":                    "_nul.txt",
		"com1.log":                   "_com1.log",
		"tehlikeli.exe. . ":          "tehlikeli.exe",
		"a<b>c|d?e*f\"g.txt":         "a_b_c_d_e_f_g.txt",
		"satır\nsonu\x00\x1b[2J.txt": "satırsonu.txt",
		"":                           "dosya",
		"..":                         "dosya",
		"   ":                        "dosya",
	}
	for in, want := range cases {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	// Yazı yönünü tersine çeviren karakter uzantıyı gizleyemez.
	if got := safeFilename("fatura" + string(rune(0x202E)) + "fdp.exe"); got != "faturafdp.exe" {
		t.Errorf("bidi override atılmalı, got %q", got)
	}
	// Uzun adlar uzantı korunarak kısaltılır.
	long := strings.Repeat("a", 400) + ".pdf"
	if got := safeFilename(long); len(got) > maxFilenameBytes || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("uzun ad %d bayta indirilmeli ve uzantı korunmalı, got %d bayt %q", maxFilenameBytes, len(got), got[len(got)-8:])
	}
}

func TestValidChunkLayout(t *testing.T) {
	const cs = chunkBytes
	cases := []struct {
		size        int64
		cs, chunks  uint32
		want        bool
		description string
	}{
		{1, cs, 1, true, "tek küçük parça"},
		{int64(cs) * 3, cs, 3, true, "tam bölünen"},
		{int64(cs)*3 + 1, cs, 4, true, "artıklı"},
		{int64(cs) * 3, cs, 4, false, "fazla parça sayısı"},
		{int64(cs) * 3, cs, 2, false, "eksik parça sayısı"},
		{100, 0, 1, false, "sıfır parça boyutu"},
		{100, 2*cs + 1, 1, false, "çok büyük parça boyutu"},
		{int64(cs) * 4, 1, 4, false, "küçük parça boyutuyla tutarsız sayı"},
	}
	for _, c := range cases {
		if got := validChunkLayout(c.size, c.cs, c.chunks); got != c.want {
			t.Errorf("%s: validChunkLayout(%d, %d, %d) = %v, want %v", c.description, c.size, c.cs, c.chunks, got, c.want)
		}
	}
}

func TestChunkFits(t *testing.T) {
	st := &recvState{TotalSize: 1000, ChunkSize: 400, TotalChunks: 3}
	if !chunkFits(st, 0, 400) || !chunkFits(st, 2, 200) {
		t.Fatal("geçerli parçalar kabul edilmeli")
	}
	if chunkFits(st, 3, 1) {
		t.Fatal("son parçadan sonraki indeks reddedilmeli")
	}
	if chunkFits(st, 2, 201) {
		t.Fatal("bildirilen boyutun ötesine taşan parça reddedilmeli")
	}
	if chunkFits(st, 0, 401) {
		t.Fatal("parça boyutundan büyük veri reddedilmeli")
	}
}

func TestResumableStateMustMatchSender(t *testing.T) {
	st := &recvState{TransferID: "t", Filename: "a.txt", SenderName: "peerA", TotalSize: 10, ChunkSize: chunkBytes, TotalChunks: 1}
	if !st.matches("a.txt", "peerA", 10, chunkBytes, 1) {
		t.Fatal("aynı gönderen ve boyutlar devam edebilmeli")
	}
	if st.matches("a.txt", "peerB", 10, chunkBytes, 1) {
		t.Fatal("başka bir eş, yarım kalmış bir aktarımı sürdürememeli")
	}
	if st.matches("a.txt", "peerA", 11, chunkBytes, 1) {
		t.Fatal("boyutu farklı aktarım yarım kalanı sürdürememeli")
	}
}
