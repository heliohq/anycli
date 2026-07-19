package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const singlePartLimit = 20 * 1024 * 1024

func (s *Service) newFileUploadCmd(token string) *cobra.Command {
	var name, contentType string
	cmd := &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload a local file to Notion-managed storage",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return &usageError{msg: fmt.Sprintf("file upload: read %s: %v", path, err)}
			}
			if len(data) > singlePartLimit {
				return &usageError{msg: fmt.Sprintf("file upload: %s is %d bytes; single-part uploads are limited to 20 MB", path, len(data))}
			}
			filename := uploadName(name, path)
			ct := contentType
			if strings.TrimSpace(ct) == "" {
				ct = sourceMime(path)
			}
			createBody, err := s.createFileUpload(cmd.Context(), token, filename, ct)
			if err != nil {
				return err
			}
			id, err := fileUploadID(createBody)
			if err != nil {
				return err
			}
			sendBody, err := s.sendFileUpload(cmd.Context(), token, id, path, filename, ct, data, nil)
			if err != nil {
				return &apiError{msg: fmt.Sprintf("file upload: created file_upload %s but sending bytes failed: %v", id, err), err: err}
			}
			jsonMode, _ := cmd.Flags().GetBool("json")
			if jsonMode {
				return s.emitJSON(sendBody)
			}
			fmt.Fprintln(s.stdout(), id)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "filename to store in Notion (default: basename of path)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "MIME type (default: inferred from extension)")
	return cmd
}

func (s *Service) newFileAttachCmd(token string) *cobra.Command {
	var property, uploadID, externalURL, name string
	cmd := &cobra.Command{
		Use:   "attach <page-id>",
		Short: "Attach an uploaded or external file to a page files property",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := resolveID(args[0])
			if err != nil {
				return err
			}
			property = strings.TrimSpace(property)
			if property == "" {
				return &usageError{msg: "file attach requires --property"}
			}
			hasUpload := strings.TrimSpace(uploadID) != ""
			hasExternal := strings.TrimSpace(externalURL) != ""
			if hasUpload == hasExternal {
				return &usageError{msg: "file attach requires exactly one of --upload-id or --external-url"}
			}
			fileObj := map[string]any{}
			if strings.TrimSpace(name) != "" {
				fileObj["name"] = name
			}
			if hasUpload {
				id, err := resolveID(uploadID)
				if err != nil {
					return err
				}
				fileObj["type"] = "file_upload"
				fileObj["file_upload"] = map[string]any{"id": id}
			} else {
				if !isURL(externalURL) {
					return &usageError{msg: "--external-url must be an http(s) URL"}
				}
				if strings.TrimSpace(name) == "" {
					fileObj["name"] = filepath.Base(externalURL)
				}
				fileObj["type"] = "external"
				fileObj["external"] = map[string]any{"url": externalURL}
			}
			payload := map[string]any{"properties": map[string]any{property: map[string]any{"files": []any{fileObj}}}}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, "/pages/"+url.PathEscape(pageID), payload)
			if err != nil {
				return err
			}
			jsonMode, _ := cmd.Flags().GetBool("json")
			if jsonMode {
				return s.emitJSON(body)
			}
			fmt.Fprintln(s.stdout(), pageID)
			return nil
		},
	}
	cmd.Flags().StringVar(&property, "property", "", "files property name to replace")
	cmd.Flags().StringVar(&uploadID, "upload-id", "", "Notion file_upload id")
	cmd.Flags().StringVar(&externalURL, "external-url", "", "external file URL")
	cmd.Flags().StringVar(&name, "name", "", "display filename")
	return cmd
}

func (s *Service) createFileUpload(ctx context.Context, token, filename, contentType string) ([]byte, error) {
	payload := map[string]any{"mode": "single_part", "filename": filename, "content_type": contentType}
	return s.call(ctx, token, http.MethodPost, "/file_uploads", payload)
}

func (s *Service) sendFileUpload(ctx context.Context, token, id, path, filename, contentType string, data []byte, fields map[string]string) ([]byte, error) {
	return s.callMultipart(ctx, token, "/file_uploads/"+url.PathEscape(id)+"/send", fields, "file", filename, contentType, data)
}

func fileUploadID(body []byte) (string, error) {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return "", &apiError{msg: fmt.Sprintf("file upload: decode create response: %v", err), err: err}
	}
	if obj.ID == "" {
		return "", &apiError{msg: "file upload: create response did not include id"}
	}
	return obj.ID, nil
}

func (s *Service) callMultipart(ctx context.Context, token, path string, fields map[string]string, fileField, fileName, contentType string, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			_ = mw.Close()
			return nil, &apiError{msg: fmt.Sprintf("notion: build multipart field: %v", err), err: err}
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, fileField, fileName))
	if strings.TrimSpace(contentType) != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := mw.CreatePart(partHeader)
	if err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("notion: build multipart file: %v", err), err: err}
	}
	if _, err := part.Write(data); err != nil {
		_ = mw.Close()
		return nil, &apiError{msg: fmt.Sprintf("notion: write multipart file: %v", err), err: err}
	}
	if err := mw.Close(); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: close multipart: %v", err), err: err}
	}
	return s.callRaw(ctx, token, http.MethodPost, path, buf.Bytes(), map[string]string{"Content-Type": mw.FormDataContentType()})
}

func sourceMime(path string) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func uploadName(name, path string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return filepath.Base(path)
}
