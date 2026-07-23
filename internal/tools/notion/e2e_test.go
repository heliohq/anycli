//go:build e2e

// Real-API e2e for the notion service (design 008 D4) — full command-surface
// coverage in four closed-loop scenarios.
//
// The connected workspace is DEDICATED to e2e: every run creates artifacts
// named with the e2e.Prefix() run marker ("anycli-e2e-<runid>-") and leaves
// them behind as the run's dated record (no cleanup by agreement — the
// workspace exists for exactly this traffic; data-source update --in-trash
// is exercised as a round-trip, not as cleanup).
//
// Read-after-write note: Notion reads can lag writes (search indexing,
// title/property propagation, comment listing), so every assert-after-write
// goes through pollUntil (up to 5 tries, 2s apart) instead of a single read.
package notion_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/e2e"
)

const (
	pollTries    = 5
	pollInterval = 2 * time.Second
)

// TestE2EReadOnlySmoke covers the read-only surface: all four `user get`
// forms, `fetch self`, `search` (single-page and --all aggregation), the raw
// `api` passthrough, and the `task get` command path via a real not-found.
func TestE2EReadOnlySmoke(t *testing.T) {
	// (1) user get self: the connection's own bot user.
	out := mustRun(t, "user", "get", "self", "--json")
	var self struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	decodeJSON(t, out, &self)
	if self.ID == "" {
		t.Fatalf("user get self: empty id:\n%s", out)
	}
	if self.Type != "bot" {
		t.Fatalf("user get self: type = %q, want \"bot\":\n%s", self.Type, out)
	}
	botID := self.ID

	// (2) user get <id>: resolve the bot by its own id.
	out = mustRun(t, "user", "get", botID)
	var byID struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &byID)
	if byID.ID != botID {
		t.Fatalf("user get %s: id = %q, want %q:\n%s", botID, byID.ID, botID, out)
	}

	// (3) user get (no arg): paginated enumeration aggregated into one list
	// envelope. The bot itself is always listed, so results is never empty.
	out = mustRun(t, "user", "get")
	var users listEnvelope
	decodeJSON(t, out, &users)
	if users.Object != "list" || users.HasMore || users.NextCursor != nil {
		t.Fatalf("user get: want aggregated envelope object=list has_more=false next_cursor=null:\n%s", out)
	}
	if len(users.Results) == 0 {
		t.Fatalf("user get: empty results (the bot user must always be listed):\n%s", out)
	}

	// (4) user get --query: substring match against a name known to exist
	// (the bot's own), so the bot id must be among the matches.
	out = mustRun(t, "user", "get", "--query", self.Name)
	var queried listEnvelope
	decodeJSON(t, out, &queried)
	if queried.Object != "list" {
		t.Fatalf("user get --query: object = %q, want \"list\":\n%s", queried.Object, out)
	}
	if !hasResultID(queried, botID) {
		t.Fatalf("user get --query %q: bot id %s not in results:\n%s", self.Name, botID, out)
	}

	// (5) fetch self: the GET /users/me branch of fetch.
	out = mustRun(t, "fetch", "self")
	var fetched struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &fetched)
	if fetched.ID != botID {
		t.Fatalf("fetch self: id = %q, want the bot id %q:\n%s", fetched.ID, botID, out)
	}

	// (6) search: the dedicated workspace must share at least one page.
	out = mustRun(t, "search", "--query=", "--type", "page", "--json")
	if ids := pageIDsFromSearch(t, out); len(ids) == 0 {
		t.Fatal("search returned no pages — the e2e workspace must share at least one page with the connection")
	}

	// (7) search --all: the aggregation contract collapses pagination into a
	// single terminal envelope. Shape-only — results may be empty on a fresh
	// workspace.
	out = mustRun(t, "search", "--query", "anycli-e2e", "--all", "--page-size", "25")
	var agg listEnvelope
	decodeJSON(t, out, &agg)
	if agg.HasMore || agg.NextCursor != nil {
		t.Fatalf("search --all: want has_more=false next_cursor=null (aggregation contract):\n%s", out)
	}

	// (8) api: generic passthrough GET, always-JSON contract.
	out = mustRun(t, "api", "GET", "/users/me")
	var viaAPI struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &viaAPI)
	if viaAPI.ID != botID {
		t.Fatalf("api GET /users/me: id = %q, want the bot id %q:\n%s", viaAPI.ID, botID, out)
	}

	// (9) task get, negative path against the real /async_tasks/{id}
	// endpoint. Small e2e payloads complete synchronously and the write path
	// auto-polls to completion, so no queued task id is deterministically
	// observable — this not-found call still exercises auth, URL routing,
	// and response handling end to end. The not-found detail renders on
	// stderr (the harness captures stdout only), so the observable contract
	// here is the nonzero exit.
	out, exit := e2e.RunTool(t, "notion", "", "task", "get", "00000000-0000-0000-0000-000000000000")
	if exit == 0 {
		t.Fatalf("task get on a nonexistent id: exit = 0, want nonzero; output:\n%s", out)
	}
}

