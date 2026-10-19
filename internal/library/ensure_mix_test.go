package library

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
)

// mixServer is a minimal Subsonic double for the playlist calls EnsureMix
// makes. It records every call so a test can assert on what was NOT sent.
type mixServer struct {
	lists      []Playlist
	calls      []string
	renames    map[string]string // playlistId -> new name
	creates    []string
	failRename bool
}

func (s *mixServer) call(uri string) (string, error) {
	endpoint, query, _ := strings.Cut(uri, "?")
	p, _ := url.ParseQuery(query)
	s.calls = append(s.calls, endpoint)
	switch endpoint {
	case "getPlaylists":
		items := ""
		for i, l := range s.lists {
			if i > 0 {
				items += ","
			}
			items += fmt.Sprintf(`{"id":%q,"name":%q,"comment":%q,"owner":%q,"changed":%q,"created":%q}`,
				l.ID, l.Name, l.Comment, l.Owner, l.Changed, l.Created)
		}
		return `{"subsonic-response":{"status":"ok","playlists":{"playlist":[` + items + `]}}}`, nil
	case "createPlaylist":
		s.creates = append(s.creates, p.Get("name"))
		return `{"subsonic-response":{"status":"ok","playlist":{"id":"new` + fmt.Sprint(len(s.creates)) + `"}}}`, nil
	case "updatePlaylist":
		if n := p.Get("name"); n != "" {
			if s.failRename {
				return `{"subsonic-response":{"status":"failed","error":{"code":0,"message":"nope"}}}`, nil
			}
			if s.renames == nil {
				s.renames = map[string]string{}
			}
			s.renames[p.Get("playlistId")] = n
		}
	}
	return `{"subsonic-response":{"status":"ok"}}`, nil
}

func (s *mixServer) count(endpoint string) int {
	n := 0
	for _, c := range s.calls {
		if c == endpoint {
			n++
		}
	}
	return n
}

func marker(kind, slot string) string {
	return protocol.Format("human", protocol.Meta{Kind: kind, Slot: slot, Date: "2026-01-01", Mode: "affinity", Count: 30})
}

// found by marker although the name differs, same id, renamed.
func TestEnsureMixFindsByMarkerAndRenamesInPlace(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "a1", Name: "Old Prefix Artist Radio 1", Owner: "bob", Comment: marker("artistradio", "artistradio-1")},
	}}
	c := New(s.call, "bob")
	id, _, err := c.EnsureMix("artistradio", "artistradio-1", "New Prefix Artist Radio 1")
	if err != nil || id != "a1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if s.renames["a1"] != "New Prefix Artist Radio 1" {
		t.Errorf("rename not sent: %v", s.renames)
	}
	if len(s.creates) != 0 {
		t.Errorf("a duplicate was created: %v", s.creates)
	}
	// The cache follows the rename, so a second call sends nothing.
	if _, _, err := c.EnsureMix("artistradio", "artistradio-1", "New Prefix Artist Radio 1"); err != nil {
		t.Fatal(err)
	}
	if len(s.renames) != 1 || s.count("updatePlaylist") != 1 {
		t.Errorf("second call renamed again: %d updatePlaylist", s.count("updatePlaylist"))
	}
}

// An unchanged name is not renamed at all.
func TestEnsureMixSkipsRenameWhenNameMatches(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "a1", Name: "Same", Owner: "bob", Comment: marker("loved", "loved")},
	}}
	if _, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Same"); err != nil {
		t.Fatal(err)
	}
	if s.count("updatePlaylist") != 0 {
		t.Error("renamed a playlist that already had the name")
	}
}

// a playlist from an older version, no marker, matched by exact name
// and adopted rather than duplicated.
func TestEnsureMixAdoptsByNameWhenThereIsNoMarker(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "old", Name: "Daily Mix 1", Owner: "bob", Comment: "just a sentence"},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("dailymix", "dailymix-1", "Daily Mix 1")
	if err != nil || id != "old" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(s.creates) != 0 {
		t.Errorf("created %v", s.creates)
	}
}

