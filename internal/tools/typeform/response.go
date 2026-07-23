package typeform

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// responseTypes is the allowed set for --response-type members (validated
// member-by-member). The API default when unset is completed.
var responseTypes = []string{"started", "partial", "completed"}

// newResponseListCmd is `response list <form_id>` (GET /forms/{id}/responses):
// the dominant read job — pull responses, filtered by date window,
// completeness, text query, and specific fields. Pagination is via the token
// cursors --after/--before (max page size 1000); the agent drives traversal.
//
// The chosen --response-type changes which timestamp --since/--until filter on:
// submitted_at (completed), staged_at (partial), landed_at (started). Cursors
// (--after/--before) cannot be combined with --sort per the API; v1 passes both
// through and lets the API fail fast rather than pre-validating. Items are
// passed through untransformed (answers[].field.{id,ref,type} + typed values),
// so an agent joins them against `form get`'s field dictionary. Output JSON.
func (s *Service) newResponseListCmd(token string) *cobra.Command {
	var since, until, after, before, query, sort string
	var responseType, fields, answeredFields, includedIDs, excludedIDs []string
	var pageSize int
	cmd := &cobra.Command{
		Use:         "list <form_id>",
		Short:       "List a form's responses (GET /forms/{id}/responses)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, rt := range responseType {
				if err := enumCheck("response-type", rt, responseTypes...); err != nil {
					return err
				}
			}
			q := url.Values{}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if since != "" {
				q.Set("since", since)
			}
			if until != "" {
				q.Set("until", until)
			}
			if after != "" {
				q.Set("after", after)
			}
			if before != "" {
				q.Set("before", before)
			}
			if len(responseType) > 0 {
				q.Set("response_type", strings.Join(responseType, ","))
			}
			if query != "" {
				q.Set("query", query)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			if len(fields) > 0 {
				q.Set("fields", strings.Join(fields, ","))
			}
			if len(answeredFields) > 0 {
				q.Set("answered_fields", strings.Join(answeredFields, ","))
			}
			if len(includedIDs) > 0 {
				q.Set("included_response_ids", strings.Join(includedIDs, ","))
			}
			if len(excludedIDs) > 0 {
				q.Set("excluded_response_ids", strings.Join(excludedIDs, ","))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet,
				"/forms/"+url.PathEscape(args[0])+"/responses", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max responses per page (default 25, max 1000)")
	cmd.Flags().StringVar(&since, "since", "", "responses since this time (unix seconds or ISO 8601 UTC)")
	cmd.Flags().StringVar(&until, "until", "", "responses until this time (unix seconds or ISO 8601 UTC)")
	cmd.Flags().StringVar(&after, "after", "", "cursor: responses after this token (cannot combine with --sort)")
	cmd.Flags().StringVar(&before, "before", "", "cursor: responses before this token (cannot combine with --sort)")
	cmd.Flags().StringSliceVar(&responseType, "response-type", nil, "response types: started|partial|completed (repeatable/comma-separated; default completed)")
	cmd.Flags().StringVar(&query, "query", "", "restrict to responses containing this string")
	cmd.Flags().StringVar(&sort, "sort", "", "sort spec: {fieldID},{asc|desc} (e.g. submitted_at,desc)")
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "only show these fields in answers (comma-separated)")
	cmd.Flags().StringSliceVar(&answeredFields, "answered-fields", nil, "only responses answering at least one of these fields (comma-separated)")
	cmd.Flags().StringSliceVar(&includedIDs, "included-response-ids", nil, "restrict to these response ids (comma-separated)")
	cmd.Flags().StringSliceVar(&excludedIDs, "excluded-response-ids", nil, "exclude these response ids (comma-separated)")
	return cmd
}
