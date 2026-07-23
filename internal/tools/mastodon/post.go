package mastodon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	// maxAttachments is Mastodon's per-status attachment cap.
	maxAttachments = 4
	// mediaPollInterval / mediaPollMaxAttempts bound the async media-processing
	// poll after a 202 Accepted upload.
	mediaPollInterval    = 1 * time.Second
	mediaPollMaxAttempts = 30
)

// validVisibilities is the closed set Mastodon accepts for a status.
var validVisibilities = map[string]bool{"public": true, "unlisted": true, "private": true, "direct": true}

func (rt *runContext) newPostCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Post a status (text, reply, content warning, visibility, media)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE:        rt.runPostCreate,
	}
	cmd.Flags().String("text", "", "status text (required unless media is attached)")
	cmd.Flags().String("reply-to", "", "id of the status this replies to")
	cmd.Flags().String("cw", "", "content warning / spoiler text")
	cmd.Flags().String("visibility", "", "public, unlisted, private, or direct")
	cmd.Flags().String("lang", "", "ISO 639 language code, e.g. en")
	cmd.Flags().StringArray("image", nil, "path to an image to attach (repeatable, max 4)")
	cmd.Flags().StringArray("alt", nil, "alt text for the image at the same position (repeatable)")
	return cmd
}

// statusCreateRequest is the JSON body for POST /api/v1/statuses.
type statusCreateRequest struct {
	Status      string   `json:"status,omitempty"`
	InReplyToID string   `json:"in_reply_to_id,omitempty"`
	SpoilerText string   `json:"spoiler_text,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
	Language    string   `json:"language,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

func (rt *runContext) runPostCreate(cmd *cobra.Command, _ []string) error {
	text, _ := cmd.Flags().GetString("text")
	images, _ := cmd.Flags().GetStringArray("image")
	alts, _ := cmd.Flags().GetStringArray("alt")

	if strings.TrimSpace(text) == "" && len(images) == 0 {
		return &usageError{msg: "post create requires --text or at least one --image"}
	}
	if len(images) > maxAttachments {
		return &usageError{msg: fmt.Sprintf("at most %d images may be attached", maxAttachments)}
	}

	req := statusCreateRequest{Status: text}
	if v, _ := cmd.Flags().GetString("reply-to"); v != "" {
		req.InReplyToID = v
	}
	if v, _ := cmd.Flags().GetString("cw"); v != "" {
		req.SpoilerText = v
	}
	if v, _ := cmd.Flags().GetString("visibility"); v != "" {
		if !validVisibilities[v] {
			return &usageError{msg: "invalid --visibility (want public, unlisted, private, or direct)"}
		}
		req.Visibility = v
	}
	if v, _ := cmd.Flags().GetString("lang"); v != "" {
		req.Language = v
	}

	for i, path := range images {
		alt := ""
		if i < len(alts) {
			alt = alts[i]
		}
		if alt == "" {
			fmt.Fprintf(rt.svc.stderr(), "note: image %q has no --alt text; screen-reader users cannot read it\n", path)
		}
		id, err := rt.uploadMedia(cmd.Context(), path, alt)
		if err != nil {
			return err
		}
		req.MediaIDs = append(req.MediaIDs, id)
	}

	body, _, err := rt.postStatus(cmd.Context(), req)
	if err != nil {
		return err
	}
	status, err := decodeStatus(body)
	if err != nil {
		return err
	}
	return rt.emitJSON(createdFromStatus(status))
}

// postStatus POSTs /api/v1/statuses with a deterministic Idempotency-Key so a
// retried invocation (each heliox tool call is a fresh process) never
// double-posts: identical parameters within Mastodon's idempotency window
// return the already-created status instead of a duplicate.
func (rt *runContext) postStatus(ctx context.Context, req statusCreateRequest) ([]byte, http.Header, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: encode status: %v", err), err: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rt.baseURL()+"/api/v1/statuses", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: build request: %v", err), err: err}
	}
	httpReq.Header.Set("Authorization", "Bearer "+rt.token)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey(req))
	return rt.do(httpReq)
}

