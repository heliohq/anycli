package x

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
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

func TestMediaUploadSimpleImageDefaultCategory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.png")
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 32))
	if err := os.WriteFile(file, png, 0o600); err != nil {
		t.Fatal(err)
	}

	// A small PNG without --category must stay on the one-shot simple
	// endpoint with media_category tweet_image; any chunked endpoint hit
	// fails the path assertion below.
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/2/media/upload" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			MediaCategory string `json:"media_category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload.MediaCategory != "tweet_image" {
			t.Fatalf("media_category = %q, want tweet_image", payload.MediaCategory)
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"111"}}`)
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if stdout != `{"data":{"id":"111"}}`+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestMediaUploadRejectsUnsupportedFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, nil)
	defer server.Close()
	code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
	if code == 0 || !strings.Contains(stderr, "unsupported media file type") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestMediaUploadChunkedVideo(t *testing.T) {
	file := filepath.Join(t.TempDir(), "clip.mp4")
	content := make([]byte, 9<<20) // 4 MiB + 4 MiB + 1 MiB = 3 segments
	for i := range content {
		content[i] = byte(i)
	}
	if err := os.WriteFile(file, content, 0o600); err != nil {
		t.Fatal(err)
	}

	statusBody := `{"data":{"id":"777","media_key":"13_777","processing_info":{"state":"succeeded","progress_percent":100}}}`
	var uploaded []byte
	var appendCount int
	var finalized bool
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/initialize":
			var payload struct {
				MediaType     string `json:"media_type"`
				TotalBytes    int64  `json:"total_bytes"`
				MediaCategory string `json:"media_category"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode initialize body: %v", err)
			}
			if payload.MediaType != "video/mp4" || payload.TotalBytes != int64(len(content)) || payload.MediaCategory != "tweet_video" {
				t.Fatalf("initialize payload = %+v", payload)
			}
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","media_key":"13_777"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/append":
			var payload struct {
				Media        string `json:"media"`
				SegmentIndex int    `json:"segment_index"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode append body: %v", err)
			}
			if payload.SegmentIndex != appendCount {
				t.Fatalf("segment_index = %d, want %d", payload.SegmentIndex, appendCount)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload.Media)
			if err != nil {
				t.Fatalf("decode segment media: %v", err)
			}
			uploaded = append(uploaded, decoded...)
			appendCount++
			jsonResponse(w, http.StatusOK, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/finalize":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read finalize body: %v", err)
			}
			if len(body) != 0 {
				t.Fatalf("finalize body = %q, want empty", body)
			}
			finalized = true
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","media_key":"13_777","processing_info":{"state":"pending","check_after_secs":1}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/2/media/upload":
			query := r.URL.Query()
			if query.Get("media_id") != "777" || query.Get("command") != "STATUS" {
				t.Fatalf("status query = %q", r.URL.RawQuery)
			}
			jsonResponse(w, http.StatusOK, statusBody)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()

	code, stdout, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if appendCount != 3 {
		t.Fatalf("append count = %d, want 3", appendCount)
	}
	if !bytes.Equal(uploaded, content) {
		t.Fatalf("uploaded bytes do not match the original file (%d vs %d bytes)", len(uploaded), len(content))
	}
	if !finalized {
		t.Fatal("finalize endpoint was never called")
	}
	if stdout != statusBody+"\n" {
		t.Fatalf("stdout = %q, want final STATUS body", stdout)
	}
}

func TestMediaUploadCategoryDerivation(t *testing.T) {
	gif := []byte("GIF89a" + strings.Repeat("x", 64))
	mp4 := bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03}, 16)
	cases := []struct {
		name         string
		fileName     string
		contents     []byte
		extraArgs    []string
		wantType     string
		wantCategory string
	}{
		{"mp4 derives tweet_video", "clip.mp4", mp4, nil, "video/mp4", "tweet_video"},
		{"gif derives tweet_gif", "anim.gif", gif, nil, "image/gif", "tweet_gif"},
		{"explicit category passes through", "clip.mp4", mp4, []string{"--category", "dm_video"}, "video/mp4", "dm_video"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tc.fileName)
			if err := os.WriteFile(file, tc.contents, 0o600); err != nil {
				t.Fatal(err)
			}
			finalizeBody := `{"data":{"id":"777","processing_info":{"state":"succeeded","progress_percent":100}}}`
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/initialize":
					var payload struct {
						MediaType     string `json:"media_type"`
						MediaCategory string `json:"media_category"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatalf("decode initialize body: %v", err)
					}
					if payload.MediaType != tc.wantType || payload.MediaCategory != tc.wantCategory {
						t.Fatalf("initialize payload = type %q category %q, want type %q category %q",
							payload.MediaType, payload.MediaCategory, tc.wantType, tc.wantCategory)
					}
					jsonResponse(w, http.StatusOK, `{"data":{"id":"777"}}`)
				case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/append":
					jsonResponse(w, http.StatusOK, `{}`)
				case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/finalize":
					jsonResponse(w, http.StatusOK, finalizeBody)
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			})
			defer server.Close()

			args := append([]string{"media", "upload", "--file", file}, tc.extraArgs...)
			code, stdout, stderr := run(t, server, fullEnv(), args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr)
			}
			if stdout != finalizeBody+"\n" {
				t.Fatalf("stdout = %q, want finalize body", stdout)
			}
		})
	}
}

func TestMediaUploadProcessingFailed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(file, bytes.Repeat([]byte{0x01}, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/initialize":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/append":
			jsonResponse(w, http.StatusOK, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/finalize":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","processing_info":{"state":"pending","check_after_secs":1}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/2/media/upload":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","processing_info":{"state":"failed","error":{"code":3,"name":"UnsupportedMedia","message":"Unsupported video format"}}}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
	if code == 0 {
		t.Fatalf("exit code = %d, want non-zero", code)
	}
	if !strings.Contains(stderr, "media processing failed") || !strings.Contains(stderr, "Unsupported video format") {
		t.Fatalf("stderr = %q, want processing-failed error with platform detail", stderr)
	}
}

func TestMediaUploadPollTimeout(t *testing.T) {
	file := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(file, bytes.Repeat([]byte{0x01}, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	// The single STATUS poll reports in_progress with a check_after_secs far
	// beyond the 5-minute budget, so the command times out without sleeping.
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/initialize":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/append":
			jsonResponse(w, http.StatusOK, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/777/finalize":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","processing_info":{"state":"pending","check_after_secs":1}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/2/media/upload":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"777","processing_info":{"state":"in_progress","check_after_secs":3600,"progress_percent":40}}}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer server.Close()

	code, _, stderr := run(t, server, fullEnv(), "media", "upload", "--file", file)
	if code == 0 {
		t.Fatalf("exit code = %d, want non-zero", code)
	}
	if !strings.Contains(stderr, "media 777 still processing after 5m") ||
		!strings.Contains(stderr, "x media status 777") ||
		!strings.Contains(stderr, "x post create --media-id 777") {
		t.Fatalf("stderr = %q, want timeout self-serve message", stderr)
	}
}

func TestMediaStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/2/media/upload" ||
			query.Get("media_id") != "111" || query.Get("command") != "STATUS" {
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
