package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// batchUpdateReplies is the trimmed batchUpdate response used to surface the
// new tab's gid for add/duplicate.
type batchUpdateReplies struct {
	Replies []struct {
		AddSheet *struct {
			Properties sheetProps `json:"properties"`
		} `json:"addSheet"`
		DuplicateSheet *struct {
			Properties sheetProps `json:"properties"`
		} `json:"duplicateSheet"`
	} `json:"replies"`
}

// batchUpdate posts a spreadsheets.batchUpdate with the given requests.
func (s *Service) batchUpdate(ctx context.Context, token, id string, requests ...any) ([]byte, error) {
	payload := map[string]any{"requests": requests}
	return s.call(ctx, token, http.MethodPost, "/spreadsheets/"+url.PathEscape(id)+":batchUpdate", nil, payload)
}

func (s *Service) newTabsAddCmd(token string) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "add <id> --title T",
		Short: "Add a new tab (batchUpdate: AddSheet)",
		Long: "Sheets rejects a --title another tab in the same spreadsheet already holds.\n" +
			"The tab is appended after the existing ones at the default grid size;\n" +
			"placing it at a specific index, or presetting row and column counts, needs\n" +
			"an AddSheet request through `spreadsheets batch-update`.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if title == "" {
				return fmt.Errorf("sheets: --title is required")
			}
			req := map[string]any{"addSheet": map[string]any{"properties": map[string]any{"title": title}}}
			body, err := s.batchUpdate(cmd.Context(), token, id, req)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			gid := newTabGID(body)
			fmt.Fprintf(s.stdout(), "added tab %q (gid=%d)\n", title, gid)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new tab title")
	return cmd
}

func (s *Service) newTabsRenameCmd(token string) *cobra.Command {
	var tab, title string
	cmd := &cobra.Command{
		Use:   "rename <id> --tab <name|gid> --title T",
		Short: "Rename a tab (batchUpdate: UpdateSheetProperties)",
		Long: "--tab names the target by title or numeric gid, --title is the new name.\n" +
			"Formulas inside the spreadsheet survive: Sheets rewrites A1 references to\n" +
			"the tab automatically. Anything outside it that hardcodes the old title\n" +
			"does not — including ranges an earlier command in the same run computed.\n" +
			"The gid never changes, which is why it is the stable handle.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if title == "" {
				return fmt.Errorf("sheets: --title is required")
			}
			sheetID, err := s.resolveTab(cmd.Context(), token, id, tab)
			if err != nil {
				return err
			}
			req := map[string]any{"updateSheetProperties": map[string]any{
				"properties": map[string]any{"sheetId": sheetID, "title": title},
				"fields":     "title",
			}}
			if _, err := s.batchUpdate(cmd.Context(), token, id, req); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"sheetId": sheetID, "title": title, "status": "renamed"})
			}
			fmt.Fprintf(s.stdout(), "renamed tab gid=%d to %q\n", sheetID, title)
			return nil
		},
	}
	cmd.Flags().StringVar(&tab, "tab", "", "target tab (title or numeric gid)")
	cmd.Flags().StringVar(&title, "title", "", "new tab title")
	return cmd
}

func (s *Service) newTabsDuplicateCmd(token string) *cobra.Command {
	var tab, title string
	cmd := &cobra.Command{
		Use:   "duplicate <id> --tab <name|gid> [--title T]",
		Short: "Duplicate a tab within the spreadsheet (batchUpdate: DuplicateSheet)",
		Long: "Copies values and formatting into the SAME spreadsheet — crossing files is\n" +
			"`tabs copy-to`. --title names the copy and Sheets chooses one when it is\n" +
			"omitted. The new tab gets a fresh gid, printed on success, and formulas in\n" +
			"the copy that named the source tab still point at the source rather than\n" +
			"at the duplicate.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			sheetID, err := s.resolveTab(cmd.Context(), token, id, tab)
			if err != nil {
				return err
			}
			dup := map[string]any{"sourceSheetId": sheetID}
			if title != "" {
				dup["newSheetName"] = title
			}
			req := map[string]any{"duplicateSheet": dup}
			body, err := s.batchUpdate(cmd.Context(), token, id, req)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			gid := newTabGID(body)
			fmt.Fprintf(s.stdout(), "duplicated tab gid=%d → new tab gid=%d\n", sheetID, gid)
			return nil
		},
	}
	cmd.Flags().StringVar(&tab, "tab", "", "source tab (title or numeric gid)")
	cmd.Flags().StringVar(&title, "title", "", "name for the duplicate (auto-chosen if omitted)")
	return cmd
}

