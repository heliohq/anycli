package braintree

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newQueryCmd is the raw-GraphQL escape hatch — READ-ONLY in v1. The curated
// money-movement verbs (refund/void/reverse) exist so the design-318 approval
// gate can reason over structured action facts before funds move; an
// unrestricted passthrough would re-admit refundTransaction / reverseTransaction
// under a benign-looking `query` command, bypassing that gate. So this verb
// parses the supplied operation and REJECTS any mutation locally (exit 2, no
// network call); reads pass through unchanged.
func (s *Service) newQueryCmd(cl *client) *cobra.Command {
	var vars []string
	cmd := &cobra.Command{
		Use:   "query <graphql>",
		Short: "Run a raw READ-ONLY GraphQL query (mutations are rejected)",
		Args:  cobra.ExactArgs(1),
		// Read-only: mutations are rejected before any request, so no verb
		// reachable through this command can move money.
		Annotations: map[string]string{"anycli.side_effect": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			document := args[0]
			if isMutation(document) {
				return &usageError{msg: "braintree query is read-only: GraphQL mutations are rejected — use `transaction refund|void|reverse` for money movement"}
			}
			variables, err := parseVars(vars)
			if err != nil {
				return err
			}
			data, derr := cl.do(cmd.Context(), document, variables)
			if derr != nil {
				return derr
			}
			var value any
			if uerr := json.Unmarshal(data, &value); uerr != nil {
				return &apiError{msg: fmt.Sprintf("braintree: decode query response: %v", uerr), err: uerr}
			}
			return s.emit(cmd, value)
		},
	}
	cmd.Flags().StringArrayVar(&vars, "var", nil, "GraphQL variable as key=value (repeatable)")
	return cmd
}

// isMutation reports whether the GraphQL document's first operation is a
// mutation. It skips leading whitespace, commas, and #-to-end-of-line comments,
// then reads the first identifier: an anonymous selection ("{ … }") and the
// `query`/`subscription` keywords are reads; only `mutation` is rejected.
func isMutation(document string) bool {
	i := 0
	for i < len(document) {
		c := document[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
		case c == '#':
			for i < len(document) && document[i] != '\n' {
				i++
			}
		default:
			// First meaningful token.
			if c == '{' {
				return false // anonymous operation is always a query
			}
			start := i
			for i < len(document) && isIdentChar(document[i]) {
				i++
			}
			return document[start:i] == "mutation"
		}
	}
	return false
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// parseVars turns repeated key=value flags into the GraphQL variables map.
// Values are strings; a document needing typed variables can inline literals.
func parseVars(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(pairs))
	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		if !ok || key == "" {
			return nil, &usageError{msg: fmt.Sprintf("invalid --var %q (expected key=value)", p)}
		}
		out[key] = value
	}
	return out, nil
}
