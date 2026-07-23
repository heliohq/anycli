package main

// This file is the `go run ./internal/e2e/affected` entry the e2e workflow
// calls. Modes:
//
//	-base <ref>   diff HEAD against <ref>, print affected tools as JSON
//	-all          print every tool that has an e2e_test.go, as JSON
//
// On any git failure the program falls back to the smoke subset and says so
// on stderr — a broken diff must degrade to "run something", never to
// "silently run nothing" (design 008: no silent caps).

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/heliohq/anycli/definitions"
)

func main() {
	base := flag.String("base", "", "git ref to diff HEAD against")
	all := flag.Bool("all", false, "list every tool that has e2e tests")
	flag.Parse()

	defs, err := definitions.ListBundled()
	if err != nil {
		fatal(err)
	}
	var tools []string
	for _, d := range defs {
		tools = append(tools, d.Name)
	}

	var result []string
	switch {
	case *all:
		result = toolsWithE2ETests(tools)
	case *base != "":
		changed, err := gitDiff(*base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "affected: git diff failed (%v); falling back to smoke subset\n", err)
			result = SmokeTools
			break
		}
		var smoke bool
		result, smoke = Affected(changed, tools)
		if smoke {
			result = mergeSorted(result, SmokeTools)
		}
	default:
		fatal(fmt.Errorf("one of -base or -all is required"))
	}

	for _, tool := range result {
		if PolicyFor(tool) == LevelSkip {
			fmt.Fprintf(os.Stderr, "affected: %s is policy-skipped (design 008 D8)\n", tool)
		}
	}
	out, err := json.Marshal(matrixEntries(result))
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func gitDiff(base string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base+"...HEAD").Output()
	if err != nil {
		// Fall back to a two-dot diff for shallow/force-push cases where
		// the merge base is unavailable.
		out, err = exec.Command("git", "diff", "--name-only", base, "HEAD").Output()
		if err != nil {
			return nil, err
		}
	}
	return strings.Fields(string(out)), nil
}

func toolsWithE2ETests(tools []string) []string {
	var out []string
	for _, tool := range tools {
		if _, err := os.Stat("internal/tools/" + PkgDir(tool) + "/e2e_test.go"); err == nil {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func mergeSorted(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "affected:", err)
	os.Exit(1)
}
