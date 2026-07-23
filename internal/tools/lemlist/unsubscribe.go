package lemlist

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newUnsubscribeCmd manages the account-wide suppression list (compliance).
func (s *Service) newUnsubscribeCmd(key string) *cobra.Command {
	cmd := newGroupCmd("unsubscribe", "Suppression list: list, add, delete (email or domain)")
	cmd.AddCommand(
		s.newUnsubscribeListCmd(key),
		s.newUnsubscribeAddCmd(key),
		s.newUnsubscribeDeleteCmd(key),
	)
	return cmd
}

func (s *Service) newUnsubscribeListCmd(key string) *cobra.Command {
	var offset, limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the suppression entries (GET /unsubscribes)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			body, err := s.call(cmd.Context(), key, http.MethodGet, "/unsubscribes", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().IntVar(&offset, "offset", 0, "records to skip (pagination)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max entries to return")
	return cmd
}

func (s *Service) newUnsubscribeAddCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "add <emailOrDomain>",
		Short:       "Add an email or domain to the suppression list (POST /unsubscribes/{email})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodPost, "/unsubscribes/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newUnsubscribeDeleteCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <emailOrDomain>",
		Short:       "Remove an email or domain from the suppression list (DELETE /unsubscribes/{email})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), key, http.MethodDelete, "/unsubscribes/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}
