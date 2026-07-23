package gumroad

import "testing"

func TestProductList(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"products":[{"id":"p1"}]}`, &got)
	defer srv.Close()

	exit, _, stderr := run(t, srv, "product", "list")
	if exit != 0 {
		t.Fatalf("exit = %d, stderr = %q", exit, stderr)
	}
	if got.Method != "GET" || got.Path != "/v2/products" {
		t.Fatalf("request = %s %s, want GET /v2/products", got.Method, got.Path)
	}
}

func TestProductGet(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"product":{"id":"p1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "product", "get", "--id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "GET" || got.Path != "/v2/products/p1" {
		t.Fatalf("request = %s %s, want GET /v2/products/p1", got.Method, got.Path)
	}
}

func TestProductEnable(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"product":{"id":"p1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "product", "enable", "--id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/products/p1/enable" {
		t.Fatalf("request = %s %s, want PUT /v2/products/p1/enable", got.Method, got.Path)
	}
}

func TestProductDisable(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true,"product":{"id":"p1"}}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "product", "disable", "--id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "PUT" || got.Path != "/v2/products/p1/disable" {
		t.Fatalf("request = %s %s, want PUT /v2/products/p1/disable", got.Method, got.Path)
	}
}

func TestProductDelete(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, 200, `{"success":true}`, &got)
	defer srv.Close()

	exit, _, _ := run(t, srv, "product", "delete", "--id", "p1")
	if exit != 0 {
		t.Fatalf("exit = %d", exit)
	}
	if got.Method != "DELETE" || got.Path != "/v2/products/p1" {
		t.Fatalf("request = %s %s, want DELETE /v2/products/p1", got.Method, got.Path)
	}
}
