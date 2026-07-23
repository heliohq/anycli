package helpscout

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

func (s *Service) newConversationCmd(token string) *cobra.Command {
	cmd := newGroupCmd("conversation", "Triage, read, and act on conversations")
	cmd.AddCommand(
		s.newConversationListCmd(token),
		s.newConversationGetCmd(token),
		s.newConversationCreateCmd(token),
		s.newConversationUpdateCmd(token),
		s.newConversationTagCmd(token),
		s.newConversationSnoozeCmd(token),
		s.newConversationUnsnoozeCmd(token),
	)
	return cmd
}

// newConversationListCmd — GET /conversations. HAL list passes through
// verbatim so the agent sees _embedded/page/_links pagination.
func (s *Service) newConversationListCmd(token string) *cobra.Command {
	var mailbox, folder, status, tag, assignedTo, modifiedSince, number, query, sortField, sortOrder string
	var page int
	var embedThreads bool
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List/filter/search conversations (GET /conversations)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := enumValidator("status", "active", "all", "closed", "open", "pending", "spam")(status); err != nil {
				return err
			}
			if err := enumValidator("sort-order", "asc", "desc")(sortOrder); err != nil {
				return err
			}
			q := url.Values{}
			setIf(q, "mailbox", mailbox)
			setIf(q, "folder", folder)
			setIf(q, "status", status)
			setIf(q, "tag", tag)
			setIf(q, "assigned_to", assignedTo)
			setIf(q, "modifiedSince", modifiedSince)
			setIf(q, "number", number)
			setIf(q, "query", query)
			setIf(q, "sortField", sortField)
			setIf(q, "sortOrder", sortOrder)
			setPage(q, page)
			if embedThreads {
				q.Set("embed", "threads")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().StringVar(&mailbox, "mailbox", "", "inbox id filter (comma-separated for multiple)")
	cmd.Flags().StringVar(&folder, "folder", "", "folder id filter")
	cmd.Flags().StringVar(&status, "status", "", "status filter: active|all|closed|open|pending|spam (API default active)")
	cmd.Flags().StringVar(&tag, "tag", "", "tag filter (comma-separated for multiple)")
	cmd.Flags().StringVar(&assignedTo, "assigned-to", "", "assignee user id filter")
	cmd.Flags().StringVar(&modifiedSince, "modified-since", "", "ISO 8601 timestamp; only conversations modified after")
	cmd.Flags().StringVar(&number, "number", "", "look up a conversation by its number")
	cmd.Flags().StringVar(&query, "query", "", "advanced Lucene-style search string (passed through verbatim)")
	cmd.Flags().StringVar(&sortField, "sort-field", "", "sort field, e.g. createdAt|modifiedAt|number|subject")
	cmd.Flags().StringVar(&sortOrder, "sort-order", "", "sort order: asc|desc (API default desc)")
	cmd.Flags().IntVar(&page, "page", 0, "1-based page number")
	cmd.Flags().BoolVar(&embedThreads, "embed-threads", false, "embed each conversation's threads")
	return cmd
}

// newConversationGetCmd — GET /conversations/{id}.
func (s *Service) newConversationGetCmd(token string) *cobra.Command {
	var embedThreads bool
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one conversation with its threads (GET /conversations/{id})",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if embedThreads {
				q.Set("embed", "threads")
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/conversations/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp.body)
		},
	}
	cmd.Flags().BoolVar(&embedThreads, "embed-threads", false, "embed the conversation's threads")
	return cmd
}

