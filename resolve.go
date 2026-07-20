// resolve.go is the exported binary-resolution surface for host-side
// pre-warming (design: Helio 313 §gh 纳入懒安装). A host (heliox `tool warm`)
// enumerates WarmEligibleTools, resolves each with ResolveToolBinary — which
// triggers the sha256-verified lazy install when levels ①/② miss — and
// symlinks the resulting binary onto the engine PATH. Nothing here duplicates
// the engine's own resolution: both funnel into internal/exec.ResolveBinary.
package anycli

import (
	"context"
	"fmt"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/exec"
	"github.com/heliohq/anycli/internal/exec/binresolve"
)

// WarmTool is one bundled tool a host may pre-warm: the tool's definition
// name plus the base name of the external binary it wraps (e.g. {github, gh}).
type WarmTool struct {
	Name   Tool
	Binary string
}

// WarmEligibleTools returns, in definition order, every bundled CLI-type tool
// whose source can be lazily installed on the current platform (official
// direct download + pinned version + a sha256 digest for this platform).
// Service-type tools are excluded even when lazily installable — their binary
// is resolved in-process at Execute time only, never handed out for PATH
// exposure.
func WarmEligibleTools() ([]WarmTool, error) {
	bundled, err := definitions.ListBundled()
	if err != nil {
		return nil, fmt.Errorf("list bundled definitions: %w", err)
	}
	var eligible []WarmTool
	for _, def := range bundled {
		if def.Type != "" && def.Type != string(ToolKindCLI) {
			continue
		}
		if def.Binary == "" || !binresolve.LazyInstallable(def.Source) {
			continue
		}
		eligible = append(eligible, WarmTool{Name: Tool(def.Name), Binary: def.Binary})
	}
	return eligible, nil
}

// ResolveToolBinary returns the absolute path of a bundled CLI tool's
// underlying binary via the engine's three-level resolution: pinned-versions
// dir → PATH (skipping the anycli shim dir) → sha256-verified lazy install.
// A first call on a cold host may download the pinned release archive (level
// ③); progress notes go to stderr. Unknown tools and service-type tools are
// errors.
func ResolveToolBinary(ctx context.Context, tool Tool) (string, error) {
	def, err := definitions.LoadBundled(string(tool))
	if err != nil {
		return "", err
	}
	if def.Type == string(ToolKindService) {
		return "", fmt.Errorf("tool %q is a service tool; its binary is resolved in-process only", tool)
	}
	path, err := exec.ResolveBinary(ctx, def)
	if err != nil {
		return "", fmt.Errorf("resolve %s binary: %w", tool, err)
	}
	return path, nil
}
