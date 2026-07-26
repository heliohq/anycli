package hubspot

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// objectPathBase is the CRM v3 objects base for a plural object type.
func objectPathBase(plural string) string {
	return "/crm/v3/objects/" + plural
}

// searchableObjects are the object types whose group carries a search verb.
// notes and tasks reuse the generic get/list/update/delete builders but have no
// search, so a Long that points at "search" must not be printed for them.
var searchableObjects = map[string]bool{
	"contacts": true, "companies": true, "deals": true, "tickets": true,
}

// stageWriteHint is the extra write-verb clause for the two object types that
// live in a pipeline. It is per-type because "set the stage" is meaningless for
// a contact and wrong for a note.
var stageWriteHint = map[string]string{
	"deals": "\nMoving a deal along its pipeline means setting `dealstage` to a stage ID\n" +
		"from `pipeline list deals`, never the label shown in the UI.",
	"tickets": "\nMoving a ticket along its pipeline means setting its stage property to a\n" +
		"stage ID from `pipeline list tickets`, never the label shown in the UI.",
}

// The object CRUD Longs are built per (singular, plural) rather than written
// once, because the shared builders serve six object types whose siblings and
// pipeline semantics differ — contacts have an email lookup, deals and tickets
// have stages, and notes and tasks have no search verb to point at.

func longObjectGet(singular, plural string) string {
	long := "Takes the numeric record id and returns HubSpot's DEFAULT properties only;\n" +
		"name anything else — custom fields included — in `--properties a,b,c`, with\n" +
		"`property list " + plural + "` as the list of what exists."
	if singular == "contact" {
		long += "\n`--by-email` reinterprets the argument as an email address, which is often\n" +
			"the only handle available for a person."
	}
	return long
}

func longObjectList(plural string) string {
	long := "Pages through " + plural + " with no filtering or ordering of its own.\n" +
		"`--limit` sets the page size and `--after` continues from the cursor in the\n" +
		"previous response. `--archived` returns archived records instead of live\n" +
		"ones — the only way to see what `delete` removed."
	if searchableObjects[plural] {
		long += "\nFor anything selective use `search`, which takes `--filter` and `--sort`."
	} else {
		long += "\nThere is no search verb for " + plural + ", so narrowing has to happen\n" +
			"after the page comes back."
	}
	return long
}

func longObjectCreate(plural string) string {
	return "`--prop key=value` is repeatable and at least one is required. Only the\n" +
		"first `=` splits, so a value may itself contain `=`. Keys are the portal's\n" +
		"INTERNAL property names (`firstname`, `dealstage`, `amount`), not the labels\n" +
		"shown in the UI — read them from `property list " + plural + "`, because an\n" +
		"unknown name fails the whole call." + stageWriteHint[plural]
}

func longObjectUpdate(plural string) string {
	return "Only the properties named in `--prop key=value` are touched, and at least\n" +
		"one is required. There is no read-modify-write here: unnamed properties keep\n" +
		"their stored values. An unknown property name fails the whole call rather\n" +
		"than being skipped." + stageWriteHint[plural]
}

func longObjectDelete() string {
	return "ARCHIVES the record rather than destroying it: it drops out of normal reads\n" +
		"but is still returned by `list --archived`, and HubSpot purges archived\n" +
		"records on its own schedule. There is no restore verb here."
}

func longObjectSearch(plural string) string {
	return "`--filter property:operator[:value]` is repeatable and every predicate lands\n" +
		"in ONE filter group, which HubSpot ANDs — this command cannot express OR.\n" +
		"The operator is HubSpot's own (EQ, NEQ, GT, GTE, LT, LTE, BETWEEN, IN,\n" +
		"CONTAINS_TOKEN, HAS_PROPERTY, ...) and is upper-cased before it is sent;\n" +
		"HAS_PROPERTY and NOT_HAS_PROPERTY take no value. Only the first two colons\n" +
		"split the triple, so a value may contain `:`.\n" +
		"\n" +
		"`--query` is a free-text search and can be combined with the filters.\n" +
		"`--sort prop[:asc|desc]` is repeatable and ascending by default. Results\n" +
		"carry default properties only — including the property just filtered on — so\n" +
		"add `--properties` to see it. Page with `--limit` and `--after`."
}

// newObjectGroup builds a CRM object command group (contact/company/deal/ticket)
// with identical verbs. singular is the CLI command word; plural is the API path
// segment. contacts additionally support --by-email lookup on get.
func (s *Service) newObjectGroup(token, singular, plural string) *cobra.Command {
	group := newGroupCmd(singular, "Manage "+plural)
	group.AddCommand(
		s.newObjectGetCmd(token, singular, plural),
		s.newObjectListCmd(token, plural),
		s.newObjectCreateCmd(token, plural),
		s.newObjectUpdateCmd(token, plural),
		s.newObjectDeleteCmd(token, plural),
		s.newObjectSearchCmd(token, plural),
	)
	return group
}

