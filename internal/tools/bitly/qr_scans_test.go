package bitly

import (
	"net/http"
	"testing"
)

func TestQRScans_TotalsNoSize(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "scans", "scans", "--qr", "q1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodGet || got.Path != "/qr-codes/q1/scans" {
		t.Errorf("request = %s %s, want GET /qr-codes/q1/scans", got.Method, got.Path)
	}
	q := parseQuery(t, got.Query)
	if q.Get("unit") != "day" || q.Get("units") != "-1" {
		t.Errorf("query = %q, want default unit window", got.Query)
	}
	if _, ok := q["size"]; ok {
		t.Errorf("size should be absent on scans totals, query = %q", got.Query)
	}
}

func TestQRScans_DeviceOSWithSize(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "scans", "device-os", "--qr", "q1", "--size", "9")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/qr-codes/q1/scans/device_os" {
		t.Errorf("path = %q, want /qr-codes/q1/scans/device_os", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("size") != "9" {
		t.Errorf("size = %q, want 9", q.Get("size"))
	}
}

func TestQRScans_SummaryPath(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "scans", "summary", "--qr", "q1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/qr-codes/q1/scans/summary" {
		t.Errorf("path = %q, want /qr-codes/q1/scans/summary", got.Path)
	}
}
