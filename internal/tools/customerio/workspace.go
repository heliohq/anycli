package customerio

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newWorkspaceListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workspaces (GET /v1/workspaces); doubles as the connectivity check",
		Long: "The one call that needs no ids and no permissions beyond a valid key,\n" +
			"which makes it the way to tell a bad credential apart from a bad request\n" +
			"when something else returns 401. It also names the workspace the key is\n" +
			"bound to — a key reaches exactly one, so a missing campaign or segment is\n" +
			"often the wrong workspace rather than a deleted object.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd, key, http.MethodGet, "/v1/workspaces", nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
}
