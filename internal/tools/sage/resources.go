package sage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newListCmd builds a `list` leaf: GET <path> with --page / --items-per-page
// query params and the resolved X-Business header. Output is Sage's list
// envelope ($items / $total / $next) verbatim, so the caller continues by
// re-requesting the next page.
func (s *Service) newListCmd(token, use, path, short string) *cobra.Command {
	var page, itemsPerPage int
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&itemsPerPage, "items-per-page", 0, "max items per page")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodGet, withPaging(path, page, itemsPerPage), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newGetCmd builds a `get <id>` leaf: GET <path>/<id> with the resolved
// X-Business header. Output is the resource JSON verbatim.
func (s *Service) newGetCmd(token, use, path, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         use + " <id>",
		Short:       short,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if id == "" {
			return &usageError{msg: "empty id"}
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodGet, path+"/"+url.PathEscape(id), nil)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newCreateCmd builds a `create` leaf: POST <path> with a verbatim --body JSON
// payload (the caller supplies the exact Sage resource envelope, e.g.
// {"contact":{…}} / {"sales_invoice":{…}} / {"contact_payment":{…}}) and the
// resolved X-Business header. Output is the created resource JSON verbatim.
func (s *Service) newCreateCmd(token, use, path, short string) *cobra.Command {
	var bodyFlag string
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&bodyFlag, "body", "", "request body: the full Sage resource JSON envelope (required)")
	_ = cmd.MarkFlagRequired("body")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := parseBody(bodyFlag)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), http.MethodPost, path, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// newFetchCmd is the top-level generic passthrough: `fetch --method --path
// [--body]` reaches any v3.1 resource not modeled above, on the same Bearer +
// X-Business path. --path is the resource path below the v3.1 base (a leading
// slash is optional). Annotated side-effecting because --method can mutate.
func (s *Service) newFetchCmd(token string) *cobra.Command {
	var method, path, bodyFlag string
	cmd := &cobra.Command{
		Use:         "fetch",
		Short:       "Call any Sage Accounting v3.1 endpoint directly",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "true"},
	}
	cmd.Flags().StringVar(&method, "method", http.MethodGet, "HTTP method (GET|POST|PUT|DELETE)")
	cmd.Flags().StringVar(&path, "path", "", "resource path below /v3.1 (e.g. /contacts or contacts) (required)")
	cmd.Flags().StringVar(&bodyFlag, "body", "", "request body JSON (for POST/PUT)")
	_ = cmd.MarkFlagRequired("path")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		m := strings.ToUpper(strings.TrimSpace(method))
		switch m {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		default:
			return &usageError{msg: fmt.Sprintf("--method must be one of GET|POST|PUT|DELETE|PATCH, got %q", method)}
		}
		p := strings.TrimSpace(path)
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		var payload any
		if strings.TrimSpace(bodyFlag) != "" {
			parsed, err := parseBody(bodyFlag)
			if err != nil {
				return err
			}
			payload = parsed
		}
		body, err := s.call(cmd.Context(), token, businessFlag(cmd), m, p, payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}

// parseBody validates a --body JSON payload on parse. An empty value is a
// fail-fast usage error (create/fetch-with-body require a body); invalid JSON
// is likewise a usage error.
func parseBody(val string) (json.RawMessage, error) {
	if strings.TrimSpace(val) == "" {
		return nil, &usageError{msg: "--body is required and must be valid JSON"}
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(val), &raw); err != nil {
		return nil, &usageError{msg: fmt.Sprintf("--body is not valid JSON: %v", err)}
	}
	return raw, nil
}
