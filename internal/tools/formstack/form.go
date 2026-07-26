package formstack

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newFormCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "form", Short: "Forms (list, get, fields, create, copy, delete)"}
	cmd.AddCommand(
		s.newFormListCmd(token),
		s.newFormGetCmd(token),
		s.newFormFieldsCmd(token),
		s.newFormCreateCmd(token),
		s.newFormCopyCmd(token),
		s.newFormDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newFormListCmd(token string) *cobra.Command {
	var search, folder, sort string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List forms (GET /form.json)",
		Long: "`--search` matches the form NAME as a substring; it does not reach into\n" +
			"field labels or submitted answers. `--folder` takes an id from\n" +
			"`folder list`, and `--sort` accepts `id`, `name-asc` or `desc`.\n" +
			"`--per-page` is capped at 100 (default 25); page and per-page are only\n" +
			"sent when explicitly passed, so Formstack's own defaults apply otherwise.\n" +
			"This is the only account-wide enumeration in the tool — everything else\n" +
			"needs a form id first.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if search != "" {
				q.Set("search", search)
			}
			if folder != "" {
				q.Set("folder", folder)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			if cmd.Flags().Changed("page") {
				q.Set("page", itoa(page))
			}
			if cmd.Flags().Changed("per-page") {
				q.Set("per_page", itoa(perPage))
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form.json", q, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "search forms by name")
	cmd.Flags().StringVar(&folder, "folder", "", "filter by folder id")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order: id|name-asc|desc")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&perPage, "per-page", 25, "results per page (max 100)")
	return cmd
}

func (s *Service) newFormGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <form-id>",
		Short: "Get a form (GET /form/{id}.json)",
		Long: "Returns the form's own record: `name`, the `submissions` count and\n" +
			"`last_submission_time`, the public `url` a respondent would open, its\n" +
			"folder and whether the form is encrypted. Read `submissions` before\n" +
			"pulling responses — it is how you size a `submission list` walk without\n" +
			"paging it. `form fields` is the reliable way to enumerate the questions\n" +
			"and their ids.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormFieldsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fields <form-id>",
		Short: "List a form's fields (GET /form/{id}/field.json)",
		Long: "The mapping the rest of the tool depends on: each entry's `id` is the\n" +
			"numeric field id that keys a submission's values, that\n" +
			"`submission list --search <id>=<value>` filters on, and that\n" +
			"`submission create --field <id>=…` writes. `label` is the human question\n" +
			"text and is accepted nowhere as a substitute. `type` tells you the value\n" +
			"shape — an answer to a `select`, `radio` or `checkbox` field has to match\n" +
			"one of that field's `options` exactly, and `name` and `address` fields\n" +
			"carry sub-values rather than one string.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+"/field.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormCreateCmd(token string) *cobra.Command {
	var name, folder string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a form (POST /form.json)",
		Long: "Creates an empty form — `--name` is required, `--folder` takes an id from\n" +
			"`folder list`. No fields come with it, so the form has no questions until\n" +
			"`field create` adds them one call at a time, and the returned `id` is\n" +
			"what those calls take. When a similar form already exists, `form copy`\n" +
			"reproduces its whole field set in one call and is the cheaper start.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"name": name}
			if folder != "" {
				body["folder"] = folder
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form.json", nil, body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "form name")
	cmd.Flags().StringVar(&folder, "folder", "", "folder id to create the form in")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func (s *Service) newFormCopyCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copy <form-id>",
		Short: "Copy a form (POST /form/{id}/copy.json)",
		Long: "Duplicates the form's structure and settings into a new form and returns\n" +
			"it with its own id. Submissions are not copied, and the copy's fields are\n" +
			"new fields with NEW ids — anything that referenced the original's field\n" +
			"ids must be remapped through `form fields` on the copy before it will\n" +
			"read or write answers correctly.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form/"+url.PathEscape(args[0])+"/copy.json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newFormDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <form-id>",
		Short: "Delete a form (DELETE /form/{id}.json); soft delete per the API",
		Long: "The form stops accepting responses and leaves the account's active list;\n" +
			"Formstack keeps it rather than purging it, but nothing in this tool\n" +
			"restores one — that is a Formstack UI action. The submissions collected\n" +
			"so far go out of reach with it, so pull anything worth keeping through\n" +
			"`submission list` first.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/form/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
