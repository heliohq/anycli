package definitions

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/shipbase/anycli/internal/registry"
)

//go:embed *.json
var fs embed.FS

// LoadBundled loads a bundled wrapper definition by name.
func LoadBundled(name string) (*registry.Definition, error) {
	data, err := fs.ReadFile(name + ".json")
	if err != nil {
		return nil, fmt.Errorf("no bundled definition for %q", name)
	}

	var def registry.Definition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid bundled definition for %q: %w", name, err)
	}
	return &def, nil
}

// List returns all bundled definition names.
func List() ([]string, error) {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if len(n) > 5 && n[len(n)-5:] == ".json" {
			names = append(names, n[:len(n)-5])
		}
	}
	return names, nil
}
