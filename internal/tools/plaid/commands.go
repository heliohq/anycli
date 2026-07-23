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
		Use:         "get",
		Short:       "List institutions Plaid supports",
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
		Use:         "get-by-id",
		Short:       "Look up one institution by its Plaid institution_id",
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
		Use:         "get",
		Short:       "List accounts for a linked Item",
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
		Use:         "balance",
		Short:       "Fetch real-time balances for a linked Item",
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
		Use:         "get",
		Short:       "Fetch account and routing numbers for a linked Item",
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
		Use:         "sync",
		Short:       "Sync transaction updates for a linked Item (cursor-based)",
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
		Use:         "get",
		Short:       "Fetch transactions in a date window for a linked Item",
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
		Use:         "get",
		Short:       "Fetch account-holder identity for a linked Item",
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
		Use:         "get",
		Short:       "Fetch metadata and status for a linked Item",
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
		Use:         "remove",
		Short:       "Remove (unlink) an Item and invalidate its access token",
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
		Use:         "exchange-public-token",
		Short:       "Exchange a public_token for an access_token",
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
		Use:         "public-token-create",
		Short:       "Mint a sandbox public_token for a test institution (sandbox only)",
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
