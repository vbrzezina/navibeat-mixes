// Package naming turns a name template and a subject into a playlist name.
//
// Every mix family that can be named after what is in it has exactly one
// subject (the artist, the genre, the decade), so there is exactly one
// placeholder, {subject}. Keeping the grammar this small is the point: no
// escaping rules, no conditionals, nothing to get wrong in a settings field.
//
// This package is pure on purpose. It has no idea what a playlist is, so the
// rules a reviewer cares about (cleaning, the length cut, collisions) are
// plain functions over strings that a test can call directly.
package naming

import (
	"strconv"
	"strings"
	"unicode"
)

// Placeholder is the only token a template understands. It is case sensitive:
// "{Subject}" is ordinary text and is kept as written.
const Placeholder = "{subject}"

// MaxLen is the longest a rendered name may be, in characters. The configured
// prefix is added by the caller after rendering and is not counted.
const MaxLen = 100

// Render fills a template with the subject and cleans the result.
//
// An empty template, or one whose cleaned result is empty, gives def, which is
// the name the mix has always had. That is what makes the feature safe to
// ship: a server that never fills a template field sees no change at all.
func Render(template, subject, def string) string {
	if strings.TrimSpace(template) == "" {
		return def
	}
	out := Clean(strings.ReplaceAll(template, Placeholder, subject))
	if out == "" {
		return def
	}
	return out
}

// Clean makes a string safe to use as a playlist name: control characters are
// removed, every run of whitespace becomes one space, the ends are trimmed,
// and the result is cut to MaxLen characters.
//
// Tab, newline and the like are control characters AND whitespace. They are
// treated as whitespace (a space, then collapsed) rather than deleted, so an
// artist tag with a stray newline reads "A B" and not "AB".
//
// The cut counts runes, not bytes. Slicing bytes would split a multi-byte
// character in half and hand the server invalid UTF-8, which is a real risk
// with the artist names this plugin sees.
func Clean(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsControl(r):
			// Dropped without breaking the word it sits in.
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	runes := []rune(b.String())
	if len(runes) > MaxLen {
		runes = runes[:MaxLen]
	}
	return strings.TrimRightFunc(string(runes), unicode.IsSpace)
}

// Item is one mix in the ordered plan: the name it would get now, and the
// numbered default it falls back to.
type Item struct {
	// Name is the full name, prefix included.
	Name string
	// Default is the full numbered default name, prefix included.
	Default string
}

// ResolveCollisions returns the final name of every item, in order.
//
// Two mixes with the same name would be two playlists a person cannot tell
// apart, and the one written second would look like it replaced the first. So
// when a name equals one already taken earlier in the plan, that later mix
// uses its default instead. The plan order is fixed, so the same library
// always resolves the same way and a playlist does not flip names night to
// night.
//
// The default can be taken too: by a fixed-name mix (a genre literally called
// "Morning" against the time-of-day mix of that name) or by a template that
// renders exactly another mix's default ("Daily Mix 1"). Then " (2)", " (3)"
// and so on is appended to the default until the name is free. The suffix is
// added to the finished name, after the 100 character cut and the prefix, so
// it can never be cut off.
func ResolveCollisions(items []Item) []string {
	taken := make(map[string]bool, len(items))
	out := make([]string, len(items))
	for i, it := range items {
		name := it.Name
		if taken[name] {
			name = it.Default
			for n := 2; taken[name]; n++ {
				name = it.Default + " (" + strconv.Itoa(n) + ")"
			}
		}
		taken[name] = true
		out[i] = name
	}
	return out
}
