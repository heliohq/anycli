package mixpanel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newLexiconListCmd — GET /api/app/projects/{project_id}/schemas. Lists only
// authored event/property schemas — a documentation overlay, NOT the discovery
// primitive. A project that never authored schemas returns a partial/empty list
// even while events actively fire, so an empty result must not be read as "no
// events"; use `events-names` for discovery (design §1).
func (s *Service) newLexiconListCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List authored Lexicon schemas (GET /projects/{id}/schemas)",
		Long: "Only events and properties with an AUTHORED schema appear — this is a\n" +
			"documentation overlay, not discovery. A project that never wrote schemas\n" +
			"returns a partial or empty list while events fire normally, so an empty\n" +
			"result says nothing about whether the project has data. Take the real\n" +
			"inventory from `events-names` and use this for the descriptions and types\n" +
			"layered on top. Runs on the App API, so it does not spend Query API budget.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/projects/" + url.PathEscape(c.projectID) + "/schemas"
			body, err := c.getJSON(cmd.Context(), c.appBase, path, nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
}

// newMeCmd — GET /api/app/me. Runtime identity/auth probe (region-aware); NOT a
// connect-time verifier — identity is credential-derived. A wrong secret
// surfaces here as a distinct 401 (credential error kind).
func (s *Service) newMeCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Runtime identity/auth probe (GET /me)",
		Long: "Returns the Service Account's own identity and the projects it can reach,\n" +
			"which is how to confirm a credential works and which region it landed in.\n" +
			"It runs on the App API rather than the Query API, so it costs nothing\n" +
			"against the 60-queries-per-hour budget — the right first call after a\n" +
			"connect or after an unexplained 401.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := c.getJSON(cmd.Context(), c.appBase, "/me", nil)
			if err != nil {
				return err
			}
			return c.emitJSON(body)
		},
	}
}

// newExportCmd — GET /api/2.0/export (export host). Streams raw events as JSONL
// (one event object per line) for a bounded date range. The window is mandatory
// because the stream is unbounded by default and can be very large.
func (s *Service) newExportCmd(c *client) *cobra.Command {
	var from, to, where, limit string
	var events []string
	cmd := &cobra.Command{
		Use:         "export",
		Short:       "Bounded raw event export as JSONL (GET /export on the export host)",
		Annotations: readOnly,
		Long: "Streams raw events as line-delimited JSON — one event object per line, NOT\n" +
			"a single JSON document — straight to stdout without buffering, so parse it\n" +
			"per line. `--from` and `--to` are required precisely because the export is\n" +
			"otherwise unbounded and a wide window can run to gigabytes; narrow it\n" +
			"further with `--event` (repeatable), `--where` and `--limit`. It rides\n" +
			"Mixpanel's separate export host, which carries its own rate limits rather\n" +
			"than the Query API's 60 per hour.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := c.projectValues()
			v.Set("from_date", from)
			v.Set("to_date", to)
			setIf(v, "where", where)
			setIf(v, "limit", limit)
			if len(events) > 0 {
				arr, err := jsonArray(events)
				if err != nil {
					return &usageError{msg: "encode --event list: " + err.Error()}
				}
				v.Set("event", arr)
			}
			return c.stream(cmd.Context(), c.exportBase, "/export", v)
		},
	}
	f := cmd.Flags()
	f.StringVar(&from, "from", "", "start date YYYY-MM-DD (required)")
	f.StringVar(&to, "to", "", "end date YYYY-MM-DD (required)")
	f.StringArrayVar(&events, "event", nil, "restrict to event name (repeatable)")
	f.StringVar(&where, "where", "", "filter expression")
	f.StringVar(&limit, "limit", "", "max number of events to return")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// stream issues a GET and copies the response body straight to stdout without
// buffering the whole thing — the export payload is JSONL and can be large. A
// non-2xx status is read fully and classified like any other apiError.
func (c *client) stream(ctx context.Context, base, path string, query url.Values) error {
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("mixpanel: build request: %v", err), kind: "api", err: err}
	}
	req.Header.Set("Authorization", c.authHeader)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("mixpanel: GET %s: %v", path, err), kind: "api", err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return classifyError(resp.StatusCode, resp.Header.Get("Retry-After"), body)
	}
	if _, err := io.Copy(c.out, resp.Body); err != nil {
		return &apiError{msg: fmt.Sprintf("mixpanel: stream export: %v", err), kind: "api", err: err}
	}
	return nil
}
