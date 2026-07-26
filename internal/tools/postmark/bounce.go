package postmark

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newBounceCmd(token string) *cobra.Command {
	group := newGroupCmd("bounce", "Diagnose deliverability via bounces")
	group.AddCommand(
		s.newBounceListCmd(token),
		s.newBounceGetCmd(token),
		s.newBounceActivateCmd(token),
	)
	return group
}

func (s *Service) newBounceListCmd(token string) *cobra.Command {
	var count, offset int
	var bounceType, email, tag, messageID string
	var inactive bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Search bounces (GET /bounces)",
		Long: "`--type` takes a Postmark bounce type (HardBounce, SoftBounce,\n" +
			"SpamComplaint, Transient, ...). `--inactive` is three-state: omitted returns\n" +
			"every bounce, `--inactive=true` only recipients Postmark has SUPPRESSED, and\n" +
			"`--inactive=false` those still deliverable. `--email` filters on the\n" +
			"recipient address and `--message-id` ties bounces back to one send. Only a\n" +
			"suppressed recipient is a candidate for `bounce activate`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("count", itoa(count))
			q.Set("offset", itoa(offset))
			setQ(q, "type", bounceType)
			setQ(q, "emailFilter", email)
			setQ(q, "tag", tag)
			setQ(q, "messageID", messageID)
			if cmd.Flags().Changed("inactive") {
				if inactive {
					q.Set("inactive", "true")
				} else {
					q.Set("inactive", "false")
				}
			}
			return s.getAndEmit(cmd.Context(), token, "/bounces", q)
		},
	}
	registerPaging(cmd, &count, &offset)
	cmd.Flags().StringVar(&bounceType, "type", "", "filter by bounce type (e.g. HardBounce, SpamComplaint)")
	cmd.Flags().StringVar(&email, "email", "", "filter by recipient email (emailFilter)")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().StringVar(&messageID, "message-id", "", "filter by message id")
	cmd.Flags().BoolVar(&inactive, "inactive", false, "filter by inactive (deactivated) recipients")
	return cmd
}

func (s *Service) newBounceGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <bounce-id>",
		Short: "Get one bounce (GET /bounces/{id})",
		Long: "Takes the numeric bounce id from `bounce list`, not a message id. Returns\n" +
			"the bounce type, the receiving server's raw `Details` text, whether the\n" +
			"recipient is now `Inactive`, and `CanActivate` — which is what decides\n" +
			"whether `bounce activate` will be accepted for it.",
		Args:        requireArgs(1, "get requires a <bounce-id>"),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/bounces/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (s *Service) newBounceActivateCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "activate <bounce-id>",
		Short: "Reactivate a deactivated recipient (PUT /bounces/{id}/activate)",
		Long: "Clears Postmark's suppression so this server can mail the address again. It\n" +
			"does NOT resend the message that bounced — that is a fresh `email send`.\n" +
			"Reopening a hard bounce or a spam complaint means the next send really goes\n" +
			"out, and repeated hard bounces cost the server its sending reputation.\n" +
			"`bounce get` reports `CanActivate` false when Postmark will refuse.",
		Args:        requireArgs(1, "activate requires a <bounce-id>"),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := s.call(cmd.Context(), token, http.MethodPut, "/bounces/"+url.PathEscape(args[0])+"/activate", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(raw)
		},
	}
}

func (s *Service) newStatsCmd(token string) *cobra.Command {
	group := newGroupCmd("stats", "Delivery statistics")
	group.AddCommand(&cobra.Command{
		Use:   "delivery",
		Short: "Delivery / bounce summary (GET /deliverystats)",
		Long: "One server-wide roll-up: the inactive-recipient count plus a per-bounce-type\n" +
			"breakdown. It takes no date range, tag or stream filter, so these are\n" +
			"running totals rather than a window — use `message list-outbound` or\n" +
			"`bounce list` when the question is time-scoped.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.getAndEmit(cmd.Context(), token, "/deliverystats", nil)
		},
	})
	return group
}
