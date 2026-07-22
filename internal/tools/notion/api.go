package notion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw Notion REST escape hatch, similar in spirit to `gh api`.
// It keeps credential and Notion-Version injection inside AnyCLI while allowing
// uncommon or new endpoints to be exercised before they deserve a first-class
// command.
func (s *Service) newAPICmd(token string) *cobra.Command {
	var body, bodyFile string
	var forms, formFiles, headers []string
	cmd := &cobra.Command{
		Use:         "api <method> <path>",
		Annotations: map[string]string{"anycli.side_effect": "true"},
		Short:       "Make a raw Notion API request",
		Args:        cobra.ExactArgs(2),
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
			hasBody := cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file")
			hasForm := len(forms) > 0 || len(formFiles) > 0
			if hasBody && hasForm {
				return &usageError{msg: "notion api: --body/--body-file cannot be combined with --form/--form-file"}
			}
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return &usageError{msg: "notion api: --body and --body-file are mutually exclusive"}
			}
			if hasForm {
				payload, contentType, err := buildMultipartPayload(forms, formFiles)
				if err != nil {
					return err
				}
				extraHeaders["Content-Type"] = contentType
				resp, err := s.callRaw(cmd.Context(), token, method, path, payload, extraHeaders)
				if err != nil {
					return err
				}
				return s.emitJSON(resp)
			}
			var payload []byte
			if cmd.Flags().Changed("body-file") {
				payload, err = os.ReadFile(bodyFile)
				if err != nil {
					return &usageError{msg: fmt.Sprintf("notion api: read --body-file %s: %v", bodyFile, err)}
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
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body, usually JSON")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read request body from file")
	cmd.Flags().StringArrayVar(&forms, "form", nil, "multipart form field as name=value (repeatable)")
	cmd.Flags().StringArrayVar(&formFiles, "form-file", nil, "multipart file field as name=path (repeatable)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header as name:value (repeatable; Authorization and Notion-Version are injected)")
	return cmd
}

func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "notion api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("notion api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
		if u.RawQuery != "" {
			raw += "?" + u.RawQuery
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if raw == "/v1" {
		return "/", nil
	}
	if strings.HasPrefix(raw, "/v1/") {
		raw = strings.TrimPrefix(raw, "/v1")
	}
	return raw, nil
}

func parseAPIHeaders(vals []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range vals {
		name, val, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{msg: fmt.Sprintf("notion api: --header must be name:value, got %q", h)}
		}
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		switch strings.ToLower(canonical) {
		case "authorization", "notion-version":
			return nil, &usageError{msg: fmt.Sprintf("notion api: %s is injected and cannot be overridden", canonical)}
		}
		out[canonical] = strings.TrimSpace(val)
	}
	return out, nil
}

func buildMultipartPayload(forms, formFiles []string) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range forms {
		name, val, ok := strings.Cut(f, "=")
		if !ok || strings.TrimSpace(name) == "" {
			_ = mw.Close()
			return nil, "", &usageError{msg: fmt.Sprintf("notion api: --form must be name=value, got %q", f)}
		}
		if err := mw.WriteField(strings.TrimSpace(name), val); err != nil {
			_ = mw.Close()
			return nil, "", &apiError{msg: fmt.Sprintf("notion api: build multipart field: %v", err), err: err}
		}
	}
	for _, f := range formFiles {
		name, path, ok := strings.Cut(f, "=")
		if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(path) == "" {
			_ = mw.Close()
			return nil, "", &usageError{msg: fmt.Sprintf("notion api: --form-file must be name=path, got %q", f)}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			_ = mw.Close()
			return nil, "", &usageError{msg: fmt.Sprintf("notion api: read --form-file %s: %v", path, err)}
		}
		filename := uploadName("", path)
		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, strings.TrimSpace(name), filename))
		partHeader.Set("Content-Type", sourceMime(path))
		part, err := mw.CreatePart(partHeader)
		if err != nil {
			_ = mw.Close()
			return nil, "", &apiError{msg: fmt.Sprintf("notion api: build multipart file: %v", err), err: err}
		}
		if _, err := part.Write(data); err != nil {
			_ = mw.Close()
			return nil, "", &apiError{msg: fmt.Sprintf("notion api: write multipart file: %v", err), err: err}
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", &apiError{msg: fmt.Sprintf("notion api: close multipart: %v", err), err: err}
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func (s *Service) callRaw(ctx context.Context, token, method, path string, payload []byte, headers map[string]string) ([]byte, error) {
	base := s.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: build request: %v", err), err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionVersion)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	hc := s.HC
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: %s %s: %v", method, path, err), err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &apiError{msg: fmt.Sprintf("notion: read response: %v", err), err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw := fmt.Errorf("notion API error (HTTP %d): %s%s", resp.StatusCode, apiMessage(body), accessHint(resp.StatusCode))
		classified := classifyNotionCredentialError(resp.StatusCode, body, raw)
		return nil, &apiError{msg: classified.Error(), status: resp.StatusCode, err: classified}
	}
	return body, nil
}
