package x

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaUploadSimpleImage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.png")
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 32))
	if err := os.WriteFile(file, png, 0o600); err != nil {
		t.Fatal(err)
	}

	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/2/media/upload" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Media         string `json:"media"`
			MediaType     string `json:"media_type"`
			MediaCategory string `json:"media_category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(payload.Media)
		if err != nil {
			t.Fatalf("decode media: %v", err)
		}
		if string(decoded) != string(png) || payload.MediaType != "image/png" || payload.MediaCategory != "dm_image" {
			t.Fatalf("payload = type %q category %q media %q", payload.MediaType, payload.MediaCategory, decoded)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"111"}}`)
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file, "--category", "dm_image")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestMediaUploadRejectsUnsupportedOrOversizedFiles(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(file, []byte("not an image"), 0o600); err != nil {
			t.Fatal(err)
		}
		server := newTestServer(t, nil)
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
		if code == 0 || !strings.Contains(stderr, "only JPEG, PNG, and WebP") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "large.png")
		contents := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxSimpleImageBytes)...)
		if err := os.WriteFile(file, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		server := newTestServer(t, nil)
		defer server.Close()
		code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
		if code == 0 || !strings.Contains(stderr, "exceeds the 5 MiB") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
	})
}

func TestMediaStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/2/media/upload" || r.URL.Query().Get("media_id") != "111" {
			t.Fatalf("request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"111","processing_info":{"state":"succeeded"}}}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "media", "status", "111")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}

func TestMediaMetadataAltText(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/2/media/metadata" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		metadata := payload["metadata"].(map[string]any)
		alt := metadata["alt_text"].(map[string]any)
		if payload["id"] != "111" || alt["text"] != "a sunrise" {
			t.Fatalf("payload = %#v", payload)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"111"}}`)
	})
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "media", "metadata", "111", "--alt-text", "a sunrise")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
}
