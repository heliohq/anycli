package gateprobe

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heliohq/anycli/internal/tools/execution"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	s := &Service{Out: &out, Err: &errBuf, FS: execution.OS{}}
	result, err := s.Execute(context.Background(), args, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	return result.ExitCode, out.String(), errBuf.String()
}

func TestExecute(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string // substrings that must appear on stdout
		wantErr    bool     // expect a non-empty stderr
	}{
		{
			name:       "probe send echoes a local receipt",
			args:       []string{"probe", "send"},
			wantCode:   0,
			wantStdout: []string{`"tool":"gate-probe"`, `"action":"gate-probe.probe_send"`, `"status":"sent"`},
		},
		{
			name:       "probe send echoes the note back",
			args:       []string{"probe", "send", "--note", "run-42"},
			wantCode:   0,
			wantStdout: []string{`"note":"run-42"`},
		},
		{
			name:     "positional args are rejected",
			args:     []string{"probe", "send", "extra"},
			wantCode: 1,
			wantErr:  true,
		},
		{
			name:     "unknown flag fails the parse",
			args:     []string{"probe", "send", "--bogus"},
			wantCode: 1,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := run(t, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tc.wantCode, stderr)
			}
			for _, want := range tc.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want it to contain %q", stdout, want)
				}
			}
			if tc.wantErr && stderr == "" {
				t.Error("expected an error message on stderr")
			}
		})
	}
}

// TestCommandTreeShape pins the harness contract on the tree
// itself: the probe path is hidden, the leaf carries an explicit
// side_effect=true annotation, and the group is help-only (nil RunE, no
// annotation) per the annotation lint predicates.
func TestCommandTreeShape(t *testing.T) {
	root := (&Service{}).NewCommandTree()

	probe, _, err := root.Find([]string{"probe"})
	if err != nil || probe == nil || probe.Name() != "probe" {
		t.Fatalf("Find(probe) = %v, %v", probe, err)
	}
	if !probe.Hidden {
		t.Error("probe group is not Hidden")
	}
	if probe.RunE != nil {
		t.Error("probe group has a RunE; want help-only group")
	}
	if _, ok := probe.Annotations["anycli.side_effect"]; ok {
		t.Error("probe group carries a side_effect annotation; groups must not")
	}

	send, _, err := root.Find([]string{"probe", "send"})
	if err != nil || send == nil || send.Name() != "send" {
		t.Fatalf("Find(probe send) = %v, %v", send, err)
	}
	if !send.Hidden {
		t.Error("send leaf is not Hidden")
	}
	if send.HasSubCommands() {
		t.Error("send has subcommands; want a runnable leaf")
	}
	if got := send.Annotations["anycli.side_effect"]; got != "true" {
		t.Errorf("send side_effect annotation = %q, want %q", got, "true")
	}
}

// countingFS records what a tool asked for and answers from a map, so a test
// can prove the bytes went through the seam instead of past it.
type countingFS struct {
	execution.FileSystem
	files map[string][]byte
	reads []string
	wrote map[string][]byte
}

func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.reads = append(c.reads, name)
	data, ok := c.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return data, nil
}

func (c *countingFS) Stat(name string) (fs.FileInfo, error) {
	data, ok := c.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return statOf(name, int64(len(data))), nil
}

func (c *countingFS) WriteFile(name string, data []byte, _ fs.FileMode) error {
	if c.wrote == nil {
		c.wrote = map[string][]byte{}
	}
	c.wrote[name] = append([]byte(nil), data...)
	return nil
}

type sizedInfo struct {
	name string
	size int64
}

func statOf(name string, size int64) fs.FileInfo { return sizedInfo{name: name, size: size} }

func (i sizedInfo) Name() string       { return i.name }
func (i sizedInfo) Size() int64        { return i.size }
func (i sizedInfo) Mode() fs.FileMode  { return 0o644 }
func (i sizedInfo) IsDir() bool        { return false }
func (i sizedInfo) Sys() any           { return nil }
func (i sizedInfo) ModTime() time.Time { return time.Time{} }

// TestProbeCopyGoesThroughTheFilesystemSeam is what makes this action worth
// having: a host that redirects file access can run it and see, on its own
// side, exactly which paths were asked for and what was written. A copy that
// quietly used the real disk would leave this filesystem empty.
func TestProbeCopyGoesThroughTheFilesystemSeam(t *testing.T) {
	seam := &countingFS{files: map[string][]byte{"./in.txt": []byte("relayed")}}
	var out, errBuf bytes.Buffer
	service := &Service{Out: &out, Err: &errBuf, FS: seam}

	result, err := service.Execute(context.Background(), []string{"probe", "copy", "--in", "./in.txt", "--out", "./out.txt"}, nil)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("exit = %d err = %v stderr = %q", result.ExitCode, err, errBuf.String())
	}
	if len(seam.reads) != 1 || seam.reads[0] != "./in.txt" {
		t.Fatalf("the seam saw reads %v", seam.reads)
	}
	if got := string(seam.wrote["./out.txt"]); got != "relayed" {
		t.Fatalf("the seam received %q", got)
	}
	for _, want := range []string{`"action":"gate-probe.probe_copy"`, `"bytes":7`, `"stat_bytes":7`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("receipt %q is missing %s", out.String(), want)
		}
	}
}

// TestProbeCopyOnRealFiles keeps the ordinary path honest: with the os
// filesystem, --out is a file on this disk.
func TestProbeCopyOnRealFiles(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "in.txt")
	target := filepath.Join(directory, "out.txt")
	if err := os.WriteFile(source, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run(t, "probe", "copy", "--in", source, "--out", target)
	if code != 0 {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
	landed, err := os.ReadFile(target)
	if err != nil || string(landed) != "on disk" {
		t.Fatalf("target = %q (%v)", landed, err)
	}
}

func TestProbeCopyRequiresBothPaths(t *testing.T) {
	if code, _, stderr := run(t, "probe", "copy", "--in", "x"); code == 0 || !strings.Contains(stderr, "--in and --out") {
		t.Fatalf("exit = %d stderr = %q", code, stderr)
	}
}
