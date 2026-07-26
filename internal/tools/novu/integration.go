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

// The integration Longs, grouped above the constructors that use them because
// both leaves are built by the shared leafCmd helper.
const (
	longIntegrationList = "Every configured channel provider, active or not, with the channel it\n" +
		"serves. A workflow step can only deliver through a provider configured\n" +
		"here, so an email step in an environment with no email integration\n" +
		"produces nothing while the trigger still reports processed. The response\n" +
		"is a bare JSON array, not a data-wrapped object."

	longIntegrationActive = "The subset actually in service — the honest answer to whether a channel\n" +
		"works right now, since `integration list` also returns disabled entries\n" +
		"that look configured and deliver nothing. Read-only: providers are\n" +
		"configured in the Novu dashboard, not from here."
)

func (s *Service) newIntegrationListCmd(c *client) *cobra.Command {
	return leafCmd("list", "List all integrations", longIntegrationList, readOnly, func(cmd *cobra.Command, _ []string) error {
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/integrations", nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
}

func (s *Service) newIntegrationActiveCmd(c *client) *cobra.Command {
	return leafCmd("active", "List active integrations", longIntegrationActive, readOnly, func(cmd *cobra.Command, _ []string) error {
		out, err := c.call(cmd.Context(), http.MethodGet, "/v1/integrations/active", nil, nil)
		if err != nil {
			return err
		}
		return s.emit(out)
	})
}
