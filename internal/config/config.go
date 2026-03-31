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

// BinDir returns the shim binary directory.
func BinDir() string {
	return filepath.Join(Dir(), "bin")
}

// RegistryDir returns the wrapper definitions directory.
func RegistryDir() string {
	return filepath.Join(Dir(), "registry")
}

// CredentialsDir returns the credentials directory.
func CredentialsDir() string {
	return filepath.Join(Dir(), "credentials")
}

// ToolsDir returns the directory for downloaded tool binaries.
func ToolsDir() string {
	return filepath.Join(Dir(), "tools")
}

// CacheDir returns the credential cache directory.
func CacheDir() string {
	return filepath.Join(Dir(), "cache")
}

// TmpDir returns the temporary files directory.
func TmpDir() string {
	return filepath.Join(Dir(), "tmp")
}

// EnsureDirs creates all required directories.
func EnsureDirs() error {
	dirs := []string{BinDir(), RegistryDir(), CredentialsDir(), ToolsDir(), CacheDir(), TmpDir()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
