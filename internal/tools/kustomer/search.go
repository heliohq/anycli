package kustomer

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newSearchCustomersCmd: POST /customers/search with a raw JSON query body
// (free-form "find customers where…").
func (s *Service) newSearchCustomersCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "customers",
		Short: "Search customers with a JSON query body",
		Long: "The fallback when an exact identifier fails: the body is Kustomer's own\n" +
			"search-query document — an `and`/`or` tree of field/operator/value clauses\n" +
			"— passed through unmodified, so it must match that schema rather than\n" +
			"being a plain search string. When an email address is already known,\n" +
			"`customer get-by-email` is one cheap call and needs no query body.\n" +
			"Customers are the only searchable resource here; conversations, messages\n" +
			"and notes have no search command.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
	}
	data, file := registerBodyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		payload, err := readBody(*data, *file)
		if err != nil {
			return err
		}
		body, err := s.call(cmd.Context(), base, token, http.MethodPost, "/customers/search", payload)
		if err != nil {
			return err
		}
		return s.emitJSON(body)
	}
	return cmd
}
