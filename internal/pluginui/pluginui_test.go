package pluginui

import (
	"testing"
)

func TestSafe_StripsANSIEscapes(t *testing.T) {
	in := "merhaba\x1b[31mkirmizi\x1b[0mdunya"
	if got := Safe(in); got != "merhabakirmizidunya" {
		t.Fatalf("Safe(%q) = %q, want %q", in, got, "merhabakirmizidunya")
	}
}

func TestSafe_StripsControlCharsButKeepsNewlineAndTab(t *testing.T) {
	in := "a\x00b\x07c\nd\te"
	want := "abc\nd\te"
	if got := Safe(in); got != want {
		t.Fatalf("Safe(%q) = %q, want %q", in, got, want)
	}
}

func TestSafe_StripsOSCSequence(t *testing.T) {
	// OSC 8 hyperlink escape (ESC ] ... BEL) — a common terminal-injection
	// vector (can also carry OSC 52 clipboard-write, OSC 9 notifications, etc.
	// on terminals that support them). The whole framed sequence, including
	// its payload, must be removed — not just the leading ESC byte.
	in := "\x1b]8;;http://evil.example\x07tikla\x1b]8;;\x07"
	if got := Safe(in); got != "tikla" {
		t.Fatalf("Safe(OSC) = %q, want %q", got, "tikla")
	}
}

func TestSafe_NoRawControlByteEverSurvives(t *testing.T) {
	// Belt-and-suspenders: whatever the regex does or doesn't fully consume,
	// the final control-character filter must guarantee no raw ESC/BEL (or
	// any other sub-0x20 byte besides \n/\t) ever reaches the terminal.
	in := "\x1b]8;;http://evil.example\x07tikla\x1b]8;;\x07\x1b[2J\x07"
	got := Safe(in)
	for _, r := range got {
		if r < 0x20 && r != '\n' && r != '\t' {
			t.Fatalf("Safe(%q) = %q still contains raw control byte %#x", in, got, r)
		}
	}
}

func TestSafe_LeavesPlainTextUntouched(t *testing.T) {
	in := "sıradan bir başlık — ğüşiöç 123"
	if got := Safe(in); got != in {
		t.Fatalf("Safe(%q) = %q, want unchanged", in, got)
	}
}
