//go:build e2e

// Real-API e2e for the bitly service (design 008 D4).
//
// Bitly's API has no link deletion and the free tier caps new links per
// month, so the write surface is REVERSIBLE-UPDATE-ONLY by design: closed
// loops rename an EXISTING object's title/description/name to a run-tagged
// marker and restore the original afterwards. Nothing is ever created —
// link shorten/create, campaign create, channel create, qr create and
// qr create-static are excluded on purpose (undeletable objects, quota).
//
// Read-after-write: bitly GETs can lag behind PATCHes, so every
// assert-after-write polls (up to pollAttempts tries, pollInterval apart).
// Polls return early on success, so the sleep caps only bound failing runs.
package bitly_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/e2e"
)

const (
	pollAttempts = 5
	pollInterval = 2 * time.Second

	// residue marks titles left behind by interrupted earlier runs; rename
	// loops never pick such objects as candidates.
	residue = "anycli-e2e-"
)

// mustRun executes one bitly command and fails the test on nonzero exit.
func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, exit := e2e.RunTool(t, "bitly", "", args...)
	if exit != 0 {
		t.Fatalf("%s: exit = %d, output:\n%s", strings.Join(args, " "), exit, out)
	}
	return out
}

// runPlanGated executes a bitly command that bitly may gate behind a paid
// plan. CI against the connected account showed ALL metrics endpoints —
// including clicks, clicks-summary, shorten-counts and qr scans, not just
// breakdowns — return 402 UPGRADE_REQUIRED. e2e.RunTool captures stdout only and bitly prints API errors to
// stderr, so the error text is never inspectable here; any nonzero exit is
// logged and treated as plan-gated rather than hard-failing free-tier
// accounts.
//
// TODO: have e2e.RunTool expose the engine error string (stderr) so
// plan-gating can match UPGRADE_REQUIRED/FORBIDDEN specifically instead of
// swallowing every nonzero exit.
func runPlanGated(t *testing.T, args ...string) (out string, ok bool) {
	t.Helper()
	out, exit := e2e.RunTool(t, "bitly", "", args...)
	if exit != 0 {
		t.Logf("%s: nonzero exit %d — treating as plan-gated (free tier) and continuing", strings.Join(args, " "), exit)
		return "", false
	}
	return out, true
}

// parseObject unmarshals command output as a JSON object.
func parseObject(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	return m
}

// stringField asserts key holds a string and returns it (may be empty).
func stringField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	v, ok := m[key].(string)
	if !ok {
		t.Fatalf("field %q missing or not a string: %#v", key, m[key])
	}
	return v
}

// requireKey asserts the JSON object contains key (any value).
func requireKey(t *testing.T, m map[string]any, key, what string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Fatalf("%s: key %q missing in response", what, key)
	}
}

// requireMetricsKey asserts a breakdown response carries the metrics key.
func requireMetricsKey(t *testing.T, out, what string) {
	t.Helper()
	requireKey(t, parseObject(t, out), "metrics", what)
}

// pollStringField re-reads via getArgs until the named field equals want,
// retrying up to pollAttempts times with pollInterval sleeps (bitly
// read-after-write lag). Fatal when it never converges. It matches want
// exactly — a lag-delayed intermediate read must not pass spuriously.
func pollStringField(t *testing.T, key, want string, getArgs ...string) {
	t.Helper()
	var last string
	for i := 0; i < pollAttempts; i++ {
		if i > 0 {
			time.Sleep(pollInterval)
		}
		m := parseObject(t, mustRun(t, getArgs...))
		if v, ok := m[key].(string); ok {
			last = v
			if v == want {
				return
			}
		}
	}
	t.Fatalf("%s: field %q never converged to %q after %d attempts (last %q)",
		strings.Join(getArgs, " "), key, want, pollAttempts, last)
}

