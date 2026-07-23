package typefully

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newSocialSetCmd groups social-set discovery — the first call an agent makes,
// since every other scoped command needs a social_set_id.
func (s *Service) newSocialSetCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "social-set", Short: "Discover social sets (connected accounts)"}
	cmd.AddCommand(s.newSocialSetListCmd(token), s.newSocialSetGetCmd(token))
	return cmd
}

func (s *Service) newSocialSetListCmd(token string) *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List social sets (GET /v2/social-sets)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			addPaging(q, limit, offset)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/social-sets", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	registerPaging(cmd, &limit, &offset)
	return cmd
}

func (s *Service) newSocialSetGetCmd(token string) *cobra.Command {
	var socialSet string
	cmd := &cobra.Command{
		Use:         "get",
		Short:       "Get one social set's connected platforms + quota (GET /v2/social-sets/{id}/)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/"), nil, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	return cmd
}
