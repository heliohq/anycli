package omnisend

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newContactCmd builds the `contact` resource group: the audience the teammate
// looks up, adds, and tags.
func (s *Service) newContactCmd(token string) *cobra.Command {
	cmd := newGroupCmd("contact", "Contacts (list, get, create, update)")
	cmd.AddCommand(
		s.newContactListCmd(token),
		s.newContactGetCmd(token),
		s.newContactCreateCmd(token),
		s.newContactUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newContactListCmd(token string) *cobra.Command {
	var email string
	var limit int
	var after string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (GET /contacts)",
		Long: "--email filters to one address and is the only search dimension this\n" +
			"endpoint exposes — there is no name, tag or segment filter, so anything\n" +
			"narrower means paging the whole audience and filtering client-side. Each\n" +
			"entry carries the `contactID` that `contact get` and `contact update`\n" +
			"take.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			applyListQuery(q, limit, after)
			if email != "" {
				q.Set("email", email)
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by contact email")
	registerListFlags(cmd, &limit, &after)
	return cmd
}

func (s *Service) newContactGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a contact by id (GET /contacts/{id})",
		Long: "--id is the Omnisend `contactID` from a `contact list` response, not the\n" +
			"email address; look the address up with `contact list --email` first.\n" +
			"The response carries the per-channel subscription status, which is what\n" +
			"decides whether a campaign or automation may actually mail this person —\n" +
			"an existing contact is not necessarily a mailable one.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/contacts/"+url.PathEscape(id), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newContactCreateCmd(token string) *cobra.Command {
	var data string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact (POST /contacts). --data is the raw Omnisend contact JSON body.",
		Long: "--data is the whole Omnisend contact body: an `identifiers` array whose\n" +
			"entries pair a `type` and `id` (an email address, a phone number) with a\n" +
			"`channels` object holding that channel's subscription `status`, plus\n" +
			"optional top-level fields such as `firstName` and `tags`. Consent lives\n" +
			"inside `channels`, so a contact created without an explicit subscribed\n" +
			"status is stored but unmailable.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/contacts", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "raw contact JSON body (Omnisend contact schema)")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func (s *Service) newContactUpdateCmd(token string) *cobra.Command {
	var id, data string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a contact (PATCH /contacts/{id}). --data is the raw partial JSON body.",
		Long: "A PATCH: fields absent from --data are left as they are, and a field\n" +
			"that is present replaces its previous value outright — an array such as\n" +
			"`tags` must therefore be sent complete, since there is no append form.\n" +
			"--id is the `contactID`, not the email address. Changing whether the\n" +
			"contact may be mailed means updating the status inside `channels`, not a\n" +
			"top-level field.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := decodeJSONFlag("data", data)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/contacts/"+url.PathEscape(id), nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contact id")
	cmd.Flags().StringVar(&data, "data", "", "raw partial contact JSON body")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}
