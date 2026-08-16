package execution

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestOSTakesOperatingSystemPaths pins the one place this deliberately parts
// company with io/fs. fs.ValidPath would reject every one of these names, and a
// conforming fs.FS must reject them too — but they are exactly what a tool is
// handed in argv, so accepting them is the job. This is why the io/fs
// interfaces are not embedded, only its types reused.
func TestOSTakesOperatingSystemPaths(t *testing.T) {
	directory := t.TempDir()
	absolute := filepath.Join(directory, "note.txt")
	if err := os.WriteFile(absolute, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		absolute, // --out /tmp/x/note.txt
		filepath.Join(directory, ".", "note.txt"),         // a "." element
		filepath.Join(directory, "sub", "..", "note.txt"), // a ".." element
	} {
		if fs.ValidPath(name) {
			t.Fatalf("%q is a valid io/fs name; the test no longer proves anything", name)
		}
		data, err := OS{}.ReadFile(name)
		if err != nil || string(data) != "hello" {
			t.Fatalf("ReadFile(%q) = %q, %v", name, data, err)
		}
	}
}

func TestOSStatReportsMissingAsErrNotExist(t *testing.T) {
	if _, err := (OS{}).Stat(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat of a missing name = %v, want fs.ErrNotExist", err)
	}
}

// TestOSOpenSupportsRandomAccess pins what linkedin's ranged upload asserts
// for. io/fs says a file may implement io.ReaderAt as an optimization; on a
// disk it does, so a video is streamed rather than buffered.
func TestOSOpenSupportsRandomAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := OS{}.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	at, ok := file.(io.ReaderAt)
	if !ok {
		t.Fatal("OS.Open returned a file with no random access")
	}
	buffer := make([]byte, 3)
	if _, err := at.ReadAt(buffer, 4); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "456" {
		t.Fatalf("ReadAt = %q, want %q", buffer, "456")
	}
}

// TestOSWriteFileHonorsTheMode is the whole reason WriteFile sits in the
// interface beside Create: docusign and dropbox-sign write signed documents
// 0600, and a writer with no mode could not say so.
func TestOSWriteFileHonorsTheMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signed.pdf")
	if err := (OS{}).WriteFile(path, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestMkdirAllBuildsEveryParent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a", "b", "c")
	if err := MkdirAll(OS{}, target, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("stat %s = %v, %v", target, info, err)
	}
	// Again on a path that already exists, which is what a second download into
	// the same --save directory does.
	if err := MkdirAll(OS{}, target, 0o755); err != nil {
		t.Fatalf("second call = %v, want nil", err)
	}
}

func TestMkdirAllRefusesAPathThatIsAFile(t *testing.T) {
	root := t.TempDir()
	occupied := filepath.Join(root, "taken")
	if err := os.WriteFile(occupied, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MkdirAll(OS{}, occupied, 0o755); err == nil {
		t.Fatal("MkdirAll over a regular file succeeded")
	}
}
