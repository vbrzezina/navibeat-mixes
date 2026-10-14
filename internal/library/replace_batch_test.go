package library

import (
	"fmt"
	"strings"
	"testing"
)

func ids(n, width int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%0*d", width, i)
	}
	return out
}

// The biggest mix a person can ask for (200 tracks of normal 22 character
// ids) must still go out as the single replace it always was.
func TestTwoHundredNormalIDsStayOneCreatePlaylistCall(t *testing.T) {
	s := &playlistServer{}
	want := ids(200, 22)
	if err := New(s.call, "alice").ReplaceTracks("pl1", want); err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.calls, ",") != "createPlaylist" {
		t.Fatalf("calls = %v, want a single createPlaylist", s.calls)
	}
	if len(s.rows) != 200 {
		t.Fatalf("rows = %d, want 200", len(s.rows))
	}
}

// Unusually long ids would push the request line past what proxies accept, so
// the tail goes in follow-up calls and the order is kept.
func TestVeryLongIDsAreSplitAcrossCallsInOrder(t *testing.T) {
	s := &playlistServer{rows: []string{"old"}}
	want := ids(200, 60)
	if err := New(s.call, "alice").ReplaceTracks("pl1", want); err != nil {
		t.Fatal(err)
	}
	if s.calls[0] != "createPlaylist" || len(s.calls) < 2 {
		t.Fatalf("calls = %v, want createPlaylist then updatePlaylist", s.calls)
	}
	for _, c := range s.calls[1:] {
		if c != "updatePlaylist" {
			t.Fatalf("calls = %v, want only updatePlaylist after the first", s.calls)
		}
	}
	if strings.Join(s.rows, ",") != strings.Join(want, ",") {
		t.Fatalf("rows differ from the requested ids or their order (got %d)", len(s.rows))
	}
}
