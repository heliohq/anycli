package installer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/registry"
)

func installFromNpm(def *registry.Definition) (*Result, error) {
	src := def.Source

	// Resolve version
	version := src.Version
	if version == "" {
		var err error
		version, err = resolveNpmLatestVersion(src.Repo)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("downloading %s v%s via npm...\n", def.Name, version)

	// Install to ~/.anycli/tools/<name>/<version>/ using npm
	toolDir := filepath.Join(config.ToolsDir(), def.Name, version)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return nil, err
	}

	// npm install <package>@<version> --prefix <toolDir>
	pkg := src.Repo + "@" + version
	cmd := exec.Command("npm", "install", pkg, "--prefix", toolDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("npm install failed: %w", err)
	}

	// Resolve binary path
	binaryName := def.Binary
	if src.BinaryPath != "" {
		binaryName = src.BinaryPath
	}
	binaryPath := filepath.Join(toolDir, "node_modules", ".bin", binaryName)

	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found at %s: %w", binaryPath, err)
	}

	return &Result{
		Version:    version,
		BinaryPath: binaryPath,
	}, nil
}

// resolveNpmLatestVersion fetches the latest version from npm registry.
func resolveNpmLatestVersion(pkg string) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", pkg)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch npm package info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned %d for %s", resp.StatusCode, pkg)
	}

	var info struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}

	return strings.TrimPrefix(info.Version, "v"), nil
}
