package klaviyo

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSegmentCmd builds the `segment` group: list/get plus a member-profiles
// read.
func (s *Service) newSegmentCmd(token string) *cobra.Command {
	group := newGroupCmd("segment", "Read segments and their membership")
	group.AddCommand(
		s.newCollectionListCmd(token, "list", "List segments (GET /segments)", "/segments", "segment"),
		s.newResourceGetCmd(token, "get", "Get one segment (GET /segments/{id})", "/segments/", "segment"),
		s.newSegmentProfilesCmd(token),
	)
	return group
}

func (s *Service) newSegmentProfilesCmd(token string) *cobra.Command {
	f := &listFlags{}
	cmd := &cobra.Command{
		Use:   "profiles <id>",
		Short: "List a segment's member profiles (GET /segments/{id}/profiles)",
		Args:  cobra.ExactArgs(1),
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
