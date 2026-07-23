package knock

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageCmd groups the message verbs: did the notification land, and was it
// seen/read? Messages are the delivery + engagement record.
func (s *Service) newMessageCmd(key string) *cobra.Command {
	group := newGroupCmd("message", "Inspect delivered messages and their engagement status")
	group.AddCommand(
		s.newMessageListCmd(key),
		s.newMessageGetCmd(key),
		s.newMessageSubCmd(key, "content", "Get a message's rendered content"),
		s.newMessageSubCmd(key, "events", "List a message's events"),
		s.newMessageSubCmd(key, "activities", "List a message's activities"),
		s.newMessageSubCmd(key, "delivery-logs", "List a message's delivery logs"),
		s.newMessageMarkCmd(key),
	)
	return group
}

func (s *Service) newMessageListCmd(key string) *cobra.Command {
	var (
		recipient string
		channelID string
		status    string
		tenant    string
		workflow  string
		pageSize  int
		after     string
		before    string
	)
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List messages, filtered by recipient/channel/status/tenant/workflow",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if recipient != "" {
				q.Set("recipient", recipient)
			}
			if channelID != "" {
				q.Set("channel_id", channelID)
			}
			if status != "" {
				q.Set("status", status)
			}
			if tenant != "" {
				q.Set("tenant", tenant)
			}
			if workflow != "" {
				q.Set("workflow", workflow)
			}
			addPaging(q, pageSize, after, before)
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/messages", q, nil, nil)
		},
	}
	cmd.Flags().StringVar(&recipient, "recipient", "", "filter by recipient id")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "filter by channel id")
	cmd.Flags().StringVar(&status, "status", "", "filter by delivery status (queued|sent|delivered|undelivered|not_sent|…)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "filter by tenant id")
	cmd.Flags().StringVar(&workflow, "workflow", "", "filter by workflow key")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (Knock default 50)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor (next page)")
	cmd.Flags().StringVar(&before, "before", "", "pagination cursor (previous page)")
	return cmd
}

func (s *Service) newMessageGetCmd(key string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get a message",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/messages/"+url.PathEscape(id), nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "message id (required)")
	return cmd
}

// newMessageSubCmd builds a read-only GET /messages/{id}/<segment> command. The
// CLI word "delivery-logs" maps to the API segment "delivery_logs".
func (s *Service) newMessageSubCmd(key, use, short string) *cobra.Command {
	var id string
	segment := use
	if use == "delivery-logs" {
		segment = "delivery_logs"
	}
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			return s.callEmit(cmd.Context(), key, http.MethodGet, "/messages/"+url.PathEscape(id)+"/"+segment, nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "message id (required)")
	return cmd
}

// newMessageMarkCmd sets or clears a message's engagement status. PUT marks the
// state, DELETE (--undo) clears it. "interacted" is mark-only (Knock has no
// un-interact endpoint).
func (s *Service) newMessageMarkCmd(key string) *cobra.Command {
	var (
		id    string
		state string
		undo  bool
	)
	cmd := &cobra.Command{
		Use:         "mark",
		Short:       "Mark a message seen|read|interacted|archived (--undo to clear)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID("id", id); err != nil {
				return err
			}
			if !isMarkState(state) {
				return &usageError{msg: "--state must be one of seen|read|interacted|archived"}
			}
			method := http.MethodPut
			if undo {
				if state == "interacted" {
					return &usageError{msg: "interacted cannot be undone (Knock has no un-interact endpoint)"}
				}
				method = http.MethodDelete
			}
			return s.callEmit(cmd.Context(), key, method, "/messages/"+url.PathEscape(id)+"/"+state, nil, nil, nil)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "message id (required)")
	cmd.Flags().StringVar(&state, "state", "", "engagement state: seen|read|interacted|archived (required)")
	cmd.Flags().BoolVar(&undo, "undo", false, "clear the state instead of setting it (not valid for interacted)")
	return cmd
}

func isMarkState(state string) bool {
	switch state {
	case "seen", "read", "interacted", "archived":
		return true
	default:
		return false
	}
}
