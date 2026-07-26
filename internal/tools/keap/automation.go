package keap

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newAutomationCmd(token string) *cobra.Command {
	cmd := newGroupCmd("automation", "Automations (list, get, add-contacts)")
	cmd.AddCommand(
		s.newAutomationListCmd(token),
		s.newAutomationGetCmd(token),
		s.newAutomationAddContactsCmd(token),
	)
	return cmd
}

func (s *Service) newAutomationListCmd(token string) *cobra.Command {
	var lf *listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List automations (GET /v2/automations)",
		Long: "Returns the automations with the ids `automation get` and\n" +
			"`automation add-contacts` take. An automation contains sequences, and\n" +
			"enrolling somebody needs the SEQUENCE id as well as the automation id —\n" +
			"read that from `automation get`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/automations", lf.values(), nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	lf = registerListFlags(cmd)
	return cmd
}

func (s *Service) newAutomationGetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <automation-id>",
		Short: "Get an automation (GET /v2/automations/{id})",
		Long: "Returns the automation's definition, including the sequence ids that\n" +
			"`automation add-contacts --sequence-id` requires. Takes no flags.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/v2/automations/"+url.PathEscape(args[0]), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	return cmd
}

func (s *Service) newAutomationAddContactsCmd(token string) *cobra.Command {
	var sequenceID string
	var contactIDs []string
	cmd := &cobra.Command{
		Use:   "add-contacts <automation-id>",
		Short: "Add contacts to an automation sequence (POST /v2/automations/{id}/sequences/{seq}:addContacts)",
		Long: "The automation id is the positional argument, `--sequence-id` names the\n" +
			"sequence inside it, and `--contact-id` is repeatable; all three are required\n" +
			"and checked locally. This STARTS the sequence for those contacts, so\n" +
			"whatever it does — send email, apply tags, create tasks — begins on Keap's\n" +
			"schedule. There is no matching remove verb here, so an accidental\n" +
			"enrolment has to be stopped in Keap itself.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			if sequenceID == "" {
				return &usageError{msg: "--sequence-id is required"}
			}
			if len(contactIDs) == 0 {
				return &usageError{msg: "at least one --contact-id is required"}
			}
			path := "/v2/automations/" + url.PathEscape(args[0]) + "/sequences/" + url.PathEscape(sequenceID) + ":addContacts"
			body := map[string]any{"contact_ids": contactIDs}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&sequenceID, "sequence-id", "", "sequence id within the automation (required)")
	cmd.Flags().StringArrayVar(&contactIDs, "contact-id", nil, "contact id to add (repeatable, required)")
	return cmd
}
