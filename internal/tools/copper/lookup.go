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
	long  string
}

var lookupEndpoints = []lookupEndpoint{
	{name: "pipelines", path: "/pipelines", short: "List opportunity pipelines", long: "The `id` here is the `pipeline_id` an `opportunity create` or\n" +
		"`opportunity update` payload needs. Each pipeline owns its OWN stages, so a\n" +
		"pipeline id and a stage id must come from the same pipeline — Copper rejects\n" +
		"the mismatch rather than correcting it."},
	{name: "pipeline-stages", path: "/pipeline_stages", short: "List pipeline stages", long: "Returns every stage across EVERY pipeline, not one pipeline's, so check each\n" +
		"entry's parent pipeline before using its id — an opportunity whose stage\n" +
		"belongs to another pipeline is rejected. Stages also carry a win probability,\n" +
		"which is where a deal's forecast weighting comes from."},
	{name: "customer-sources", path: "/customer_sources", short: "List customer sources", long: "The `id` here is the `customer_source_id` on a lead or opportunity payload:\n" +
		"where the business came from. The list is configured per account, so these\n" +
		"are this customer's own vocabulary and a plausible-sounding source that is\n" +
		"not in the list will be rejected."},
	{name: "loss-reasons", path: "/loss_reasons", short: "List opportunity loss reasons", long: "The `id` here is the `loss_reason_id` to send when closing an opportunity as\n" +
		"`Lost`. Copper accepts no free text for it, so a lost deal closed without one\n" +
		"of these ids records no reason at all — which is what leaves lost-deal\n" +
		"reporting empty later."},
	{name: "activity-types", path: "/activity_types", short: "List activity types", long: "An activity's `type` is an object of `{\"category\":..., \"id\":...}` and BOTH\n" +
		"halves come from here. `user` categories are the ones a person logs (call,\n" +
		"meeting, note); `system` categories are Copper's own automatic entries and\n" +
		"cannot be created. Pass the pair verbatim into `activity create`."},
	{name: "contact-types", path: "/contact_types", short: "List contact types", long: "The `id` here is the contact-type id on a person payload — the account's own\n" +
		"classification of a contact, such as customer, prospect or partner. It is\n" +
		"configured per account, so the vocabulary differs between Copper instances\n" +
		"and has to be read rather than assumed."},
}

// newLookupCmd exposes the read-only id→name lookup tables an agent needs to
// build valid create/update payloads.
func (s *Service) newLookupCmd(token string) *cobra.Command {
	group := newGroupCmd("lookup", "Read-only id lookups (pipelines, stages, sources, types)")
	for _, e := range lookupEndpoints {
		e := e
		group.AddCommand(&cobra.Command{
			Use:         e.name,
			Short:       e.short + " (GET " + e.path + ")",
			Long:        e.long,
			Args:        cobra.NoArgs,
			Annotations: readOnly,
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
