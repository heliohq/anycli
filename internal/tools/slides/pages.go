package slides

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func (s *Service) newPagesGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <presentation-id-or-url> <page-object-id>",
		Short: "Show one page's full element tree (locate element object ids for edits)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid := extractPresentationID(args[0])
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/presentations/"+pid+"/pages/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var pg page
			if err := json.Unmarshal(body, &pg); err != nil {
				return fmt.Errorf("slides: decode page: %w", err)
			}
			fmt.Fprintf(s.stdout(), "page=%s\n", pg.ObjectID)
			writeElementsOutline(s.stdout(), pg.PageElements, "  ")
			return nil
		},
	}
	return cmd
}

// savedThumbnail is the --json contract for a rendered page thumbnail.
type savedThumbnail struct {
	PageObjectID string `json:"pageObjectId"`
	Path         string `json:"path"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Size         int    `json:"size"`
}

func (s *Service) newPagesThumbnailCmd(token string) *cobra.Command {
	var saveDir, size string
	cmd := &cobra.Command{
		Use:   "thumbnail <presentation-id-or-url> <page-object-id>",
		Short: "Render a page to a PNG on disk (getThumbnail contentUrl is short-lived, so download it here)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch size {
			case "LARGE", "MEDIUM", "SMALL":
			default:
				return fmt.Errorf("slides: --size must be LARGE, MEDIUM, or SMALL, got %q", size)
			}
			pid := extractPresentationID(args[0])
			pageID := args[1]
			q := url.Values{}
			q.Set("thumbnailProperties.thumbnailSize", size)
			q.Set("thumbnailProperties.mimeType", "PNG")
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/presentations/"+pid+"/pages/"+url.PathEscape(pageID)+"/thumbnail", q, nil)
			if err != nil {
				return err
			}
			var thumb struct {
				ContentURL string `json:"contentUrl"`
				Width      int    `json:"width"`
				Height     int    `json:"height"`
			}
			if err := json.Unmarshal(body, &thumb); err != nil {
				return fmt.Errorf("slides: decode thumbnail: %w", err)
			}
			if thumb.ContentURL == "" {
				return fmt.Errorf("slides: thumbnail response had no contentUrl")
			}
			if err := os.MkdirAll(saveDir, 0o755); err != nil {
				return fmt.Errorf("slides: create save dir: %w", err)
			}
			path := filepath.Join(saveDir, safeThumbName(pageID))
			n, err := s.downloadThumbnail(cmd.Context(), thumb.ContentURL, path)
			if err != nil {
				return err
			}
			saved := savedThumbnail{PageObjectID: pageID, Path: path, Width: thumb.Width, Height: thumb.Height, Size: n}
			if jsonOut(cmd) {
				return s.emitJSON(saved)
			}
			fmt.Fprintf(s.stdout(), "saved %s (%dx%d, %d bytes)\n", saved.Path, saved.Width, saved.Height, saved.Size)
			return nil
		},
	}
	cmd.Flags().StringVar(&saveDir, "save", ".", "directory to save the PNG into")
	cmd.Flags().StringVar(&size, "size", "LARGE", "relative thumbnail size: LARGE (largest), MEDIUM, or SMALL (smallest); exact pixels depend on slide aspect ratio")
	return cmd
}

// downloadThumbnail fetches the short-lived contentUrl (a signed Google URL,
// no Bearer needed) and writes the PNG to path.
func (s *Service) downloadThumbnail(ctx context.Context, contentURL, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return 0, fmt.Errorf("slides: build thumbnail download: %w", err)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("slides: download thumbnail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("slides: download thumbnail: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("slides: read thumbnail body: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, fmt.Errorf("slides: write thumbnail: %w", err)
	}
	return len(data), nil
}

// safeThumbName derives a filesystem-safe PNG name from a page object id.
func safeThumbName(pageID string) string {
	name := filepath.Base(pageID)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "page"
	}
	return name + ".png"
}
