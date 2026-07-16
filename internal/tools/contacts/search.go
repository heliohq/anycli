package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// searchResponse is the shared shape of people.searchContacts and
// otherContacts.search (both return a SearchResponse).
type searchResponse struct {
	Results []struct {
		Person person `json:"person"`
	} `json:"results"`
}

// clampSearchSize bounds a requested page size to the People API search limits
// (1..30). A non-positive value falls back to the API default of 10.
func clampSearchSize(n int) int {
	if n <= 0 {
		return 10
	}
	if n > searchPageCap {
		return searchPageCap
	}
	return n
}

// searchOnce runs one People API search endpoint end to end: it primes the
// lazy search cache with an empty warmup query (best effort — errors there are
// ignored because they resurface on the real query), waits, then issues the
// real query. It returns the raw body plus the decoded persons.
func (s *Service) searchOnce(ctx context.Context, token, path, query, readMask string, size int) ([]byte, []person, error) {
	warm := url.Values{}
	warm.Set("query", "")
	warm.Set("readMask", readMask)
	warm.Set("pageSize", "1")
	_, _ = s.call(ctx, token, http.MethodGet, path, warm, nil)
	s.pause(warmupDelay)

	q := url.Values{}
	q.Set("query", query)
	q.Set("readMask", readMask)
	q.Set("pageSize", strconv.Itoa(clampSearchSize(size)))
	body, err := s.call(ctx, token, http.MethodGet, path, q, nil)
	if err != nil {
		return nil, nil, err
	}
	var resp searchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("contacts: decode search response: %w", err)
	}
	people := make([]person, 0, len(resp.Results))
	for _, r := range resp.Results {
		people = append(people, r.Person)
	}
	return body, people, nil
}

func (s *Service) newSearchCmd(token string) *cobra.Command {
	var query, fields string
	var max int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search My Contacts by prefix phrase (people.searchContacts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if query == "" {
				return fmt.Errorf("contacts: --query is required")
			}
			body, people, err := s.searchOnce(cmd.Context(), token, "/people:searchContacts", query, fields, max)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			if len(people) == 0 {
				fmt.Fprintf(s.stdout(), "no contacts matched %q\n", query)
				return nil
			}
			for i := range people {
				writeLine(s.stdout(), &people[i])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "prefix-phrase search query (native People API semantics)")
	cmd.Flags().IntVar(&max, "max", 10, "max results to return (API hard cap 30)")
	cmd.Flags().StringVar(&fields, "fields", defaultPersonFields, "readMask field mask (comma-separated)")
	return cmd
}
