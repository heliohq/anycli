package adobesign

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// singlePartLimit is Adobe's practical single-request transient-document size.
const singlePartLimit = 100 * 1024 * 1024

func (s *Service) newDocumentUploadCmd(token, baseURI string) *cobra.Command {
	var name, contentType string
	cmd := &cobra.Command{
		Use:         "upload <path>",
		Short:       "Upload a local file to transient storage (returns a transient document id)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := s.uploadTransient(cmd.Context(), token, baseURI, args[0], name, contentType)
			if err != nil {
				return err
			}
			if jsonMode(cmd) {
				return s.emitJSON(map[string]string{"transient_document_id": id})
			}
			fmt.Fprintln(s.stdout(), id)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filename to store (default: basename of path)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "MIME type (default: inferred from extension)")
	return cmd
}

// uploadTransient POSTs a file to /transientDocuments and returns the
// transientDocumentId. Shared by `document upload` and the file-based
// `agreement send` two-step.
func (s *Service) uploadTransient(ctx context.Context, token, baseURI, path, name, contentType string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &usageError{msg: fmt.Sprintf("document upload: read %s: %v", path, err)}
	}
	if len(data) > singlePartLimit {
		return "", &usageError{msg: fmt.Sprintf("document upload: %s is %d bytes; single-request uploads are limited to 100 MB", path, len(data))}
	}
	filename := name
	if strings.TrimSpace(filename) == "" {
		filename = filepath.Base(path)
	}
	ct := contentType
	if strings.TrimSpace(ct) == "" {
		ct = sourceMime(path)
	}
	body, err := s.callMultipart(ctx, token, baseURI, "/transientDocuments", filename, ct, data, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		TransientDocumentID string `json:"transientDocumentId"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", &apiError{msg: fmt.Sprintf("document upload: decode response: %v", err), err: err}
	}
	if out.TransientDocumentID == "" {
		return "", &apiError{msg: "document upload: response did not include transientDocumentId"}
	}
	return out.TransientDocumentID, nil
}
