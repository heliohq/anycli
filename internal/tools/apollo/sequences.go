package apollo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSequencesCmd builds the `sequences` group (Apollo emailer_campaigns): list
// sequences, enroll contacts, and stop/remove them. add / stop are documented
// as master-API-key-gated and may return 403 to an OAuth token.
func (s *Service) newSequencesCmd(token string) *cobra.Command {
	cmd := newGroupCmd("sequences", "Manage outbound sequences (emailer campaigns)")
	cmd.AddCommand(
		s.newSequencesListCmd(token),
		s.newSequencesAddCmd(token),
		s.newSequencesStopCmd(token),
	)
	return cmd
}

// newSequencesListCmd wraps POST /emailer_campaigns/search.
func (s *Service) newSequencesListCmd(token string) *cobra.Command {
	var body, q string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List sequences (POST /emailer_campaigns/search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "q_name", q)
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/emailer_campaigns/search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "sequence-name keyword filter")
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}

// newSequencesAddCmd wraps POST /emailer_campaigns/{id}/add_contact_ids. It
// needs the contact ids and a sending mailbox (email_account_id resolved via
// `email-accounts list`). Master-API-key-gated.
func (s *Service) newSequencesAddCmd(token string) *cobra.Command {
	var body, emailAccountID string
	var contactIDs []string
	cmd := &cobra.Command{
		Use:         "add <sequence_id>",
		Short:       "Enroll contacts into a sequence (POST /emailer_campaigns/{id}/add_contact_ids)",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStrSlice(b, "contact_ids", contactIDs)
			setStr(b, "send_email_from_email_account_id", emailAccountID)
			resp, err := s.call(cmd.Context(), token, http.MethodPost,
				"/emailer_campaigns/"+url.PathEscape(args[0])+"/add_contact_ids", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringArrayVar(&contactIDs, "contact-ids", nil, "contact id to enroll (repeatable)")
	cmd.Flags().StringVar(&emailAccountID, "email-account-id", "", "sending mailbox id (from `email-accounts list`)")
	registerBodyFlag(cmd, &body)
	_ = cmd.MarkFlagRequired("contact-ids")
	return cmd
}

// newSequencesStopCmd wraps POST /emailer_campaigns/remove_or_stop_contact_ids.
// Master-API-key-gated.
func (s *Service) newSequencesStopCmd(token string) *cobra.Command {
	var body, sequenceID, mode string
	var contactIDs []string
	cmd := &cobra.Command{
		Use:         "stop",
		Short:       "Stop or remove contacts in a sequence (POST /emailer_campaigns/remove_or_stop_contact_ids)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "emailer_campaign_id", sequenceID)
			setStrSlice(b, "contact_ids", contactIDs)
			setStr(b, "mode", mode)
			resp, err := s.call(cmd.Context(), token, http.MethodPost,
				"/emailer_campaigns/remove_or_stop_contact_ids", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&sequenceID, "sequence-id", "", "sequence (emailer_campaign) id")
	cmd.Flags().StringArrayVar(&contactIDs, "contact-ids", nil, "contact id to stop/remove (repeatable)")
	cmd.Flags().StringVar(&mode, "mode", "", "action: remove_from_sequence|stop_from_sequence|mark_as_finished")
	registerBodyFlag(cmd, &body)
	_ = cmd.MarkFlagRequired("sequence-id")
	_ = cmd.MarkFlagRequired("contact-ids")
	return cmd
}
