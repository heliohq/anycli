package pandadoc

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw PandaDoc REST escape hatch, similar in spirit to
// `gh api`. It keeps credential injection inside AnyCLI while allowing uncommon
// or new endpoints to be exercised before they deserve a first-class command.
func (s *Service) newAPICmd(authz string) *cobra.Command {
	var body, bodyFile string
	var queries []string
	cmd := &cobra.Command{
		Use:   "api <method> <path>",
		Short: "Make a raw PandaDoc API request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(strings.TrimSpace(args[0]))
			path, err := normalizeAPIPath(args[1])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("body") && cmd.Flags().Changed("body-file") {
				return &usageError{msg: "pandadoc api: --body and --body-file are mutually exclusive"}
			}
			query, err := parseAPIQuery(queries)
			if err != nil {
				return err
			}
			var payload any
			if cmd.Flags().Changed("body-file") {
				raw, err := os.ReadFile(bodyFile)
				if err != nil {
					return &usageError{msg: fmt.Sprintf("pandadoc api: read --body-file %s: %v", bodyFile, err)}
				}
				payload = jsonBody(raw)
			} else if cmd.Flags().Changed("body") {
				payload = jsonBody([]byte(body))
			}
			resp, err := s.call(cmd.Context(), authz, method, path, query, payload)
			if err != nil {
				return err
			}
			return s.emitJSON(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", "raw request body (usually JSON)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read the request body from a file")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter as key=value (repeatable)")
	return cmd
}

// jsonBody wraps a raw byte slice so call() forwards it verbatim as the JSON
// body (json.RawMessage marshals to itself).
func jsonBody(raw []byte) any {
	return rawJSON(raw)
}

// rawJSON is a byte slice that marshals to itself, used to forward a raw body.
type rawJSON []byte

func (r rawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}

// normalizeAPIPath ensures a leading slash and strips a redundant /public/v1
// prefix (the base URL already carries it), so callers may pass either form.
func normalizeAPIPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &usageError{msg: "pandadoc api: empty path"}
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", &usageError{msg: fmt.Sprintf("pandadoc api: bad URL %q: %v", raw, err)}
		}
		raw = u.EscapedPath()
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if raw == "/public/v1" {
		return "/", nil
	}
	raw = strings.TrimPrefix(raw, "/public/v1")
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return raw, nil
}

// parseAPIQuery turns repeated --query key=value flags into url.Values.
func parseAPIQuery(vals []string) (url.Values, error) {
	q := url.Values{}
	for _, kv := range vals {
		name, value, err := parseKeyValue("query", kv)
		if err != nil {
			return nil, err
		}
		q.Add(name, value)
	}
	return q, nil
}
