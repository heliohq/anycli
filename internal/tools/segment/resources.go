package segment

import (
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// pageFlags holds the pagination flags shared by every list command. They map
// to Segment's count/cursor pagination via paginationQuery.
type pageFlags struct {
	count  int
	cursor string
}

// registerPaginationFlags attaches --count / --cursor to a list command.
func registerPaginationFlags(cmd *cobra.Command, pf *pageFlags) {
	cmd.Flags().IntVar(&pf.count, "count", 0, "items per page (1-1000; Segment defaults to 200 when omitted)")
	cmd.Flags().StringVar(&pf.cursor, "cursor", "", "pagination cursor from a prior response's pagination.next")
}

// query builds the pagination query for this page.
func (pf pageFlags) query() url.Values { return paginationQuery(pf.count, pf.cursor) }

// newListCmd builds a paginated list command hitting a fixed path.
func (s *Service) newListCmd(token, use, short, path string) *cobra.Command {
	var pf pageFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, path, pf.query())
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerPaginationFlags(cmd, &pf)
	return cmd
}

// newGetByIDCmd builds a single-resource GET command reading --id and hitting
// pathFor(id).
func (s *Service) newGetByIDCmd(token, use, short, idFlag string, pathFor func(id string) string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, pathFor(url.PathEscape(id)), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, idFlag, "", "resource id (required)")
	_ = cmd.MarkFlagRequired(idFlag)
	return cmd
}

// newListByIDCmd builds a paginated sub-list command reading --id and hitting
// pathFor(id) (e.g. a source's connected destinations, a space's audiences).
func (s *Service) newListByIDCmd(token, use, short, idFlag string, pathFor func(id string) string) *cobra.Command {
	var id string
	var pf pageFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, pathFor(url.PathEscape(id)), pf.query())
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&id, idFlag, "", "resource id (required)")
	_ = cmd.MarkFlagRequired(idFlag)
	registerPaginationFlags(cmd, &pf)
	return cmd
}

// --- Workspace ---

func (s *Service) newWorkspaceGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get",
		Short:       "Get the current workspace (also the identity endpoint)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.get(cmd.Context(), token, "/", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// --- Sources ---

func (s *Service) newSourceListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List sources", "/sources")
}

func (s *Service) newSourceGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one source", "id",
		func(id string) string { return "/sources/" + id })
}

func (s *Service) newSourceConnectedDestinationsCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "connected-destinations", "List a source's connected destinations", "id",
		func(id string) string { return "/sources/" + id + "/connected-destinations" })
}

// --- Destinations ---

func (s *Service) newDestinationListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List destinations", "/destinations")
}

func (s *Service) newDestinationGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one destination", "id",
		func(id string) string { return "/destinations/" + id })
}

// --- Warehouses ---

func (s *Service) newWarehouseListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List warehouses", "/warehouses")
}

func (s *Service) newWarehouseGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one warehouse", "id",
		func(id string) string { return "/warehouses/" + id })
}

// --- Tracking plans ---

func (s *Service) newTrackingPlanListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List tracking plans", "/tracking-plans")
}

func (s *Service) newTrackingPlanGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one tracking plan", "id",
		func(id string) string { return "/tracking-plans/" + id })
}

func (s *Service) newTrackingPlanRulesCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "rules", "List a tracking plan's rules", "id",
		func(id string) string { return "/tracking-plans/" + id + "/rules" })
}

// --- Functions ---

func (s *Service) newFunctionListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List functions", "/functions")
}

// --- Spaces (Unify) ---

func (s *Service) newSpaceListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List Unify spaces", "/spaces")
}

func (s *Service) newSpaceAudiencesCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "audiences", "List a space's audiences", "id",
		func(id string) string { return "/spaces/" + id + "/audiences" })
}

// --- IAM ---
//
// The REST paths are /users and /groups (NOT /iam/users), verified against the
// official Segment public-api-sdk-go; the "IAM" grouping is a docs tag, not a
// URL prefix. The `iam` CLI group is a UX affordance mirroring that tag.

func (s *Service) newIAMUserListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List IAM users", "/users")
}

func (s *Service) newIAMGroupListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List IAM user groups", "/groups")
}

// --- Delivery / observability ---

// newEventsVolumeCmd wraps the workspace-scoped GET /events/volume: the whole
// workspace's event volume over time. Convenience flags map to the
// recipe-confirmed query params (granularity/startTime/endTime); --param passes
// any additional query pair through unchanged (exact filter names are L2-gated).
func (s *Service) newEventsVolumeCmd(token string) *cobra.Command {
	var granularity, start, end string
	var params []string
	cmd := &cobra.Command{
		Use:         "events-volume",
		Short:       "Workspace event volume over time (GET /events/volume)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := parseParams(params)
			if err != nil {
				return err
			}
			if granularity != "" {
				q.Set("granularity", granularity)
			}
			if start != "" {
				q.Set("startTime", start)
			}
			if end != "" {
				q.Set("endTime", end)
			}
			body, err := s.get(cmd.Context(), token, "/events/volume", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&granularity, "granularity", "", "time bucket, e.g. HOUR or DAY")
	cmd.Flags().StringVar(&start, "start", "", "start time (ISO-8601), sent as startTime")
	cmd.Flags().StringVar(&end, "end", "", "end time (ISO-8601), sent as endTime")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra query param as name=value (repeatable)")
	return cmd
}

// newDeliveryMetricsCmd wraps the destination-scoped GET
// /destinations/{id}/delivery-metrics: a delivery metrics summary for one
// destination. The associated source and time window are supplied as --param
// query pairs (exact names are L2-gated).
func (s *Service) newDeliveryMetricsCmd(token string) *cobra.Command {
	var destinationID string
	var params []string
	cmd := &cobra.Command{
		Use:         "metrics",
		Short:       "Delivery metrics summary for a destination (GET /destinations/{id}/delivery-metrics)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := parseParams(params)
			if err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token,
				"/destinations/"+url.PathEscape(destinationID)+"/delivery-metrics", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&destinationID, "destination-id", "", "destination id (required)")
	_ = cmd.MarkFlagRequired("destination-id")
	cmd.Flags().StringArrayVar(&params, "param", nil, "extra query param as name=value (repeatable; e.g. sourceId, granularity)")
	return cmd
}

// parseParams turns repeatable name=value flags into url.Values. A pair missing
// the '=' is a usage error.
func parseParams(pairs []string) (url.Values, error) {
	q := url.Values{}
	for _, p := range pairs {
		name, val, ok := strings.Cut(p, "=")
		if !ok || name == "" {
			return nil, &usageError{msg: "--param must be name=value, got " + p}
		}
		q.Add(name, val)
	}
	return q, nil
}
