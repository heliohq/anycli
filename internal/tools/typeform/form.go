package typeform

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newFormListCmd is `form list` (GET /forms): search/list forms across the
// account, filtered by workspace and public state, sorted by created_at or
// last_updated_at. Pagination is surfaced, not auto-drained — the agent drives
// --page/--page-size (max 200). The list envelope keeps total_items/page_count/
// items verbatim. Output JSON.
func (s *Service) newFormListCmd(token string) *cobra.Command {
	var search, workspaceID, sortBy, orderBy string
	var page, pageSize int
	var isPublic bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List/search forms (GET /forms)",
		Long: "--search matches the form TITLE as a substring; it does not look inside\n" +
			"questions or match ids. --page-size defaults to 10 and caps at 200, and\n" +
			"paging is not drained for you — read `total_items` and `page_count` off\n" +
			"the envelope and advance --page. --sort-by (created_at|last_updated_at)\n" +
			"and --order-by (asc|desc) are case-exact and rejected locally on a miss.\n" +
			"Items carry metadata only; the `fields[]` dictionary needs `form get`.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sortBy != "" {
				if err := enumCheck("sort-by", sortBy, "created_at", "last_updated_at"); err != nil {
					return err
				}
			}
			if orderBy != "" {
				if err := enumCheck("order-by", orderBy, "asc", "desc"); err != nil {
					return err
				}
			}
			q := url.Values{}
			if search != "" {
				q.Set("search", search)
			}
			if workspaceID != "" {
				q.Set("workspace_id", workspaceID)
			}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if sortBy != "" {
				q.Set("sort_by", sortBy)
			}
			if orderBy != "" {
				q.Set("order_by", orderBy)
			}
			if cmd.Flags().Changed("is-public") {
				q.Set("is_public", strconv.FormatBool(isPublic))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "return forms whose title contains this string")
	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "restrict to one workspace")
	cmd.Flags().IntVar(&page, "page", 0, "page number (default 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page (default 10, max 200)")
	cmd.Flags().StringVar(&sortBy, "sort-by", "", "sort field: created_at|last_updated_at")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "order direction: asc|desc")
	cmd.Flags().BoolVar(&isPublic, "is-public", false, "filter by settings.is_public")
	return cmd
}

// newFormGetCmd is `form get <form_id>` (GET /forms/{id}): the full form
// definition — the field dictionary (ids, refs, types, choice labels) an agent
// needs to interpret a response's answers array. Output JSON.
func (s *Service) newFormGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <form_id>",
		Short: "Retrieve a form definition (GET /forms/{id})",
		Long: "Returns the whole form definition, including the `fields[]` dictionary of\n" +
			"ids, refs, types and choice labels — the only way to attach a question\n" +
			"title to an answer coming out of `response list`. Fetch it once per form\n" +
			"and reuse it; the definition changes only when someone edits the form.\n" +
			"It is also the correct starting point for `form update`, which overwrites\n" +
			"whatever it is not sent.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}

// newFormCreateCmd is `form create --definition <json|@file>` (POST /forms):
// draft a new form from a full form-definition JSON. Output JSON (the created
// form, including its assigned id).
func (s *Service) newFormCreateCmd(token string) *cobra.Command {
	var definition string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a form (POST /forms)",
		Long: "--definition is a complete form-definition JSON object, inline or `@file`;\n" +
			"there is no flag-per-field shortcut for a title or a question. The created\n" +
			"form comes back in full, including the assigned `id` that every other\n" +
			"command in the tool takes. Where it lands is decided by the definition's\n" +
			"`workspace` href — resolve one with `workspace list` first if the form\n" +
			"must not go to the default workspace.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := readJSONArg("definition", definition)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/forms", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&definition, "definition", "", "full form-definition JSON, inline or @file (required)")
	return cmd
}

// newFormUpdateCmd is `form update <form_id> --definition <json|@file>`
// (PUT /forms/{id}): a full overwrite — the ONLY way to change a form's
// fields/questions. Output JSON.
func (s *Service) newFormUpdateCmd(token string) *cobra.Command {
	var definition string
	cmd := &cobra.Command{
		Use:   "update <form_id>",
		Short: "Overwrite a form (PUT /forms/{id}); the only way to edit fields/questions",
		Long: "A PUT of the COMPLETE definition, not a merge: anything absent from\n" +
			"--definition is dropped from the form. Read the current definition with\n" +
			"`form get`, edit that object, and send the whole thing back. This is the\n" +
			"only path that reaches fields and questions — `form patch` cannot touch\n" +
			"them. The change is live for respondents as soon as the call returns.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readJSONArg("definition", definition)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodPut, "/forms/"+url.PathEscape(args[0]), nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&definition, "definition", "", "full form-definition JSON, inline or @file (required)")
	return cmd
}

// newFormPatchCmd is `form patch <form_id> --patch <json|@file>`
// (PATCH /forms/{id}): a JSON-Patch operations array restricted by the API to
// metadata paths (/title, /theme, /workspace, /settings/*) — there is no path
// for fields/questions, so question edits must go through `form update`.
// Success is 204 No Content; a client-side receipt is emitted for a definite
// success signal.
func (s *Service) newFormPatchCmd(token string) *cobra.Command {
	var patch string
	cmd := &cobra.Command{
		Use:   "patch <form_id>",
		Short: "Patch form metadata (PATCH /forms/{id}); metadata-only JSON-Patch, use 'form update' to change questions",
		Long: "The API restricts this JSON-Patch array to metadata paths — `/title`,\n" +
			"`/theme`, `/workspace` and `/settings/*` — so no operation here can reach\n" +
			"a field or a question; those go through `form update`. --patch is the\n" +
			"operations array itself, inline or `@file`, e.g.\n" +
			"`[{\"op\":\"replace\",\"path\":\"/title\",\"value\":\"New title\"}]`. The API answers\n" +
			"204 with no body, so success prints a local receipt rather than the\n" +
			"patched form — re-read it with `form get` to confirm.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readJSONArg("patch", patch)
			if err != nil {
				return err
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPatch, "/forms/"+url.PathEscape(args[0]), nil, payload); err != nil {
				return err
			}
			return s.emitOK(map[string]any{"patched": true, "form_id": args[0]})
		},
	}
	cmd.Flags().StringVar(&patch, "patch", "", "JSON-Patch operations array, inline or @file (required)")
	return cmd
}

// newFormDeleteCmd is `form delete <form_id>` (DELETE /forms/{id}). Success is
// 204 No Content; a client-side receipt is emitted. Output JSON.
func (s *Service) newFormDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <form_id>",
		Short: "Delete a form (DELETE /forms/{id})",
		Long: "The form's collected responses go with it. The API answers 204 with no\n" +
			"body, so success prints a local receipt `{\"deleted\":true,\"form_id\":…}`\n" +
			"rather than anything from Typeform, and nothing in this tool restores\n" +
			"either the form or its responses — export what is needed with\n" +
			"`response list` first.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/forms/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			return s.emitOK(map[string]any{"deleted": true, "form_id": args[0]})
		},
	}
	return cmd
}

// enumCheck validates a case-exact value against a fixed allowed set, returning
// a usageError on a miss.
func enumCheck(flag, val string, allowed ...string) error {
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}
	return &usageError{msg: "--" + flag + " must be one of " + joinPipe(allowed) + ", got " + strconv.Quote(val)}
}

// joinPipe renders a set as a|b|c for enum error messages.
func joinPipe(vals []string) string {
	out := ""
	for i, v := range vals {
		if i > 0 {
			out += "|"
		}
		out += v
	}
	return out
}
