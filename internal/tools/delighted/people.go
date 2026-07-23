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
		Args:  cobra.NoArgs,
	}
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