// idempotencyKey derives a stable key from the canonical status parameters, so
// the same post attempted twice carries the same key. Media ids are sorted so
// attachment order does not change the key.
func idempotencyKey(req statusCreateRequest) string {
	ids := append([]string(nil), req.MediaIDs...)
	sort.Strings(ids)
	canonical := strings.Join([]string{
		req.Status, req.InReplyToID, req.SpoilerText, req.Visibility, req.Language,
		strings.Join(ids, ","),
	}, "\x00")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// uploadMedia POSTs a file to /api/v2/media and returns its id. v2 may reply
// 202 Accepted while the server processes the media asynchronously; in that
// case the id is usable for attachment but we poll GET /api/v1/media/:id until
// the URL is ready (bounded) so the status create does not race processing.
func (rt *runContext) uploadMedia(ctx context.Context, path, description string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", &usageError{msg: fmt.Sprintf("open image %q: %v", path, err)}
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", baseName(path))
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("mastodon: build media upload: %v", err), err: err}
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", &apiError{msg: fmt.Sprintf("mastodon: read image %q: %v", path, err), err: err}
	}
	if description != "" {
		_ = writer.WriteField("description", description)
	}
	if err := writer.Close(); err != nil {
		return "", &apiError{msg: fmt.Sprintf("mastodon: finalize media upload: %v", err), err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rt.baseURL()+"/api/v2/media", &buf)
	if err != nil {
		return "", &apiError{msg: fmt.Sprintf("mastodon: build media request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+rt.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	body, _, err := rt.doMedia(req)
	if err != nil {
		return "", err
	}
	var media mediaResult
	if err := json.Unmarshal(body, &media); err != nil {
		return "", &apiError{msg: "mastodon: decode media upload response", err: err}
	}
	if media.ID == "" {
		return "", &apiError{msg: "mastodon: media upload returned no id"}
	}
	if media.URL == "" {
		if err := rt.waitForMedia(ctx, media.ID); err != nil {
			return "", err
		}
	}
	return media.ID, nil
}

type mediaResult struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// doMedia sends a prepared media request and returns the body; a 200 or 202 is
// success (202 = accepted-and-processing), anything else is an apiError.
func (rt *runContext) doMedia(req *http.Request) ([]byte, http.Header, error) {
	hc := rt.svc.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: media upload: %v", err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &apiError{msg: fmt.Sprintf("mastodon: read media response: %v", err), err: err}
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		return body, resp.Header, nil
	}
	provider := apiMessage(body)
	return nil, resp.Header, classifyCredentialError(resp.StatusCode, &apiError{
		msg:           fmt.Sprintf("mastodon media API error (HTTP %d): %s", resp.StatusCode, provider),
		status:        resp.StatusCode,
		providerError: provider,
	})
}

// waitForMedia polls GET /api/v1/media/:id until the attachment has a URL
// (processing complete) or the bounded attempt cap is reached.
func (rt *runContext) waitForMedia(ctx context.Context, id string) error {
	for attempt := 0; attempt < mediaPollMaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return &apiError{msg: fmt.Sprintf("mastodon: wait for media %s: %v", id, ctx.Err()), err: ctx.Err()}
		case <-time.After(mediaPollInterval):
		}
		body, _, err := rt.call(ctx, http.MethodGet, "/api/v1/media/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		var media mediaResult
		if json.Unmarshal(body, &media) == nil && media.URL != "" {
			return nil
		}
	}
	return &apiError{msg: fmt.Sprintf("mastodon: media %s did not finish processing after %d polls", id, mediaPollMaxAttempts)}
}

func (rt *runContext) newPostDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete",
		Short:       "Delete a status",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, _ := cmd.Flags().GetString("id")
			if id == "" {
				return &usageError{msg: "post delete requires --id"}
			}
			body, _, err := rt.call(cmd.Context(), http.MethodDelete, "/api/v1/statuses/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			status, err := decodeStatus(body)
			if err != nil {
				return err
			}
			return rt.emitJSON(createdFromStatus(status))
		},
	}
	cmd.Flags().String("id", "", "status id (required)")
	return cmd
}

func (rt *runContext) newPostGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Read a status and its thread context",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, _ := cmd.Flags().GetString("id")
			if id == "" {
				return &usageError{msg: "post get requires --id"}
			}
			esc := url.PathEscape(id)
			statusBody, _, err := rt.call(cmd.Context(), http.MethodGet, "/api/v1/statuses/"+esc, nil, nil)
			if err != nil {
				return err
			}
			status, err := decodeStatus(statusBody)
			if err != nil {
				return err
			}
			out := map[string]any{"status": summarizeStatus(status)}

			// Best-effort thread context: ancestors + descendants. A context
			// failure never fails the primary read.
			if ctxBody, _, ctxErr := rt.call(cmd.Context(), http.MethodGet, "/api/v1/statuses/"+esc+"/context", nil, nil); ctxErr == nil {
				var raw struct {
					Ancestors   []rawStatus `json:"ancestors"`
					Descendants []rawStatus `json:"descendants"`
				}
				if json.Unmarshal(ctxBody, &raw) == nil {
					out["ancestors"] = summarizeAll(raw.Ancestors)
					out["descendants"] = summarizeAll(raw.Descendants)
				}
			}
			return rt.emitJSON(out)
		},
	}
	cmd.Flags().String("id", "", "status id (required)")
	return cmd
}

func summarizeAll(list []rawStatus) []statusSummary {
	out := make([]statusSummary, 0, len(list))
	for _, s := range list {
		out = append(out, summarizeStatus(s))
	}
	return out
}

// baseName returns the final path segment for the multipart filename.
func baseName(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		return path[i+1:]
	}
	return path
}
