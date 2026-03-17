package cmd

import (
	"bytes"
	"compress/gzip"
	"archive/tar"
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

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update anycli to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("checking for updates...")

		// Find the asset URL for current platform
		assetURL, err := findAssetURL(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}

		fmt.Println("downloading latest version...")

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
		binary, err := extractBinaryFromTarGz(&buf, "anycli")
		if err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}

		// Replace self
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine binary path: %w", err)
		}

		if err := os.WriteFile(execPath, binary, 0755); err != nil {
			return fmt.Errorf("cannot write binary: %w", err)
		}

		fmt.Println("updated successfully!")
		return nil
	},
}

func findAssetURL(goos, goarch string) (string, error) {
	resp, err := http.Get(releaseURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

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
