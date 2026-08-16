package execution

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOSCreateIsAtomic pins the property tools now rely on instead of each
// spelling out its own temporary-file dance: nothing is visible under the name
// until Close, and giving up leaves what was there alone.
func TestOSCreateIsAtomic(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "asset.pdf")
	if err := os.WriteFile(target, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer, err := OS{}.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("half a download")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "previous" {
		t.Fatalf("an unclosed write was visible: %q (%v)", data, err)
	}

	Abandon(writer)
	if data, err := os.ReadFile(target); err != nil || string(data) != "previous" {
		t.Fatalf("abandoning replaced the file: %q (%v)", data, err)
	}
	// The partial write is released, not merely hidden. A download that fails
	// repeatedly must not fill the directory with spools.
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "asset.pdf" {
			t.Fatalf("abandoning left %q behind", entry.Name())
		}
	}

	writer, err = OS{}.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("committed")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "committed" {
		t.Fatalf("close did not commit: %q (%v)", data, err)
	}
}

// TestOSCreateLeavesTheModeToTheUmask keeps provider exports from becoming
// readable to other local users on a host whose umask says otherwise.
func TestOSCreateLeavesTheModeToTheUmask(t *testing.T) {
	target := filepath.Join(t.TempDir(), "export.zip")

	reference := filepath.Join(t.TempDir(), "reference")
	referenceFile, err := os.Create(reference)
	if err != nil {
		t.Fatal(err)
	}
	if err := referenceFile.Close(); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(reference)
	if err != nil {
		t.Fatal(err)
	}

	writer, err := OS{}.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("mode = %v, want %v (what os.Create produces here)", got.Mode().Perm(), want.Mode().Perm())
	}
}

func TestOSStatReportsMissingAsErrNotExist(t *testing.T) {
	if _, err := (OS{}).Stat(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, ErrNotExist) {
		t.Fatalf("stat of a missing name = %v, want ErrNotExist", err)
	}
}

// TestOSOpenSupportsRandomAccess pins what linkedin's ranged upload asserts
// for: on a disk the stream is seekable, so a video is not buffered.
func TestOSOpenSupportsRandomAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := OS{}.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	at, ok := source.(interface {
		ReadAt(p []byte, off int64) (int, error)
	})
	if !ok {
		t.Fatal("OS.Open returned a reader with no random access")
	}
	buffer := make([]byte, 3)
	if _, err := at.ReadAt(buffer, 4); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "456" {
		t.Fatalf("ReadAt = %q, want %q", buffer, "456")
	}
}

func TestOSCreateRefusesAnUnwritableDirectory(t *testing.T) {
	_, err := OS{}.Create(filepath.Join(t.TempDir(), "missing-directory", "file"))
	if err == nil {
		t.Fatal("creating under a missing directory succeeded")
	}
	if strings.Contains(err.Error(), "tmp") && errors.Is(err, os.ErrExist) {
		t.Fatalf("retry loop misread the failure: %v", err)
	}
}
