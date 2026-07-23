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
		Use:         "get <path>",
		Short:       "Make a raw Brex GET request (e.g. /v2/cards)",
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
		Use:         "card",
		Short:       "Get card account balances",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/accounts/card")
		},
	}
}

func (s *Service) newAccountCashCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "cash [id]",
		Short:       "List cash accounts, or get one by id",
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
		Use:         "card-primary",
		Short:       "List transactions on the primary card account",
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
		Use:         "cash <id>",
		Short:       "List transactions on a cash account",
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
		Use:         "list",
		Short:       "List expenses",
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
		Use:         "card",
		Short:       "List card expenses",
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
		Use:         "get <id>",
		Short:       "Get one expense by id",
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
		Use:         "list",
		Short:       "List issued cards",
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
		Use:         "get <id>",
		Short:       "Get one card by id",
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
		Use:         "list",
		Short:       "List users",
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
		Use:         "me",
		Short:       "Get the authenticated user",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/users/me")
		},
	}
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one user by id",
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
		Use:         "list",
		Short:       "List budgets",
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
		Use:         "get <id>",
		Short:       "Get one budget by id",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, "/v2/budgets/"+url.PathEscape(args[0]))
		},
	}
}

func (s *Service) newSpendLimitsCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "spend-limits",
		Short:       "List spend limits",
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
		Use:         "list",
		Short:       "List departments",
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
		Use:         "list",
		Short:       "List locations",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, "/v2/locations", pf, nil)
	}
	return cmd
}
