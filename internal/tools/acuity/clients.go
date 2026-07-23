package acuity

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newClientCmd(token string) *cobra.Command {
	cmd := newGroupCmd("client", "Clients (lookup, create, update, delete)")
	cmd.AddCommand(
		s.newClientListCmd(token),
		s.newClientCreateCmd(token),
		s.newClientUpdateCmd(token),
		s.newClientDeleteCmd(token),
	)
	return cmd
}

func (s *Service) newClientListCmd(token string) *cobra.Command {
	var search string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List / search clients (GET /clients)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			setStringQuery(q, "search", search)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/clients", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "match against client name / email / phone")
	return cmd
}

func (s *Service) newClientCreateCmd(token string) *cobra.Command {
	var firstName, lastName, email, phone, notes string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a client (POST /clients)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := map[string]any{"firstName": firstName, "lastName": lastName}
			setStringIfSet(body, "email", email)
			setStringIfSet(body, "phone", phone)
			setStringIfSet(body, "notes", notes)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/clients", nil, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "client first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "client last name")
	cmd.Flags().StringVar(&email, "email", "", "client email")
	cmd.Flags().StringVar(&phone, "phone", "", "client phone")
	cmd.Flags().StringVar(&notes, "notes", "", "client notes")
	_ = cmd.MarkFlagRequired("first-name")
	_ = cmd.MarkFlagRequired("last-name")
	return cmd
}

func (s *Service) newClientUpdateCmd(token string) *cobra.Command {
	var firstName, lastName, email, phone, notes string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a client, identified by name (PUT /clients)",
		Long: "Update a client. Acuity keys the update on the client's name: --first-name and " +
			"--last-name identify the client (sent as query params) and the updated values are " +
			"sent in the body. --phone/--email/--notes set the new values.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Identity in the query string; new values (with the identity echoed,
			// as Acuity's PUT /clients requires) in the body.
			q := url.Values{}
			q.Set("firstName", firstName)
			q.Set("lastName", lastName)
			body := map[string]any{"firstName": firstName, "lastName": lastName}
			setStringIfSet(body, "email", email)
			setStringIfSet(body, "phone", phone)
			setStringIfSet(body, "notes", notes)
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/clients", q, body)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "client first name (identifies the client)")
	cmd.Flags().StringVar(&lastName, "last-name", "", "client last name (identifies the client)")
	cmd.Flags().StringVar(&email, "email", "", "new email")
	cmd.Flags().StringVar(&phone, "phone", "", "new phone")
	cmd.Flags().StringVar(&notes, "notes", "", "new notes")
	_ = cmd.MarkFlagRequired("first-name")
	_ = cmd.MarkFlagRequired("last-name")
	return cmd
}

func (s *Service) newClientDeleteCmd(token string) *cobra.Command {
	var firstName, lastName, phone string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a client, identified by name (DELETE /clients)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("firstName", firstName)
			q.Set("lastName", lastName)
			setStringQuery(q, "phone", phone)
			resp, err := s.call(cmd.Context(), token, http.MethodDelete, "/clients", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&firstName, "first-name", "", "client first name (identifies the client)")
	cmd.Flags().StringVar(&lastName, "last-name", "", "client last name (identifies the client)")
	cmd.Flags().StringVar(&phone, "phone", "", "client phone (disambiguates duplicate names)")
	_ = cmd.MarkFlagRequired("first-name")
	_ = cmd.MarkFlagRequired("last-name")
	return cmd
}
