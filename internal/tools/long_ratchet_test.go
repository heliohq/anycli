package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/heliohq/anycli/internal/toolhelp"
)

// longBaselinePath is the checked-in snapshot of every visible leaf that has
// no Long today. It is the ratchet's only escape hatch, and it only shrinks.
var longBaselinePath = filepath.Join("testdata", "long_baseline.txt")

const longBaselineHeader = `# Leaves with no Long — design 331 ratchet baseline.
#
# A tool's capability face gives coverage (each leaf's Short); "<leaf> --help"
# gives depth, and depth is the leaf's Long. Long is where provider API facts
# live so they sit next to the code they describe and cannot drift (D1).
#
# TestLeafLongCoverageRatchet asserts that every visible leaf NOT listed here
# has a non-empty Long. A newly added command must therefore ship its prose.
#
# This file only shrinks: delete lines as leaves gain a Long. Adding a line is
# an explicit, review-visible admission that a command shipped undocumented.
# When the list is empty, drop the file and assert Long on every leaf.
#
# Format: "<tool id> <command path below the tool root>", sorted.
`

// leafKey is the baseline's line format: the registry tool id plus the command
// path a caller types after it.
func leafKey(tool, path string) string { return tool + " " + path }

// emptyLongLeaves returns the baseline-format key of every visible leaf whose
// Long is empty, across all registered service tools, sorted.
func emptyLongLeaves(t *testing.T) []string {
	t.Helper()
	var keys []string
	for _, tool := range ServiceNames() {
		svc, err := GetService(tool)
		if err != nil {
			t.Fatalf("tool %q: get service: %v", tool, err)
		}
		root := svc.NewCommandTree()
		if root == nil {
			t.Fatalf("tool %q: NewCommandTree returned nil", tool)
		}
		for _, leaf := range toolhelp.Leaves(root) {
			if strings.TrimSpace(leaf.Long) != "" {
				continue
			}
			keys = append(keys, leafKey(tool, toolhelp.LeafPath(root, leaf)))
		}
	}
	sort.Strings(keys)
	return keys
}

// readLongBaseline parses the checked-in snapshot, skipping comments and blank
// lines.
func readLongBaseline(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(longBaselinePath)
	if err != nil {
		t.Fatalf("read %s: %v", longBaselinePath, err)
	}
	defer f.Close()

	baseline := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		baseline[line] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", longBaselinePath, err)
	}
	return baseline
}

// TestLeafLongCoverageRatchet enforces the design-331 migration ratchet.
//
// Asserting "every leaf has a Long" outright would fail on day one (16 of
// 2334 leaves have one), so the assertion is relative to a checked-in
// baseline: a leaf that is not in the baseline must have a non-empty Long.
// New commands therefore cannot ship undocumented, and the baseline can only
// shrink as prose is migrated in.
//
// Known limit, stated plainly: this can only tell empty from non-empty. A
// placeholder satisfies it. It stops "forgot to write one" (the default
// outcome); "wrote a bad one" is a review problem.
func TestLeafLongCoverageRatchet(t *testing.T) {
	res := ratchetViolations(emptyLongLeaves(t), readLongBaseline(t))
	for _, v := range res.Undocumented {
		t.Error(v)
	}
	for _, v := range res.Stale {
		t.Error(v)
	}
	// The regeneration hint is emitted for the *stale* direction only.
	// Regenerating is legitimate there — the file shrinks. Advertising it for a
	// new undocumented leaf would hand the author a command that grows the
	// baseline and turns the red test green, which is the exact review-visible
	// admission the ratchet exists to force.
	if len(res.Stale) > 0 {
		t.Logf("after migrating a batch, shrink the baseline with: ANYCLI_WRITE_LONG_BASELINE=1 go test ./internal/tools -run TestWriteLongBaseline")
	}
	if len(res.Undocumented) > 0 {
		t.Logf("write the command's Long. If it genuinely must ship undocumented, add its single line to %s by hand — that diff is the deliberate exception a reviewer signs off on. Regenerating the baseline is not the fix and will be rejected.", longBaselinePath)
	}
}

