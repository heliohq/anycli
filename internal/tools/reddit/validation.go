package reddit

import (
	"fmt"
	"strings"
)

// requireLimit validates an optional page-size flag. 0 means "unset" (let Reddit
// default); otherwise it must fall within the API's 1..100 window.
func requireLimit(limit int) error {
	if limit != 0 && (limit < 1 || limit > 100) {
		return &usageError{msg: fmt.Sprintf("--limit must be between 1 and 100, got %d", limit)}
	}
	return nil
}

// requireEnum validates a flag value against an allowed set (empty = unset/ok).
func requireEnum(flag, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return &usageError{msg: fmt.Sprintf("--%s must be one of %s, got %q", flag, strings.Join(allowed, "|"), value)}
}

// requireFullname validates a Reddit thing fullname (e.g. t3_abc123). The prefix
// encodes the type; edit/delete/comment/mark-read all address things by fullname.
func requireFullname(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &usageError{msg: "a thing fullname is required (e.g. t3_abc123)"}
	}
	prefix, rest, ok := strings.Cut(name, "_")
	if !ok || rest == "" {
		return &usageError{msg: fmt.Sprintf("%q is not a fullname; expected <type>_<id> like t3_abc123", name)}
	}
	switch prefix {
	case "t1", "t2", "t3", "t4", "t5", "t6":
		return nil
	default:
		return &usageError{msg: fmt.Sprintf("%q has an unknown type prefix %q", name, prefix)}
	}
}

// articleID strips a t3_ prefix so a post id36 can address /comments/{id}, which
// takes the bare id36 rather than a fullname.
func articleID(id string) string {
	id = strings.TrimSpace(id)
	if rest, ok := strings.CutPrefix(id, "t3_"); ok {
		return rest
	}
	return id
}
