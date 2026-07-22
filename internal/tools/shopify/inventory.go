package shopify

import "github.com/spf13/cobra"

const inventoryLevelsQuery = `query($id: ID!, $first: Int!) {
  inventoryItem(id: $id) {
    id sku tracked
    inventoryLevels(first: $first) {
      edges { node {
        id
        location { id name }
        quantities(names: ["available", "on_hand", "committed"]) { name quantity }
      } }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const inventoryAdjustMutation = `mutation($input: InventoryAdjustQuantitiesInput!) {
  inventoryAdjustQuantities(input: $input) {
    inventoryAdjustmentGroup { reason changes { name delta quantityAfterChange } }
    userErrors { field message }
  }
}`

// newInventoryLevelsCmd is `inventory levels <inventory-item-id>`: the stock
// levels of one inventory item across its locations.
func (c *client) newInventoryLevelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "levels <inventory-item-id>",
		Short:       "Show an inventory item's stock levels by location",
		Args:        cobra.ExactArgs(1),
		Annotations: readAnnotation(),
	}
	var limit int
	cmd.Flags().IntVar(&limit, "limit", 20, "max locations to return")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if limit <= 0 {
			limit = 20
		}
		vars := map[string]any{"id": gidOrRaw("InventoryItem", args[0]), "first": limit}
		data, err := c.gql(cmd.Context(), apiVersion(cmd), inventoryLevelsQuery, vars)
		if err != nil {
			return err
		}
		item, _ := data["inventoryItem"].(map[string]any)
		if item == nil {
			return c.emit(map[string]any{"inventory_item": nil})
		}
		out := connectionOut(item, "inventoryLevels", "levels")
		out["inventory_item_id"] = item["id"]
		out["sku"] = item["sku"]
		return c.emit(out)
	}
	return cmd
}

// newInventoryAdjustCmd is `inventory adjust`: apply an available-quantity
// delta to one inventory item at one location.
func (c *client) newInventoryAdjustCmd() *cobra.Command {
	var item, location, reason, name string
	var delta int
	cmd := &cobra.Command{
		Use:         "adjust",
		Short:       "Adjust an inventory item's available quantity at a location",
		Args:        cobra.NoArgs,
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&item, "item", "", "inventory item id or gid (required)")
	cmd.Flags().StringVar(&location, "location", "", "location id or gid (required)")
	cmd.Flags().IntVar(&delta, "delta", 0, "quantity delta, positive or negative (required)")
	cmd.Flags().StringVar(&reason, "reason", "correction", "adjustment reason")
	cmd.Flags().StringVar(&name, "name", "available", "quantity name to adjust")
	_ = cmd.MarkFlagRequired("item")
	_ = cmd.MarkFlagRequired("location")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("delta") || delta == 0 {
			return &usageError{msg: "--delta is required and must be non-zero"}
		}
		input := map[string]any{
			"reason": reason,
			"name":   name,
			"changes": []map[string]any{{
				"delta":           delta,
				"inventoryItemId": gidOrRaw("InventoryItem", item),
				"locationId":      gidOrRaw("Location", location),
			}},
		}
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), inventoryAdjustMutation, "inventoryAdjustQuantities", map[string]any{"input": input})
		if err != nil {
			return err
		}
		return c.emit(payload["inventoryAdjustmentGroup"])
	}
	return cmd
}
