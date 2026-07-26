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
		Use:   "list",
		Short: "List customers (cursor-paginated)",
		Long: "Each row carries `id`, `displayName`, `email`, `phone`, `numberOfOrders`\n" +
			"and `createdAt`. `--query` takes Shopify customer search syntax\n" +
			"(`email:jane@acme.com`, `country:CA`, `orders_count:>5`). The contact fields\n" +
			"are the ones Protected Customer Data approval gates, so on an unapproved app\n" +
			"they read null while the call still succeeds.",
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
		Use:   "get <id>",
		Short: "Get one customer by numeric id or gid",
		Long: "Accepts a bare numeric id or `gid://shopify/Customer/<n>`. Adds\n" +
			"`firstName`/`lastName`, `note`, `tags` and lifetime `amountSpent` to the\n" +
			"list fields. It returns no addresses and no order history — reach those\n" +
			"through `order list --query` or `graphql`.",
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
		Use:   "create",
		Short: "Create a customer",
		Long: "`--email` is required and must be unique in the store: a duplicate comes\n" +
			"back as a `userErrors` entry and exits non-zero. `--phone` has to be E.164\n" +
			"(+15551234567) or the write is rejected the same way. Addresses, tags and\n" +
			"marketing consent are not settable here. No account-invite email is sent —\n" +
			"that is a separate mutation, reachable only through `graphql`.",
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
		Use:   "update <id>",
		Short: "Update a customer's contact fields or note",
		Long: "At least one field flag is required. Only the flags passed are sent, so\n" +
			"omitted fields keep their values, and `--note` OVERWRITES the existing note\n" +
			"rather than appending to it. Changing `--email` runs the store's uniqueness\n" +
			"check again and fails with a `userErrors` entry if another customer already\n" +
			"holds that address.",
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
