package credential

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/shipbase/anycli/internal/config"
)

// LoadLocal reads credential values from the local credential file.
// The file format is map[string]string keyed by local_key.
// Returns a map of local_key -> value.
// Returns nil, nil if the credential file does not exist.
func LoadLocal(toolName string) (map[string]string, error) {
	path := filepath.Join(config.CredentialsDir(), toolName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var creds map[string]string
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return creds, nil
}
