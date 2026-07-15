package figma

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAssetsDownloadRendersNodesWithoutLeakingPAT(t *testing.T) {
	var server *httptest.Server
	var mu sync.Mutex
	assetToken := "not-called"
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"images":{"1:2":"`+server.URL+`/assets/hero.png"}}`)
		case "/assets/hero.png":
			mu.Lock()
			assetToken = r.Header.Get("X-Figma-Token")
			mu.Unlock()
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outputDir := t.TempDir()
	code, stdout, stderr := runService(t, server,
		"assets", "download", "--file-key", "abc", "--ids", "1:2", "--output-dir", outputDir,
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, "1_2.png"))
	if err != nil {
		t.Fatalf("read downloaded asset: %v", err)
	}
	if string(contents) != "png-bytes" {
		t.Errorf("contents = %q", contents)
	}
	mu.Lock()
	defer mu.Unlock()
	if assetToken != "" {
		t.Errorf("signed asset request leaked X-Figma-Token %q", assetToken)
	}
	if !strings.Contains(stdout, `"file":"1_2.png"`) || !strings.Contains(stdout, `"bytes":9`) {
		t.Errorf("stdout = %s", stdout)
	}
}

func TestAssetsDownloadOriginalImageFills(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/files/abc/images":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"error":false,"status":200,"meta":{"images":{"ref/hash":"`+server.URL+`/assets/fill"}}}`)
		case "/assets/fill":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outputDir := t.TempDir()
	code, _, stderr := runService(t, server,
		"assets", "download-fills", "--file-key", "abc", "--output-dir", outputDir,
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, "ref_hash.jpg"))
	if err != nil || string(contents) != "jpeg" {
		t.Fatalf("downloaded fill = %q, err = %v", contents, err)
	}
}

func TestAssetsDownloadRefusesOverwriteByDefault(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/abc":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"images":{"1:2":"`+server.URL+`/assets/hero.png"}}`)
		case "/assets/hero.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("new"))
		}
	}))
	defer server.Close()

	outputDir := t.TempDir()
	target := filepath.Join(outputDir, "1_2.png")
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runService(t, server,
		"assets", "download", "--file-key", "abc", "--ids", "1:2", "--output-dir", outputDir,
	)
	if code != 1 || !strings.Contains(stderr, "already exists; pass --overwrite") {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	contents, _ := os.ReadFile(target)
	if string(contents) != "existing" {
		t.Errorf("existing file was changed: %q", contents)
	}
}

func TestDownloadAssetRejectsPlaintextURL(t *testing.T) {
	service := &Service{}
	_, err := service.downloadAsset(t.Context(), assetSource{
		ID:       "1:2",
		URL:      "http://example.com/asset.png",
		BaseName: "1_2",
	}, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "invalid asset URL") {
		t.Fatalf("downloadAsset error = %v, want invalid asset URL", err)
	}
}

func TestDownloadAssetRejectsHTTPSDowngradeRedirect(t *testing.T) {
	targetReached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetReached = true
	}))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/asset.png", http.StatusFound)
	}))
	defer source.Close()

	service := &Service{HC: source.Client()}
	_, err := service.downloadAsset(t.Context(), assetSource{
		ID:       "1:2",
		URL:      source.URL + "/signed.png",
		BaseName: "1_2",
	}, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("downloadAsset error = %v, want HTTPS redirect rejection", err)
	}
	if targetReached {
		t.Fatal("HTTPS downgrade redirect reached the plaintext target")
	}
}
