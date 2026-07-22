package gorgias

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// pageFlags holds the cursor-pagination flags shared by every Gorgias list
// endpoint: cursor, limit (1–100, provider default 30), and order_by.
type pageFlags struct {
	cursor  string
	limit   int
	orderBy string
}

// register wires --cursor / --limit / --order-by onto a list command. limit
// defaults to 0 so it is only sent when the caller sets it (letting Gorgias
// apply its own default of 30).
func (p *pageFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&p.cursor, "cursor", "", "pagination cursor (from meta.next_cursor)")
	cmd.Flags().IntVar(&p.limit, "limit", 0, "page size (1-100, provider default 30)")
	cmd.Flags().StringVar(&p.orderBy, "order-by", "", "sort attribute, e.g. created_datetime:desc")
}

// apply writes the set pagination flags into a query value set.
func (p pageFlags) apply(q url.Values) {
	if p.cursor != "" {
		q.Set("cursor", p.cursor)
	}
	if p.limit > 0 {
		q.Set("limit", strconv.Itoa(p.limit))
	}
	if p.orderBy != "" {
		q.Set("order_by", p.orderBy)
	}
}
