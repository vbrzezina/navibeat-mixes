package library

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/nenadjokic/navibeat-mixes/internal/protocol"
)

// qaServer is a small stateful Subsonic double, written independently of the
// implementers' mixServer. It applies renames and comments like Navidrome and
// records every call.
type qaServer struct {
	lists       []Playlist
	calls       []string
	failList    bool
	failRename  bool
	failCreate  bool
	truncateAt  int // if >0, comments are cut to this many runes on write
	nextID      int
	deleteCalls int
}

func (s *qaServer) call(uri string) (string, error) {
	endpoint, query, _ := strings.Cut(uri, "?")
	p, _ := url.ParseQuery(query)
	s.calls = append(s.calls, endpoint+"?"+query)
	if strings.Contains(endpoint, "delete") {
		s.deleteCalls++
	}
	ok := `{"subsonic-response":{"status":"ok"}}`
	switch endpoint {
	case "getPlaylists":
		if s.failList {
			return "", fmt.Errorf("boom")
		}
		var items []string
		for _, l := range s.lists {
			items = append(items, fmt.Sprintf(`{"id":%q,"name":%q,"comment":%q,"owner":%q,"changed":%q,"created":%q}`,
				l.ID, l.Name, l.Comment, l.Owner, l.Changed, l.Created))
		}
		return `{"subsonic-response":{"status":"ok","playlists":{"playlist":[` + strings.Join(items, ",") + `]}}}`, nil
	case "createPlaylist":
		if id := p.Get("playlistId"); id != "" {
			return ok, nil
		}
		if s.failCreate {
			return `{"subsonic-response":{"status":"failed","error":{"code":0,"message":"no"}}}`, nil
		}
		s.nextID++
		id := fmt.Sprintf("n%d", s.nextID)
		s.lists = append(s.lists, Playlist{ID: id, Name: p.Get("name"), Owner: p.Get("u")})
		return `{"subsonic-response":{"status":"ok","playlist":{"id":"` + id + `"}}}`, nil
	case "updatePlaylist":
		for i := range s.lists {
			if s.lists[i].ID != p.Get("playlistId") {
				continue
			}
			if _, has := p["name"]; has {
				if s.failRename {
					return `{"subsonic-response":{"status":"failed","error":{"code":0,"message":"nope"}}}`, nil
				}
				s.lists[i].Name = p.Get("name")
			}
			if c, has := p["comment"]; has {
				v := c[0]
				if s.truncateAt > 0 && len([]rune(v)) > s.truncateAt {
					v = string([]rune(v)[:s.truncateAt])
				}
				s.lists[i].Comment = v
			}
		}
	}
	return ok, nil
}

func (s *qaServer) n(prefix string) int {
	c := 0
	for _, x := range s.calls {
		if strings.HasPrefix(x, prefix) {
			c++
		}
	}
	return c
}

func qaMarker(kind, slot string) string {
	return protocol.Format("human", protocol.Meta{Kind: kind, Slot: slot, Date: "2026-01-01", Mode: "affinity", Count: 30})
}

func TestQAAdminSeesOthersAndOnlyTouchesOwn(t *testing.T) {
	s := &qaServer{lists: []Playlist{
		{ID: "bobs", Name: "P Artist Radio 1", Owner: "bob", Comment: qaMarker("artistradio", "artistradio-1"), Changed: "2026-02-01T00:00:00Z"},
		{ID: "alices", Name: "P Artist Radio 1", Owner: "alice", Comment: qaMarker("artistradio", "artistradio-1"), Changed: "2026-03-01T00:00:00Z"},
	}}
	// admin sees both; admin has none of its own
	c := New(s.call, "admin")
	id, err := c.ensureID("artistradio", "artistradio-1", "P Artist Radio 1")
	if err != nil {
		t.Fatal(err)
	}
	if id == "bobs" || id == "alices" {
		t.Fatalf("admin took %s", id)
	}
	for _, l := range s.lists {
		if (l.ID == "bobs" || l.ID == "alices") && l.Name != "P Artist Radio 1" {
			t.Errorf("foreign playlist %s renamed to %q", l.ID, l.Name)
		}
	}
	// and a rename request must not rename foreign playlist either
	c2 := New(s.call, "admin")
	if _, err := c2.ensureID("artistradio", "artistradio-1", "Totally New"); err != nil {
		t.Fatal(err)
	}
	for _, l := range s.lists {
		if l.Owner != "admin" && l.Name != "P Artist Radio 1" {
			t.Errorf("foreign playlist renamed: %+v", l)
		}
	}
}

