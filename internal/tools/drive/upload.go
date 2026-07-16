package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// uploadFields is the field mask requested for an uploaded file: enough to
// report the id and the delivery link.
const uploadFields = "id,name,mimeType,webViewLink"

// multipartBoundary delimits the metadata and media parts of a multipart
// upload. A fixed literal is fine — the body is generated, never user text.
const multipartBoundary = "helio-drive-boundary-8f2a1c"

// workspaceTargets maps a source file extension to the Google Workspace
// mimeType Drive converts it to when --convert is set.
var workspaceTargets = map[string]string{
	".doc":  "application/vnd.google-apps.document",
	".docx": "application/vnd.google-apps.document",
	".odt":  "application/vnd.google-apps.document",
	".rtf":  "application/vnd.google-apps.document",
	".txt":  "application/vnd.google-apps.document",
	".html": "application/vnd.google-apps.document",
	".htm":  "application/vnd.google-apps.document",
	".xls":  "application/vnd.google-apps.spreadsheet",
	".xlsx": "application/vnd.google-apps.spreadsheet",
	".ods":  "application/vnd.google-apps.spreadsheet",
	".csv":  "application/vnd.google-apps.spreadsheet",
	".tsv":  "application/vnd.google-apps.spreadsheet",
	".ppt":  "application/vnd.google-apps.presentation",
	".pptx": "application/vnd.google-apps.presentation",
	".odp":  "application/vnd.google-apps.presentation",
}

func (s *Service) newFilesUploadCmd(token string) *cobra.Command {
	var parent, name string
	var convert bool
	cmd := &cobra.Command{
		Use:   "upload <path>...",
		Short: "Upload local files to Drive (returns webViewLink). --convert turns Office/CSV/text into Google Docs/Sheets.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name != "" && len(args) > 1 {
				return fmt.Errorf("drive: --name cannot be combined with multiple paths")
			}
			results := make([]savedFile, 0, len(args))
			for _, path := range args {
				uploaded, err := s.uploadOne(cmd.Context(), token, path, name, parent, convert)
				if err != nil {
					return err
				}
				results = append(results, uploaded)
			}
			if jsonOut(cmd) {
				if len(results) == 1 {
					return s.emitJSON(results[0])
				}
				return s.emitJSON(results)
			}
			for _, r := range results {
				fmt.Fprintf(s.stdout(), "uploaded %s (%s)\n  link: %s\n", r.Name, r.ID, r.Path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "destination folder id (default: My Drive root)")
	cmd.Flags().StringVar(&name, "name", "", "name for the uploaded file (single path only)")
	cmd.Flags().BoolVar(&convert, "convert", false, "convert Office/CSV/text into the matching Google Workspace format")
	return cmd
}

// uploadOne uploads a single local file, choosing multipart or resumable by
// size. It returns a savedFile whose Path field carries the webViewLink (the
// delivery link is the meaningful "location" for an upload).
func (s *Service) uploadOne(ctx context.Context, token, path, name, parent string, convert bool) (savedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return savedFile{}, fmt.Errorf("drive: read %s: %w", path, err)
	}
	srcMime := sourceMime(path)
	meta := map[string]any{"name": uploadName(name, path)}
	if parent != "" {
		meta["parents"] = []string{parent}
	}
	if convert {
		target, ok := workspaceTargets[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return savedFile{}, fmt.Errorf("drive: --convert does not support %q (supported: Office, CSV/TSV, text, HTML)", filepath.Ext(path))
		}
		meta["mimeType"] = target
	}

	var body []byte
	if int64(len(data)) > s.uploadCutover() {
		body, err = s.uploadResumable(ctx, token, meta, srcMime, data)
	} else {
		body, err = s.uploadMultipart(ctx, token, meta, srcMime, data)
	}
	if err != nil {
		return savedFile{}, err
	}
	var f driveFile
	if err := json.Unmarshal(body, &f); err != nil {
		return savedFile{}, fmt.Errorf("drive: decode upload response: %w", err)
	}
	return savedFile{ID: f.ID, Name: f.Name, Path: f.WebViewLink, Size: len(data)}, nil
}

// uploadMultipart sends metadata + media in one multipart/related request.
func (s *Service) uploadMultipart(ctx context.Context, token string, meta map[string]any, srcMime string, data []byte) ([]byte, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("drive: encode upload metadata: %w", err)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "--%s\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n", multipartBoundary)
	buf.Write(metaJSON)
	fmt.Fprintf(&buf, "\r\n--%s\r\nContent-Type: %s\r\n\r\n", multipartBoundary, srcMime)
	buf.Write(data)
	fmt.Fprintf(&buf, "\r\n--%s--", multipartBoundary)

	q := driveParams()
	q.Set("uploadType", "multipart")
	q.Set("fields", uploadFields)
	endpoint := s.uploadBase() + "/files?" + q.Encode()
	headers := map[string]string{"Content-Type": "multipart/related; boundary=" + multipartBoundary}
	status, _, respBody, err := s.uploadRequest(ctx, token, http.MethodPost, endpoint, "/files", headers, buf.Bytes())
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, s.apiError(status, "/files", respBody)
	}
	return respBody, nil
}

// uploadResumable initiates a resumable session then PUTs the media in a single
// request (design 303: >5MB path).
func (s *Service) uploadResumable(ctx context.Context, token string, meta map[string]any, srcMime string, data []byte) ([]byte, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("drive: encode upload metadata: %w", err)
	}
	q := driveParams()
	q.Set("uploadType", "resumable")
	q.Set("fields", uploadFields)
	initEndpoint := s.uploadBase() + "/files?" + q.Encode()
	initHeaders := map[string]string{
		"Content-Type":            "application/json; charset=UTF-8",
		"X-Upload-Content-Type":   srcMime,
		"X-Upload-Content-Length": strconv.Itoa(len(data)),
	}
	status, respHeaders, respBody, err := s.uploadRequest(ctx, token, http.MethodPost, initEndpoint, "/files", initHeaders, metaJSON)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, s.apiError(status, "/files", respBody)
	}
	session := respHeaders.Get("Location")
	if session == "" {
		return nil, fmt.Errorf("drive: resumable upload: no session URI in response")
	}

	putHeaders := map[string]string{"Content-Type": srcMime}
	status, _, respBody, err = s.uploadRequest(ctx, token, http.MethodPut, session, "/files (resumable)", putHeaders, data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, s.apiError(status, "/files (resumable)", respBody)
	}
	return respBody, nil
}

// uploadRequest performs one upload round trip with arbitrary headers, returning
// status, response headers, and body. It does not classify errors — callers map
// non-2xx via s.apiError so the upload/session boundary is explicit.
func (s *Service) uploadRequest(ctx context.Context, token, method, endpoint, label string, headers map[string]string, payload []byte) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: build upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: %s %s: %w", method, label, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("drive: read upload response: %w", err)
	}
	return resp.StatusCode, resp.Header, body, nil
}

// sourceMime detects a local file's MIME type from its extension, defaulting to
// application/octet-stream.
func sourceMime(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// uploadName resolves the Drive file name: explicit --name, else the basename.
func uploadName(name, path string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return filepath.Base(path)
}
