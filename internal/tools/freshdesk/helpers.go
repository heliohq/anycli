package freshdesk

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// registerPagingFlags wires the shared --page / --per-page flags. per_page
// defaults to Freshdesk's own default (30) and is documented max 100.
func registerPagingFlags(cmd *cobra.Command, page, perPage *int) {
	cmd.Flags().IntVar(page, "page", 0, "page number (1-based; 0 = first page)")
	cmd.Flags().IntVar(perPage, "per-page", 0, "results per page (default 30, max 100)")
}

// applyPaging writes page / per_page onto a query value set, omitting zero
// values so Freshdesk applies its own defaults.
func applyPaging(q url.Values, page, perPage int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(perPage))
	}
}

// setNonEmpty sets a query key only when the value is non-empty.
func setNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// setBodyStr sets a request-body key only when the string value is non-empty.
func setBodyStr(body map[string]any, key, value string) {
	if value != "" {
		body[key] = value
	}
}

// setBodyInt sets a request-body key as an integer when the value parses as
// one, otherwise leaves it as the raw non-empty string (so an invalid value
// surfaces as a Freshdesk validation error rather than being silently
// dropped). Empty values are omitted.
func setBodyInt(body map[string]any, key, value string) {
	if value == "" {
		return
	}
	if n, err := strconv.Atoi(value); err == nil {
		body[key] = n
		return
	}
	body[key] = value
}

// applyCustomFields decodes a raw-JSON custom-fields flag into the body under
// the Freshdesk "custom_fields" key. An empty value is a no-op.
func applyCustomFields(body map[string]any, raw string) error {
	if raw == "" {
		return nil
	}
	v, err := decodeJSONFlag("custom-fields", raw)
	if err != nil {
		return err
	}
	body["custom_fields"] = v
	return nil
}

// joinCSV joins include-style slices into the comma-separated form Freshdesk
// expects for its include parameter.
func joinCSV(values []string) string {
	return strings.Join(values, ",")
}

// quoteQuery wraps a Freshdesk search query in the double quotes the API
// requires, unless the caller already supplied them.
func quoteQuery(q string) string {
	if len(q) >= 2 && strings.HasPrefix(q, "\"") && strings.HasSuffix(q, "\"") {
		return q
	}
	return "\"" + q + "\""
}
