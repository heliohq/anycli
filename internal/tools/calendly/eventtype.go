package calendly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newEventTypeListCmd(token string) *cobra.Command {
	var user string
	var org bool
	var active string
	var count int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bookable event types (GET /event_types)",
		Long: "Event types are the bookable meeting kinds. Each entry carries the public\n" +
			"`scheduling_url` that is shareable as-is and the `uri` that\n" +
			"`availability slots`, `link create` and `book create` all require. Defaults\n" +
			"to the connected user's own types; `--org` widens the listing to the whole\n" +
			"organization and makes `--user` irrelevant. `--active true|false` separates\n" +
			"live event types from hidden ones. Paginate with `--count` and\n" +
			"`--page-token`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if org {
				_, orgURI, err := s.resolveMe(cmd.Context(), token)
				if err != nil {
					return err
				}
				if orgURI == "" {
					return &usageError{msg: "calendly: no current_organization on /users/me; cannot scope --org"}
				}
				q.Set("organization", orgURI)
			} else {
				userURI, err := s.resolveUserURI(cmd.Context(), token, user)
				if err != nil {
					return err
				}
				q.Set("user", userURI)
			}
			if active != "" {
				q.Set("active", active)
			}
			addPaging(q, count, pageToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/event_types", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&user, "user", "me", "user URI, bare UUID, or \"me\" (ignored with --org)")
	cmd.Flags().BoolVar(&org, "org", false, "scope to the current organization instead of a user")
	cmd.Flags().StringVar(&active, "active", "", "filter by active state: true|false")
	cmd.Flags().IntVar(&count, "count", 0, "page size (cursor pagination)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination cursor")
	return cmd
}

func (s *Service) newEventTypeGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <event-type-id|uri>",
		Short: "Get one event type (GET /event_types/{uuid})",
		Long: "Returns the duration, location rules, custom booking questions and\n" +
			"`scheduling_url` for one event type. Worth reading before `book create`:\n" +
			"the `--location-kind` that command accepts is dictated by THIS event type's\n" +
			"location configuration, and its custom questions are the ones invitees\n" +
			"answered in `event invitees`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/event_types/"+url.PathEscape(uuidOf(args[0])), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	return cmd
}
