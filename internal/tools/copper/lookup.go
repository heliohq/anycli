package copper

import (
	"net/http"

	"github.com/spf13/cobra"
)

// lookupEndpoint maps a lookup subcommand word to its Copper GET path. These
// resolve the ids that create/update payloads reference (pipeline_id,
// pipeline_stage_id, customer_source_id, loss_reason_id, activity type id,
// contact_type_id).
type lookupEndpoint struct {
	name  string
	path  string
	short string
}

var lookupEndpoints = []lookupEndpoint{
	{name: "pipelines", path: "/pipelines", short: "List opportunity pipelines"},
	{name: "pipeline-stages", path: "/pipeline_stages", short: "List pipeline stages"},
	{name: "customer-sources", path: "/customer_sources", short: "List customer sources"},
	{name: "loss-reasons", path: "/loss_reasons", short: "List opportunity loss reasons"},
	{name: "activity-types", path: "/activity_types", short: "List activity types"},
	{name: "contact-types", path: "/contact_types", short: "List contact types"},
}

// newLookupCmd exposes the read-only id→name lookup tables an agent needs to
// build valid create/update payloads.
func (s *Service) newLookupCmd(token string) *cobra.Command {
	group := newGroupCmd("lookup", "Read-only id lookups (pipelines, stages, sources, types)")
	for _, e := range lookupEndpoints {
		e := e
		group.AddCommand(&cobra.Command{
			Use:   e.name,
			Short: e.short + " (GET " + e.path + ")",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				resp, err := s.call(cmd.Context(), token, http.MethodGet, e.path, nil)
				if err != nil {
					return err
				}
				return s.emit(resp)
			},
		})
	}
	return group
}
