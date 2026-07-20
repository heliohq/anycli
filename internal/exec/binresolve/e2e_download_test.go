package binresolve_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/exec/binresolve"
)

// TestE2ERealMongoshLazyInstall exercises the REAL lazy-install path end to
// end: the bundled mongodb definition's official download URL, the real
// sha256 pin, extraction into the versions/ layout, and execution of the
// installed standalone binary. Guarded by ANYCLI_E2E_DOWNLOAD=1 because it
// downloads ~85MB from downloads.mongodb.com — CI unit runs stay offline.
func TestE2ERealMongoshLazyInstall(t *testing.T) {
	if os.Getenv("ANYCLI_E2E_DOWNLOAD") != "1" {
		t.Skip("set ANYCLI_E2E_DOWNLOAD=1 to run the real-download e2e")
	}
	def, err := definitions.LoadBundled("mongodb")
	if err != nil {
		t.Fatalf("load bundled mongodb: %v", err)
	}
	root := t.TempDir()
	t.Setenv("HELIO_BIN_DIR", root)
	// Strip PATH so level ② misses and lazy install is the only route.
	t.Setenv("PATH", filepath.Join(root, "empty-path"))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	var notice bytes.Buffer
	installed, err := binresolve.Resolve(ctx, def.Name, def.Binary, def.Source, binresolve.Options{Notice: &notice})
	if err != nil {
		t.Fatalf("lazy install resolve: %v", err)
	}
	wantPrefix := filepath.Join(root, "versions", "mongodb", def.Source.Version)
	if !strings.HasPrefix(installed, wantPrefix) {
		t.Fatalf("installed path %q not under pin layout %q", installed, wantPrefix)
	}
	if !strings.Contains(notice.String(), "installing") {
		t.Errorf("first-call notice missing install progress line; got %q", notice.String())
	}

	// The installed standalone binary must actually run: version check plus a
	// real --nodb eval round (the exact non-interactive mode the tool uses).
	out, err := exec.CommandContext(ctx, installed, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("exec installed mongosh --version: %v (out=%s)", err, out)
	}
	if !strings.Contains(string(out), def.Source.Version) {
		t.Fatalf("--version output %q does not contain pinned %q", out, def.Source.Version)
	}
	evalOut, err := exec.CommandContext(ctx, installed, "--nodb", "--quiet", "--norc", "--json=relaxed", "--eval", "1+1").CombinedOutput()
	if err != nil {
		t.Fatalf("exec installed mongosh eval: %v (out=%s)", err, evalOut)
	}
	if !strings.Contains(string(evalOut), "2") {
		t.Fatalf("eval output %q does not contain result 2", evalOut)
	}

	// Second resolve must hit the pinned path without re-downloading.
	var notice2 bytes.Buffer
	again, err := binresolve.Resolve(ctx, def.Name, def.Binary, def.Source, binresolve.Options{Notice: &notice2})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if again != installed {
		t.Fatalf("second resolve %q != first %q", again, installed)
	}
	if notice2.Len() != 0 {
		t.Errorf("second resolve emitted install notice %q; want cache hit", notice2.String())
	}
}

// TestGhLazyInstallFailsClosedWithoutSha256 pins the deliberate contract that
// gh remains PATH-provisioned only: its bundled definition declares no sha256
// table, so directInstallable gates it out of the lazy-install route entirely —
// off PATH it fails with the classic "not found in PATH" error before any
// download is attempted, and nothing is written under the pin root.
func TestGhLazyInstallFailsClosedWithoutSha256(t *testing.T) {
	def, err := definitions.LoadBundled("github")
	if err != nil {
		t.Fatalf("load bundled github: %v", err)
	}
	if len(def.Source.SHA256) != 0 {
		t.Skip("github definition gained sha256 pins; PATH-only contract no longer applies")
	}
	root := t.TempDir()
	t.Setenv("HELIO_BIN_DIR", root)
	t.Setenv("PATH", filepath.Join(root, "empty-path"))

	_, err = binresolve.Resolve(context.Background(), def.Name, def.Binary, def.Source, binresolve.Options{})
	if err == nil {
		t.Fatal("resolve succeeded; want fail-closed for a definition without sha256 pins")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("error %q; want the PATH-only miss error (lazy route must not engage)", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "versions")); len(entries) != 0 {
		t.Fatalf("pin root gained entries %v; want no install attempt", entries)
	}
}
