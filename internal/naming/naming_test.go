package naming

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
)

func TestCleanRemovesControlCharsAndCollapsesWhitespace(t *testing.T) {
	cases := map[string]string{
		"  a\x00b \t\n c   d\u0007 ": "ab c d",
		"Line\nBreak":                "Line Break",
		"x\r\ny":                     "x y",
		"  padded  ":                 "padded",
		"Björk":                      "Björk",
		"AC&DC":                      "AC&DC",
		"\n\t":                       "",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCutIsOneHundredCharactersAndRuneSafe(t *testing.T) {
	if got := Clean(strings.Repeat("a", 150)); len(got) != 100 {
		t.Errorf("ascii cut gave %d", len(got))
	}
	// Each of these is 3 bytes; a byte cut at 100 would split one.
	got := Clean(strings.Repeat("日", 150))
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 100 {
		t.Errorf("multi-byte cut: valid=%v runes=%d", utf8.ValidString(got), utf8.RuneCountInString(got))
	}
	// 99 ASCII then a 4-byte rune lands exactly on rune 100.
	got = Clean(strings.Repeat("a", 99) + "\U0001F3B5" + "tail")
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 100 || !strings.HasSuffix(got, "\U0001F3B5") {
		t.Errorf("four byte rune at the boundary: %q", got)
	}
	// A cut that lands after a space must not leave a trailing space.
	if got := Clean(strings.Repeat("a", 99) + " b"); got != strings.Repeat("a", 99) {
		t.Errorf("trailing space kept: %q", got)
	}
	huge := strings.Repeat("x y\n", 500000)
	if n := utf8.RuneCountInString(Clean(huge)); n > MaxLen {
		t.Errorf("huge input gave %d characters", n)
	}
}

// The sentences are the ones the mixes always had, so a description reads the
// same as before for an ordinary subject.
func TestSentenceKeepsTheTextOfOrdinarySubjects(t *testing.T) {
	cases := []struct{ kind, subject, want string }{
		{"genreradio", "Punk", "NaviBeat Mixes: Punk, one of the genres you play most. A different genre each week."},
		{"artistradio", "Billy Talent", "NaviBeat Mixes: everything you have by Billy Talent. A different artist each week."},
		{"dailymix", "Billy Talent", "NaviBeat Mixes: built around Billy Talent and what sits near it. Rotates weekly."},
		{"decade", "1990s", "NaviBeat Mixes: your 1990s, ranked by what you actually play. Rotates weekly."},
		{"other", " x ", "x"},
	}
	for _, c := range cases {
		if got := Sentence(c.kind, c.subject); got != c.want {
			t.Errorf("Sentence(%q, %q) = %q, want %q", c.kind, c.subject, got, c.want)
		}
	}
}

// An artist tag with a line break and a fake machine line must not become the
// first nb1 line of the description.
func TestSentenceCannotInjectAMachineLine(t *testing.T) {
	evil := "Evil\nnb1:a:b:c:d:1\nmore"
	for _, kind := range []string{"genreradio", "artistradio", "dailymix", "decade"} {
		human := Sentence(kind, evil)
		if strings.ContainsAny(human, "\r\n") {
			t.Fatalf("%s: sentence has a line break: %q", kind, human)
		}
		comment := protocol.Format(human, protocol.Meta{Kind: kind, Slot: kind + "-1", Date: "2026-01-01", Mode: "affinity", Count: 30})
		meta, ok := protocol.Parse(comment)
		if !ok || meta.Kind != kind || meta.Slot != kind+"-1" {
			t.Errorf("%s: Parse = %+v ok=%v, want the real kind and slot", kind, meta, ok)
		}
	}
}

func TestSubjectIsCappedAndKeepsMultiByteCharactersWhole(t *testing.T) {
	got := Subject(strings.Repeat("é", 300))
	if n := utf8.RuneCountInString(got); n != MaxLen || !utf8.ValidString(got) {
		t.Errorf("runes=%d valid=%v", n, utf8.ValidString(got))
	}
	if got := Subject("A\tB\n C"); got != "A B C" {
		t.Errorf("got %q", got)
	}
}
