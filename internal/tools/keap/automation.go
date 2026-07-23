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
		Args:  cobra.NoArgs,
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(1),
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
