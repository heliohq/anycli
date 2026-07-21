package customerio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

func (s *Service) newExportDeliveriesCmd(key string) *cobra.Command {
	var newsletter, campaign, action, start, end, metric string
	cmd := &cobra.Command{
		Use:   "deliveries",
		Short: "Start a deliveries export (POST /v1/exports/deliveries)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set := 0
			for _, v := range []string{newsletter, campaign, action} {
				if v != "" {
					set++
				}
			}
			if set != 1 {
				return &usageError{msg: "exactly one of --newsletter, --campaign, or --action is required"}
			}
			body := map[string]any{}
			setIfPresent(body, "newsletter_id", newsletter)
			setIfPresent(body, "campaign_id", campaign)
			setIfPresent(body, "action_id", action)
			setIfPresent(body, "start", start)
			setIfPresent(body, "end", end)
			setIfPresent(body, "metric", metric)
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/exports/deliveries", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&newsletter, "newsletter", "", "newsletter id to export deliveries for")
	cmd.Flags().StringVar(&campaign, "campaign", "", "campaign id to export deliveries for")
	cmd.Flags().StringVar(&action, "action", "", "action id to export deliveries for")
	cmd.Flags().StringVar(&start, "start", "", "start unix timestamp")
	cmd.Flags().StringVar(&end, "end", "", "end unix timestamp")
	cmd.Flags().StringVar(&metric, "metric", "", "metric filter (e.g. delivered, opened)")
	return cmd
}

func (s *Service) newExportPeopleCmd(key string) *cobra.Command {
	var filters string
	cmd := &cobra.Command{
		Use:   "people",
		Short: "Start a people export (POST /v1/exports/customers)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The App API requires the audience filter under `filters` (plural);
			// omitting it would otherwise export the entire workspace.
			v, err := decodeJSONFlag("filters", filters)
			if err != nil {
				return err
			}
			body := map[string]any{"filters": v}
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/exports/customers", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&filters, "filters", "", "audience filter as raw JSON (and/or/not over segment & attribute conditions); required")
	_ = cmd.MarkFlagRequired("filters")
	return cmd
}

func (s *Service) newExportListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List exports (GET /v1/exports)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/exports", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newExportGetCmd(key string) *cobra.Command {
	var id, out string
	var download bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get an export (GET /v1/exports/{id}); with --download, fetch its signed link (GET /v1/exports/{id}/download) and save the file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !download {
				resp, err := s.call(cmd, key, http.MethodGet, "/v1/exports/"+url.PathEscape(id), nil, nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			}
			if out == "" {
				return &usageError{msg: "--out is required with --download"}
			}
			// The export object itself carries no link — only a signed URL from
			// the dedicated /download endpoint (expires after 15 minutes).
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/exports/"+url.PathEscape(id)+"/download", nil, nil)
			if err != nil {
				return err
			}
			downloadURL, err := extractDownloadURL(resp)
			if err != nil {
				return err
			}
			n, err := s.downloadFile(cmd.Context(), downloadURL, out)
			if err != nil {
				return err
			}
			return s.emitValue(map[string]any{"ok": true, "path": out, "bytes": n})
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "export id")
	cmd.Flags().BoolVar(&download, "download", false, "fetch the export's signed download link and save the file")
	cmd.Flags().StringVar(&out, "out", "", "output file path (required with --download)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// extractDownloadURL reads the signed link from GET /v1/exports/{id}/download,
// whose body is {"url":"…"}. The link expires 15 minutes after it is issued; an
// empty url means the export is not ready to download yet (still processing).
func extractDownloadURL(body []byte) (string, error) {
	var env struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", &apiError{msg: fmt.Sprintf("customer-io: decode export download: %v", err), err: err}
	}
	if env.URL != "" {
		return env.URL, nil
	}
	return "", &apiError{msg: "customer-io: export has no download url yet (still processing?)"}
}

// downloadFile GETs an absolute (pre-signed) URL without the App API bearer
// header and streams the body to path. Returns the byte count written.
func (s *Service) downloadFile(ctx context.Context, rawURL, path string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, &apiError{msg: fmt.Sprintf("customer-io: build download request: %v", err), err: err}
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, &apiError{msg: fmt.Sprintf("customer-io: download: %v", err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, &apiError{msg: fmt.Sprintf("customer-io: download returned HTTP %d", resp.StatusCode), status: resp.StatusCode}
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, &apiError{msg: fmt.Sprintf("customer-io: create %s: %v", path, err), err: err}
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, &apiError{msg: fmt.Sprintf("customer-io: write %s: %v", path, err), err: err}
	}
	return n, nil
}