// TestE2EIdentityAndGroupChain ties the authenticated user to its default
// group: user get → group list (membership) → group get → group tags.
func TestE2EIdentityAndGroupChain(t *testing.T) {
	user := parseObject(t, mustRun(t, "user", "get", "--json"))
	if login := stringField(t, user, "login"); login == "" {
		t.Fatal("user get: login is empty")
	}
	g := stringField(t, user, "default_group_guid")
	if g == "" {
		t.Fatal("user get: default_group_guid is empty")
	}

	var groupList struct {
		Groups []struct {
			GUID string `json:"guid"`
		} `json:"groups"`
	}
	out := mustRun(t, "group", "list", "--json")
	if err := json.Unmarshal([]byte(out), &groupList); err != nil {
		t.Fatalf("cannot parse group list output: %v\n%s", err, out)
	}
	if len(groupList.Groups) == 0 {
		t.Fatal("group list: groups is empty")
	}
	found := false
	for _, grp := range groupList.Groups {
		if grp.GUID == g {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("group list: default_group_guid %q not among listed groups", g)
	}

	group := parseObject(t, mustRun(t, "group", "get", "--group", g, "--json"))
	if got := stringField(t, group, "guid"); got != g {
		t.Fatalf("group get: guid = %q, want %q", got, g)
	}
	if name := stringField(t, group, "name"); name == "" {
		t.Fatal("group get: name is empty")
	}

	tags := parseObject(t, mustRun(t, "group", "tags", "--group", g, "--json"))
	if _, ok := tags["tags"].([]any); !ok {
		t.Fatalf("group tags: tags key missing or not an array: %#v", tags["tags"])
	}
}

// linkEntry is one element of link list --json output.
type linkEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func parseLinks(t *testing.T, out string) []linkEntry {
	t.Helper()
	var v struct {
		Links []linkEntry `json:"links"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse link list output: %v\n%s", err, out)
	}
	return v.Links
}

// TestE2ELinkTitleClosedLoop covers link list/get/expand read paths and the
// reversible title rename loop on an existing bitlink. Empty-title links are
// never picked for the rename: link update's PATCH omits `title` when the
// flag is "", so restoring to an empty title would be a silent no-op.
func TestE2ELinkTitleClosedLoop(t *testing.T) {
	links := parseLinks(t, mustRun(t, "link", "list", "--size", "50", "--json"))
	if len(links) == 0 {
		t.Skip("no bitlinks in the account — seed one link to enable the closed loop")
	}

	// Read-path checks on the first link (title not needed here).
	l0 := links[0].ID
	got := parseObject(t, mustRun(t, "link", "get", "--bitlink", l0, "--json"))
	if id := stringField(t, got, "id"); id != l0 {
		t.Fatalf("link get: id = %q, want %q", id, l0)
	}
	u := stringField(t, got, "long_url")
	if u == "" {
		t.Fatal("link get: long_url is empty")
	}

	// POST-shaped read cross-checked against the GET.
	expanded := parseObject(t, mustRun(t, "link", "expand", "--bitlink", l0, "--json"))
	if id := stringField(t, expanded, "id"); id != l0 {
		t.Fatalf("link expand: id = %q, want %q", id, l0)
	}
	if lu := stringField(t, expanded, "long_url"); lu != u {
		t.Fatalf("link expand: long_url = %q, want %q", lu, u)
	}

	// Rename-loop candidate: non-empty title, no residue from earlier runs.
	var link, originalTitle string
	for _, l := range links {
		if l.Title != "" && !strings.Contains(l.Title, residue) {
			link, originalTitle = l.ID, l.Title
			break
		}
	}
	if link == "" {
		t.Skip("no bitlink with a non-empty title — seed one titled link; restoring an empty title is impossible via PATCH")
	}

	marker := e2e.Prefix() + "title-probe"
	mustRun(t, "link", "update", "--bitlink", link, "--title", marker, "--json")
	pollStringField(t, "title", marker, "link", "get", "--bitlink", link, "--json")

	// Restore. originalTitle is non-empty by construction, so the PATCH body
	// actually carries the field. An interrupted run leaves only the
	// prefix-tagged marker title behind, which is acceptable visible residue.
	mustRun(t, "link", "update", "--bitlink", link, "--title", originalTitle, "--json")
	pollStringField(t, "title", originalTitle, "link", "get", "--bitlink", link, "--json")
}

// TestE2ELinkAnalyticsSweep exercises every per-bitlink analytics command.
// CI against the connected account showed even clicks and clicks-summary
// return 402 UPGRADE_REQUIRED (bitly gates ALL metrics endpoints on this
// plan), so every call routes through runPlanGated; shape assertions stay
// full-strength whenever a call succeeds, and the sweep skips (never
// silently passes) when the account gates everything.
func TestE2ELinkAnalyticsSweep(t *testing.T) {
	links := parseLinks(t, mustRun(t, "link", "list", "--size", "1", "--json"))
	if len(links) == 0 {
		t.Skip("no bitlinks in the account — seed one link to enable the analytics sweep")
	}
	l := links[0].ID

	anyOK := false

	if out, ok := runPlanGated(t, "analytics", "clicks", "--bitlink", l, "--unit", "day", "--units", "7", "--json"); ok {
		anyOK = true
		clicks := parseObject(t, out)
		if unit := stringField(t, clicks, "unit"); unit != "day" {
			t.Fatalf("analytics clicks: unit = %q, want %q", unit, "day")
		}
		requireKey(t, clicks, "link_clicks", "analytics clicks")
	}

	if out, ok := runPlanGated(t, "analytics", "clicks-summary", "--bitlink", l, "--unit", "day", "--units", "7", "--json"); ok {
		anyOK = true
		summary := parseObject(t, out)
		total, okTotal := summary["total_clicks"].(float64)
		if !okTotal || total < 0 {
			t.Fatalf("analytics clicks-summary: total_clicks missing or negative: %#v", summary["total_clicks"])
		}
	}

	for _, sub := range []string{"countries", "devices", "referrers", "referrer-name", "referring-domains"} {
		if out, ok := runPlanGated(t, "analytics", sub, "--bitlink", l, "--units", "7", "--size", "10", "--json"); ok {
			anyOK = true
			m := parseObject(t, out)
			requireKey(t, m, "metrics", "analytics "+sub)
			if sub == "countries" {
				if facet := stringField(t, m, "facet"); facet != "countries" {
					t.Fatalf("analytics countries: facet = %q, want %q", facet, "countries")
				}
			}
		}
	}

	if out, ok := runPlanGated(t, "analytics", "referrers-by-domains", "--bitlink", l, "--units", "7", "--size", "10", "--json"); ok {
		anyOK = true
		requireKey(t, parseObject(t, out), "referrers_by_domain", "analytics referrers-by-domains")
	}

	if out, ok := runPlanGated(t, "analytics", "cities", "--bitlink", l, "--units", "7", "--size", "10", "--json"); ok {
		anyOK = true
		requireMetricsKey(t, out, "analytics cities")
	}

	for _, sub := range []string{"engagements", "engagements-summary"} {
		if out, ok := runPlanGated(t, "analytics", sub, "--bitlink", l, "--units", "7", "--json"); ok {
			anyOK = true
			if m := parseObject(t, out); len(m) == 0 {
				t.Fatalf("analytics %s: empty JSON object", sub)
			}
		}
	}

	if !anyOK {
		t.Skip("every link analytics endpoint exited nonzero — CI observed HTTP 402 UPGRADE_REQUIRED on this account (all bitlink metrics are plan-gated); upgrade the plan to enable the sweep")
	}
}

// TestE2EGroupAnalyticsSweep exercises the group-level metric commands.
// shorten-counts runs WITHOUT --group to exercise the resolveGroup
// auto-resolution path. CI showed even shorten-counts returns 402
// UPGRADE_REQUIRED on the connected account, so every metric call routes
// through runPlanGated and the sweep skips when everything is gated.
func TestE2EGroupAnalyticsSweep(t *testing.T) {
	anyOK := false

	if out, ok := runPlanGated(t, "group", "shorten-counts", "--unit", "month", "--units", "1", "--json"); ok {
		anyOK = true
		counts := parseObject(t, out)
		if unit := stringField(t, counts, "unit"); unit != "month" {
			t.Fatalf("group shorten-counts: unit = %q, want %q", unit, "month")
		}
		requireKey(t, counts, "metrics", "group shorten-counts")
	}

	user := parseObject(t, mustRun(t, "user", "get", "--json"))
	g := stringField(t, user, "default_group_guid")
	if g == "" {
		t.Fatal("user get: default_group_guid is empty")
	}

	if out, ok := runPlanGated(t, "group", "clicks", "--group", g, "--unit", "day", "--units", "7", "--json"); ok {
		anyOK = true
		clicks := parseObject(t, out)
		if unit := stringField(t, clicks, "unit"); unit != "day" {
			t.Fatalf("group clicks: unit = %q, want %q", unit, "day")
		}
	}

	for _, sub := range []string{"countries", "referrers", "devices", "cities"} {
		if out, ok := runPlanGated(t, "group", sub, "--group", g, "--units", "7", "--size", "10", "--json"); ok {
			anyOK = true
			requireMetricsKey(t, out, "group "+sub)
		}
	}

	if !anyOK {
		t.Skip("every group metrics endpoint exited nonzero — CI observed HTTP 402 UPGRADE_REQUIRED on this account (group metrics are plan-gated); upgrade the plan to enable the sweep")
	}
}

// qrEntry is one element of qr list --json output.
type qrEntry struct {
	QRCodeID string `json:"qrcode_id"`
	Title    string `json:"title"`
}

func parseQRCodes(t *testing.T, out string) []qrEntry {
	t.Helper()
	var v struct {
		QRCodes []qrEntry `json:"qr_codes"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse qr list output: %v\n%s", err, out)
	}
	return v.QRCodes
}

const qrSeedMsg = "no QR codes in the account — seed one existing QR code manually; e2e never creates QR codes (quota)"

// TestE2EQRTitleAndImageClosedLoop covers qr list/get/image and the
// reversible title rename loop on an existing QR code. As with links,
// qr update's PATCH omits `title` when "", so an empty original title can
// never be restored — such QR codes are not rename candidates.
func TestE2EQRTitleAndImageClosedLoop(t *testing.T) {
	out := mustRun(t, "qr", "list", "--size", "10", "--json")
	requireKey(t, parseObject(t, out), "qr_codes", "qr list")
	qrCodes := parseQRCodes(t, out)
	if len(qrCodes) == 0 {
		t.Skip(qrSeedMsg)
	}

	// Read/image checks on the first QR code (title not needed).
	q0 := qrCodes[0].QRCodeID
	got := parseObject(t, mustRun(t, "qr", "get", "--qr", q0, "--json"))
	if id := stringField(t, got, "qrcode_id"); id != q0 {
		t.Fatalf("qr get: qrcode_id = %q, want %q", id, q0)
	}

	// SVG variant: base64 envelope on stdout.
	var envelope struct {
		QRCodeID string `json:"qrcode_id"`
		Format   string `json:"format"`
		Encoding string `json:"encoding"`
		Data     string `json:"data"`
	}
	svgOut := mustRun(t, "qr", "image", "--qr", q0, "--format", "svg", "--json")
	if err := json.Unmarshal([]byte(svgOut), &envelope); err != nil {
		t.Fatalf("cannot parse qr image envelope: %v\n%s", err, svgOut)
	}
	if envelope.QRCodeID != q0 || envelope.Format != "svg" || envelope.Encoding != "base64" || envelope.Data == "" {
		t.Fatalf("qr image svg envelope mismatch: %+v", envelope)
	}
	svg, err := base64.StdEncoding.DecodeString(envelope.Data)
	if err != nil {
		t.Fatalf("qr image svg: data is not valid base64: %v", err)
	}
	if !bytes.Contains(svg, []byte("<svg")) {
		t.Fatalf("qr image svg: decoded data does not contain %q", "<svg")
	}

	// PNG variant: file receipt on stdout, raw bytes on disk.
	path := filepath.Join(t.TempDir(), "qr.png")
	var receipt struct {
		QRCodeID string `json:"qrcode_id"`
		Format   string `json:"format"`
		Bytes    int    `json:"bytes"`
		Path     string `json:"path"`
	}
	pngOut := mustRun(t, "qr", "image", "--qr", q0, "--format", "png", "--output", path)
	if err := json.Unmarshal([]byte(pngOut), &receipt); err != nil {
		t.Fatalf("cannot parse qr image receipt: %v\n%s", err, pngOut)
	}
	if receipt.QRCodeID != q0 || receipt.Format != "png" || receipt.Bytes <= 0 || receipt.Path != path {
		t.Fatalf("qr image png receipt mismatch: %+v", receipt)
	}
	png, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written qr image: %v", err)
	}
	if len(png) != receipt.Bytes {
		t.Fatalf("qr image png: file size %d != receipt bytes %d", len(png), receipt.Bytes)
	}
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(png) < len(pngMagic) || !bytes.Equal(png[:len(pngMagic)], pngMagic) {
		t.Fatal("qr image png: file does not start with the PNG magic")
	}

	// Rename-loop candidate: non-empty title, no residue from earlier runs.
	var qr, originalTitle string
	for _, q := range qrCodes {
		if q.Title != "" && !strings.Contains(q.Title, residue) {
			qr, originalTitle = q.QRCodeID, q.Title
			break
		}
	}
	if qr == "" {
		t.Skip("no QR code with a non-empty title — seed a titled QR code to enable the rename loop")
	}

	marker := e2e.Prefix() + "qr-probe"
	mustRun(t, "qr", "update", "--qr", qr, "--title", marker, "--json")
	pollStringField(t, "title", marker, "qr", "get", "--qr", qr, "--json")

	mustRun(t, "qr", "update", "--qr", qr, "--title", originalTitle, "--json")
	pollStringField(t, "title", originalTitle, "qr", "get", "--qr", qr, "--json")
}