// TestE2EPageLifecycle drives the page write surface end to end: create a
// dated page under an existing anchor, read both faces back, walk every
// content-edit alias plus the canonical update, comment and reply, then
// duplicate and move. The page and its duplicate stay behind as the run's
// dated record (workspace agreement, no cleanup).
func TestE2EPageLifecycle(t *testing.T) {
	name := e2e.Prefix() + "page-" + time.Now().UTC().Format("20060102-150405")

	// (1) Anchor: any page the connection can see.
	anchor := anchorPageID(t)

	// (2) Create a dated page with a seed body line.
	pages := fmt.Sprintf(
		`[{"parent":{"page_id":%q},"properties":{"title":[{"text":{"content":%q}}]},"content":"seed-line"}]`,
		anchor, name)
	out := mustRun(t, "page", "create", "--pages", pages, "--json")
	pageID := createdPageID(t, out)

	// (3) fetch --json returns the page-markdown envelope: the BODY only —
	// no title (hard contract; titles live in the raw page JSON, step 4).
	out = mustRun(t, "fetch", pageID, "--json")
	var pm struct {
		Object   string `json:"object"`
		Markdown string `json:"markdown"`
	}
	decodeJSON(t, out, &pm)
	if pm.Object != "page_markdown" {
		t.Fatalf("fetch --json: object = %q, want \"page_markdown\":\n%s", pm.Object, out)
	}
	if !strings.Contains(pm.Markdown, "seed-line") {
		t.Fatalf("fetch --json: markdown missing seed-line:\n%s", out)
	}

	// (4) The title is only visible in the raw page JSON.
	pollUntil(t, "created title visible in raw page JSON", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "api", "GET", "/pages/"+pageID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, name) {
			return false, fmt.Sprintf("title %q not in page JSON yet:\n%s", name, out)
		}
		return true, ""
	})

	// (5) append: stdout is the post-update body markdown.
	out = mustRun(t, "page", "append", pageID, "--content", "marker-append")
	if !strings.Contains(out, "marker-append") {
		t.Fatalf("page append: marker-append missing from post-update markdown:\n%s", out)
	}

	// (6) insert at start: the new line must land before the seed line.
	out = mustRun(t, "page", "insert", pageID, "--content", "marker-top", "--at", "start")
	top, seed := strings.Index(out, "marker-top"), strings.Index(out, "seed-line")
	if top < 0 || seed < 0 || top >= seed {
		t.Fatalf("page insert --at start: want marker-top before seed-line (top=%d seed=%d):\n%s", top, seed, out)
	}

	// (7) edit: search-and-replace via the --old/--new alias.
	out = mustRun(t, "page", "edit", pageID, "--old", "marker-append", "--new", "marker-edited")
	if !strings.Contains(out, "marker-edited") || strings.Contains(out, "marker-append") {
		t.Fatalf("page edit: want marker-edited and no marker-append:\n%s", out)
	}

	// (8a) Canonical content update via --command update_content.
	mustRun(t, "page", "update", pageID, "--command", "update_content",
		"--content-updates", `[{"old_str":"marker-top","new_str":"marker-updated"}]`,
		"--allow-deleting-content", "--json")
	pollUntil(t, "update_content visible via bare fetch", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "fetch", pageID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, "marker-updated") {
			return false, "marker-updated not in body yet:\n" + out
		}
		return true, ""
	})

	// (8b) Properties-only update: rename plus an emoji icon.
	renamed := name + "-renamed"
	mustRun(t, "page", "update", pageID,
		"--properties", fmt.Sprintf(`{"title":[{"text":{"content":%q}}]}`, renamed),
		"--icon", "🧪")
	pollUntil(t, "renamed title and icon visible in raw page JSON", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "api", "GET", "/pages/"+pageID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, renamed) || !strings.Contains(out, `"emoji":"🧪"`) {
			return false, fmt.Sprintf("renamed title %q and emoji icon not both visible yet:\n%s", renamed, out)
		}
		return true, ""
	})

	// (9) replace: full-body replacement wipes everything else.
	out = mustRun(t, "page", "replace", pageID, "--new-str", "final-body", "--allow-deleting-content")
	if !strings.Contains(out, "final-body") {
		t.Fatalf("page replace: final-body missing from post-update markdown:\n%s", out)
	}
	pollUntil(t, "replaced body visible via bare fetch", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "fetch", pageID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, "final-body") || strings.Contains(out, "seed-line") {
			return false, "body not yet fully replaced:\n" + out
		}
		return true, ""
	})

	// (10) Comment on the page (output is the JSON envelope verbatim).
	out = mustRun(t, "comment", "create", "--page-id", pageID, "--content", name+"-comment-1")
	var c1 struct {
		ID           string `json:"id"`
		DiscussionID string `json:"discussion_id"`
	}
	decodeJSON(t, out, &c1)
	if c1.ID == "" {
		t.Fatalf("comment create: empty comment id:\n%s", out)
	}
	if c1.DiscussionID == "" {
		t.Fatalf("comment create: empty discussion_id:\n%s", out)
	}

	// (11) Reply branch: same discussion thread.
	out = mustRun(t, "comment", "create", "--discussion-id", c1.DiscussionID, "--content", name+"-reply-1")
	var c2 struct {
		ID           string `json:"id"`
		DiscussionID string `json:"discussion_id"`
	}
	decodeJSON(t, out, &c2)
	if c2.ID == "" || c2.DiscussionID != c1.DiscussionID {
		t.Fatalf("comment reply: want a new comment in discussion %s, got id=%q discussion=%q:\n%s",
			c1.DiscussionID, c2.ID, c2.DiscussionID, out)
	}

	// (12) List both comments back.
	pollUntil(t, "comment and reply listed", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "comment", "list", pageID, "--all")
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, name+"-comment-1") || !strings.Contains(out, name+"-reply-1") {
			return false, "comment and reply not both listed yet:\n" + out
		}
		var env listEnvelope
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			return false, err.Error()
		}
		if env.Object != "list" || env.HasMore {
			return false, "want a terminal list envelope (object=list, has_more=false):\n" + out
		}
		return true, ""
	})

	// (13) Duplicate via the template endpoint.
	out = mustRun(t, "page", "duplicate", pageID, "--title", name+"-copy", "--json")
	var dup struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &dup)
	if dup.ID == "" {
		t.Fatalf("page duplicate: empty id:\n%s", out)
	}
	pollUntil(t, "duplicate title visible in raw page JSON", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "api", "GET", "/pages/"+dup.ID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, name+"-copy") {
			return false, fmt.Sprintf("title %q not in duplicate JSON yet:\n%s", name+"-copy", out)
		}
		return true, ""
	})

	// (14) Move the duplicate under the page. Bare stdout is the moved id;
	// Notion returns dashed UUIDs, so compare dash-stripped.
	out = mustRun(t, "page", "move",
		"--page-or-database-ids", fmt.Sprintf(`[%q]`, dup.ID), "--new-parent", pageID)
	if stripDashes(strings.TrimSpace(out)) != stripDashes(dup.ID) {
		t.Fatalf("page move: stdout = %q, want the moved id %q", strings.TrimSpace(out), dup.ID)
	}
	pollUntil(t, "duplicate reparented under the page", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "api", "GET", "/pages/"+dup.ID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		var pg struct {
			Parent struct {
				PageID string `json:"page_id"`
			} `json:"parent"`
		}
		if err := json.Unmarshal([]byte(out), &pg); err != nil {
			return false, err.Error()
		}
		if stripDashes(pg.Parent.PageID) != stripDashes(pageID) {
			return false, fmt.Sprintf("parent.page_id = %q, want %q", pg.Parent.PageID, pageID)
		}
		return true, ""
	})
}

