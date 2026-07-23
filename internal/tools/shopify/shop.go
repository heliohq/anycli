package shopify

import "github.com/spf13/cobra"

// shopInfoQuery reads store identity/health — the lightweight connectivity
// check and the source of the store's currency/plan context.
const shopInfoQuery = `query {
  shop {
    id
    name
    myshopifyDomain
    primaryDomain { url host }
    email
    currencyCode
    ianaTimezone
    plan { displayName }
  }
}`

// newShopInfoCmd is `shop info`: GET-equivalent store identity read.
func (c *client) newShopInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "info",
		Short:       "Show store identity, currency, timezone, and plan",
		Args:        cobra.NoArgs,
		Annotations: readAnnotation(),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		data, err := c.gql(cmd.Context(), apiVersion(cmd), shopInfoQuery, nil)
		if err != nil {
			return err
		}
		return c.emit(data)
	}
	return cmd
}
