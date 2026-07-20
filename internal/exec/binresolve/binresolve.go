// Package binresolve resolves a tool's underlying binary through three levels:
//
//	① the pinned-versions directory: <pin root>/versions/<tool>/<version>/<platform>/<binary>
//	② PATH scan (skipping the anycli shim directory)
//	③ lazy install from the official direct-download source, then ①
//
// The pin root is the first of HELIO_BIN_DIR, $HELIO_HOME/bin, ~/.helio/bin.
// Lazy install (level ③) only runs for definitions whose SourceConfig declares
// type "direct" with a url_template and a pinned version; the per-platform
// sha256 is mandatory and a mismatch discards the download (fail fast — no
// mirror, no fallback source). Tools without a direct source (e.g. gh) keep
// today's PATH-only behavior and error text.
package binresolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/heliohq/anycli/internal/registry"
)

// installTimeout bounds one lazy install (download + verify + extract) when
// the caller's context has no tighter deadline. Install deliberately runs
// outside the per-invocation --timeout budget, so without this bound a stalled
// download (TCP established, server stops sending) would hang forever. It also
// transitively bounds a concurrent installer waiting on the file lock: the
// holder releases within this window.
const installTimeout = 10 * time.Minute

// Downloader fetches one URL and returns the response body. Tests inject
// fixture bytes; production uses plain HTTPS against the official host.
type Downloader func(ctx context.Context, url string) (io.ReadCloser, error)

// Options tunes one Resolve call.
type Options struct {
	// Downloader overrides the HTTP downloader; nil uses HTTPS.
	Downloader Downloader
	// Notice receives human-readable progress notes (first-call install);
	// nil writes to os.Stderr.
	Notice io.Writer
	// SkipPATHDir is excluded from the PATH scan (the anycli shim dir), so a
	// shim never resolves to itself.
	SkipPATHDir string
}

func (o Options) downloader() Downloader {
	if o.Downloader != nil {
		return o.Downloader
	}
	return httpDownload
}

func (o Options) notice() io.Writer {
	if o.Notice != nil {
		return o.Notice
	}
	return os.Stderr
}

// Resolve returns an absolute path to the tool's binary using the three-level
// resolution above.
func Resolve(ctx context.Context, toolName, binaryName string, src *registry.SourceConfig, opts Options) (string, error) {
	// ① Pinned-versions directory.
	if pinned, ok := pinnedPath(toolName, binaryName, src); ok {
		if info, err := os.Stat(pinned); err == nil && !info.IsDir() {
			return pinned, nil
		}
	}

	// ② PATH scan, skipping the shim directory.
	if found, ok := searchPATH(binaryName, opts.SkipPATHDir); ok {
		return found, nil
	}

	// ③ Lazy install from the official direct source, then ①.
	if !directInstallable(src) {
		return "", fmt.Errorf("%s not found in PATH", binaryName)
	}
	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	installed, err := install(ctx, toolName, binaryName, src, opts)
	if err != nil {
		return "", fmt.Errorf("install %s %s: %w", binaryName, src.Version, err)
	}
	return installed, nil
}

// PinRoot returns the pinned-binaries root: the first of HELIO_BIN_DIR,
// $HELIO_HOME/bin, ~/.helio/bin.
func PinRoot() string {
	if d := os.Getenv("HELIO_BIN_DIR"); d != "" {
		return d
	}
	if h := os.Getenv("HELIO_HOME"); h != "" {
		return filepath.Join(h, "bin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".helio", "bin")
	}
	return filepath.Join(home, ".helio", "bin")
}

// Platform returns the release-naming platform key "<os>-<arch>" for the
// current runtime, applying the source's os/arch maps (e.g. amd64 -> x64).
func Platform(src *registry.SourceConfig) string {
	return platformFor(src, runtime.GOOS, runtime.GOARCH)
}

func platformFor(src *registry.SourceConfig, goos, goarch string) string {
	osName, arch := mappedPlatform(src, goos, goarch)
	return osName + "-" + arch
}

// mappedPlatform applies the source's os/arch maps to a Go platform pair,
// defaulting to the Go names. A nil source maps identically.
func mappedPlatform(src *registry.SourceConfig, goos, goarch string) (osName, arch string) {
	osName, arch = goos, goarch
	if src == nil {
		return osName, arch
	}
	if m, ok := src.OsMap[goos]; ok {
		osName = m
	}
	if m, ok := src.ArchMap[goarch]; ok {
		arch = m
	}
	return osName, arch
}

// DownloadURL expands the source's url_template for the current platform.
// src must be non-nil — callers gate on directInstallable first.
func DownloadURL(src *registry.SourceConfig) string {
	return expand(src.URLTemplate, src)
}

// pinnedPath computes the level-① path. A source without a pinned version has
// no pinned path.
func pinnedPath(toolName, binaryName string, src *registry.SourceConfig) (string, bool) {
	if src == nil || src.Version == "" {
		return "", false
	}
	return filepath.Join(PinRoot(), "versions", toolName, src.Version, Platform(src), binaryName+exeSuffix()), true
}

// searchPATH scans PATH for the binary, skipping skipDir.
func searchPATH(binaryName, skipDir string) (string, bool) {
	name := binaryName + exeSuffix()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || dir == skipDir {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// directInstallable reports whether the source declares an official
// direct-download that lazy install can use.
func directInstallable(src *registry.SourceConfig) bool {
	return src != nil && src.Type == "direct" && src.URLTemplate != "" && src.Version != ""
}

// expand substitutes {version}, {os}, {arch}, {ext}, and {exe} in a template.
// src must be non-nil — callers gate on directInstallable first.
func expand(template string, src *registry.SourceConfig) string {
	return expandFor(template, src, runtime.GOOS, runtime.GOARCH)
}

func expandFor(template string, src *registry.SourceConfig, goos, goarch string) string {
	osName, arch := mappedPlatform(src, goos, goarch)
	r := strings.NewReplacer(
		"{version}", src.Version,
		"{os}", osName,
		"{arch}", arch,
		"{ext}", src.ExtMap[goos],
		"{exe}", exeSuffixFor(goos),
	)
	return r.Replace(template)
}

func exeSuffix() string {
	return exeSuffixFor(runtime.GOOS)
}

func exeSuffixFor(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// httpDownload is the production Downloader: plain HTTPS against the official
// host, no mirror, no fallback.
func httpDownload(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

// verifySHA256 compares a computed digest with the pinned hex digest.
func verifySHA256(sum [sha256.Size]byte, want string) error {
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}
