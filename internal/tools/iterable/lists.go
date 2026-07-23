package iterable

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListCmd groups the list (subscription-list) verbs.
func (s *Service) newListCmd(cred credential) *cobra.Command {
	cmd := newGroupCmd("list", "Manage subscription lists and membership")
	cmd.AddCommand(
		s.newListListCmd(cred),
		s.newListSubscribeCmd(cred),
		s.newListUnsubscribeCmd(cred),
		s.newListUsersCmd(cred),
	)
	return cmd
}

func (s *Service) newListListCmd(cred credential) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all lists (GET /api/lists)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/lists", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}

func (s *Service) newListSubscribeCmd(cred credential) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Add users to a list (POST /api/lists/subscribe)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("body", body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodPost, "/api/lists/subscribe", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `JSON body, e.g. {"listId":123,"subscribers":[{"email":"a@b.com"}]} (required)`)
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newListUnsubscribeCmd(cred credential) *cobra.Command {
	var body string
	cmd := &cobra.Command{
		Use:   "unsubscribe",
		Short: "Remove users from a list (POST /api/lists/unsubscribe)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := decodeJSONFlag("body", body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), cred, http.MethodPost, "/api/lists/unsubscribe", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `JSON body, e.g. {"listId":123,"subscribers":[{"email":"a@b.com"}]} (required)`)
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func (s *Service) newListUsersCmd(cred credential) *cobra.Command {
	var listID string
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List the emails on a list (GET /api/lists/getUsers?listId=…)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if listID == "" {
				return &usageError{msg: "iterable: --list-id is required"}
			}
			query := url.Values{"listId": {listID}}
			resp, err := s.call(cmd.Context(), cred, http.MethodGet, "/api/lists/getUsers", query, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&listID, "list-id", "", "list id (required)")
	_ = cmd.MarkFlagRequired("list-id")
	return cmd
}
