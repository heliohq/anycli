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
		Use:         "search",
		Short:       "Run a live search (GET /search); --engine selects the vertical",
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
		Use:         "get <search_id>",
		Short:       "Fetch an archived search by id (GET /searches/<id>.json)",
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
		Use:         "locations",
		Short:       "Resolve a place name to a canonical location (free, no credential)",
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
		Use:         "account",
		Short:       "Account API: plan, searches left, rate limit (api_key redacted)",
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
