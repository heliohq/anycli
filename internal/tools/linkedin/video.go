// Video upload via the versioned /rest/videos API: initializeUpload →
// chunk PUTs to server-issued pre-signed URLs → finalizeUpload → status poll.
//
// NOTE: unlike this repo's "no client-side polling loop" default (hunter,
// tiktok), `video upload` deliberately polls until the video is AVAILABLE:
// the URN is unusable for posting while processing (attaching it 400s), and
// processing time is bounded. Exit 0 means the URN on stdout is immediately
// attachable with `post create --video-urn`. `video get` is the read-only
// self-rescue channel after a poll timeout.

package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// videoPollInterval / videoReadyTimeout bound the post-finalize processing
// poll: GET the video every 3s until AVAILABLE, giving up after 5 minutes.
const videoPollInterval = 3 * time.Second
const videoReadyTimeout = 5 * time.Minute

// maxVideoBytes is LinkedIn's 500MB video size cap, pre-checked locally.
const maxVideoBytes = 500 << 20

// uploadInstruction is one server-defined byte range of the file to PUT to a
// pre-signed URL. The server decides the chunking (4194304-byte slices); the
// client never picks a chunk size.
type uploadInstruction struct {
	UploadURL string `json:"uploadUrl"`
	FirstByte int64  `json:"firstByte"`
	LastByte  int64  `json:"lastByte"`
}

func (s *Service) newVideoCmd(token, personURN string) *cobra.Command {
	video := &cobra.Command{Use: "video", Short: "Videos"}
	video.AddCommand(s.newVideoUploadCmd(token, personURN), s.newVideoGetCmd(token))
	return video
}

func (s *Service) newVideoUploadCmd(token, personURN string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:         "upload",
		Short:       "Upload an MP4 video and wait until it is AVAILABLE",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"}, // POST /rest/videos?action=initializeUpload|finalizeUpload
		RunE: func(cmd *cobra.Command, _ []string) error {
			if personURN == "" {
				return fmt.Errorf("person_urn missing — reconnect LinkedIn to capture it")
			}
			body, err := s.uploadVideo(cmd.Context(), token, personURN, file)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "local MP4 video file to upload")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (s *Service) newVideoGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <video-urn>",
		Short:       "Show a video's processing status by URN",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET /rest/videos/{urn}
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/rest/videos/"+encodeVideoURN(args[0]), true, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// uploadVideo runs the full flow: initializeUpload → uploadParts →
// finalizeUpload → waitVideoAvailable. The returned body is the final GET
// video object (id + status AVAILABLE), emitted verbatim.
func (s *Service) uploadVideo(ctx context.Context, token, personURN, file string) ([]byte, error) {
	size, err := checkVideoFile(file)
	if err != nil {
		return nil, err
	}
	initBody, err := s.call(ctx, token, http.MethodPost, "/rest/videos?action=initializeUpload", true, map[string]any{
		"initializeUploadRequest": map[string]any{
			"owner":         personURN,
			"fileSizeBytes": size,
		},
	})
	if err != nil {
		return nil, err
	}
	var init struct {
		Value struct {
			Video              string              `json:"video"`
			UploadInstructions []uploadInstruction `json:"uploadInstructions"`
		} `json:"value"`
	}
	if err := json.Unmarshal(initBody, &init); err != nil {
		return nil, fmt.Errorf("linkedin: decode initializeUpload response: %w", err)
	}
	if init.Value.Video == "" || len(init.Value.UploadInstructions) == 0 {
		return nil, fmt.Errorf("linkedin: initializeUpload response missing video URN or uploadInstructions")
	}
	etags, err := s.uploadParts(ctx, file, init.Value.UploadInstructions)
	if err != nil {
		return nil, err
	}
	if _, err := s.call(ctx, token, http.MethodPost, "/rest/videos?action=finalizeUpload", true, map[string]any{
		"finalizeUploadRequest": map[string]any{
			"video":           init.Value.Video,
			"uploadToken":     "",
			"uploadedPartIds": etags,
		},
	}); err != nil {
		return nil, err
	}
	return s.waitVideoAvailable(ctx, token, init.Value.Video, videoReadyTimeout)
}

// uploadParts PUTs each server-defined byte range to its pre-signed URL, in
// instruction order, and returns the collected part ids in the same order.
func (s *Service) uploadParts(ctx context.Context, file string, instructions []uploadInstruction) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("linkedin: open video file: %w", err)
	}
	defer f.Close()

	etags := make([]string, 0, len(instructions))
	for i, in := range instructions {
		length := in.LastByte - in.FirstByte + 1
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, in.UploadURL,
			io.NewSectionReader(f, in.FirstByte, length))
		if err != nil {
			return nil, fmt.Errorf("linkedin: build upload request for part %d/%d: %w", i+1, len(instructions), err)
		}
		// *io.SectionReader is not in net/http's ContentLength auto-detection
		// whitelist (*bytes.Reader / *bytes.Buffer / *strings.Reader): without
		// an explicit length the PUT goes out as chunked transfer-encoding,
		// which pre-signed upload hosts reject.
		req.ContentLength = length
		// Pre-signed upload URL: raw bytes only — no Bearer token, no
		// versioned headers.
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := s.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("linkedin: upload part %d/%d: %w", i+1, len(instructions), err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("linkedin: upload part %d/%d (HTTP %d): %s", i+1, len(instructions), resp.StatusCode, string(body))
		}
		// The part id is the ETag response *header* (not a body field), and
		// finalize needs uploadedPartIds in uploadInstructions order —
		// sequential upload keeps them aligned by construction.
		etag := resp.Header.Get("ETag")
		if etag == "" {
			return nil, fmt.Errorf("linkedin: upload part %d/%d: response missing ETag header", i+1, len(instructions))
		}
		etags = append(etags, etag)
		fmt.Fprintf(s.stderr(), "upload part %d/%d\n", i+1, len(instructions))
	}
	return etags, nil
}

