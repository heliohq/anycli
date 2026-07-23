package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/heliohq/anycli/definitions"
)

var testTools = []string{"attio", "adobe-sign", "gate-probe", "github", "hunter", "billcom", "gmail"}

func TestAffectedByDefinitionFile(t *testing.T) {
	got, smoke := Affected([]string{"definitions/tools/attio.json"}, testTools)
	if smoke || !reflect.DeepEqual(got, []string{"attio"}) {
		t.Errorf("got %v smoke=%v", got, smoke)
	}
}

func TestAffectedByServicePackageWithDashDivergence(t *testing.T) {
	got, _ := Affected([]string{"internal/tools/adobesign/client.go"}, testTools)
	if !reflect.DeepEqual(got, []string{"adobe-sign"}) {
		t.Errorf("got %v, want [adobe-sign] (pkg dir has no dash)", got)
	}
}

func TestSharedCodeTriggersSmoke(t *testing.T) {
	for _, p := range []string{
		"anycli.go", "go.mod", "internal/exec/exec.go", "internal/credential/inject.go",
		"internal/middleware/engine.go", "internal/registry/schema.go",
		"internal/config/dirs.go", "definitions/embed.go",
		"internal/tools/register.go", "internal/tools/registry.go",
		"internal/e2e/resolver.go",
	} {
		if _, smoke := Affected([]string{p}, testTools); !smoke {
			t.Errorf("path %q must trigger the smoke subset", p)
		}
	}
}

func TestDocsAndWorkflowChangesAreIgnored(t *testing.T) {
	got, smoke := Affected([]string{"docs/design/008-x.md", "README.md", ".github/workflows/ci.yml"}, testTools)
	if len(got) != 0 || smoke {
		t.Errorf("got %v smoke=%v, want none", got, smoke)
	}
}

func TestAffectedDeduplicatesAndSorts(t *testing.T) {
	got, _ := Affected([]string{
		"internal/tools/attio/records.go",
		"definitions/tools/attio.json",
		"definitions/tools/hunter.json",
	}, testTools)
	if !reflect.DeepEqual(got, []string{"attio", "hunter"}) {
		t.Errorf("got %v", got)
	}
}

// Naming lint (design 008 D5): every bundled service definition must map to
// an existing internal/tools/<PkgDir(name)> directory via the strip-dash
// rule. This is what makes manifest-free path mapping safe.
func TestEveryServiceDefinitionHasMatchingPackageDir(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range defs {
		if d.Type != "service" {
			continue
		}
		dir := "../../tools/" + PkgDir(d.Name)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Errorf("service definition %q has no package dir internal/tools/%s", d.Name, PkgDir(d.Name))
		}
	}
}

func TestPolicyDefaultsToWarn(t *testing.T) {
	if got := PolicyFor("some-unlisted-tool"); got != LevelWarn {
		t.Errorf("PolicyFor(unlisted) = %q, want warn", got)
	}
}

func TestPolicyTableOnlyContainsBundledTools(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for tool := range policy {
		if !byName[tool] {
			t.Errorf("policy table entry %q has no bundled definition", tool)
		}
	}
}

func TestMatrixEntriesFilterSkipAndCarryLevel(t *testing.T) {
	// matrixEntries is the shared output shaping used by both -base and
	// -all modes.
	old := policy
	policy = map[string]Level{"hunter": LevelSkip, "attio": LevelRequired}
	defer func() { policy = old }()

	got := matrixEntries([]string{"attio", "hunter", "gmail"})
	want := []MatrixEntry{
		{Tool: "attio", Level: LevelRequired},
		{Tool: "gmail", Level: LevelWarn},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSmokeToolsAreBundled(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for _, s := range SmokeTools {
		if !byName[s] {
			t.Errorf("smoke tool %q has no bundled definition", s)
		}
	}
}
