package googleanalytics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// accountSummary mirrors the Admin API accountSummaries entry subset the
// human list renders.
type accountSummary struct {
	Account           string `json:"account"`
	DisplayName       string `json:"displayName"`
	PropertySummaries []struct {
		Property    string `json:"property"`
		DisplayName string `json:"displayName"`
	} `json:"propertySummaries"`
}

// newPropertyListCmd lists every account + GA4 property the token can read —
// the discovery step yielding the numeric property id every report call needs.
func (s *Service) newPropertyListCmd(token string) *cobra.Command {
	var pageSize int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accessible accounts and their GA4 properties (accountSummaries)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if cmd.Flags().Changed("page-size") {
				q.Set("pageSize", strconv.Itoa(pageSize))
			}
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, s.adminBase(), "/accountSummaries", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				AccountSummaries []accountSummary `json:"accountSummaries"`
				NextPageToken    string           `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return &apiError{msg: fmt.Sprintf("google-analytics: decode account summaries: %v", err), err: err}
			}
			properties := 0
			for _, account := range resp.AccountSummaries {
				for _, p := range account.PropertySummaries {
					fmt.Fprintf(s.stdout(), "%s\t%s\t(account: %s %s)\n",
						p.Property, p.DisplayName, account.Account, account.DisplayName)
					properties++
				}
			}
			if properties == 0 {
				fmt.Fprintln(s.stdout(), "no properties")
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max account summaries per page (provider default 50)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	return cmd
}

// normalizeProperty accepts a bare numeric GA4 property id or the
// properties/<id> resource form and returns the canonical resource name.
func normalizeProperty(v string) (string, error) {
	id := strings.TrimPrefix(strings.TrimSpace(v), "properties/")
	if id == "" {
		return "", &usageError{msg: "google-analytics: --property is required (numeric GA4 property id; discover ids with `property list`)"}
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return "", &usageError{msg: fmt.Sprintf("google-analytics: --property must be a numeric GA4 property id or properties/<id>, got %q (discover ids with `property list`)", v)}
		}
	}
	return "properties/" + id, nil
}
