package courier

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListGetCmd builds `list get <id>` — GET /lists/{id}.
func (s *Service) newListGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <list-id>",
		Short:       "Get a mailing list",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/lists/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newListListCmd builds `list list` — GET /lists with optional cursor + pattern.
func (s *Service) newListListCmd(key string) *cobra.Command {
	var cursor, pattern string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List mailing lists (cursor-paginated)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "cursor", cursor)
			setIf(q, "pattern", pattern)
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/lists", q, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor for the next page")
	cmd.Flags().StringVar(&pattern, "pattern", "", "filter by list-id pattern (* / ** wildcards)")
	return cmd
}

// newListSubscribeCmd builds `list subscribe <list-id> <user-id>` —
// PUT /lists/{id}/subscriptions/{user}.
func (s *Service) newListSubscribeCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "subscribe <list-id> <user-id>",
		Short:       "Subscribe a user to a list",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/lists/" + url.PathEscape(args[0]) + "/subscriptions/" + url.PathEscape(args[1])
			out, err := s.call(cmd.Context(), key, http.MethodPut, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newListUnsubscribeCmd builds `list unsubscribe <list-id> <user-id>` —
// DELETE /lists/{id}/subscriptions/{user}.
func (s *Service) newListUnsubscribeCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "unsubscribe <list-id> <user-id>",
		Short:       "Unsubscribe a user from a list",
		Args:        cobra.ExactArgs(2),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/lists/" + url.PathEscape(args[0]) + "/subscriptions/" + url.PathEscape(args[1])
			out, err := s.call(cmd.Context(), key, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}
