package instantly

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// pageFlags are the shared cursor-pagination flags Instantly list endpoints
// accept. Values are only sent when set, so an omitted flag leaves the
// provider default in force.
type pageFlags struct {
	limit         int
	startingAfter string
}

// registerPageFlags wires --limit / --starting-after onto a list command.
func registerPageFlags(cmd *cobra.Command, p *pageFlags) {
	cmd.Flags().IntVar(&p.limit, "limit", 0, "max items to return (0 = provider default)")
	cmd.Flags().StringVar(&p.startingAfter, "starting-after", "", "pagination cursor from a prior response's next_starting_after")
}

// applyQuery writes the pagination params into a query value set.
func (p pageFlags) applyQuery(q url.Values) {
	if p.limit > 0 {
		q.Set("limit", strconv.Itoa(p.limit))
	}
	if p.startingAfter != "" {
		q.Set("starting_after", p.startingAfter)
	}
}

// applyBody writes the pagination params into a request-body map (for the
// POST-based `lead list`).
func (p pageFlags) applyBody(m map[string]any) {
	if p.limit > 0 {
		m["limit"] = p.limit
	}
	if p.startingAfter != "" {
		m["starting_after"] = p.startingAfter
	}
}

// setIfChanged copies a string flag into a query set only when the user set it,
// keyed by the provider query-parameter name.
func setIfChanged(cmd *cobra.Command, q url.Values, flag, param, value string) {
	if cmd.Flags().Changed(flag) {
		q.Set(param, value)
	}
}

// setBodyIfChanged copies a string flag into a request-body map only when the
// user set it, keyed by the provider field name.
func setBodyIfChanged(cmd *cobra.Command, body map[string]any, flag, field, value string) {
	if cmd.Flags().Changed(flag) {
		body[field] = value
	}
}
