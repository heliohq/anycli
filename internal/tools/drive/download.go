package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// exportFormats maps the --format flag to the Workspace export MIME type and
// the on-disk extension (design 303; strings verified against Drive's export
// format reference).
var exportFormats = map[string]struct {
	mime string
	ext  string
}{
	"pdf":  {"application/pdf", ".pdf"},
	"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
	"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
	"pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
	"csv":  {"text/csv", ".csv"},
	"txt":  {"text/plain", ".txt"},
}

// savedFile is one file written to disk (--json contract).
type savedFile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Path   string `json:"path"`
	Size   int    `json:"size"`
	Format string `json:"format,omitempty"`
}

func (s *Service) newFilesDownloadCmd(token string) *cobra.Command {
	var saveDir string
	cmd := &cobra.Command{
		Use:         "download <file-id>",
		Short:       "Download a binary file's content (files.get alt=media). For Workspace docs use `export`.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			name, _, err := s.fileNameType(cmd.Context(), token, id)
			if err != nil {
				return err
			}
			q := driveParams()
			q.Set("alt", "media")
			data, err := s.callRaw(cmd.Context(), token, http.MethodGet, s.base()+"/files/"+url.PathEscape(id), "/files/"+id, q)
			if err != nil {
				return err
			}
			saved, err := s.writeDownload(saveDir, safeName(name, id), id, data, "")
			if err != nil {
				return err
			}
			return s.reportSaved(cmd, saved)
		},
	}
	cmd.Flags().StringVar(&saveDir, "save", ".", "directory to save the file into")
	return cmd
}

func (s *Service) newFilesExportCmd(token string) *cobra.Command {
	var format, saveDir string
	cmd := &cobra.Command{
		Use:         "export <file-id> --format pdf|docx|xlsx|pptx|csv|txt",
		Short:       "Export a Google Workspace doc (Docs/Sheets/Slides) to a downloadable format. API caps exports at 10MB.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, ok := exportFormats[strings.ToLower(format)]
			if !ok {
				return fmt.Errorf("drive: unsupported --format %q (want pdf, docx, xlsx, pptx, csv, or txt)", format)
			}
			id := args[0]
			name, _, err := s.fileNameType(cmd.Context(), token, id)
			if err != nil {
				return err
			}
			// files.export defines only fileId and mimeType (Drive v3
			// discovery doc); it does NOT accept supportsAllDrives, so build
			// the query directly instead of via driveParams().
			q := url.Values{}
			q.Set("mimeType", spec.mime)
			data, err := s.callRaw(cmd.Context(), token, http.MethodGet, s.base()+"/files/"+url.PathEscape(id)+"/export", "/files/"+id+"/export", q)
			if err != nil {
				return err
			}
			saved, err := s.writeDownload(saveDir, exportName(name, id, spec.ext), id, data, strings.ToLower(format))
			if err != nil {
				return err
			}
			return s.reportSaved(cmd, saved)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "export format: pdf, docx, xlsx, pptx, csv, or txt")
	cmd.Flags().StringVar(&saveDir, "save", ".", "directory to save the export into")
	_ = cmd.MarkFlagRequired("format")
	return cmd
}

// fileNameType fetches just the name and mimeType of a file (used to name a
// download/export target).
func (s *Service) fileNameType(ctx context.Context, token, id string) (name, mimeType string, err error) {
	body, err := s.getFileMeta(ctx, token, id, "id,name,mimeType")
	if err != nil {
		return "", "", err
	}
	var f struct {
		Name     string `json:"name"`
		MimeType string `json:"mimeType"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return "", "", fmt.Errorf("drive: decode file metadata: %w", err)
	}
	return f.Name, f.MimeType, nil
}

// writeDownload writes bytes to saveDir/name, creating the directory.
func (s *Service) writeDownload(saveDir, name, id string, data []byte, format string) (savedFile, error) {
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return savedFile{}, fmt.Errorf("drive: create save dir: %w", err)
	}
	path := filepath.Join(saveDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return savedFile{}, fmt.Errorf("drive: write %s: %w", name, err)
	}
	return savedFile{ID: id, Name: name, Path: path, Size: len(data), Format: format}, nil
}

func (s *Service) reportSaved(cmd *cobra.Command, saved savedFile) error {
	if jsonOut(cmd) {
		return s.emitJSON(saved)
	}
	fmt.Fprintf(s.stdout(), "saved %s (%d bytes)\n", saved.Path, saved.Size)
	return nil
}

// safeName picks a safe on-disk name from a Drive file name, falling back to
// the id when empty or path-like.
func safeName(name, id string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "file-" + id
	}
	return base
}

// exportName appends the export extension unless the file already carries it.
func exportName(name, id, ext string) string {
	base := safeName(name, id)
	if strings.EqualFold(filepath.Ext(base), ext) {
		return base
	}
	return base + ext
}
