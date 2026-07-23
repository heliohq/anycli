package customerio

import (
	"net/http"

	"github.com/spf13/cobra"
)

func (s *Service) newWorkspaceListCmd(key string) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List workspaces (GET /v1/workspaces); doubles as the connectivity check",
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
