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
		Args:  cobra.NoArgs,
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
		Args:  cobra.NoArgs,
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
