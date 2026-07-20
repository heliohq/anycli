package binresolve

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/registry"
)

// mongoshSource mirrors the shape of definitions/tools/mongodb.json with a
// synthetic sha256 table filled per test.
func mongoshSource(sha map[string]string) *registry.SourceConfig {
	return &registry.SourceConfig{
		Type:        "direct",
		URLTemplate: "https://downloads.mongodb.com/compass/mongosh-{version}-{os}-{arch}{ext}",
		BinaryPath:  "mongosh-{version}-{os}-{arch}/bin/mongosh{exe}",
		Version:     "2.9.2",
		OsMap:       map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win32"},
		ArchMap:     map[string]string{"amd64": "x64", "arm64": "arm64"},
		ExtMap:      map[string]string{"darwin": ".zip", "linux": ".tgz", "windows": ".zip"},
		SHA256:      sha,
	}
}

// declarativeSource mirrors a github-release definition (e.g. lark.json):
// declarative provisioning metadata only, no direct-download support.
func declarativeSource() *registry.SourceConfig {
	return &registry.SourceConfig{
		Type:         "github-release",
		Repo:         "larksuite/cli",
		AssetPattern: "lark-cli_{version}_{os}_{arch}{ext}",
		BinaryPath:   "lark-cli_{version}_{os}_{arch}/bin/lark-cli",
		Version:      "1.0.71",
	}
}

// setupPinRoot points HELIO_BIN_DIR at a fresh temp dir and empties PATH-adjacent env.
func setupPinRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HELIO_BIN_DIR", root)
	return root
}

// fatalDownloader fails the test if lazy install is ever attempted.
func fatalDownloader(t *testing.T) Downloader {
	return func(context.Context, string) (io.ReadCloser, error) {
		t.Fatal("downloader must not be called")
		return nil, nil
	}
}

