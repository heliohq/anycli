package serpapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// readOnly marks a leaf command as side-effect-free for the design-318 approval
// gate. Every SerpApi command is a search/read GET, so all leaves carry it.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// newSearchCmd builds `serpapi search`: one generic command over every SerpApi
// engine. The cross-engine common params are first-class flags; engine-specific
// params ride the repeatable `--param key=value` escape hatch. `--engine`
// passes through unvalidated (SerpApi adds engines continuously — an unknown
// engine fails with the provider's own error, not a stale local whitelist).
func (s *Service) newSearchCmd(apiKey string) *cobra.Command {
	var (
		query, engine, location, gl, hl, googleDomain, device string
		num, start                                            int
		noCache                                               bool
		params                                                []string
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Run a live search (GET /search); --engine selects the vertical",
		Long: "The only command here that COSTS a search from the plan's quota, so check\n" +
			"`account` before a batch and reach for `archive get` to re-read anything\n" +
			"already fetched. `--engine` defaults to `google` and is the one param always\n" +
			"sent; every other flag is omitted from the request unless explicitly set, so\n" +
			"unset flags never force a provider default.\n" +
			"\n" +
			"`--location` must be a CANONICAL SerpApi location name — a raw city string\n" +
			"usually matches nothing — so resolve it through `locations` first and copy\n" +
			"`canonical_name`. `--gl` and `--hl` are the country and language codes,\n" +
			"`--num` the result count and `--start` the pagination offset. `--no-cache`\n" +
			"forces a fresh crawl instead of SerpApi's cached copy, and costs quota either\n" +
			"way.\n" +
			"\n" +
			"`--param key=value` is applied AFTER the first-class flags, so it overrides\n" +
			"one of the same name — but never the injected `api_key`, which is set later\n" +
			"and cannot be displaced. The response's `search_metadata.id` is what\n" +
			"`archive get` later takes.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			// engine always ships (has a default); the rest ship only when set.
			q.Set("engine", engine)
			flags := cmd.Flags()
			setIfChanged(flags, q, "q", query, "q")
			setIfChanged(flags, q, "location", location, "location")
			setIfChanged(flags, q, "gl", gl, "gl")
			setIfChanged(flags, q, "hl", hl, "hl")
			setIfChanged(flags, q, "google-domain", googleDomain, "google_domain")
			setIfChanged(flags, q, "device", device, "device")
			if flags.Changed("num") {
				q.Set("num", strconv.Itoa(num))
			}
			if flags.Changed("start") {
				q.Set("start", strconv.Itoa(start))
			}
			if flags.Changed("no-cache") {
				q.Set("no_cache", strconv.FormatBool(noCache))
			}
			// The --param escape hatch is applied last so it overrides a
			// first-class flag of the same name. api_key is protected in get():
			// the resolved credential is set there after this map is built, so a
			// `--param api_key=...` can never take effect.
			if err := applyParams(q, params); err != nil {
				return err
			}
			if err := requireKey(apiKey); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), apiKey, "/search", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&query, "q", "q", "", "search query (engine-dependent; e.g. keywords for google)")
	f.StringVar(&engine, "engine", "google", "SerpApi engine (google, google_news, google_maps, youtube, …)")
	f.StringVar(&location, "location", "", "canonical location name (resolve via `serpapi locations`)")
	f.StringVar(&gl, "gl", "", "two-letter country code")
	f.StringVar(&hl, "hl", "", "two-letter language code")
	f.StringVar(&googleDomain, "google-domain", "", "Google domain (e.g. google.com)")
	f.StringVar(&device, "device", "", "device: desktop, tablet, or mobile")
	f.IntVar(&num, "num", 0, "number of results")
	f.IntVar(&start, "start", 0, "result offset for pagination")
	f.BoolVar(&noCache, "no-cache", false, "force a fresh search, bypassing SerpApi's cache")
	f.StringArrayVar(&params, "param", nil, "engine-specific param as key=value (repeatable)")
	return cmd
}

