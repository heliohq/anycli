package servicenow

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// whoami verifies the API key against the instance and echoes the integration
// user's identity. It reads sys_user filtered to the current session's user via
// the JavaScript getUserID() encoded query — the minimal probe that both proves
// the credential works and reveals which user the key acts as.
func (s *Service) newWhoamiCmd(c *client) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Verify the API key and echo the integration user's identity",
		Long: "The cheapest way to prove that the instance URL and the API key are both\n" +
			"correct, since neither is verified when the connection is made — a wrong\n" +
			"host or a revoked key surfaces here rather than at connect time. It also\n" +
			"names the integration user the key acts as, and that user's roles are what\n" +
			"decide every permission, so an unexpected 403 elsewhere is usually\n" +
			"explained by the identity this returns. Reads a single sys_user row.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := url.Values{}
			v.Set("sysparm_query", "sys_id=javascript:gs.getUserID()")
			v.Set("sysparm_limit", "1")
			v.Set("sysparm_fields", "sys_id,user_name,name,email")
			body, err := c.callTable(cmd.Context(), http.MethodGet, "sys_user", "", v, nil)
			if err != nil {
				return err
			}
			result, err := unwrapResult(body)
			if err != nil {
				return err
			}
			// The query returns an array; unwrap to the single user object when
			// present so the identity echo is an object, not a one-element array.
			var rows []json.RawMessage
			if json.Unmarshal(result, &rows) == nil && len(rows) == 1 {
				return s.emitJSON(rows[0])
			}
			return s.emitJSON(result)
		},
	}
}