// ratchetResult separates the ratchet's two failure directions so the caller
// can react to each one differently — only the stale direction is fixable by
// regenerating the baseline.
type ratchetResult struct {
	// Undocumented is one message per leaf with no Long that the baseline does
	// not exempt. Fixed by writing prose, or by a hand-added baseline line.
	Undocumented []string
	// Stale is one message per baseline entry that no longer describes an
	// undocumented leaf. Fixed by regeneration; the file shrinks.
	Stale []string
}

// ratchetViolations compares the leaves that have no Long today against the
// checked-in baseline, in both directions: an undocumented leaf outside the
// baseline (the ratchet's job), and a baseline entry that no longer describes
// an undocumented leaf (the ratchet only shrinks, so a spent line must go).
func ratchetViolations(empty []string, baseline map[string]bool) ratchetResult {
	emptyNow := make(map[string]bool, len(empty))
	for _, key := range empty {
		emptyNow[key] = true
	}

	var res ratchetResult
	for _, key := range baselineAdditions(empty, baseline) {
		res.Undocumented = append(res.Undocumented, fmt.Sprintf("leaf %q has an empty Long — every command outside %s must carry its parameters, limits and cost notes (design 331 D1). Adding it to the baseline instead is a deliberate, review-visible exception.", key, longBaselinePath))
	}

	var stale []string
	for key := range baseline {
		if !emptyNow[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		res.Stale = append(res.Stale, fmt.Sprintf("stale baseline entry %q — the leaf now has a Long (or no longer exists). Remove the line from %s; the baseline only shrinks.", key, longBaselinePath))
	}
	return res
}

// baselineAdditions returns, sorted, the keys in empty that the baseline does
// not already contain — i.e. the lines a regeneration would ADD. The ratchet
// reports them as violations and the writer refuses to persist them, so the
// snapshot only shrinks by construction rather than by comment.
func baselineAdditions(empty []string, baseline map[string]bool) []string {
	var added []string
	for _, key := range empty {
		if !baseline[key] {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	return added
}

// TestRatchetViolationsDetectsBothDirections proves the ratchet actually
// fires, on synthetic input so it does not depend on the real baseline.
func TestRatchetViolationsDetectsBothDirections(t *testing.T) {
	cases := []struct {
		name             string
		empty            []string
		baseline         map[string]bool
		wantUndocumented string // substring of exactly one expected message; "" = none
		wantStale        string // substring of exactly one expected message; "" = none
	}{
		{
			name:     "documented leaf outside the baseline is clean",
			empty:    []string{"x post create"},
			baseline: map[string]bool{"x post create": true},
		},
		{
			name:             "new undocumented leaf fails, and only in the undocumented direction",
			empty:            []string{"x post create", "x post search"},
			baseline:         map[string]bool{"x post create": true},
			wantUndocumented: `leaf "x post search" has an empty Long`,
		},
		{
			name:      "leaf that gained a Long leaves a stale entry",
			empty:     []string{},
			baseline:  map[string]bool{"x post create": true},
			wantStale: `stale baseline entry "x post create"`,
		},
		{
			name:      "removed leaf leaves a stale entry",
			empty:     []string{"x post create"},
			baseline:  map[string]bool{"x post create": true, "x post gone": true},
			wantStale: `stale baseline entry "x post gone"`,
		},
		{
			name:             "both directions are reported separately",
			empty:            []string{"x post search"},
			baseline:         map[string]bool{"x post create": true},
			wantUndocumented: `leaf "x post search" has an empty Long`,
			wantStale:        `stale baseline entry "x post create"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ratchetViolations(tc.empty, tc.baseline)
			assertMessages(t, "undocumented", res.Undocumented, tc.wantUndocumented)
			assertMessages(t, "stale", res.Stale, tc.wantStale)
		})
	}
}

// assertMessages asserts that got is empty when want is "", and otherwise
// contains at least one message with want as a substring.
func assertMessages(t *testing.T, direction string, got []string, want string) {
	t.Helper()
	if want == "" {
		if len(got) != 0 {
			t.Fatalf("want no %s violations, got: %v", direction, got)
		}
		return
	}
	for _, v := range got {
		if strings.Contains(v, want) {
			return
		}
	}
	t.Fatalf("no %s violation containing %q; got: %v", direction, want, got)
}

// TestBaselineAdditionsFindsGrowth covers the predicate that makes the snapshot
// shrink-only: it must name exactly the keys a regeneration would add.
func TestBaselineAdditionsFindsGrowth(t *testing.T) {
	cases := []struct {
		name     string
		empty    []string
		baseline map[string]bool
		want     []string
	}{
		{
			name:     "same set adds nothing",
			empty:    []string{"x post create"},
			baseline: map[string]bool{"x post create": true},
			want:     nil,
		},
		{
			name:     "shrinking adds nothing",
			empty:    []string{"x post create"},
			baseline: map[string]bool{"x post create": true, "x post gone": true},
			want:     nil,
		},
		{
			name:     "a new undocumented leaf is growth",
			empty:    []string{"x post search", "x post create"},
			baseline: map[string]bool{"x post create": true},
			want:     []string{"x post search"},
		},
		{
			name:     "growth is reported sorted",
			empty:    []string{"x post search", "x me show"},
			baseline: map[string]bool{},
			want:     []string{"x me show", "x post search"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := baselineAdditions(tc.empty, tc.baseline)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("baselineAdditions = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLongBaselineEntriesAreWellFormed keeps the snapshot machine-checkable:
// every line must name a real registered tool, so a hand-edited typo cannot
// silently exempt nothing (and therefore exempt the wrong leaf forever).
func TestLongBaselineEntriesAreWellFormed(t *testing.T) {
	known := map[string]bool{}
	for _, tool := range ServiceNames() {
		known[tool] = true
	}
	for line := range readLongBaseline(t) {
		tool, path, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(path) == "" {
			t.Errorf("baseline line %q is not \"<tool id> <command path>\"", line)
			continue
		}
		if !known[tool] {
			t.Errorf("baseline line %q names unregistered tool %q", line, tool)
		}
	}
}

// TestWriteLongBaseline rewrites the baseline snapshot from the current trees.
//
// It is opt-in, and it refuses to GROW the file: regeneration is only ever
// legitimate for shrinking the snapshot after a migration batch. Running it to
// silence a newly added undocumented command is rejected here rather than
// discouraged by a comment, so the ratchet cannot be defeated by the very
// command a failing run would otherwise hand the author. A command that
// genuinely must ship undocumented gets its single line added by hand, which
// is the review-visible admission design 331 asks for.
func TestWriteLongBaseline(t *testing.T) {
	if os.Getenv("ANYCLI_WRITE_LONG_BASELINE") != "1" {
		t.Skip("set ANYCLI_WRITE_LONG_BASELINE=1 to rewrite the baseline snapshot")
	}
	keys := emptyLongLeaves(t)
	if added := baselineAdditions(keys, readLongBaseline(t)); len(added) > 0 {
		t.Fatalf("refusing to write %s: it would ADD %d line(s) and the baseline only shrinks: %s\n"+
			"These leaves have no Long. Write it. If one genuinely must ship undocumented, add its line to the file by hand so the addition shows up in review.",
			longBaselinePath, len(added), strings.Join(added, ", "))
	}
	var b strings.Builder
	b.WriteString(longBaselineHeader)
	for _, key := range keys {
		b.WriteString(key + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(longBaselinePath), 0o755); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	if err := os.WriteFile(longBaselinePath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", longBaselinePath, err)
	}
	fmt.Printf("wrote %s with %d entries\n", longBaselinePath, len(keys))
}
