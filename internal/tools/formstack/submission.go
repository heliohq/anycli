package formstack

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func (s *Service) newSubmissionCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "submission", Short: "Submissions (list, get, create, delete)"}
	cmd.AddCommand(
		s.newSubmissionListCmd(token),
		s.newSubmissionGetCmd(token),
		s.newSubmissionCreateCmd(token),
		s.newSubmissionDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newSubmissionListCmd(token string) *cobra.Command {
	var since, until, sort, encryptionPassword string
	var search []string
	var page, perPage int
	var noData, expandData bool
	cmd := &cobra.Command{
		Use:   "list <form-id>",
		Short: "List a form's submissions (GET /form/{id}/submission.json)",
		Long: "`--since` and `--until` take `YYYY-MM-DD` or `YYYY-MM-DD HH:MM:SS` and\n" +
			"Formstack reads them in US/Eastern, not UTC — a window that looks short\n" +
			"by a few hours is almost always that assumption rather than missing data.\n" +
			"`--search field=value` is repeatable and matches exactly; the `field`\n" +
			"part must be a numeric field id from `form fields`, and a label there\n" +
			"matches nothing rather than erroring. Answers are inlined by default, so\n" +
			"`--no-data` is the light call when only counts and timestamps matter.\n" +
			"`--per-page` caps at 100 (default 25) and `--sort` is `ASC` or `DESC` on\n" +
			"submission time. An encrypted form returns unreadable values unless\n" +
			"`--encryption-password` is passed.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if noData {
				q.Set("data", "false")
			} else {
				q.Set("data", "true")
			}
			if expandData {
				q.Set("expand_data", "true")
			}
			if since != "" {
				q.Set("min_time", since)
			}
			if until != "" {
				q.Set("max_time", until)
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
			for i, pair := range search {
				field, value, ok := strings.Cut(pair, "=")
				if !ok {
					return fmt.Errorf("formstack: --search %q must be field=value", pair)
				}
				q.Set(fmt.Sprintf("search_field_%d", i), field)
				q.Set(fmt.Sprintf("search_value_%d", i), value)
			}
			var headers map[string]string
			if encryptionPassword != "" {
				headers = map[string]string{EncryptionPasswordHeader: encryptionPassword}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/form/"+url.PathEscape(args[0])+"/submission.json", q, nil, headers)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "earliest submission time (min_time)")
	cmd.Flags().StringVar(&until, "until", "", "latest submission time (max_time)")
	cmd.Flags().StringArrayVar(&search, "search", nil, "field=value filter (repeatable, maps to search_field_N/search_value_N)")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&perPage, "per-page", 25, "results per page (max 100)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort by submission time: ASC|DESC")
	cmd.Flags().BoolVar(&noData, "no-data", false, "return metadata only (data=false)")
	cmd.Flags().BoolVar(&expandData, "expand-data", false, "expand field data (expand_data=true)")
	cmd.Flags().StringVar(&encryptionPassword, "encryption-password", "", "password for encrypted forms (X-FS-ENCRYPTION-PASSWORD)")
	return cmd
}

func (s *Service) newSubmissionGetCmd(token string) *cobra.Command {
	var encryptionPassword string
	cmd := &cobra.Command{
		Use:   "get <submission-id>",
		Short: "Get a submission (GET /submission/{id}.json)",
		Long: "Takes a SUBMISSION id, not a form id, and there is no search from here —\n" +
			"the id comes from `submission list`. Values arrive keyed by numeric field\n" +
			"id, so `form fields` on the parent form is what makes them readable. An\n" +
			"encrypted form needs the same `--encryption-password` the list command\n" +
			"takes.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			var headers map[string]string
			if encryptionPassword != "" {
				headers = map[string]string{EncryptionPasswordHeader: encryptionPassword}
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/submission/"+url.PathEscape(args[0])+".json", nil, nil, headers)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&encryptionPassword, "encryption-password", "", "password for encrypted forms (X-FS-ENCRYPTION-PASSWORD)")
	return cmd
}

func (s *Service) newSubmissionCreateCmd(token string) *cobra.Command {
	var fields []string
	var read bool
	cmd := &cobra.Command{
		Use:   "create <form-id>",
		Short: "Create a submission (POST /form/{id}/submission.json)",
		Long: "Records a response on the form as though a respondent had filled it in.\n" +
			"`--field id=value` is repeatable and required at least once, and `id` is\n" +
			"the numeric field id from `form fields`: a label there is accepted by the\n" +
			"CLI and then ignored by Formstack, which surfaces as a submission with\n" +
			"blanks rather than an error. Choice-field values have to match one of\n" +
			"that field's `options` exactly. `--read` marks the submission read so it\n" +
			"does not sit in the account as new. Answers cannot be edited afterwards —\n" +
			"correcting one means `submission delete` and a fresh create.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			for _, pair := range fields {
				id, value, ok := strings.Cut(pair, "=")
				if !ok {
					return fmt.Errorf("formstack: --field %q must be id=value", pair)
				}
				body["field_"+id] = value
			}
			if read {
				body["read"] = true
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/form/"+url.PathEscape(args[0])+"/submission.json", nil, body, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "id=value field answer (repeatable, maps to field_<id>)")
	cmd.Flags().BoolVar(&read, "read", false, "mark the submission read")
	_ = cmd.MarkFlagRequired("field")
	return cmd
}

func (s *Service) newSubmissionDeleteCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <submission-id>",
		Short: "Delete a submission (DELETE /submission/{id}.json)",
		Long: "Removes one respondent's answers, addressed by submission id. Nothing\n" +
			"here restores it and the form's `submissions` count drops accordingly.\n" +
			"Because answers cannot be edited in place, a correction is this command\n" +
			"followed by `submission create` — which loses the original's timestamp\n" +
			"and id.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/submission/"+url.PathEscape(args[0])+".json", nil, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}
