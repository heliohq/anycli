package braintree

import (
	"strings"

	"github.com/spf13/cobra"
)

// GraphQL documents. One pinned document per verb: a schema-field rename or a
// Braintree-Version regression surfaces as an L1 diff here (and an L2 errors[]
// unknown-field entry against the live sandbox). The node field selections are
// deliberately modest — the operational facts a teammate reads.

const transactionNodeFields = `id legacyId status amount { value currencyCode } createdAt orderId`

const transactionSearchQuery = `query BraintreeTransactionSearch($input: TransactionSearchInput!, $first: Int, $after: String) {
  search {
    transactions(input: $input, first: $first, after: $after) {
      pageInfo { hasNextPage endCursor }
      edges { node { ` + transactionNodeFields + ` } }
    }
  }
}`

const transactionGetQuery = `query BraintreeTransactionGet($id: ID!) {
  node(id: $id) { ... on Transaction { ` + transactionNodeFields + ` } }
}`

// refundTransaction issues a refund on a settled transaction.
const refundTransactionMutation = `mutation BraintreeRefundTransaction($input: RefundTransactionInput!) {
  refundTransaction(input: $input) {
    refund { ` + transactionNodeFields + ` }
  }
}`

// voidTransaction cancels an UNSETTLED transaction. It errors once the
// transaction has settled (Braintree: "Transaction cannot be voided in its
// current state") and never moves money that already left.
const voidTransactionMutation = `mutation BraintreeVoidTransaction($input: VoidTransactionInput!) {
  voidTransaction(input: $input) {
    transaction { ` + transactionNodeFields + ` }
  }
}`

// reverseTransaction is the universal reversal: it VOIDS an unsettled
// transaction but issues a FULL REFUND on an already-settled one. That is why
// it is its own explicit verb and never aliased to `void`.
const reverseTransactionMutation = `mutation BraintreeReverseTransaction($input: ReverseTransactionInput!) {
  reverseTransaction(input: $input) {
    reversal {
      __typename
      ... on Transaction { ` + transactionNodeFields + ` }
      ... on Refund { ` + transactionNodeFields + ` }
    }
  }
}`

func (s *Service) newTransactionCmd(cl *client) *cobra.Command {
	group := newGroupCmd("transaction", "Search, inspect, and reverse transactions")
	group.AddCommand(
		s.newTransactionSearchCmd(cl),
		s.newTransactionGetCmd(cl),
		s.newTransactionRefundCmd(cl),
		s.newTransactionVoidCmd(cl),
		s.newTransactionReverseCmd(cl),
	)
	return group
}

func (s *Service) newTransactionSearchCmd(cl *client) *cobra.Command {
	var (
		statuses      []string
		amountMin     string
		amountMax     string
		createdAfter  string
		createdBefore string
		customerID    string
		orderID       string
		first         int
		after         string
	)
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search transactions (status, amount, date, customer, order)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			input := map[string]any{}
			if len(statuses) > 0 {
				input["status"] = map[string]any{"in": upperAll(statuses)}
			}
			if r := rangeInput(amountMin, amountMax); r != nil {
				input["amount"] = r
			}
			if r := rangeInput(createdAfter, createdBefore); r != nil {
				input["createdAt"] = r
			}
			if customerID != "" {
				input["customerId"] = map[string]any{"is": customerID}
			}
			if orderID != "" {
				input["orderId"] = map[string]any{"is": orderID}
			}
			vars := map[string]any{"input": input, "first": first}
			if after != "" {
				vars["after"] = after
			}
			data, err := cl.do(cmd.Context(), transactionSearchQuery, vars)
			if err != nil {
				return err
			}
			result, err := decodeConnection(data, "transactions")
			if err != nil {
				return err
			}
			return s.emit(cmd, result)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&statuses, "status", nil, "filter by transaction status (repeatable), e.g. SETTLED, AUTHORIZED, SETTLING")
	f.StringVar(&amountMin, "amount-min", "", "minimum amount (inclusive)")
	f.StringVar(&amountMax, "amount-max", "", "maximum amount (inclusive)")
	f.StringVar(&createdAfter, "created-after", "", "created at or after (ISO 8601)")
	f.StringVar(&createdBefore, "created-before", "", "created at or before (ISO 8601)")
	f.StringVar(&customerID, "customer-id", "", "filter by customer id")
	f.StringVar(&orderID, "order-id", "", "filter by order id")
	f.IntVar(&first, "first", 50, "maximum results to return")
	f.StringVar(&after, "after", "", "page cursor from a previous page_info.end_cursor")
	return cmd
}

func (s *Service) newTransactionGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one transaction by id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := cl.do(cmd.Context(), transactionGetQuery, map[string]any{"id": args[0]})
			if err != nil {
				return err
			}
			node, err := decodeNode(data, "node", "braintree: no transaction found for id "+args[0])
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}

func (s *Service) newTransactionRefundCmd(cl *client) *cobra.Command {
	var (
		amount  string
		orderID string
	)
	cmd := &cobra.Command{
		Use:   "refund <id>",
		Short: "Refund a settled transaction (full, or partial with --amount)",
		Args:  cobra.ExactArgs(1),
		// Money movement: refunds funds to the customer.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			input := map[string]any{"transactionId": args[0]}
			refund := map[string]any{}
			if amount != "" {
				refund["amount"] = amount
			}
			if orderID != "" {
				refund["orderId"] = orderID
			}
			if len(refund) > 0 {
				input["refund"] = refund
			}
			data, err := cl.do(cmd.Context(), refundTransactionMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			node, err := decodeNestedNode(data, "refundTransaction", "refund")
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
	cmd.Flags().StringVar(&amount, "amount", "", "partial refund amount (omit for a full refund)")
	cmd.Flags().StringVar(&orderID, "order-id", "", "custom order id for the refund")
	return cmd
}

func (s *Service) newTransactionVoidCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:   "void <id>",
		Short: "Void an UNSETTLED transaction (errors once settled — never refunds)",
		Args:  cobra.ExactArgs(1),
		// Money movement: cancels an unsettled authorization.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			input := map[string]any{"transactionId": args[0]}
			data, err := cl.do(cmd.Context(), voidTransactionMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			node, err := decodeNestedNode(data, "voidTransaction", "transaction")
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}

func (s *Service) newTransactionReverseCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:   "reverse <id>",
		Short: "Reverse a transaction: void if unsettled, FULL REFUND if settled",
		Args:  cobra.ExactArgs(1),
		// Money movement: issues a full refund on an already-settled transaction.
		Annotations: map[string]string{"anycli.side_effect": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			input := map[string]any{"transactionId": args[0]}
			data, err := cl.do(cmd.Context(), reverseTransactionMutation, map[string]any{"input": input})
			if err != nil {
				return err
			}
			node, err := decodeNestedNode(data, "reverseTransaction", "reversal")
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}

// rangeInput builds a Braintree range matcher ({greaterThanOrEqualTo,
// lessThanOrEqualTo}) from an optional lower/upper bound, or nil if both empty.
func rangeInput(min, max string) map[string]any {
	r := map[string]any{}
	if min != "" {
		r["greaterThanOrEqualTo"] = min
	}
	if max != "" {
		r["lessThanOrEqualTo"] = max
	}
	if len(r) == 0 {
		return nil
	}
	return r
}

// upperAll upper-cases each value; Braintree enum values (transaction statuses,
// dispute statuses) are upper-case, so callers may pass any case.
func upperAll(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToUpper(strings.TrimSpace(v))
	}
	return out
}
