package format

import (
	"fmt"
	"os"
	"path/filepath"
)

// PatchFile patches specific fields in a file.
// If the file doesn't exist, creates it with the given fields.
// format: "yaml", "json", "toml", "ini"
// fields: map of dot-path -> value (e.g., "github.com.oauth_token" -> "xxx")
func PatchFile(path, fileFormat string, fields map[string]string, mode os.FileMode) error {
	h, err := getHandler(fileFormat)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var out []byte
	if os.IsNotExist(err) || len(existing) == 0 {
		out, err = h.Create(fields)
	} else {
		out, err = h.Patch(existing, fields)
	}
	if err != nil {
		return fmt.Errorf("patching %s as %s: %w", path, fileFormat, err)
	}

	return os.WriteFile(path, out, mode)
}

// CleanupFields removes credential fields from a file.
// For vault mode cleanup: removes only the specific dot-path fields that were injected.
func CleanupFields(path, fileFormat string, fields map[string]string) error {
	h, err := getHandler(fileFormat)
	if err != nil {
		return err
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to clean
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	out, err := h.Remove(existing, fields)
	if err != nil {
		return fmt.Errorf("cleaning fields from %s: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, info.Mode())
}

// handler defines the interface each format must implement.
type handler interface {
	// Create generates a new file from a fields map.
	Create(fields map[string]string) ([]byte, error)
	// Patch applies fields to an existing file's contents.
	Patch(existing []byte, fields map[string]string) ([]byte, error)
	// Remove deletes specific fields from an existing file.
	Remove(existing []byte, fields map[string]string) ([]byte, error)
}

func getHandler(fileFormat string) (handler, error) {
	switch fileFormat {
	case "json":
		return jsonHandler{}, nil
	case "yaml", "yml":
		return yamlHandler{}, nil
	case "toml":
		return tomlHandler{}, nil
	case "ini":
		return iniHandler{}, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", fileFormat)
	}
}
