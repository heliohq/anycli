package segment

import (
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRequestCmd is the raw Segment Public API escape hatch, similar in spirit
// to `gh api`. The Public API has 100+ endpoints; this keeps credential
// injection inside AnyCLI while making any endpoint (including writes, via an
// explicit non-GET --method) reachable without a first-class subcommand. Output
// is the provider body verbatim.
func (s *Service) newRequestCmd(token string) *cobra.Command {
	var method, path, body, bodyFile string
	var params []string
	cmd := &cobra.Command{
		Use:         "request",
		Short:       "Make a raw Segment Public API request",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m := strings.ToUpper(strings.TrimSpace(method))
			if m == "" {
				return &usageError{msg: "--method is required"}
			}
			p, err := normalizePath(path)
			if err != nil {
				return err
			}
			q, err := parseParams(params)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return &usageError{msg: "--body and --body-file are mutually exclusive"}
			}
			var payload []byte
			if cmd.Flags().Changed("body-file") {
				payload, err = os.ReadFile(bodyFile)
				if err != nil {
					return &usageError{msg: "read --body-file " + bodyFile + ": " + err.Error()}
				}
			} else if cmd.Flags().Changed("body") {
				payload = []byte(body)
			}
			resp, err := s.do(cmd.Context(), token, m, p, q, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&method, "method", "GET", "HTTP method (GET|POST|PUT|PATCH|DELETE)")
	cmd.Flags().StringVar(&path, "path", "", "API path, e.g. /sources (required)")
	cmd.Flags().StringArrayVar(&params, "query", nil, "query param as name=value (repeatable)")
	cmd.Flags().StringVar(&body, "body", "", "raw request body, usually JSON")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read request body from file")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

// normalizePath accepts a bare path, a /-prefixed path, or a full URL and
// returns a root-relative path. Any query string in a full URL is dropped
// (query pairs are supplied via --query).
func normalizePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "--path is empty"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: "bad URL " + raw + ": " + err.Error()}
		}
		raw = u.EscapedPath()
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw, nil
}