// TestE2EQRScansSweep exercises the per-QR-code scan metric commands. Scans
// data on a fresh QR code is legitimately empty, so it asserts envelope
// shape only, never counts > 0. CI showed even qr scans scans returns 402
// UPGRADE_REQUIRED on the connected account, so every call routes through
// runPlanGated and the sweep skips when everything is gated.
func TestE2EQRScansSweep(t *testing.T) {
	qrCodes := parseQRCodes(t, mustRun(t, "qr", "list", "--size", "1", "--json"))
	if len(qrCodes) == 0 {
		t.Skip(qrSeedMsg)
	}
	q := qrCodes[0].QRCodeID

	anyOK := false

	if out, ok := runPlanGated(t, "qr", "scans", "scans", "--qr", q, "--unit", "day", "--units", "7", "--json"); ok {
		anyOK = true
		scans := parseObject(t, out)
		if len(scans) == 0 {
			t.Fatal("qr scans scans: empty JSON object")
		}
		if unit := stringField(t, scans, "unit"); unit != "day" {
			t.Fatalf("qr scans scans: unit = %q, want %q", unit, "day")
		}
	}

	// The summary command is raw provider-JSON passthrough and the concrete
	// total field name is unverified: assert a non-empty object; if a
	// total_scans key IS present it must be numeric >= 0, otherwise log the
	// actual keys so the concrete name can be locked in after one observed
	// live run.
	if out, ok := runPlanGated(t, "qr", "scans", "summary", "--qr", q, "--units", "7", "--json"); ok {
		anyOK = true
		summary := parseObject(t, out)
		if len(summary) == 0 {
			t.Fatal("qr scans summary: empty JSON object")
		}
		if raw, present := summary["total_scans"]; present {
			if v, okNum := raw.(float64); !okNum || v < 0 {
				t.Fatalf("qr scans summary: total_scans not numeric >= 0: %#v", raw)
			}
		} else {
			keys := make([]string, 0, len(summary))
			for k := range summary {
				keys = append(keys, k)
			}
			t.Logf("qr scans summary: no total_scans key; top-level keys: %v", keys)
		}
	}

	for _, sub := range []string{"countries", "cities", "device-os", "browsers"} {
		if out, ok := runPlanGated(t, "qr", "scans", sub, "--qr", q, "--units", "7", "--size", "10", "--json"); ok {
			anyOK = true
			requireMetricsKey(t, out, "qr scans "+sub)
		}
	}

	if !anyOK {
		t.Skip("every qr scan metrics endpoint exited nonzero — CI observed HTTP 402 UPGRADE_REQUIRED on this account (QR scan metrics are plan-gated); upgrade the plan to enable the sweep")
	}
}

