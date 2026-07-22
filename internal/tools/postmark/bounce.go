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
		Args:  cobra.NoArgs,
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
		Args:  requireArgs(1, "get requires a <bounce-id>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getAndEmit(cmd.Context(), token, "/bounces/"+url.PathEscape(args[0]), nil)
		},
	}
}

func (s *Service) newBounceActivateCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "activate <bounce-id>",
		Short: "Reactivate a deactivated recipient (PUT /bounces/{id}/activate)",
		Args:  requireArgs(1, "activate requires a <bounce-id>"),
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.getAndEmit(cmd.Context(), token, "/deliverystats", nil)
		},
	})
	return group
}
