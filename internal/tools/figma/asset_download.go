package figma

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/heliohq/anycli/internal/tools/execution"
)

const (
	maxAssetBytes       = 100 << 20
	assetDownloadWorker = 4
)

type assetSource struct {
	ID        string
	URL       string
	Extension string
	BaseName  string
}

type downloadedAsset struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
}

type assetManifest struct {
	OutputDir string            `json:"output_dir"`
	Assets    []downloadedAsset `json:"assets"`
}

type assetDownloadResult struct {
	Asset downloadedAsset
	Err   error
}

func (s *Service) downloadAssets(ctx context.Context, sources []assetSource, outputDir string, overwrite bool) (assetManifest, error) {
	if outputDir == "" {
		return assetManifest{}, fmt.Errorf("output directory is required")
	}
	if err := s.FS.MkdirAll(outputDir, 0o755); err != nil {
		return assetManifest{}, fmt.Errorf("create asset output directory: %w", err)
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left].ID < sources[right].ID })
	assignUniqueAssetNames(sources)
	results := make([]assetDownloadResult, len(sources))
	jobs := make(chan int)
	workers := min(assetDownloadWorker, len(sources))
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			for index := range jobs {
				asset, err := s.downloadAsset(ctx, sources[index], outputDir, overwrite)
				results[index] = assetDownloadResult{Asset: asset, Err: err}
			}
		}()
	}
	for index := range sources {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	manifest := assetManifest{OutputDir: outputDir, Assets: make([]downloadedAsset, 0, len(sources))}
	for index, result := range results {
		if result.Err != nil {
			return assetManifest{}, fmt.Errorf("download Figma asset %s: %w", sources[index].ID, result.Err)
		}
		manifest.Assets = append(manifest.Assets, result.Asset)
	}
	return manifest, nil
}

func (s *Service) downloadAsset(ctx context.Context, source assetSource, outputDir string, overwrite bool) (downloadedAsset, error) {
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return downloadedAsset{}, fmt.Errorf("provider returned an invalid asset URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return downloadedAsset{}, fmt.Errorf("build asset request: %w", err)
	}
	hc := s.HC
	if hc == nil {
		hc = defaultHTTPClient
	}
	hc = withRedirectPolicy(hc, httpsAssetRedirect)
	response, err := hc.Do(req)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return downloadedAsset{}, fmt.Errorf("fetch signed asset URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return downloadedAsset{}, fmt.Errorf("signed asset URL returned HTTP %d", response.StatusCode)
	}
	contentType := normalizedContentType(response.Header.Get("Content-Type"))
	extension := source.Extension
	if extension == "" {
		extension = extensionForContentType(contentType)
	}
	targetName := source.BaseName + extension
	targetPath := filepath.Join(outputDir, targetName)
	// --overwrite is figma's own semantic, so figma is what expresses it rather
	// than every caller of the filesystem seam having to.
	//
	// The check is not atomic. It was, once: the asset used to be installed
	// with os.Link, which fails if the name is taken. What is still guaranteed
	// is that a run does not clobber assets that were already there; what is
	// not is two runs downloading the same asset at the same moment, where both
	// can find nothing and the later Close wins. Expressing that would need
	// exclusive creation in the seam, which is a decision about the interface,
	// not about figma.
	if !overwrite {
		if _, statErr := s.FS.Stat(targetPath); statErr == nil {
			return downloadedAsset{}, fmt.Errorf("%s already exists; pass --overwrite to replace it", targetName)
		} else if !errors.Is(statErr, execution.ErrNotExist) {
			return downloadedAsset{}, fmt.Errorf("install asset %s: %w", targetName, statErr)
		}
	}
	// Create commits on Close, so a copy that fails or overruns the limit
	// leaves whatever was at targetPath untouched.
	asset, err := s.FS.Create(targetPath)
	if err != nil {
		return downloadedAsset{}, fmt.Errorf("create asset: %w", err)
	}
	written, copyErr := io.Copy(asset, io.LimitReader(response.Body, maxAssetBytes+1))
	if copyErr != nil {
		execution.Abandon(asset)
		return downloadedAsset{}, fmt.Errorf("write asset: %w", copyErr)
	}
	if written > maxAssetBytes {
		execution.Abandon(asset)
		return downloadedAsset{}, fmt.Errorf("asset exceeds %d bytes", maxAssetBytes)
	}
	if err := asset.Close(); err != nil {
		return downloadedAsset{}, fmt.Errorf("install asset %s: %w", targetName, err)
	}
	return downloadedAsset{ID: source.ID, File: targetName, Bytes: written, ContentType: contentType}, nil
}

func httpsAssetRedirect(req *http.Request, _ []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("figma asset redirect must use HTTPS")
	}
	return nil
}

func assignUniqueAssetNames(sources []assetSource) {
	seen := map[string]struct{}{}
	for index := range sources {
		name := sanitizeAssetName(sources[index].ID)
		if _, exists := seen[name]; exists {
			digest := sha256.Sum256([]byte(sources[index].ID))
			name += "_" + hex.EncodeToString(digest[:4])
		}
		seen[name] = struct{}{}
		sources[index].BaseName = name
	}
}

func sanitizeAssetName(value string) string {
	value = strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '.' || char == '-' || char == '_' {
			return char
		}
		return '_'
	}, value)
	value = strings.Trim(value, ".")
	if value == "" {
		return "asset"
	}
	const maxBaseNameBytes = 120
	if len(value) > maxBaseNameBytes {
		digest := sha256.Sum256([]byte(value))
		prefixBytes := maxBaseNameBytes - 9
		for len(value) > prefixBytes {
			_, size := utf8.DecodeLastRuneInString(value)
			value = value[:len(value)-size]
		}
		value += "_" + hex.EncodeToString(digest[:4])
	}
	return value
}

func normalizedContentType(value string) string {
	contentType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return contentType
}

func extensionForContentType(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/svg+xml":
		return ".svg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}
