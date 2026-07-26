package forms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newResponsesListCmd(token string) *cobra.Command {
	var filter, pageToken string
	var max int
	cmd := &cobra.Command{
		Use:   "list <form-id>",
		Short: "List responses (forms.responses.list). --filter passes the API's native 'timestamp > / >= <RFC3339>' syntax verbatim",
		Long: "The summary lists response ids with their timestamps and respondent email,\n" +
			"NOT the answers — those come from `responses get`, or from `--json` here,\n" +
			"which returns the full response objects. `--filter` is the API's own\n" +
			"syntax, `timestamp >= 2026-01-01T00:00:00Z` or `timestamp > …`, and is the\n" +
			"only incremental mechanism: nothing notifies on a new submission, so a\n" +
			"watch is a poll. `--max` sets the page size and `--page-token` continues\n" +
			"from the previous `nextPageToken`. There is NO response-count endpoint, so\n" +
			"a total means paging to the end and counting.",
		Args: cobra.ExactArgs(1),
		// GET /forms/{id}/responses — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			q := url.Values{}
			if filter != "" {
				q.Set("filter", filter)
			}
			if max > 0 {
				q.Set("pageSize", strconv.Itoa(max))
			}
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(formID)+"/responses", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Responses []struct {
					ResponseID  string `json:"responseId"`
					CreateTime  string `json:"createTime"`
					RespondentE string `json:"respondentEmail"`
				} `json:"responses"`
				NextPageToken string `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("forms: decode response list: %w", err)
			}
			if len(resp.Responses) == 0 {
				fmt.Fprintln(s.stdout(), "no responses")
				return nil
			}
			for _, r := range resp.Responses {
				fmt.Fprintf(s.stdout(), "%s\t%s\t%s\n", r.ResponseID, r.CreateTime, r.RespondentE)
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "API filter, e.g. 'timestamp >= 2026-01-01T00:00:00Z'")
	cmd.Flags().IntVar(&max, "max", 0, "max responses per page (0 = API default)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	return cmd
}

func (s *Service) newResponsesGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <form-id> <response-id>",
		Short: "Show a single response with its answers (forms.responses.get)",
		Long: "Takes the form id and a response id from `responses list`, in that order.\n" +
			"Answers are keyed by QUESTION ID, not by question text, so the labels have\n" +
			"to be mapped through the items `get` returns. The default summary prints\n" +
			"text answers only; `--json` returns the full response, which is where file\n" +
			"uploads, grades and per-question metadata live.",
		Args: cobra.ExactArgs(2),
		// GET /forms/{id}/responses/{id} — read-only (design 318).
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			formID, err := extractFormID(args[0])
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/forms/"+url.PathEscape(formID)+"/responses/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var r struct {
				ResponseID string `json:"responseId"`
				CreateTime string `json:"createTime"`
				Answers    map[string]struct {
					TextAnswers struct {
						Answers []struct {
							Value string `json:"value"`
						} `json:"answers"`
					} `json:"textAnswers"`
				} `json:"answers"`
			}
			if err := json.Unmarshal(body, &r); err != nil {
				return fmt.Errorf("forms: decode response: %w", err)
			}
			fmt.Fprintf(s.stdout(), "ResponseId: %s\nCreated:    %s\nAnswers:    %d\n", r.ResponseID, r.CreateTime, len(r.Answers))
			for qid, a := range r.Answers {
				vals := make([]string, 0, len(a.TextAnswers.Answers))
				for _, v := range a.TextAnswers.Answers {
					vals = append(vals, v.Value)
				}
				fmt.Fprintf(s.stdout(), "  %s\t%v\n", qid, vals)
			}
			return nil
		},
	}
}
