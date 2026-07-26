package pandadoc

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newContactListCmd(authz string) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts (optionally filter by exact email)",
		Long: "--email is an EXACT match rather than a search, so a partial address\n" +
			"returns nothing. Contacts are a convenience record of names, companies and\n" +
			"phone numbers; `document create --recipient` accepts a bare email and does\n" +
			"not require the person to exist as a contact first.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			query := url.Values{}
			setIf(query, "email", email)
			body, err := s.call(cmd.Context(), authz, http.MethodGet, "/contacts", query, nil)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderList(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by exact email match")
	return cmd
}

func (s *Service) newContactCreateCmd(authz string) *cobra.Command {
	var email, first, last, company, phone string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Long: "--email is required and identifies the contact. Creating one is optional\n" +
			"for sending — recipients on `document create` are accepted as bare emails.\n" +
			"It pays off when the same person is sent documents repeatedly, since their\n" +
			"name and company are then reused instead of retyped per document.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload := map[string]any{"email": email}
			if first != "" {
				payload["first_name"] = first
			}
			if last != "" {
				payload["last_name"] = last
			}
			if company != "" {
				payload["company"] = company
			}
			if phone != "" {
				payload["phone"] = phone
			}
			body, err := s.call(cmd.Context(), authz, http.MethodPost, "/contacts", nil, payload)
			if err != nil {
				return err
			}
			if jsonOut(cmd) {
				return s.emitJSON(body)
			}
			return s.renderItem(body)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "contact email")
	cmd.Flags().StringVar(&first, "first", "", "first name")
	cmd.Flags().StringVar(&last, "last", "", "last name")
	cmd.Flags().StringVar(&company, "company", "", "company name")
	cmd.Flags().StringVar(&phone, "phone", "", "phone number")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}
