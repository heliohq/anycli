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
		Long: "COQL is Zoho's SQL-shaped read language and the only filtered or\n" +
			"aggregated read in this tool: `select Last_Name, Email from Leads where\n" +
			"Email is not null limit 200`. Both the columns and the table are API\n" +
			"names, not labels. It queries the live table instead of the search\n" +
			"index, so it sees records written seconds ago that `record search`\n" +
			"still returns nothing for. Zoho caps one response at 200 rows, so a\n" +
			"larger read is a sequence of queries with a growing offset. An empty\n" +
			"result is a 204 that prints nothing and exits 0.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "`--module` and `--id` name the parent record: notes are always read\n" +
			"through the record they hang on, and there is no org-wide note listing\n" +
			"here. Entries carry `Note_Title`, `Note_Content`, the author and the\n" +
			"created time. This command exposes no paging flags, so a record with a\n" +
			"long history returns only Zoho's first page of notes.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "All four of `--module`, `--id`, `--title` and `--content` are required\n" +
			"and checked before the call. The note is attributed to the connected\n" +
			"user, is visible to everyone who can see the record, and cannot be\n" +
			"edited or removed through this tool. It is the durable place for a call\n" +
			"summary or any context that has no field of its own — writing that into\n" +
			"a record field instead would overwrite CRM data.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
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
