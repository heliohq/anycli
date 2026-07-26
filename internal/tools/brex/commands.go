package brex

import (
	"context"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// readOnly is the design-318 side-effect annotation shared by every Brex leaf:
// the tool is read-mostly and wraps only GET endpoints.
var readOnly = map[string]string{"anycli.side_effect": "false"}

// plainGet is a leaf that GETs a fixed or id-parameterized path and emits the
// JSON response verbatim (no pagination envelope handling).
func (s *Service) plainGet(ctx context.Context, token, path string) error {
	body, err := s.get(ctx, token, path, nil)
	if err != nil {
		return err
	}
	return s.emitJSON(body)
}

// list is a leaf that runs a paginated GET (--limit / --cursor / --all) over
// Brex's {items, next_cursor} envelope and emits the result.
func (s *Service) list(cmd *cobra.Command, token, path string, pf *pageFlags, extra url.Values) error {
	body, err := s.runList(cmd.Context(), token, path, pf, extra)
	if err != nil {
		return err
	}
	return s.emitJSON(body)
}

// newGetCmd is the top-level raw-GET passthrough: `brex get <path>` for the
// long tail of read endpoints without a first-class verb. --param name=value
// (repeatable) adds query parameters.
func (s *Service) newGetCmd(token string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Make a raw Brex GET request (e.g. /v2/cards)",
		Long: "The escape hatch for read endpoints that have no first-class verb. The path\n" +
			"must be RELATIVE — an absolute URL is rejected locally, because the host and\n" +
			"the credential are injected rather than caller-chosen — and a missing leading\n" +
			"slash is added. `--param name=value` is repeatable and becomes query\n" +
			"parameters. It issues GET and nothing else, so it cannot stand in for a write,\n" +
			"and it does not follow pagination: a `next_cursor` has to be fed back manually\n" +
			"through `--param cursor=...`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := normalizePath(args[0])
			if err != nil {
				return err
			}
			q, err := parseParams(params)
			if err != nil {
				return err
			}
			body, err := s.get(cmd.Context(), token, path, q)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringArrayVar(&params, "param", nil, "query parameter as name=value (repeatable)")
	return cmd
}

// normalizePath validates and normalizes a passthrough path: it must be a
// relative API path (leading slash added if missing), never an absolute URL —
// the host and credentials are injected, not caller-chosen.
func normalizePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", &usageError{msg: "brex get: empty path"}
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return "", &usageError{msg: "brex get: path must be relative (e.g. /v2/cards), not an absolute URL"}
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p, nil
}

// parseParams turns repeated name=value flags into query values.
func parseParams(vals []string) (url.Values, error) {
	q := url.Values{}
	for _, v := range vals {
		name, val, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, &usageError{msg: "brex get: --param must be name=value, got " + v}
		}
		q.Add(strings.TrimSpace(name), val)
	}
	return q, nil
}

// --- account ---

func (s *Service) newAccountCardCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "card",
		Short: "Get card account balances",
		Long: "The credit side of Brex: current and available balance on the card account, as\n" +
			"integer minor units with a currency code. It is a snapshot, not a ledger — the\n" +
			"spend behind the number is `transaction card-primary`, and the reviewable\n" +
			"records for that spend are `expense card`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/accounts/card")
		},
	}
}

func (s *Service) newAccountCashCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "cash [id]",
		Short: "List cash accounts, or get one by id",
		Long: "With no argument this lists every cash account; with one it returns that\n" +
			"account alone. Either way it is the mandatory first step before any cash ledger\n" +
			"read, because `transaction cash` requires an explicit account id and there is\n" +
			"no primary-cash shortcut matching `transaction card-primary`. Balances are\n" +
			"integer minor units.",
		Args:        cobra.MaximumNArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/v2/accounts/cash"
			if len(args) == 1 {
				path += "/" + url.PathEscape(args[0])
			}
			return s.plainGet(cmd.Context(), token, path)
		},
	}
}

// --- transaction ---

func (s *Service) newTransactionCardPrimaryCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card-primary",
		Short: "List transactions on the primary card account",
		Long: "The posted spend ledger for the PRIMARY card account specifically — it takes\n" +
			"no account id and there is no verb here for a secondary card account. Rows\n" +
			"carry the settled amount and merchant, but not the memo, receipt or review\n" +
			"state; those live on the matching expense, via `expense card`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/transactions/card/primary", pf, nil)
	}
	return cmd
}

func (s *Service) newTransactionCashCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cash <id>",
		Short: "List transactions on a cash account",
		Long: "Requires a cash account id from `account cash` — there is no default. Rows\n" +
			"cover money moving into and out of that account (transfers, payments, ACH),\n" +
			"not card spend, which is `transaction card-primary`. Prefer `--limit` for a\n" +
			"look at recent activity; `--all` walks the entire history of the account.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return s.list(cmd, token, "/v2/transactions/cash/"+url.PathEscape(args[0]), pf, nil)
	}
	return cmd
}

// --- expense ---

