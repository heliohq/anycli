package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// DefaultUploadBaseURL is the resumable-upload base (different host prefix
// from the Data API base).
const DefaultUploadBaseURL = "https://www.googleapis.com/upload/youtube/v3"

// newVideosUploadCmd uploads a video via the resumable protocol as a single
// full-body PUT: init yields a session URL in the Location header, then one
// PUT streams the whole file. No 308 resume, no chunking, no retry — a
// failure means the whole upload reruns. A 2xx on the PUT means the video
// exists; transcoding is YouTube's business, so there is no status polling.
func (s *Service) newVideosUploadCmd(token string) *cobra.Command {
	var file, title, description, tags, categoryID, privacy string
	var madeForKids bool
	cmd := &cobra.Command{
		Use:         "upload",
		Short:       "Upload a video file (resumable protocol, single PUT)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return &usageError{msg: "--title is required"}
			}
			if file == "" {
				return &usageError{msg: "--file is required"}
			}
			if !validPrivacy[privacy] {
				return &usageError{msg: fmt.Sprintf("--privacy must be public|unlisted|private, got %q", privacy)}
			}
			info, err := os.Stat(file)
			if err != nil {
				return &usageError{msg: fmt.Sprintf("read video file: %v", err)}
			}
			if info.IsDir() {
				return &usageError{msg: fmt.Sprintf("--file %q is a directory", file)}
			}
			if info.Size() == 0 {
				return &usageError{msg: fmt.Sprintf("--file %q is empty", file)}
			}
			size := info.Size()
			ctype := videoContentType(file)

			snippet := map[string]any{"title": title}
			if description != "" {
				snippet["description"] = description
			}
			if tags != "" {
				snippet["tags"] = splitCSV(tags)
			}
			if categoryID != "" {
				snippet["categoryId"] = categoryID
			}
			meta := map[string]any{
				"snippet": snippet,
				"status": map[string]any{
					"privacyStatus":           privacy,
					"selfDeclaredMadeForKids": madeForKids,
				},
			}

			sessionURL, err := s.initResumableUpload(cmd.Context(), token, meta, size, ctype)
			if err != nil {
				return err
			}
			body, err := s.putResumableBody(cmd.Context(), token, sessionURL, file, size, ctype)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var v struct {
				ID      string `json:"id"`
				Snippet struct {
					Title string `json:"title"`
				} `json:"snippet"`
				Status struct {
					PrivacyStatus string `json:"privacyStatus"`
				} `json:"status"`
			}
			if err := json.Unmarshal(body, &v); err != nil {
				return &apiError{msg: fmt.Sprintf("youtube: decode upload response: %v", err), err: err}
			}
			fmt.Fprintf(s.stdout(), "uploaded video %s — %s (%s) https://youtu.be/%s\n",
				v.ID, v.Snippet.Title, v.Status.PrivacyStatus, v.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to the local video file")
	cmd.Flags().StringVar(&title, "title", "", "video title")
	cmd.Flags().StringVar(&description, "description", "", "video description")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags")
	cmd.Flags().StringVar(&categoryID, "category-id", "", "numeric category id")
	cmd.Flags().StringVar(&privacy, "privacy", "private", "public|unlisted|private")
	cmd.Flags().BoolVar(&madeForKids, "made-for-kids", false, "declare the video as made for kids")
	return cmd
}

// initResumableUpload POSTs the video metadata to the resumable-upload base
// and returns the session URL from the Location response header. The Location
// is an absolute URL and is used verbatim — no base substitution. These
// requests don't fit call()'s JSON-in/body-out contract (X-Upload-Content-*
// headers, Location read), so they are bespoke, but a non-2xx still surfaces
// as a *apiError through the same apiMessage/classification chain.
func (s *Service) initResumableUpload(ctx context.Context, token string, meta map[string]any, size int64, ctype string) (string, error) {
	q := url.Values{}
	q.Set("uploadType", "resumable")
	q.Set("part", "snippet,status")
	endpoint := s.uploadBase() + "/videos?" + q.Encode()
	payload, err := json.Marshal(meta)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("youtube: encode upload metadata: %v", err), err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("youtube: build upload init request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("X-Upload-Content-Type", ctype)
	resp, err := s.client().Do(req)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("youtube: upload init: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("youtube: read upload init response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", newUploadAPIError(resp.StatusCode, body)
	}
	sessionURL := resp.Header.Get("Location")
	if sessionURL == "" {
		return "", &apiError{msg: fmt.Sprintf("youtube: resumable upload init returned no Location header (HTTP %d)", resp.StatusCode), status: resp.StatusCode}
	}
	return sessionURL, nil
}

// putResumableBody streams the whole file to the session URL in one PUT and
// returns the video resource body on 2xx.
func (s *Service) putResumableBody(ctx context.Context, token, sessionURL, path string, size int64, ctype string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: open video file: %v", err), err: err}
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURL, f)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: build upload request: %v", err), err: err}
	}
	// The body is an *os.File, which net/http would send with chunked
	// transfer-encoding — contradicting the X-Upload-Content-Length declared
	// at init. Set ContentLength explicitly so the PUT carries it.
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ctype)
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: upload video: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("youtube: read upload response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newUploadAPIError(resp.StatusCode, body)
	}
	return body, nil
}

// newUploadAPIError mirrors call()'s non-2xx handling for the bespoke upload
// requests: provider message via apiMessage, the missing-scope hint on
// 401/403, and credential-rejection classification, wrapped as *apiError.
func newUploadAPIError(status int, body []byte) *apiError {
	hint := ""
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		hint = scopeHint
	}
	raw := fmt.Errorf("youtube API error (HTTP %d): %s%s", status, apiMessage(body), hint)
	classified := classifyCredentialError(status, body, raw)
	return &apiError{msg: classified.Error(), status: status, err: classified}
}

// videoContentType maps a file extension to a video content type, defaulting
// to video/mp4. Copied from internal/tools/tiktok/upload.go — extract a
// shared helper on the third copy.
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
