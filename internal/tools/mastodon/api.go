package mastodon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw Mastodon REST escape hatch, similar in spirit to
// `gh api`: it keeps credential injection and the per-instance base URL inside
// AnyCLI while letting uncommon or new endpoints (lists, bookmarks, filters,
// scheduled statuses, admin) be exercised before they deserve a first-class
// command. The Authorization header is injected and cannot be overridden.
func (rt *runContext) newAPICmd() *cobra.Command {
	var body string
	var queries, headers []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Short:       "Make a raw Mastodon API request",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			path, err := normalizeAPIPath(args[1])
			if err != nil {
				return err
			}
			query, err := parseAPIQuery(queries)
			if err != nil {
				return err
			}
			extraHeaders, err := parseAPIHeaders(headers)
			if err != nil {
				return err
			}
			resp, err := rt.callRaw(cmd.Context(), method, path, query, body, extraHeaders)
			if err != nil {
				return err
			}
			return rt.emitRaw(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body, usually JSON")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter as key=value (repeatable)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header as name:value (repeatable; Authorization is injected and cannot be overridden)")
	return cmd
}

// normalizeAPIPath reduces a raw path to an instance-relative path starting
// with "/". A full URL is accepted (path + query extracted); a bare path is
// prefixed with "/".
func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "mastodon api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("mastodon api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
		if u.RawQuery != "" {
			raw += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw, nil
}

func parseAPIQuery(vals []string) (url.Values, error) {
	q := url.Values{}
	for _, kv := range vals {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, &usageError{msg: fmt.Sprintf("mastodon api: --query must be key=value, got %q", kv)}
		}
		q.Add(strings.TrimSpace(key), val)
	}
	return q, nil
}

func parseAPIHeaders(vals []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range vals {
		name, val, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{msg: fmt.Sprintf("mastodon api: --header must be name:value, got %q", h)}
		}
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if strings.EqualFold(canonical, "Authorization") {
			return nil, &usageError{msg: "mastodon api: Authorization is injected and cannot be overridden"}
		}
		out[canonical] = strings.TrimSpace(val)
	}
	return out, nil
}

// callRaw performs a raw request with the bearer token injected. The path may
// already carry a query string; --query params are merged on top.
func (rt *runContext) callRaw(ctx context.Context, method, path string, query url.Values, body string, headers map[string]string) ([]byte, error) {
	target := rt.baseURL() + path
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + query.Encode()
	}
	var reqBody io.Reader
	if body != "" {
		reqBody = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mastodon: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+rt.token)
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	respBody, _, err := rt.do(req)
	return respBody, err
}
