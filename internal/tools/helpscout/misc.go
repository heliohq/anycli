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
		Use:   "list",
		Short: "List inboxes (GET /mailboxes)",
		Long: "Help Scout's UI says Inbox and its API says mailbox; they are the same\n" +
			"thing. The numeric ids here are what --mailbox takes on `conversation\n" +
			"list`, `conversation create`, `customer list` and `user list`, and what\n" +
			"--inbox takes on the saved-reply commands. Page with --page.",
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
		Use:   "get <id>",
		Short: "Get one inbox (GET /mailboxes/{id})",
		Long: "One inbox's own record — its name, slug and email address — for an id\n" +
			"already known. It lists neither the conversations in the inbox\n" +
			"(`conversation list --mailbox <id>`) nor its folders (`inbox folders`).",
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
		Use:   "folders <id>",
		Short: "List an inbox's folders (GET /mailboxes/{id}/folders)",
		Long: "Folders are the inbox's queue views, each with its own id and an open\n" +
			"count. Those ids are what `conversation list --folder` takes, and there is\n" +
			"no filter by folder name, so this lookup is unavoidable when scoping a\n" +
			"queue read to one view.",
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
		Use:   "list --inbox <id>",
		Short: "List an inbox's saved replies (GET /mailboxes/{id}/saved-replies)",
		Long: "--inbox is required: saved replies are scoped to one inbox and there is no\n" +
			"account-wide listing, so finding a canned answer means knowing which inbox\n" +
			"it belongs to. Use the ids from here with `saved-reply get` to read the\n" +
			"body, then paste it into `thread reply --text`.",
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
		Use:   "get --inbox <id> <reply-id>",
		Short: "Get one saved reply (GET /mailboxes/{id}/saved-replies/{reply-id})",
		Long: "Takes the reply id positionally AND --inbox, because the endpoint is nested\n" +
			"under the inbox and a reply id alone does not address it. The text comes\n" +
			"back as stored; nothing here sends it, so hand it to `thread reply --text`\n" +
			"once it has been adapted to the conversation.",
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
		Use:   "list",
		Short: "List tags (GET /tags)",
		Long: "The account's whole tag vocabulary, paged with --page. Worth reading before\n" +
			"`conversation tag`, which replaces a conversation's set outright and\n" +
			"creates any name Help Scout does not recognise — checking here is what\n" +
			"keeps a typo from becoming a permanent account-wide tag.",
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
		Use:   "list",
		Short: "List users (GET /users)",
		Long: "Users are staff, a different directory and a different id space from\n" +
			"customers. The ids here are what --assign-to takes on `conversation\n" +
			"update`, `conversation create` and `thread reply`, and what --assigned-to\n" +
			"filters `conversation list` by. --email matches a user's address and\n" +
			"--mailbox narrows to users with access to one inbox.",
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
		Use:   "me",
		Short: "Get the authenticated user (GET /users/me)",
		Long: "The staff account the connected token acts as. Every reply and note written\n" +
			"through this tool is attributed to this user, and its id is the one to\n" +
			"compare a conversation's assignee against before deciding whether a\n" +
			"conversation already belongs to somebody else.",
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
