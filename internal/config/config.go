package config

import (
	"os"
	"path/filepath"
)

// Dir returns the anycli home directory (~/.anycli).
func Dir() string {
	if d := os.Getenv("ANYCLI_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".anycli")
	}
	return filepath.Join(home, ".anycli")
}

// BinDir returns the directory that may hold an AnyCLI shim. resolveBinary
// skips it during PATH search so a future shim never resolves to itself.
func BinDir() string {
	return filepath.Join(Dir(), "bin")
}

// CacheDir returns the credential cache directory.
func CacheDir() string {
	return filepath.Join(Dir(), "cache")
}

// TmpDir returns the directory for ephemeral file-injection temp files.
func TmpDir() string {
	return filepath.Join(Dir(), "tmp")
}