// TestE2EDatabaseLifecycle drives the database surface: create a container,
// fetch both faces, seed rows, query with filter/sorts and all three
// pagination modes, patch the schema, create and update a view, and
// round-trip --in-trash. The database stays behind as the run's record.
func TestE2EDatabaseLifecycle(t *testing.T) {
	prefix := e2e.Prefix()

	// (1) Anchor page for the database's parent.
	anchor := anchorPageID(t)

	// (2) Create the database container with an initial schema.
	out := mustRun(t, "db", "create", "--parent", anchor, "--title", prefix+"db",
		"--properties", `{"Name":{"title":{}},"Score":{"number":{}}}`)
	var db struct {
		Object      string `json:"object"`
		ID          string `json:"id"`
		DataSources []struct {
			ID string `json:"id"`
		} `json:"data_sources"`
	}
	decodeJSON(t, out, &db)
	if db.Object != "database" {
		t.Fatalf("db create: object = %q, want \"database\":\n%s", db.Object, out)
	}
	if len(db.DataSources) == 0 || db.DataSources[0].ID == "" {
		t.Fatalf("db create: response carries no data_sources — cannot derive the data-source id:\n%s", out)
	}
	dbID, dsID := db.ID, db.DataSources[0].ID

	// (3) fetch the container face.
	out = mustRun(t, "fetch", dbID, "--type", "database")
	var fetchedDB struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &fetchedDB)
	if stripDashes(fetchedDB.ID) != stripDashes(dbID) {
		t.Fatalf("fetch --type database: id = %q, want %q:\n%s", fetchedDB.ID, dbID, out)
	}
	if !strings.Contains(out, prefix+"db") {
		t.Fatalf("fetch --type database: title %q not in response:\n%s", prefix+"db", out)
	}

	// (4) fetch the data-source face: the schema must carry Score.
	out = mustRun(t, "fetch", dsID, "--type", "data_source")
	if !strings.Contains(out, `"Score"`) {
		t.Fatalf("fetch --type data_source: property \"Score\" not in schema:\n%s", out)
	}

	// (5) Seed 3 rows in one fan-out create.
	rows := make([]string, 0, 3)
	for i := 1; i <= 3; i++ {
		rows = append(rows, fmt.Sprintf(
			`{"parent":{"data_source_id":%q},"properties":{"Name":{"title":[{"text":{"content":"%srow-%d"}}]},"Score":{"number":%d}}}`,
			dsID, prefix, i, i))
	}
	out = mustRun(t, "page", "create", "--pages", "["+strings.Join(rows, ",")+"]", "--json")
	if ids := createdPageIDs(t, out); len(ids) != 3 {
		t.Fatalf("page create fan-out: created %d rows, want 3:\n%s", len(ids), out)
	}

	// (6) All three rows visible (indexing lag → poll).
	pollUntil(t, "seeded rows visible in data-source query", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "data-source", "query", dsID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		var env listEnvelope
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			return false, err.Error()
		}
		if env.Object != "list" || len(env.Results) != 3 {
			return false, fmt.Sprintf("object=%q len(results)=%d, want list with 3 rows:\n%s", env.Object, len(env.Results), out)
		}
		return true, ""
	})

	// (7) Filter + sorts: Score > 1 descending → [3, 2].
	out = mustRun(t, "data-source", "query", dsID,
		"--filter", `{"property":"Score","number":{"greater_than":1}}`,
		"--sorts", `[{"property":"Score","direction":"descending"}]`)
	var filtered struct {
		Results []struct {
			ID         string `json:"id"`
			Properties struct {
				Score struct {
					Number float64 `json:"number"`
				} `json:"Score"`
			} `json:"properties"`
		} `json:"results"`
	}
	decodeJSON(t, out, &filtered)
	if len(filtered.Results) != 2 {
		t.Fatalf("filtered query: %d results, want 2:\n%s", len(filtered.Results), out)
	}
	if filtered.Results[0].Properties.Score.Number != 3 {
		t.Fatalf("sorted query: results[0].Score = %v, want 3 (descending):\n%s",
			filtered.Results[0].Properties.Score.Number, out)
	}

	// (8) Pagination triplet: manual first page, cursor continuation, --all.
	out = mustRun(t, "data-source", "query", dsID, "--page-size", "1")
	var page1 listEnvelope
	decodeJSON(t, out, &page1)
	if len(page1.Results) != 1 || !page1.HasMore || page1.NextCursor == nil || *page1.NextCursor == "" {
		t.Fatalf("query --page-size 1: want 1 result, has_more=true, non-null next_cursor:\n%s", out)
	}
	out = mustRun(t, "data-source", "query", dsID, "--page-size", "1", "--start-cursor", *page1.NextCursor)
	var page2 listEnvelope
	decodeJSON(t, out, &page2)
	if len(page2.Results) == 0 || page2.Results[0].ID == page1.Results[0].ID {
		t.Fatalf("query --start-cursor: second page must return a different row than the first:\n%s", out)
	}
	out = mustRun(t, "data-source", "query", dsID, "--all", "--page-size", "1")
	var all listEnvelope
	decodeJSON(t, out, &all)
	if len(all.Results) != 3 || all.HasMore || all.NextCursor != nil {
		t.Fatalf("query --all: want 3 aggregated results, has_more=false, next_cursor=null:\n%s", out)
	}

	// (9) Schema patch: rename plus a new Tag property.
	out = mustRun(t, "data-source", "update", dsID,
		"--name", prefix+"db-renamed", "--properties", `{"Tag":{"rich_text":{}}}`)
	if !strings.Contains(out, "db-renamed") || !strings.Contains(out, `"Tag"`) {
		t.Fatalf("data-source update: renamed title and Tag property not both in PATCH response:\n%s", out)
	}

	// (10) Create a table view over the data source.
	out = mustRun(t, "view", "create", "--database-id", dbID, "--data-source-id", dsID,
		"--name", prefix+"view", "--type", "table")
	var view struct {
		ID string `json:"id"`
	}
	decodeJSON(t, out, &view)
	if view.ID == "" {
		t.Fatalf("view create: empty view id:\n%s", out)
	}

	// (11) Rename the view and set sorts.
	out = mustRun(t, "view", "update", view.ID, "--name", prefix+"view-renamed",
		"--sorts", `[{"property":"Score","direction":"ascending"}]`)
	if !strings.Contains(out, prefix+"view-renamed") {
		t.Fatalf("view update: renamed name %q not in response:\n%s", prefix+"view-renamed", out)
	}

	// (12) --in-trash round-trip, leaving the dated database live as the
	// run's record.
	out = mustRun(t, "data-source", "update", dsID, "--in-trash", "true")
	if !strings.Contains(out, `"in_trash":true`) && !strings.Contains(out, `"archived":true`) {
		t.Fatalf("data-source update --in-trash true: response does not show a trashed state:\n%s", out)
	}
	out = mustRun(t, "data-source", "update", dsID, "--in-trash", "false")
	if !strings.Contains(out, `"in_trash":false`) && !strings.Contains(out, `"archived":false`) {
		t.Fatalf("data-source update --in-trash false: response does not show a restored state:\n%s", out)
	}
}

