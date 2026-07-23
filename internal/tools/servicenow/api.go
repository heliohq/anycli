package servicenow

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw ServiceNow REST escape hatch, similar in spirit to
// `gh api`. It keeps credential injection (x-sn-apikey) inside AnyCLI while
// reaching any /api/now/... path (Aggregate, Import Set, Attachment, …) that
// does not deserve a first-class command. The path is relative to the instance
// host: `/api/now/table/incident` or the shorthand `/now/table/incident`.
func (s *Service) newAPICmd(c *client) *cobra.Command {
	var body string
	var queries, headers []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Short:       "Make a raw ServiceNow REST request",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			path, err := normalizeAPIPath(args[1])
			if err != nil {
				return err
			}
			extraHeaders, err := parseAPIHeaders(headers)
			if err != nil {
				return err
			}
			q, err := parseQueryPairs(queries)
			if err != nil {
				return err
			}
			var payload any
			if cmd.Flags().Changed("body") {
				payload, err = parseDataObject(body)
				if err != nil {
					return err
				}
			}
			resp, err := c.do(cmd.Context(), method, path, q, payload, extraHeaders)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body as a JSON object")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query param as k=v (repeatable)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header as name:value (repeatable; x-sn-apikey is injected and cannot be overridden)")
	return cmd
}

// normalizeAPIPath ensures a leading slash and, for the /now/... shorthand,
// prefixes /api so both `/api/now/...` and `/now/...` reach the same endpoint.
func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "servicenow api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("servicenow api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
		if u.RawQuery != "" {
			raw += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if strings.HasPrefix(raw, "/now/") {
		raw = "/api" + raw
	}
	return raw, nil
}

// parseAPIHeaders turns repeatable name:value flags into a canonical header map
// that is forwarded to the request. It rejects an attempt to override the
// injected x-sn-apikey auth header (the notion precedent); the auth/content
// headers are re-applied after these in client.do so they always win.
func parseAPIHeaders(vals []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range vals {
		name, val, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{msg: fmt.Sprintf("servicenow api: --header must be name:value, got %q", h)}
		}
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if strings.EqualFold(canonical, apiKeyHeader) {
			return nil, &usageError{msg: fmt.Sprintf("servicenow api: %s is injected and cannot be overridden", apiKeyHeader)}
		}
		out[canonical] = strings.TrimSpace(val)
	}
	return out, nil
}

// parseQueryPairs turns repeatable k=v flags into url.Values.
func parseQueryPairs(vals []string) (url.Values, error) {
	q := url.Values{}
	for _, p := range vals {
		k, v, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, &usageError{msg: fmt.Sprintf("servicenow api: --query must be k=v, got %q", p)}
		}
		q.Add(k, v)
	}
	return q, nil
}
