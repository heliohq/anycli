package lusha

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newAccountCmd(key string) *cobra.Command {
	cmd := newGroupCmd("account", "Account (credit usage, plan, rate limits, pricing)")
	cmd.AddCommand(s.newAccountUsageCmd(key))
	return cmd
}

// newAccountUsageCmd reads credits used/remaining/total, plan, rate limits, and
// per-action pricing via GET /account/usage. It is the credit-free pre-flight
// check before a credit-heavy sweep (and the provider-side verify endpoint).
// The response object is passed through under "data".
func (s *Service) newAccountUsageCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "usage",
		Short:       "Get account credit usage, plan, and pricing (GET /account/usage)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), key, http.MethodGet, "/account/usage", nil)
			if err != nil {
				return err
			}
			var data json.RawMessage
			if err := json.Unmarshal(resp, &data); err != nil {
				return &apiError{msg: fmt.Sprintf("lusha: decode usage response: %v", err), err: err}
			}
			return s.emit(map[string]any{"data": data})
		},
	}
}
