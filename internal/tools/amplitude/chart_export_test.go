package amplitude

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// (e) chart returns CSV, wrapped in a JSON envelope on stdout.
func TestChartCSVEnvelope(t *testing.T) {
	const csv = "date,uniques\n20220101,42\n"
	var got capturedRequest
	srv := newServer(t, http.StatusOK, csv, &got)
	defer srv.Close()

	res, stdout, stderr := run(t, srv, "chart", "--id", "abc123")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Path != "/api/3/chart/abc123/csv" {
		t.Errorf("path = %q", got.Path)
	}
	var env chartEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("stdout not JSON: %v (%q)", err, stdout)
	}
	if env.Format != "csv" || env.ChartID != "abc123" || env.Data != csv {
		t.Errorf("envelope = %+v", env)
	}
}

func TestChartRequiresID(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "x", &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "chart")
	if res.ExitCode != 2 {
		t.Errorf("missing --id exit = %d, want 2", res.ExitCode)
	}
}

// (f) export streams the archive to a file and emits a JSON receipt; the raw
// bytes never appear on stdout.
func TestExportFileReceipt(t *testing.T) {
	zip := "PK\x03\x04fake-zip-bytes"
	var got capturedRequest
	srv := newServer(t, http.StatusOK, zip, &got)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "export.zip")
	res, stdout, stderr := run(t, srv,
		"export", "--start", "20220101T00", "--end", "20220101T05", "--output", out)
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.ExitCode, stderr)
	}
	if got.Path != "/api/2/export" {
		t.Errorf("path = %q", got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("start") != "20220101T00" || q.Get("end") != "20220101T05" {
		t.Errorf("start/end = %q/%q", q.Get("start"), q.Get("end"))
	}
	// stdout is a JSON receipt, never the raw zip bytes.
	if strings.Contains(stdout, "PK\x03\x04") {
		t.Errorf("raw archive bytes leaked to stdout: %q", stdout)
	}
	var rc exportReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &rc); err != nil {
		t.Fatalf("stdout not a JSON receipt: %v (%q)", err, stdout)
	}
	if rc.Saved != out || rc.Bytes != int64(len(zip)) || rc.Start != "20220101T00" || rc.End != "20220101T05" {
		t.Errorf("receipt = %+v", rc)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if string(data) != zip {
		t.Errorf("file content = %q, want %q", data, zip)
	}
}

// export without --output writes to a temp file and reports its path.
func TestExportDefaultTempFile(t *testing.T) {
	zip := "PK\x03\x04tmp"
	var got capturedRequest
	srv := newServer(t, http.StatusOK, zip, &got)
	defer srv.Close()

	res, stdout, _ := run(t, srv, "export", "--start", "20220101T00", "--end", "20220101T00")
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	var rc exportReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &rc); err != nil {
		t.Fatalf("stdout not a JSON receipt: %v", err)
	}
	if rc.Saved == "" {
		t.Fatal("receipt has no saved path")
	}
	defer os.Remove(rc.Saved)
	data, err := os.ReadFile(rc.Saved)
	if err != nil {
		t.Fatalf("read temp export: %v", err)
	}
	if string(data) != zip {
		t.Errorf("temp file content = %q", data)
	}
}

// A non-2xx export (e.g. the 4GB / 365-day limit) surfaces as a typed apiError,
// not a written file.
func TestExportErrorSurfacesAPIError(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusBadRequest, `{"error":"time range too large"}`, &got)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "should-not-exist.zip")
	res, _, stderr := run(t, srv, "export", "--start", "20220101T00", "--end", "20230101T00", "--output", out)
	if res.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(stderr, "time range too large") {
		t.Errorf("stderr = %q, want the API message", stderr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("no file should be written on API error")
	}
}

func TestExportRequiresStartEnd(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "x", &got)
	defer srv.Close()
	res, _, _ := run(t, srv, "export", "--start", "20220101T00")
	if res.ExitCode != 2 {
		t.Errorf("missing --end exit = %d, want 2", res.ExitCode)
	}
}
