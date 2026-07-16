package contacts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// sortOrders maps the --sort flag to the People API sortOrder enum. An unset
// flag leaves sortOrder off (API default LAST_MODIFIED_ASCENDING).
var sortOrders = map[string]string{
	"last-modified": "LAST_MODIFIED_DESCENDING",
	"first-name":    "FIRST_NAME_ASCENDING",
	"last-name":     "LAST_NAME_ASCENDING",
}

func (s *Service) newListCmd(token string) *cobra.Command {
	var pageToken, sort, fields string
	var max int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the user's contacts (people.connections.list)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sort != "" {
				if _, ok := sortOrders[sort]; !ok {
					return fmt.Errorf("contacts: --sort must be last-modified, first-name, or last-name, got %q", sort)
				}
			}
			q := url.Values{}
			q.Set("personFields", fields)
			q.Set("pageSize", strconv.Itoa(max))
			if sort != "" {
				q.Set("sortOrder", sortOrders[sort])
			}
			if pageToken != "" {
				q.Set("pageToken", pageToken)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/people/me/connections", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Connections   []person `json:"connections"`
				NextPageToken string   `json:"nextPageToken"`
				TotalItems    int      `json:"totalItems"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("contacts: decode connections: %w", err)
			}
			if len(resp.Connections) == 0 {
				fmt.Fprintln(s.stdout(), "no contacts")
				return nil
			}
			for i := range resp.Connections {
				writeLine(s.stdout(), &resp.Connections[i])
			}
			if resp.NextPageToken != "" {
				fmt.Fprintf(s.stdout(), "next page token: %s\n", resp.NextPageToken)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&max, "max", 100, "max results to return (1-1000)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "page token from a previous list call")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order: last-modified, first-name, or last-name")
	cmd.Flags().StringVar(&fields, "fields", defaultPersonFields, "personFields mask (comma-separated)")
	return cmd
}

// cleanResourceNames splits every multi-value arg on whitespace and drops
// empties, so `get "people/c1 people/c2"` and `get people/c1 people/c2` both
// work and stray \r from pipelines never reaches the API.
func cleanResourceNames(args []string) ([]string, error) {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		names = append(names, strings.Fields(arg)...)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("contacts: no resource names given")
	}
	return names, nil
}

func (s *Service) newGetCmd(token string) *cobra.Command {
	var fields string
	cmd := &cobra.Command{
		Use:   "get <resource-name>...",
		Short: "Fetch one or more contacts by resource name (people.get / getBatchGet)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := cleanResourceNames(args)
			if err != nil {
				return err
			}
			if len(names) == 1 {
				q := url.Values{}
				q.Set("personFields", fields)
				body, err := s.call(cmd.Context(), token, http.MethodGet, "/"+names[0], q, nil)
				if err != nil {
					return err
				}
				if jsonOut(cmd) {
					return s.emit(body)
				}
				var p person
				if err := json.Unmarshal(body, &p); err != nil {
					return fmt.Errorf("contacts: decode person: %w", err)
				}
				renderPerson(s.stdout(), &p)
				return nil
			}
			q := url.Values{}
			for _, n := range names {
				q.Add("resourceNames", n)
			}
			q.Set("personFields", fields)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/people:batchGet", q, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emit(body)
			}
			var resp struct {
				Responses []struct {
					Person person `json:"person"`
				} `json:"responses"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("contacts: decode batch get: %w", err)
			}
			for i := range resp.Responses {
				writeLine(s.stdout(), &resp.Responses[i].Person)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fields, "fields", defaultPersonFields, "personFields mask (comma-separated)")
	return cmd
}

// renderPerson prints a multi-line detail view for a single contact.
func renderPerson(w io.Writer, p *person) {
	fmt.Fprintf(w, "Name:         %s\n", p.displayName())
	fmt.Fprintf(w, "ResourceName: %s\n", p.ResourceName)
	if e := p.emails(); len(e) > 0 {
		fmt.Fprintf(w, "Emails:       %s\n", strings.Join(e, ", "))
	}
	if ph := p.phones(); len(ph) > 0 {
		fmt.Fprintf(w, "Phones:       %s\n", strings.Join(ph, ", "))
	}
	if org := p.organization(); org != "" {
		fmt.Fprintf(w, "Organization: %s\n", org)
	}
}
