package shopify

import "github.com/spf13/cobra"

const customerListQuery = `query($first: Int!, $after: String, $query: String) {
  customers(first: $first, after: $after, query: $query) {
    edges { node { id displayName email phone numberOfOrders createdAt } }
    pageInfo { hasNextPage endCursor }
  }
}`

const customerGetQuery = `query($id: ID!) {
  customer(id: $id) {
    id displayName firstName lastName email phone note tags numberOfOrders
    amountSpent { amount currencyCode }
  }
}`

const customerCreateMutation = `mutation($input: CustomerInput!) {
  customerCreate(input: $input) {
    customer { id displayName email }
    userErrors { field message }
  }
}`

const customerUpdateMutation = `mutation($input: CustomerInput!) {
  customerUpdate(input: $input) {
    customer { id displayName email }
    userErrors { field message }
  }
}`

// newCustomerListCmd is `customer list`: paginated customer query.
func (c *client) newCustomerListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List customers (cursor-paginated)",
		Args:        cobra.NoArgs,
		Annotations: readAnnotation(),
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data, err := c.gql(cmd.Context(), apiVersion(cmd), customerListQuery, lf.vars())
		if err != nil {
			return err
		}
		return c.emit(connectionOut(data, "customers", "customers"))
	}
	return cmd
}

// newCustomerGetCmd is `customer get <id>`.
func (c *client) newCustomerGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "get <id>",
		Short:       "Get one customer by numeric id or gid",
		Args:        cobra.ExactArgs(1),
		Annotations: readAnnotation(),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		vars := map[string]any{"id": gidOrRaw("Customer", args[0])}
		data, err := c.gql(cmd.Context(), apiVersion(cmd), customerGetQuery, vars)
		if err != nil {
			return err
		}
		return c.emit(data["customer"])
	}
	return cmd
}

// newCustomerCreateCmd is `customer create`.
func (c *client) newCustomerCreateCmd() *cobra.Command {
	var email, firstName, lastName, phone, note string
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a customer",
		Args:        cobra.NoArgs,
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&email, "email", "", "email (required)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "last name")
	cmd.Flags().StringVar(&phone, "phone", "", "phone (E.164)")
	cmd.Flags().StringVar(&note, "note", "", "note")
	_ = cmd.MarkFlagRequired("email")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		input := map[string]any{"email": email}
		putIfSet(input, "firstName", firstName)
		putIfSet(input, "lastName", lastName)
		putIfSet(input, "phone", phone)
		putIfSet(input, "note", note)
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), customerCreateMutation, "customerCreate", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["customer"])
	}
	return cmd
}

// newCustomerUpdateCmd is `customer update <id>`.
func (c *client) newCustomerUpdateCmd() *cobra.Command {
	var email, firstName, lastName, phone, note string
	cmd := &cobra.Command{
		Use:         "update <id>",
		Short:       "Update a customer's contact fields or note",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&email, "email", "", "new email")
	cmd.Flags().StringVar(&firstName, "first-name", "", "new first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "new last name")
	cmd.Flags().StringVar(&phone, "phone", "", "new phone (E.164)")
	cmd.Flags().StringVar(&note, "note", "", "new note")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		input := map[string]any{"id": gidOrRaw("Customer", args[0])}
		putIfSet(input, "email", email)
		putIfSet(input, "firstName", firstName)
		putIfSet(input, "lastName", lastName)
		putIfSet(input, "phone", phone)
		putIfSet(input, "note", note)
		if len(input) == 1 {
			return &usageError{msg: "customer update requires at least one field flag (--email/--first-name/--last-name/--phone/--note)"}
		}
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), customerUpdateMutation, "customerUpdate", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["customer"])
	}
	return cmd
}
