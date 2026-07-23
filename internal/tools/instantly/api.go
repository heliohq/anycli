package instantly

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw Instantly REST escape hatch, similar in spirit to
// `gh api`. It keeps credential injection inside AnyCLI while allowing uncommon
// or new endpoints (subsequences, custom tags, block lists, webhooks, inbox
// placement, …) to be exercised without a first-class command. The
// Authorization header is injected and cannot be overridden.
func (s *Service) newAPICmd(token string) *cobra.Command {
	var data string
	var queries []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Annotations: writeAction,
		Short:       "Make a raw Instantly API request",
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			path, err := normalizeAPIPath(args[1])
			if err != nil {
				return err
			}
			q, err := parseQueryPairs(queries)
			if err != nil {
				return err
			}
			var payload any
			if cmd.Flags().Changed("data") {
				payload, err = decodeDataFlag(data)
				if err != nil {
					return err
				}
			}
			body, err := s.call(cmd.Context(), token, method, path, q, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw JSON request body")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter as key=value (repeatable)")
	return cmd
}

// normalizeAPIPath accepts a full URL, an /api/v2-prefixed path, or a bare
// resource path and returns a path relative to the base URL (which already
// carries /api/v2).
func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "instantly api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("instantly api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
		if u.RawQuery != "" {
			raw += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	// The base URL already ends in /api/v2, so strip a redundant prefix.
	raw = strings.TrimPrefix(raw, "/api/v2")
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw, nil
}

// parseQueryPairs turns repeatable key=value flags into url.Values.
func parseQueryPairs(pairs []string) (url.Values, error) {
	q := url.Values{}
	for _, p := range pairs {
		key, val, ok := strings.Cut(p, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, &usageError{msg: fmt.Sprintf("instantly api: --query must be key=value, got %q", p)}
		}
		q.Add(strings.TrimSpace(key), val)
	}
	return q, nil
}
