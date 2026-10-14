package library

import (
	"testing"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
)

// F1: a template can render any name, so a hand made playlist that merely has
// the same name must not be taken over and overwritten.
func TestEnsureMixDoesNotAdoptAHandMadePlaylistThatOnlyHasTheName(t *testing.T) {
	for _, comment := range []string{"", "my favourite punk songs"} {
		s := &mixServer{lists: []Playlist{
			{ID: "mine", Name: "Punk Radio", Owner: "bob", Comment: comment},
		}}
		id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio", "Genre Radio 1")
		if err != nil {
			t.Fatal(err)
		}
		if id == "mine" || !created || len(s.creates) != 1 {
			t.Errorf("comment %q: adopted a hand made playlist (id=%q created=%v creates=%v)", comment, id, created, s.creates)
		}
	}
}

// F1: an unmarked playlist called the legacy default name is what 0.9.x
// before the marker wrote; it is adopted and renamed to the template name.
func TestEnsureMixAdoptsAnUnmarkedLegacyDefaultNamedPlaylist(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "old", Name: "Genre Radio 1", Owner: "bob", Comment: "hand typed"},
	}}
	id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio", "Genre Radio 1")
	if err != nil || id != "old" || created {
		t.Fatalf("id=%q created=%v err=%v", id, created, err)
	}
	if s.renames["old"] != "Punk Radio" || len(s.creates) != 0 {
		t.Errorf("renames=%v creates=%v", s.renames, s.creates)
	}
}

// F1: an unmarked playlist with the plugin's attribution line is ours even if
// its name is the rendered one.
func TestEnsureMixAdoptsAnUnmarkedPlaylistCarryingTheAttribution(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "ours", Name: "Punk Radio", Owner: "bob", Comment: "a sentence\n" + protocol.Attribution},
	}}
	id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio", "Genre Radio 1")
	if err != nil || id != "ours" || created {
		t.Fatalf("id=%q created=%v err=%v", id, created, err)
	}
}

// F2: copies carry the marker too. Tier 1: the one already called the wanted
// name wins even though a copy is newer.
func TestFindByMarkerPrefersTheWantedName(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "copy", Name: "My copy", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-05-01T10:00:00Z"},
		{ID: "legacy", Name: "Your Loved Songs", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-04-01T10:00:00Z"},
		{ID: "real", Name: "Loved", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-01-01T10:00:00Z"},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved", "Your Loved Songs")
	if err != nil || id != "real" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(s.renames) != 0 {
		t.Errorf("renamed %v", s.renames)
	}
}

// F2 tier 2: no playlist has the wanted name, the one with the legacy default
// name wins over a newer copy.
func TestFindByMarkerPrefersTheLegacyNameOverANewerCopy(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "copy", Name: "My copy", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-05-01T10:00:00Z"},
		{ID: "legacy", Name: "Your Loved Songs", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-01-01T10:00:00Z"},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved", "Your Loved Songs")
	if err != nil || id != "legacy" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

// F2 tier 3: nothing is named right, so recency decides: changed, then
// created, then list order.
func TestFindByMarkerFallsBackToChangedThenCreatedThenListOrder(t *testing.T) {
	m := marker("loved", "loved")
	pick := func(lists ...Playlist) string {
		s := &mixServer{lists: lists}
		id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved", "Loved")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if got := pick(
		Playlist{ID: "a", Name: "A", Owner: "bob", Comment: m, Changed: "2026-01-01T00:00:00Z"},
		Playlist{ID: "b", Name: "B", Owner: "bob", Comment: m, Changed: "2026-02-01T00:00:00Z"},
	); got != "b" {
		t.Errorf("changed: got %q", got)
	}
	if got := pick(
		Playlist{ID: "a", Name: "A", Owner: "bob", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-01T00:00:00Z"},
		Playlist{ID: "b", Name: "B", Owner: "bob", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-05T00:00:00Z"},
	); got != "b" {
		t.Errorf("created: got %q", got)
	}
	if got := pick(
		Playlist{ID: "a", Name: "A", Owner: "bob", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-01T00:00:00Z"},
		Playlist{ID: "b", Name: "B", Owner: "bob", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-01T00:00:00Z"},
	); got != "a" {
		t.Errorf("list order: got %q", got)
	}
}
