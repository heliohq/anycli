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
		Long: "--search is one string matched across a client's name, email and phone;\n" +
			"there is no per-field filter and no pagination flag on this endpoint. This\n" +
			"is the account's address book — the client details stored on a single\n" +
			"booking are edited with `appointment update` instead. Run it before\n" +
			"`client update` or `client delete`, both of which identify a client by\n" +
			"name and will act on whichever record matches.",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
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
		Long: "--first-name and --last-name are required and together become the client's\n" +
			"identity for `client update` and `client delete`, neither of which has an\n" +
			"id-based path — so two clients sharing a name are afterwards separable\n" +
			"only by phone. Creating a client books nothing; `appointment create`\n" +
			"carries its own client fields and does not need a record here first.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
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
		Use:         "update",
		Short:       "Update a client, identified by name (PUT /clients)",
		Annotations: writeAction,
		Long: "Keyed on the NAME, not an id: --first-name and --last-name select which\n" +
			"client to change and are echoed back into the body unchanged, so this\n" +
			"command cannot rename anyone. --email, --phone and --notes carry the new\n" +
			"values and only the ones passed are sent. Duplicate names cannot be\n" +
			"disambiguated here — unlike `client delete` there is no --phone selector,\n" +
			"so confirm with `client list --search` first.",
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
		Long: "Keyed on the NAME: --first-name and --last-name are required and --phone\n" +
			"disambiguates duplicates. With duplicate names and no --phone, whichever\n" +
			"record Acuity matches is the one removed, so confirm the target with\n" +
			"`client list --search` first. There is no id-based delete and nothing in\n" +
			"this tool restores a deleted client.",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
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