// waitVideoAvailable polls GET /rest/videos/{urn} (poll first, then sleep)
// until the video is AVAILABLE. timeout is videoReadyTimeout in production;
// tests pass 0 so the first non-terminal poll trips the deadline with no
// sleep.
func (s *Service) waitVideoAvailable(ctx context.Context, token, urn string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	for {
		body, err := s.call(ctx, token, http.MethodGet, "/rest/videos/"+encodeVideoURN(urn), true, nil)
		if err != nil {
			return nil, err
		}
		var video struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(body, &video); err != nil {
			return nil, fmt.Errorf("linkedin: decode video status: %w", err)
		}
		switch video.Status {
		case "AVAILABLE":
			return body, nil
		case "PROCESSING_FAILED":
			return nil, fmt.Errorf("linkedin: video processing failed (status PROCESSING_FAILED)")
		}
		fmt.Fprintf(s.stderr(), "processing: %s\n", video.Status)
		if !time.Now().Before(deadline) {
			// The rescue hint carries the *raw* URN — escaping is video get's
			// internal concern; the AI passes the URN back verbatim. "5m" is
			// videoReadyTimeout, the production value.
			return nil, fmt.Errorf("linkedin: video %s not AVAILABLE after 5m (last status %s); check later with: linkedin video get %s",
				urn, video.Status, urn)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(videoPollInterval):
		}
	}
}

// encodeVideoURN percent-encodes a video URN for use as a Restli path
// segment. url.PathEscape is deliberately NOT used: RFC 3986 allows ':' in
// path segments (pchar), so PathEscape is a no-op on URNs and the Restli
// router then rejects the bare urn:li:video:… path. The URN character set
// has no other characters needing escape.
func encodeVideoURN(urn string) string {
	return strings.ReplaceAll(urn, ":", "%3A")
}

// checkVideoFile runs the cheap local pre-checks (extension, existence,
// non-directory, non-empty, size cap) and returns the file size. Everything
// else (duration, codec, resolution) is the platform's call — its error is
// surfaced verbatim.
func checkVideoFile(file string) (int64, error) {
	if ext := strings.ToLower(filepath.Ext(file)); ext != ".mp4" {
		return 0, fmt.Errorf("linkedin: only MP4 video is supported (got %s)", ext)
	}
	info, err := os.Stat(file)
	if err != nil {
		return 0, fmt.Errorf("linkedin: read video file: %w", err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("linkedin: --file %q is a directory", file)
	}
	if info.Size() == 0 {
		return 0, fmt.Errorf("linkedin: --file %q is empty", file)
	}
	if info.Size() > maxVideoBytes {
		return 0, fmt.Errorf("linkedin: --file %q exceeds the 500MB limit", file)
	}
	return info.Size(), nil
}
