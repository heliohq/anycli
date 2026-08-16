package execution

import (
	"io"
	"os"
)

// FileSystem is the host's control over every local file a tool touches.
//
// AnyCLI normally runs on the machine of the person typing the command, where
// `--out ./contract.pdf` writes to their disk and `--attach ./photo.png` reads
// from it. That is the whole point of a CLI. A host that runs AnyCLI on shared
// infrastructure has a different machine underneath: "local" is the server, its
// files are not the caller's, and a path in argv is the caller choosing which
// of the server's files to read or overwrite.
//
// A host supplies a FileSystem to make that choice its own. A nil FileSystem
// keeps the ordinary behavior, so nothing changes for a person at a terminal.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Open(name string) (io.ReadCloser, error)
	Create(name string) (io.WriteCloser, error)
	// MkdirAll is what a tool calls before writing into a directory the caller
	// named. A host whose storage has no directories can do nothing and return
	// nil; the tools that call it only need the subsequent Create to work.
	MkdirAll(name string, perm os.FileMode) error
}

// The helpers below are what tool packages call instead of the os functions
// they replace. Each falls back to os when fs is nil, which is why a tool can
// adopt the seam without changing how it behaves on a laptop.

func ReadFile(fs FileSystem, name string) ([]byte, error) {
	if fs == nil {
		return os.ReadFile(name)
	}
	return fs.ReadFile(name)
}

func WriteFile(fs FileSystem, name string, data []byte, perm os.FileMode) error {
	if fs == nil {
		return os.WriteFile(name, data, perm)
	}
	return fs.WriteFile(name, data, perm)
}

func Open(fs FileSystem, name string) (io.ReadCloser, error) {
	if fs == nil {
		return os.Open(name)
	}
	return fs.Open(name)
}

func Create(fs FileSystem, name string) (io.WriteCloser, error) {
	if fs == nil {
		return os.Create(name)
	}
	return fs.Create(name)
}

func MkdirAll(fs FileSystem, name string, perm os.FileMode) error {
	if fs == nil {
		return os.MkdirAll(name, perm)
	}
	return fs.MkdirAll(name, perm)
}
