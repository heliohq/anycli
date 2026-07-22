package shopify

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// newGraphQLCmd is the top-level `graphql` raw passthrough: run an arbitrary
// GraphQL query or mutation against the Admin API. This is the escape hatch for
// anything the modeled subcommands do not cover, keeping the modeled surface
// small while leaving the full Admin schema reachable. Marked side-effecting
// because a passthrough can carry a mutation.
func (c *client) newGraphQLCmd() *cobra.Command {
	var query, variables string
	cmd := &cobra.Command{
		Use:         "graphql",
		Short:       "Run a raw GraphQL query or mutation (escape hatch)",
		Args:        cobra.NoArgs,
		Annotations: writeAnnotation(),
	}
	cmd.Flags().StringVar(&query, "query", "", "GraphQL query or mutation document (required)")
	cmd.Flags().StringVar(&variables, "variables", "", "GraphQL variables as a JSON object")
	_ = cmd.MarkFlagRequired("query")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		var vars map[string]any
		if variables != "" {
			if err := json.Unmarshal([]byte(variables), &vars); err != nil {
				return &usageError{msg: "--variables is not valid JSON: " + err.Error()}
			}
		}
		// Raw passthrough surfaces the full data object verbatim; the caller
		// owns userErrors interpretation for its own document.
		data, err := c.gql(cmd.Context(), apiVersion(cmd), query, vars)
		if err != nil {
			return err
		}
		return c.emit(data)
	}
	return cmd
}
