package naming

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// QA probes written from the spec (S2-1..S2-5), not from the implementation.

func TestQAEmptyAndWhitespaceTemplates(t *testing.T) {
	for _, tpl := range []string{"", " ", "\t\n ", " ", " "} {
		if got := Render(tpl, "Billy Talent", "Artist Radio 1"); got != "Artist Radio 1" {
			t.Errorf("template %q => %q, want default", tpl, got)
		}
	}
}

func TestQAPlaceholderCaseSensitive(t *testing.T) {
	for _, tpl := range []string{"{Subject}", "{SUBJECT}", "{ subject }", "{subject", "subject}", "{{subject"} {
		got := Render(tpl, "X", "D")
		want := Clean(strings.ReplaceAll(tpl, "{subject}", "X"))
		if got != want {
			t.Errorf("%q => %q want %q", tpl, got, want)
		}
	}
	// {{subject}} is one literal brace each side around the replacement.
	if got := Render("{{subject}}", "X", "D"); got != "{X}" {
		t.Errorf("{{subject}} => %q", got)
	}
}

func TestQASubjectContainingThePlaceholderIsNotReexpanded(t *testing.T) {
	got := Render("{subject}|{subject}", "{subject}", "D")
	if got != "{subject}|{subject}" {
		t.Errorf("got %q (recursive expansion?)", got)
	}
	got = Render("a {subject} b", "{subject}{subject}{subject}", "D")
	if got != "a {subject}{subject}{subject} b" {
		t.Errorf("got %q", got)
	}
}

func TestQAHostileSubjects(t *testing.T) {
	cases := map[string]string{
		"AC&DC":              "This is AC&DC",
		"a=b":                "This is a=b",
		"100%":               "This is 100%",
		`Say "Hi" 'x'`:       `This is Say "Hi" 'x'`,
		"Line\nBreak":        "This is Line Break",
		"Tab\there":          "This is Tab here",
		"a\x00b":             "This is ab",
		"a\u0007b":           "This is ab",
		"  padded  ":         "This is padded",
		"x\r\ny":             "This is x y",
		"Björk":              "This is Björk",
		"%s %d %!":           "This is %s %d %!",
		"<script>a</script>": "This is <script>a</script>",
	}
	for subj, want := range cases {
		if got := Render("This is {subject}", subj, "D"); got != want {
			t.Errorf("subject %q => %q want %q", subj, got, want)
		}
	}
}

func TestQAOnlyWhitespaceSubjectFallsBack(t *testing.T) {
	for _, subj := range []string{"", " ", "\n\t", "\x00\x01", "  "} {
		if got := Render("{subject}", subj, "Genre Radio 1"); got != "Genre Radio 1" {
			t.Errorf("subject %q => %q", subj, got)
		}
	}
}

func TestQAMultiplePlaceholders(t *testing.T) {
	if got := Render("{subject} {subject} {subject}", "Go", "D"); got != "Go Go Go" {
		t.Error(got)
	}
	if got := Render("no placeholder", "Go", "D"); got != "no placeholder" {
		t.Error(got)
	}
	// Placeholder at both ends with whitespace subject in the middle.
	if got := Render("[{subject}]", " ", "D"); got != "[ ]" {
		t.Errorf("got %q", got)
	}
}

