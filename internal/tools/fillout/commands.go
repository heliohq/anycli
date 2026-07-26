package fillout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// newFormListCmd: GET /v1/api/forms — all forms in the account.
func (s *Service) newFormListCmd(token, apiBase string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all forms in the account",
		Long: "Returns every form in the account in a single response — there is no\n" +
			"pagination, filter or search on this endpoint. It is also the only way to\n" +
			"discover a form id, which every other command in this tool requires.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodGet, "/forms", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newFormGetCmd: GET /v1/api/forms/{formId} — form metadata + question schema.
func (s *Service) newFormGetCmd(token, apiBase string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <formId>",
		Short: "Get a form's metadata and question schema",
		Long: "Returns the form's settings alongside its question schema: the id, label and\n" +
			"type of every field. That schema is the decoder for `submission list` and\n" +
			"`submission get`, whose answers reference question ids and nothing else, and\n" +
			"it supplies the ids `submission create` has to send.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodGet, "/forms/"+args[0], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newSubmissionListCmd: GET /v1/api/forms/{formId}/submissions — responses,
// with the documented filters passed through as query params.
func (s *Service) newSubmissionListCmd(token, apiBase string) *cobra.Command {
	var (
		limit, offset                   int
		status, afterDate, beforeDate   string
		sort, search                    string
		includeEditLink, includePreview bool
	)
	cmd := &cobra.Command{
		Use:   "list <formId>",
		Short: "List a form's submissions (responses)",
		Long: "Paging is OFFSET-based, not cursored: `--offset` (default 0) with `--limit`,\n" +
			"which Fillout defaults to 50 and caps at 150. That cap is enforced by the\n" +
			"provider, not locally, so an out-of-range page size surfaces as an API\n" +
			"error rather than a usage one.\n" +
			"\n" +
			"`--status` accepts only finished or in_progress and `--sort` only asc or\n" +
			"desc; both are rejected locally at exit 2 before any request goes out.\n" +
			"`--after-date` / `--before-date` take ISO date-times, `--search` matches text\n" +
			"anywhere in a response, and `--include-preview` adds preview responses that\n" +
			"are otherwise left out.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := map[string]string{}
			flags := cmd.Flags()
			if flags.Changed("limit") {
				q["limit"] = fmt.Sprintf("%d", limit)
			}
			if flags.Changed("offset") {
				q["offset"] = fmt.Sprintf("%d", offset)
			}
			if flags.Changed("status") {
				if status != "finished" && status != "in_progress" {
					return &usageError{msg: fmt.Sprintf("invalid --status %q: want finished or in_progress", status)}
				}
				q["status"] = status
			}
			if flags.Changed("sort") {
				if sort != "asc" && sort != "desc" {
					return &usageError{msg: fmt.Sprintf("invalid --sort %q: want asc or desc", sort)}
				}
				q["sort"] = sort
			}
			if afterDate != "" {
				q["afterDate"] = afterDate
			}
			if beforeDate != "" {
				q["beforeDate"] = beforeDate
			}
			if search != "" {
				q["search"] = search
			}
			if includeEditLink {
				q["includeEditLink"] = "true"
			}
			if includePreview {
				q["includePreview"] = "true"
			}
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodGet, "/forms/"+args[0]+"/submissions", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Annotations = readOnly
	f := cmd.Flags()
	f.IntVar(&limit, "limit", 0, "max submissions per request (1-150; Fillout default 50)")
	f.IntVar(&offset, "offset", 0, "starting position (Fillout default 0)")
	f.StringVar(&status, "status", "", "filter by status: finished or in_progress")
	f.StringVar(&afterDate, "after-date", "", "only submissions after this ISO date-time")
	f.StringVar(&beforeDate, "before-date", "", "only submissions before this ISO date-time")
	f.StringVar(&sort, "sort", "", "sort order: asc or desc")
	f.StringVar(&search, "search", "", "filter to submissions containing this text")
	f.BoolVar(&includeEditLink, "include-edit-link", false, "include an editLink per submission")
	f.BoolVar(&includePreview, "include-preview", false, "include preview responses")
	return cmd
}

// newSubmissionGetCmd: GET /v1/api/forms/{formId}/submissions/{submissionId}.
func (s *Service) newSubmissionGetCmd(token, apiBase string) *cobra.Command {
	var includeEditLink bool
	cmd := &cobra.Command{
		Use:   "get <formId> <submissionId>",
		Short: "Get a single submission",
		Long: "Both ids are positional and ordered form-first; there is no way to fetch a\n" +
			"submission by id alone. `--include-edit-link` adds an `editLink`, a URL that\n" +
			"lets whoever holds it reopen and change the response, so it is a capability\n" +
			"rather than an identifier — do not paste it around.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := map[string]string{}
			if includeEditLink {
				q["includeEditLink"] = "true"
			}
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodGet, "/forms/"+args[0]+"/submissions/"+args[1], q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Annotations = readOnly
	cmd.Flags().BoolVar(&includeEditLink, "include-edit-link", false, "include an editLink for the submission")
	return cmd
}

// newSubmissionCreateCmd: POST /v1/api/forms/{formId}/submissions. The request
// body is passed through verbatim from --data or --file; the service validates
// it is JSON but does not reshape it (thin passthrough of Fillout's own shape).
func (s *Service) newSubmissionCreateCmd(token, apiBase string) *cobra.Command {
	var data, file string
	cmd := &cobra.Command{
		Use:   "create <formId>",
		Short: "Create submission(s) on a form (body from --data or --file)",
		Long: "Supply the body through exactly one of `--data` (inline) or `--file`;\n" +
			"both or neither is a usage error. It is forwarded VERBATIM — the only local\n" +
			"check is that it parses as JSON, so a wrong shape comes back as a Fillout\n" +
			"4xx. The top level is `{\"submissions\": [...]}` with at most 10 entries, each\n" +
			"holding a `questions` array of id/value pairs whose ids come from\n" +
			"`form get`.\n" +
			"\n" +
			"Fillout treats API-created submissions as silent: no email notification,\n" +
			"workflow or integration fires for them, so anything downstream of a normal\n" +
			"response will not happen here.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := readBody(data, file)
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodPost, "/forms/"+args[0]+"/submissions", nil, bytes.NewReader(raw))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&data, "data", "", "submissions JSON body (e.g. {\"submissions\":[...]})")
	cmd.Flags().StringVar(&file, "file", "", "path to a file holding the submissions JSON body")
	return cmd
}

// newSubmissionDeleteCmd: DELETE /v1/api/forms/{formId}/submissions/{submissionId}.
func (s *Service) newSubmissionDeleteCmd(token, apiBase string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <formId> <submissionId>",
		Short: "Delete a submission",
		Long: "Takes the form id then the submission id, both positional. Removes one\n" +
			"response per call — there is no bulk or filtered form — and nothing in this\n" +
			"tool restores it afterwards.",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodDelete, "/forms/"+args[0]+"/submissions/"+args[1], nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newWebhookCreateCmd: POST /v1/api/webhook/create with {formId, url}.
func (s *Service) newWebhookCreateCmd(token, apiBase string) *cobra.Command {
	var formID, url string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register a webhook on a form for new submissions",
		Long: "`--form-id` and `--url` are both required flags; the form id is NOT\n" +
			"positional here as it is on the `submission` verbs. The response carries the\n" +
			"new webhook's id, and that response is the ONLY place it appears — there is\n" +
			"no webhook list verb, so an id that is not recorded now cannot be recovered\n" +
			"later and `webhook delete` becomes unreachable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := json.Marshal(map[string]string{"formId": formID, "url": url})
			if err != nil {
				return &apiError{msg: fmt.Sprintf("fillout: encode request: %v", err)}
			}
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodPost, "/webhook/create", nil, bytes.NewReader(raw))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&formID, "form-id", "", "public form identifier")
	cmd.Flags().StringVar(&url, "url", "", "endpoint that receives submission events")
	_ = cmd.MarkFlagRequired("form-id")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

// newWebhookDeleteCmd: POST /v1/api/webhook/delete with {webhookId}.
func (s *Service) newWebhookDeleteCmd(token, apiBase string) *cobra.Command {
	var webhookID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove a webhook",
		Long: "`--webhook-id` is the id returned by `webhook create`. Nothing here can\n" +
			"enumerate existing webhooks, so an unrecorded id has to be found in\n" +
			"Fillout's own UI. Deleting stops future deliveries only; submissions already\n" +
			"delivered are untouched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := json.Marshal(map[string]string{"webhookId": webhookID})
			if err != nil {
				return &apiError{msg: fmt.Sprintf("fillout: encode request: %v", err)}
			}
			body, err := s.call(cmd.Context(), token, apiBase, http.MethodPost, "/webhook/delete", nil, bytes.NewReader(raw))
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Annotations = writeAction
	cmd.Flags().StringVar(&webhookID, "webhook-id", "", "the webhook ID returned at creation")
	_ = cmd.MarkFlagRequired("webhook-id")
	return cmd
}

// readBody resolves the create-submission request body from exactly one of
// --data (inline JSON) or --file (path), validating that it parses as JSON so
// a malformed body fails as a usage error (exit 2) before any network call.
func readBody(data, file string) ([]byte, error) {
	if (data == "") == (file == "") {
		return nil, &usageError{msg: "provide exactly one of --data or --file"}
	}
	raw := []byte(data)
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, &usageError{msg: fmt.Sprintf("read --file: %v", err)}
		}
		raw = b
	}
	if !json.Valid(raw) {
		return nil, &usageError{msg: "request body is not valid JSON"}
	}
	return raw, nil
}
