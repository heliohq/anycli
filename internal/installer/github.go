package installer

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/shipbase/anycli/internal/registry"
)

// githubRelease represents a GitHub release from the API.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// resolveLatestVersion fetches the latest release tag from GitHub.
func resolveLatestVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned %d for %s", resp.StatusCode, repo)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release: %w", err)
	}

	version := strings.TrimPrefix(release.TagName, "v")
	return version, nil
}

// resolveAssetURL finds the download URL for a specific asset pattern.
func resolveAssetURL(repo, version string, src *registry.SourceConfig, goos, goarch string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/v%s", repo, version)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch release v%s: %w", version, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned %d for %s v%s", resp.StatusCode, repo, version)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release: %w", err)
	}

	expanded := ExpandPattern(src.AssetPattern, version, goos, goarch, src.OsMap, src.ExtMap)

	for _, asset := range release.Assets {
		if asset.Name == expanded {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("asset %q not found in release v%s (expanded from %q)", expanded, version, src.AssetPattern)
}

// downloadFile downloads a URL to a writer.
func downloadFile(url string, w io.Writer) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	_, err = io.Copy(w, resp.Body)
	return err
}

// ExpandPattern replaces {version}, {os}, {arch}, {ext} placeholders.
func ExpandPattern(pattern, version, goos, goarch string, osMap, extMap map[string]string) string {
	osName := goos
	if mapped, ok := osMap[goos]; ok {
		osName = mapped
	}

	archName := goarch

	ext := ".tar.gz" // default
	if mapped, ok := extMap[goos]; ok {
		ext = mapped
	}

	s := strings.ReplaceAll(pattern, "{version}", version)
	s = strings.ReplaceAll(s, "{os}", osName)
	s = strings.ReplaceAll(s, "{arch}", archName)
	s = strings.ReplaceAll(s, "{ext}", ext)
	return s
}
