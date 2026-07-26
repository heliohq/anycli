package ramp

import (
	"context"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// readOnly is the design-318 side-effect annotation shared by every Ramp leaf:
// the tool is read-only and wraps only GET endpoints.
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
// Ramp's {data, page.next} envelope and emits the result.
func (s *Service) list(cmd *cobra.Command, token, path string, pf *pageFlags, extra url.Values) error {
	body, err := s.runList(cmd.Context(), token, path, pf, extra)
	if err != nil {
		return err
	}
	return s.emitJSON(body)
}

// newListCmd builds a paginated `list` subcommand for a fixed collection path.
func (s *Service) newListCmd(token, short, long, path string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       short,
		Long:        long,
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return s.list(cmd, token, path, pf, nil)
	}
	return cmd
}

// newGetByIDCmd builds a `get <id>` subcommand that GETs collection/{id}.
func (s *Service) newGetByIDCmd(token, short, long, collection string) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       short,
		Long:        long,
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.plainGet(cmd.Context(), token, collection+"/"+url.PathEscape(args[0]))
		},
	}
}

// newGetCmd is the top-level raw-GET passthrough: `ramp get <path>` for the
// long tail of read endpoints without a first-class verb. --param name=value
// (repeatable) adds query parameters.
func (s *Service) newGetCmd(token string) *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Make a raw Ramp GET request (e.g. /developer/v1/transactions)",
		Long: "The escape hatch for everything the typed commands cannot express: Ramp's\n" +
			"own query parameters (`--param state=CLEARED`, `--param from_date=…`,\n" +
			"repeatable) and any read endpoint without a verb here. The path is\n" +
			"RELATIVE and carries its own version prefix — `/developer/v1/transactions`\n" +
			"— and an absolute URL is rejected, because the host and the credential are\n" +
			"injected rather than chosen by the caller.\n" +
			"\n" +
			"No pagination handling happens on this path: the response is emitted\n" +
			"exactly as Ramp sends it, `page.next` included, and there is no `--all`.\n" +
			"Continue by passing the next cursor yourself as `--param start=<cursor>`.",
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
		return "", &usageError{msg: "ramp get: empty path"}
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return "", &usageError{msg: "ramp get: path must be relative (e.g. /developer/v1/transactions), not an absolute URL"}
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
			return nil, &usageError{msg: "ramp get: --param must be name=value, got " + v}
		}
		q.Add(strings.TrimSpace(name), val)
	}
	return q, nil
}

// --- transaction ---

func (s *Service) newTransactionListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "List card transactions", longTransactionList, "/developer/v1/transactions")
}

func (s *Service) newTransactionGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "Get one transaction by id", longTransactionGet, "/developer/v1/transactions")
}

// --- reimbursement ---

func (s *Service) newReimbursementListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "List reimbursements", longReimbursementList, "/developer/v1/reimbursements")
}

func (s *Service) newReimbursementGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "Get one reimbursement by id", longReimbursementGet, "/developer/v1/reimbursements")
}

// --- card (virtual / physical; Ramp has no plain /cards list) ---

func (s *Service) newCardVirtualCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "virtual [id]",
		Short: "List virtual cards, or get one by id",
		Long: "With no argument this pages virtual cards; with an `[id]` it fetches that\n" +
			"one card and the pagination flags are ignored. Ramp has NO combined cards\n" +
			"endpoint, so a complete card inventory is this plus `card physical` —\n" +
			"running only one of them silently under-reports. Virtual cards are the\n" +
			"ones issued per vendor or subscription, so this is usually where a\n" +
			"recurring charge's card lives.",
		Args:        cobra.MaximumNArgs(1),
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return s.plainGet(cmd.Context(), token, "/developer/v1/cards/virtual/"+url.PathEscape(args[0]))
		}
		return s.list(cmd, token, "/developer/v1/cards/virtual", pf, nil)
	}
	return cmd
}

func (s *Service) newCardPhysicalCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "physical [id]",
		Short: "List physical cards, or get one by id",
		Long: "With no argument this pages physical cards; with an `[id]` it fetches that\n" +
			"one card and the pagination flags are ignored. It is the other half of the\n" +
			"inventory `card virtual` returns — Ramp exposes no combined list, so both\n" +
			"have to be asked. Each card names its cardholder by user id, which\n" +
			"`user get` resolves.",
		Args:        cobra.MaximumNArgs(1),
		Annotations: readOnly,
	}
	pf := registerPaginationFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return s.plainGet(cmd.Context(), token, "/developer/v1/cards/physical/"+url.PathEscape(args[0]))
		}
		return s.list(cmd, token, "/developer/v1/cards/physical", pf, nil)
	}
	return cmd
}

// --- user ---

func (s *Service) newUserListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "List users", longUserList, "/developer/v1/users")
}

func (s *Service) newUserGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "Get one user by id", longUserGet, "/developer/v1/users")
}

// --- department ---

func (s *Service) newDepartmentListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "List departments", longDepartmentList, "/developer/v1/departments")
}

func (s *Service) newDepartmentGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "Get one department by id", longDepartmentGet, "/developer/v1/departments")
}

// --- location ---

func (s *Service) newLocationListCmd(token string) *cobra.Command {
	return s.newListCmd(token, "List locations", longLocationList, "/developer/v1/locations")
}

func (s *Service) newLocationGetCmd(token string) *cobra.Command {
	return s.newGetByIDCmd(token, "Get one location by id", longLocationGet, "/developer/v1/locations")
}

// --- business ---

func (s *Service) newBusinessInfoCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Get connected-business info (name, entity, id)",
		Long: "Names the business the token belongs to, which is the only way to confirm\n" +
			"which entity a ledger read is about — no other command states it. It takes\n" +
			"no parameters and returns a single object, so it is also the cheapest\n" +
			"proof that the connection still works.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/developer/v1/business")
		},
	}
}

func (s *Service) newBusinessBalanceCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Get the connected business's balance",
		Long: "A point-in-time figure with no history behind it: there is no balance-over-\n" +
			"time endpoint here, so a trend can only be built by sampling this\n" +
			"repeatedly. It reports the business's available and total funds, not the\n" +
			"sum of `transaction list`, which counts card spend instead.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.plainGet(cmd.Context(), token, "/developer/v1/business/balance")
		},
	}
}
