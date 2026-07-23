package mailjet

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newListCmd groups contact-list management over /v3/REST/contactslist plus
// list membership over /v3/REST/listrecipient.
func (s *Service) newListCmd(basic string) *cobra.Command {
	cmd := newGroupCmd("list", "Manage contact lists (list, create, add-contact)")
	cmd.AddCommand(
		s.newListListCmd(basic),
		s.newListCreateCmd(basic),
		s.newListAddContactCmd(basic),
	)
	return cmd
}

func (s *Service) newListListCmd(basic string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List contact lists (GET /v3/REST/contactslist)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			q := url.Values{}
			q.Set("Limit", itoa(limit))
			q.Set("Offset", itoa(offset))
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodGet, "/v3/REST/contactslist", q, nil)
			if err != nil {
				return err
			}
			return s.emitList(resp)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "max lists to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "pagination offset")
	return cmd
}

func (s *Service) newListCreateCmd(basic string) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a contact list (POST /v3/REST/contactslist)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			body := map[string]any{"Name": name}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodPost, "/v3/REST/contactslist", nil, body)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "list name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// newListAddContactCmd (un)subscribes a contact to a list via a listrecipient
// row (POST /v3/REST/listrecipient with ContactID + ListID).
func (s *Service) newListAddContactCmd(basic string) *cobra.Command {
	var contactID, listID int64
	var unsubscribed bool
	cmd := &cobra.Command{
		Use:         "add-contact",
		Short:       "Add a contact to a list (POST /v3/REST/listrecipient)",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := s.resolveBaseURL(cmd)
			if err != nil {
				return err
			}
			body := map[string]any{
				"ContactID":      contactID,
				"ListID":         listID,
				"IsUnsubscribed": unsubscribed,
			}
			resp, err := s.call(cmd.Context(), basic, baseURL, http.MethodPost, "/v3/REST/listrecipient", nil, body)
			if err != nil {
				return err
			}
			return s.emitOne(resp)
		},
	}
	cmd.Flags().Int64Var(&contactID, "contact-id", 0, "contact ID")
	cmd.Flags().Int64Var(&listID, "list-id", 0, "list ID")
	cmd.Flags().BoolVar(&unsubscribed, "unsubscribed", false, "add as unsubscribed instead of subscribed")
	_ = cmd.MarkFlagRequired("contact-id")
	_ = cmd.MarkFlagRequired("list-id")
	return cmd
}
