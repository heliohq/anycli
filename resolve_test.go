package anycli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests pin the exported warm surface (a host consumes
// WarmEligibleTools + ResolveToolBinary through a plain go.mod bump
// — no internal packages, no re-derivation of eligibility on the host side).

func TestWarmEligibleToolsIsGithubAndLark(t *testing.T) {
	tools, err := WarmEligibleTools()
	if err != nil {
		t.Fatalf("WarmEligibleTools: %v", err)
	}
	// github and lark: both cli-type with a direct source and a full sha256
	// table. mongodb ships one too but is a service tool (single consumer,
	// in-process resolution only), so it stays out. A new entry appearing here
	// is a deliberate contract change — hosts symlink every listed binary onto
	// the engine PATH, and a host that used to provision one itself must drop
	// that provisioning in the same change or keep shadowing the pin.
	want := []WarmTool{{Name: Tool("github"), Binary: "gh"}, {Name: Tool("lark"), Binary: "lark-cli"}}
	if len(tools) != len(want) {
		t.Fatalf("warm-eligible set = %+v; want %+v", tools, want)
	}
	for i, w := range want {
		if tools[i] != w {
			t.Errorf("warm-eligible[%d] = %+v, want %+v", i, tools[i], w)
		}
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
