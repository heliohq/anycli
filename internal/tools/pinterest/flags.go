package pinterest

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// pageParams holds the shared cursor-pagination flags Pinterest's list
// endpoints accept. Pinterest paginates with an opaque `bookmark` cursor; the
// tool surfaces it (--bookmark / --page-size) and echoes the returned bookmark
// in the raw response body so the assistant drives paging explicitly — no
// hidden auto-follow that could fan out unboundedly.
type pageParams struct {
	pageSize int
	bookmark string
}

// registerPageFlags wires --page-size / --bookmark onto a list command.
func registerPageFlags(cmd *cobra.Command, p *pageParams) {
	cmd.Flags().IntVar(&p.pageSize, "page-size", 0, "max items per page (Pinterest default when omitted)")
	cmd.Flags().StringVar(&p.bookmark, "bookmark", "", "pagination cursor from a previous response")
}

// apply writes the paging params into a query value set, omitting unset ones.
func (p pageParams) apply(q url.Values) {
	if p.pageSize > 0 {
		q.Set("page_size", strconv.Itoa(p.pageSize))
	}
	if p.bookmark != "" {
		q.Set("bookmark", p.bookmark)
	}
}
