package braintree

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

const pingQuery = `query BraintreePing { ping }`

const customerNodeFields = `id legacyId firstName lastName company email createdAt`

const customerGetQuery = `query BraintreeCustomerGet($id: ID!) {
  node(id: $id) { ... on Customer { ` + customerNodeFields + ` } }
}`

const customerSearchQuery = `query BraintreeCustomerSearch($input: CustomerSearchInput!, $first: Int, $after: String) {
  search {
    customers(input: $input, first: $first, after: $after) {
      pageInfo { hasNextPage endCursor }
      edges { node { ` + customerNodeFields + ` } }
    }
  }
}`

const disputeNodeFields = `id legacyId status reason amountDisputed { value currencyCode } createdAt receivedDate replyByDate`

const disputeGetQuery = `query BraintreeDisputeGet($id: ID!) {
  node(id: $id) { ... on Dispute { ` + disputeNodeFields + ` } }
}`

const disputeSearchQuery = `query BraintreeDisputeSearch($input: DisputeSearchInput!, $first: Int, $after: String) {
  search {
    disputes(input: $input, first: $first, after: $after) {
      pageInfo { hasNextPage endCursor }
      edges { node { ` + disputeNodeFields + ` } }
    }
  }
}`

const subscriptionGetQuery = `query BraintreeSubscriptionGet($id: ID!) {
  node(id: $id) { ... on Subscription { id legacyId status } }
}`

func (s *Service) newPingCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "ping",
		Short:       "Verify the API key pair and connectivity (returns pong)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := cl.do(cmd.Context(), pingQuery, nil)
			if err != nil {
				return err
			}
			var envelope struct {
				Ping string `json:"ping"`
			}
			if uerr := json.Unmarshal(data, &envelope); uerr != nil {
				return &apiError{msg: "braintree: decode ping response", err: uerr}
			}
			return s.emit(cmd, map[string]string{"result": envelope.Ping})
		},
	}
}

func (s *Service) newCustomerCmd(cl *client) *cobra.Command {
	group := newGroupCmd("customer", "Look up and search customers")
	group.AddCommand(s.newCustomerGetCmd(cl), s.newCustomerSearchCmd(cl))
	return group
}

func (s *Service) newCustomerGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one customer by id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := cl.do(cmd.Context(), customerGetQuery, map[string]any{"id": args[0]})
			if err != nil {
				return err
			}
			node, err := decodeNode(data, "node", "braintree: no customer found for id "+args[0])
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}

func (s *Service) newCustomerSearchCmd(cl *client) *cobra.Command {
	var (
		email string
		id    string
		first int
		after string
	)
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search customers (email, id)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			input := map[string]any{}
			if email != "" {
				input["email"] = map[string]any{"is": email}
			}
			if id != "" {
				input["id"] = map[string]any{"is": id}
			}
			vars := map[string]any{"input": input, "first": first}
			if after != "" {
				vars["after"] = after
			}
			data, err := cl.do(cmd.Context(), customerSearchQuery, vars)
			if err != nil {
				return err
			}
			result, err := decodeConnection(data, "customers")
			if err != nil {
				return err
			}
			return s.emit(cmd, result)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "filter by email")
	cmd.Flags().StringVar(&id, "id", "", "filter by customer id")
	cmd.Flags().IntVar(&first, "first", 50, "maximum results to return")
	cmd.Flags().StringVar(&after, "after", "", "page cursor from a previous page_info.end_cursor")
	return cmd
}

func (s *Service) newDisputeCmd(cl *client) *cobra.Command {
	group := newGroupCmd("dispute", "Look up and search disputes")
	group.AddCommand(s.newDisputeGetCmd(cl), s.newDisputeSearchCmd(cl))
	return group
}

func (s *Service) newDisputeGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one dispute by id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := cl.do(cmd.Context(), disputeGetQuery, map[string]any{"id": args[0]})
			if err != nil {
				return err
			}
			node, err := decodeNode(data, "node", "braintree: no dispute found for id "+args[0])
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}

func (s *Service) newDisputeSearchCmd(cl *client) *cobra.Command {
	var (
		statuses       []string
		receivedAfter  string
		receivedBefore string
		first          int
		after          string
	)
	cmd := &cobra.Command{
		Use:         "search",
		Short:       "Search disputes (status, received date)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			input := map[string]any{}
			if len(statuses) > 0 {
				input["status"] = map[string]any{"in": upperAll(statuses)}
			}
			if r := rangeInput(receivedAfter, receivedBefore); r != nil {
				input["receivedDate"] = r
			}
			vars := map[string]any{"input": input, "first": first}
			if after != "" {
				vars["after"] = after
			}
			data, err := cl.do(cmd.Context(), disputeSearchQuery, vars)
			if err != nil {
				return err
			}
			result, err := decodeConnection(data, "disputes")
			if err != nil {
				return err
			}
			return s.emit(cmd, result)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&statuses, "status", nil, "filter by dispute status (repeatable), e.g. OPEN, WON, LOST")
	f.StringVar(&receivedAfter, "received-after", "", "received on or after (YYYY-MM-DD)")
	f.StringVar(&receivedBefore, "received-before", "", "received on or before (YYYY-MM-DD)")
	f.IntVar(&first, "first", 50, "maximum results to return")
	f.StringVar(&after, "after", "", "page cursor from a previous page_info.end_cursor")
	return cmd
}

func (s *Service) newSubscriptionCmd(cl *client) *cobra.Command {
	group := newGroupCmd("subscription", "Look up subscriptions")
	group.AddCommand(s.newSubscriptionGetCmd(cl))
	return group
}

func (s *Service) newSubscriptionGetCmd(cl *client) *cobra.Command {
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one subscription by id",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := cl.do(cmd.Context(), subscriptionGetQuery, map[string]any{"id": args[0]})
			if err != nil {
				return err
			}
			node, err := decodeNode(data, "node", "braintree: no subscription found for id "+args[0])
			if err != nil {
				return err
			}
			return s.emit(cmd, node)
		},
	}
}
