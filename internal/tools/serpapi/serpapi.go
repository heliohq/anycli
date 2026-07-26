// Package serpapi is the built-in SerpApi service: a non-interactive cobra
// tree over the serpapi.com live-SERP API. SerpApi authenticates by the
// `api_key` query parameter only (no header auth exists); the Locations API is
// free and unauthenticated. Errors are non-2xx with a JSON body carrying a
// top-level "error" string; 401 rejects the credential. Every command emits
// the provider JSON on stdout (passthrough + newline) except `account`, which
// redacts the echoed `api_key` field so the private key never reaches the
// agent transcript.
package serpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// DefaultBaseURL is the production SerpApi base.
const DefaultBaseURL = "https://serpapi.com"

// EnvAPIKey is the env var the credential binding injects
// (definitions/tools/serpapi.json). SerpApi keys are long-lived private keys
// with no scopes, expiry, or refresh cycle.
const EnvAPIKey = "SERPAPI_API_KEY"

// Service implements the built-in SerpApi tool. It satisfies tools.Service by
// duck typing (this package never imports the registry — no import cycle).
type Service struct {
	// BaseURL overrides the SerpApi base; empty = DefaultBaseURL. Tests point
	// it at an httptest server.
	BaseURL string
	// HC is the HTTP client; nil = http.DefaultClient.
	HC *http.Client
	// Out / Err override stdout / stderr; nil = the process streams.
	Out io.Writer
	Err io.Writer
}

// Execute runs one serpapi subcommand with the resolved credentials in env.
// Success is exit 0; usage/param errors (bad --param shape, missing args,
// unknown subcommands) are exit 2; runtime/API errors (SerpApi non-2xx,
// transport failure, missing credential on an authed call) are exit 1. Errors
// render to stderr — as JSON under --json, plain text otherwise. The API key
// is checked lazily per call, not up front, because `locations` is free and
// unauthenticated per the official docs.
func (s *Service) Execute(ctx context.Context, args []string, env map[string]string) (execution.Result, error) {
	root := s.newRoot(env[EnvAPIKey])
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return execution.Result{}, nil
	}

	jsonMode, _ := root.PersistentFlags().GetBool("json")
	s.renderError(jsonMode, err)

	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// Runtime/API failure: exit 1, preserving credential-rejection
		// classification carried through the wrapped cause.
		return execution.Failure(err), nil
	}
	// usageError plus every cobra-originated parse/arg/unknown-command error
	// is inherently a usage error → exit 2.
	return execution.Result{ExitCode: 2}, nil
}

// renderError writes err to stderr. Under --json the shape is
// {"error":{"message":…,"kind":"usage|api","status":<HTTP or omitted>}}.
func (s *Service) renderError(jsonMode bool, err error) {
	if !jsonMode {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	payload := map[string]any{"message": err.Error(), "kind": "usage"}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		payload["kind"] = "api"
		if apiErr.status != 0 {
			payload["status"] = apiErr.status
		}
	}
	b, mErr := json.Marshal(map[string]any{"error": payload})
	if mErr != nil {
		fmt.Fprintln(s.stderr(), err)
		return
	}
	fmt.Fprintln(s.stderr(), string(b))
}

// newRoot builds the cobra tree: search / archive / locations / account.
func (s *Service) newRoot(apiKey string) *cobra.Command {
	root := &cobra.Command{
		Use:   "serpapi",
		Short: "SerpApi built-in service (live search engine results)",
		Long: "Runs live search-engine queries and returns structured JSON — organic results,\n" +
			"knowledge graph, answer boxes, local packs — instead of HTML to be scraped.\n" +
			"\n" +
			"There is no per-engine subcommand and there never will be. `search` is one\n" +
			"generic command over two axes: `--engine` picks the vertical (`google`,\n" +
			"`google_news`, `google_maps`, `google_scholar`, `youtube`, `bing`, and the\n" +
			"rest of SerpApi's roughly seventy), and the params are the knobs. The\n" +
			"cross-engine params are first-class flags; anything engine-specific rides the\n" +
			"repeatable `--param key=value`. `--engine` is passed through unvalidated, so\n" +
			"an engine added by SerpApi after this build still works.\n" +
			"\n" +
			"Output shape follows the engine, not the tool: `organic_results` for google,\n" +
			"`news_results`, `local_results`, `shopping_results` and so on elsewhere. Read\n" +
			"`search_metadata` and then the array the chosen engine actually populates.\n" +
			"\n" +
			"Only `search` spends quota. `locations`, `archive get` and `account` are free,\n" +
			"which makes the cheap path explicit: budget with `account`, resolve places\n" +
			"with `locations`, and re-read a result already paid for with `archive get`\n" +
			"rather than repeating the search.\n" +
			"\n" +
			"An unknown engine or a malformed param fails with SerpApi's own message rather\n" +
			"than a local validation error — read the `error` field, which usually names\n" +
			"the offending parameter.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(s.stdout())
	root.SetErr(s.stderr())
	root.PersistentFlags().Bool("json", false, "output JSON (always on; accepted for uniformity)")

	root.AddCommand(
		s.newSearchCmd(apiKey),
		s.newArchiveCmd(apiKey),
		s.newLocationsCmd(),
		s.newAccountCmd(apiKey),
	)
	return root
}

func (s *Service) baseURL() string {
	if s.BaseURL != "" {
		return strings.TrimRight(s.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (s *Service) client() *http.Client {
	if s.HC != nil {
		return s.HC
	}
	return http.DefaultClient
}

func (s *Service) stdout() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Err != nil {
		return s.Err
	}
	return os.Stderr
}