// a playlist already marked as another mix is not adopted by
// name, it belongs to its own slot.
func TestEnsureMixDoesNotStealAnotherMixByName(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "other", Name: "Punk Radio", Owner: "bob", Comment: marker("genreradio", "genreradio-2")},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("genreradio", "genreradio-1", "Punk Radio")
	if err != nil {
		t.Fatal(err)
	}
	if id == "other" {
		t.Fatal("took a playlist that belongs to genreradio-2")
	}
	if len(s.creates) != 1 {
		t.Errorf("creates=%v", s.creates)
	}
}

// another user's playlist never matches, by marker or by name.
func TestEnsureMixNeverMatchesAnotherUsersPlaylist(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "alice1", Name: "Morning", Owner: "alice", Comment: marker("timeofday", "morning")},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("timeofday", "morning", "Morning")
	if err != nil {
		t.Fatal(err)
	}
	if id == "alice1" {
		t.Fatal("matched alice's playlist")
	}
	if len(s.renames) != 0 {
		t.Errorf("renamed something: %v", s.renames)
	}
	if len(s.creates) != 1 || s.creates[0] != "Morning" {
		t.Errorf("creates=%v", s.creates)
	}
}

// duplicates carrying the same marker: the most recently changed wins,
// the others are untouched.
func TestEnsureMixPicksMostRecentlyChangedAndLeavesTheRest(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "old", Name: "A", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-01-01T10:00:00Z"},
		{ID: "newest", Name: "B", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-03-01T10:00:00Z"},
		{ID: "mid", Name: "C", Owner: "bob", Comment: marker("loved", "loved"), Changed: "2026-02-01T10:00:00Z"},
	}}
	id, _, err := New(s.call, "bob").EnsureMix("loved", "loved", "Loved")
	if err != nil || id != "newest" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if len(s.renames) != 1 || s.renames["newest"] != "Loved" {
		t.Errorf("only the newest should be renamed: %v", s.renames)
	}
}

// a failed rename is not an error; the id still comes back.
func TestEnsureMixFailedRenameStillReturnsThePlaylist(t *testing.T) {
	s := &mixServer{failRename: true, lists: []Playlist{
		{ID: "a1", Name: "Old", Owner: "bob", Comment: marker("loved", "loved")},
	}}
	c := New(s.call, "bob")
	id, _, err := c.EnsureMix("loved", "loved", "New")
	if err != nil || id != "a1" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if c.playlists[0].Name != "Old" {
		t.Errorf("cache claims a rename that failed: %q", c.playlists[0].Name)
	}
}

// Nothing matches: it is created with the wanted name, and the cache then
// finds it once its marker is written.
func TestEnsureMixCreatesAndCachesWhenNothingMatches(t *testing.T) {
	s := &mixServer{}
	c := New(s.call, "bob")
	id, created, err := c.EnsureMix("loved", "loved", "Loved")
	if err != nil || id != "new1" || !created {
		t.Fatalf("id=%q created=%v err=%v", id, created, err)
	}
	if err := c.SetComment(id, marker("loved", "loved")); err != nil {
		t.Fatal(err)
	}
	// The name Renamed matches nothing, so only the marker SetComment just wrote
	// into the cache can lead back to this playlist.
	again, _, _ := c.EnsureMix("loved", "loved", "Renamed")
	if again != "new1" || len(s.creates) != 1 {
		t.Errorf("again=%q creates=%v", again, s.creates)
	}
	if s.count("getPlaylists") != 1 {
		t.Errorf("getPlaylists fetched %d times", s.count("getPlaylists"))
	}
}

// EnsurePlaylist and FindControl keep their behaviour.
func TestEnsurePlaylistStillMatchesByNameAndOwner(t *testing.T) {
	s := &mixServer{lists: []Playlist{
		{ID: "x", Name: "N", Owner: "alice"},
		{ID: "y", Name: "N", Owner: "bob"},
	}}
	id, err := New(s.call, "bob").EnsurePlaylist("N")
	if err != nil || id != "y" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
