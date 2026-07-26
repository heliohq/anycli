package rocketreach

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newPersonLookupCmd is `person lookup` (GET /api/v2/person/lookup): enrich a
// known person into verified emails/phones. Identify the person by name +
// current employer, by LinkedIn URL, or by a profile id from a prior search.
// The lookup is asynchronous — the response carries a `status`
// (complete/searching/waiting/progress/failed); when it is not yet complete the
// agent polls with `person status`. Credits are charged only on a match.
func (s *Service) newPersonLookupCmd(key string) *cobra.Command {
	var name, currentEmployer, linkedinURL, id string
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Enrich a person into emails/phones (GET /person/lookup)",
		Long: "Identify the person one of three ways: --id (a profile id from `person\n" +
			"search`, the most precise), --linkedin-url, or --name optionally narrowed\n" +
			"by --current-employer. They are checked in that order and the first one\n" +
			"set wins silently, so passing --id alongside --name means the name is\n" +
			"ignored.\n" +
			"\n" +
			"The response is NOT final. Read its `status`: only `complete` carries\n" +
			"populated emails and phones, while searching, waiting or progress mean the\n" +
			"enrichment is still running and the record must be polled with `person\n" +
			"status --ids`. Credits are charged only when RocketReach matches someone.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			switch {
			case id != "":
				q.Set("id", id)
			case linkedinURL != "":
				q.Set("linkedin_url", linkedinURL)
			case name != "":
				q.Set("name", name)
				if currentEmployer != "" {
					q.Set("current_employer", currentEmployer)
				}
			default:
				return &usageError{msg: "provide one of --id, --linkedin-url, or --name (optionally with --current-employer)"}
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/api/v2/person/lookup", q, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "person's full name (pair with --current-employer)")
	cmd.Flags().StringVar(&currentEmployer, "current-employer", "", "current employer, to disambiguate --name")
	cmd.Flags().StringVar(&linkedinURL, "linkedin-url", "", "person's LinkedIn profile URL")
	cmd.Flags().StringVar(&id, "id", "", "RocketReach profile id from a prior search")
	return cmd
}

// newPersonStatusCmd is `person status` (GET /api/v2/person/checkStatus): poll
// one or more asynchronous lookups by id. RocketReach recommends webhooks over
// polling, but the runtime has no inbound endpoint, so polling is the
// agent-usable path.
func (s *Service) newPersonStatusCmd(key string) *cobra.Command {
	var ids string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Poll async lookups by id (GET /person/checkStatus)",
		Long: "--ids is required and takes a COMMA-SEPARATED list, so several in-flight\n" +
			"lookups can be polled in a single request instead of one call each. The\n" +
			"ids are the ones returned by `person lookup`, not profile ids from `person\n" +
			"search`. Poll until the status reads complete; polling costs no additional\n" +
			"credits.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("ids", ids)
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/api/v2/person/checkStatus", q, nil)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&ids, "ids", "", "comma-separated lookup ids to poll (required)")
	_ = cmd.MarkFlagRequired("ids")
	return cmd
}

// newPersonSearchCmd is `person search` (POST /api/v2/person/search): find
// people by role/company/location to build a prospect list. Returns matching
// profiles (name/title/employer/profile id) but no contact info — the agent
// then `person lookup --id <profileId>` to enrich a chosen result. Common
// filters are repeatable-value flags; --json-query is the escape hatch for the
// full RocketReach query object.
func (s *Service) newPersonSearchCmd(key string) *cobra.Command {
	var name, currentEmployer, title, jsonQuery string
	var pageSize, start int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search people to build a prospect list (POST /person/search)",
		Long: "Returns profiles — name, title, employer and a profile id — and never\n" +
			"emails or phones; enrich a chosen id with `person lookup`. At least one of\n" +
			"--name, --current-employer, --title or --json-query is required.\n" +
			"--json-query is the full RocketReach query object, whose fields are ARRAYS\n" +
			"of strings, and an explicit flag overwrites the same key inside it. Page\n" +
			"with --page-size and a 1-based --start; both are omitted from the request\n" +
			"when left at 0, leaving RocketReach's defaults.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := map[string]any{}
			if jsonQuery != "" {
				decoded, err := decodeJSONQuery(jsonQuery)
				if err != nil {
					return err
				}
				query = decoded
			}
			addStringFilter(query, "name", name)
			addStringFilter(query, "current_employer", currentEmployer)
			addStringFilter(query, "current_title", title)
			if len(query) == 0 {
				return &usageError{msg: "provide at least one filter (--name, --current-employer, --title, or --json-query)"}
			}
			payload := map[string]any{"query": query}
			if pageSize > 0 {
				payload["page_size"] = pageSize
			}
			if start > 0 {
				payload["start"] = start
			}
			body, err := s.call(cmd.Context(), key, http.MethodPost, "/api/v2/person/search", nil, payload)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "match on person name")
	cmd.Flags().StringVar(&currentEmployer, "current-employer", "", "match on current employer")
	cmd.Flags().StringVar(&title, "title", "", "match on current job title")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "max results per page")
	cmd.Flags().IntVar(&start, "start", 0, "1-based result offset for pagination")
	cmd.Flags().StringVar(&jsonQuery, "json-query", "", "raw RocketReach query object as JSON (escape hatch)")
	return cmd
}

// addStringFilter sets key to a single-element string array (RocketReach's
// search query fields are arrays of strings) when val is non-empty. An explicit
// flag overrides any same-key value from --json-query.
func addStringFilter(query map[string]any, key, val string) {
	if val != "" {
		query[key] = []string{val}
	}
}
