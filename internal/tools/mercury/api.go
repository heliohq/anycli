package mercury

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// newAPICmd is the raw Mercury REST escape hatch, similar in spirit to
// `gh api`. It keeps credential injection inside AnyCLI while allowing
// uncommon or new endpoints to be exercised before they deserve a
// first-class command. The method is runtime input, so it is annotated
// side-effecting.
func (s *Service) newAPICmd(token string) *cobra.Command {
	var body, bodyFile string
	var headers []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Short:       "Make a raw Mercury API request (path starts after /api/v1)",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"}, // arbitrary method
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
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return &usageError{msg: "mercury api: --body and --body-file are mutually exclusive"}
			}
			var payload []byte
			if cmd.Flags().Changed("body-file") {
				payload, err = execution.ReadFile(s.FS, bodyFile)
				if err != nil {
					return &usageError{msg: fmt.Sprintf("mercury api: read --body-file %s: %v", bodyFile, err)}
				}
			} else if cmd.Flags().Changed("body") {
				payload = []byte(body)
			}
			if payload != nil {
				if _, ok := extraHeaders["Content-Type"]; !ok {
					extraHeaders["Content-Type"] = "application/json"
				}
			}
			resp, err := s.callRaw(cmd.Context(), token, method, path, payload, extraHeaders)
			if err != nil {
				return err
			}
			return s.emitRaw(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body, usually JSON")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read request body from file")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header as name:value (repeatable; Authorization is injected)")
	return cmd
}

// normalizeAPIPath accepts a bare path ("/accounts"), a path carrying the
// /api/v1 prefix, or a full Mercury URL, and returns the path relative to the
// /api/v1 base (query string preserved).
func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "mercury api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("mercury api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
		if u.RawQuery != "" {
			raw += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if raw == "/api/v1" {
		return "/", nil
	}
	if strings.HasPrefix(raw, "/api/v1/") {
		raw = strings.TrimPrefix(raw, "/api/v1")
	}
	return raw, nil
}

// parseAPIHeaders parses repeated --header name:value flags. Authorization is
// injected from the resolved credential and cannot be overridden.
func parseAPIHeaders(vals []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range vals {
		name, val, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{msg: fmt.Sprintf("mercury api: --header must be name:value, got %q", h)}
		}
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if strings.EqualFold(canonical, "Authorization") {
			return nil, &usageError{msg: fmt.Sprintf("mercury api: %s is injected and cannot be overridden", canonical)}
		}
		out[canonical] = strings.TrimSpace(val)
	}
	return out, nil
}

// callRaw performs one Mercury API request with an arbitrary method, body,
// and extra headers, sharing call's auth and error contract: a 401 marks the
// credential rejected, any other non-2xx surfaces Mercury's message as an
// apiError carrying the HTTP status, a transport failure is an apiError with
// status 0.
func (s *Service) callRaw(ctx context.Context, token, method, path string, payload []byte, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL()+path, bytes.NewReader(payload))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("mercury: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("mercury API error (HTTP %d): %s", resp.StatusCode, apiMessage(body))
		if resp.StatusCode == http.StatusUnauthorized {
			rejected := execution.RejectCredential(raw)
			return nil, &apiError{msg: rejected.Error(), status: resp.StatusCode, err: rejected}
		}
		return nil, &apiError{msg: raw.Error(), status: resp.StatusCode, err: raw}
	}
	return body, nil
}

// emitRaw writes the provider's response body to stdout verbatim (with a
// trailing newline). The api escape hatch skips the {"data": ...}
// normalization so agents see exactly what Mercury returned.
func (s *Service) emitRaw(body []byte) error {
	if _, err := s.stdout().Write(body); err != nil {
		return err
	}
	if len(body) > 0 && body[len(body)-1] == '\n' {
		return nil
	}
	_, err := io.WriteString(s.stdout(), "\n")
	return err
}
