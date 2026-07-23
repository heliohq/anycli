package courier

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAudienceGetCmd builds `audience get <id>` — GET /audiences/{id}.
func (s *Service) newAudienceGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <audience-id>",
		Short:       "Get an audience",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newAudienceListCmd builds `audience list` — GET /audiences with cursor paging.
func (s *Service) newAudienceListCmd(key string) *cobra.Command {
	var cursor string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List audiences (cursor-paginated)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "cursor", cursor)
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/audiences", q, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor for the next page")
	return cmd
}
