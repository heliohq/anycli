package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// The segment read Longs. They live here because the computed-vs-manual
// distinction against `list` is segment-specific and the generic builders in
// common.go cannot express it.
const (
	longSegmentList = "Segments are COMPUTED audiences defined by conditions, which is why nothing\n" +
		"here adds or removes a member — membership follows the definition. `list`\n" +
		"is the manually managed counterpart, and only it has add/remove commands."

	longSegmentGet = "One segment's record and its definition. Its members are not included;\n" +
		"those come from `segment profiles <id>`, which recomputes them."
)

// newSegmentCmd builds the `segment` group: list/get plus a member-profiles
// read.
func (s *Service) newSegmentCmd(token string) *cobra.Command {
	group := newGroupCmd("segment", "Read segments and their membership")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List segments (GET /segments)", longSegmentList, "/segments", "segment"),
		s.newResourceGetCmd(token, "get", "Get one segment (GET /segments/{id})", longSegmentGet, "/segments/", "segment"),
		s.newSegmentProfilesCmd(token),
	)
	return group
}

func (s *Service) newSegmentProfilesCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:   "profiles <id>",
		Short: "List a segment's member profiles (GET /segments/{id}/profiles)",
		Long: "The segment's members right now. Membership is evaluated from the segment's\n" +
			"conditions rather than stored, so two calls can differ without anybody\n" +
			"editing anything. The shared --filter and --sort apply to the PROFILES,\n" +
			"not to the segment. Cursor-paged, so a large segment is many calls.",
		Args:        cobra.ExactArgs(1),
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := f.query("profile")
			if err != nil {
				return err
			}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/segments/"+url.PathEscape(args[0])+"/profiles", q, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	registerListFlags(cmd, f)
	return cmd
}
