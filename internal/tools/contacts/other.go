package contacts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newOtherListCmd(token string) *cobra.Command {
	var pageToken string
	var max int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List Other Contacts (otherContacts.list)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("readMask", otherReadMask)
			q.Set("pageSize", strconv.Itoa(max))
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/otherContacts", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				OtherContacts []person `json:"otherContacts"`
				NextPageToken string   `json:"nextPageToken"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("contacts: decode other contacts: %w", err)
			}
			if len(resp.OtherContacts) == 0 {
				fmt.Fprintln(s.stdout(), "no other contacts")
				return nil
			}
			for i := range resp.OtherContacts {
				writeLine(s.stdout(), &resp.OtherContacts[i])
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 100, "max results to return (1-1000)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	return cmd
}

func (s *Service) newOtherSearchCmd(token string) *cobra.Command {
	var query string
	var max int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search Other Contacts by prefix phrase (otherContacts.search)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return fmt.Errorf("contacts: --query is required")
			}
			body, people, err := s.searchOnce(cmd.Context(), token, "/otherContacts:search", query, otherReadMask, max)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			if len(people) == 0 {
				fmt.Fprintf(s.stdout(), "no other contacts matched %q\n", query)
				return nil
			}
			for i := range people {
				writeLine(s.stdout(), &people[i])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "prefix-phrase search query (matches names/emails/phones)")
	cmd.Flags().IntVar(&max, "max", 10, "max results to return (API hard cap 30)")
	return cmd
}