// newArchiveCmd builds `serpapi archive get <search_id>`: a free re-read of a
// prior search from the Search Archive API (within 31 days), spending no quota.
func (s *Service) newArchiveCmd(apiKey string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Search Archive API (free re-read of a prior search)",
	}
	get := &cobra.Command{
		Use:   "get <search_id>",
		Short: "Fetch an archived search by id (GET /searches/<id>.json)",
		Long: "Takes the `search_metadata.id` from a previous `search` response and returns\n" +
			"that exact result again, free and without spending quota, for up to 31 days.\n" +
			"Prefer it to re-running a query whose answer is already in hand — the results\n" +
			"are the ones originally paid for, frozen, which also makes them stable to\n" +
			"cite. A search older than the retention window is simply gone; re-running is\n" +
			"then the only option.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireKey(apiKey); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), apiKey, "/searches/"+args[0]+".json", nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.AddCommand(get)
	return cmd
}

// newLocationsCmd builds `serpapi locations`: the free, unauthenticated
// Locations API that resolves a human place name to the canonical_name the
// search `--location` param requires. It sends no api_key.
func (s *Service) newLocationsCmd() *cobra.Command {
	var query string
	var limit int
	cmd := &cobra.Command{
		Use:   "locations",
		Short: "Resolve a place name to a canonical location (free, no credential)",
		Long: "The prerequisite for `search --location`, which needs SerpApi's canonical\n" +
			"name rather than free text. Pass rough text to `--q` and take `canonical_name`\n" +
			"from the row that matches — the full comma-joined form, not the city alone.\n" +
			"`--limit` caps the number of candidates. This call sends no credential at all\n" +
			"and costs nothing, so there is no reason to guess a location string instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIfChanged(cmd.Flags(), q, "q", query, "q")
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			// Empty apiKey → get() injects no api_key query param.
			body, err := s.get(cmd.Context(), "", "/locations.json", q)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&query, "q", "", "place-name text to match")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of locations to return")
	return cmd
}

// newAccountCmd builds `serpapi account`: the free Account API (plan, searches
// left, rate limit). It doubles as the credential smoke test. The provider
// response echoes the private key in an `api_key` field; that field is redacted
// before emit so the secret never reaches the agent transcript.
func (s *Service) newAccountCmd(apiKey string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Account API: plan, searches left, rate limit (api_key redacted)",
		Long: "Free to call and the natural budget check before a batch: it reports the plan,\n" +
			"`total_searches_left` and the hourly rate limit. It also doubles as the\n" +
			"credential smoke test, since it is the cheapest authenticated call in the\n" +
			"tool. SerpApi echoes the private key back in this response; that field is\n" +
			"stripped before anything is printed, so the secret never lands in the\n" +
			"transcript.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireKey(apiKey); err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), apiKey, "/account.json", nil)
			if err != nil {
				return err
			}
			redacted, err := redactAPIKey(body)
			if err != nil {
				return err
			}
			return s.emit(redacted)
		},
	}
	return cmd
}

// setIfChanged copies a string flag into the query under paramName, but only
// when the user actually set it — unset optional flags never leak an empty
// param onto the request.
func setIfChanged(flags interface{ Changed(string) bool }, q url.Values, flagName, value, paramName string) {
	if flags.Changed(flagName) {
		q.Set(paramName, value)
	}
}

// applyParams parses each `key=value` escape-hatch entry and sets it on q,
// overriding any first-class flag of the same name. A missing `=` is a usage
// error. An empty value is preserved (e.g. `--param filter=`).
func applyParams(q url.Values, params []string) error {
	for _, p := range params {
		key, value, found := strings.Cut(p, "=")
		if !found {
			return &usageError{msg: fmt.Sprintf("invalid --param %q: must be key=value", p)}
		}
		q.Set(key, value)
	}
	return nil
}

// redactAPIKey removes the echoed `api_key` field from the Account API
// response, preserving all other fields and their JSON types.
func redactAPIKey(body []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, &apiError{msg: fmt.Sprintf("serpapi: parse account response: %v", err), err: err}
	}
	delete(obj, "api_key")
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("serpapi: encode account response: %v", err), err: err}
	}
	return out, nil
}
