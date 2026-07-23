package shopify

import "github.com/spf13/cobra"

const orderListQuery = `query($first: Int!, $after: String, $query: String) {
  orders(first: $first, after: $after, query: $query, sortKey: CREATED_AT, reverse: true) {
    edges { node {
      id name createdAt displayFinancialStatus displayFulfillmentStatus
      totalPriceSet { shopMoney { amount currencyCode } }
      customer { id displayName email }
    } }
    pageInfo { hasNextPage endCursor }
  }
}`

const orderGetQuery = `query($id: ID!) {
  order(id: $id) {
    id name note tags createdAt displayFinancialStatus displayFulfillmentStatus
    totalPriceSet { shopMoney { amount currencyCode } }
    customer { id displayName email }
    lineItems(first: 50) { edges { node { title quantity sku } } }
  }
}`

const orderUpdateMutation = `mutation($input: OrderInput!) {
  orderUpdate(input: $input) {
    order { id name note tags }
    userErrors { field message }
  }
}`

// newOrderListCmd is `order list`: recent orders first, cursor-paginated.
func (c *client) newOrderListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List orders, newest first (cursor-paginated)",
		Args:        cobra.NoArgs,
		Annotations: readAnnotation(),
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data, err := c.gql(cmd.Context(), apiVersion(cmd), orderListQuery, lf.vars())
		if err != nil {
			return err
		}
		return c.emit(connectionOut(data, "orders", "orders"))
	}
	return cmd
}

// newOrderGetCmd is `order get <id>`: one order with line items.
func (c *client) newOrderGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one order by numeric id or gid",
		Args:        cobra.ExactArgs(1),
		Annotations: readAnnotation(),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		vars := map[string]any{"id": gidOrRaw("Order", args[0])}
		data, err := c.gql(cmd.Context(), apiVersion(cmd), orderGetQuery, vars)
		if err != nil {
			return err
		}
		return c.emit(data["order"])
	}
	return cmd
}

// newOrderUpdateCmd is `order update <id>`: set an order's note or tags.
func (c *client) newOrderUpdateCmd() *cobra.Command {
	var note string
	var tags []string
	var noteSet bool
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update an order's note or tags",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&note, "note", "", "order note")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "replace order tags (repeatable)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		noteSet = cmd.Flags().Changed("note")
		if !noteSet && !cmd.Flags().Changed("tag") {
			return &usageError{msg: "order update requires --note and/or --tag"}
		}
		input := map[string]any{"id": gidOrRaw("Order", args[0])}
		if noteSet {
			input["note"] = note
		}
		if cmd.Flags().Changed("tag") {
			input["tags"] = tags
		}
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), orderUpdateMutation, "orderUpdate", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["order"])
	}
	return cmd
}
