package plaid

import (
	"github.com/spf13/cobra"
)

// sideEffect annotates a command as mutating (true) or read-only (false) for
// policy coverage, mirroring the notion/bitly convention.
func sideEffect(v string) map[string]string { return map[string]string{"anycli.side_effect": v} }

// accessTokenFlag registers the shared, required --access-token flag used by
// every Item-scoped read/mutate command. The token is per-linked-bank runtime
// data supplied at call time — never a stored Helio credential.
func accessTokenFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVar(dst, "access-token", "", "Item access token (from the user's Link integration, or `item exchange-public-token` in sandbox)")
	_ = cmd.MarkFlagRequired("access-token")
}

// newInstitutionsGetCmd: POST /institutions/get — list supported institutions.
// No access_token required; usable in production the moment the app is approved.
func (s *Service) newInstitutionsGetCmd(c creds) *cobra.Command {
	var count, offset int
	var countryCodes, products []string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List institutions Plaid supports",
		Long: "--count is 1-500 per Plaid and defaults to 10, with --offset to page\n" +
			"through the rest. --country-codes is a comma-separated ISO-3166-1 alpha-2\n" +
			"list defaulting to US. --products narrows the list to institutions\n" +
			"supporting ALL the products named, which is the check to run before asking\n" +
			"a user to link a bank for a product that bank does not offer. No\n" +
			"--access-token is involved, so this works the moment the app is approved.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	cmd.Flags().IntVar(&count, "count", 10, "number of institutions to return (1-500)")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of institutions to skip")
	cmd.Flags().StringSliceVar(&countryCodes, "country-codes", []string{"US"}, "ISO-3166-1 alpha-2 country codes (comma-separated)")
	cmd.Flags().StringSliceVar(&products, "products", nil, "filter to institutions supporting all listed products (comma-separated)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{
			"count":         count,
			"offset":        offset,
			"country_codes": countryCodes,
		}
		if len(products) > 0 {
			payload["options"] = map[string]any{"products": products}
		}
		body, err := s.call(cmd.Context(), c, "/institutions/get", payload)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newInstitutionsGetByIDCmd: POST /institutions/get_by_id — one institution.