// bytesDownloader serves fixed archive bytes and counts invocations.
func bytesDownloader(data []byte, calls *atomic.Int64) Downloader {
	return func(context.Context, string) (io.ReadCloser, error) {
		if calls != nil {
			calls.Add(1)
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
}

// makeTgz builds an in-memory .tgz fixture.
func makeTgz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// makeZip builds an in-memory .zip fixture.
func makeZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestPinRootPrecedence(t *testing.T) {
	t.Setenv("HELIO_BIN_DIR", "/explicit/bin")
	t.Setenv("HELIO_HOME", "/helio/home")
	if got := PinRoot(); got != "/explicit/bin" {
		t.Errorf("PinRoot with HELIO_BIN_DIR = %q, want /explicit/bin", got)
	}

	t.Setenv("HELIO_BIN_DIR", "")
	if got := PinRoot(); got != filepath.Join("/helio/home", "bin") {
		t.Errorf("PinRoot with HELIO_HOME = %q, want /helio/home/bin", got)
	}

	t.Setenv("HELIO_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := PinRoot(); got != filepath.Join(home, ".helio", "bin") {
		t.Errorf("PinRoot default = %q, want %s/.helio/bin", got, home)
	}
}

func TestDownloadURLTable(t *testing.T) {
	src := mongoshSource(nil)
	cases := []struct {
		goos, goarch string
		wantURL      string
		wantEntry    string
		wantPlatform string
	}{
		{"darwin", "arm64",
			"https://downloads.mongodb.com/compass/mongosh-2.9.2-darwin-arm64.zip",
			"mongosh-2.9.2-darwin-arm64/bin/mongosh", "darwin-arm64"},
		{"darwin", "amd64",
			"https://downloads.mongodb.com/compass/mongosh-2.9.2-darwin-x64.zip",
			"mongosh-2.9.2-darwin-x64/bin/mongosh", "darwin-x64"},
		{"linux", "amd64",
			"https://downloads.mongodb.com/compass/mongosh-2.9.2-linux-x64.tgz",
			"mongosh-2.9.2-linux-x64/bin/mongosh", "linux-x64"},
		{"linux", "arm64",
			"https://downloads.mongodb.com/compass/mongosh-2.9.2-linux-arm64.tgz",
			"mongosh-2.9.2-linux-arm64/bin/mongosh", "linux-arm64"},
		{"windows", "amd64",
			"https://downloads.mongodb.com/compass/mongosh-2.9.2-win32-x64.zip",
			"mongosh-2.9.2-win32-x64/bin/mongosh.exe", "win32-x64"},
	}
	for _, c := range cases {
		if got := expandFor(src.URLTemplate, src, c.goos, c.goarch); got != c.wantURL {
			t.Errorf("url(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantURL)
		}
		if got := expandFor(src.BinaryPath, src, c.goos, c.goarch); got != c.wantEntry {
			t.Errorf("entry(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantEntry)
		}
		if got := platformFor(src, c.goos, c.goarch); got != c.wantPlatform {
			t.Errorf("platform(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantPlatform)
		}
	}
}

// TestGitHubDownloadURLTable pins the bundled github definition's expansion
// against gh's official release asset naming (verified from the v2.96.0
// checksums file): macOS/linux/windows OS names, native amd64/arm64 arches,
// zip for macOS+windows and tar.gz for linux, and the windows zip's root-level
// bin/gh.exe entry (no versioned top dir) via binary_path_map.
func TestGitHubDownloadURLTable(t *testing.T) {
	def, err := definitions.LoadBundled("github")
	if err != nil {
		t.Fatalf("load bundled github: %v", err)
	}
	src := def.Source
	cases := []struct {
		goos, goarch string
		wantURL      string
		wantEntry    string
		wantPlatform string
	}{
		{"darwin", "arm64",
			"https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_macOS_arm64.zip",
			"gh_2.96.0_macOS_arm64/bin/gh", "macOS-arm64"},
		{"darwin", "amd64",
			"https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_macOS_amd64.zip",
			"gh_2.96.0_macOS_amd64/bin/gh", "macOS-amd64"},
		{"linux", "amd64",
			"https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_linux_amd64.tar.gz",
			"gh_2.96.0_linux_amd64/bin/gh", "linux-amd64"},
		{"linux", "arm64",
			"https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_linux_arm64.tar.gz",
			"gh_2.96.0_linux_arm64/bin/gh", "linux-arm64"},
		{"windows", "amd64",
			"https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_windows_amd64.zip",
			"bin/gh.exe", "windows-amd64"},
	}
	for _, c := range cases {
		if got := expandFor(src.URLTemplate, src, c.goos, c.goarch); got != c.wantURL {
			t.Errorf("url(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantURL)
		}
		tmpl := src.BinaryPath
		if m, ok := src.BinaryPathMap[c.goos]; ok && m != "" {
			tmpl = m
		}
		if got := expandFor(tmpl, src, c.goos, c.goarch); got != c.wantEntry {
			t.Errorf("entry(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantEntry)
		}
		if got := platformFor(src, c.goos, c.goarch); got != c.wantPlatform {
			t.Errorf("platform(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.wantPlatform)
		}
		if src.SHA256[c.wantPlatform] == "" {
			t.Errorf("sha256 missing for platform %s", c.wantPlatform)
		}
	}
}

func TestResolvePinnedPathWins(t *testing.T) {
	root := setupPinRoot(t)
	t.Setenv("PATH", "")
	src := mongoshSource(map[string]string{Platform(mongoshSource(nil)): "unused"})

	pinned := filepath.Join(root, "versions", "mongodb", "2.9.2", Platform(src), "mongosh"+exeSuffix())
	if err := os.MkdirAll(filepath.Dir(pinned), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinned, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: fatalDownloader(t)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != pinned {
		t.Errorf("Resolve = %q, want pinned %q", got, pinned)
	}
}

func TestResolvePATHFallback(t *testing.T) {
	setupPinRoot(t)
	binDir := t.TempDir()
	real := filepath.Join(binDir, "mongosh"+exeSuffix())
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	got, err := Resolve(context.Background(), "mongodb", "mongosh", mongoshSource(map[string]string{}), Options{Downloader: fatalDownloader(t)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != real {
		t.Errorf("Resolve = %q, want PATH hit %q", got, real)
	}
}

func TestResolveSkipsShimDir(t *testing.T) {
	setupPinRoot(t)
	shimDir := t.TempDir()
	realDir := t.TempDir()
	for _, dir := range []string{shimDir, realDir} {
		if err := os.WriteFile(filepath.Join(dir, "mytool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+realDir)

	got, err := Resolve(context.Background(), "mytool", "mytool", nil, Options{SkipPATHDir: shimDir, Downloader: fatalDownloader(t)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != filepath.Join(realDir, "mytool") {
		t.Errorf("Resolve = %q, want the non-shim path", got)
	}
}

// TestResolveDeclarativeSourcePATHOnlyEquivalence pins that definitions with a
// declarative (github-release) source keep the historical PATH-only behavior:
// lazy install never engages, and the miss error text is unchanged.
func TestResolveDeclarativeSourcePATHOnlyEquivalence(t *testing.T) {
	setupPinRoot(t)
	t.Setenv("PATH", t.TempDir())

	// Not in PATH and no direct source: identical error to the historical
	// PATH-only resolution.
	_, err := Resolve(context.Background(), "lark", "lark-cli", declarativeSource(), Options{Downloader: fatalDownloader(t)})
	if err == nil || err.Error() != "lark-cli not found in PATH" {
		t.Fatalf("err = %v, want \"lark-cli not found in PATH\"", err)
	}

	// In PATH: resolves to the PATH entry, no install involvement.
	binDir := t.TempDir()
	real := filepath.Join(binDir, "lark-cli"+exeSuffix())
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	got, err := Resolve(context.Background(), "lark", "lark-cli", declarativeSource(), Options{Downloader: fatalDownloader(t)})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != real {
		t.Errorf("Resolve = %q, want %q", got, real)
	}
}

// installFixture prepares a source + archive fixture matching the current
// platform for the given archive kind ("tgz" or "zip").
func installFixture(t *testing.T, kind string) (*registry.SourceConfig, []byte, string) {
	t.Helper()
	src := mongoshSource(map[string]string{})
	entry := expand(src.BinaryPath, src)
	content := []byte("#!/bin/sh\necho fixture\n")
	var archive []byte
	switch kind {
	case "tgz":
		archive = makeTgz(t, map[string][]byte{entry: content, "mongosh-x/README": []byte("readme")})
		src.ExtMap = map[string]string{runtime.GOOS: ".tgz"}
	case "zip":
		archive = makeZip(t, map[string][]byte{entry: content, "mongosh-x/README": []byte("readme")})
		src.ExtMap = map[string]string{runtime.GOOS: ".zip"}
	default:
		t.Fatalf("unknown archive kind %q", kind)
	}
	src.SHA256[Platform(src)] = sha256Hex(archive)
	return src, archive, string(content)
}

func TestResolveLazyInstall(t *testing.T) {
	for _, kind := range []string{"tgz", "zip"} {
		t.Run(kind, func(t *testing.T) {
			root := setupPinRoot(t)
			t.Setenv("PATH", "")
			src, archive, wantContent := installFixture(t, kind)

			var calls atomic.Int64
			var notice bytes.Buffer
			opts := Options{Downloader: bytesDownloader(archive, &calls), Notice: &notice}

			got, err := Resolve(context.Background(), "mongodb", "mongosh", src, opts)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			wantPath := filepath.Join(root, "versions", "mongodb", src.Version, Platform(src), "mongosh"+exeSuffix())
			if got != wantPath {
				t.Errorf("installed path = %q, want %q", got, wantPath)
			}
			data, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("read installed binary: %v", err)
			}
			if string(data) != wantContent {
				t.Errorf("installed content = %q, want fixture body", data)
			}
			info, _ := os.Stat(got)
			if info.Mode()&0o111 == 0 {
				t.Error("installed binary is not executable")
			}
			if !strings.Contains(notice.String(), "installing mongosh "+src.Version) {
				t.Errorf("notice = %q, want installing note", notice.String())
			}
			if calls.Load() != 1 {
				t.Errorf("downloads = %d, want 1", calls.Load())
			}

			// Second resolve hits the pinned path without downloading again.
			got2, err := Resolve(context.Background(), "mongodb", "mongosh", src, opts)
			if err != nil {
				t.Fatalf("second Resolve: %v", err)
			}
			if got2 != wantPath || calls.Load() != 1 {
				t.Errorf("second Resolve = %q (downloads %d), want cached path with 1 download", got2, calls.Load())
			}
		})
	}
}

// TestResolveLazyInstallBinaryPathMapOverride pins the per-OS archive-entry
// override: when binary_path_map has an entry for the current OS (the gh
// windows zip lays bin/gh.exe at the archive root), extraction uses it instead
// of the shared binary_path template.
func TestResolveLazyInstallBinaryPathMapOverride(t *testing.T) {
	root := setupPinRoot(t)
	t.Setenv("PATH", "")
	src := mongoshSource(map[string]string{})
	src.BinaryPathMap = map[string]string{runtime.GOOS: "bin/rootlevel{exe}"}
	src.ExtMap = map[string]string{runtime.GOOS: ".zip"}
	entry := expand(src.BinaryPathMap[runtime.GOOS], src)
	sharedEntry := expand(src.BinaryPath, src)
	content := []byte("#!/bin/sh\necho override\n")
	archive := makeZip(t, map[string][]byte{
		entry:       content,
		sharedEntry: []byte("wrong entry — the override must win"),
	})
	src.SHA256[Platform(src)] = sha256Hex(archive)

	got, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: bytesDownloader(archive, nil), Notice: io.Discard})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantPath := filepath.Join(root, "versions", "mongodb", src.Version, Platform(src), "mongosh"+exeSuffix())
	if got != wantPath {
		t.Errorf("installed path = %q, want %q", got, wantPath)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("installed content = %q, want the binary_path_map entry body", data)
	}
}

func TestResolveSha256MismatchFailsAndInstallsNothing(t *testing.T) {
	root := setupPinRoot(t)
	t.Setenv("PATH", "")
	src, archive, _ := installFixture(t, "tgz")
	src.SHA256[Platform(src)] = strings.Repeat("0", 64) // wrong digest

	_, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: bytesDownloader(archive, nil), Notice: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("err = %v, want sha256 mismatch", err)
	}
	pinned := filepath.Join(root, "versions", "mongodb", src.Version, Platform(src), "mongosh"+exeSuffix())
	if _, statErr := os.Stat(pinned); !os.IsNotExist(statErr) {
		t.Errorf("pinned path %q exists after mismatch, want nothing installed", pinned)
	}
}

func TestResolveMissingSha256PlatformFails(t *testing.T) {
	setupPinRoot(t)
	t.Setenv("PATH", "")
	src, archive, _ := installFixture(t, "tgz")
	src.SHA256 = map[string]string{} // no entry for this platform

	_, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: bytesDownloader(archive, nil), Notice: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "no sha256 pinned") {
		t.Fatalf("err = %v, want missing-sha256 failure", err)
	}
}

func TestResolveConcurrentInstallDownloadsOnce(t *testing.T) {
	root := setupPinRoot(t)
	t.Setenv("PATH", "")
	src, archive, _ := installFixture(t, "tgz")

	var calls atomic.Int64
	opts := Options{Downloader: bytesDownloader(archive, &calls), Notice: io.Discard}

	const workers = 4
	var wg sync.WaitGroup
	errs := make([]error, workers)
	paths := make([]string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			paths[i], errs[i] = Resolve(context.Background(), "mongodb", "mongosh", src, opts)
		}(i)
	}
	wg.Wait()

	wantPath := filepath.Join(root, "versions", "mongodb", src.Version, Platform(src), "mongosh"+exeSuffix())
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if paths[i] != wantPath {
			t.Errorf("worker %d path = %q, want %q", i, paths[i], wantPath)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("downloads = %d, want exactly 1 (file lock serializes)", calls.Load())
	}
}

// TestResolveInstallContextHasDeadline pins the install bound: the lazy
// install deliberately runs outside the per-invocation --timeout budget, so
// Resolve must impose its own deadline — otherwise a stalled download (TCP
// established, server silent) would hang the tool call forever.
func TestResolveInstallContextHasDeadline(t *testing.T) {
	setupPinRoot(t)
	t.Setenv("PATH", "")
	src, archive, _ := installFixture(t, "tgz")

	var sawDeadline bool
	dl := func(ctx context.Context, url string) (io.ReadCloser, error) {
		_, sawDeadline = ctx.Deadline()
		return io.NopCloser(bytes.NewReader(archive)), nil
	}
	if _, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: dl, Notice: io.Discard}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !sawDeadline {
		t.Error("download context has no deadline; a stalled download would hang forever")
	}
}

// TestInstallSweepsStalePartials pins the crash-orphan cleanup: partial files
// stranded by a SIGKILL mid-install are removed by the next install instead of
// accumulating forever.
func TestInstallSweepsStalePartials(t *testing.T) {
	root := setupPinRoot(t)
	t.Setenv("PATH", "")
	src, archive, _ := installFixture(t, "tgz")

	versionDir := filepath.Join(root, "versions", "mongodb", src.Version)
	platformDir := filepath.Join(versionDir, Platform(src))
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleDownload := filepath.Join(versionDir, "download-123456.partial")
	staleBinary := filepath.Join(platformDir, "mongosh"+exeSuffix()+".partial")
	for _, f := range []string{staleDownload, staleBinary} {
		if err := os.WriteFile(f, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: bytesDownloader(archive, nil), Notice: io.Discard}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, f := range []string{staleDownload, staleBinary} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("stale partial %q survived the install", f)
		}
	}
}

func TestResolveBinaryEntryMissingFromArchive(t *testing.T) {
	setupPinRoot(t)
	t.Setenv("PATH", "")
	src := mongoshSource(map[string]string{})
	archive := makeTgz(t, map[string][]byte{"unrelated/file": []byte("x")})
	src.ExtMap = map[string]string{runtime.GOOS: ".tgz"}
	src.SHA256[Platform(src)] = sha256Hex(archive)

	_, err := Resolve(context.Background(), "mongodb", "mongosh", src, Options{Downloader: bytesDownloader(archive, nil), Notice: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "not found in archive") {
		t.Fatalf("err = %v, want entry-missing failure", err)
	}
}
