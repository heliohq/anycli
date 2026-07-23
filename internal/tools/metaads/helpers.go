package metaads

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func itoa(v int) string { return strconv.Itoa(v) }

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

// edgeListFlags are the paging/selection flags shared by every act_<id> edge
// list command (campaigns, ad sets, ads, creatives).
type edgeListFlags struct {
	account string
	fields  string
	limit   int
	after   string
}

func (f *edgeListFlags) bind(cmd *cobra.Command, defaultFields string) {
	cmd.Flags().StringVar(&f.account, "account", "", "ad account id in act_<number> form (required)")
	cmd.Flags().StringVar(&f.fields, "fields", defaultFields, "comma-separated fields to return")
	cmd.Flags().IntVar(&f.limit, "limit", 50, "maximum objects in this page (1-500)")
	cmd.Flags().StringVar(&f.after, "after", "", "paging cursor from a previous page")
}

// query validates the shared flags and builds the base query values. Callers
// add any resource-specific filters to the returned url.Values.
func (f *edgeListFlags) query() (url.Values, error) {
	if err := requireAccountID(f.account); err != nil {
		return nil, err
	}
	if err := requireLimit(f.limit, 1, 500); err != nil {
		return nil, err
	}
	values := url.Values{
		"fields": {f.fields},
		"limit":  {itoa(f.limit)},
	}
	if f.after != "" {
		values.Set("after", f.after)
	}
	return values, nil
}

// listEdge runs a validated GET against /act_<id>/<edge> and emits the body.
func (s *Service) listEdge(cmd *cobra.Command, token, edge string, f *edgeListFlags, extra map[string]string) error {
	values, err := f.query()
	if err != nil {
		return err
	}
	for key, value := range extra {
		if value != "" {
			values.Set(key, value)
		}
	}
	body, err := s.get(cmd.Context(), token, "/"+f.account+"/"+edge, values)
	if err != nil {
		return err
	}
	return s.emit(body)
}

// getObject runs a validated GET against a single object node.
func (s *Service) getObject(cmd *cobra.Command, token, name, id, fields string) error {
	if err := requireObjectID(name, id); err != nil {
		return err
	}
	query := url.Values{}
	if fields != "" {
		query.Set("fields", fields)
	}
	body, err := s.get(cmd.Context(), token, "/"+id, query)
	if err != nil {
		return err
	}
	return s.emit(body)
}
