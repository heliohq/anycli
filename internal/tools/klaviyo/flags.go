package klaviyo

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// listFlags holds the JSON:API query flags shared by every collection endpoint.
// resourceType names the primary resource so --fields maps to the correct
// sparse-fieldset param (fields[<type>]). Every mapping is a straight
// passthrough — anycli never interprets Klaviyo's filter/sort grammar.
type listFlags struct {
	filter   string
	sort     string
	cursor   string
	pageSize int
	include  string
	fields   string
	params   []string
}

// registerListFlags attaches the shared collection query flags to cmd.
func registerListFlags(cmd *cobra.Command, f *listFlags) {
	cmd.Flags().StringVar(&f.filter, "filter", "", "JSON:API filter expression, e.g. equals(email,\"x@y.com\")")
	cmd.Flags().StringVar(&f.sort, "sort", "", "sort field, e.g. -created")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor (from a prior response's links.next)")
	cmd.Flags().IntVar(&f.pageSize, "page-size", 0, "page size (1-100)")
	cmd.Flags().StringVar(&f.include, "include", "", "comma-separated related resources to include")
	cmd.Flags().StringVar(&f.fields, "fields", "", "comma-separated sparse fieldset for the primary resource")
	cmd.Flags().StringArrayVar(&f.params, "param", nil, "extra raw query parameter name=value (repeatable)")
}

// query builds the url.Values for a collection request from the shared flags.
// resourceType is the JSON:API type used for the sparse-fieldset param.
func (f *listFlags) query(resourceType string) (url.Values, error) {
	q := url.Values{}
	if f.filter != "" {
		q.Set("filter", f.filter)
	}
	if f.sort != "" {
		q.Set("sort", f.sort)
	}
	if f.cursor != "" {
		q.Set("page[cursor]", f.cursor)
	}
	if f.pageSize != 0 {
		if f.pageSize < 1 || f.pageSize > 100 {
			return nil, &usageError{msg: fmt.Sprintf("--page-size must be between 1 and 100, got %d", f.pageSize)}
		}
		q.Set("page[size]", strconv.Itoa(f.pageSize))
	}
	if f.include != "" {
		q.Set("include", f.include)
	}
	if f.fields != "" {
		q.Set("fields["+resourceType+"]", f.fields)
	}
	for _, p := range f.params {
		name, value, ok := strings.Cut(p, "=")
		if !ok || name == "" {
			return nil, &usageError{msg: fmt.Sprintf("--param must be name=value, got %q", p)}
		}
		q.Add(name, value)
	}
	return q, nil
}
