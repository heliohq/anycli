package courier

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newMessageGetCmd builds `message get <id>` — GET /messages/{id}, the delivery
// status/outcome of a requestId returned by `send`.
func (s *Service) newMessageGetCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <message-id>",
		Short: "Get a message's delivery status",
		Long: "Takes the `requestId` that `send` returned. Reports one current status —\n" +
			"ENQUEUED, SENT, DELIVERED, UNDELIVERABLE and the like — plus the reason\n" +
			"when it failed. The status is not instant: a fresh send reads as enqueued\n" +
			"for a while, so a single check straight after sending proves nothing. For\n" +
			"why a message ended where it did, `message history` carries the per-step\n" +
			"trail.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/messages/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newMessageListCmd builds `message list` — GET /messages with optional cursor
// paging and filters. Courier paginates by cursor only (no limit param).
func (s *Service) newMessageListCmd(key string) *cobra.Command {
	var cursor, status, recipient, notification, list, tags, traceID, enqueuedAfter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent messages (cursor-paginated)",
		Long: "Filters combine as AND: --status, --recipient, --notification, --list,\n" +
			"--tags (comma-delimited), --trace-id and --enqueued-after. There is no\n" +
			"page-size flag — take `paging.cursor` from the response and pass it back\n" +
			"as --cursor while `paging.more` is true. This is the way back to a message\n" +
			"whose requestId was not kept.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setIf(q, "cursor", cursor)
			setIf(q, "status", status)
			setIf(q, "recipient", recipient)
			setIf(q, "notification", notification)
			setIf(q, "list", list)
			setIf(q, "tags", tags)
			setIf(q, "traceId", traceID)
			setIf(q, "enqueued_after", enqueuedAfter)
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/messages", q, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	pf := cmd.Flags()
	pf.StringVar(&cursor, "cursor", "", "pagination cursor for the next page")
	pf.StringVar(&status, "status", "", "filter by message status")
	pf.StringVar(&recipient, "recipient", "", "filter by recipient id")
	pf.StringVar(&notification, "notification", "", "filter by notification id")
	pf.StringVar(&list, "list", "", "filter by list id")
	pf.StringVar(&tags, "tags", "", "comma-delimited tag filter")
	pf.StringVar(&traceID, "trace-id", "", "filter by trace id")
	pf.StringVar(&enqueuedAfter, "enqueued-after", "", "ISO-8601 lower bound on enqueue time")
	return cmd
}

// newMessageHistoryCmd builds `message history <id>` — GET /messages/{id}/history,
// the per-message delivery timeline.
func (s *Service) newMessageHistoryCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "history <message-id>",
		Short: "Get a message's delivery timeline",
		Long: "The per-step trail for one requestId — enqueued, routed, rendered, sent,\n" +
			"delivered or errored — each entry timestamped and attributed to a channel.\n" +
			"A provider-side bounce or a channel rejection is visible only here;\n" +
			"`message get` collapses all of it into one status value.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodGet, "/messages/"+url.PathEscape(args[0])+"/history", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}

// newMessageCancelCmd builds `message cancel <id>` — POST /messages/{id}/cancel
// with no body, cancelling an enqueued/delayed message.
func (s *Service) newMessageCancelCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <message-id>",
		Short: "Cancel an enqueued or delayed message",
		Long: "Only bites while the message is still Courier's — enqueued, delayed or\n" +
			"waiting inside an automation step. Once it has been handed to the delivery\n" +
			"channel there is nothing to recall and the call fails rather than undoing\n" +
			"anything, so the window after a plain `send` is short. Takes the\n" +
			"requestId.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := s.call(cmd.Context(), key, http.MethodPost, "/messages/"+url.PathEscape(args[0])+"/cancel", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
}
