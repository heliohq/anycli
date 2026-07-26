package helpscout

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newThreadCmd(token string) *cobra.Command {
	cmd := newGroupCmd("thread", "Read threads and answer (reply / note)")
	cmd.AddCommand(
		s.newThreadListCmd(token),
		s.newThreadReplyCmd(token),
		s.newThreadNoteCmd(token),
	)
	return cmd
}

// newThreadListCmd — GET /conversations/{id}/threads.
func (s *Service) newThreadListCmd(token string) *cobra.Command {
	var page int
	cmd := &cobra.Command{
		Use:   "list <conversation-id>",
		Short: "List a conversation's threads (GET /conversations/{id}/threads)",
		Long: "The message bodies without the conversation record around them — the\n" +
			"cheaper half of `conversation get --embed-threads` once the metadata is\n" +
			"already known. Each thread carries a `type`, and that is what separates a\n" +
			"customer message from a staff reply and from an internal note. Page with\n" +
			"--page; a long-running conversation is not returned in one call.",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			setPage(q, page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(args[0])+"/threads", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	return cmd
}

// newThreadReplyCmd — POST /conversations/{id}/reply. The API requires a
// customer in the body; when --customer-id is omitted it is defaulted to the
// conversation's primaryCustomer (one extra GET). Optional status/assignTo let
// the agent answer and set state in one call.
func (s *Service) newThreadReplyCmd(token string) *cobra.Command {
	var text, customerID, status, assignTo, cc, bcc string
	var draft bool
	cmd := &cobra.Command{
		Use:   "reply <conversation-id> --text ...",
		Short: "Reply to the customer (POST /conversations/{id}/reply)",
		Long: "Emails the customer the moment it returns. --draft stores it as a draft\n" +
			"for a human to send instead, which is the reversible form of this command.\n" +
			"Omitting --customer-id costs an extra GET of the conversation to read its\n" +
			"primaryCustomer, so pass it when it is already in hand.\n" +
			"\n" +
			"--status and --assign-to move the conversation in the same request, which\n" +
			"saves a separate `conversation update` and its per-op round trips. --cc\n" +
			"and --bcc take comma-separated addresses. The response is an id/status\n" +
			"receipt for the new thread, not the conversation.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := enumValidator("status", "active", "closed", "open", "pending", "spam")(status); err != nil {
				return err
			}
			convID := args[0]
			custID, err := s.resolveReplyCustomer(cmd, token, convID, customerID)
			if err != nil {
				return err
			}
			body := map[string]any{
				"text":     text,
				"customer": map[string]any{"id": custID},
			}
			if draft {
				body["draft"] = true
			}
			if status != "" {
				body["status"] = status
			}
			if assignTo != "" {
				id, err := strconv.Atoi(assignTo)
				if err != nil {
					return &usageError{msg: "--assign-to must be a user id number"}
				}
				body["assignTo"] = id
			}
			if cc != "" {
				body["cc"] = splitCSV(cc)
			}
			if bcc != "" {
				body["bcc"] = splitCSV(bcc)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/"+url.PathEscape(convID)+"/reply", nil, body)
			if err != nil {
				return err
			}
			return s.emitReceipt(resp.resourceID(), "created")
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "reply body (required)")
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer id (defaults to the conversation's primary customer)")
	cmd.Flags().StringVar(&status, "status", "", "set status after replying: active|closed|open|pending|spam")
	cmd.Flags().StringVar(&assignTo, "assign-to", "", "assign the conversation after replying (user id)")
	cmd.Flags().BoolVar(&draft, "draft", false, "create a draft instead of publishing")
	cmd.Flags().StringVar(&cc, "cc", "", "comma-separated cc addresses")
	cmd.Flags().StringVar(&bcc, "bcc", "", "comma-separated bcc addresses")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// resolveReplyCustomer returns the explicit --customer-id when set, otherwise
// GETs the conversation and reads primaryCustomer.id.
func (s *Service) resolveReplyCustomer(cmd *cobra.Command, token, convID, explicit string) (int, error) {
	if explicit != "" {
		id, err := strconv.Atoi(explicit)
		if err != nil {
			return 0, &usageError{msg: "--customer-id must be a number"}
		}
		return id, nil
	}
	resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(convID), nil, nil)
	if err != nil {
		return 0, err
	}
	id, ok := primaryCustomerID(resp.body)
	if !ok {
		return 0, &usageError{msg: "could not determine the conversation's primary customer; pass --customer-id explicitly"}
	}
	return id, nil
}

// newThreadNoteCmd — POST /conversations/{id}/notes. Internal note for the team.
func (s *Service) newThreadNoteCmd(token string) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "note <conversation-id> --text ...",
		Short: "Add an internal note (POST /conversations/{id}/notes)",
		Long: "Internal only: the note is visible to the team inside Help Scout and is\n" +
			"never emailed to the customer, which is the whole difference from `thread\n" +
			"reply`. --text is the only field — a note cannot set status or assignee in\n" +
			"the same call the way a reply can, so pair it with `conversation update`\n" +
			"when the conversation also has to move.",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"text": text}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations/"+url.PathEscape(args[0])+"/notes", nil, body)
			if err != nil {
				return err
			}
			return s.emitReceipt(resp.resourceID(), "created")
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "note body (required)")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}
