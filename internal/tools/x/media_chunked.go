package x

// Chunked media upload over the X API v2 initialize/append/finalize/STATUS
// endpoints. Unlike the repo's "no client-side polling loop" default, this
// path deliberately polls until the media is ready: a video media id is
// unusable (posting 400s) until processing succeeds, and processing time is
// bounded. `x media status` stays available as the self-serve escape hatch
// after a poll timeout.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	chunkBytes        = 4 << 20 // 512 MB / 4 MiB = 128 segments, far below the 999-segment cap
	mediaReadyTimeout = 5 * time.Minute

	maxGIFBytes   = 15 << 20
	maxVideoBytes = 512 << 20
)

// chunkedUpload runs initialize → appendSegments → finalize → waitMediaReady
// and returns the final FINALIZE/STATUS response body once the media is ready
// to attach.
func (s *Service) chunkedUpload(ctx context.Context, token, file, category string) ([]byte, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	sniff, err := sniffMediaFile(file)
	if err != nil {
		return nil, err
	}
	mediaType, _, err := mediaTypeForUpload(sniff, file)
	if err != nil {
		return nil, err
	}
	if mediaType == "image/gif" && info.Size() > maxGIFBytes {
		return nil, fmt.Errorf("GIF media file exceeds the 15 MB limit")
	}
	if strings.HasPrefix(mediaType, "video/") && info.Size() > maxVideoBytes {
		return nil, fmt.Errorf("video media file exceeds the 512 MB limit")
	}

	initPayload := struct {
		MediaType     string `json:"media_type"`
		TotalBytes    int64  `json:"total_bytes"`
		MediaCategory string `json:"media_category"`
	}{MediaType: mediaType, TotalBytes: info.Size(), MediaCategory: category}
	body, err := s.call(ctx, token, http.MethodPost, "/2/media/upload/initialize", nil, initPayload)
	if err != nil {
		return nil, err
	}
	var init struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &init); err != nil {
		return nil, fmt.Errorf("x: decode initialize response: %w", err)
	}
	if init.Data.ID == "" {
		return nil, fmt.Errorf("x: initialize response missing media id")
	}

	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("open media file: %w", err)
	}
	defer f.Close()
	if err := s.appendSegments(ctx, token, init.Data.ID, f, info.Size()); err != nil {
		return nil, err
	}

	body, err = s.call(ctx, token, http.MethodPost, "/2/media/upload/"+init.Data.ID+"/finalize", nil, nil)
	if err != nil {
		return nil, err
	}
	return s.waitMediaReady(ctx, token, init.Data.ID, body)
}

// appendSegments posts the file as ordered base64 JSON segments with
// segment_index starting at 0. JSON+base64 keeps client.go's single JSON
// request path; a 4 MiB chunk is ~5.6 MB encoded, which is acceptable.
func (s *Service) appendSegments(ctx context.Context, token, mediaID string, f *os.File, size int64) error {
	total := (size + chunkBytes - 1) / chunkBytes
	buf := make([]byte, chunkBytes)
	for i, remaining := int64(0), size; remaining > 0; i++ {
		n := int64(chunkBytes)
		if remaining < n {
			n = remaining
		}
		if _, err := io.ReadFull(f, buf[:n]); err != nil {
			return fmt.Errorf("read media file: %w", err)
		}
		remaining -= n
		fmt.Fprintf(s.stderr(), "append %d/%d\n", i+1, total)
		payload := struct {
			Media        string `json:"media"`
			SegmentIndex int64  `json:"segment_index"`
		}{Media: base64.StdEncoding.EncodeToString(buf[:n]), SegmentIndex: i}
		if _, err := s.call(ctx, token, http.MethodPost, "/2/media/upload/"+mediaID+"/append", nil, payload); err != nil {
			return err
		}
	}
	return nil
}

// waitMediaReady polls GET /2/media/upload?command=STATUS until processing
// reaches a terminal state, starting from the finalize response body. Three
// terminal conditions: state "succeeded" (return the body), state "failed"
// (error), and a response without processing_info — a rare but API-legal
// shape that means there is nothing to wait for (terminal success).
func (s *Service) waitMediaReady(ctx context.Context, token, mediaID string, first []byte) ([]byte, error) {
	deadline := time.Now().Add(mediaReadyTimeout)
	body := first
	for polls := 0; ; polls++ {
		var status struct {
			Data struct {
				ProcessingInfo *struct {
					State           string          `json:"state"`
					CheckAfterSecs  int64           `json:"check_after_secs"`
					ProgressPercent int64           `json:"progress_percent"`
					Error           json.RawMessage `json:"error"`
				} `json:"processing_info"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &status); err != nil {
			return nil, fmt.Errorf("x: decode media status: %w", err)
		}
		info := status.Data.ProcessingInfo
		if info == nil {
			return body, nil
		}
		switch info.State {
		case "succeeded":
			return body, nil
		case "failed":
			detail := strings.TrimSpace(string(info.Error))
			if detail == "" || detail == "null" {
				detail = "no error details provided"
			}
			return nil, fmt.Errorf("x: media processing failed: %s", detail)
		}
		fmt.Fprintf(s.stderr(), "processing: %s %d%%\n", info.State, info.ProgressPercent)
		// Poll-then-sleep: the first STATUS goes out immediately; between
		// polls honor check_after_secs (default 2s) under the hard 5m cap —
		// a wait that would overrun the deadline times out right away.
		if polls > 0 {
			interval := time.Duration(info.CheckAfterSecs) * time.Second
			if interval <= 0 {
				interval = 2 * time.Second
			}
			if time.Until(deadline) < interval {
				return nil, fmt.Errorf("x: media %s still processing after 5m; check later with: x media status %s; once state is succeeded, attach with: x post create --media-id %s", mediaID, mediaID, mediaID)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
		query := url.Values{"media_id": {mediaID}, "command": {"STATUS"}}
		next, err := s.call(ctx, token, http.MethodGet, "/2/media/upload", query, nil)
		if err != nil {
			return nil, err
		}
		body = next
	}
}

// mediaTypeForUpload resolves the upload media_type and the default
// media_category for a file. Images are sniffed via http.DetectContentType;
// video types are mapped from the file extension (mirrors tiktok's
// videoContentType in internal/tools/tiktok/upload.go).
func mediaTypeForUpload(sniff []byte, path string) (mediaType, defaultCategory string, err error) {
	switch detected := http.DetectContentType(sniff); detected {
	case "image/gif":
		return detected, "tweet_gif", nil
	case "image/jpeg", "image/png", "image/webp":
		return detected, "tweet_image", nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4":
		return "video/mp4", "tweet_video", nil
	case ".webm":
		return "video/webm", "tweet_video", nil
	case ".mov":
		return "video/quicktime", "tweet_video", nil
	case ".ts":
		return "video/mp2t", "tweet_video", nil
	}
	return "", "", fmt.Errorf("unsupported media file type (want a JPEG, PNG, WebP, or GIF image, or an .mp4, .webm, .mov, or .ts video)")
}

// sniffMediaFile reads the first 512 bytes of a file for content-type
// detection.
func sniffMediaFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open media file: %w", err)
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	return buf[:n], nil
}
