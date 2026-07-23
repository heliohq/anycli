package zohocrm

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newQueryCmd is top-level `query` — POST /crm/v8/coql with
// {"select_query":"…"}. COQL is the precise filtered/aggregated read path and
// the workaround for search-index lag (freshly written records are readable
// via COQL immediately, while search may 204). An empty result is a 204 that
// emits nothing and exits 0.
func (s *Service) newQueryCmd(token string) *cobra.Command {
	var coql string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Run a COQL select query",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&coql, "coql", "", "COQL select query, e.g. select Last_Name from Leads where ... (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(coql) == "" {
			return &usageError{msg: "--coql is required (e.g. select Last_Name, Email from Leads where Email is not null limit 200)"}
		}
		payload := map[string]any{"select_query": coql}
		body, err := s.call(cmd.Context(), token, http.MethodPost, "/coql", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newNoteListCmd is `note list` — GET /crm/v8/{module}/{id}/Notes: the notes
// attached to one record.
func (s *Service) newNoteListCmd(token string) *cobra.Command {
	var module, id string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List notes on a record",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		path := modulePath(module) + "/" + url.PathEscape(strings.TrimSpace(id)) + "/Notes"
		body, err := s.call(cmd.Context(), token, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newNoteAddCmd is `note add` — POST /crm/v8/{module}/{id}/Notes with
// {"data":[{"Note_Title":…,"Note_Content":…}]}.
func (s *Service) newNoteAddCmd(token string) *cobra.Command {
	var module, id, title, content string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a note to a record",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&module, "module", "", "module API name (required)")
	cmd.Flags().StringVar(&id, "id", "", "record id (required)")
	cmd.Flags().StringVar(&title, "title", "", "note title (required)")
	cmd.Flags().StringVar(&content, "content", "", "note content (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireModule(module); err != nil {
			return err
		}
		if strings.TrimSpace(id) == "" {
			return &usageError{msg: "--id is required"}
		}
		if strings.TrimSpace(title) == "" {
			return &usageError{msg: "--title is required"}
		}
		if strings.TrimSpace(content) == "" {
			return &usageError{msg: "--content is required"}
		}
		payload := map[string]any{"data": []any{
			map[string]any{"Note_Title": title, "Note_Content": content},
		}}
		path := modulePath(module) + "/" + url.PathEscape(strings.TrimSpace(id)) + "/Notes"
		body, err := s.call(cmd.Context(), token, http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
