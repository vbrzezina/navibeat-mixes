package naming

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// S2-1
func TestEmptyTemplateGivesTheDefaultName(t *testing.T) {
	for _, tpl := range []string{"", "   "} {
		if got := Render(tpl, "Billy Talent", "Artist Radio 1"); got != "Artist Radio 1" {
			t.Errorf("template %q gave %q", tpl, got)
		}
	}
}

// S2-2
func TestPlaceholderIsReplacedAndOtherTextKept(t *testing.T) {
	cases := []struct{ tpl, want string }{
		{"This is {subject}", "This is Billy Talent"},
		{"{subject} Mix", "Billy Talent Mix"},
		{"{subject} / {subject}", "Billy Talent / Billy Talent"},
		{"No placeholder", "No placeholder"},
		{"{Subject} stays", "{Subject} stays"},
		{"{SUBJECT}", "{SUBJECT}"},
	}
	for _, c := range cases {
		if got := Render(c.tpl, "Billy Talent", "def"); got != c.want {
			t.Errorf("Render(%q) = %q, want %q", c.tpl, got, c.want)
		}
	}
}

// S2-3
func TestCleanRemovesControlCharsAndCollapsesWhitespace(t *testing.T) {
	if got := Clean("  a\x00b \t\n c   d\u0007 "); got != "ab c d" {
		t.Errorf("got %q", got)
	}
	if got := Render("{subject}\n Radio", "Pu\x01nk", "def"); got != "Punk Radio" {
		t.Errorf("got %q", got)
	}
}

// S2-3
func TestCutIsOneHundredCharactersAndRuneSafe(t *testing.T) {
	long := strings.Repeat("a", 150)
	if got := Clean(long); len(got) != 100 {
		t.Errorf("ascii cut gave %d", len(got))
	}
	// Each of these is 3 bytes; a byte cut at 100 would split one.
	multi := strings.Repeat("日", 150)
	got := Clean(multi)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 100 {
		t.Errorf("multi-byte cut: valid=%v runes=%d", utf8.ValidString(got), utf8.RuneCountInString(got))
	}
	// A cut that lands after a space must not leave a trailing space.
	spaced := strings.Repeat("a", 99) + " b"
	if got := Clean(spaced); got != strings.Repeat("a", 99) {
		t.Errorf("trailing space kept: %q", got)
	}
}

// S2-3: the prefix is not part of the 100, because the caller adds it after.
func TestRenderDoesNotKnowAboutThePrefix(t *testing.T) {
	got := Render(strings.Repeat("x", 100), "s", "def")
	if len(got) != 100 {
		t.Errorf("rendered length %d", len(got))
	}
}

// S2-4
func TestEmptyResultFallsBackToDefault(t *testing.T) {
	if got := Render("{subject}", "", "Genre Radio 1"); got != "Genre Radio 1" {
		t.Errorf("got %q", got)
	}
	if got := Render("\x00 {subject} \t", "  ", "Daily Mix 2"); got != "Daily Mix 2" {
		t.Errorf("got %q", got)
	}
}

// S2-5
func TestLaterCollisionUsesItsDefaultName(t *testing.T) {
	items := []Item{
		{Name: "P Billy", Default: "P Artist Radio 1"},
		{Name: "P Billy", Default: "P Artist Radio 2"},
		{Name: "P Other", Default: "P Artist Radio 3"},
		{Name: "P Billy", Default: "P Artist Radio 4"},
	}
	got := ResolveCollisions(items)
	want := []string{"P Billy", "P Artist Radio 2", "P Other", "P Artist Radio 4"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// S2-5: deterministic, and an earlier default also counts as taken.
func TestCollisionWithAnEarlierDefaultName(t *testing.T) {
	items := []Item{
		{Name: "P Artist Radio 1", Default: "P Artist Radio 1"},
		{Name: "P Artist Radio 1", Default: "P Artist Radio 2"},
	}
	got := ResolveCollisions(items)
	if got[0] != "P Artist Radio 1" || got[1] != "P Artist Radio 2" {
		t.Errorf("got %v", got)
	}
}
