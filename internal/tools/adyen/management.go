package adyen

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"
)

// readOnly marks a leaf command as side-effect-free for the design-318 approval
// gate. Every v1 command is a Management GET, so all leaves carry it.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// pageFlags holds the shared Management pagination window. Adyen list endpoints
// accept offset-based pageSize/pageNumber (default pageSize 10, terminals 20;
// max 100). Unset flags are omitted so Adyen applies its own defaults.
type pageFlags struct {
	pageSize int
	page     int
}

func registerPageFlags(cmd *cobra.Command, p *pageFlags) {
	cmd.Flags().IntVar(&p.pageSize, "page-size", 0, "items per page (Adyen max 100; 0 = provider default)")
	cmd.Flags().IntVar(&p.page, "page", 0, "page number to fetch (1-based; 0 = provider default)")
}

func (p pageFlags) apply(q url.Values) {
	if p.pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(p.pageSize))
	}
	if p.page > 0 {
		q.Set("pageNumber", strconv.Itoa(p.page))
	}
}

// getEmit runs a GET and passes the body through to stdout verbatim.
func (s *Service) getEmit(cmd *cobra.Command, key, path string, query url.Values) error {
	body, err := s.call(cmd.Context(), key, http.MethodGet, path, query)
	if err != nil {
		return err
	}
	return s.emit(body)
}

func (s *Service) newManagementCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "management", Short: "Management API v3 (accounts, config, webhooks, terminals)"}
	cmd.AddCommand(
		s.newWhoamiCmd(key),
		s.newMerchantCmd(key),
		s.newCompanyCmd(key),
		s.newPaymentMethodsCmd(key),
		s.newWebhookCmd(key),
		s.newStoreCmd(key),
		s.newTerminalCmd(key),
	)
	return cmd
}

func (s *Service) newWhoamiCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show this API credential's identity (GET /me)",
		Long: "Reports which company and merchant accounts the credential is bound to\n" +
			"and which roles it carries — the account ids every other command in this\n" +
			"tree needs as input. It succeeds with ANY Management role, so a clean\n" +
			"result proves the key is live but says nothing about whether a\n" +
			"particular later command is permitted.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.getEmit(cmd, key, "/me", nil)
		},
	}
}

func (s *Service) newMerchantCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "merchant", Short: "Merchant accounts"}

	var page pageFlags
	list := &cobra.Command{
		Use:   "list",
		Short: "List merchant accounts (GET /merchants)",
		Long: "The merchantId is the key most of this tree turns on:\n" +
			"`payment-methods list`, `store list`, `terminal list --merchant` and the\n" +
			"--merchant scope of the webhook commands all take one from here. Adyen\n" +
			"merchant ids are opaque strings, commonly the company name with an\n" +
			"ECOM or POS suffix, never numbers. Needs the Accounts read role.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			return s.getEmit(cmd, key, "/merchants", q)
		},
	}
	registerPageFlags(list, &page)

	get := &cobra.Command{
		Use:   "get <merchantId>",
		Short: "Get a merchant account (GET /merchants/{merchantId})",
		Long: "Takes the merchantId positionally, exactly as `merchant list` prints it.\n" +
			"The record includes the account's status, which is what distinguishes a\n" +
			"merchant that exists from one that is actually live for traffic. Needs\n" +
			"the Accounts read role.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getEmit(cmd, key, "/merchants/"+url.PathEscape(args[0]), nil)
		},
	}
	cmd.AddCommand(list, get)
	return cmd
}

func (s *Service) newCompanyCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "company", Short: "Company accounts"}

	var page pageFlags
	list := &cobra.Command{
		Use:   "list",
		Short: "List company accounts (GET /companies)",
		Long: "A company account is the parent that owns merchant accounts, so a\n" +
			"company-level key sees several merchants while a merchant-level key sees\n" +
			"only its own. The ids here are what the webhook commands accept as\n" +
			"--company. Needs the Accounts read role.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			page.apply(q)
			return s.getEmit(cmd, key, "/companies", q)
		},
	}
	registerPageFlags(list, &page)

	get := &cobra.Command{
		Use:   "get <companyId>",
		Short: "Get a company account (GET /companies/{companyId})",
		Long: "Takes the companyId positionally, exactly as `company list` prints it.\n" +
			"Configuration read at this level sits above the merchant accounts\n" +
			"beneath it, so it describes the whole account family rather than one\n" +
			"storefront. Needs the Accounts read role.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getEmit(cmd, key, "/companies/"+url.PathEscape(args[0]), nil)
		},
	}
	cmd.AddCommand(list, get)
	return cmd
}

func (s *Service) newPaymentMethodsCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "payment-methods", Short: "Payment-method settings"}
	list := &cobra.Command{
		Use:   "list <merchantId>",
		Short: "List a merchant's payment-method settings (GET /merchants/{merchantId}/paymentMethodSettings)",
		Long: "Reports what is CONFIGURED on the merchant, not what a shopper is\n" +
			"offered at checkout — what a shopper sees also depends on amount,\n" +
			"country and currency. Each entry carries the method type with its\n" +
			"enablement and currency/store scoping, which is where a \"why is this\n" +
			"method missing\" question is normally settled. Needs the Payment methods\n" +
			"read role.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.getEmit(cmd, key, "/merchants/"+url.PathEscape(args[0])+"/paymentMethodSettings", nil)
		},
	}
	cmd.AddCommand(list)
	return cmd
}

