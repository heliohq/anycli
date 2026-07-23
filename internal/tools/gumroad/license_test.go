package gumroad

import "testing"

func TestLicenseVerify(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"uses":3,"purchase":{}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "license", "verify", "--product-id", "p1", "--license-key", "KEY-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "POST" || got.Path != "/v2/licenses/verify" {
		t.Fatalf("request = %s %s, want POST /v2/licenses/verify", got.Method, got.Path)
	}
	if got.Form.Get("product_id") != "p1" || got.Form.Get("license_key") != "KEY-1" {
		t.Fatalf("form = %v, want product_id/license_key (body=%s)", got.Form, got.Body)
	}
	// increment_uses_count defaults to false so a plain verification does not
	// consume a seat.
	if got.Form.Get("increment_uses_count") != "false" {
		t.Fatalf("form increment_uses_count = %q, want false", got.Form.Get("increment_uses_count"))
	}
}

func TestLicenseVerifyIncrement(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"uses":4}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "license", "verify",
		"--product-id", "p1", "--license-key", "KEY-1", "--increment-uses-count")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Form.Get("increment_uses_count") != "true" {
		t.Fatalf("form increment_uses_count = %q, want true", got.Form.Get("increment_uses_count"))
	}
}

func TestLicenseEnable(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "license", "enable", "--product-id", "p1", "--license-key", "KEY-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/licenses/enable" {
		t.Fatalf("request = %s %s, want PUT /v2/licenses/enable", got.Method, got.Path)
	}
	if got.Form.Get("product_id") != "p1" || got.Form.Get("license_key") != "KEY-1" {
		t.Fatalf("form = %v, want product_id/license_key", got.Form)
	}
}

func TestLicenseDisable(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "license", "disable", "--product-id", "p1", "--license-key", "KEY-1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/licenses/disable" {
		t.Fatalf("request = %s %s, want PUT /v2/licenses/disable", got.Method, got.Path)
	}
}