// newConversationCreateCmd — POST /conversations. The API requires status and
// at least one thread; --status defaults to active and is always sent, and the
// initial thread is built from --text (type --thread-type, default customer).
func (s *Service) newConversationCreateCmd(token string) *cobra.Command {
	var mailbox, subject, customerEmail, customerID, convType, status, text, threadType, assignTo, tags string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a conversation with an initial thread (POST /conversations)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := enumValidator("type", "email", "phone", "chat")(convType); err != nil {
				return err
			}
			// Create accepts a narrower status set than update (no spam).
			if err := enumValidator("status", "active", "closed", "pending")(status); err != nil {
				return err
			}
			if err := enumValidator("thread-type", "customer", "reply", "note", "phone", "chat")(threadType); err != nil {
				return err
			}
			if customerEmail == "" && customerID == "" {
				return &usageError{msg: "one of --customer-email or --customer-id is required"}
			}
			mailboxID, err := parseMailbox(mailbox)
			if err != nil {
				return err
			}
			customer := map[string]any{}
			if customerID != "" {
				id, err := strconv.Atoi(customerID)
				if err != nil {
					return &usageError{msg: "--customer-id must be a number"}
				}
				customer["id"] = id
			} else {
				customer["email"] = customerEmail
			}
			thread := map[string]any{"type": threadType, "text": text}
			// A customer-type thread must name the customer it came from.
			if threadType == "customer" {
				thread["customer"] = customer
			}
			body := map[string]any{
				"subject":   subject,
				"type":      convType,
				"mailboxId": mailboxID,
				"status":    status,
				"customer":  customer,
				"threads":   []any{thread},
			}
			if assignTo != "" {
				id, err := strconv.Atoi(assignTo)
				if err != nil {
					return &usageError{msg: "--assign-to must be a user id number"}
				}
				body["assignTo"] = id
			}
			if tags != "" {
				body["tags"] = splitCSV(tags)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/conversations", nil, body)
			if err != nil {
				return err
			}
			return s.emitReceipt(resp.resourceID(), "created")
		},
	}
	cmd.Flags().StringVar(&mailbox, "mailbox", "", "inbox id to create the conversation in (required)")
	cmd.Flags().StringVar(&subject, "subject", "", "conversation subject (required)")
	cmd.Flags().StringVar(&customerEmail, "customer-email", "", "customer email (or --customer-id)")
	cmd.Flags().StringVar(&customerID, "customer-id", "", "customer id (or --customer-email)")
	cmd.Flags().StringVar(&convType, "type", "email", "conversation type: email|phone|chat")
	cmd.Flags().StringVar(&status, "status", "active", "initial status: active|closed|pending")
	cmd.Flags().StringVar(&text, "text", "", "initial thread body (required)")
	cmd.Flags().StringVar(&threadType, "thread-type", "customer", "initial thread type: customer|reply|note|phone|chat")
	cmd.Flags().StringVar(&assignTo, "assign-to", "", "assignee user id")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags")
	_ = cmd.MarkFlagRequired("mailbox")
	_ = cmd.MarkFlagRequired("subject")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// newConversationUpdateCmd — PATCH /conversations/{id} with the API's
// JSON-Patch dialect. Flags compile to replace/remove ops so the agent never
// writes patch JSON. 204 → an "updated" receipt.
func (s *Service) newConversationUpdateCmd(token string) *cobra.Command {
	var status, assignTo, subject string
	var unassign bool
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update status / assignee / subject (PATCH /conversations/{id})",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Matches the reply status set (Errors doc: active|spam|open|closed|
			// pending); create is intentionally narrower per its own endpoint.
			if err := enumValidator("status", "active", "closed", "open", "pending", "spam")(status); err != nil {
				return err
			}
			if assignTo != "" && unassign {
				return &usageError{msg: "--assign-to and --unassign are mutually exclusive"}
			}
			ops := []map[string]any{}
			if status != "" {
				ops = append(ops, map[string]any{"op": "replace", "path": "/status", "value": status})
			}
			if subject != "" {
				ops = append(ops, map[string]any{"op": "replace", "path": "/subject", "value": subject})
			}
			if unassign {
				ops = append(ops, map[string]any{"op": "remove", "path": "/assignTo"})
			} else if assignTo != "" {
				id, err := strconv.Atoi(assignTo)
				if err != nil {
					return &usageError{msg: "--assign-to must be a user id number"}
				}
				ops = append(ops, map[string]any{"op": "replace", "path": "/assignTo", "value": id})
			}
			if len(ops) == 0 {
				return &usageError{msg: "nothing to update: pass at least one of --status, --assign-to, --unassign, --subject"}
			}
			// Help Scout applies one JSON-Patch op per request; send them in order.
			id := url.PathEscape(args[0])
			for _, op := range ops {
				if _, err := s.call(cmd.Context(), token, http.MethodPatch, "/conversations/"+id, nil, op); err != nil {
					return err
				}
			}
			return s.emitReceipt(args[0], "updated")
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "new status: active|closed|open|pending|spam")
	cmd.Flags().StringVar(&assignTo, "assign-to", "", "assignee user id")
	cmd.Flags().BoolVar(&unassign, "unassign", false, "remove the assignee")
	cmd.Flags().StringVar(&subject, "subject", "", "new subject")
	return cmd
}

