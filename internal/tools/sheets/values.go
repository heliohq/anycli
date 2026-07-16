package sheets

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// valueRange mirrors the API ValueRange resource (values.get / update /
// append). majorDimension defaults to ROWS on write.
type valueRange struct {
	Range          string  `json:"range,omitempty"`
	MajorDimension string  `json:"majorDimension,omitempty"`
	Values         [][]any `json:"values,omitempty"`
}

func (s *Service) newValuesGetCmd(token string) *cobra.Command {
	var ranges []string
	var render string
	cmd := &cobra.Command{
		Use:   "get <id> --range R [--range R]...",
		Short: "Read values from one or more A1 ranges (batchGet for multiple --range)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if len(ranges) == 0 {
				return fmt.Errorf("sheets: at least one --range is required")
			}
			ro, err := renderOption(render)
			if err != nil {
				return err
			}
			if len(ranges) == 1 {
				q := url.Values{}
				if ro != "" {
					q.Set("valueRenderOption", ro)
				}
				body, err := s.call(cmd.Context(), token, http.MethodGet,
					"/spreadsheets/"+url.PathEscape(id)+"/values/"+url.PathEscape(ranges[0]), q, nil)
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				var vr valueRange
				if err := json.Unmarshal(body, &vr); err != nil {
					return fmt.Errorf("sheets: decode values: %w", err)
				}
				renderValueRange(s.stdout(), vr)
				return nil
			}
			q := url.Values{}
			for _, r := range ranges {
				q.Add("ranges", r)
			}
			if ro != "" {
				q.Set("valueRenderOption", ro)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/spreadsheets/"+url.PathEscape(id)+"/values:batchGet", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ValueRanges []valueRange `json:"valueRanges"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode batchGet: %w", err)
			}
			for i, vr := range resp.ValueRanges {
				if i > 0 {
					fmt.Fprintln(s.stdout())
				}
				renderValueRange(s.stdout(), vr)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&ranges, "range", nil, "A1 range to read (repeatable → batchGet)")
	cmd.Flags().StringVar(&render, "render", "", "value render: formula | unformatted (default: formatted)")
	return cmd
}

func (s *Service) newValuesUpdateCmd(token string) *cobra.Command {
	var rng, valuesJSON, csvFile string
	var raw bool
	cmd := &cobra.Command{
		Use:   "update <id> --range R (--values-json <json> | --csv-file <path>)",
		Short: "Overwrite an A1 range with new values (values.update)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if rng == "" {
				return fmt.Errorf("sheets: --range is required")
			}
			rows, err := loadValues(valuesJSON, csvFile)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("valueInputOption", valueInputOption(raw))
			payload := valueRange{Range: rng, MajorDimension: "ROWS", Values: rows}
			body, err := s.call(cmd.Context(), token, http.MethodPut,
				"/spreadsheets/"+url.PathEscape(id)+"/values/"+url.PathEscape(rng), q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				UpdatedRange string `json:"updatedRange"`
				UpdatedCells int64  `json:"updatedCells"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode update response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "updated %d cell(s) in %s\n", resp.UpdatedCells, resp.UpdatedRange)
			return nil
		},
	}
	addRangeValueFlags(cmd, &rng, &valuesJSON, &csvFile, &raw)
	return cmd
}

func (s *Service) newValuesAppendCmd(token string) *cobra.Command {
	var rng, valuesJSON, csvFile string
	var raw bool
	cmd := &cobra.Command{
		Use:   "append <id> --range R (--values-json <json> | --csv-file <path>)",
		Short: "Append rows after the table the range points at (values.append)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if rng == "" {
				return fmt.Errorf("sheets: --range is required")
			}
			rows, err := loadValues(valuesJSON, csvFile)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("valueInputOption", valueInputOption(raw))
			payload := valueRange{Range: rng, MajorDimension: "ROWS", Values: rows}
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/spreadsheets/"+url.PathEscape(id)+"/values/"+url.PathEscape(rng)+":append", q, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Updates struct {
					UpdatedRange string `json:"updatedRange"`
					UpdatedCells int64  `json:"updatedCells"`
				} `json:"updates"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode append response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "appended %d cell(s) at %s\n", resp.Updates.UpdatedCells, resp.Updates.UpdatedRange)
			return nil
		},
	}
	addRangeValueFlags(cmd, &rng, &valuesJSON, &csvFile, &raw)
	return cmd
}

