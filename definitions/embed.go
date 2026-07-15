// Package definitions holds the tool definitions AnyCLI ships, embedded at
// build time. These definitions are internal to AnyCLI — the embedder never
// supplies them. The design 003 toolset ships under tools/<name>.json:
// slack / notion / google / discord / linkedin / x / figma (service) and
// github (cli).
package definitions

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/heliohq/anycli/internal/registry"
)

// fs embeds the tools/ directory. The directory is embedded (rather than a
// *.json glob) so the build also stays green with zero definition files.
//
//go:embed tools
var fs embed.FS

// LoadBundled loads an embedded tool definition by name from tools/<name>.json.
// An unknown name (no embedded definition) returns an error.
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

// ListBundled loads every embedded JSON definition in deterministic name
// order. Non-JSON files (such as tools/README.md) are ignored.
func ListBundled() ([]*registry.Definition, error) {
	entries, err := fs.ReadDir("tools")
	if err != nil {
		return nil, fmt.Errorf("read bundled definitions: %w", err)
	}

	bundled := make([]*registry.Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".json")
		definition, err := LoadBundled(name)
		if err != nil {
			return nil, err
		}
		if definition.Name != name {
			return nil, fmt.Errorf("bundled definition %q declares name %q", entry.Name(), definition.Name)
		}
		bundled = append(bundled, definition)
	}

	sort.Slice(bundled, func(i, j int) bool {
		return bundled[i].Name < bundled[j].Name
	})
	return bundled, nil
}