func TestQADuplicateMarkersMostRecentWinsOthersUntouched(t *testing.T) {
	m := qaMarker("loved", "loved")
	s := &qaServer{lists: []Playlist{
		{ID: "old", Name: "A", Owner: "bob", Comment: m, Changed: "2025-01-01T00:00:00Z"},
		{ID: "newest", Name: "B", Owner: "bob", Comment: m, Changed: "2026-05-01T10:00:00.000Z"},
		{ID: "mid", Name: "C", Owner: "bob", Comment: m, Changed: "2026-01-01T00:00:00Z"},
		{ID: "nodate", Name: "D", Owner: "bob", Comment: m},
	}}
	id, err := New(s.call, "bob").ensureID("loved", "loved", "Wanted")
	if err != nil || id != "newest" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	for _, l := range s.lists {
		if l.ID != "newest" && (l.Name == "Wanted") {
			t.Errorf("renamed the wrong duplicate %s", l.ID)
		}
	}
	if s.n("updatePlaylist") != 1 {
		t.Errorf("updatePlaylist calls: %d", s.n("updatePlaylist"))
	}
}

func TestQAUnparseableTimesTieKeepsFirstListed(t *testing.T) {
	m := qaMarker("loved", "loved")
	s := &qaServer{lists: []Playlist{
		{ID: "first", Name: "A", Owner: "bob", Comment: m, Changed: "garbage"},
		{ID: "second", Name: "B", Owner: "bob", Comment: m, Changed: "not a time"},
	}}
	id, _ := New(s.call, "bob").ensureID("loved", "loved", "A")
	if id != "first" {
		t.Errorf("got %s", id)
	}
	// a dated one beats undated ones
	s = &qaServer{lists: []Playlist{
		{ID: "undated", Name: "B", Owner: "bob", Comment: m},
		{ID: "dated", Name: "C", Owner: "bob", Comment: m, Changed: "2020-01-01T00:00:00Z"},
	}}
	// Neither carries the wanted name ("A"), so both sit in the same ranking
	// tier and recency decides.
	id, _ = New(s.call, "bob").ensureID("loved", "loved", "A")
	if id != "dated" {
		t.Errorf("got %s", id)
	}
}

func TestQAMarkerWithDifferentKindSameSlotOrSameName(t *testing.T) {
	// Same name, marker says another kind with the same slot string.
	s := &qaServer{lists: []Playlist{
		{ID: "x", Name: "Punk Radio", Owner: "bob", Comment: qaMarker("genreradio", "genreradio-1")},
	}}
	id, _ := New(s.call, "bob").ensureID("artistradio", "genreradio-1", "Punk Radio")
	if id == "x" {
		t.Error("kind mismatch adopted by name")
	}
	// Same kind, different slot, same name: not adopted.
	s = &qaServer{lists: []Playlist{
		{ID: "x", Name: "Punk Radio", Owner: "bob", Comment: qaMarker("genreradio", "genreradio-2")},
	}}
	id, _ = New(s.call, "bob").ensureID("genreradio", "genreradio-1", "Punk Radio")
	if id == "x" {
		t.Error("slot mismatch adopted by name")
	}
}

func TestQAGarbageComments(t *testing.T) {
	for _, comment := range []string{
		"", "   ", "nb1:", "nb1:a:b", "nb1:loved:loved:2026:affinity:x",
		"nb1:loved:loved:2026-01-01:affinity:30:extra", "NB1:loved:loved:d:m:3",
		"\n\n\n", strings.Repeat("x", 100000), "nb1:loved:loved:d:m:30\r", "  nb1:loved:loved:d:m:30  ",
	} {
		s := &qaServer{lists: []Playlist{{ID: "g", Name: "Your Loved Songs", Owner: "bob", Comment: comment}}}
		id, err := New(s.call, "bob").ensureID("loved", "loved", "Your Loved Songs")
		if err != nil || id != "g" {
			t.Errorf("comment %q: id=%q err=%v (should adopt by name)", truncateForLog(comment), id, err)
		}
	}
}

