package gateprobe

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	s := &Service{Out: &out, Err: &errBuf}
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

// TestCommandTreeShape pins the design-318 harness contract on the tree
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
