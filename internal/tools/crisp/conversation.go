package crisp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newConversationCmd builds the `crisp conversation` resource group.
func (s *Service) newConversationCmd(token string) *cobra.Command {
	group := &cobra.Command{
		Use:   "conversation",
		Short: "Website inbox conversations",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	group.AddCommand(
		s.newConversationListCmd(token),
		s.newConversationGetCmd(token),
		s.newConversationMessagesCmd(token),
		s.newConversationReplyCmd(token),
		s.newConversationStateCmd(token),
		s.newConversationRouteCmd(token),
	)
	return group
}

func (s *Service) newConversationListCmd(token string) *cobra.Command {
	var page int
	var filterStatus string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List (triage) conversations in the website inbox",
		Long: "The entry point: every other conversation verb needs a `session_id` and this\n" +
			"is where one comes from. `--filter-status` accepts only resolved or\n" +
			"unresolved and is rejected locally otherwise; leaving it off returns both,\n" +
			"including pending threads. `--page` is 1-based and the page size is Crisp's\n" +
			"to choose — there is no size flag and no total count in the response, so\n" +
			"walk pages until one comes back short.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			query := url.Values{}
			switch filterStatus {
			case "":
				// no filter
			case "resolved":
				query.Set("filter_resolved", "1")
			case "unresolved":
				query.Set("filter_not_resolved", "1")
			default:
				return &usageError{msg: "--filter-status must be resolved or unresolved"}
			}
			path := fmt.Sprintf("/website/%s/conversations/%d", website, page)
			data, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "page": page})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().StringVar(&filterStatus, "filter-status", "", "filter by state: resolved|unresolved")
	return cmd
}

func (s *Service) newConversationGetCmd(token string) *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read one conversation thread",
		Long: "Returns the thread's metadata — state, assignment, the visitor's profile\n" +
			"stub, unread counts and the last message preview — but NOT the message\n" +
			"history. For the messages themselves call `conversation messages` with the\n" +
			"same `--session`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "session"); err != nil {
				return err
			}
			path := fmt.Sprintf("/website/%s/conversation/%s", website, session)
			data, err := s.call(cmd.Context(), token, http.MethodGet, path, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "session": session})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "conversation session_id (required)")
	return cmd
}

func (s *Service) newConversationMessagesCmd(token string) *cobra.Command {
	var session string
	var before int64
	cmd := &cobra.Command{
		Use:   "messages",
		Short: "Read a conversation's messages (latest page; --before to paginate)",
		Long: "Paginates by TIME, not page number: `--before` takes a Unix-MILLISECOND\n" +
			"timestamp and returns messages older than it, so walk backwards by feeding\n" +
			"the oldest returned message's timestamp into the next call. A seconds-based\n" +
			"timestamp silently matches nothing useful. Each message carries a `from`\n" +
			"of user or operator, which is what distinguishes the customer from the\n" +
			"support side.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "session"); err != nil {
				return err
			}
			query := url.Values{}
			if before > 0 {
				query.Set("timestamp_before", fmt.Sprintf("%d", before))
			}
			path := fmt.Sprintf("/website/%s/conversation/%s/messages", website, session)
			data, err := s.call(cmd.Context(), token, http.MethodGet, path, query, nil)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "session": session})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "conversation session_id (required)")
	cmd.Flags().Int64Var(&before, "before", 0, "return messages before this Unix-ms timestamp (pagination)")
	return cmd
}

