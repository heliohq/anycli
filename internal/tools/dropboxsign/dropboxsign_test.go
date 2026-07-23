package dropboxsign

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMissingTokenExitsOne(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	res, err := svc.Execute(context.Background(), []string{"account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d, want 1", res.ExitCode)
	}
	contains(t, errBuf.String(), "DROPBOX_SIGN_ACCESS_TOKEN is not set", "missing-token stderr")
}

func TestMissingTokenJSONEnvelope(t *testing.T) {
	var out, errBuf strings.Builder
	svc := &Service{Out: &out, Err: &errBuf}
	_, err := svc.Execute(context.Background(), []string{"--json", "account", "get"}, map[string]string{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Kind    string `json:"kind"`
		} `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(errBuf.String()), &env); jerr != nil {
		t.Fatalf("stderr not JSON: %v (%q)", jerr, errBuf.String())
	}
	if env.Error.Kind != "usage" {
		t.Fatalf("kind = %q, want usage", env.Error.Kind)
	}
}

func TestUnknownSubcommandIsUsageExitTwo(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{})
	defer srv.Close()
	exit, _, _ := run(t, srv, "signature-request", "bogus")
	if exit != 2 {
		t.Fatalf("exit = %d, want 2 for unknown subcommand", exit)
	}
}

func TestBareGroupShowsHelpExitZero(t *testing.T) {
	srv := newServer(t, 200, `{}`, &capturedRequest{})
	defer srv.Close()
	exit, _, _ := run(t, srv, "signature-request")
	if exit != 0 {
		t.Fatalf("exit = %d, want 0 for bare group", exit)
	}
}
