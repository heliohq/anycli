package typefully

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// newMediaCmd groups media upload + status. Attaching an image is a two-step
// flow: request a presigned upload URL, PUT the bytes to it, then reference the
// returned media_id from a draft's post (`draft create --media-id <id>`).
func (s *Service) newMediaCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "media", Short: "Upload media and check media processing status"}
	cmd.AddCommand(s.newMediaUploadCmd(token), s.newMediaStatusCmd(token))
	return cmd
}

func (s *Service) newMediaUploadCmd(token string) *cobra.Command {
	var socialSet, file string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a media file (POST /v2/social-sets/{id}/media/upload, then PUT bytes)",
		Long: "Two round trips inside one command: it asks Typefully for a presigned URL,\n" +
			"then PUTs the bytes straight to that URL. The whole file is read into\n" +
			"memory first and the content type is inferred from the extension, so an\n" +
			"extensionless file is uploaded as application/octet-stream. It prints\n" +
			"`media_id`, which `draft create --media-id` attaches to the first post of\n" +
			"a thread. Upload finishing is not processing finishing — that is\n" +
			"`media status`.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := os.ReadFile(file)
			if err != nil {
				return &usageError{msg: "cannot read --file: " + err.Error()}
			}
			contentType := mime.TypeByExtension(filepath.Ext(file))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			reqBody := map[string]any{"filename": filepath.Base(file), "content_type": contentType}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, scopedPath(socialSet, "/media/upload"), nil, reqBody)
			if err != nil {
				return err
			}
			var presign map[string]any
			if err := json.Unmarshal(resp, &presign); err != nil {
				return &apiError{message: "decode media upload response: " + err.Error(), kind: "api"}
			}
			uploadURL := firstString(presign, "upload_url", "url", "presigned_url")
			mediaID := firstString(presign, "media_id", "id")
			if uploadURL == "" || mediaID == "" {
				return &apiError{message: "media upload response missing upload_url/media_id; use the raw API for this media flow", kind: "api"}
			}
			if err := s.putBytes(cmd.Context(), uploadURL, contentType, data); err != nil {
				return err
			}
			return s.emitValue(map[string]any{"media_id": mediaID, "uploaded": true, "content_type": contentType})
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&file, "file", "", "path to the media file; required")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (s *Service) newMediaStatusCmd(token string) *cobra.Command {
	var socialSet, id string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Get media processing status (GET /v2/social-sets/{id}/media/{media_id})",
		Long: "Reports whether the uploaded file has finished processing on Typefully's\n" +
			"side. Video and large images are not usable in a published post until it\n" +
			"has, so this is the check between `media upload` and a draft that has to\n" +
			"go out immediately.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/media/"+id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&id, "id", "", "media id; required")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// putBytes uploads raw bytes to a presigned (absolute, off-Typefully) URL. No
// Authorization header — the presigned URL carries its own auth.
func (s *Service) putBytes(ctx context.Context, uploadURL, contentType string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return &apiError{message: "build media PUT: " + err.Error(), kind: "api"}
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.client().Do(req)
	if err != nil {
		return &apiError{message: "media PUT: " + err.Error(), kind: "api"}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiError{status: resp.StatusCode, message: "presigned media upload rejected the bytes", kind: "api"}
	}
	return nil
}

// firstString returns the first key present in m whose value is a non-empty
// string.
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
