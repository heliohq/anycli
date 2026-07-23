package postmark

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// itoa renders an int as a base-10 query value.
func itoa(n int) string { return strconv.Itoa(n) }

func (s *Service) newMessageCmd(token string) *cobra.Command {
	group := newGroupCmd("message", "Search and inspect sent/received messages")
	group.AddCommand(
		s.newMessageListOutboundCmd(token),
		s.newMessageGetOutboundCmd(token),
		s.newMessageListInboundCmd(token),
		s.newMessageGetInboundCmd(token),
	)
	return group
}

func (s *Service) newMessageListOutboundCmd(token string) *cobra.Command {
	var count, offset int
	var recipient, fromEmail, tag, subject, status, stream string
	cmd := &cobra.Command{
		Use:         "list-outbound",
		Short:       "Search sent messages (GET /messages/outbound)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("count", itoa(count))
			q.Set("offset", itoa(offset))
			setQ(q, "recipient", recipient)
			setQ(q, "fromemail", fromEmail)
			setQ(q, "tag", tag)
			setQ(q, "subject", subject)
			setQ(q, "status", status)
			setQ(q, "messagestream", stream)
			return s.getAndEmit(cmd.Context(), token, "/messages/outbound", q)
		},
	}
	registerPaging(cmd, &count, &offset)
	cmd.Flags().StringVar(&recipient, "recipient", "", "filter by recipient address")
	cmd.Flags().StringVar(&fromEmail, "from-email", "", "filter by sender address")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().StringVar(&subject, "subject", "", "filter by subject")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (queued|sent|processed)")
	cmd.Flags().StringVar(&stream, "stream", "", "filter by message stream id")
	return cmd
}

func (s *Service) newMessageGetOutboundCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get-outbound <message-id>",
		Short:       "Get one outbound message's detail and events (GET /messages/outbound/{id}/details)",
		Args:        requireArgs(1, "get-outbound requires a <message-id>"),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/messages/outbound/"+url.PathEscape(args[0])+"/details", nil)
		},
	}
}

func (s *Service) newMessageListInboundCmd(token string) *cobra.Command {
	var count, offset int
	var recipient, fromEmail, subject, status string
	cmd := &cobra.Command{
		Use:         "list-inbound",
		Short:       "Search inbound messages (GET /messages/inbound)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("count", itoa(count))
			q.Set("offset", itoa(offset))
			setQ(q, "recipient", recipient)
			setQ(q, "fromemail", fromEmail)
			setQ(q, "subject", subject)
			setQ(q, "status", status)
			return s.getAndEmit(cmd.Context(), token, "/messages/inbound", q)
		},
	}
	registerPaging(cmd, &count, &offset)
	cmd.Flags().StringVar(&recipient, "recipient", "", "filter by recipient address")
	cmd.Flags().StringVar(&fromEmail, "from-email", "", "filter by sender address")
	cmd.Flags().StringVar(&subject, "subject", "", "filter by subject")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (blocked|processed|queued|failed|scheduled)")
	return cmd
}

func (s *Service) newMessageGetInboundCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get-inbound <message-id>",
		Short:       "Get one inbound message's detail (GET /messages/inbound/{id}/details)",
		Args:        requireArgs(1, "get-inbound requires a <message-id>"),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/messages/inbound/"+url.PathEscape(args[0])+"/details", nil)
		},
	}
}

// registerPaging wires the shared --count / --offset flags (Postmark search
// endpoints require both; defaults 100 / 0).
func registerPaging(cmd *cobra.Command, count, offset *int) {
	cmd.Flags().IntVar(count, "count", 100, "max results to return (1-500)")
	cmd.Flags().IntVar(offset, "offset", 0, "number of results to skip")
}

// setQ writes key=value into a query only when value is non-empty.
func setQ(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}
