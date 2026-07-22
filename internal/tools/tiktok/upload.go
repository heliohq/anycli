package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// uploadSpec carries what the post-init call needs to complete a FILE_UPLOAD:
// the local path and its byte size (echoed into source_info and used to build
// the single-chunk Content-Range on PUT).
type uploadSpec struct {
	path string
	size int64
}

// buildSource turns the mutually-exclusive --file / --url flags into a TikTok
// source_info object. --url uses PULL_FROM_URL (TikTok fetches the video, no
// second step). --file uses FILE_UPLOAD as a single chunk; the returned
// uploadSpec is non-nil so the caller PUTs the bytes after init.
func (s *Service) buildSource(file, videoURL string) (map[string]any, *uploadSpec, error) {
	if videoURL != "" {
		return map[string]any{
			"source":    "PULL_FROM_URL",
			"video_url": videoURL,
		}, nil, nil
	}

	info, err := os.Stat(file)
	if err != nil {
		return nil, nil, fmt.Errorf("tiktok: read video file: %w", err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("tiktok: --file %q is a directory", file)
	}
	size := info.Size()
	source := map[string]any{
		"source":            "FILE_UPLOAD",
		"video_size":        size,
		"chunk_size":        size,
		"total_chunk_count": 1,
	}
	return source, &uploadSpec{path: file, size: size}, nil
}

// uploadFile PUTs the whole video file to the upload_url returned by the
// post-init call, as a single byte range.
func (s *Service) uploadFile(ctx context.Context, data json.RawMessage, spec *uploadSpec) error {
	var init struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(data, &init); err != nil {
		return fmt.Errorf("tiktok: decode init response: %w", err)
	}
	if init.UploadURL == "" {
		return fmt.Errorf("tiktok: init response missing upload_url for file upload")
	}

	f, err := os.Open(spec.path)
	if err != nil {
		return fmt.Errorf("tiktok: open video file: %w", err)
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, init.UploadURL, f)
	if err != nil {
		return fmt.Errorf("tiktok: build upload request: %w", err)
	}
	req.ContentLength = spec.size
	req.Header.Set("Content-Type", videoContentType(spec.path))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", spec.size-1, spec.size))

	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("tiktok: upload video: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
		return fmt.Errorf("tiktok: upload video (HTTP %d %s): %s",
			resp.StatusCode, http.StatusText(resp.StatusCode), redact(string(body), ""))
	}
	return nil
}

// videoContentType maps a file extension to one of TikTok's accepted video
// content types, defaulting to video/mp4.
func videoContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		if ct := mime.TypeByExtension(filepath.Ext(path)); strings.HasPrefix(ct, "video/") {
			return ct
		}
		return "video/mp4"
	}
}