func (s *Service) newWebhookCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "webhook", Short: "Configured webhooks"}
	cmd.AddCommand(s.newWebhookListCmd(key), s.newWebhookGetCmd(key))
	return cmd
}

func (s *Service) newWebhookListCmd(key string) *cobra.Command {
	var merchant, company string
	list := &cobra.Command{
		Use:   "list",
		Short: "List webhooks (GET /{merchants|companies}/{id}/webhooks)",
		Long: "Pass exactly one of --merchant or --company; neither and both are\n" +
			"rejected locally before any request goes out. The two scopes are\n" +
			"different sets, not filters on one set: a webhook configured on the\n" +
			"company covers every merchant beneath it yet does NOT appear in the\n" +
			"merchant-scoped list, so a merchant showing no webhooks can still be\n" +
			"delivering events. Needs the Webhooks read role.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, err := webhookScopePath(merchant, company)
			if err != nil {
				return err
			}
			return s.getEmit(cmd, key, base+"/webhooks", nil)
		},
	}
	registerScopeFlags(list, &merchant, &company)
	return list
}

func (s *Service) newWebhookGetCmd(key string) *cobra.Command {
	var merchant, company string
	get := &cobra.Command{
		Use:   "get <webhookId>",
		Short: "Get one webhook (GET /{merchants|companies}/{id}/webhooks/{webhookId})",
		Long: "The webhookId is positional but still needs its scope — pass the same\n" +
			"--merchant or --company it was listed under, because the id alone does\n" +
			"not resolve to a path. Returns the target url, the subscribed event\n" +
			"types and the active flag, but never the HMAC key: Adyen reveals that\n" +
			"only at the moment a webhook is created.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			base, err := webhookScopePath(merchant, company)
			if err != nil {
				return err
			}
			return s.getEmit(cmd, key, base+"/webhooks/"+url.PathEscape(args[0]), nil)
		},
	}
	registerScopeFlags(get, &merchant, &company)
	return get
}

// registerScopeFlags wires the mutually-exclusive --merchant / --company scope
// selectors used by the webhook commands.
func registerScopeFlags(cmd *cobra.Command, merchant, company *string) {
	cmd.Flags().StringVar(merchant, "merchant", "", "merchant account id (mutually exclusive with --company)")
	cmd.Flags().StringVar(company, "company", "", "company account id (mutually exclusive with --merchant)")
}

// webhookScopePath resolves exactly one of --merchant / --company into the
// resource base path, or a usageError when neither or both are set.
func webhookScopePath(merchant, company string) (string, error) {
	switch {
	case merchant != "" && company != "":
		return "", &usageError{msg: "pass exactly one of --merchant or --company, not both"}
	case merchant != "":
		return "/merchants/" + url.PathEscape(merchant), nil
	case company != "":
		return "/companies/" + url.PathEscape(company), nil
	default:
		return "", &usageError{msg: "webhook commands require one of --merchant <id> or --company <id>"}
	}
}

func (s *Service) newStoreCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "store", Short: "In-person stores"}
	var page pageFlags
	list := &cobra.Command{
		Use:   "list <merchantId>",
		Short: "List a merchant's stores (GET /merchants/{merchantId}/stores)",
		Long: "Stores are the physical point-of-sale locations under a merchant, so a\n" +
			"purely online merchant legitimately returns none — an empty list is not\n" +
			"evidence of a permission problem. Terminals are assigned to these store\n" +
			"ids. Needs the Stores read role.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			page.apply(q)
			return s.getEmit(cmd, key, "/merchants/"+url.PathEscape(args[0])+"/stores", q)
		},
	}
	registerPageFlags(list, &page)
	cmd.AddCommand(list)
	return cmd
}

func (s *Service) newTerminalCmd(key string) *cobra.Command {
	cmd := &cobra.Command{Use: "terminal", Short: "Payment terminals"}
	// Management v3 terminals live at the TOP-LEVEL GET /terminals endpoint,
	// filtered by merchantIds — not nested under /merchants/{id}/terminals.
	var merchant string
	var page pageFlags
	list := &cobra.Command{
		Use:   "list",
		Short: "List terminals (GET /terminals; optional --merchant filter)",
		Long: "Rides Adyen's TOP-LEVEL terminals endpoint rather than a per-merchant\n" +
			"one, so with no --merchant it returns every terminal the credential can\n" +
			"see across all merchant accounts; --merchant narrows it through the\n" +
			"merchantIds filter. Adyen's default page size on this endpoint is 20,\n" +
			"not the 10 the other lists use. Needs the Terminals read role.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if merchant != "" {
				q.Set("merchantIds", merchant)
			}
			page.apply(q)
			return s.getEmit(cmd, key, "/terminals", q)
		},
	}
	list.Flags().StringVar(&merchant, "merchant", "", "filter by merchant account id (merchantIds)")
	registerPageFlags(list, &page)
	cmd.AddCommand(list)
	return cmd
}
