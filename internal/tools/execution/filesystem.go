package execution

import (
	"errors"
	"io"
	"io/fs"
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
// that choice its own — by answering these calls from wherever the caller's
// files actually are.
//
// There is no second path. Tool packages never call the os package, so there is
// no arrangement under which a caller-named path reaches the host directly, and
// no behavior that exists only when a host declines to install one.
//
// The types are io/fs's — fs.File, fs.FileInfo, fs.FileMode, fs.ErrNotExist —
// and the method names are os.Root's, which is Go's own vocabulary for
// filesystem access confined to a scope.
//
// The io/fs *interfaces* are deliberately not embedded. fs.FS is a virtual
// namespace: fs.ValidPath requires unrooted, slash-separated names with no "."
// or ".." element, and a conforming fs.FS must reject anything else. What
// arrives here is whatever a tool was handed in argv — `--out ./contract.pdf`,
// `--attach /tmp/photo.png` — which is an operating-system path, and rejecting
// those is the opposite of the job. Claiming fs.FS would be claiming a contract
// this cannot keep.
//
// # Scope
//
// The seam governs the files a tool opens itself. A tool that hands a path to a
// subprocess is outside it by construction: the child reads with its own system
// calls, and no Go interface can intervene. mongodb (which runs mongosh) and
// the passthrough CLI tools are the whole of that category, and a host that
// cares about isolation declines to run them rather than assume this covers
// them.
//
// # Concurrency
//
// An implementation must be safe for concurrent use. One Engine.Execute can
// call it from several goroutines at once — figma's asset download runs four —
// and a host may share one FileSystem across concurrent executions.
type FileSystem interface {
	// Open reports fs.ErrNotExist for a name that is not there. io/fs puts the
	// rest better than a paraphrase would: "a file may implement io.ReaderAt or
	// io.Seeker as optimizations". linkedin's ranged video upload asserts for
	// io.ReaderAt and buffers when the assertion fails, so an implementation
	// able to offer random access spares it from reading a whole video into
	// memory.
	Open(name string) (fs.File, error)

	// Stat reports what a tool checks before opening a path: how large it is,
	// and whether it is a directory.
	Stat(name string) (fs.FileInfo, error)

	// ReadFile is here rather than left to a helper over Open because most
	// callers need the whole file anyway — they base64 it into a JSON request
	// body — and asking for it once spares an implementation that fetches the
	// bytes from elsewhere a round trip per chunk.
	ReadFile(name string) ([]byte, error)

	// Create returns a writer for name, replacing whatever was there. It
	// promises nothing about when the contents become visible: a write that
	// fails partway can leave a partial file, the same as every download tool
	// that writes where it was told to.
	//
	// Staging a write and swapping it in at the end is a thing an
	// implementation may do, not a thing this interface asks for. Promising it
	// here would oblige every implementation to be transactional, and a
	// transaction needs a way to roll back — a second verb no filesystem has.
	//
	// A tool that must not replace an existing name checks with Stat first;
	// that is its own semantic (figma's --overwrite), not something every
	// caller of this interface should have to express.
	Create(name string) (io.WriteCloser, error)

	// WriteFile exists beside Create because it carries a mode, and Create
	// cannot: docusign and dropbox-sign write signed documents 0600. The os
	// package keeps both for the same reason.
	WriteFile(name string, data []byte, perm fs.FileMode) error

	// Mkdir makes one directory, like os.Root.Mkdir. MkdirAll is a function
	// over it, the way io/fs keeps an interface to one operation.
	Mkdir(name string, perm fs.FileMode) error

	Remove(name string) error
}

// OS is the FileSystem for the machine the process runs on: the implementation
// a person at a terminal gets, and the one an Engine installs when its host
// supplies none.
type OS struct{}

func (OS) Open(name string) (fs.File, error)     { return os.Open(name) }
func (OS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
func (OS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }

func (OS) Create(name string) (io.WriteCloser, error) { return os.Create(name) }

func (OS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (OS) Mkdir(name string, perm fs.FileMode) error { return os.Mkdir(name, perm) }

func (OS) Remove(name string) error { return os.Remove(name) }

// MkdirAll makes name and every parent it needs. It is a function for the same
// reason fs.ReadFile is one: it is Mkdir in a loop, and every implementation
// would otherwise write that loop again.
func MkdirAll(fsys FileSystem, name string, perm fs.FileMode) error {
	if info, err := fsys.Stat(name); err == nil {
		if info.IsDir() {
			return nil
		}
		return &fs.PathError{Op: "mkdir", Path: name, Err: errors.New("not a directory")}
	}
	if parent := filepath.Dir(name); parent != name {
		if err := MkdirAll(fsys, parent, perm); err != nil {
			return err
		}
	}
	if err := fsys.Mkdir(name, perm); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return nil
}
