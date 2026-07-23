package amplitude

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

// exportReceipt is emitted after the export archive is streamed to a file. The
// raw zip (up to 4GB) never touches stdout.
type exportReceipt struct {
	Saved string `json:"saved"`
	Bytes int64  `json:"bytes"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// newExportCmd — GET /api/2/export. Raw event export (zipped JSONL) for a
// start/end HOUR range (YYYYMMDDTHH). The response is bytes, so it is streamed
// to a file and a JSON receipt is emitted; a non-2xx (e.g. Amplitude's 4GB /
// 365-day limit) surfaces as a typed apiError.
func (s *Service) newExportCmd(authHeader string) *cobra.Command {
	var start, end, output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Raw event export as a zip archive (GET /api/2/export)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if start == "" || end == "" {
				return &usageError{msg: "--start and --end are required (YYYYMMDDTHH hour range)"}
			}
			inv, err := s.resolve(cmd, authHeader)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("start", start)
			q.Set("end", end)

			written, path, err := s.download(cmd, inv, "/api/2/export", q, output)
			if err != nil {
				return err
			}
			return s.emitValue(exportReceipt{Saved: path, Bytes: written, Start: start, End: end})
		},
	}
	f := cmd.Flags()
	f.StringVar(&start, "start", "", "first hour YYYYMMDDTHH (required)")
	f.StringVar(&end, "end", "", "last hour YYYYMMDDTHH (required)")
	f.StringVar(&output, "output", "", "file path for the zip archive (default: a temp file)")
	return cmd
}

// download performs a GET and streams a 2xx body to a file (never to stdout),
// returning the bytes written and the file path. A non-2xx body is read (small
// error payload) and returned as a classified apiError. When output is empty a
// temp file is created.
func (s *Service) download(cmd *cobra.Command, inv *invocation, path string, query url.Values, output string) (int64, string, error) {
	req, err := inv.newRequest(cmd.Context(), http.MethodGet, path, query)
	if err != nil {
		return 0, "", err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, "", &apiError{msg: fmt.Sprintf("amplitude: GET %s: %v", path, err), err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return 0, "", newAPIError(inv, resp.StatusCode, body)
	}

	f, path, err := createOutput(output)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, "", &apiError{msg: fmt.Sprintf("amplitude: write export archive: %v", err), err: err}
	}
	if err := f.Close(); err != nil {
		return 0, "", &apiError{msg: fmt.Sprintf("amplitude: finalize export archive: %v", err), err: err}
	}
	return written, path, nil
}

// createOutput opens the requested output path, or a temp file when empty.
func createOutput(output string) (*os.File, string, error) {
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return nil, "", &usageError{msg: fmt.Sprintf("cannot create --output %s: %v", output, err)}
		}
		return f, output, nil
	}
	f, err := os.CreateTemp("", "amplitude-export-*.zip")
	if err != nil {
		return nil, "", &apiError{msg: fmt.Sprintf("amplitude: create temp export file: %v", err), err: err}
	}
	return f, f.Name(), nil
}
