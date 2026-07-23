package courier

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newProfileGetCmd builds `profile get <id>` — GET /profiles/{id}, the
// recipient's channels on file.
func (s *Service) newProfileGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <user-id>",
		Short:       "Get a recipient profile",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/profiles/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newProfileSubscriptionsCmd builds `profile subscriptions <id>` —
// GET /profiles/{id}/lists, the lists a user is subscribed to.
func (s *Service) newProfileSubscriptionsCmd(key string) *cobra.Command {
	var cursor string
	cmd := &cobra.Command{
		Use:         "subscriptions <user-id>",
		Short:       "List the lists a user is subscribed to",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setIf(q, "cursor", cursor)
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/profiles/"+url.PathEscape(args[0])+"/lists", q, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor for the next page")
	return cmd
}
