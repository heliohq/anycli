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
		Use:   "get <id>",
		Short: "Retrieve one " + singular + " by id",
		Args:  cobra.ExactArgs(1),
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
		Use:   "list",
		Short: "List " + plural,
		Args:  cobra.NoArgs,
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
		Use:   "create",
		Short: "Create a " + plural + " record",
		Args:  cobra.NoArgs,
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
		Use:   "update <id>",
		Short: "Update a " + plural + " record",
		Args:  cobra.ExactArgs(1),
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
		Use:   "delete <id>",
		Short: "Archive a " + plural + " record",
		Args:  cobra.ExactArgs(1),
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
		Use:   "search",
		Short: "Search " + plural + " with filters and/or a text query",
		Args:  cobra.NoArgs,
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