func (s *Service) newTabsCopyToCmd(token string) *cobra.Command {
	var tab, dest string
	cmd := &cobra.Command{
		Use:   "copy-to <id> --tab <name|gid> --dest <spreadsheetId>",
		Short: "Copy a tab into another spreadsheet (spreadsheets.sheets.copyTo)",
		Long: "--dest takes the destination spreadsheet id or URL, and the connected\n" +
			"account needs write access there as well as read access on the source.\n" +
			"Values and formatting come across; formulas that referenced OTHER tabs of\n" +
			"the source spreadsheet do not, because those tabs do not exist in the\n" +
			"destination. The copy arrives with a name derived from the original and a\n" +
			"new gid, both reported on success.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			if dest == "" {
				return fmt.Errorf("sheets: --dest is required")
			}
			destID, err := parseSpreadsheetID(dest)
			if err != nil {
				return err
			}
			sheetID, err := s.resolveTab(cmd.Context(), token, id, tab)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost,
				fmt.Sprintf("/spreadsheets/%s/sheets/%d:copyTo", url.PathEscape(id), sheetID),
				nil, map[string]any{"destinationSpreadsheetId": destID})
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var props sheetProps
			if err := json.Unmarshal(body, &props); err != nil {
				return fmt.Errorf("sheets: decode copyTo response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "copied tab gid=%d to spreadsheet %s as %q (gid=%d)\n", sheetID, destID, props.Title, props.SheetID)
			return nil
		},
	}
	cmd.Flags().StringVar(&tab, "tab", "", "source tab (title or numeric gid)")
	cmd.Flags().StringVar(&dest, "dest", "", "destination spreadsheet id or URL")
	return cmd
}

func (s *Service) newTabsDeleteCmd(token string) *cobra.Command {
	var tab string
	cmd := &cobra.Command{
		Use:   "delete <id> --tab <name|gid>",
		Short: "Delete a tab — irreversible; cross-tab formula references become #REF! (batchUpdate: DeleteSheet)",
		Long: "Version history can bring the data back but not the structure: formulas in\n" +
			"other tabs that referenced this one become #REF! and stay broken. --tab\n" +
			"resolves a title first and a numeric gid second, and when two tabs share a\n" +
			"title the command refuses and demands the gid. Sheets will not let the\n" +
			"last remaining tab be deleted.",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseSpreadsheetID(args[0])
			if err != nil {
				return err
			}
			sheetID, err := s.resolveTab(cmd.Context(), token, id, tab)
			if err != nil {
				return err
			}
			req := map[string]any{"deleteSheet": map[string]any{"sheetId": sheetID}}
			if _, err := s.batchUpdate(cmd.Context(), token, id, req); err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(map[string]any{"sheetId": sheetID, "status": "deleted"})
			}
			fmt.Fprintf(s.stdout(), "deleted tab gid=%d\n", sheetID)
			return nil
		},
	}
	cmd.Flags().StringVar(&tab, "tab", "", "target tab (title or numeric gid)")
	return cmd
}

// resolveTab fetches the spreadsheet metadata and maps a --tab value (title or
// gid) to its sheetId.
func (s *Service) resolveTab(ctx context.Context, token, id, tab string) (int64, error) {
	meta, err := s.fetchMeta(ctx, token, id)
	if err != nil {
		return 0, err
	}
	return resolveTabID(meta, tab)
}

// newTabGID extracts the new tab's gid from an AddSheet/DuplicateSheet
// batchUpdate reply, or -1 if absent.
func newTabGID(body []byte) int64 {
	var r batchUpdateReplies
	if err := json.Unmarshal(body, &r); err != nil {
		return -1
	}
	for _, reply := range r.Replies {
		if reply.AddSheet != nil {
			return reply.AddSheet.Properties.SheetID
		}
		if reply.DuplicateSheet != nil {
			return reply.DuplicateSheet.Properties.SheetID
		}
	}
	return -1
}
