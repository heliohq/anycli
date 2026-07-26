package delighted

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newPeopleCmd wires the people resource: list, send (create/update + schedule),
// delete, and cancel-pending over /people(.json).
func (s *Service) newPeopleCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "people", Short: "Survey recipients (people)"}
	cmd.AddCommand(
		s.newPeopleListCmd(key),
		s.newPeopleSendCmd(key),
		s.newPeopleDeleteCmd(key),
		s.newPeopleCancelPendingCmd(key),
	)
	return cmd
}

func (s *Service) newPeopleListCmd(key string) *cobra.Command {
	var pageInfo string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List people (GET /people.json, cursor pagination via page_info)",
		Long: "The roster of survey recipients, and the way to turn an email address into\n" +
			"the person id that `response create` needs. Paging is cursor-based: take\n" +
			"the opaque cursor Delighted returns in the Link header and pass it as\n" +
			"`--page-info`. The `--page` number this command inherits from the other\n" +
			"lists is ignored — asking for page 2 silently re-reads page 1.",
		Args: cobra.NoArgs,
	}
	cmd.Annotations = readOnly
	perPage, _ := registerPaging(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		q := url.Values{}
		if *perPage > 0 {
			q.Set("per_page", intToString(*perPage))
		}
		setIfNonEmpty(q, "page_info", pageInfo)
		resp, err := s.call(cmd.Context(), key, http.MethodGet, "/people.json", q, nil)
		if err != nil {
			return err
		}
		return s.emit(resp)
	}
	cmd.Flags().StringVar(&pageInfo, "page-info", "", "opaque cursor from a prior page's Link header")
	return cmd
}

func (s *Service) newPeopleSendCmd(key string) *cobra.Command {
	var email, name, properties, delay, channel string
	var send bool
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Create/update a person and optionally schedule a survey (POST /people.json)",
		Long: "This is both the create and the update path — the email address is the\n" +
			"key, so a second call with the same `--email` edits the existing person\n" +
			"instead of duplicating them. It also MAILS them: a survey goes out unless\n" +
			"`--send=false` is passed explicitly, so use that to register someone\n" +
			"quietly. `--delay` holds the send for that many seconds, `--channel` picks\n" +
			"email or sms, and `--properties-json` sets the custom attributes reports\n" +
			"segment on. Delighted's own throttling may still suppress a survey to\n" +
			"someone surveyed recently.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"email": email}
			if name != "" {
				payload["name"] = name
			}
			if cmd.Flags().Changed("send") {
				payload["send"] = send
			}
			if delay != "" {
				payload["delay"] = delay
			}
			if channel != "" {
				payload["channel"] = channel
			}
			if properties != "" {
				v, err := decodeJSONFlag("properties-json", properties)
				if err != nil {
					return err
				}
				payload["properties"] = v
			}
			resp, err := s.call(cmd.Context(), key, http.MethodPost, "/people.json", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "person's email address")
	cmd.Flags().StringVar(&name, "name", "", "person's name")
	cmd.Flags().BoolVar(&send, "send", true, "whether to schedule a survey now")
	cmd.Flags().StringVar(&delay, "delay", "", "seconds to delay the survey send")
	cmd.Flags().StringVar(&channel, "channel", "", "survey channel: email or sms")
	cmd.Flags().StringVar(&properties, "properties-json", "", "custom person properties as a raw JSON object")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func (s *Service) newPeopleDeleteCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "GDPR-delete a person and their data (DELETE /people/{id}.json)",
		Long: "An erasure, not an opt-out: it removes the person AND their survey\n" +
			"responses, which lowers the historical scores `metrics get` reports. It\n" +
			"cannot be undone and nothing here restores the data. To stop mailing\n" +
			"someone while keeping their feedback, use `unsubscribes add`, or\n" +
			"`people cancel-pending` to drop only the surveys not yet sent. `--id` is\n" +
			"the person id from `people list`, not an email.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, "/people/"+url.PathEscape(id)+".json", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "person id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newPeopleCancelPendingCmd(key string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "cancel-pending",
		Short: "Cancel a person's scheduled-but-unsent surveys (DELETE /people/{email}/survey_requests/pending.json)",
		Long: "Addresses the person by `--email`, not by id, unlike `people delete`. It\n" +
			"clears only survey requests still queued — one already delivered cannot be\n" +
			"recalled, and the person stays on the roster and eligible for future\n" +
			"sends. This is the undo for a `people send` fired with the wrong\n" +
			"`--delay`, not a way to unsubscribe someone.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/people/" + url.PathEscape(email) + "/survey_requests/pending.json"
			resp, err := s.call(cmd.Context(), key, http.MethodDelete, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "person's email address")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
