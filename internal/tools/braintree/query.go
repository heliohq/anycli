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
// parses the supplied document and REJECTS it locally if ANY top-level
// operation is a mutation (exit 2, no network call) — including documents that
// lead with fragment definitions or comments before the mutation; reads pass
// through unchanged.
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

// isMutation reports whether the GraphQL document contains ANY top-level
// mutation operation — not just when the first token is `mutation`.
//
// This is a money-movement safety boundary: the curated refund/void/reverse
// verbs route through the design-318 approval gate, and this read-only
// passthrough must never become an un-gated write channel. A single valid
// document may legally lead with fragment definitions (or comments) before the
// operation — e.g. `fragment F on Transaction { id } mutation M {
// refundTransaction(input:{transactionId:"x"}){ refund { ...F } } }` — so
// inspecting only the first identifier is exploitable: it reads `fragment`,
// classifies the document as a read, and forwards the mutation to Braintree.
//
// So this scans the whole document and rejects it if a `mutation` keyword
// appears in operation-definition position — i.e. at top level (brace depth 0),
// outside string literals and #-comments. Field and argument identifiers live
// inside a selection set (depth > 0) and never trip the guard; string literals
// (regular and block) are skipped so their braces cannot corrupt the depth
// count. The classifier errs toward rejection (an operation merely *named*
// "mutation" is refused), which is the safe direction for a payments tool.
func isMutation(document string) bool {
	i := 0
	depth := 0
	for i < len(document) {
		c := document[i]
		switch {
		case c == '#':
			for i < len(document) && document[i] != '\n' {
				i++
			}
		case c == '"':
			i = skipGraphQLString(document, i)
		case c == '{':
			depth++
			i++
		case c == '}':
			if depth > 0 {
				depth--
			}
			i++
		case isIdentStart(c):
			start := i
			for i < len(document) && isIdentChar(document[i]) {
				i++
			}
			// Only top-level definition keywords start operations; a `mutation`
			// token inside a selection set is a field/argument name, not a
			// write operation.
			if depth == 0 && document[start:i] == "mutation" {
				return true
			}
		default:
			i++
		}
	}
	return false
}

// skipGraphQLString advances past a string literal starting at index i (the
// opening quote) and returns the index just after the closing delimiter. It
// handles both regular ("…") and block ("""…""") strings, honoring escapes, so
// braces or the word "mutation" inside a string cannot corrupt isMutation's
// top-level scan.
func skipGraphQLString(s string, i int) int {
	if strings.HasPrefix(s[i:], `"""`) {
		j := i + 3
		for j < len(s) {
			if s[j] == '\\' && strings.HasPrefix(s[j+1:], `"""`) {
				j += 4
				continue
			}
			if strings.HasPrefix(s[j:], `"""`) {
				return j + 3
			}
			j++
		}
		return len(s)
	}
	j := i + 1
	for j < len(s) {
		switch s[j] {
		case '\\':
			j += 2
		case '"':
			return j + 1
		case '\n':
			return j // unterminated at line end; stop scanning the string
		default:
			j++
		}
	}
	return len(s)
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
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
