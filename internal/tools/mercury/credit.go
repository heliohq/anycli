package mercury

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newCreditListCmd lists IO credit card accounts (GET /credit). Mercury
// filters credit accounts out of GET /accounts server-side, so this is the
// only way to discover credit account ids; those ids then work with
// `transaction list --account <id>`.
func (s *Service) newCreditListCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List IO credit card accounts (GET /credit; absent from account list)",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"anycli.side_effect": "false"}, // GET
		RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/credit", nil)
			if err != nil {
				return err
			}
			return s.emitList(body, "accounts")
		},
	}
}
