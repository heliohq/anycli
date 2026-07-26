package salesloft

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newAccountCmd groups account lookup and upkeep.
func (s *Service) newAccountCmd(token string) *cobra.Command {
	cmd := newGroupCmd("account", "Manage accounts")
	cmd.AddCommand(
		s.newAccountListCmd(token),
		s.newAccountGetCmd(token),
		s.newAccountCreateCmd(token),
		s.newAccountUpdateCmd(token),
	)
	return cmd
}

func (s *Service) newAccountListCmd(token string) *cobra.Command {
	var lf listFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accounts (GET /v2/accounts)",
		Long: "Accounts are companies; the people at them are `person` records carrying\n" +
			"an `account_id`. No named filter exists here beyond the shared list\n" +
			"controls, so narrowing goes through `--filter` with Salesloft's\n" +
			"documented account filters. `--updated-since` with\n" +
			"`--sort-by updated_at` is the cheap way to track change against the\n" +
			"team's shared rate budget.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := lf.values()
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/accounts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerListFlags(cmd, &lf)
	return cmd
}

func (s *Service) newAccountGetCmd(token string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch one account (GET /v2/accounts/{id})",
		Long: "`--id` is required. Returns the company record — name, domain, website,\n" +
			"industry, owner and Salesloft's counters — and nothing about the people\n" +
			"at it. To reach those, filter `person list` by this account's id\n" +
			"instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/accounts/"+id, nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "account id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func (s *Service) newAccountCreateCmd(token string) *cobra.Command {
	var name, domain, website, industry, body string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an account (POST /v2/accounts)",
		Long: "`--name`, `--domain`, `--website` and `--industry` have flags; every\n" +
			"other field goes through `--body`, whose keys override them. Nothing\n" +
			"checks whether the company already exists, so search `account list`\n" +
			"first — a duplicate account splits its people across two records and\n" +
			"there is no merge here. Keep the returned id: it is what\n" +
			"`person create --account-id` takes.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := mergeBody(accountNamedBody(name, domain, website, industry), body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPost, "/accounts", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerAccountWriteFlags(cmd, &name, &domain, &website, &industry, &body)
	return cmd
}

func (s *Service) newAccountUpdateCmd(token string) *cobra.Command {
	var id, name, domain, website, industry, body string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an account (PUT /v2/accounts/{id})",
		Long: "`--id` is required and the write is partial: only the flags passed are\n" +
			"sent and everything else is left alone. `--body` overrides the named\n" +
			"flags and is the only way to reach fields without one. Changing\n" +
			"`--domain` does not re-link anybody — people already attached keep their\n" +
			"`account_id`, and people at the new domain are not pulled in.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
		RunE: func(cmd *cobra.Command, _ []string) error {
			payload, err := mergeBody(accountNamedBody(name, domain, website, industry), body)
			if err != nil {
				return err
			}
			resp, err := s.call(cmd.Context(), token, http.MethodPut, "/accounts/"+id, nil, payload)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "account id")
	_ = cmd.MarkFlagRequired("id")
	registerAccountWriteFlags(cmd, &name, &domain, &website, &industry, &body)
	return cmd
}

// accountNamedBody builds the account body from the named flags, omitting empty
// fields so an update only touches what was passed.
func accountNamedBody(name, domain, website, industry string) map[string]any {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if domain != "" {
		body["domain"] = domain
	}
	if website != "" {
		body["website"] = website
	}
	if industry != "" {
		body["industry"] = industry
	}
	return body
}

func registerAccountWriteFlags(cmd *cobra.Command, name, domain, website, industry, body *string) {
	cmd.Flags().StringVar(name, "name", "", "account name")
	cmd.Flags().StringVar(domain, "domain", "", "primary domain")
	cmd.Flags().StringVar(website, "website", "", "website URL")
	cmd.Flags().StringVar(industry, "industry", "", "industry")
	cmd.Flags().StringVar(body, "body", "", "raw JSON body; keys override the named flags for full fidelity")
}
