package missive

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newContactBooksCmd builds the `contact-books` group. Contact books are the
// containers a contacts list must be scoped to (contacts list requires one).
func (s *Service) newContactBooksCmd(token string) *cobra.Command {
	group := newGroupCmd("contact-books", "List contact books (containers for contacts)")
	group.AddCommand(s.newContactBooksListCmd(token))
	return group
}

func (s *Service) newContactBooksListCmd(token string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contact books",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/contact_books", q, nil)
			if err != nil {
				return err
			}
			return s.emitOffsetList(body, "contact_books", offset, limit)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max contact books (Missive max 200)")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}
