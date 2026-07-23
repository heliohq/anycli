package novu

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newIntegrationCmd is the read-only `integration` group over /v1/integrations:
// which channel providers are configured. These endpoints return a bare JSON
// array (no data wrapper), which emit passes through unchanged.
func (s *Service) newIntegrationCmd(c *client) *cobra.Command {
	group := newGroupCmd("integration", "Inspect configured channel providers (read-only)")
	group.AddCommand(
		s.newIntegrationListCmd(c),
		s.newIntegrationActiveCmd(c),
	)
	return group
}

func (s *Service) newIntegrationListCmd(c *client) *cobra.Command {
	return leafCmd("list", "List all integrations", readOnly, func(cmd *cobra.Command, _ []string) error {
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/integrations", nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
}

func (s *Service) newIntegrationActiveCmd(c *client) *cobra.Command {
	return leafCmd("active", "List active integrations", readOnly, func(cmd *cobra.Command, _ []string) error {
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/integrations/active", nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
}
