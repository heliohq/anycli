package anycli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests pin the exported warm surface (design: heliox `tool warm`
// consumes WarmEligibleTools + ResolveToolBinary through a plain go.mod bump
// — no internal packages, no re-derivation of eligibility on the host side).

func TestWarmEligibleToolsIsExactlyGithub(t *testing.T) {
	tools, err := WarmEligibleTools()
	if err != nil {
		t.Fatalf("WarmEligibleTools: %v", err)
	}
	// Exactly github today: mongodb also ships a full sha256 table but is a
	// service tool (single consumer, in-process resolution only), and lark is
	// cli-type without a direct-download source. A new entry appearing here is
	// a deliberate contract change — hosts symlink every listed binary onto
	// the engine PATH.
	if len(tools) != 1 || tools[0].Name != Tool("github") || tools[0].Binary != "gh" {
		t.Fatalf("warm-eligible set = %+v; want exactly [{github gh}]", tools)
	}
}

func TestResolveToolBinaryUnknownTool(t *testing.T) {
	_, err := ResolveToolBinary(context.Background(), Tool("definitely-not-a-shipped-tool"))
	if err == nil || !strings.Contains(err.Error(), "no bundled definition") {
		t.Fatalf("want no-bundled-definition error, got %v", err)
	}
}

func TestResolveToolBinaryRejectsServiceTool(t *testing.T) {
	// mongodb declares a direct source + sha256 table, but it is a service
	// tool: its binary is resolved in-process at Execute time only, never
	// handed out for PATH exposure.
	_, err := ResolveToolBinary(context.Background(), Tool("mongodb"))
	if err == nil || !strings.Contains(err.Error(), "service") {
		t.Fatalf("want service-tool rejection, got %v", err)
	}
}

func TestResolveToolBinaryPATHHit(t *testing.T) {
	binDir := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name = "gh.exe"
	}
	fake := filepath.Join(binDir, name)
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Empty pin root so level ① misses and level ② (PATH) hits the fake —
	// no network, no lazy install.
	t.Setenv("HELIO_BIN_DIR", t.TempDir())
	t.Setenv("PATH", binDir)

	got, err := ResolveToolBinary(context.Background(), Tool("github"))
	if err != nil {
		t.Fatalf("ResolveToolBinary: %v", err)
	}
	if got != fake {
		t.Fatalf("resolved %q; want PATH hit %q", got, fake)
	}
}
