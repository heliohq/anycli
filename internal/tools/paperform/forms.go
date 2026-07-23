package paperform

import (
	"net/url"

	"github.com/spf13/cobra"
)

// listParams holds the shared pagination + filter flags Paperform's list
// endpoints accept. Every field is optional and omitted from the query when
// unset, so the provider applies its own defaults.
type listParams struct {
	limit        int
	skip         int
	sort         string
	afterID      string
	beforeID     string
	afterDate    string
	beforeDate   string
	search       string
	searchFields string
}

// registerListFlags wires the pagination/filter flags onto a list command.
func registerListFlags(cmd *cobra.Command, p *listParams) {
	f := cmd.Flags()
	f.IntVar(&p.limit, "limit", 0, "max results to return (Paperform default 20, max 100)")
	f.IntVar(&p.skip, "skip", 0, "number of results to skip")
	f.StringVar(&p.sort, "sort", "", "sort direction: ASC or DESC (default DESC by created_at)")
	f.StringVar(&p.afterID, "after-id", "", "return results after this ID")
	f.StringVar(&p.beforeID, "before-id", "", "return results before this ID")
	f.StringVar(&p.afterDate, "after-date", "", "return results after this UTC date")
	f.StringVar(&p.beforeDate, "before-date", "", "return results before this UTC date")
	f.StringVar(&p.search, "search", "", "search forms by title")
	f.StringVar(&p.searchFields, "search-fields", "", "comma-separated fields to match --search against")
}

// query builds the URL query from the set flags, omitting any left at their
// zero value so unset flags never override provider defaults.
func (p listParams) query() url.Values {
	q := url.Values{}
	if p.limit > 0 {
		q.Set("limit", intToString(p.limit))
	}
	if p.skip > 0 {
		q.Set("skip", intToString(p.skip))
	}
	setIf(q, "sort", p.sort)
	setIf(q, "after_id", p.afterID)
	setIf(q, "before_id", p.beforeID)
	setIf(q, "after_date", p.afterDate)
	setIf(q, "before_date", p.beforeDate)
	setIf(q, "search", p.search)
	setIf(q, "search_fields", p.searchFields)
	return q
}

// newFormCmd builds the `form` group: list and get.
func (s *Service) newFormCmd(key string) *cobra.Command {
	group := newGroupCmd("form", "List and inspect forms")

	var lp listParams
	list := &cobra.Command{
		Use:   "list",
		Short: "List forms accessible by the API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runGet(cmd, key, "/forms", lp.query())
		},
	}
	registerListFlags(list, &lp)

	var formID string
	get := &cobra.Command{
		Use:   "get",
		Short: "Get a single form by slug or ID",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if formID == "" {
				return &usageError{msg: "form get: --form is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(formID), nil)
		},
	}
	get.Flags().StringVar(&formID, "form", "", "form slug or ID (required)")

	group.AddCommand(list, get)
	return group
}

// newFieldCmd builds the `field` group: list a form's fields and get one field.
func (s *Service) newFieldCmd(key string) *cobra.Command {
	group := newGroupCmd("field", "Inspect a form's fields")

	var formID string
	list := &cobra.Command{
		Use:   "list",
		Short: "List a form's fields",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if formID == "" {
				return &usageError{msg: "field list: --form is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(formID)+"/fields", nil)
		},
	}
	list.Flags().StringVar(&formID, "form", "", "form slug or ID (required)")

	var getFormID, fieldKey string
	get := &cobra.Command{
		Use:   "get",
		Short: "Get a single field by key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if getFormID == "" {
				return &usageError{msg: "field get: --form is required"}
			}
			if fieldKey == "" {
				return &usageError{msg: "field get: --key is required"}
			}
			return s.runGet(cmd, key, "/forms/"+url.PathEscape(getFormID)+"/fields/"+url.PathEscape(fieldKey), nil)
		},
	}
	get.Flags().StringVar(&getFormID, "form", "", "form slug or ID (required)")
	get.Flags().StringVar(&fieldKey, "key", "", "field key (required)")

	group.AddCommand(list, get)
	return group
}
