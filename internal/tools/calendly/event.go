package calendly

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newEventListCmd(token string) *cobra.Command {
	var user string
	var org bool
	var status, inviteeEmail, from, to, sort string
	var count int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List booked meetings (GET /scheduled_events)",
		Long: "Scoped to one user by default, or to every member of the organization with\n" +
			"`--org`, which makes `--user` irrelevant. Without `--status` the result\n" +
			"mixes ACTIVE and CANCELED meetings, so a count of \"meetings this week\"\n" +
			"needs `--status active`. `--from`/`--to` bound the meeting start time,\n" +
			"`--invitee-email` finds meetings with one person without knowing the event,\n" +
			"and `--sort` takes provider keys such as `start_time:asc`. Each entry\n" +
			"carries the `uri` that `event get`, `event invitees` and `event cancel`\n" +
			"take.",
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
			if status != "" {
				q.Set("status", status)
			}
			if inviteeEmail != "" {
				q.Set("invitee_email", inviteeEmail)
			}
			if from != "" {
				q.Set("min_start_time", from)
			}
			if to != "" {
				q.Set("max_start_time", to)
			}
			if sort != "" {
				q.Set("sort", sort)
			}
			addPaging(q, count, pageToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/scheduled_events", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&user, "user", "me", "user URI, bare UUID, or \"me\" (ignored with --org)")
	cmd.Flags().BoolVar(&org, "org", false, "scope to the current organization instead of a user")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active|canceled")
	cmd.Flags().StringVar(&inviteeEmail, "invitee-email", "", "filter by invitee email")
	cmd.Flags().StringVar(&from, "from", "", "min_start_time (ISO-8601)")
	cmd.Flags().StringVar(&to, "to", "", "max_start_time (ISO-8601)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order, e.g. start_time:asc")
	cmd.Flags().IntVar(&count, "count", 0, "page size (cursor pagination)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination cursor")
	return cmd
}

func (s *Service) newEventGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <event-id|uri>",
		Short: "Inspect one booked meeting (GET /scheduled_events/{uuid})",
		Long: "Returns the meeting itself: name, start and end times, location or join\n" +
			"URL, status, and the host memberships. It does NOT carry who booked, their\n" +
			"answers or their cancel and reschedule links — that is `event invitees`, a\n" +
			"separate call against the same id.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/scheduled_events/"+url.PathEscape(uuidOf(args[0])), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newEventInviteesCmd(token string) *cobra.Command {
	var status, email string
	var count int
	var pageToken string
	cmd := &cobra.Command{
		Use:   "invitees <event-id|uri>",
		Short: "Who booked, Q&A answers, cancel/reschedule URLs (GET /scheduled_events/{uuid}/invitees)",
		Long: "The invitee side of a meeting: names, emails, timezones, answers to the\n" +
			"event type's custom questions, and the per-invitee `cancel_url` and\n" +
			"`reschedule_url`. Since the API has no reschedule endpoint, that\n" +
			"`reschedule_url` IS the reschedule path — send it to the invitee. It is\n" +
			"also the only source of the full invitee URI that `invitee no-show`\n" +
			"requires. `--status active|canceled` and `--email` filter; group event\n" +
			"types can have many invitees, so it paginates with `--count` and\n" +
			"`--page-token`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if email != "" {
				q.Set("email", email)
			}
			addPaging(q, count, pageToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/scheduled_events/"+url.PathEscape(uuidOf(args[0]))+"/invitees", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status: active|canceled")
	cmd.Flags().StringVar(&email, "email", "", "filter by invitee email")
	cmd.Flags().IntVar(&count, "count", 0, "page size (cursor pagination)")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination cursor")
	return cmd
}

// newEventCancelCmd wraps POST /scheduled_events/{uuid}/cancellation. There is
// no reschedule endpoint — to reschedule, share the invitee's reschedule_url
// (from `event invitees`) or cancel and send a new booking link.
func (s *Service) newEventCancelCmd(token string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "cancel <event-id|uri>",
		Short: "Cancel a booked meeting with a reason (POST /scheduled_events/{uuid}/cancellation)",
		Long: "Calendly emails every invitee when the meeting is cancelled and `--reason`\n" +
			"is the text they see, so omitting it sends a cancellation with no\n" +
			"explanation. It cancels for ALL invitees of a group event, not one of\n" +
			"them. There is no un-cancel: putting the meeting back means a fresh booking\n" +
			"link or `book create`. When the invitee rather than the host should be the\n" +
			"one cancelling, send them the `cancel_url` from `event invitees` instead.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if reason != "" {
				body["reason"] = reason
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/scheduled_events/"+url.PathEscape(uuidOf(args[0]))+"/cancellation", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "cancellation reason shown to the invitee")
	return cmd
}
