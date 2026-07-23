package hootsuite

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newMessageScheduleCmd schedules (or sends soonest-possible) a post fanned out
// across one or more social profiles.
func (s *Service) newMessageScheduleCmd(token string) *cobra.Command {
	var (
		text           string
		profiles       []string
		sendTime       string
		tags           []string
		mediaIDs       []string
		emailNotify    bool
		boardID        string
		destinationURL string
	)
	cmd := &cobra.Command{
		Use:         "schedule",
		Short:       "Schedule or send a post (POST /v1/messages)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if text == "" {
				return &usageError{msg: "--text is required"}
			}
			if len(profiles) == 0 {
				return &usageError{msg: "at least one --profile is required"}
			}
			ids, err := parseProfileIDs(profiles)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"text":              text,
				"socialProfileIds":  ids,
				"emailNotification": emailNotify,
			}
			if sendTime != "" {
				if err := validateUTC("send-time", sendTime); err != nil {
					return err
				}
				payload["scheduledSendTime"] = sendTime
			}
			if len(tags) > 0 {
				payload["tags"] = tags
			}
			if len(mediaIDs) > 0 {
				media := make([]map[string]string, 0, len(mediaIDs))
				for _, id := range mediaIDs {
					media = append(media, map[string]string{"id": id})
				}
				payload["media"] = media
			}
			// Pinterest: a board post cannot be bundled with other profiles and
			// needs an extendedInfo entry carrying the board + destination.
			if boardID != "" || destinationURL != "" {
				if boardID == "" || destinationURL == "" {
					return &usageError{msg: "--board-id and --destination-url must be provided together for a Pinterest post"}
				}
				if len(ids) != 1 {
					return &usageError{msg: "a Pinterest post (--board-id) cannot be bundled with other profiles; pass exactly one --profile"}
				}
				payload["extendedInfo"] = []map[string]any{{
					"socialProfileType": "PINTEREST",
					"socialProfileId":   ids[0],
					"data": map[string]string{
						"boardId":        boardID,
						"destinationUrl": destinationURL,
					},
				}}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/messages", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "post text (required)")
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "target social profile id (repeatable; numeric)")
	cmd.Flags().StringVar(&sendTime, "send-time", "", "UTC ISO-8601 send time ending in Z (omit for soonest possible)")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "message tag (repeatable)")
	cmd.Flags().StringArrayVar(&mediaIDs, "media-id", nil, "attached media id from `media create` (repeatable)")
	cmd.Flags().BoolVar(&emailNotify, "email-notification", false, "email the author on send")
	cmd.Flags().StringVar(&boardID, "board-id", "", "Pinterest board id (requires --destination-url; single --profile)")
	cmd.Flags().StringVar(&destinationURL, "destination-url", "", "Pinterest destination URL (requires --board-id)")
	return cmd
}

// newMessageListCmd lists scheduled/queued posts with optional filters.
func (s *Service) newMessageListCmd(token string) *cobra.Command {
	var (
		state    string
		start    string
		end      string
		profiles []string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List scheduled/queued posts (GET /v1/messages)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if state != "" {
				q.Set("state", state)
			}
			if start != "" {
				if err := validateUTC("start", start); err != nil {
					return err
				}
				q.Set("startTime", start)
			}
			if end != "" {
				if err := validateUTC("end", end); err != nil {
					return err
				}
				q.Set("endTime", end)
			}
			for _, p := range profiles {
				q.Add("socialProfileIds", p)
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&state, "state", "", "filter by state (e.g. SCHEDULED, SENT)")
	cmd.Flags().StringVar(&start, "start", "", "startTime filter (UTC ISO-8601 ending in Z)")
	cmd.Flags().StringVar(&end, "end", "", "endTime filter (UTC ISO-8601 ending in Z)")
	cmd.Flags().StringArrayVar(&profiles, "profile", nil, "filter by social profile id (repeatable)")
	return cmd
}

// newMessageGetCmd fetches one message by id.
func (s *Service) newMessageGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one message (GET /v1/messages/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/messages/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newMessageDeleteCmd unschedules/deletes a message.
func (s *Service) newMessageDeleteCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Unschedule/delete a message (DELETE /v1/messages/{id})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodDelete, "/messages/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newMessageApproveCmd approves a message in an approval workflow.
func (s *Service) newMessageApproveCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "approve <id>",
		Short:       "Approve a message (POST /v1/messages/{id}/approve)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/messages/"+url.PathEscape(args[0])+"/approve", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

// newMessageRejectCmd rejects a message in an approval workflow.
func (s *Service) newMessageRejectCmd(token string) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:         "reject <id>",
		Short:       "Reject a message (POST /v1/messages/{id}/reject)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var payload any
			if reason != "" {
				payload = map[string]string{"reason": reason}
			}
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/messages/"+url.PathEscape(args[0])+"/reject", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "rejection reason")
	return cmd
}

// parseProfileIDs converts numeric --profile flag values to int64s (Hootsuite
// socialProfileIds are numeric). A non-numeric value is a usage error.
func parseProfileIDs(profiles []string) ([]int64, error) {
	ids := make([]int64, 0, len(profiles))
	for _, p := range profiles {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, &usageError{msg: "--profile must be a numeric social profile id (got " + strconv.Quote(p) + ")"}
		}
		ids = append(ids, n)
	}
	return ids, nil
}
