package library

import (
	"fmt"
	"strings"
	"testing"
)

// An unmarked playlist that is called the wanted name is adopted (as
// EnsurePlaylist always did), but one that only looks similar is not: a hand
// made playlist with another name is never taken over and overwritten.
func TestEnsureMixAdoptsOnlyAnUnmarkedPlaylistWithTheExactName(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "close", Name: "Genre Radio 1 ", Owner: "bob", Comment: "hand typed"},
		{ID: "case", Name: "genre radio 1", Owner: "bob"},
		{ID: "old", Name: "Genre Radio 1", Owner: "bob", Comment: "hand typed"},
	}}
	id, created, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Genre Radio 1")
	if err != nil || id != "old" || created {
		t.Fatalf("id=%q created=%v err=%v", id, created, err)
	}
	if len(s.renames) != 0 || len(s.creates) != 0 {
		t.Errorf("renames=%v creates=%v", s.renames, s.creates)
	}

	s = &mixServer{lists: []Playlist{
		{ID: "close", Name: "Genre Radio 1 ", Owner: "bob"},
	}}
	id, created, err = New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Genre Radio 1")
	if err != nil || id == "close" || !created || len(s.creates) != 1 {
		t.Errorf("adopted a playlist with another name: id=%q created=%v err=%v", id, created, err)
	}
}

// A mix you renamed by hand is found by its marker and renamed back.
func TestEnsureMixRenamesAHandRenamedMixBack(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "mine", Name: "my own title", Owner: "bob", Comment: marker("loved", "loved")},
	}}
	id, created, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved Songs")
	if err != nil || id != "mine" || created || s.renames["mine"] != "Loved Songs" || len(s.creates) != 0 {
		t.Errorf("id=%q created=%v err=%v renames=%v creates=%v", id, created, err, s.renames, s.creates)
	}
}

// Copies carry the marker too. Tier 1: the one already called the wanted name
// wins even though a copy is newer.
func TestFindByMarkerPrefersTheWantedName(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "copy", Name: "My copy", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-05-01T10:00:00Z"},
		{ID: "other", Name: "Another copy", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-04-01T10:00:00Z"},
		{ID: "real", Name: "Loved", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-01-01T10:00:00Z"},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved")
	if err != nil || id != "real" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(s.renames) != 0 {
		t.Errorf("renamed %v", s.renames)
	}
}

// Tier 2: nothing is named right, so recency decides: changed, then
// created, then list order.
func TestFindByMarkerFallsBackToChangedThenCreatedThenListOrder(t *testing.T) {
	m := marker("loved", "loved")
	pick := func(lists ...Playlist) string {
		s := &mixServer{lists: lists}
		id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved")
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

// A rename onto a name another of the owner's playlists already has is
// still done (Navidrome allows duplicate names) but is logged with both ids.
func TestEnsureMixLogsWhenTheRenameTargetIsAlreadyTaken(t *testing.T) {
	var lines []string
	Logf = func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) }
	defer func() { Logf = nil }()
	s := &mixServer{lists: []Playlist{
		{ID: "mine", Name: "Old", Owner: "bob", Comment: marker("genreradio", "genreradio-1")},
		{ID: "other", Name: "Punk Radio", Owner: "bob", Comment: marker("genreradio", "genreradio-2")},
		{ID: "alices", Name: "Punk Radio", Owner: "alice"},
	}}
	if _, _, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio"); err != nil {
		t.Fatal(err)
	}
	if s.renames["mine"] != "Punk Radio" {
		t.Errorf("the rename must still happen: %v", s.renames)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "mine") || !strings.Contains(lines[0], "other") || strings.Contains(lines[0], "alices") {
		t.Errorf("want one line naming mine and other only, got %q", lines)
	}
}
