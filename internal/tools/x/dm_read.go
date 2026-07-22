package x

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newDMListCmd(token string) *cobra.Command {
	var nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List DM events (one page)",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLimit(limit, 1, 100); err != nil {
				return err
			}
			query := dmPageQuery(limit, nextToken)
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/dm_events", query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	addDMPageFlags(cmd, &limit, &nextToken)
	return cmd
}

func (s *Service) newDMGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <event-id>",
		Short:       "Get one DM event",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("DM event id", args[0]); err != nil {
				return err
			}
			query := dmFieldsQuery()
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/dm_events/"+url.PathEscape(args[0]), query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newDMHistoryCmd(token string) *cobra.Command {
	var conversationID, participantID, nextToken string
	var limit int
	cmd := &cobra.Command{
		Use:         "history",
		Short:       "List events for one DM conversation (one page)",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExactlyOne("--conversation-id", conversationID, "--participant-id", participantID); err != nil {
				return err
			}
			if conversationID != "" {
				if err := requireDMConversationID(conversationID); err != nil {
					return err
				}
			}
			if err := requireLimit(limit, 1, 100); err != nil {
				return err
			}
			path := "/2/dm_conversations/" + url.PathEscape(conversationID) + "/dm_events"
			if participantID != "" {
				if err := requireNumericID("participant id", participantID); err != nil {
					return err
				}
				path = "/2/dm_conversations/with/" + url.PathEscape(participantID) + "/dm_events"
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, path, dmPageQuery(limit, nextToken), nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&conversationID, "conversation-id", "", "DM conversation id")
	cmd.Flags().StringVar(&participantID, "participant-id", "", "participant user id for a one-to-one conversation")
	addDMPageFlags(cmd, &limit, &nextToken)
	return cmd
}

func addDMPageFlags(cmd *cobra.Command, limit *int, nextToken *string) {
	cmd.Flags().IntVar(limit, "limit", 20, "maximum DM events in this page (1-100)")
	cmd.Flags().StringVar(nextToken, "next-token", "", "provider token for the next page")
}

func dmPageQuery(limit int, nextToken string) url.Values {
	query := dmFieldsQuery()
	query.Set("max_results", strconv.Itoa(limit))
	if nextToken != "" {
		query.Set("pagination_token", nextToken)
	}
	return query
}

func dmFieldsQuery() url.Values {
	return url.Values{
		"dm_event.fields": {defaultDMFields},
		"expansions":      {defaultDMExpansions},
		"media.fields":    {defaultDMMediaFields},
	}
}
