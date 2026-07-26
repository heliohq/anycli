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

// These Longs sit here because every leaf they describe comes from the shared
// list/get constructors in common.go.
const (
	longMailboxList = "A mailbox is the sending seat a cadence goes out through, and\n" +
		"`enrollment add --mailbox-id` requires one — a guessed id fails the enroll.\n" +
		"There are no resource-specific filters, which is workable because an org\n" +
		"has few mailboxes."

	longMailingGet = "One email Outreach sent, with its engagement timestamps and the prospect\n" +
		"and mailbox hoisted. `state` is the outcome field. A mailing exists per\n" +
		"recipient, so one sequence step across ten prospects is ten mailings."

	longCallGet = "One logged call with its outcome, duration and hoisted `prospect_id`.\n" +
		"Outreach records calls placed through its own dialer; a call made elsewhere\n" +
		"appears only if something synced it in."

	longOpportunityList = "Pipeline records, read-only here — nothing in this tool creates an\n" +
		"opportunity or advances its stage. There are no resource-specific filters,\n" +
		"so narrowing means --sort, --fields and paging."

	longOpportunityGet = "One opportunity: amount, stage and close date, with `account_id` and\n" +
		"`owner_id` hoisted. Reading only — forecast changes happen in Outreach or\n" +
		"the CRM behind it."
)

// newMailboxCmd builds the mailbox group. A mailbox is the required relationship
// for enrolling a prospect in a sequence.
func (s *Service) newMailboxCmd(token string) *cobra.Command {
	group := newGroupCmd("mailbox", "List mailboxes")
	group.AddCommand(s.newListCmd(token, mailboxResource, longMailboxList))
	return group
}

// newMailingCmd builds the mailing group — email outcomes (delivered/opened/
// clicked/replied).
func (s *Service) newMailingCmd(token string) *cobra.Command {
	group := newGroupCmd("mailing", "Read email outcomes (mailings)")
	group.AddCommand(
		s.newMailingListCmd(token),
		s.newGetCmd(token, mailingResource, longMailingGet),
	)
	return group
}

func (s *Service) newMailingListCmd(token string) *cobra.Command {
	var prospectID, state string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List mailings (one page)",
		Long: "Email outcomes, one row per recipient rather than per sequence step:\n" +
			"--state filters on delivered, opened, clicked, replied or bounced.\n" +
			"Combined with --prospect-id it answers whether one person ever engaged,\n" +
			"which is the read to run before enrolling them again.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		s.newGetCmd(token, callResource, longCallGet),
	)
	return group
}

func (s *Service) newCallListCmd(token string) *cobra.Command {
	var prospectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List calls (one page)",
		Long: "--prospect-id is the only filter. Outreach logs calls placed through its\n" +
			"own dialer, so an empty list is not proof nobody phoned — a call dialed\n" +
			"elsewhere shows up only if it was synced in.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		s.newListCmd(token, opportunityResource, longOpportunityList),
		s.newGetCmd(token, opportunityResource, longOpportunityGet),
	)
	return group
}
