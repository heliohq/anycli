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
		Use:   "list",
		Short: "List sequences (POST /emailer_campaigns/search)",
		Long: "Sequences are Apollo emailer campaigns; the id returned here is what\n" +
			"`sequences add` and `sequences stop --sequence-id` take. --q filters on\n" +
			"the sequence name. This read is reachable with the connected OAuth token\n" +
			"even though `sequences add` and `sequences stop` are not.",
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
		Use:   "add <sequence_id>",
		Short: "Enroll contacts into a sequence (POST /emailer_campaigns/{id}/add_contact_ids)",
		Long: "Enrollment starts real outbound email to real prospects on Apollo's own\n" +
			"schedule; there is no draft or preview state. --contact-ids is repeatable\n" +
			"and takes contact ids from `contacts create` / `contacts search`, NOT the\n" +
			"person ids `people search` returns. --email-account-id names the sending\n" +
			"mailbox and is not enforced locally, but an enrollment with no mailbox has\n" +
			"nothing to send from — resolve one with `email-accounts list` first.\n" +
			"Apollo documents this endpoint as master-API-key-only, so the connected\n" +
			"OAuth token can come back 403 with a master-key hint.",
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
		Use:   "stop",
		Short: "Stop or remove contacts in a sequence (POST /emailer_campaigns/remove_or_stop_contact_ids)",
		Long: "--mode picks the outcome: `remove_from_sequence` detaches the contacts\n" +
			"entirely, `stop_from_sequence` halts further steps while leaving them\n" +
			"attached, `mark_as_finished` closes them out as completed. Both\n" +
			"--sequence-id and --contact-ids are required. Apollo documents this\n" +
			"endpoint as master-API-key-only, so the connected OAuth token can come\n" +
			"back 403 with a master-key hint.",
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
