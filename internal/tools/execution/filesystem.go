package execution

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
)

// FileSystem is where every local file a tool reads or writes goes.
//
// AnyCLI normally runs on the machine of the person typing the command, where
// `--out ./contract.pdf` writes to their disk and `--attach ./photo.png` reads
// from it. That is the whole point of a CLI, and OS is the implementation that
// does exactly that. A host running AnyCLI on shared infrastructure has a
// different machine underneath: "local" is the server, its files are not the
// caller's, and a path in argv is the caller choosing which of the server's
// files to read or overwrite. Supplying a FileSystem is how such a host makes
// that choice its own.
//
// There is no second path. Tool packages never call the os package, so there is
// no arrangement under which a caller-named path reaches the host directly, and
// no behavior that exists only when a host declines to install one.
//
// # Scope
//
// The seam governs the files a tool opens itself. A tool that hands a path to a
// subprocess is outside it by construction: the child reads with its own system
// calls, and no Go interface can intervene. mongodb (which runs mongosh) and
// the passthrough CLI tools are the whole of that category, and a host that
// cares about isolation should decline to run them rather than assume this
// covers them.
//
// # Concurrency
//
// An implementation must be safe for concurrent use. One Engine.Execute can
// call it from several goroutines at once — figma's asset download runs four —
// and a host may share one FileSystem across concurrent executions.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error

	// Open returns the file's contents as a stream. The returned value may
	// also implement io.ReaderAt; a caller that needs random access should
	// assert for it and fall back to buffering when the assertion fails.
	// linkedin's video upload is the one caller that does, because it sends
	// server-defined byte ranges.
	Open(name string) (io.ReadCloser, error)

	// Create returns a writer for name. It is atomic: the contents become
	// visible under name only after Close returns nil, so a tool can abandon a
	// half-written download without destroying whatever was there before.
	//
	// A tool that gives up must say so by calling Abandon on the writer, which
	// is how the partial write is released. Close and Abandon are each safe to
	// call once; whichever comes first decides the outcome.
	//
	// Create replaces an existing name. A tool that must not replace one
	// checks with Stat first; that is its own semantic (figma's --overwrite),
	// not something every caller of this interface should have to express.
	Create(name string) (io.WriteCloser, error)

	// Stat reports what a tool needs before it opens a path: how large it is
	// and whether it is a directory. ErrNotExist is returned for a name that
	// is not there.
	Stat(name string) (FileInfo, error)

	MkdirAll(name string, perm os.FileMode) error
	Remove(name string) error
}

// FileInfo is the metadata tools actually use, as plain data. It is not
// fs.FileInfo: a host has no modification time or mode bits to invent, and
// requiring six methods to report a size would be a tax on every implementer.
type FileInfo struct {
	Size  int64
	IsDir bool
}

// ErrNotExist is what Stat and Open report for a name that is not there. A
// host may return its own error wrapping this one.
var ErrNotExist = os.ErrNotExist

// OS is the FileSystem for the machine the process runs on: the implementation
// a person at a terminal gets, and the one an Engine installs when its host
// supplies none.
type OS struct{}

func (OS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func (OS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (OS) Open(name string) (io.ReadCloser, error) { return os.Open(name) }

func (OS) MkdirAll(name string, perm os.FileMode) error { return os.MkdirAll(name, perm) }

func (OS) Remove(name string) error { return os.Remove(name) }

func (OS) Stat(name string) (FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Size: info.Size(), IsDir: info.IsDir()}, nil
}

// Create writes through a sibling temporary file and renames it into place on
// Close, which is what makes an interrupted download leave the previous file
// alone. Every tool used to spell this out for itself; it belongs here, once.
//
// The temporary file is opened 0666, so the process umask decides the mode the
// caller ends up with — the same mode os.Create would have produced, and the
// reason this does not chmod afterwards. A tool that needs a tighter one says
// so with WriteFile.
func (OS) Create(name string) (io.WriteCloser, error) {
	directory, base := filepath.Dir(name), filepath.Base(name)
	for attempt := 0; ; attempt++ {
		candidate := filepath.Join(directory, fmt.Sprintf(".%s.%d.tmp", base, rand.Uint64()))
		file, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
			return &atomicFile{file: file, target: name}, nil
		}
		// O_EXCL means an existing name is the one error worth retrying, and
		// only so many times: a directory that is not writable fails the same
		// way forever.
		if !errors.Is(err, os.ErrExist) || attempt >= 10 {
			return nil, err
		}
	}
}

type atomicFile struct {
	file   *os.File
	target string
	closed bool
}

func (a *atomicFile) Write(data []byte) (int, error) { return a.file.Write(data) }

func (a *atomicFile) Close() error {
	if a.closed {
		return errors.New("close of an already closed file")
	}
	a.closed = true
	if err := a.file.Close(); err != nil {
		_ = os.Remove(a.file.Name())
		return err
	}
	if err := os.Rename(a.file.Name(), a.target); err != nil {
		_ = os.Remove(a.file.Name())
		return err
	}
	return nil
}

// Abandon discards a partial write.
func (a *atomicFile) Abandon() {
	if a.closed {
		return
	}
	a.closed = true
	_ = a.file.Close()
	_ = os.Remove(a.file.Name())
}

// Abandon releases a writer a tool is giving up on. A writer that does not know
// how to be abandoned is closed instead, which is the most any io.WriteCloser
// promises; the atomic one produced by OS discards its partial file.
func Abandon(w io.WriteCloser) {
	if w == nil {
		return
	}
	if a, ok := w.(interface{ Abandon() }); ok {
		a.Abandon()
		return
	}
	_ = w.Close()
}
