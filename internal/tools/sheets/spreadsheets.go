package sheets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

func (s *Service) newSpreadsheetsGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Show a spreadsheet's title and tab list (no cell data) — run this before touching any tab",
		Long: "Returns the title, the spreadsheet URL and one entry per tab — title, gid,\n" +
			"index and grid size — with a field mask that excludes cell data, so it\n" +
			"stays cheap on a huge file. `rowCount` and `columnCount` are the allocated\n" +
			"grid CAPACITY, which a new sheet sets to 1000 rows whatever it contains;\n" +
			"treat them as an upper bound for chunked reads, never as a row count. How\n" +
			"much data actually exists comes from the length of what `values get`\n" +
			"returns.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			body, err := s.callMeta(cmd.Context(), token, id)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var meta spreadsheetMeta
			if err := json.Unmarshal(body, &meta); err != nil {
				return fmt.Errorf("sheets: decode spreadsheet metadata: %w", err)
			}
			fmt.Fprintf(s.stdout(), "%s\nid:  %s\nurl: %s\ntabs:\n", meta.Properties.Title, meta.SpreadsheetID, meta.SpreadsheetURL)
			for _, sh := range meta.Sheets {
				p := sh.Properties
				fmt.Fprintf(s.stdout(), "  %s\tgid=%d\t%dx%d (grid capacity)\n",
					p.Title, p.SheetID, p.GridProperties.RowCount, p.GridProperties.ColumnCount)
			}
			return nil
		},
	}
	return cmd
}

func (s *Service) newSpreadsheetsCreateCmd(token string) *cobra.Command {
	var title string
	var tabs []string
	cmd := &cobra.Command{
		Use:   "create --title T [--tab N]...",
		Short: "Create a spreadsheet in the user's My Drive root (spreadsheets.create)",
		Long: "The new file lands in the connected account's My Drive root and there is no\n" +
			"destination flag — filing it into a folder is a Drive operation, not a\n" +
			"Sheets one. --tab is repeatable and names the initial tabs, replacing the\n" +
			"single default sheet. The response carries the new id and URL; keep both,\n" +
			"because nothing in this tool can find the file again by name.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return fmt.Errorf("sheets: --title is required")
			}
			payload := map[string]any{"properties": map[string]any{"title": title}}
			if len(tabs) > 0 {
				sheets := make([]any, len(tabs))
				for i, name := range tabs {
					sheets[i] = map[string]any{"properties": map[string]any{"title": name}}
				}
				payload["sheets"] = sheets
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/spreadsheets", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				SpreadsheetID  string `json:"spreadsheetId"`
				SpreadsheetURL string `json:"spreadsheetUrl"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode create response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "created %q\nid:  %s\nurl: %s\n", title, resp.SpreadsheetID, resp.SpreadsheetURL)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "spreadsheet title")
	cmd.Flags().StringArrayVar(&tabs, "tab", nil, "initial tab title (repeatable)")
	return cmd
}

func (s *Service) newSpreadsheetsBatchUpdateCmd(token string) *cobra.Command {
	var requestFile string
	cmd := &cobra.Command{
		Use:   "batch-update <id> --request-file <json>",
		Short: "Apply a raw spreadsheets.batchUpdate request (escape hatch for formatting, sorting, charts, ...)",
		Long: "--request-file is a path to either a full `{\"requests\":[...]}` body or a\n" +
			"bare `[...]` array, handed to the Sheets API untouched. Everything the\n" +
			"safe verbs do not cover lives here: cell formatting, conditional\n" +
			"formatting, sorting, frozen rows, data validation, charts, pivot tables,\n" +
			"and row or column deletion that actually closes the gap. Requests apply in\n" +
			"order inside one atomic call, so a single invalid request rejects the\n" +
			"whole batch and changes nothing. Requests address tabs by numeric sheetId\n" +
			"(gid), never by title — read the gids from `spreadsheets get`.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if requestFile == "" {
				return fmt.Errorf("sheets: --request-file is required")
			}
			payload, err := loadBatchUpdatePayload(requestFile)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/spreadsheets/"+url.PathEscape(id)+":batchUpdate", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Replies []json.RawMessage `json:"replies"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode batchUpdate response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "batch update applied (%d replies)\n", len(resp.Replies))
			return nil
		},
	}
	cmd.Flags().StringVar(&requestFile, "request-file", "", "path to a JSON file: either a full {\"requests\":[...]} body or a bare [...] array of requests")
	return cmd
}

// loadBatchUpdatePayload reads the raw batchUpdate request file. It accepts
// either a full request object (passed through verbatim) or a bare array of
// requests (wrapped into {"requests": [...]}).
func loadBatchUpdatePayload(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sheets: read request file: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("sheets: request file is empty")
	}
	if trimmed[0] == '[' {
		var reqs []json.RawMessage
		if err := json.Unmarshal(trimmed, &reqs); err != nil {
			return nil, fmt.Errorf("sheets: request file must be a JSON array of requests: %w", err)
		}
		return map[string]any{"requests": reqs}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("sheets: request file must be a JSON object or array: %w", err)
	}
	return obj, nil
}