func (s *Service) newValuesClearCmd(token string) *cobra.Command {
	var ranges []string
	cmd := &cobra.Command{
		Use:   "clear <id> --range R [--range R]...",
		Short: "Clear values from one or more A1 ranges, keeping formatting (values.clear / batchClear)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if len(ranges) == 0 {
				return fmt.Errorf("sheets: at least one --range is required")
			}
			if len(ranges) == 1 {
				body, err := s.call(cmd.Context(), token, http.MethodPost,
					"/spreadsheets/"+url.PathEscape(id)+"/values/"+url.PathEscape(ranges[0])+":clear", nil, map[string]any{})
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				var resp struct {
					ClearedRange string `json:"clearedRange"`
				}
				if err := json.Unmarshal(body, &resp); err != nil {
					return fmt.Errorf("sheets: decode clear response: %w", err)
				}
				fmt.Fprintf(s.stdout(), "cleared %s\n", resp.ClearedRange)
				return nil
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				"/spreadsheets/"+url.PathEscape(id)+"/values:batchClear", nil, map[string]any{"ranges": ranges})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				ClearedRanges []string `json:"clearedRanges"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("sheets: decode batchClear response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "cleared %d range(s): %s\n", len(resp.ClearedRanges), strings.Join(resp.ClearedRanges, ", "))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&ranges, "range", nil, "A1 range to clear (repeatable → batchClear)")
	return cmd
}

// addRangeValueFlags wires the shared write flags for update/append.
func addRangeValueFlags(cmd *cobra.Command, rng, valuesJSON, csvFile *string, raw *bool) {
	cmd.Flags().StringVar(rng, "range", "", "A1 range to write (target region)")
	cmd.Flags().StringVar(valuesJSON, "values-json", "", "row-major 2D JSON array of values, e.g. [[\"a\",1],[\"b\",2]]")
	cmd.Flags().StringVar(csvFile, "csv-file", "", "read the value grid from a CSV file")
	cmd.Flags().BoolVar(raw, "raw", false, "write values verbatim (RAW) instead of parsing them like UI input (USER_ENTERED)")
	cmd.MarkFlagsMutuallyExclusive("values-json", "csv-file")
}

// loadValues reads a row-major value grid from exactly one of --values-json or
// --csv-file.
func loadValues(valuesJSON, csvFile string) ([][]any, error) {
	switch {
	case valuesJSON != "" && csvFile != "":
		return nil, fmt.Errorf("sheets: pass only one of --values-json or --csv-file")
	case valuesJSON != "":
		var rows [][]any
		if err := json.Unmarshal([]byte(valuesJSON), &rows); err != nil {
			return nil, fmt.Errorf("sheets: --values-json must be a JSON array of rows: %w", err)
		}
		return rows, nil
	case csvFile != "":
		return readCSVGrid(csvFile)
	default:
		return nil, fmt.Errorf("sheets: provide --values-json or --csv-file")
	}
}

func readCSVGrid(path string) ([][]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sheets: open csv file: %w", err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // allow ragged rows
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("sheets: read csv file: %w", err)
	}
	rows := make([][]any, len(records))
	for i, rec := range records {
		row := make([]any, len(rec))
		for j, cell := range rec {
			row[j] = cell
		}
		rows[i] = row
	}
	return rows, nil
}

// renderOption maps the --render flag to the API ValueRenderOption; the empty
// (default) case omits the param so the API applies FORMATTED_VALUE.
func renderOption(flag string) (string, error) {
	switch flag {
	case "", "formatted":
		return "", nil
	case "formula":
		return "FORMULA", nil
	case "unformatted":
		return "UNFORMATTED_VALUE", nil
	default:
		return "", fmt.Errorf("sheets: --render must be formula or unformatted, got %q", flag)
	}
}

// valueInputOption maps --raw to the API ValueInputOption. Default is
// USER_ENTERED (formulas/dates parsed like UI input); --raw writes verbatim.
func valueInputOption(raw bool) string {
	if raw {
		return "RAW"
	}
	return "USER_ENTERED"
}

// renderValueRange prints a value range as tab-separated rows.
func renderValueRange(w io.Writer, vr valueRange) {
	if vr.Range != "" {
		fmt.Fprintf(w, "range: %s\n", vr.Range)
	}
	if len(vr.Values) == 0 {
		fmt.Fprintln(w, "(no values)")
		return
	}
	for _, row := range vr.Values {
		cells := make([]string, len(row))
		for i, c := range row {
			cells[i] = cellString(c)
		}
		fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
}

// cellString renders one cell value for the human view.
func cellString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
