package shopify

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

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

// inventoryAdjustMutation carries the @idempotent directive: as of Admin API
// version 2026-04 the idempotency key is MANDATORY on inventoryAdjustQuantities
// (optional since 2026-01), and a request that omits it is rejected. Since this
// tool pins 2026-07 (>= 2026-04), the key is always required. The key is a
// per-invocation UUID passed as the $idempotencyKey variable (Shopify's own
// docs use a variable in the directive); reuse the same key across retries to
// have Shopify collapse duplicate adjustments to a single effect.
const inventoryAdjustMutation = `mutation($input: InventoryAdjustQuantitiesInput!, $idempotencyKey: String!) {
  inventoryAdjustQuantities(input: $input) @idempotent(key: $idempotencyKey) {
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
	var item, location, reason, name, idempotencyKey string
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
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for safe retries (default: a fresh UUID per call)")
	_ = cmd.MarkFlagRequired("item")
	_ = cmd.MarkFlagRequired("location")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("delta") || delta == 0 {
			return &usageError{msg: "--delta is required and must be non-zero"}
		}
		key := idempotencyKey
		if key == "" {
			key = newIdempotencyKey()
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
		vars := map[string]any{"input": input, "idempotencyKey": key}
		payload, err := c.mutationResult(cmd.Context(), apiVersion(cmd), inventoryAdjustMutation, "inventoryAdjustQuantities", vars)
		if err != nil {
			return err
		}
		return c.emit(payload["inventoryAdjustmentGroup"])
	}
	return cmd
}

// newIdempotencyKey returns a canonical RFC 4122 v4 UUID string for the
// @idempotent directive. A fresh key per invocation is the correct default:
// each CLI call is one deliberate adjustment. crypto/rand failure falls back to
// a time-seeded value so a call never reuses the zero UUID.
func newIdempotencyKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("helio-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
