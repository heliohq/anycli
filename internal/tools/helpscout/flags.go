package helpscout

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// enumValidator returns a case-exact validator for a fixed value set. An empty
// value passes (the flag is unset); any other out-of-set value is a usage
// error. Each command builds its own validator — enum sets are intentionally
// not shared.
func enumValidator(flag string, allowed ...string) func(string) error {
	return func(v string) error {
		if v == "" {
			return nil
		}
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return &usageError{msg: fmt.Sprintf("--%s must be one of %s, got %q", flag, strings.Join(allowed, "|"), v)}
	}
}

// setIf writes key=value into q only when value is non-empty.
func setIf(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// setPage writes the 1-based page number into q when > 0.
func setPage(q url.Values, page int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
}

// splitCSV turns "a, b ,c" into ["a","b","c"], dropping empties. Help Scout's
// tags PUT replaces the whole set, so an empty input yields an empty slice
// (clears all tags) rather than nil.
func splitCSV(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