func truncateForLog(s string) string {
	if len(s) > 30 {
		return s[:30] + "..."
	}
	return s
}

func TestQAMarkerLineInsideALongerDescription(t *testing.T) {
	// A user who edited the description in a client and kept the marker line.
	c := "my notes\nnb1:loved:loved:2026-01-01:affinity:30\nmore notes"
	s := &qaServer{lists: []Playlist{{ID: "e", Name: "Renamed By User", Owner: "bob", Comment: c}}}
	id, _ := New(s.call, "bob").ensureID("loved", "loved", "Your Loved Songs")
	if id != "e" {
		t.Errorf("marker among user text not found: %s", id)
	}
}

func TestQARenameFailureStillReturnsIDAndDoesNotCreate(t *testing.T) {
	s := &qaServer{failRename: true, lists: []Playlist{
		{ID: "a", Name: "Old", Owner: "bob", Comment: qaMarker("decade", "decade-1990")},
	}}
	c := New(s.call, "bob")
	id, err := c.ensureID("decade", "decade-1990", "New")
	if err != nil || id != "a" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if s.n("createPlaylist") != 0 {
		t.Error("created on rename failure")
	}
}

func TestQAGetPlaylistsFailureCreatesNothing(t *testing.T) {
	s := &qaServer{failList: true}
	c := New(s.call, "bob")
	id, err := c.ensureID("loved", "loved", "X")
	if err == nil || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if s.n("createPlaylist") != 0 {
		t.Error("created a playlist although listing failed (risk of duplicates)")
	}
	// A retry on the SAME client after failure must re-fetch, not use nil cache.
	s.failList = false
	if _, err := c.ensureID("loved", "loved", "X"); err != nil {
		t.Errorf("retry failed: %v", err)
	}
}

func TestQACreateFailure(t *testing.T) {
	s := &qaServer{failCreate: true}
	if id, err := New(s.call, "bob").ensureID("loved", "loved", "X"); err == nil {
		t.Errorf("expected error, id=%q", id)
	}
}

func TestQASpecialCharactersInIDsAndNames(t *testing.T) {
	weird := "id/with space&and=eq%25?#+é"
	name := "P Rock & Roll = 100% \"q\" 'x' \U0001F3B5\nline"
	s := &qaServer{lists: []Playlist{
		{ID: weird, Name: "old", Owner: "bob", Comment: qaMarker("genreradio", "genreradio-1")},
	}}
	id, err := New(s.call, "bob").ensureID("genreradio", "genreradio-1", name)
	if err != nil || id != weird {
		t.Fatalf("id=%q err=%v", id, err)
	}
	var last string
	for _, c := range s.calls {
		if strings.HasPrefix(c, "updatePlaylist") {
			last = c
		}
	}
	q, _ := url.ParseQuery(strings.TrimPrefix(last, "updatePlaylist?"))
	if q.Get("playlistId") != weird || q.Get("name") != name {
		t.Errorf("round trip failed: %q %q", q.Get("playlistId"), q.Get("name"))
	}
}

func TestQANeverCallsDelete(t *testing.T) {
	s := &qaServer{lists: []Playlist{
		{ID: "a", Name: "Old", Owner: "bob", Comment: qaMarker("loved", "loved"), Changed: "2026-01-01T00:00:00Z"},
		{ID: "b", Name: "Old2", Owner: "bob", Comment: qaMarker("loved", "loved"), Changed: "2026-02-01T00:00:00Z"},
	}}
	c := New(s.call, "bob")
	c.ensureID("loved", "loved", "New")
	c.ReplaceTracks("b", []string{"1", "2", "3"})
	c.SetComment("b", "x")
	if s.deleteCalls != 0 {
		t.Errorf("delete called %d times", s.deleteCalls)
	}
	if len(s.lists) != 2 {
		t.Errorf("playlist count changed to %d", len(s.lists))
	}
}

// ensureID is EnsureMix without the created flag, which these tests do not need.
func (c *Client) ensureID(kind, slot, name string) (string, error) {
	id, _, err := c.EnsureMix(kind, slot, name)
	return id, err
}
