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
		s.newMessageSubCmd(key, "content", "Get a message's rendered content", longMessageContent),
		s.newMessageSubCmd(key, "events", "List a message's events", longMessageEvents),
		s.newMessageSubCmd(key, "activities", "List a message's activities", longMessageActivities),
		s.newMessageSubCmd(key, "delivery-logs", "List a message's delivery logs", longMessageDeliveryLogs),
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
		Use:   "list",
		Short: "List messages, filtered by recipient/channel/status/tenant/workflow",
		Long: "Filters are exact: --recipient, --channel-id, --tenant, --workflow (a\n" +
			"workflow KEY) and --status (queued, sent, delivered, undelivered,\n" +
			"not_sent, …). A message exists only once a workflow run produced one, so\n" +
			"an environment that has never sent anything returns empty `entries` — that\n" +
			"is not a mis-typed filter. Knock's own page size is 50; page with --after.",
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
		Use:   "get",
		Short: "Get a message",
		Long: "--id is a message id, not the workflow_run_id `workflow trigger` returned —\n" +
			"a run id does not resolve here, so find the message with `message list\n" +
			"--recipient` first. The record carries the delivery status and engagement\n" +
			"timestamps but not the rendered body, which is `message content`.",
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
// The four message-detail Longs. They sit next to the shared builder because
// the builder is what fixes their identical shape (--id, one GET, no paging),
// while what each sub-resource actually contains — and therefore which failure
// it diagnoses — is what tells them apart.
const (
	longMessageContent = "The body Knock actually produced for this message after the workflow\n" +
		"template ran, which is what the recipient was shown rather than what went\n" +
		"in as --data. Requires --id."

	longMessageEvents = "The message's own lifecycle inside Knock — queued, sent, and the\n" +
		"engagement states after that. Requires --id. This is Knock's side of the\n" +
		"story; what the downstream email or push vendor said is `message\n" +
		"delivery-logs`."

	longMessageActivities = "The trigger activities Knock rolled up into this message, which is how a\n" +
		"batched or digest message is traced back to the individual events that\n" +
		"produced it. Requires --id. A message from a non-batching workflow has one."

	longMessageDeliveryLogs = "The raw request and response exchanged with the downstream provider — the\n" +
		"email or push vendor — which is where a bounce or a provider-side rejection\n" +
		"actually shows up. Requires --id, and logs exist only for a message that\n" +
		"reached a send attempt."
)

func (s *Service) newMessageSubCmd(key, use, short, long string) *cobra.Command {
	var id string
	segment := use
	if use == "delivery-logs" {
		segment = "delivery_logs"
	}
	cmd := &cobra.Command{
		Use:         use,
		Short:       short,
		Long:        long,
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
		Use:   "mark",
		Short: "Mark a message seen|read|interacted|archived (--undo to clear)",
		Long: "--state is one of seen, read, interacted or archived. --undo clears the\n" +
			"state instead of setting it, EXCEPT for interacted, which Knock has no\n" +
			"endpoint to reverse — that combination is rejected before any request.\n" +
			"These are in-app feed engagement states: marking a message read here does\n" +
			"not recall an email that already went out.",
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