func (s *Service) newExpenseListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List expenses",
		Long: "Covers EVERY expense type — card spend, reimbursements and bill pay — whereas\n" +
			"`expense card` narrows to card expenses alone, so a count from here is not a\n" +
			"card-spend count. Each row carries the memo, receipt status and review state\n" +
			"the transaction ledger lacks. On an active account this is a large collection;\n" +
			"page with `--limit` before reaching for `--all`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v1/expenses", pf, nil)
	}
	return cmd
}

func (s *Service) newExpenseCardCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card",
		Short: "List card expenses",
		Long: "The card-only slice of `expense list`, and the right read for \"what did this\n" +
			"card buy, and is the receipt in\". Reimbursements and bill-pay expenses are\n" +
			"excluded by construction. Each row pairs with a transaction: the expense holds\n" +
			"merchant, memo and receipt/review state, the transaction holds the posted\n" +
			"settlement.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v1/expenses/card", pf, nil)
	}
	return cmd
}

func (s *Service) newExpenseGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one expense by id",
		Long: "Takes the EXPENSE id, which is a different id space from the transaction id\n" +
			"returned by `transaction card-primary` — an id from the ledger will not\n" +
			"resolve here. Returns the memo, merchant, receipt and review state alongside\n" +
			"the amount in integer minor units.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, "/v1/expenses/"+url.PathEscape(args[0]))
		},
	}
}

// --- card ---

func (s *Service) newCardListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issued cards",
		Long: "Every issued card with its status, last four digits, owning user id and spend\n" +
			"controls. Full card numbers are never returned. The rows identify the\n" +
			"cardholder by id only, so pair them with `user get` (or one `user list`) to\n" +
			"put names to cards. Read-only: nothing here issues, freezes or terminates a\n" +
			"card.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/cards", pf, nil)
	}
	return cmd
}

func (s *Service) newCardGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one card by id",
		Long: "One card with the fuller spend-control detail the list rows summarize. Only\n" +
			"the last four digits of the number are ever exposed. The limits shown are\n" +
			"reportable but not changeable — this tool has no write path to them.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/cards/"+url.PathEscape(args[0]))
		},
	}
}

// --- user ---

func (s *Service) newUserListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		Long: "The team directory: everyone who can hold a card or file an expense, with id,\n" +
			"name, email and status. This is the lookup that turns the bare `user_id` found\n" +
			"on a card, expense or budget into a person, and one paged read of it is\n" +
			"usually cheaper than a `user get` per row.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/users", pf, nil)
	}
	return cmd
}

func (s *Service) newUserMeCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Get the authenticated user",
		Long: "The identity anchor. Read the connected user's own id from here rather than\n" +
			"guessing it — several reads key off a user id and there is no \"my expenses\"\n" +
			"shortcut anywhere in the tool. It doubles as the cheapest confirmation that\n" +
			"the credential is live and which Brex account it belongs to.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/users/me")
		},
	}
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one user by id",
		Long: "Names the person behind a `user_id` found on a card, expense, transaction or\n" +
			"budget. For the connected user's own record use `user me`, which needs no id;\n" +
			"for more than a couple of lookups, one `user list` page beats repeated calls\n" +
			"here.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/users/"+url.PathEscape(args[0]))
		},
	}
}

// --- budget ---

func (s *Service) newBudgetListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List budgets",
		Long: "A budget is Brex's spend container — an amount, a period and a set of members,\n" +
			"with card spend attributed against it. Rows carry the limit next to current\n" +
			"spend, both as integer minor units, which is what answers \"how much of this\n" +
			"budget is left\". Read-only: budgets cannot be created or adjusted through this\n" +
			"tool.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/budgets", pf, nil)
	}
	return cmd
}

func (s *Service) newBudgetGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get one budget by id",
		Long: "One budget with its full membership and period detail. The id comes from\n" +
			"`budget list`; it is NOT a spend-limit id, which is a separate record type\n" +
			"returned by `budget spend-limits` even though the two sit in the same command\n" +
			"group.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/budgets/"+url.PathEscape(args[0]))
		},
	}
}

func (s *Service) newSpendLimitsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spend-limits",
		Short: "List spend limits",
		Long: "A spend limit is the enforcement object — the cap actually applied to a card or\n" +
			"a person — while a budget is the container it hangs under. They are distinct\n" +
			"records with distinct ids, so this is the read for \"what would stop this card\n" +
			"at the till\", not `budget list`.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/spend_limits", pf, nil)
	}
	return cmd
}

// --- department / location ---

func (s *Service) newDepartmentListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List departments",
		Long: "A reporting dimension, not a spend read: departments exist to group expenses\n" +
			"and budgets. Nothing in this tool aggregates spend BY department — take the\n" +
			"ids from these rows and correlate them against the department field on expense\n" +
			"and budget records.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/departments", pf, nil)
	}
	return cmd
}

func (s *Service) newLocationListCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locations",
		Long: "The second reporting dimension alongside `department list`, with the same\n" +
			"shape and the same caveat: it enumerates locations, it does not aggregate\n" +
			"spend by them. Correlate the ids returned here against the location field on\n" +
			"expense and budget records.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/locations", pf, nil)
	}
	return cmd
}
