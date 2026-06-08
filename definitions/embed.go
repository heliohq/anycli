// Package definitions holds the tool definitions AnyCLI ships, embedded at
// build time. These definitions are internal to AnyCLI — the embedder never
// supplies them. The real Helio tool definitions are added here (as
// tools/<name>.json) in a later round; none ship yet.
package definitions

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/shipbase/anycli/internal/registry"
)

// fs embeds the tools/ directory. The directory is embedded (rather than a
// *.json glob) so the build stays green with zero definition files — only the
// directory's README placeholder is required to exist.
//
//go:embed tools
var fs embed.FS

// LoadBundled loads an embedded tool definition by name from tools/<name>.json.
// An unknown name (no embedded definition) returns an error. With zero
// definitions shipped, every lookup returns the not-found error — that is
// expected until the real definitions are added.
func LoadBundled(name string) (*registry.Definition, error) {
	data, err := fs.ReadFile("tools/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("no bundled definition for %q", name)
	}

	var def registry.Definition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid bundled definition for %q: %w", name, err)
	}
	return &def, nil
}
