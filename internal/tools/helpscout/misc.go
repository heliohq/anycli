package helpscout

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// --- inbox (mailbox) ---

func (s *Service) newInboxCmd(token string) *cobra.Command {
	cmd := newGroupCmd("inbox", "List inboxes and their folders")
	cmd.AddCommand(
		s.newInboxListCmd(token),
		s.newInboxGetCmd(token),
		s.newInboxFoldersCmd(token),
	)
	return cmd
}

func (s *Service) newInboxListCmd(token string) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List inboxes (GET /mailboxes)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/mailboxes", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

func (s *Service) newInboxGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one inbox (GET /mailboxes/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/mailboxes/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	return cmd
}

func (s *Service) newInboxFoldersCmd(token string) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:         "folders <id>",
		Short:       "List an inbox's folders (GET /mailboxes/{id}/folders)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/mailboxes/"+url.PathEscape(args[0])+"/folders", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

// --- saved-reply ---

func (s *Service) newSavedReplyCmd(token string) *cobra.Command {
	cmd := newGroupCmd("saved-reply", "Read saved replies for consistent drafting")
	cmd.AddCommand(
		s.newSavedReplyListCmd(token),
		s.newSavedReplyGetCmd(token),
	)
	return cmd
}

func (s *Service) newSavedReplyListCmd(token string) *cobra.Command {
	var inbox string
	cmd := &cobra.Command{
		Use:         "list --inbox <id>",
		Short:       "List an inbox's saved replies (GET /mailboxes/{id}/saved-replies)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/mailboxes/"+url.PathEscape(inbox)+"/saved-replies", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().StringVar(&inbox, "inbox", "", "inbox id (required)")
	_ = cmd.MarkFlagRequired("inbox")
	return cmd
}

func (s *Service) newSavedReplyGetCmd(token string) *cobra.Command {
	var inbox string
	cmd := &cobra.Command{
		Use:         "get --inbox <id> <reply-id>",
		Short:       "Get one saved reply (GET /mailboxes/{id}/saved-replies/{reply-id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/mailboxes/"+url.PathEscape(inbox)+"/saved-replies/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().StringVar(&inbox, "inbox", "", "inbox id (required)")
	_ = cmd.MarkFlagRequired("inbox")
	return cmd
}

// --- tag ---

func (s *Service) newTagCmd(token string) *cobra.Command {
	cmd := newGroupCmd("tag", "Read the account's tags")
	cmd.AddCommand(s.newTagListCmd(token))
	return cmd
}

func (s *Service) newTagListCmd(token string) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List tags (GET /tags)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/tags", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

// --- user ---

func (s *Service) newUserCmd(token string) *cobra.Command {
	cmd := newGroupCmd("user", "List users and read the authenticated user")
	cmd.AddCommand(
		s.newUserListCmd(token),
		s.newUserMeCmd(token),
	)
	return cmd
}

func (s *Service) newUserListCmd(token string) *cobra.Command {
	var email, mailbox string
	var page int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List users (GET /users)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "email", email)
			setIf(q, "mailbox", mailbox)
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email filter")
	cmd.Flags().StringVar(&mailbox, "mailbox", "", "inbox id filter")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

func (s *Service) newUserMeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "me",
		Short:       "Get the authenticated user (GET /users/me)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/users/me", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	return cmd
}
