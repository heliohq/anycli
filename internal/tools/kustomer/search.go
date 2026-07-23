package kustomer

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newSearchCustomersCmd: POST /customers/search with a raw JSON query body
// (free-form "find customers where…").
func (s *Service) newSearchCustomersCmd(base, token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "customers",
		Short:       "Search customers with a JSON query body",
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
