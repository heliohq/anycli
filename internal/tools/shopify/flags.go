package shopify

import "github.com/spf13/cobra"

// side-effect annotation values (design 318). Reads are "false"; writes and the
// raw graphql passthrough (which can mutate) are "true".
const (
	sideEffectRead  = "false"
	sideEffectWrite = "true"
)

func readAnnotation() map[string]string {
	return map[string]string{"anycli.side_effect": sideEffectRead}
}
func writeAnnotation() map[string]string {
	return map[string]string{"anycli.side_effect": sideEffectWrite}
}

// apiVersion resolves the persistent --api-version flag; empty falls back to the
// service default at request time.
func apiVersion(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("api-version")
	return v
}

// listFlags holds the cursor-pagination + search flags shared by every list
// command. Shopify uses first/after GraphQL connection pagination; the service
// never auto-follows unbounded pages.
type listFlags struct {
	limit int
	after string
	query string
}

// registerListFlags attaches --limit / --after / --query to a list command.
func registerListFlags(cmd *cobra.Command) *listFlags {
	lf := &listFlags{}
	cmd.Flags().IntVar(&lf.limit, "limit", 20, "max items to return (GraphQL first)")
	cmd.Flags().StringVar(&lf.after, "after", "", "resume from a prior response's end_cursor")
	cmd.Flags().StringVar(&lf.query, "query", "", "Shopify search query filter")
	return lf
}

// vars builds the common connection variables ($first, $after, $query) from the
// list flags, clamping first into Shopify's 1..250 range.
func (lf *listFlags) vars() map[string]any {
	first := lf.limit
	if first <= 0 {
		first = 20
	}
	if first > 250 {
		first = 250
	}
	v := map[string]any{"first": first}
	if lf.after != "" {
		v["after"] = lf.after
	}
	if lf.query != "" {
		v["query"] = lf.query
	}
	return v
}
