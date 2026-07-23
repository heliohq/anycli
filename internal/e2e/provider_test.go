package e2e

import (
	"testing"

	"github.com/heliohq/anycli/definitions"
)

func TestProviderFor(t *testing.T) {
	cases := map[string]string{
		// identity for tools whose anycli id equals the provider key
		"attio":  "attio",
		"gmail":  "gmail",
		"github": "github",
		// google short-name family
		"drive":  "google_drive",
		"sheets": "google_sheets",
		// mechanical dash↔underscore
		"adobe-sign":        "adobe_sign",
		"microsoft-outlook": "microsoft_outlook",
		// folded ids (anycli c269a6e) keep underscore provider keys
		"billcom":    "bill_com",
		"customerio": "customer_io",
		// irregular
		"search-console": "google_search_console",
	}
	for tool, want := range cases {
		if got := ProviderFor(tool); got != want {
			t.Errorf("ProviderFor(%q) = %q, want %q", tool, got, want)
		}
	}
}

// TestToolToProviderKeysAreBundled lints toolToProvider against the bundled
// definitions (same pattern as
// internal/e2e/affected.TestPolicyTableOnlyContainsBundledTools): a future
// tool-id rename that isn't mirrored here would silently degrade the
// mapping back to identity instead of failing loudly.
func TestToolToProviderKeysAreBundled(t *testing.T) {
	defs, err := definitions.ListBundled()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for tool := range toolToProvider {
		if !byName[tool] {
			t.Errorf("toolToProvider entry %q has no bundled definition", tool)
		}
	}
}
