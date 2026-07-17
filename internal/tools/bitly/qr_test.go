package bitly

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQRCreate_DestinationBitlink(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"qrcode_id":"q1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "create", "--group", "Bg1",
		"--destination-bitlink", "bit.ly/2ab", "--title", "T")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPost || got.Path != "/qr-codes" {
		t.Errorf("request = %s %s, want POST /qr-codes", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	dest, ok := body["destination"].(map[string]any)
	if !ok || dest["bitlink_id"] != "bit.ly/2ab" {
		t.Errorf("destination = %v, want bitlink_id", body["destination"])
	}
	if body["group_guid"] != "Bg1" {
		t.Errorf("group_guid = %v", body["group_guid"])
	}
}

func TestQRCreate_RequiresExactlyOneDestination(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{}`, &got)
	defer srv.Close()

	// both set
	code, _, stderr := run(t, srv, "qr", "create", "--group", "Bg1",
		"--destination-bitlink", "bit.ly/2ab", "--destination-long-url", "https://e.com")
	if code != 1 || !strings.Contains(stderr, "exactly one") {
		t.Errorf("both destinations: code=%d stderr=%q", code, stderr)
	}

	// neither set
	code, _, stderr = run(t, srv, "qr", "create", "--group", "Bg1")
	if code != 1 || !strings.Contains(stderr, "exactly one") {
		t.Errorf("no destination: code=%d stderr=%q", code, stderr)
	}
}

func TestQRCreateStatic(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"qrcode_id":"q1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "create-static", "--content", "hello", "--group", "Bg1",
		"--customizations-json", `{"background_color":"#fff"}`)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/qr-codes/static" {
		t.Errorf("path = %q", got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["content"] != "hello" {
		t.Errorf("content = %v", body["content"])
	}
	if _, ok := body["render_customizations"].(map[string]any); !ok {
		t.Errorf("render_customizations = %v, want object passthrough", body["render_customizations"])
	}
}

func TestQRList_PathAndQuery(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"qr_codes":[]}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "list", "--group", "Bg1", "--size", "7", "--archived", "on")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/groups/Bg1/qr-codes" {
		t.Errorf("path = %q", got.Path)
	}
	if q := parseQuery(t, got.Query); q.Get("size") != "7" || q.Get("archived") != "on" {
		t.Errorf("query = %q", got.Query)
	}
}

func TestQRUpdate(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `{"qrcode_id":"q1"}`, &got)
	defer srv.Close()

	code, _, _ := run(t, srv, "qr", "update", "--qr", "q1", "--title", "New", "--archived")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Method != http.MethodPatch || got.Path != "/qr-codes/q1" {
		t.Errorf("request = %s %s, want PATCH /qr-codes/q1", got.Method, got.Path)
	}
	body := decodeBody(t, got.Body)
	if body["title"] != "New" || body["archived"] != true {
		t.Errorf("body = %v", body)
	}
}

func TestQRImage_Base64Envelope(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, `<svg>hi</svg>`, &got)
	defer srv.Close()

	code, stdout, _ := run(t, srv, "qr", "image", "--qr", "q1")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Path != "/qr-codes/q1/image" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Accept != "image/svg+xml" {
		t.Errorf("Accept = %q, want image/svg+xml (overrides default JSON)", got.Accept)
	}
	if q := parseQuery(t, got.Query); q.Get("format") != "svg" {
		t.Errorf("format = %q, want svg", q.Get("format"))
	}
	var env qrImageEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("stdout not a JSON envelope: %v (%s)", err, stdout)
	}
	if env.Encoding != "base64" || env.Format != "svg" || env.QRCodeID != "q1" {
		t.Errorf("envelope = %+v", env)
	}
	decoded, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil || string(decoded) != `<svg>hi</svg>` {
		t.Errorf("decoded data = %q err=%v, want raw svg bytes", decoded, err)
	}
}

func TestQRImage_PNGToOutputFile(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusOK, "PNGBYTES", &got)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "qr.png")
	code, stdout, _ := run(t, srv, "qr", "image", "--qr", "q1", "--format", "png", "--output", out)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if got.Accept != "image/png" {
		t.Errorf("Accept = %q, want image/png", got.Accept)
	}
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "PNGBYTES" {
		t.Fatalf("output file = %q err=%v", data, err)
	}
	var receipt qrImageReceipt
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &receipt); err != nil {
		t.Fatalf("stdout not a JSON receipt: %v (%s)", err, stdout)
	}
	if receipt.Path != out || receipt.Bytes != len("PNGBYTES") || receipt.Format != "png" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestQRImage_UnauthorizedRejectsCredential(t *testing.T) {
	var got capturedRequest
	srv := newServer(t, http.StatusUnauthorized, `{"message":"UNAUTHORIZED"}`, &got)
	defer srv.Close()

	result, _, _ := runResult(t, srv, "qr", "image", "--qr", "q1")
	if result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", result.ExitCode)
	}
	if !result.CredentialRejected {
		t.Error("want CredentialRejected on 401 from the image endpoint")
	}
}
