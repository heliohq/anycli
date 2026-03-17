package installer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shipbase/anycli/internal/config"
	"github.com/shipbase/anycli/internal/registry"
)

// Result holds the outcome of an install operation.
type Result struct {
	Version    string
	BinaryPath string
}

// Install downloads and installs a tool binary based on its source config.
func Install(def *registry.Definition) (*Result, error) {
	if def.Source == nil {
		return nil, fmt.Errorf("no source config for %s", def.Name)
	}

	switch def.Source.Type {
	case "github-release":
		return installFromGitHub(def)
	case "npm":
		return installFromNpm(def)
	default:
		return nil, fmt.Errorf("unknown source type: %s", def.Source.Type)
	}
}

func installFromGitHub(def *registry.Definition) (*Result, error) {
	src := def.Source

	// Resolve version
	version := src.Version
	if version == "" {
		var err error
		version, err = resolveLatestVersion(src.Repo)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("downloading %s v%s...\n", def.Name, version)

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Find the asset download URL
	assetURL, err := resolveAssetURL(src.Repo, version, src, goos, goarch)
	if err != nil {
		return nil, err
	}

	// Download to memory
	var buf bytes.Buffer
	if err := downloadFile(assetURL, &buf); err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	// Determine install directory
	toolDir := filepath.Join(config.ToolsDir(), def.Name, version)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		return nil, err
	}

	// Extract based on file extension
	assetName := ExpandPattern(src.AssetPattern, version, goos, goarch, src.OsMap, src.ExtMap)
	switch {
	case strings.HasSuffix(assetName, ".tar.gz"), strings.HasSuffix(assetName, ".tgz"):
		if err := extractTarGz(&buf, toolDir); err != nil {
			return nil, fmt.Errorf("extraction failed: %w", err)
		}
	case strings.HasSuffix(assetName, ".zip"):
		if err := extractZip(buf.Bytes(), toolDir); err != nil {
			return nil, fmt.Errorf("extraction failed: %w", err)
		}
	default:
		// Assume raw binary
		binaryDst := filepath.Join(toolDir, def.Binary)
		if err := os.WriteFile(binaryDst, buf.Bytes(), 0755); err != nil {
			return nil, err
		}
	}

	// Resolve the binary path inside the extracted archive
	binaryPath := filepath.Join(toolDir, ExpandPattern(src.BinaryPath, version, goos, goarch, src.OsMap, src.ExtMap))
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("binary not found at expected path %s: %w", binaryPath, err)
	}

	// Ensure executable
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return nil, err
	}

	return &Result{
		Version:    version,
		BinaryPath: binaryPath,
	}, nil
}

func extractTarGz(r io.Reader, dst string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func extractZip(data []byte, dst string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	for _, f := range r.File {
		target := filepath.Join(dst, f.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid zip path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}
