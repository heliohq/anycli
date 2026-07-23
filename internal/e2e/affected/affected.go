// affected maps changed file paths to the e2e tool matrix (design 008 D5).
// There is no checked-in manifest: definition filename == tool name
// (enforced by definitions.ListBundled) and service package dir == tool
// name with dashes stripped (enforced by a lint test here). The package is
// `main` (together with main.go) so `go run ./internal/e2e/affected` works.
package main

import (
	"sort"
	"strings"
)

// SmokeTools is the fixed representative subset run when shared code
// changes: service + cli tool types, single- and multi-field credential
// shapes, and one Google-family tool. Tools without e2e tests or without a
// connection simply skip (design 008 D1/D9).
var SmokeTools = []string{"attio", "github", "hunter", "billcom", "gmail"}

// sharedPrefixes are the paths whose change affects every tool: the engine
// pipeline, credential handling, registry/schema, the embed loader, the
// service registry files directly under internal/tools/, and the e2e
// helper itself.
var sharedPrefixes = []string{
	"anycli.go",
	"go.mod",
	"go.sum",
	"definitions/embed",
	"internal/exec/",
	"internal/credential/",
	"internal/middleware/",
	"internal/registry/",
	"internal/config/",
	"internal/e2e/",
	"cmd/",
}

// Level is a tool's e2e blocking policy (design 008 D8).
type Level string

const (
	LevelSkip     Level = "skip"     // explicitly silenced, filtered from the matrix
	LevelWarn     Level = "warn"     // runs, failure visible, does not block merge
	LevelRequired Level = "required" // failure fails the e2e-gate job (branch protection)
)

// policy assigns non-default levels. Every unlisted tool is warn. Promote a
// tool to required after a proven stable streak; demote to skip when its
// provider is known-broken (design 008 D8: a per-tool graduation path, not
// a global switch).
var policy = map[string]Level{
	// No required tools yet — the table starts warn-only by design.
	// Example promotions/demotions:
	//   "stripe": LevelRequired,
	//   "hunter": LevelSkip, // provider maintenance until 2026-08-01
}

// PolicyFor returns the blocking level for a tool; unlisted tools are warn.
func PolicyFor(tool string) Level {
	if l, ok := policy[tool]; ok {
		return l
	}
	return LevelWarn
}

// MatrixEntry is one element of the JSON matrix the workflow consumes.
type MatrixEntry struct {
	Tool  string `json:"tool"`
	Level Level  `json:"level"`
}

// matrixEntries shapes a tool list into matrix entries: skip-level tools
// are filtered out (announced on stderr by main), the rest carry their
// level so the workflow can set continue-on-error per job.
func matrixEntries(tools []string) []MatrixEntry {
	out := []MatrixEntry{}
	for _, tool := range tools {
		if PolicyFor(tool) == LevelSkip {
			continue
		}
		out = append(out, MatrixEntry{Tool: tool, Level: PolicyFor(tool)})
	}
	return out
}

// PkgDir returns the internal/tools package directory name for a tool:
// the tool name with dashes removed (adobe-sign -> adobesign).
func PkgDir(tool string) string {
	return strings.ReplaceAll(tool, "-", "")
}

// Affected classifies changed paths against the known tool list. It returns
// the sorted, deduplicated affected tool names and whether shared code
// changed (caller substitutes SmokeTools).
func Affected(changed []string, tools []string) ([]string, bool) {
	byPkg := make(map[string]string, len(tools))
	for _, tool := range tools {
		byPkg[PkgDir(tool)] = tool
	}
	byName := make(map[string]bool, len(tools))
	for _, tool := range tools {
		byName[tool] = true
	}

	hit := map[string]bool{}
	smoke := false
	for _, p := range changed {
		switch {
		case strings.HasPrefix(p, "definitions/tools/") && strings.HasSuffix(p, ".json"):
			name := strings.TrimSuffix(strings.TrimPrefix(p, "definitions/tools/"), ".json")
			if byName[name] {
				hit[name] = true
			}
		case strings.HasPrefix(p, "internal/tools/"):
			rest := strings.TrimPrefix(p, "internal/tools/")
			dir, _, isSub := strings.Cut(rest, "/")
			if isSub {
				if tool, ok := byPkg[dir]; ok {
					hit[tool] = true
					continue
				}
			}
			// Files directly under internal/tools/ (register.go,
			// registry.go, lint_test.go) are shared plumbing.
			smoke = true
		default:
			for _, prefix := range sharedPrefixes {
				if strings.HasPrefix(p, prefix) {
					smoke = true
					break
				}
			}
		}
	}

	out := make([]string, 0, len(hit))
	for tool := range hit {
		out = append(out, tool)
	}
	sort.Strings(out)
	return out, smoke
}
