package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// sheetProps is one tab's properties (spreadsheets.sheets[].properties),
// limited to the fields metaFields requests.
type sheetProps struct {
	SheetID        int64  `json:"sheetId"`
	Title          string `json:"title"`
	Index          int64  `json:"index"`
	GridProperties struct {
		RowCount    int64 `json:"rowCount"`
		ColumnCount int64 `json:"columnCount"`
	} `json:"gridProperties"`
}

// spreadsheetMeta is the trimmed spreadsheets.get response (no grid data).
type spreadsheetMeta struct {
	SpreadsheetID  string `json:"spreadsheetId"`
	SpreadsheetURL string `json:"spreadsheetUrl"`
	Properties     struct {
		Title string `json:"title"`
	} `json:"properties"`
	Sheets []struct {
		Properties sheetProps `json:"properties"`
	} `json:"sheets"`
}

// parseSpreadsheetID accepts either a bare spreadsheetId or a full
// docs.google.com URL and returns the id. Sheets URLs look like
// https://docs.google.com/spreadsheets/d/<ID>/edit#gid=0 — the id is the
// path segment after "/d/".
func parseSpreadsheetID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("sheets: empty spreadsheet id")
	}
	if !strings.Contains(s, "/") {
		return s, nil
	}
	if i := strings.Index(s, "/d/"); i >= 0 {
		rest := s[i+len("/d/"):]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		if rest != "" {
			return rest, nil
		}
	}
	return "", fmt.Errorf("sheets: cannot parse spreadsheet id from %q", raw)
}

// fetchMeta loads the trimmed spreadsheet metadata (title + tab properties).
func (s *Service) fetchMeta(ctx context.Context, token, id string) (*spreadsheetMeta, error) {
	body, err := s.callMeta(ctx, token, id)
	if err != nil {
		return nil, err
	}
	var meta spreadsheetMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("sheets: decode spreadsheet metadata: %w", err)
	}
	return &meta, nil
}

// callMeta issues spreadsheets.get with the metaFields mask and returns the raw
// body so callers can either parse it or pass it straight through to --json.
func (s *Service) callMeta(ctx context.Context, token, id string) ([]byte, error) {
	q := url.Values{}
	q.Set("fields", metaFields)
	return s.call(ctx, token, http.MethodGet, "/spreadsheets/"+url.PathEscape(id), q, nil)
}

// resolveTabID maps a --tab value (title or numeric gid) to its sheetId. Title
// matches win first (human-natural); a title shared by several tabs is
// ambiguous and forces the caller to pass the gid instead.
func resolveTabID(meta *spreadsheetMeta, tab string) (int64, error) {
	tab = strings.TrimSpace(tab)
	if tab == "" {
		return 0, fmt.Errorf("sheets: --tab is required (tab title or numeric gid)")
	}
	var matches []int64
	for _, sh := range meta.Sheets {
		if sh.Properties.Title == tab {
			matches = append(matches, sh.Properties.SheetID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("sheets: %d tabs are titled %q — pass the numeric gid instead", len(matches), tab)
	}
	if gid, err := strconv.ParseInt(tab, 10, 64); err == nil {
		for _, sh := range meta.Sheets {
			if sh.Properties.SheetID == gid {
				return gid, nil
			}
		}
		return 0, fmt.Errorf("sheets: no tab with gid %d", gid)
	}
	return 0, fmt.Errorf("sheets: no tab named %q", tab)
}
