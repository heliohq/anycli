//go:build e2e

// Real-API e2e for the bitly service (design 008 D4).
//
// Bitly's API has no link deletion and the free tier caps new links per
// month, so the write loop is REVERSIBLE-UPDATE-ONLY by design: it renames
// an existing bitlink's title and restores the original afterwards. No new
// links are created, no quota is consumed, nothing is left behind.
package bitly_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EUserGet(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "user", "get", "--json")
	if exit != 0 {
		t.Fatalf("user get exit = %d, output:\n%s", exit, out)
	}
	assertNonEmptyJSON(t, out)
}

func TestE2EGroupListSmoke(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "group", "list", "--json")
	if exit != 0 {
		t.Fatalf("group list exit = %d, output:\n%s", exit, out)
	}
	assertNonEmptyJSON(t, out)
}

// TestE2ELinkTitleClosedLoop renames an existing bitlink and restores it:
// list → pick first link → set title to a run-tagged marker → verify via
// get → restore the original title → verify restored.
func TestE2ELinkTitleClosedLoop(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "link", "list", "--json")
	if exit != 0 {
		t.Fatalf("link list exit = %d, output:\n%s", exit, out)
	}
	id, originalTitle := firstLink(t, out)
	if id == "" {
		t.Skip("no bitlinks in the account — seed one link to enable the closed loop")
	}

	marker := e2e.Prefix() + "title-probe"

	// Rename.
	out, exit = e2e.RunTool(t, "bitly", "", "link", "update", "--bitlink", id, "--title", marker, "--json")
	if exit != 0 {
		t.Fatalf("link update exit = %d, output:\n%s", exit, out)
	}
	out, exit = e2e.RunTool(t, "bitly", "", "link", "get", "--bitlink", id, "--json")
	if exit != 0 {
		t.Fatalf("link get exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, marker) {
		t.Fatalf("renamed title %q not visible:\n%s", marker, out)
	}

	// Restore. Best-effort even if the verify above failed the test — but
	// since t.Fatalf stops us before this point on failure, an interrupted
	// loop leaves the marker title behind, which is exactly the visible,
	// prefix-tagged residue design 008 D4 calls for.
	out, exit = e2e.RunTool(t, "bitly", "", "link", "update", "--bitlink", id, "--title", originalTitle, "--json")
	if exit != 0 {
		t.Fatalf("restore title exit = %d, output:\n%s", exit, out)
	}
	out, exit = e2e.RunTool(t, "bitly", "", "link", "get", "--bitlink", id, "--json")
	if exit != 0 {
		t.Fatalf("link get after restore exit = %d, output:\n%s", exit, out)
	}
	if strings.Contains(out, marker) {
		t.Fatalf("marker title still present after restore:\n%s", out)
	}
}

func assertNonEmptyJSON(t *testing.T, out string) {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(v) == 0 {
		t.Fatal("empty JSON output")
	}
}

// firstLink pulls the first bitlink's id and title from link list --json
// output ({"links": [{"id": "bit.ly/x", "title": ...}, ...]}).
func firstLink(t *testing.T, out string) (id, title string) {
	t.Helper()
	var v struct {
		Links []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse link list output: %v\n%s", err, out)
	}
	if len(v.Links) == 0 {
		return "", ""
	}
	return v.Links[0].ID, v.Links[0].Title
}
