package execution

import (
	"errors"
	"io"
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
	// visible under name only after Close returns nil, and a writer that is
	// never closed leaves nothing behind. A tool can therefore abandon a
	// half-written download without destroying whatever was there before.
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
func (OS) Create(name string) (io.WriteCloser, error) {
	temporary, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*")
	if err != nil {
		return nil, err
	}
	return &atomicFile{file: temporary, target: name}, nil
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
	// The temporary file is created 0600; the visible file keeps the mode a
	// caller expects from a tool that wrote it directly.
	if err := os.Chmod(a.file.Name(), 0o644); err != nil {
		_ = os.Remove(a.file.Name())
		return err
	}
	if err := os.Rename(a.file.Name(), a.target); err != nil {
		_ = os.Remove(a.file.Name())
		return err
	}
	return nil
}

// Abandon discards a partial write. Tools do not call it — leaving a writer
// unclosed is enough — but it lets the temporary file go on an error path that
// returns early.
func (a *atomicFile) Abandon() {
	if a.closed {
		return
	}
	a.closed = true
	_ = a.file.Close()
	_ = os.Remove(a.file.Name())
}
