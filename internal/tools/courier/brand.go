package courier

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newBrandGetCmd builds `brand get <id>` — GET /brands/{id}.
func (s *Service) newBrandGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <brand-id>",
		Short:       "Get a brand",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/brands/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newBrandListCmd builds `brand list` — GET /brands with cursor paging.
func (s *Service) newBrandListCmd(key string) *cobra.Command {
	var cursor string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List brands (cursor-paginated)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "cursor", cursor)
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/brands", q, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor for the next page")
	return cmd
}
