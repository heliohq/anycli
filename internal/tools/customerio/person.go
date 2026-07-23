package customerio

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newPersonSearchCmd(key string) *cobra.Command {
	var email, filter, start string
	var limit int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Find a person by email (GET /v1/customers) or filter people by segment/attributes (POST /v1/customers)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (email == "") == (filter == "") {
				return &usageError{msg: "exactly one of --email or --filter is required"}
			}
			q := url.Values{}
			if start != "" {
				q.Set("start", start)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if email != "" {
				q.Set("email", email)
				resp, err := s.call(cmd, key, http.MethodGet, "/v1/customers", q, nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			}
			body, err := decodeJSONFlag("filter", filter)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd, key, http.MethodPost, "/v1/customers", q, map[string]any{"filter": body})
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "exact email lookup")
	cmd.Flags().StringVar(&filter, "filter", "", "segment/attribute filter as raw JSON (Customer.io filter object)")
	cmd.Flags().StringVar(&start, "start", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (0 = provider default)")
	return cmd
}

// personIDCmd builds the person subcommands that read one person's sub-resource
// by id (attributes, segments, messages, activities), sharing the --id /
// --id-type flags and optional pagination.
func (s *Service) newPersonGetCmd(key string) *cobra.Command {
	var id, idType string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a person's profile attributes (GET /v1/customers/{id}/attributes)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIDType(q, idType)
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/customers/"+url.PathEscape(id)+"/attributes", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPersonIDFlags(cmd, &id, &idType)
	return cmd
}

func (s *Service) newPersonSegmentsCmd(key string) *cobra.Command {
	var id, idType string
	cmd := &cobra.Command{
		Use:         "segments",
		Short:       "List the segments a person is in (GET /v1/customers/{id}/segments)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIDType(q, idType)
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/customers/"+url.PathEscape(id)+"/segments", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPersonIDFlags(cmd, &id, &idType)
	return cmd
}

func (s *Service) newPersonMessagesCmd(key string) *cobra.Command {
	var id, idType, start string
	var limit int
	cmd := &cobra.Command{
		Use:         "messages",
		Short:       "List a person's delivery history (GET /v1/customers/{id}/messages)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIDType(q, idType)
			if start != "" {
				q.Set("start", start)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/customers/"+url.PathEscape(id)+"/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPersonIDFlags(cmd, &id, &idType)
	cmd.Flags().StringVar(&start, "start", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (0 = provider default)")
	return cmd
}

func (s *Service) newPersonActivitiesCmd(key string) *cobra.Command {
	var id, idType, activityType, start string
	var limit int
	cmd := &cobra.Command{
		Use:         "activities",
		Short:       "List a person's activity log (GET /v1/customers/{id}/activities)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIDType(q, idType)
			if activityType != "" {
				q.Set("type", activityType)
			}
			if start != "" {
				q.Set("start", start)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/customers/"+url.PathEscape(id)+"/activities", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPersonIDFlags(cmd, &id, &idType)
	cmd.Flags().StringVar(&activityType, "type", "", "activity type filter (e.g. event, attribute_change)")
	cmd.Flags().StringVar(&start, "start", "", "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "max results per page (0 = provider default)")
	return cmd
}

// registerPersonIDFlags wires the shared --id (required) and --id-type flags.
func registerPersonIDFlags(cmd *cobra.Command, id, idType *string) {
	cmd.Flags().StringVar(id, "id", "", "customer id (or email / cio_id per --id-type)")
	cmd.Flags().StringVar(idType, "id-type", "id", "how to resolve --id: id|email|cio_id")
	_ = cmd.MarkFlagRequired("id")
}
