package library

import (
	"errors"
	"strings"
	"testing"
)

// F8: the budget counts the key the call really uses. The first call sends
// "songId=" (8 bytes of key plus "=" and "&" with the id), later ones
// "songIdToAdd=".
func TestSplitIDsBoundaryIsExactAndInclusive(t *testing.T) {
	five := ids(5, 12) // each costs len("songId")+2+12 = 20
	head, tail := splitIDs(five, 100, "songId")
	if len(head) != 5 || len(tail) != 0 {
		t.Errorf("budget exactly full: head=%d tail=%d, want 5/0", len(head), len(tail))
	}
	head, tail = splitIDs(five, 99, "songId")
	if len(head) != 4 || len(tail) != 1 {
		t.Errorf("budget-1: head=%d tail=%d, want 4/1", len(head), len(tail))
	}
	// The same ids cost 5 bytes more each under the follow-up key.
	head, _ = splitIDs(five, 100, "songIdToAdd")
	if len(head) != 4 { // 25 each: 4 fit in 100
		t.Errorf("songIdToAdd: head=%d, want 4", len(head))
	}
}

// The largest normal mix is 6000 bytes and the real first-call key makes it
// one call with room to spare (it used to be counted as 7000, on the line).
func TestSplitIDsTwoHundredNormalIDsFitWithHeadroom(t *testing.T) {
	head, tail := splitIDs(ids(200, 22), maxSongIDQuery, "songId")
	if len(head) != 200 || len(tail) != 0 {
		t.Fatalf("head=%d tail=%d", len(head), len(tail))
	}
	// 233 ids of 22 characters fill 6990 of 7000; the 234th does not fit.
	head, _ = splitIDs(ids(300, 22), maxSongIDQuery, "songId")
	if len(head) != 233 {
		t.Errorf("head = %d, want 233", len(head))
	}
}

// Ids grow when URL encoded. Counting the raw length would let this through.
func TestSplitIDsCountsTheEncodedLength(t *testing.T) {
	for _, tc := range []struct {
		id       string
		encoded  int
		perCall  int // fit in a budget of 3*(8+encoded)-1
		fitsRaw  bool
		describe string
	}{
		{"+/%", 9, 2, true, "plus, slash, percent"},
		{"a b", 3, 2, false, "space becomes plus, no growth"},
		{"éé", 12, 2, true, "two byte characters become six bytes each"},
	} {
		list := []string{tc.id, tc.id, tc.id}
		budget := 3*(len("songId")+2+tc.encoded) - 1
		head, tail := splitIDs(list, budget, "songId")
		if len(head) != tc.perCall || len(tail) != 3-tc.perCall {
			t.Errorf("%s: head=%d tail=%d, want %d", tc.describe, len(head), len(tail), tc.perCall)
		}
	}
}

// An id longer than the whole budget is never dropped: it goes alone.
func TestSplitIDsOversizedIDGoesAlone(t *testing.T) {
	big := strings.Repeat("x", 50)
	head, tail := splitIDs([]string{big, "a", "b"}, 10, "songId")
	if len(head) != 1 || head[0] != big || len(tail) != 2 {
		t.Errorf("head=%v tail=%v", head, tail)
	}
	head, tail = splitIDs([]string{"a", big}, 10, "songId")
	if len(head) != 1 || head[0] != "a" || len(tail) != 1 {
		t.Errorf("head=%v tail=%v", head, tail)
	}
}

// failingFollowUp is playlistServer whose updatePlaylist (the follow-up
// chunks) fails.
type failingFollowUp struct{ *playlistServer }

func (f failingFollowUp) call(uri string) (string, error) {
	if strings.HasPrefix(uri, "updatePlaylist") {
		f.calls = append(f.calls, "updatePlaylist")
		return "", errors.New("boom")
	}
	return f.playlistServer.call(uri)
}

// When a follow-up chunk fails the playlist is SHORTER than asked (never a mix
// of old and new) and the error is returned so the slot is retried.
func TestReplaceTracksFollowUpFailureLeavesAShorterPlaylistAndReturnsTheError(t *testing.T) {
	s := &playlistServer{rows: []string{"old1", "old2"}}
	want := ids(200, 60)
	err := New(failingFollowUp{s}.call, "alice").ReplaceTracks("pl1", want)
	if err == nil {
		t.Fatal("expected the follow-up failure to be returned")
	}
	first, _ := splitIDs(want, maxSongIDQuery, "songId")
	if len(s.rows) != len(first) || len(s.rows) >= len(want) {
		t.Fatalf("rows = %d, want only the first chunk (%d)", len(s.rows), len(first))
	}
	for _, r := range s.rows {
		if strings.HasPrefix(r, "old") {
			t.Fatalf("old rows survived: %v", s.rows)
		}
	}
}
