package binresolve

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/heliohq/anycli/internal/registry"
)

// install downloads the pinned archive from the official direct URL, verifies
// its mandatory per-platform sha256, extracts the declared binary entry, and
// atomically places it at the pinned path. A file lock serializes concurrent
// installers of the same (tool, version, platform); the loser re-checks the
// pinned path and returns without downloading.
func install(ctx context.Context, toolName, binaryName string, src *registry.SourceConfig, opts Options) (string, error) {
	platform := Platform(src)
	want, ok := src.SHA256[platform]
	if !ok || want == "" {
		return "", fmt.Errorf("no sha256 pinned for platform %s", platform)
	}

	pinned, ok := pinnedPath(toolName, binaryName, src)
	if !ok {
		return "", errors.New("source has no pinned version")
	}
	versionDir := filepath.Dir(filepath.Dir(pinned)) // versions/<tool>/<version>
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("create install dir: %w", err)
	}

	// Serialize concurrent installs of the same (tool, version, platform).
	release, err := acquireLock(filepath.Join(versionDir, platform+".lock"))
	if err != nil {
		return "", err
	}
	defer release()

	// Another process may have completed the install while we waited.
	if info, err := os.Stat(pinned); err == nil && !info.IsDir() {
		return pinned, nil
	}

	url := DownloadURL(src)
	fmt.Fprintf(opts.notice(), "[anycli] installing %s %s (%s) from %s ...\n", binaryName, src.Version, platform, url)

	archivePath, err := downloadVerified(ctx, url, want, versionDir, opts)
	if err != nil {
		return "", err
	}
	defer os.Remove(archivePath)

	entry := expand(src.BinaryPath, src)
	tmpBin := pinned + ".partial"
	if err := os.MkdirAll(filepath.Dir(pinned), 0o755); err != nil {
		return "", fmt.Errorf("create platform dir: %w", err)
	}
	if err := extractEntry(archivePath, archiveExt(src, url), entry, tmpBin); err != nil {
		os.Remove(tmpBin)
		return "", err
	}
	if err := os.Rename(tmpBin, pinned); err != nil {
		os.Remove(tmpBin)
		return "", fmt.Errorf("finalize binary: %w", err)
	}
	return pinned, nil
}

// downloadVerified streams the archive to a temp file next to its final
// consumers, hashing as it writes. A digest mismatch discards the file and
// fails — the archive is never extracted.
func downloadVerified(ctx context.Context, url, wantSHA256, dir string, opts Options) (string, error) {
	body, err := opts.downloader()(ctx, url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer body.Close()

	tmp, err := os.CreateTemp(dir, "download-*.partial")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("flush download: %w", err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	if err := verifySHA256(sum, wantSHA256); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// archiveExt picks the archive format extension from the ext map, falling
// back to the URL suffix.
func archiveExt(src *registry.SourceConfig, url string) string {
	if ext, ok := src.ExtMap[runtime.GOOS]; ok && ext != "" {
		return ext
	}
	switch {
	case strings.HasSuffix(url, ".tgz"), strings.HasSuffix(url, ".tar.gz"):
		return ".tgz"
	case strings.HasSuffix(url, ".zip"):
		return ".zip"
	}
	return ""
}

// extractEntry copies exactly one archive entry (the tool binary) to dest with
// executable permissions. Nothing else in the archive touches the filesystem.
func extractEntry(archivePath, ext, entry, dest string) error {
	switch ext {
	case ".tgz", ".tar.gz":
		return extractTarGzEntry(archivePath, entry, dest)
	case ".zip":
		return extractZipEntry(archivePath, entry, dest)
	default:
		return fmt.Errorf("unsupported archive extension %q", ext)
	}
}

func extractTarGzEntry(archivePath, entry, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || path.Clean(hdr.Name) != path.Clean(entry) {
			continue
		}
		return writeBinary(dest, tr)
	}
	return fmt.Errorf("binary %q not found in archive", entry)
}

func extractZipEntry(archivePath, entry, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("read zip: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || path.Clean(zf.Name) != path.Clean(entry) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("open zip entry: %w", err)
		}
		defer rc.Close()
		return writeBinary(dest, rc)
	}
	return fmt.Errorf("binary %q not found in archive", entry)
}

func writeBinary(dest string, r io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("write binary: %w", err)
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return fmt.Errorf("write binary: %w", err)
	}
	return out.Close()
}