func (s *Service) newObjectGetCmd(token, singular, plural string) *cobra.Command {
	var properties []string
	var byEmail bool
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Retrieve one " + singular + " by id",
		Long:        longObjectGet(singular, plural),
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			applyPropertiesQuery(q, properties)
			if byEmail {
				q.Set("idProperty", "email")
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, objectPathBase(plural)+"/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringSliceVar(&properties, "properties", nil, "comma-separated properties to return")
	if singular == "contact" {
		cmd.Flags().BoolVar(&byEmail, "by-email", false, "treat <id> as an email address (idProperty=email)")
	}
	return cmd
}

func (s *Service) newObjectListCmd(token, plural string) *cobra.Command {
	var properties []string
	var limit int
	var after string
	var archived bool
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List " + plural,
		Long:        longObjectList(plural),
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyPropertiesQuery(q, properties)
			applyPaging(q, limit, after)
			if archived {
				q.Set("archived", "true")
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, objectPathBase(plural), q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringSliceVar(&properties, "properties", nil, "comma-separated properties to return")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a prior response")
	cmd.Flags().BoolVar(&archived, "archived", false, "list archived records instead")
	return cmd
}

func (s *Service) newObjectCreateCmd(token, plural string) *cobra.Command {
	var props []string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a " + plural + " record",
		Long:        longObjectCreate(plural),
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			properties, err := parseProps(props)
			if err != nil {
				return err
			}
			if len(properties) == 0 {
				return &usageError{msg: "create needs at least one --prop key=value"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, objectPathBase(plural), nil, map[string]any{"properties": properties})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&props, "prop", nil, "property key=value (repeatable)")
	return cmd
}

func (s *Service) newObjectUpdateCmd(token, plural string) *cobra.Command {
	var props []string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a " + plural + " record",
		Long:        longObjectUpdate(plural),
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			properties, err := parseProps(props)
			if err != nil {
				return err
			}
			if len(properties) == 0 {
				return &usageError{msg: "update needs at least one --prop key=value"}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPatch, objectPathBase(plural)+"/"+url.PathEscape(args[0]), nil, map[string]any{"properties": properties})
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringArrayVar(&props, "prop", nil, "property key=value (repeatable)")
	return cmd
}

func (s *Service) newObjectDeleteCmd(token, plural string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Archive a " + plural + " record",
		Long:        longObjectDelete(),
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, objectPathBase(plural)+"/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// searchRequest is the CRM v3 search payload. Empty fields are omitted so a
// query-only or filter-only search is well-formed.
type searchRequest struct {
	FilterGroups []searchFilterGroup `json:"filterGroups,omitempty"`
	Query        string              `json:"query,omitempty"`
	Sorts        []searchSort        `json:"sorts,omitempty"`
	Properties   []string            `json:"properties,omitempty"`
	Limit        int                 `json:"limit,omitempty"`
	After        string              `json:"after,omitempty"`
}

type searchFilterGroup struct {
	Filters []searchFilter `json:"filters"`
}

func (s *Service) newObjectSearchCmd(token, plural string) *cobra.Command {
	var query string
	var filters []string
	var sorts []string
	var properties []string
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search " + plural + " with filters and/or a text query",
		Long:        longObjectSearch(plural),
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := searchRequest{Query: query, Limit: limit, After: after}
			// All --filter predicates go in one filterGroup (AND semantics).
			var group searchFilterGroup
			for _, raw := range filters {
				f, err := parseFilter(raw)
				if err != nil {
					return err
				}
				group.Filters = append(group.Filters, f)
			}
			if len(group.Filters) > 0 {
				req.FilterGroups = []searchFilterGroup{group}
			}
			for _, raw := range sorts {
				srt, err := parseSort(raw)
				if err != nil {
					return err
				}
				req.Sorts = append(req.Sorts, srt)
			}
			for _, p := range properties {
				if p = strings.TrimSpace(p); p != "" {
					req.Properties = append(req.Properties, p)
				}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, objectPathBase(plural)+"/search", nil, req)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "full-text search string")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "property:operator[:value] predicate (repeatable, AND)")
	cmd.Flags().StringArrayVar(&sorts, "sort", nil, "prop[:asc|desc] sort clause (repeatable)")
	cmd.Flags().StringSliceVar(&properties, "properties", nil, "comma-separated properties to return")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a prior response")
	return cmd
}