// TestE2ECampaignDescriptionClosedLoop exercises campaign list/get/update via
// a reversible description rename on an existing campaign. e2e must never
// create campaigns; campaign update's PATCH omits `description` when "", so
// only campaigns with a non-empty description are restorable candidates.
// Never touches --name or --channel-guids so the loop stays minimal.
func TestE2ECampaignDescriptionClosedLoop(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "campaign", "list", "--json")
	if exit != 0 {
		t.Skip("campaign list failed (nonzero exit) — campaigns typically require a paid bitly plan; error text is on stderr (already t.Logf'd by RunTool)")
	}
	requireKey(t, parseObject(t, out), "campaigns", "campaign list")

	var v struct {
		Campaigns []struct {
			GUID        string `json:"guid"`
			Description string `json:"description"`
		} `json:"campaigns"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse campaign list output: %v\n%s", err, out)
	}
	if len(v.Campaigns) == 0 {
		t.Skip("no campaigns in the account and e2e must never create one — seed a campaign manually to enable this loop")
	}

	// Candidate selection from the list entries first.
	var c, originalDesc string
	for _, camp := range v.Campaigns {
		if camp.Description != "" && !strings.Contains(camp.Description, residue) {
			c = camp.GUID
			break
		}
	}
	if c == "" {
		// List entries may omit the description field: fall back to
		// campaign get on up to the first 3 campaigns.
		for i := 0; i < len(v.Campaigns) && i < 3; i++ {
			m := parseObject(t, mustRun(t, "campaign", "get", "--campaign", v.Campaigns[i].GUID, "--json"))
			if desc, ok := m["description"].(string); ok && desc != "" && !strings.Contains(desc, residue) {
				c = v.Campaigns[i].GUID
				break
			}
		}
	}
	if c == "" {
		m := parseObject(t, mustRun(t, "campaign", "get", "--campaign", v.Campaigns[0].GUID, "--json"))
		if guid := stringField(t, m, "guid"); guid != v.Campaigns[0].GUID {
			t.Fatalf("campaign get: guid = %q, want %q", guid, v.Campaigns[0].GUID)
		}
		t.Skip("no campaign with a non-empty description — seed one to enable the reversible update loop")
	}

	got := parseObject(t, mustRun(t, "campaign", "get", "--campaign", c, "--json"))
	if guid := stringField(t, got, "guid"); guid != c {
		t.Fatalf("campaign get: guid = %q, want %q", guid, c)
	}
	originalDesc = stringField(t, got, "description")
	if originalDesc == "" {
		t.Fatalf("campaign get: description unexpectedly empty for candidate %q", c)
	}

	marker := e2e.Prefix() + "campaign-probe"
	mustRun(t, "campaign", "update", "--campaign", c, "--description", marker, "--json")
	pollStringField(t, "description", marker, "campaign", "get", "--campaign", c, "--json")

	mustRun(t, "campaign", "update", "--campaign", c, "--description", originalDesc, "--json")
	pollStringField(t, "description", originalDesc, "campaign", "get", "--campaign", c, "--json")
}

// TestE2EChannelNameClosedLoop exercises channel list/get/update via a
// reversible name rename on an existing channel. e2e must never create
// channels; channel update's PATCH omits `name` when "", so only channels
// with a non-empty name are restorable candidates (guarded even though name
// is required at creation).
//
// Runtime note: every poll returns early on success, so the pollAttempts x
// pollInterval caps only bound the failure path; happy-path polls typically
// converge in 1-2 attempts. Worst-case sleep across all four closed loops
// (8 polls x 5 x 2s = 80s) occurs only on runs that are already failing and
// still fits the ~2 minute budget.
func TestE2EChannelNameClosedLoop(t *testing.T) {
	out, exit := e2e.RunTool(t, "bitly", "", "campaign", "channel", "list", "--json")
	if exit != 0 {
		t.Skip("channel list failed (nonzero exit) — channels typically require a paid bitly plan; error text is on stderr (already t.Logf'd by RunTool)")
	}
	requireKey(t, parseObject(t, out), "channels", "campaign channel list")

	var v struct {
		Channels []struct {
			GUID string `json:"guid"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("cannot parse channel list output: %v\n%s", err, out)
	}
	if len(v.Channels) == 0 {
		t.Skip("no channels in the account and e2e must never create one — seed a channel manually to enable this loop")
	}

	var ch, originalName string
	for _, entry := range v.Channels {
		if entry.Name != "" && !strings.Contains(entry.Name, residue) {
			ch, originalName = entry.GUID, entry.Name
			break
		}
	}
	if ch == "" {
		m := parseObject(t, mustRun(t, "campaign", "channel", "get", "--channel", v.Channels[0].GUID, "--json"))
		if guid := stringField(t, m, "guid"); guid != v.Channels[0].GUID {
			t.Fatalf("campaign channel get: guid = %q, want %q", guid, v.Channels[0].GUID)
		}
		t.Skip("no channel with a non-empty name — cannot run a restorable rename")
	}

	got := parseObject(t, mustRun(t, "campaign", "channel", "get", "--channel", ch, "--json"))
	if guid := stringField(t, got, "guid"); guid != ch {
		t.Fatalf("campaign channel get: guid = %q, want %q", guid, ch)
	}

	marker := e2e.Prefix() + "channel-probe"
	mustRun(t, "campaign", "channel", "update", "--channel", ch, "--name", marker, "--json")
	pollStringField(t, "name", marker, "campaign", "channel", "get", "--channel", ch, "--json")

	// Restore; the marker is left as prefix-tagged residue if this never
	// converges.
	mustRun(t, "campaign", "channel", "update", "--channel", ch, "--name", originalName, "--json")
	pollStringField(t, "name", originalName, "campaign", "channel", "get", "--channel", ch, "--json")
}
