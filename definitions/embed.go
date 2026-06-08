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