func (s *Service) newInstitutionsGetByIDCmd(c creds) *cobra.Command {
	var institutionID string
	var countryCodes []string
	cmd := &cobra.Command{
		Use:   "get-by-id",
		Short: "Look up one institution by its Plaid institution_id",
		Long: "--institution-id is required and is a Plaid `ins_…` id rather than a bank\n" +
			"name — resolve one with `institutions get` first. --country-codes defaults\n" +
			"to US and should name the country the institution operates in. Returns the\n" +
			"institution's supported products and status, and like `institutions get`\n" +
			"it needs no --access-token.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	cmd.Flags().StringVar(&institutionID, "institution-id", "", "Plaid institution_id (required)")
	cmd.Flags().StringSliceVar(&countryCodes, "country-codes", []string{"US"}, "ISO-3166-1 alpha-2 country codes (comma-separated)")
	_ = cmd.MarkFlagRequired("institution-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{
			"institution_id": institutionID,
			"country_codes":  countryCodes,
			// options must not be null; send an empty object.
			"options": map[string]any{},
		}
		body, err := s.call(cmd.Context(), c, "/institutions/get_by_id", payload)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newAccountsGetCmd: POST /accounts/get — accounts under an Item.
func (s *Service) newAccountsGetCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List accounts for a linked Item",
		Long: "Returns every account behind one Item — checking, savings, credit card —\n" +
			"with the `account_id` values that identify them in transaction data. Plaid\n" +
			"serves the balances on this endpoint from its cache; `accounts balance` is\n" +
			"the command that forces a live pull from the bank.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/accounts/get", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newAccountsBalanceCmd: POST /accounts/balance/get — real-time balances.
func (s *Service) newAccountsBalanceCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Fetch real-time balances for a linked Item",
		Long: "Forces a live balance pull from the bank instead of serving Plaid's cache,\n" +
			"so it is slower than `accounts get` and is the call most likely to surface\n" +
			"`ITEM_LOGIN_REQUIRED` when the user's stored bank credentials have gone\n" +
			"stale. Use it when the number has to be current, and `accounts get` when\n" +
			"the account list is what is wanted.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/accounts/balance/get", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newAuthGetCmd: POST /auth/get — account & routing numbers.
func (s *Service) newAuthGetCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch account and routing numbers for a linked Item",
		Long: "Returns the real account and routing numbers behind an Item — the data an\n" +
			"ACH transfer is originated from, and the most sensitive read in this tool.\n" +
			"It needs the `auth` product on the Item, so an Item linked for\n" +
			"transactions only fails here with a product error code rather than\n" +
			"returning blank numbers.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/auth/get", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newTransactionsSyncCmd: POST /transactions/sync — incremental cursor sync
// (Plaid's preferred transactions endpoint).
func (s *Service) newTransactionsSyncCmd(c creds) *cobra.Command {
	var accessToken, cursor string
	var count int
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync transaction updates for a linked Item (cursor-based)",
		Long: "Plaid's preferred transaction read. Call it with no --cursor for history\n" +
			"from the beginning, then re-run with the `next_cursor` from the previous\n" +
			"response to receive only what changed. The response splits into `added`,\n" +
			"`modified` and `removed`: a posted transaction can be re-categorised or\n" +
			"vanish later, so a caller that reads only `added` drifts out of sync.\n" +
			"`has_more` true means another page is already waiting and the loop should\n" +
			"continue immediately with the new cursor. --count is 1-500, default 100.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.Flags().StringVar(&cursor, "cursor", "", "resume from a prior response's next_cursor (omit for full history)")
	cmd.Flags().IntVar(&count, "count", 100, "number of updates to fetch (1-500)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{"access_token": accessToken, "count": count}
		if cursor != "" {
			payload["cursor"] = cursor
		}
		body, err := s.call(cmd.Context(), c, "/transactions/sync", payload)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newTransactionsGetCmd: POST /transactions/get — a date-windowed read.
func (s *Service) newTransactionsGetCmd(c creds) *cobra.Command {
	var accessToken, startDate, endDate string
	var count, offset int
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch transactions in a date window for a linked Item",
		Long: "--start-date and --end-date are both required, as YYYY-MM-DD. This is the\n" +
			"windowed read; `transactions sync` is Plaid's preferred path for keeping a\n" +
			"copy current, because this endpoint gives no way to notice a transaction\n" +
			"that was later modified or removed. --count is 1-500 (default 100) and\n" +
			"--offset pages within the window. A freshly linked Item can answer\n" +
			"`PRODUCT_NOT_READY` here until Plaid finishes its first pull.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.Flags().StringVar(&startDate, "start-date", "", "earliest date, YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&endDate, "end-date", "", "latest date, YYYY-MM-DD (required)")
	cmd.Flags().IntVar(&count, "count", 100, "number of transactions to fetch (1-500)")
	cmd.Flags().IntVar(&offset, "offset", 0, "number of transactions to skip")
	_ = cmd.MarkFlagRequired("start-date")
	_ = cmd.MarkFlagRequired("end-date")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload := map[string]any{
			"access_token": accessToken,
			"start_date":   startDate,
			"end_date":     endDate,
			"options":      map[string]any{"count": count, "offset": offset},
		}
		body, err := s.call(cmd.Context(), c, "/transactions/get", payload)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newIdentityGetCmd: POST /identity/get — account-holder identity.
func (s *Service) newIdentityGetCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch account-holder identity for a linked Item",
		Long: "Returns the account holder's name, email, phone and address as the BANK\n" +
			"holds them, which is what makes it useful for verifying against what a\n" +
			"user typed. It needs the `identity` product on the Item; an Item linked\n" +
			"without it fails with a product error code.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/identity/get", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newItemGetCmd: POST /item/get — Item metadata & status.
func (s *Service) newItemGetCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch metadata and status for a linked Item",
		Long: "Returns the Item's institution id, the products it was linked with, and\n" +
			"its `error` field. That error field is the point of the call: a non-null\n" +
			"`ITEM_LOGIN_REQUIRED` there explains why every other read on this token is\n" +
			"failing, and it clears only once the user re-authenticates through Link.\n" +
			"No account or transaction data comes back here.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("false"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/item/get", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newItemRemoveCmd: POST /item/remove — unlink an Item (invalidates its token).
func (s *Service) newItemRemoveCmd(c creds) *cobra.Command {
	var accessToken string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove (unlink) an Item and invalidate its access token",
		Long: "Unlinks the Item at Plaid and permanently invalidates the --access-token\n" +
			"passed in. There is no undo: restoring access means the user goes through\n" +
			"Link again and a fresh public_token is exchanged for a new access_token.\n" +
			"Nothing changes at the bank itself — only Plaid's connection to it.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("true"),
	}
	accessTokenFlag(cmd, &accessToken)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/item/remove", map[string]any{"access_token": accessToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newItemExchangePublicTokenCmd: POST /item/public_token/exchange — turn a
// public_token (from Link, or `sandbox public-token-create`) into an
// access_token the read commands consume.
func (s *Service) newItemExchangePublicTokenCmd(c creds) *cobra.Command {
	var publicToken string
	cmd := &cobra.Command{
		Use:   "exchange-public-token",
		Short: "Exchange a public_token for an access_token",
		Long: "A one-time exchange: the `public_token` from Link's onSuccess callback, or\n" +
			"from `sandbox public-token-create`, becomes the long-lived `access_token`\n" +
			"every Item-scoped command needs. Plaid expires a public_token 30 minutes\n" +
			"after it is minted, so exchange promptly. The access_token in the response\n" +
			"is not stored anywhere by this tool — capture it, because the only way to\n" +
			"obtain another one for the same bank is to link it again.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("true"),
	}
	cmd.Flags().StringVar(&publicToken, "public-token", "", "public_token from Link onSuccess or `sandbox public-token-create` (required)")
	_ = cmd.MarkFlagRequired("public-token")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		body, err := s.call(cmd.Context(), c, "/item/public_token/exchange", map[string]any{"public_token": publicToken})
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}

// newSandboxPublicTokenCreateCmd: POST /sandbox/public_token/create — mint a
// public_token for a test institution fully server-side. Refuses (exit 2) under
// PLAID_ENV=production, where the endpoint does not exist; an explicit refusal
// beats a confusing 404 passthrough.
func (s *Service) newSandboxPublicTokenCreateCmd(c creds) *cobra.Command {
	var institutionID string
	var products []string
	cmd := &cobra.Command{
		Use:   "public-token-create",
		Short: "Mint a sandbox public_token for a test institution (sandbox only)",
		Long: "Refuses with a usage error against a production connection, where the\n" +
			"endpoint does not exist. --institution-id is a sandbox test institution\n" +
			"such as `ins_109508`. --products sets the Item's initial products and\n" +
			"defaults to `transactions`, so name every product the test will read — the\n" +
			"Item is created with exactly that set. Feed the result to `item\n" +
			"exchange-public-token` to stand up a complete test Item with no browser\n" +
			"and no Link widget.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect("true"),
	}
	cmd.Flags().StringVar(&institutionID, "institution-id", "", "sandbox institution_id, e.g. ins_109508 (required)")
	cmd.Flags().StringSliceVar(&products, "products", []string{"transactions"}, "initial products for the Item (comma-separated)")
	_ = cmd.MarkFlagRequired("institution-id")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if c.env == envProduction {
			return &usageError{msg: "sandbox public-token-create is not available when PLAID_ENV=production"}
		}
		payload := map[string]any{
			"institution_id":   institutionID,
			"initial_products": products,
		}
		body, err := s.call(cmd.Context(), c, "/sandbox/public_token/create", payload)
		if err != nil {
			return err
		}
		return s.emit(body)
	}
	return cmd
}
