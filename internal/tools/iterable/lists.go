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
		Long: "Returns every subscription list in the project with its numeric `listId` —\n" +
			"the id that `list subscribe`, `list unsubscribe` and `list users` all\n" +
			"require. There are no filter or pagination flags, so match on the name\n" +
			"locally.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "--body is required and takes a numeric `listId` from `list list` plus a\n" +
			"`subscribers` array whose entries identify people by email or userId.\n" +
			"Subscribing makes those people eligible for the list's campaigns, so this\n" +
			"can drop real recipients into a live send. One call carries many\n" +
			"subscribers — batch rather than looping.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "--body has the same shape as `list subscribe`: a numeric `listId` from\n" +
			"`list list` plus a `subscribers` array. Removing someone from one list is\n" +
			"not a global opt-out — they remain on every other list in the project. The\n" +
			"only way back is a fresh `list subscribe`, which is a new subscription\n" +
			"rather than a restore.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "--list-id is required and is the numeric id from `list list`. It returns\n" +
			"member email addresses only, not full profiles — pair it with `user get`\n" +
			"when attributes are needed. There is no paging flag, so a large list\n" +
			"arrives in a single response.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