func TestQACutIsRuneSafeFourByteAtBoundary(t *testing.T) {
	// 99 ASCII then a 4-byte rune lands exactly on rune 100.
	s := strings.Repeat("a", 99) + "\U0001F3B5" + "tail"
	got := Clean(s)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 100 {
		t.Fatalf("len %d valid %v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	if !strings.HasSuffix(got, "\U0001F3B5") {
		t.Error("100th rune should be the emoji")
	}
	s = strings.Repeat("a", 100) + "\U0001F3B5"
	if got := Clean(s); utf8.RuneCountInString(got) != 100 || !utf8.ValidString(got) {
		t.Error("101st-rune emoji leaked")
	}
	// All 4-byte runes: 400 bytes -> 100 runes.
	got = Clean(strings.Repeat("\U0001F3B5", 500))
	if utf8.RuneCountInString(got) != 100 || len(got) != 400 {
		t.Errorf("runes %d bytes %d", utf8.RuneCountInString(got), len(got))
	}
}

func TestQACutThenTrimsTrailingSpace(t *testing.T) {
	s := strings.Repeat("a", 99) + " " + "b"
	got := Clean(s)
	if got != strings.Repeat("a", 99) {
		t.Errorf("trailing space after cut kept: len %d", len(got))
	}
}

func TestQAEverySubjectOutputIsAtMostMaxLen(t *testing.T) {
	huge := strings.Repeat("x y\n", 500000) // ~2 MB
	got := Render("{subject} {subject}", huge, "D")
	if utf8.RuneCountInString(got) > MaxLen {
		t.Errorf("len %d", utf8.RuneCountInString(got))
	}
	if strings.ContainsAny(got, "\n\t") || strings.Contains(got, "  ") {
		t.Error("not cleaned")
	}
}

func TestQAUnicodeNotMangled(t *testing.T) {
	// Combining marks: e + U+0301 must survive as two runes.
	subj := "Café Tacvba"
	if got := Render("{subject}", subj, "D"); got != subj {
		t.Errorf("got %q", got)
	}
	// RTL text and CJK.
	for _, subj := range []string{"שלום עולם", "宇多田ヒカル", "مرحبا"} {
		if got := Render("{subject} Mix", subj, "D"); got != subj+" Mix" {
			t.Errorf("got %q", got)
		}
	}
}

// SURPRISE probe (not a spec violation): zero width and bidi format characters
// are category Cf, not Cc, so they are neither removed nor treated as space.
func TestQASurpriseFormatCharsSurvive(t *testing.T) {
	got := Render("{subject}", "​", "Genre Radio 1")
	t.Logf("subject ZWSP only => %q (default would be %q)", got, "Genre Radio 1")
	got = Render("{subject}", "a‮b", "D")
	t.Logf("RLO override kept: %q", got)
	// ZWJ emoji family cut at 100 can split a grapheme cluster.
	fam := "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466"
	cut := Clean(strings.Repeat("a", 97) + fam)
	t.Logf("cut inside a ZWJ sequence: %q (runes=%d)", cut[len(cut)-12:], utf8.RuneCountInString(cut))
}

func TestQACollisionThreeWay(t *testing.T) {
	out := ResolveCollisions([]Item{
		{"P Same", "P D1"}, {"P Same", "P D2"}, {"P Same", "P D3"}, {"P Other", "P D4"},
	})
	want := []string{"P Same", "P D2", "P D3", "P Other"}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("%d: %q want %q", i, out[i], want[i])
		}
	}
}

func TestQACollisionWithEarlierDefault(t *testing.T) {
	// Item 0 is a templated name that happens to equal item 1's default.
	out := ResolveCollisions([]Item{{"P Daily Mix 1", "P Artist Radio 1"}, {"P Daily Mix 1", "P Daily Mix 1"}})
	// Spec S2-5: later one uses its default. Its default IS the taken name.
	if out[0] == out[1] {
		t.Logf("DEFECT-CANDIDATE: both mixes end up named %q (default of the later equals the earlier's rendered name)", out[0])
	}
}

func TestQACollisionDefaultsThemselvesTaken(t *testing.T) {
	// A fallback default that was already taken is not re-checked.
	out := ResolveCollisions([]Item{{"P X", "P D1"}, {"P D2", "P D2"}, {"P X", "P D2"}})
	if out[1] == out[2] {
		t.Logf("SURPRISE: fallback default %q is used twice (%v)", out[1], out)
	}
}

func TestQACollisionIsCaseSensitive(t *testing.T) {
	out := ResolveCollisions([]Item{{"P Punk", "P D1"}, {"P punk", "P D2"}})
	if out[0] == out[1] {
		t.Error("distinct case should be distinct")
	}
	t.Logf("case-differing names kept as %v (Navidrome/clients may show them as near duplicates)", out)
}

func TestQACollisionDeterministic(t *testing.T) {
	items := []Item{{"a", "d1"}, {"a", "d2"}, {"b", "d3"}, {"a", "d4"}, {"b", "d5"}}
	first := strings.Join(ResolveCollisions(items), "|")
	for i := 0; i < 50; i++ {
		if strings.Join(ResolveCollisions(items), "|") != first {
			t.Fatal("non deterministic")
		}
	}
	if ResolveCollisions(nil) == nil || len(ResolveCollisions(nil)) != 0 {
		t.Log("nil input returns", ResolveCollisions(nil))
	}
}

func TestQAPrefixMakesNamesEqual(t *testing.T) {
	// Callers add the prefix before ResolveCollisions. Two different rendered
	// names cannot become equal by adding the same prefix; but names equal
	// only after prefixing (identical rendered text) must collide.
	p := "\U0001F7E0 "
	out := ResolveCollisions([]Item{{p + "Same", p + "Artist Radio 1"}, {p + "Same", p + "Daily Mix 1"}})
	if out[1] != p+"Daily Mix 1" {
		t.Errorf("got %v", out)
	}
}
