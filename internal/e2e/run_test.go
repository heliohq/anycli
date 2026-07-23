package e2e

import (
	"strings"
	"testing"
)

func TestPrefixIsStableAndTagged(t *testing.T) {
	p1, p2 := Prefix(), Prefix()
	if p1 != p2 {
		t.Errorf("Prefix not stable: %q vs %q", p1, p2)
	}
	if !strings.HasPrefix(p1, "anycli-e2e-") || !strings.HasSuffix(p1, "-") {
		t.Errorf("Prefix = %q, want anycli-e2e-<runid>-", p1)
	}
}

func TestCaptureStdout(t *testing.T) {
	out, err := captureStdout(func() error {
		_, werr := osStdoutWriteString("hello e2e")
		return werr
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello e2e" {
		t.Errorf("captured %q", out)
	}
}

func TestRunToolSkipsWithoutCredentials(t *testing.T) {
	t.Setenv("HELIO_E2E_API_KEY", "")
	t.Setenv("HELIO_E2E_API_BASE", "")
	res := testing.RunTests(func(pat, str string) (bool, error) { return true, nil },
		[]testing.InternalTest{{Name: "probe", F: func(st *testing.T) {
			RunTool(st, "attio", "", "whoami")
		}}})
	// The inner test must NOT fail — it must skip. RunTests returns true
	// when nothing failed (skips count as ok).
	if !res {
		t.Fatal("RunTool must skip, not fail, when no e2e credentials are configured")
	}
}
