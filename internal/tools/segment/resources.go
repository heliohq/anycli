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
func (s *Service) newListCmd(token, use, short, long, path string) *cobra.Command {
	var pf pageFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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
func (s *Service) newGetByIDCmd(token, use, short, long, idFlag string, pathFor func(id string) string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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
func (s *Service) newListByIDCmd(token, use, short, long, idFlag string, pathFor func(id string) string) *cobra.Command {
	var id string
	var pf pageFlags
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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

// The Longs below sit next to the shared list/get builders because it is the
// builder — not the call site — that fixes the --id flag, the pagination flags
// and the response envelope each of these commands describes.
const (
	longSourceList = "Sources are the data inputs: one per app, website, server or cloud\n" +
		"integration feeding the workspace. Each row's `id` is what every other\n" +
		"source-scoped command takes, and `enabled` distinguishes a source that is\n" +
		"merely configured from one actually ingesting. It does NOT show where a\n" +
		"source's events go — `source connected-destinations` does."

	longSourceGet = "Returns one source's settings and its `metadata` block, which names the\n" +
		"source TYPE (the catalog entry it was created from) — the thing that\n" +
		"determines which event properties it can emit at all. `--id` is required and\n" +
		"is a source id, not a source name or write key."

	longSourceConnectedDestinations = "The only read in this tool that shows the WIRING: which destinations this one\n" +
		"source actually forwards to. Neither `source list` nor `destination list`\n" +
		"carries that edge, so answering \"where do this app's events end up\" starts\n" +
		"here. A source with no connected destinations is ingesting into nothing."

	longDestinationList = "Destinations are the data outputs — the downstream tools events are forwarded\n" +
		"to. Each row carries its `enabled` state and its `sourceId`, so a\n" +
		"destination is scoped to one source rather than the workspace. Warehouses\n" +
		"are a separate object type and are NOT included here; use `warehouse list`."

	longDestinationGet = "One destination's full configuration, including the `settings` object holding\n" +
		"its per-destination mapping and filter setup — which is what explains why\n" +
		"events arrive transformed or not at all. `--id` is required. For whether it\n" +
		"is currently receiving anything, `delivery metrics` is the read to follow\n" +
		"with."

	longWarehouseList = "Warehouses are SQL sinks (Redshift, BigQuery, Snowflake, Postgres) that\n" +
		"Segment loads on a sync schedule rather than streaming to in real time, so\n" +
		"they lag the destinations by design. They are a distinct object type from\n" +
		"destinations and never appear in `destination list`."

	longWarehouseGet = "One warehouse's connection settings and state. `--id` is required. Sync\n" +
		"history and per-run outcomes are not exposed as a first-class command — they\n" +
		"are reachable through `request` against the warehouse's own subpaths if\n" +
		"needed."

	longTrackingPlanList = "A tracking plan is the governance contract: the schema events and their\n" +
		"properties are expected to conform to. Listing plans says which contracts\n" +
		"exist, not which sources are held to them or whether anything is violating\n" +
		"them. The rules themselves are a separate call, `tracking-plan rules`."

	longTrackingPlanGet = "The plan's own record — name, type and description — WITHOUT its rules, which\n" +
		"is the part that actually defines the schema. Call `tracking-plan rules` for\n" +
		"those; this alone will not tell you what any event is required to look like.\n" +
		"`--id` is required."

	longTrackingPlanRules = "The rules ARE the schema: one entry per event, each carrying a JSON Schema\n" +
		"for that event's properties and which of them are required. This is the read\n" +
		"that answers \"what shape is this event supposed to be\". Paginated, and a\n" +
		"mature plan has many rules, so page rather than assuming one call is the\n" +
		"whole contract."

	longFunctionList = "Functions are the workspace's custom JavaScript — source functions that ingest\n" +
		"and destination functions that transform on the way out. The list carries\n" +
		"each function's `resourceType`, which is what distinguishes the two.\n" +
		"Function CODE is not returned here, and there is no get verb for it."

	longSpaceList = "A Unify space is the identity-resolution layer sitting above sources — where\n" +
		"profiles and computed traits live — and it is a separate paid product from\n" +
		"the connections plane. A workspace with no Unify entitlement returns an\n" +
		"empty list rather than an error. Each `id` here is what `space audiences`\n" +
		"takes."

	longSpaceAudiences = "Audiences are the computed segments built on a space's profiles. `--id` is a\n" +
		"SPACE id from `space list`, not a source or workspace id. Each row carries\n" +
		"the audience's definition and enabled state; membership counts and the\n" +
		"members themselves are not exposed by this command."

	longIAMUserList = "Every human with access to the workspace, with the permissions attached to\n" +
		"each. The underlying REST path is `/users` — the `iam` grouping here mirrors\n" +
		"Segment's docs tag and is not a URL prefix, which matters when hand-writing\n" +
		"a `request --path`. Invited-but-unaccepted users appear here too."

	longIAMGroupList = "User groups are how permissions are granted in bulk rather than per person,\n" +
		"so a user whose own record looks unprivileged may still have access through\n" +
		"a group. The underlying REST path is `/groups`, not `/iam/groups`. Group\n" +
		"MEMBERSHIP is not returned here; reach it through `request`."
)

// --- Workspace ---

func (s *Service) newWorkspaceGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get the current workspace (also the identity endpoint)",
		Long: "The identity call: it names which workspace the connected token belongs to,\n" +
			"which is worth confirming before reading anything else, since the token alone\n" +
			"carries no visible hint. It takes no arguments and is the cheapest check that\n" +
			"the credential works and that the workspace has Public API access at all.",
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
	return s.newListCmd(token, "list", "List sources", longSourceList, "/sources")
}

func (s *Service) newSourceGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one source", longSourceGet, "id",
		func(id string) string { return "/sources/" + id })
}

func (s *Service) newSourceConnectedDestinationsCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "connected-destinations", "List a source's connected destinations", longSourceConnectedDestinations, "id",
		func(id string) string { return "/sources/" + id + "/connected-destinations" })
}

// --- Destinations ---

func (s *Service) newDestinationListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List destinations", longDestinationList, "/destinations")
}

func (s *Service) newDestinationGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one destination", longDestinationGet, "id",
		func(id string) string { return "/destinations/" + id })
}

// --- Warehouses ---

func (s *Service) newWarehouseListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List warehouses", longWarehouseList, "/warehouses")
}

func (s *Service) newWarehouseGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one warehouse", longWarehouseGet, "id",
		func(id string) string { return "/warehouses/" + id })
}

// --- Tracking plans ---

func (s *Service) newTrackingPlanListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List tracking plans", longTrackingPlanList, "/tracking-plans")
}

func (s *Service) newTrackingPlanGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "get", "Get one tracking plan", longTrackingPlanGet, "id",
		func(id string) string { return "/tracking-plans/" + id })
}

func (s *Service) newTrackingPlanRulesCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "rules", "List a tracking plan's rules", longTrackingPlanRules, "id",
		func(id string) string { return "/tracking-plans/" + id + "/rules" })
}

// --- Functions ---

func (s *Service) newFunctionListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List functions", longFunctionList, "/functions")
}

// --- Spaces (Unify) ---

func (s *Service) newSpaceListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List Unify spaces", longSpaceList, "/spaces")
}

func (s *Service) newSpaceAudiencesCmd(token string) *cobra.Command {
	return s.newListByIDCmd(token, "audiences", "List a space's audiences", longSpaceAudiences, "id",
		func(id string) string { return "/spaces/" + id + "/audiences" })
}

// --- IAM ---
//
// The REST paths are /users and /groups (NOT /iam/users), verified against the
// official Segment public-api-sdk-go; the "IAM" grouping is a docs tag, not a
// URL prefix. The `iam` CLI group is a UX affordance mirroring that tag.

func (s *Service) newIAMUserListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List IAM users", longIAMUserList, "/users")
}

func (s *Service) newIAMGroupListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "list", "List IAM user groups", longIAMGroupList, "/groups")
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
		Use:   "events-volume",
		Short: "Workspace event volume over time (GET /events/volume)",
		Long: "Workspace-wide, not per source or per destination — this is the \"is data\n" +
			"flowing at all\" read, and a drop here is an ingestion problem rather than a\n" +
			"delivery one. `--granularity` buckets the series (`HOUR`, `DAY`), `--start`\n" +
			"and `--end` are ISO-8601 and are sent as `startTime` / `endTime`. Any further\n" +
			"filter rides `--param name=value`, repeatable and passed through unchanged.\n" +
			"For one destination's health, `delivery metrics` is the right call instead.",
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
		Use:   "metrics",
		Short: "Delivery metrics summary for a destination (GET /destinations/{id}/delivery-metrics)",
		Long: "Scoped to ONE destination, which `--destination-id` requires. This is where a\n" +
			"delivery failure shows up — events accepted by Segment but rejected or dropped\n" +
			"downstream — as opposed to `delivery events-volume`, which only shows whether\n" +
			"anything arrived in the first place. Narrow it with `--param sourceId=<id>`\n" +
			"when several sources feed the same destination, since the unfiltered summary\n" +
			"merges them.",
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
