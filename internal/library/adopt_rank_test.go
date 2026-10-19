package library

import (
	"testing"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
)

func TestAdoptsAnUnmarkedPlaylistOnlyByExactName(t *testing.T) {
	cases := []struct {
		name      string
		existing  string
		wantID    string
		wantNewly bool
	}{
		{"exact name", "Genre Radio 1", "old", false},
		{"trailing space", "Genre Radio 1 ", "new2", true},
		{"other case", "genre radio 1", "new2", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &fakeNavidrome{lists: []Playlist{{ID: "old", Name: c.existing, Owner: "bob", Comment: "hand typed"}}}
			id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Genre Radio 1", "Genre Radio 1")
			if err != nil || id != c.wantID || created != c.wantNewly {
				t.Errorf("id=%q created=%v err=%v", id, created, err)
			}
		})
	}
}

func TestDoesNotAdoptAPlaylistMarkedAsAnotherMix(t *testing.T) {
	for _, other := range []struct{ kind, slot string }{
		{"genreradio", "genreradio-2"},  // another slot
		{"artistradio", "genreradio-1"}, // another kind
	} {
		s := &fakeNavidrome{lists: []Playlist{
			{ID: "other", Name: "Punk Radio", Owner: "bob", Comment: marker(other.kind, other.slot)},
		}}
		id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio", "Punk Radio")
		if err != nil || id == "other" || !created || s.did("rename") != 0 {
			t.Errorf("%v: id=%q created=%v err=%v ops=%v", other, id, created, err, s.ops)
		}
	}
}

func TestRanksPlaylistsThatCarryTheSameMarker(t *testing.T) {
	m := marker("loved", "loved")
	cases := []struct {
		name   string
		legacy string // the default name an older version used; "Wanted" if empty
		lists  []Playlist
		want   string
	}{
		{"legacy name beats a newer copy", "Old Default", []Playlist{
			{ID: "copy", Name: "My copy", Comment: m, Changed: "2026-05-01T10:00:00Z"},
			{ID: "old", Name: "Old Default", Comment: m, Changed: "2026-01-01T10:00:00Z"},
		}, "old"},
		{"wanted name beats the legacy name", "Old Default", []Playlist{
			{ID: "old", Name: "Old Default", Comment: m, Changed: "2026-05-01T10:00:00Z"},
			{ID: "real", Name: "Wanted", Comment: m, Changed: "2026-01-01T10:00:00Z"},
		}, "real"},
		{"wanted name beats a newer copy", "", []Playlist{
			{ID: "copy", Name: "My copy", Comment: m, Changed: "2026-05-01T10:00:00Z"},
			{ID: "real", Name: "Wanted", Comment: m, Changed: "2026-01-01T10:00:00Z"},
		}, "real"},
		{"most recently changed", "", []Playlist{
			{ID: "a", Name: "A", Comment: m, Changed: "2026-01-01T00:00:00Z"},
			{ID: "b", Name: "B", Comment: m, Changed: "2026-03-01T00:00:00Z"},
			{ID: "c", Name: "C", Comment: m, Changed: "2026-02-01T00:00:00Z"},
		}, "b"},
		{"then most recently created", "", []Playlist{
			{ID: "a", Name: "A", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-01T00:00:00Z"},
			{ID: "b", Name: "B", Comment: m, Changed: "2026-02-01T00:00:00Z", Created: "2026-01-05T00:00:00Z"},
		}, "b"},
		{"then first listed", "", []Playlist{
			{ID: "a", Name: "A", Comment: m},
			{ID: "b", Name: "B", Comment: m},
		}, "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := map[string]string{}
			for i := range c.lists {
				c.lists[i].Owner = "bob"
				before[c.lists[i].ID] = c.lists[i].Name
			}
			s := &fakeNavidrome{lists: c.lists}
			if c.legacy == "" {
				c.legacy = "Wanted"
			}
			id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Wanted", c.legacy)
			if err != nil || id != c.want {
				t.Fatalf("id=%q err=%v, want %q", id, err, c.want)
			}
			for _, l := range s.lists {
				if l.ID != c.want && l.Name != before[l.ID] {
					t.Errorf("the other playlist %s was renamed", l.ID)
				}
			}
		})
	}
}

// A name template can render any name, so a hand made playlist that merely has
// the same name must not be taken over and overwritten.
func TestAdoptsAnUnmarkedPlaylistOnlyIfItLooksLikeOursWhenTheNameIsTemplated(t *testing.T) {
	cases := []struct {
		name, existing, comment string
		adopted                 bool
	}{
		{"the legacy default name", "Genre Radio 1", "hand typed", true},
		{"the new name with our attribution", "Punk Radio", "a sentence\n" + protocol.Attribution, true},
		{"the new name only, no description", "Punk Radio", "", false},
		{"the new name only, own description", "Punk Radio", "my favourite punk songs", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &fakeNavidrome{lists: []Playlist{{ID: "x", Name: c.existing, Owner: "bob", Comment: c.comment}}}
			id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio", "Genre Radio 1")
			if err != nil || (id == "x") != c.adopted || created == c.adopted {
				t.Fatalf("id=%q created=%v err=%v", id, created, err)
			}
			if c.adopted && s.name("x") != "Punk Radio" {
				t.Errorf("adopted playlist is called %q", s.name("x"))
			}
		})
	}
}
