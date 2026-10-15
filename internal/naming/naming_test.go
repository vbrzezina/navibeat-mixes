package naming

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
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

// A fixed name always wins, in either plan order: a genre called "Morning"
// (template "{subject}") must not push the real Morning mix to "Morning (2)".
func TestFixedNameWinsOverATemplatedOneInEitherOrder(t *testing.T) {
	templated := Item{Name: "P Morning", Default: "P Genre Radio 1"}
	fixed := Item{Name: "P Morning", Default: "P Morning"} // add(): name equals default
	for _, items := range [][]Item{{templated, fixed}, {fixed, templated}} {
		got := ResolveCollisions(items)
		want := []string{"P Genre Radio 1", "P Morning"}
		if items[0] == fixed {
			want = []string{"P Morning", "P Genre Radio 1"}
		}
		if got[0] != want[0] || got[1] != want[1] {
			t.Errorf("order %v: got %v want %v", items, got, want)
		}
	}
}

// Two fixed names that clash keep their names, as upstream did (a user who
// gives two mixes the same name asked for it).
func TestTwoFixedNamesAreLeftAlone(t *testing.T) {
	items := []Item{{Name: "P X", Default: "P X"}, {Name: "P X", Default: "P X"}}
	got := ResolveCollisions(items)
	if got[0] != "P X" || got[1] != "P X" {
		t.Errorf("got %v", got)
	}
}

// A template that renders exactly a fixed mix's default name steps aside, and
// two templated claimants take their own defaults.
func TestTemplatedNameEqualToAFixedDefaultStepsAside(t *testing.T) {
	items := []Item{
		{Name: "P Daily Mix 1", Default: "P Genre Radio 1"}, // template gave "Daily Mix 1"
		{Name: "P Daily Mix 1", Default: "P Daily Mix 1"},   // the real Daily Mix 1
		{Name: "P Daily Mix 1", Default: "P Genre Radio 2"}, // a second template claimant
	}
	got := ResolveCollisions(items)
	want := []string{"P Genre Radio 1", "P Daily Mix 1", "P Genre Radio 2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The fallback can be taken too (by a fixed mix): then a numeric suffix is
// added so no two mixes share a name.
func TestFallbackAlsoTakenGetsASuffix(t *testing.T) {
	items := []Item{
		{Name: "P Morning", Default: "P Morning"},             // fixed
		{Name: "P Genre Radio 1", Default: "P Genre Radio 1"}, // fixed, holds the default below
		{Name: "P Morning", Default: "P Genre Radio 1"},       // templated, both names taken
	}
	got := ResolveCollisions(items)
	if got[2] != "P Genre Radio 1 (2)" {
		t.Errorf("got %v", got)
	}
}

// The suffix goes on after the cut, so a 100 character default keeps it.
func TestCollisionSuffixIsNeverCutOff(t *testing.T) {
	long := "P " + strings.Repeat("x", MaxLen)
	other := "P " + strings.Repeat("y", MaxLen)
	items := []Item{
		{Name: long, Default: long},
		{Name: other, Default: other},
		{Name: long, Default: other}, // templated: both names taken
	}
	got := ResolveCollisions(items)
	if got[2] != other+" (2)" {
		t.Errorf("got %q", got[2])
	}
}

// F3: an artist tag with a line break and a fake machine line must not become
// the first nb1 line of the description.
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
