package typefully

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// newLinkedInCmd groups LinkedIn-specific helpers (org mention resolution).
func (s *Service) newLinkedInCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "linkedin", Short: "LinkedIn helpers"}
	cmd.AddCommand(s.newLinkedInResolveOrgCmd(token))
	return cmd
}

func (s *Service) newLinkedInResolveOrgCmd(token string) *cobra.Command {
	var socialSet, organizationURL string
	cmd := &cobra.Command{
		Use:         "resolve-org",
		Short:       "Resolve a LinkedIn org URL to a mention (GET /v2/social-sets/{id}/linkedin/organizations/resolve)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("organization_url", organizationURL)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/linkedin/organizations/resolve"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&organizationURL, "organization-url", "", "LinkedIn company/organization URL; required")
	_ = cmd.MarkFlagRequired("organization-url")
	return cmd
}

// newCommentCmd groups reviewer-comment reads on drafts.
func (s *Service) newCommentCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "comment", Short: "Read reviewer comments on drafts"}
	cmd.AddCommand(s.newCommentThreadsCmd(token))
	return cmd
}

func (s *Service) newCommentThreadsCmd(token string) *cobra.Command {
	var socialSet, id, platform, status string
	var limit int
	cmd := &cobra.Command{
		Use:         "threads",
		Short:       "List comment threads on a draft (GET /v2/social-sets/{id}/drafts/{draft_id}/comment-threads)",
		Annotations: readOnly,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if platform != "" {
				q.Set("platform", platform)
			}
			if status != "" {
				q.Set("status", status)
			}
			addPaging(q, limit, 0)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, scopedPath(socialSet, "/drafts/"+id+"/comment-threads"), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addSocialSetFlag(cmd, &socialSet)
	cmd.Flags().StringVar(&id, "id", "", "draft id; required")
	_ = cmd.MarkFlagRequired("id")
	cmd.Flags().StringVar(&platform, "platform", "", "filter by platform")
	cmd.Flags().StringVar(&status, "status", "", "filter by thread status")
	cmd.Flags().IntVar(&limit, "limit", 0, "max threads (omitted = provider default)")
	return cmd
}
