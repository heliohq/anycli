package iterable

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// newEventCmd groups the custom-event verbs.
func (s *Service) newEventCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("event", "Track and read custom events")
	cmd.AddCommand(
		s.newEventTrackCmd(cred),
		s.newEventListCmd(cred),
	)
	return cmd
}

func (s *Service) newEventTrackCmd(cred credential) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "track",
		Short: "Track a custom event (POST /api/events/track)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("body", body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodPost, "/api/events/track", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `JSON body, e.g. {"email":"a@b.com","eventName":"signup"} (required)`)
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newEventListCmd(cred credential) *cobra.Command {
	var email string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a user's events by email (GET /api/events/{email})",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return &usageError{msg: "iterable: --email is required"}
			}
			query := url.Values{}
			if limit > 0 {
				query.Set("limit", strconv.Itoa(limit))
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/events/"+url.PathEscape(email), query, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "user email (required)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max events to return (0 = provider default)")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
