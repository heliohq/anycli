package microsoftonedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

// simpleUploadMaxBytes is the size ceiling for a single PUT ...:/content
// upload; larger files switch to a chunked upload session. Graph's own
// guidance is <4MB for the simple path.
const simpleUploadMaxBytes = 4 << 20

// uploadChunkSize is one upload-session chunk. Graph requires session chunk
// sizes to be a multiple of 320 KiB.
const uploadChunkSize = 320 * 1024 * 10

func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query string
	var max int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search the drive (Graph drive search, query passed through verbatim)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return fmt.Errorf("microsoft-onedrive: --query is required")
			}
			body, err := s.fetchCollection(cmd, token, searchResource(query), max, "")
			if err != nil {
				return err
			}
			return s.renderItemList(cmd, body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().IntVar(&max, "max", 20, "max results to return")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

// savedFile is one downloaded item (--json contract).
type savedFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int    `json:"size"`
}

func (s *Service) newDownloadCmd(token string) *cobra.Command {
	var path, saveDir string
	cmd := &cobra.Command{
		Use:   "download [item-id]",
		Short: "Download an item's contents into a local directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := firstArg(args)
			resource, err := itemResource(id, path)
			if err != nil {
				return err
			}
			// Resolve name (and give the human summary a size) from metadata,
			// then stream the content: an id-addressed item carries no local
			// filename otherwise.
			metaBody, err := s.call(cmd.Context(), token, http.MethodGet, resource, nil, nil)
			if err != nil {
				return err
			}
			var meta driveItem
			if err := json.Unmarshal(metaBody, &meta); err != nil {
				return fmt.Errorf("microsoft-onedrive: decode item: %w", err)
			}
			data, err := s.getContent(cmd.Context(), token, resource+"/content")
			if err != nil {
				return err
			}
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				return fmt.Errorf("microsoft-onedrive: create save dir: %w", err)
			}
			name := downloadName(meta.Name)
			dest := filepath.Join(saveDir, name)
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return fmt.Errorf("microsoft-onedrive: write %s: %w", name, err)
			}
			saved := savedFile{ID: meta.ID, Name: name, Path: dest, Size: len(data)}
			if jsonOut(cmd) {
				return s.emitJSON(saved)
			}
			fmt.Fprintf(s.stdout(), "saved %s (%d bytes)\n", saved.Path, saved.Size)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "root-relative item path instead of an id")
	cmd.Flags().StringVar(&saveDir, "save", ".", "directory to save the download into")
	return cmd
}

func (s *Service) newUploadCmd(token string) *cobra.Command {
	var toPath, parentID, name string
	cmd := &cobra.Command{
		Use:   "upload <local-path>",
		Short: "Upload a local file (small files direct, large files via an upload session)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local := args[0]
			data, err := os.ReadFile(local)
			if err != nil {
				return fmt.Errorf("microsoft-onedrive: read %s: %w", local, err)
			}
			targetName := name
			if targetName == "" {
				targetName = filepath.Base(local)
			}
			resource, err := uploadTargetResource(parentID, toPath, targetName)
			if err != nil {
				return err
			}
			var body []byte
			if len(data) <= simpleUploadMaxBytes {
				body, err = s.uploadSimple(cmd.Context(), token, resource, data)
			} else {
				body, err = s.uploadLarge(cmd.Context(), token, resource, data)
			}
			if err != nil {
				return err
			}
			return s.renderItemResult(cmd, body, "uploaded")
		},
	}
	cmd.Flags().StringVar(&toPath, "to", "", "destination folder path (root-relative)")
	cmd.Flags().StringVar(&parentID, "parent", "", "destination folder item id")
	cmd.Flags().StringVar(&name, "name", "", "name to store the file as (defaults to the local basename)")
	cmd.MarkFlagsMutuallyExclusive("to", "parent")
	return cmd
}

// uploadSimple PUTs the whole file to ...:/content in one request.
func (s *Service) uploadSimple(ctx context.Context, token, resource string, data []byte) ([]byte, error) {
	path := resource + "/content"
	endpoint := s.base() + path
	return s.callEndpoint(ctx, token, http.MethodPut, path, endpoint, "application/octet-stream", data)
}

// uploadLarge creates an upload session for resource and streams the file in
// fixed-size chunks, returning the final DriveItem response.
func (s *Service) uploadLarge(ctx context.Context, token, resource string, data []byte) ([]byte, error) {
	sessBody, err := s.call(ctx, token, http.MethodPost, resource+"/createUploadSession", nil,
		map[string]any{"item": map[string]any{"@microsoft.graph.conflictBehavior": "replace"}})
	if err != nil {
		return nil, err
	}
	var sess struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.Unmarshal(sessBody, &sess); err != nil {
		return nil, fmt.Errorf("microsoft-onedrive: decode upload session: %w", err)
	}
	if sess.UploadURL == "" {
		return nil, fmt.Errorf("microsoft-onedrive: upload session returned no uploadUrl")
	}
	total := len(data)
	var final []byte
	for start := 0; start < total; start += uploadChunkSize {
		end := start + uploadChunkSize
		if end > total {
			end = total
		}
		status, body, err := s.uploadChunk(ctx, sess.UploadURL, data[start:end], start, end, total)
		if err != nil {
			return nil, err
		}
		if status < 200 || status > 299 {
			return nil, s.apiError(status, "upload session", body)
		}
		final = body
	}
	if final == nil {
		return nil, fmt.Errorf("microsoft-onedrive: upload produced no final item")
	}
	return final, nil
}

// uploadChunk PUTs one byte range [start,end) of total to the pre-authenticated
// session uploadUrl. The uploadUrl carries its own auth, so no Bearer header is
// sent (Graph rejects one on some session URLs).
func (s *Service) uploadChunk(ctx context.Context, uploadURL string, chunk []byte, start, end, total int) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
	if err != nil {
		return 0, nil, fmt.Errorf("microsoft-onedrive: build upload chunk request: %w", err)
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(chunk)))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, total))
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("microsoft-onedrive: upload chunk: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("microsoft-onedrive: read upload chunk response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// downloadName picks a safe on-disk name for a downloaded item.
func downloadName(name string) string {
	base := filepath.Base(name)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "download"
	}
	return base
}
