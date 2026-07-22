package kit

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// listFlags holds the cursor-pagination flags shared by every list command.
// Kit lists are cursor-based; the tool surfaces one page per call and never
// auto-follows cursors (no unbounded fan-out). --after resumes from a prior
// response's pagination.end_cursor; --limit maps to per_page.
type listFlags struct {
	after  string
	before string
	limit  int
}

// registerListFlags attaches --after / --before / --limit to a list command.
func registerListFlags(cmd *cobra.Command) *listFlags {
	lf := &listFlags{}
	cmd.Flags().StringVar(&lf.after, "after", "", "resume from a prior response's pagination.end_cursor")
	cmd.Flags().StringVar(&lf.before, "before", "", "page backwards from a prior response's pagination.start_cursor")
	cmd.Flags().IntVar(&lf.limit, "limit", 0, "results per page (Kit per_page; default 500, max 1000)")
	return lf
}

// apply writes the pagination flags into a query.
func (lf *listFlags) apply(q url.Values) {
	if lf.after != "" {
		q.Set("after", lf.after)
	}
	if lf.before != "" {
		q.Set("before", lf.before)
	}
	if lf.limit > 0 {
		q.Set("per_page", strconv.Itoa(lf.limit))
	}
}

// membershipRequest resolves a subscriber-membership call against a collection
// endpoint <base>/subscribers for tag/form/sequence commands. Exactly one of
// subscriberID or email must be set. By id, the subscriber id becomes a path
// segment ("/{id}"). By email, POST carries the email in the JSON body while
// DELETE carries it in an email_address query parameter (Kit removes-by-email
// via query, since a DELETE has no body). It returns the path suffix, query
// values, and JSON body to send.
func membershipRequest(method string, subscriberID int, email string) (suffix string, q url.Values, body map[string]any, err error) {
	hasID := subscriberID > 0
	hasEmail := email != ""
	switch {
	case hasID && hasEmail:
		return "", nil, nil, &usageError{msg: "--subscriber-id and --email are mutually exclusive"}
	case hasID:
		return "/" + strconv.Itoa(subscriberID), nil, nil, nil
	case hasEmail:
		if method == http.MethodDelete {
			return "", url.Values{"email_address": {email}}, nil, nil
		}
		return "", nil, map[string]any{"email_address": email}, nil
	default:
		return "", nil, nil, &usageError{msg: "one of --subscriber-id or --email is required"}
	}
}

// requirePositive returns a usage error when an id flag is missing or non-positive.
func requirePositive(name string, v int) error {
	if v <= 0 {
		return &usageError{msg: fmt.Sprintf("--%s is required", name)}
	}
	return nil
}