func (s *Service) newConversationReplyCmd(token string) *cobra.Command {
	var session, text, from string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Send a text message to a customer in a conversation",
		Long: "Posts a plain-text chat message that reaches the customer immediately, with\n" +
			"no draft state and no way to edit or unsend it afterwards. `--from` defaults\n" +
			"to operator; the only other accepted value is user, which writes the message\n" +
			"INTO the thread as though the visitor had said it — useful for logging what\n" +
			"a customer said elsewhere, wrong for answering them. Text only: attachments,\n" +
			"files and rich message types are not reachable from here.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "session"); err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "text"); err != nil {
				return err
			}
			if from != "operator" && from != "user" {
				return &usageError{msg: "--from must be operator or user"}
			}
			body := map[string]any{
				"type":    "text",
				"from":    from,
				"origin":  "chat",
				"content": text,
			}
			path := fmt.Sprintf("/website/%s/conversation/%s/message", website, session)
			data, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "session": session})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "conversation session_id (required)")
	cmd.Flags().StringVar(&text, "text", "", "message text (required)")
	cmd.Flags().StringVar(&from, "from", "operator", "sender: operator|user")
	return cmd
}

func (s *Service) newConversationStateCmd(token string) *cobra.Command {
	var session, state string
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Change a conversation's state (resolve / reopen)",
		Long: "`--state` accepts resolved, pending or unresolved and is checked locally\n" +
			"before any request. They are a flat set, not a workflow, so reopening is\n" +
			"just setting unresolved again and pending is the middle ground for a triaged\n" +
			"but unfinished thread. Changing the state leaves the assignment alone —\n" +
			"`conversation route` is a separate call.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "session"); err != nil {
				return err
			}
			switch state {
			case "resolved", "pending", "unresolved":
			default:
				return &usageError{msg: "--state must be resolved, pending, or unresolved"}
			}
			body := map[string]any{"state": state}
			path := fmt.Sprintf("/website/%s/conversation/%s/state", website, session)
			data, err := s.call(cmd.Context(), token, http.MethodPatch, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "session": session})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "conversation session_id (required)")
	cmd.Flags().StringVar(&state, "state", "", "new state: resolved|pending|unresolved (required)")
	return cmd
}

func (s *Service) newConversationRouteCmd(token string) *cobra.Command {
	var session, operator string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Assign a conversation to an operator (by user_id or email)",
		Long: "`--operator` takes either a raw operator `user_id` or an email address. An\n" +
			"email costs an EXTRA call against the website's operator list to resolve the\n" +
			"id, and an address that matches no operator on this website fails outright\n" +
			"with nothing assigned — so a `user_id` is the cheaper, exact form when one\n" +
			"is already known. Assignment replaces whoever held the thread before.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			website, err := websiteFlag(cmd)
			if err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "session"); err != nil {
				return err
			}
			if _, err := requireFlag(cmd, "operator"); err != nil {
				return err
			}
			userID := operator
			if strings.Contains(operator, "@") {
				resolved, err := s.resolveOperator(cmd.Context(), token, website, operator)
				if err != nil {
					return err
				}
				userID = resolved
			}
			body := map[string]any{"assigned": map[string]any{"user_id": userID}}
			path := fmt.Sprintf("/website/%s/conversation/%s/routing", website, session)
			data, err := s.call(cmd.Context(), token, http.MethodPatch, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(data, map[string]any{"website_id": website, "session": session})
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "conversation session_id (required)")
	cmd.Flags().StringVar(&operator, "operator", "", "operator user_id or email (required)")
	return cmd
}

// resolveOperator maps an operator email to its user_id via operators/list. It
// is the tool's only multi-call verb; an unmatched email is an apiError (exit 1).
func (s *Service) resolveOperator(ctx context.Context, token, website, email string) (string, error) {
	path := fmt.Sprintf("/website/%s/operators/list", website)
	data, err := s.call(ctx, token, http.MethodGet, path, nil, nil)
	if err != nil {
		return "", err
	}
	var operators []struct {
		Details struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
		} `json:"details"`
	}
	if err := json.Unmarshal(data, &operators); err != nil {
		return "", &apiError{msg: fmt.Sprintf("crisp: decode operators: %v", err), err: err}
	}
	for _, op := range operators {
		if strings.EqualFold(op.Details.Email, email) {
			if op.Details.UserID == "" {
				break
			}
			return op.Details.UserID, nil
		}
	}
	base := fmt.Errorf("no operator with email %q in this website", email)
	return "", &apiError{msg: base.Error(), err: base}
}