// newConversationTagCmd — PUT /conversations/{id}/tags. Replaces the whole tag
// set (empty --tags clears all tags). 204 → a "tagged" receipt.
func (s *Service) newConversationTagCmd(token string) *cobra.Command {
	var tags string
	cmd := &cobra.Command{
		Use:         "tag <id> --tags t1,t2",
		Short:       "Replace a conversation's tag set (PUT /conversations/{id}/tags)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"tags": splitCSV(tags)}
			if _, err := s.call(cmd.Context(), token, http.MethodPut, "/conversations/"+url.PathEscape(args[0])+"/tags", nil, body); err != nil {
				return err
			}
			return s.emitReceipt(args[0], "tagged")
		},
	}
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tag set (replaces all; empty clears)")
	_ = cmd.MarkFlagRequired("tags")
	return cmd
}

// newConversationSnoozeCmd — PUT /conversations/{id}/snooze. Both snoozedUntil
// and unsnoozeOnCustomerReply are required by the API; --until maps to the
// former and --unsnooze-on-customer-reply defaults to true.
func (s *Service) newConversationSnoozeCmd(token string) *cobra.Command {
	var until string
	var unsnoozeOnReply bool
	cmd := &cobra.Command{
		Use:         "snooze <id> --until <ISO8601>",
		Short:       "Snooze a conversation until a future time (PUT /conversations/{id}/snooze)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"snoozedUntil":            until,
				"unsnoozeOnCustomerReply": unsnoozeOnReply,
			}
			if _, err := s.call(cmd.Context(), token, http.MethodPut, "/conversations/"+url.PathEscape(args[0])+"/snooze", nil, body); err != nil {
				return err
			}
			return s.emitReceipt(args[0], "snoozed")
		},
	}
	cmd.Flags().StringVar(&until, "until", "", "ISO 8601 future timestamp to snooze until (required)")
	cmd.Flags().BoolVar(&unsnoozeOnReply, "unsnooze-on-customer-reply", true, "wake the conversation if the customer replies")
	_ = cmd.MarkFlagRequired("until")
	return cmd
}

// newConversationUnsnoozeCmd — DELETE /conversations/{id}/snooze.
func (s *Service) newConversationUnsnoozeCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "unsnooze <id>",
		Short:       "Clear a conversation's snooze (DELETE /conversations/{id}/snooze)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := s.call(cmd.Context(), token, http.MethodDelete, "/conversations/"+url.PathEscape(args[0])+"/snooze", nil, nil); err != nil {
				return err
			}
			return s.emitReceipt(args[0], "unsnoozed")
		},
	}
	return cmd
}

// parseMailbox converts the --mailbox flag to the numeric mailboxId the create
// body requires.
func parseMailbox(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &usageError{msg: "--mailbox must be a numeric inbox id"}
	}
	return id, nil
}

// primaryCustomerID reads a conversation's primaryCustomer.id, used to default
// the reply thread's customer when --customer-id is omitted.
func primaryCustomerID(body []byte) (int, bool) {
	var c struct {
		PrimaryCustomer struct {
			ID int `json:"id"`
		} `json:"primaryCustomer"`
	}
	if err := json.Unmarshal(body, &c); err != nil || c.PrimaryCustomer.ID == 0 {
		return 0, false
	}
	return c.PrimaryCustomer.ID, true
}
