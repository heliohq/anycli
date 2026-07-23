package outreach

import (
	"net/url"

	"github.com/spf13/cobra"
)

var (
	mailboxResource     = resource{path: "mailboxes", typ: "mailbox"}
	mailingResource     = resource{path: "mailings", typ: "mailing"}
	callResource        = resource{path: "calls", typ: "call"}
	opportunityResource = resource{path: "opportunities", typ: "opportunity"}
)

// newMailboxCmd builds the mailbox group. A mailbox is the required relationship
// for enrolling a prospect in a sequence.
func (s *Service) newMailboxCmd(token string) *cobra.Command {
	group := newGroupCmd("mailbox", "List mailboxes")
	group.AddCommand(s.newListCmd(token, mailboxResource))
	return group
}

// newMailingCmd builds the mailing group — email outcomes (delivered/opened/
// clicked/replied).
func (s *Service) newMailingCmd(token string) *cobra.Command {
	group := newGroupCmd("mailing", "Read email outcomes (mailings)")
	group.AddCommand(
		s.newMailingListCmd(token),
		s.newGetCmd(token, mailingResource),
	)
	return group
}

func (s *Service) newMailingListCmd(token string) *cobra.Command {
	var prospectID, state string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List mailings (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setRelFilter(query, "prospect", prospectID)
			setFilter(query, "state", state)
			if err := listFlagsFrom(cmd).apply(query, mailingResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, mailingResource, query)
		},
	}
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "filter by prospect id")
	cmd.Flags().StringVar(&state, "state", "", "filter by mailing state (e.g. delivered, opened, clicked, replied)")
	bindListFlags(cmd)
	return cmd
}

// newCallCmd builds the call group — call activity read.
func (s *Service) newCallCmd(token string) *cobra.Command {
	group := newGroupCmd("call", "Read call activity")
	group.AddCommand(
		s.newCallListCmd(token),
		s.newGetCmd(token, callResource),
	)
	return group
}

func (s *Service) newCallListCmd(token string) *cobra.Command {
	var prospectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List calls (one page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setRelFilter(query, "prospect", prospectID)
			if err := listFlagsFrom(cmd).apply(query, callResource.typ); err != nil {
				return err
			}
			return s.runList(cmd.Context(), token, callResource, query)
		},
	}
	cmd.Flags().StringVar(&prospectID, "prospect-id", "", "filter by prospect id")
	bindListFlags(cmd)
	return cmd
}

// newOpportunityCmd builds the opportunity group — pipeline reporting.
func (s *Service) newOpportunityCmd(token string) *cobra.Command {
	group := newGroupCmd("opportunity", "Read opportunities (pipeline)")
	group.AddCommand(
		s.newListCmd(token, opportunityResource),
		s.newGetCmd(token, opportunityResource),
	)
	return group
}
