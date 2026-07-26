package shopify

import "github.com/spf13/cobra"

const productListQuery = `query($first: Int!, $after: String, $query: String) {
  products(first: $first, after: $after, query: $query) {
    edges { node { id title handle status totalInventory vendor productType updatedAt } }
    pageInfo { hasNextPage endCursor }
  }
}`

const productGetQuery = `query($id: ID!) {
  product(id: $id) {
    id title handle status descriptionHtml vendor productType tags totalInventory
    variants(first: 50) { edges { node { id title sku price inventoryQuantity } } }
  }
}`

const productCreateMutation = `mutation($input: ProductInput!) {
  productCreate(input: $input) {
    product { id title handle status }
    userErrors { field message }
  }
}`

const productUpdateMutation = `mutation($input: ProductInput!) {
  productUpdate(input: $input) {
    product { id title handle status }
    userErrors { field message }
  }
}`

// newProductListCmd is `product list`: paginated product query.
func (c *client) newProductListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List products (cursor-paginated)",
		Long: "Each row carries `id`, `title`, `handle`, `status`, `totalInventory`,\n" +
			"`vendor`, `productType` and `updatedAt`; variants, description and tags only\n" +
			"come from `product get`. `--query` takes Shopify product search syntax\n" +
			"(`status:active`, `vendor:Acme`, `created_at:>2026-01-01`). `--limit`\n" +
			"defaults to 20 and is clamped to 250 silently, so a larger value returns 250\n" +
			"rather than failing.",
		Args:        cobra.NoArgs,
		Annotations: readAnnotation(),
	}
	lf := registerListFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data, err := c.gql(cmd.Context(), apiVersion(cmd), productListQuery, lf.vars())
		if err != nil {
			return err
		}
		return c.emit(connectionOut(data, "products", "products"))
	}
	return cmd
}

// newProductGetCmd is `product get <id>`: a single product with its variants.
func (c *client) newProductGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get one product by numeric id or gid",
		Long: "Accepts a bare numeric id or a full `gid://shopify/Product/<n>`. Adds\n" +
			"`descriptionHtml`, `tags` and the first 50 variants — each with `id`, `sku`,\n" +
			"`price` and `inventoryQuantity` — to what `product list` returns. A product\n" +
			"with more than 50 variants is TRUNCATED with no marker in the output; reach\n" +
			"the rest through `graphql`.",
		Args:        cobra.ExactArgs(1),
		Annotations: readAnnotation(),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		vars := map[string]any{"id": gidOrRaw("Product", args[0])}
		data, err := c.gql(cmd.Context(), apiVersion(cmd), productGetQuery, vars)
		if err != nil {
			return err
		}
		return c.emit(data["product"])
	}
	return cmd
}

// newProductCreateCmd is `product create`: a minimal product create from
// scalar flags. Anything richer goes through the raw graphql passthrough.
func (c *client) newProductCreateCmd() *cobra.Command {
	var title, status, vendor, productType, descriptionHTML string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a product",
		Long: "`--title` is the only required flag. `--status` is checked locally against\n" +
			"ACTIVE, DRAFT and ARCHIVED; omitting it takes whatever default Shopify\n" +
			"applies, so pass DRAFT explicitly when the product must not go live. Only\n" +
			"the scalar fields offered here are settable — variants, prices, images,\n" +
			"options and inventory all need the `graphql` passthrough — and the response\n" +
			"is just `id`, `title`, `handle` and `status`.",
		Args:        cobra.NoArgs,
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&title, "title", "", "product title (required)")
	cmd.Flags().StringVar(&status, "status", "", "ACTIVE|DRAFT|ARCHIVED")
	cmd.Flags().StringVar(&vendor, "vendor", "", "vendor")
	cmd.Flags().StringVar(&productType, "type", "", "product type")
	cmd.Flags().StringVar(&descriptionHTML, "description-html", "", "product description (HTML)")
	_ = cmd.MarkFlagRequired("title")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		input := map[string]any{"title": title}
		if err := validateProductStatus(status); err != nil {
			return err
		}
		putIfSet(input, "status", status)
		putIfSet(input, "vendor", vendor)
		putIfSet(input, "productType", productType)
		putIfSet(input, "descriptionHtml", descriptionHTML)
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), productCreateMutation, "productCreate", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["product"])
	}
	return cmd
}

// newProductUpdateCmd is `product update <id>`: mutate a product's status or
// core scalar fields (e.g. flip a product ACTIVE, retitle).
func (c *client) newProductUpdateCmd() *cobra.Command {
	var title, status, vendor, productType, descriptionHTML string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a product's title/status/vendor",
		Long: "At least one field flag is required; with none it fails as a usage error\n" +
			"rather than issuing a no-op write. Only the flags actually passed are sent,\n" +
			"so every other field keeps its value. `--status` is validated locally\n" +
			"against ACTIVE|DRAFT|ARCHIVED, and flipping it is what publishes or\n" +
			"unpublishes the product — ACTIVE reaches the storefront immediately.",
		Args:        cobra.ExactArgs(1),
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&status, "status", "", "ACTIVE|DRAFT|ARCHIVED")
	cmd.Flags().StringVar(&vendor, "vendor", "", "new vendor")
	cmd.Flags().StringVar(&productType, "type", "", "new product type")
	cmd.Flags().StringVar(&descriptionHTML, "description-html", "", "new description (HTML)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateProductStatus(status); err != nil {
			return err
		}
		input := map[string]any{"id": gidOrRaw("Product", args[0])}
		putIfSet(input, "title", title)
		putIfSet(input, "status", status)
		putIfSet(input, "vendor", vendor)
		putIfSet(input, "productType", productType)
		putIfSet(input, "descriptionHtml", descriptionHTML)
		if len(input) == 1 {
			return &usageError{msg: "product update requires at least one field flag (--title/--status/--vendor/--type/--description-html)"}
		}
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), productUpdateMutation, "productUpdate", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["product"])
	}
	return cmd
}

// validateProductStatus enforces the ProductStatus enum on parse (empty is
// allowed — the field is simply omitted).
func validateProductStatus(status string) error {
	switch status {
	case "", "ACTIVE", "DRAFT", "ARCHIVED":
		return nil
	default:
		return &usageError{msg: "--status must be one of ACTIVE|DRAFT|ARCHIVED, got " + status}
	}
}

// putIfSet writes a non-empty value into a GraphQL input map.
func putIfSet(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
