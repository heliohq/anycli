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
		Use:         "list",
		Short:       "List all forms in the account",
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
		Use:         "get <formId>",
		Short:       "Get a form's metadata and question schema",
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(1),
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
		Use:         "delete <formId> <submissionId>",
		Short:       "Delete a submission",
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
