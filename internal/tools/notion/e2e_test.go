//go:build e2e

// Real-API e2e for the notion service (design 008 D4).
//
// The connected workspace is DEDICATED to e2e: every run creates a fresh
// top-level test page named anycli-e2e-<date> and leaves it as a dated
// record (no cleanup by agreement — the service exposes no page-trash
// command, and the workspace exists for exactly this traffic).
package notion_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/e2e"
)

func TestE2EUserSelf(t *testing.T) {
	out, exit := e2e.RunTool(t, "notion", "", "user", "get", "self", "--json")
	if exit != 0 {
		t.Fatalf("user get self exit = %d, output:\n%s", exit, out)
	}
	assertNonEmptyJSON(t, out)
}

func TestE2ESearchSmoke(t *testing.T) {
	out, exit := e2e.RunTool(t, "notion", "", "search", "--query=", "--type", "page", "--json")
	if exit != 0 {
		t.Fatalf("search exit = %d, output:\n%s", exit, out)
	}
	if ids := pageIDsFromSearch(t, out); len(ids) == 0 {
		t.Fatal("search returned no pages — the e2e workspace must share at least one page with the connection")
	}
}

// TestE2EPageClosedLoop drives the write surface end to end: create a dated
// page under an existing anchor page, read it back, append content, and
// comment on it. The page stays behind as the run's dated record.
func TestE2EPageClosedLoop(t *testing.T) {
	// Anchor: any page the connection can see. An empty search query lists
	// everything accessible (the --query= form satisfies the required flag).
	out, exit := e2e.RunTool(t, "notion", "", "search", "--query=", "--type", "page", "--json")
	if exit != 0 {
		t.Fatalf("anchor search exit = %d, output:\n%s", exit, out)
	}
	ids := pageIDsFromSearch(t, out)
	if len(ids) == 0 {
		t.Fatal("no anchor page — share at least one page with the e2e connection")
	}
	anchor := ids[0]

	name := "anycli-e2e-" + time.Now().UTC().Format("20060102-150405")
	pages := fmt.Sprintf(
		`[{"parent":{"page_id":%q},"properties":{"title":[{"text":{"content":%q}}]},"content":"Seed content created by anycli e2e."}]`,
		anchor, name)

	// Create.
	out, exit = e2e.RunTool(t, "notion", "", "page", "create", "--pages", pages, "--json")
	if exit != 0 {
		t.Fatalf("page create exit = %d, output:\n%s", exit, out)
	}
	pageID := createdPageID(t, out)

	// Read back: the title must be visible.
	out, exit = e2e.RunTool(t, "notion", "", "fetch", pageID)
	if exit != 0 {
		t.Fatalf("fetch exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, name) {
		t.Fatalf("fetched page does not contain title %q:\n%s", name, out)
	}

	// Update: append content, then verify it landed.
	marker := name + " updated-marker"
	out, exit = e2e.RunTool(t, "notion", "", "page", "update", pageID,
		"--command", "insert_content", "--content", marker)
	if exit != 0 {
		t.Fatalf("page update exit = %d, output:\n%s", exit, out)
	}
	out, exit = e2e.RunTool(t, "notion", "", "fetch", pageID)
	if exit != 0 {
		t.Fatalf("fetch after update exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, marker) {
		t.Fatalf("updated content %q not visible:\n%s", marker, out)
	}

	// Comment: create on the page, then list and verify.
	commentBody := name + " comment"
	out, exit = e2e.RunTool(t, "notion", "", "comment", "create",
		"--page-id", pageID, "--content", commentBody, "--json")
	if exit != 0 {
		t.Fatalf("comment create exit = %d, output:\n%s", exit, out)
	}
	out, exit = e2e.RunTool(t, "notion", "", "comment", "list", pageID, "--json")
	if exit != 0 {
		t.Fatalf("comment list exit = %d, output:\n%s", exit, out)
	}
	if !strings.Contains(out, commentBody) {
		t.Fatalf("comment %q not found in list:\n%s", commentBody, out)
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

// pageIDsFromSearch pulls page ids out of a search --json response
// ({"results": [{"id": ...}, ...]}).
func pageIDsFromSearch(t *testing.T, out string) []string {
	t.Helper()
	var v struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse search output: %v\n%s", err, out)
	}
	ids := make([]string, 0, len(v.Results))
	for _, r := range v.Results {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// createdPageID pulls the new page's id out of page create --json output
// ({"pages": [<page object>, ...]}).
func createdPageID(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse page create output: %v\n%s", err, out)
	}
	if len(v.Pages) == 0 || v.Pages[0].ID == "" {
		t.Fatalf("no page id in create output:\n%s", out)
	}
	return v.Pages[0].ID
}
