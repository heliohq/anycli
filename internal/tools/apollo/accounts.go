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
		Use:   "create",
		Short: "Create an account (POST /accounts)",
		Long: "An account is a company saved into the team's own Apollo workspace — the\n" +
			"company counterpart to `contacts`, whereas `org search` / `org enrich`\n" +
			"read Apollo's global company database. --domain is a bare domain (no\n" +
			"scheme, no `www.`). Owner, phone, stage and custom fields go through\n" +
			"`--body`.",
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
		Use:   "update <account_id>",
		Short: "Update an account (PATCH /accounts/{id})",
		Long: "Takes the account id from `accounts create` or `accounts search` and\n" +
			"PATCHes only the fields given; omitted flags leave the stored value\n" +
			"untouched. Only --name and --domain are typed here — every other account\n" +
			"field goes through `--body`.",
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
		Use:   "search",
		Short: "Search saved accounts (POST /accounts/search)",
		Long: "Searches accounts already saved in the team's workspace, not Apollo's\n" +
			"global company database — `org search` does that. --q is sent as Apollo's\n" +
			"organization-name filter, so it matches on company NAME only: a domain or\n" +
			"an industry keyword will not find a record whose name does not contain it.",
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
