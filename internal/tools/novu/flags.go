package novu

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// decodeJSONFlag validates a raw-JSON flag value and returns the decoded value
// for passthrough into a request body. An empty string yields nil (omit).
func decodeJSONFlag(name, raw string) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, &usageError{msg: "novu: --" + name + " is not valid JSON: " + err.Error()}
	}
	return v, nil
}

// setIfNonEmpty writes a string field into a request map only when non-empty, so
// optional flags are omitted rather than sent as "".
func setIfNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

// addQueryString sets a query param only when the value is non-empty.
func addQueryString(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}

// addQueryInt sets a query param only when the value is positive (0 = omit,
// letting Novu apply its own default).
func addQueryInt(q url.Values, key string, val int) {
	if val > 0 {
		q.Set(key, strconv.Itoa(val))
	}
}

// splitCSV splits a comma-separated flag value into a trimmed, non-empty slice.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// requireFlag returns a usageError when a required string flag is empty.
func requireFlag(name, val string) error {
	if strings.TrimSpace(val) == "" {
		return &usageError{msg: "novu: --" + name + " is required"}
	}
	return nil
}

// pathEscape percent-escapes a single path segment (ids, keys) so values with
// spaces or slashes cannot break out of the intended path.
func pathEscape(seg string) string {
	return url.PathEscape(seg)
}

// jsonUnmarshalStrict is a thin wrapper so callers can branch on "did this parse
// as JSON" for scalar-or-object flags.
func jsonUnmarshalStrict(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}

// leafCmd builds a resource-group leaf command with SilenceUsage/Errors so
// RunE errors flow through Execute's classifier rather than cobra's own printer.
func leafCmd(use, short string, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
	return &cobra.Command{
		Use:           use,
		Short:         short,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          run,
	}
}
