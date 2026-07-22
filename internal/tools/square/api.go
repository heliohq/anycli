package square

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// newAPICmd is the raw Square REST escape hatch, similar in spirit to `gh api`.
// It keeps credential + Square-Version injection inside AnyCLI while allowing
// uncommon or new endpoints (loyalty, subscriptions, team, …) to be exercised
// before they deserve a first-class command. The method is runtime input, so it
// is annotated side-effecting.
func (s *Service) newAPICmd(token string) *cobra.Command {
	var body, bodyFile string
	var headers []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Short:       "Make a raw Square API request (path starts at /v2/…)",
		Args:        cobra.ExactArgs(2),
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			path := strings.TrimSpace(args[1])
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return &usageError{msg: "square api: --body and --body-file are mutually exclusive"}
			}
			extraHeaders, err := parseAPIHeaders(headers)
			if err != nil {
				return err
			}
			var payload []byte
			if cmd.Flags().Changed("body-file") {
				payload, err = os.ReadFile(bodyFile)
				if err != nil {
					return &usageError{msg: fmt.Sprintf("square api: read --body-file %s: %v", bodyFile, err)}
				}
			} else if cmd.Flags().Changed("body") {
				payload = []byte(body)
			}
			resp, err := s.callRaw(cmd.Context(), token, method, path, payload, extraHeaders)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body, usually JSON")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read request body from file")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header as name:value (repeatable; Authorization and Square-Version are injected)")
	return cmd
}

// parseAPIHeaders splits repeatable name:value header flags into a map.
func parseAPIHeaders(raw []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range raw {
		idx := strings.Index(h, ":")
		if idx <= 0 {
			return nil, &usageError{msg: fmt.Sprintf("square api: invalid --header %q (want name:value)", h)}
		}
		name := strings.TrimSpace(h[:idx])
		value := strings.TrimSpace(h[idx+1:])
		if name == "" {
			return nil, &usageError{msg: fmt.Sprintf("square api: invalid --header %q (empty name)", h)}
		}
		out[name] = value
	}
	return out, nil
}

// callRaw issues an arbitrary-method request with a caller-provided body and
// extra headers, injecting Bearer auth + Square-Version. Non-2xx becomes an
// *apiError; 401 rejects the credential.
func (s *Service) callRaw(ctx context.Context, token, method, path string, payload []byte, extraHeaders map[string]string) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL()+path, reqBody)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Square-Version", squareVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: %s %s: %v", method, path, err), err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("square: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &apiError{
			msg:    fmt.Sprintf("square API error (HTTP %d): %s", resp.StatusCode, apiMessage(respBody)),
			status: resp.StatusCode,
		}
		if resp.StatusCode == http.StatusUnauthorized {
			apiErr.err = execution.RejectCredential(fmt.Errorf("%s", apiErr.msg))
		}
		return nil, apiErr
	}
	return respBody, nil
}
