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
		Use:   "get <user-id>",
		Short: "Get a recipient profile",
		Long: "Returns the channel addresses Courier holds for one user id — email, phone,\n" +
			"push tokens, Slack — which is what decides whether `send --user-id` can be\n" +
			"delivered at all. A 404 means no profile exists, and a send to that id is\n" +
			"still accepted with a 202 before failing to deliver, so checking here is\n" +
			"cheaper than reading the failure out of `message history` afterwards.",
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
		Use:   "subscriptions <user-id>",
		Short: "List the lists a user is subscribed to",
		Long: "Cursor-paginated with --cursor. This is the only direction membership can\n" +
			"be read in — there is no command that enumerates one list's members — so\n" +
			"answering who receives a list means walking the users, not the list.",
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
