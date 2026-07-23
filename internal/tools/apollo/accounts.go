package apollo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newAccountsCmd builds the `accounts` group: saved-company CRUD + search.
func (s *Service) newAccountsCmd(token string) *cobra.Command {
	cmd := newGroupCmd("accounts", "Manage saved accounts (companies)")
	cmd.AddCommand(
		s.newAccountsCreateCmd(token),
		s.newAccountsUpdateCmd(token),
		s.newAccountsSearchCmd(token),
	)
	return cmd
}

// newAccountsCreateCmd wraps POST /accounts.
func (s *Service) newAccountsCreateCmd(token string) *cobra.Command {
	var body, name, domain string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create an account (POST /accounts)",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "name", name)
			setStr(b, "domain", domain)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/accounts", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "account (company) name")
	cmd.Flags().StringVar(&domain, "domain", "", "company domain")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newAccountsUpdateCmd wraps PATCH /accounts/{id}.
func (s *Service) newAccountsUpdateCmd(token string) *cobra.Command {
	var body, name, domain string
	cmd := &cobra.Command{
		Use:         "update <account_id>",
		Short:       "Update an account (PATCH /accounts/{id})",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "name", name)
			setStr(b, "domain", domain)
			resp, err := s.call(cmd.Context(), token, http.MethodPatch, "/accounts/"+url.PathEscape(args[0]), nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "account (company) name")
	cmd.Flags().StringVar(&domain, "domain", "", "company domain")
	registerBodyFlag(cmd, &body)
	return cmd
}

// newAccountsSearchCmd wraps POST /accounts/search.
func (s *Service) newAccountsSearchCmd(token string) *cobra.Command {
	var body, q string
	var page, perPage int
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search saved accounts (POST /accounts/search)",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := bodyFromFlag(body)
			if err != nil {
				return err
			}
			setStr(b, "q_organization_name", q)
			applyPageBody(b, page, perPage)
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/accounts/search", nil, b)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&q, "q", "", "company-name keyword filter")
	registerPageFlags(cmd, &page, &perPage)
	registerBodyFlag(cmd, &body)
	return cmd
}