// TestE2EFileAttachLifecycle drives the file surface: upload a local fixture
// to Notion-managed storage, then attach it (and an external URL) to a files
// property. A files property only exists on database rows, so the scenario
// seeds its own database and row. Artifacts remain as dated records.
func TestE2EFileAttachLifecycle(t *testing.T) {
	prefix := e2e.Prefix()

	// (1) Local fixture file.
	path := filepath.Join(t.TempDir(), prefix+"note.txt")
	if err := os.WriteFile(path, []byte("anycli e2e attachment body"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// (2) Upload (create + single-part send); --json emits the send response.
	out := mustRun(t, "file", "upload", path,
		"--name", prefix+"note.txt", "--content-type", "text/plain", "--json")
	var up struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decodeJSON(t, out, &up)
	if up.ID == "" {
		t.Fatalf("file upload: empty file_upload id:\n%s", out)
	}
	if up.Status != "uploaded" {
		t.Fatalf("file upload: status = %q, want \"uploaded\":\n%s", up.Status, out)
	}

	// (3) Seed a database with a files property.
	anchor := anchorPageID(t)
	out = mustRun(t, "db", "create", "--parent", anchor, "--title", prefix+"files-db",
		"--properties", `{"Name":{"title":{}},"Attachment":{"files":{}}}`)
	var db struct {
		DataSources []struct {
			ID string `json:"id"`
		} `json:"data_sources"`
	}
	decodeJSON(t, out, &db)
	if len(db.DataSources) == 0 || db.DataSources[0].ID == "" {
		t.Fatalf("db create: response carries no data_sources — cannot derive the data-source id:\n%s", out)
	}
	dsID := db.DataSources[0].ID

	// (4) One row to attach onto.
	rowJSON := fmt.Sprintf(
		`[{"parent":{"data_source_id":%q},"properties":{"Name":{"title":[{"text":{"content":"%sfile-row"}}]}}}]`,
		dsID, prefix)
	out = mustRun(t, "page", "create", "--pages", rowJSON, "--json")
	rowID := createdPageID(t, out)

	// (5) Upload branch: attach the file_upload to the files property.
	out = mustRun(t, "file", "attach", rowID, "--property", "Attachment",
		"--upload-id", up.ID, "--json")
	files := attachmentFiles(t, out)
	if len(files) != 1 || files[0].Name != prefix+"note.txt" {
		t.Fatalf("file attach --upload-id: want exactly one file named %q:\n%s", prefix+"note.txt", out)
	}

	// (6) External branch — attach replaces the property value wholesale, so
	// the list still holds exactly one (external) file.
	const extURL = "https://www.notion.so/images/favicon.ico"
	out = mustRun(t, "file", "attach", rowID, "--property", "Attachment",
		"--external-url", extURL, "--name", prefix+"ext.ico", "--json")
	files = attachmentFiles(t, out)
	if len(files) != 1 {
		t.Fatalf("file attach --external-url: %d files, want 1 (replace semantics):\n%s", len(files), out)
	}
	if files[0].Type != "external" || files[0].External.URL != extURL || files[0].Name != prefix+"ext.ico" {
		t.Fatalf("file attach --external-url: got type=%q url=%q name=%q, want external/%s/%s:\n%s",
			files[0].Type, files[0].External.URL, files[0].Name, extURL, prefix+"ext.ico", out)
	}

	// (7) Persisted state via the raw page JSON.
	pollUntil(t, "external attachment persisted on the row", func() (bool, string) {
		out, exit := e2e.RunTool(t, "notion", "", "api", "GET", "/pages/"+rowID)
		if exit != 0 {
			return false, fmt.Sprintf("exit %d:\n%s", exit, out)
		}
		if !strings.Contains(out, prefix+"ext.ico") || !strings.Contains(out, extURL) {
			return false, "attachment name/URL not yet visible in raw page JSON:\n" + out
		}
		return true, ""
	})
}

// listEnvelope is the Notion list envelope shape shared by user get, search,
// comment list, and data-source query outputs.
type listEnvelope struct {
	Object  string `json:"object"`
	Results []struct {
		ID string `json:"id"`
	} `json:"results"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}

// hasResultID reports whether the envelope's results carry the given id.
func hasResultID(env listEnvelope, id string) bool {
	for _, r := range env.Results {
		if r.ID == id {
			return true
		}
	}
	return false
}

// mustRun executes one notion invocation and requires exit 0.
func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, exit := e2e.RunTool(t, "notion", "", args...)
	if exit != 0 {
		t.Fatalf("notion %s: exit = %d, output:\n%s", strings.Join(args, " "), exit, out)
	}
	return out
}

// decodeJSON unmarshals out into v, failing the test on malformed output.
func decodeJSON(t *testing.T, out string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), v); err != nil {
		t.Fatalf("output is not the expected JSON: %v\n%s", err, out)
	}
}

// pollUntil retries check up to pollTries times, pollInterval apart, to
// absorb Notion's read-after-write lag. check returns ok plus a detail
// string used in the failure message once retries are exhausted.
func pollUntil(t *testing.T, desc string, check func() (bool, string)) {
	t.Helper()
	detail := ""
	for i := 0; i < pollTries; i++ {
		var ok bool
		if ok, detail = check(); ok {
			return
		}
		if i < pollTries-1 {
			time.Sleep(pollInterval)
		}
	}
	t.Fatalf("%s: still failing after %d tries: %s", desc, pollTries, detail)
}

// stripDashes normalizes a Notion id for comparison: the API returns dashed
// UUIDs while inputs may be dashless.
func stripDashes(id string) string { return strings.ReplaceAll(id, "-", "") }

// anchorPageID returns any page the connection can see, used as the parent
// for created test artifacts. An empty search query lists everything
// accessible (the --query= form satisfies the required flag).
func anchorPageID(t *testing.T) string {
	t.Helper()
	out := mustRun(t, "search", "--query=", "--type", "page", "--json")
	ids := pageIDsFromSearch(t, out)
	if len(ids) == 0 {
		t.Fatal("no anchor page — share at least one page with the e2e connection")
	}
	return ids[0]
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

// createdPageIDs pulls every new page id out of page create --json output
// ({"pages": [<page object>, ...]}).
func createdPageIDs(t *testing.T, out string) []string {
	t.Helper()
	var v struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse page create output: %v\n%s", err, out)
	}
	ids := make([]string, 0, len(v.Pages))
	for _, p := range v.Pages {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// createdPageID pulls the single new page's id out of page create --json
// output.
func createdPageID(t *testing.T, out string) string {
	t.Helper()
	ids := createdPageIDs(t, out)
	if len(ids) == 0 {
		t.Fatalf("no page id in create output:\n%s", out)
	}
	return ids[0]
}

// attachedFile is one entry of a files property value in a PATCH response.
type attachedFile struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	External struct {
		URL string `json:"url"`
	} `json:"external"`
}

// attachmentFiles pulls the Attachment files array out of a file attach
// --json PATCH response.
func attachmentFiles(t *testing.T, out string) []attachedFile {
	t.Helper()
	var resp struct {
		Properties struct {
			Attachment struct {
				Files []attachedFile `json:"files"`
			} `json:"Attachment"`
		} `json:"properties"`
	}
	decodeJSON(t, out, &resp)
	return resp.Properties.Attachment.Files
}
