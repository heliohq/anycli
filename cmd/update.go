package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const releaseURL = "https://api.github.com/repos/sheet0/anycli/releases/tags/latest"

type releaseInfo struct {
	Body   string `json:"body"`
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update any to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("checking for updates...")

		release, err := fetchRelease()
		if err != nil {
			return err
		}

		// Extract remote version from asset name: any_<version>_<os>_<arch>.tar.gz
		remoteVersion := extractVersionFromAssets(release)
		currentVersion := rootCmd.Version

		if remoteVersion != "" && remoteVersion == currentVersion {
			fmt.Printf("already up to date (%s)\n", currentVersion)
			return nil
		}

		// Find asset URL for current platform
		assetURL, err := findAssetURLFromRelease(release, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}

		if remoteVersion != "" {
			fmt.Printf("updating %s -> %s\n", currentVersion, remoteVersion)
		} else {
			fmt.Println("downloading latest version...")
		}

		// Download
		resp, err := http.Get(assetURL)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
		}

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, resp.Body); err != nil {
			return err
		}

		// Extract binary from tar.gz
		binary, err := extractBinaryFromTarGz(&buf, "any")
		if err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}

		// Replace self: write to temp file then rename (avoids "text file busy" on Linux)
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}

		tmpPath := execPath + ".tmp"
		if err := os.WriteFile(tmpPath, binary, 0755); err != nil {
			return fmt.Errorf("cannot write binary: %w", err)
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot replace binary: %w", err)
		}

		fmt.Println("updated successfully!")
		return nil
	},
}

func fetchRelease() (*releaseInfo, error) {
	resp, err := http.Get(releaseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// extractVersionFromAssets extracts the version string from the first asset name.
// Asset name format: any_<version>_<os>_<arch>.tar.gz
func extractVersionFromAssets(release *releaseInfo) string {
	for _, a := range release.Assets {
		if !strings.HasPrefix(a.Name, "any_") {
			continue
		}
		// any_v0.0.1-build.41.d998573_darwin_arm64.tar.gz
		name := strings.TrimPrefix(a.Name, "any_")
		// v0.0.1-build.41.d998573_darwin_arm64.tar.gz
		// Find the version part: everything before _<os>_
		for _, os := range []string{"_darwin_", "_linux_", "_windows_"} {
			if idx := strings.Index(name, os); idx > 0 {
				return name[:idx]
			}
		}
	}
	return ""
}

func findAssetURLFromRelease(release *releaseInfo, goos, goarch string) (string, error) {
	platform := goos + "_" + goarch
	for _, a := range release.Assets {
		if strings.Contains(a.Name, platform) {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no release asset found for %s", platform)
}

func extractBinaryFromTarGz(r io.Reader, name string) ([]byte, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg && strings.HasSuffix(header.Name, name) {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
