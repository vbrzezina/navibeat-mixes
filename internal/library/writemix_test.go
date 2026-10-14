package library

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// opServer records the order of the writes WriteMix makes and can fail the
// description write.
type opServer struct {
	ops         []string
	failComment int // number of comment writes to fail
}

func (s *opServer) call(uri string) (string, error) {
	endpoint, query, _ := strings.Cut(uri, "?")
	p, _ := url.ParseQuery(query)
	switch {
	case endpoint == "updatePlaylist" && p.Get("comment") != "":
		s.ops = append(s.ops, "comment")
		if s.failComment > 0 {
			s.failComment--
			return "", errors.New("boom")
		}
	case endpoint == "createPlaylist" && p.Get("playlistId") != "":
		s.ops = append(s.ops, "tracks")
	default:
		s.ops = append(s.ops, endpoint)
	}
	return `{"subsonic-response":{"status":"ok"}}`, nil
}

func tenIDs() []string { return ids(10, 22) }

// F7: a just created playlist gets its marker BEFORE the tracks.
func TestWriteMixWritesTheMarkerFirstForACreatedPlaylist(t *testing.T) {
	s := &opServer{}
	if err := New(s.call, "bob").WriteMix("p", true, tenIDs(), "c"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.ops, ","); got != "comment,tracks" {
		t.Errorf("order = %s", got)
	}
}

// F7: if the marker cannot be written the slot fails and no tracks are sent,
// so the retry finds nothing half done. One immediate retry is allowed.
func TestWriteMixFailsACreatedPlaylistWhoseMarkerCannotBeWritten(t *testing.T) {
	s := &opServer{failComment: 2}
	if err := New(s.call, "bob").WriteMix("p", true, tenIDs(), "c"); err == nil {
		t.Fatal("expected an error")
	}
	if got := strings.Join(s.ops, ","); got != "comment,comment" {
		t.Errorf("ops = %s, want two comment attempts and no tracks", got)
	}
	s = &opServer{failComment: 1}
	if err := New(s.call, "bob").WriteMix("p", true, tenIDs(), "c"); err != nil {
		t.Fatalf("one transient failure should be retried: %v", err)
	}
}

// F7: an existing playlist keeps today's order, and a failed description is
// only logged.
func TestWriteMixKeepsTodaysOrderAndLeniencyForAnExistingPlaylist(t *testing.T) {
	s := &opServer{failComment: 1}
	if err := New(s.call, "bob").WriteMix("p", false, tenIDs(), "c"); err != nil {
		t.Fatalf("a failed description on an existing playlist must not fail the slot: %v", err)
	}
	if got := strings.Join(s.ops, ","); got != "tracks,comment" {
		t.Errorf("order = %s", got)
	}
}
