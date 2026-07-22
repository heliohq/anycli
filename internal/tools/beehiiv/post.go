package beehiiv

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func (s *Service) newPostCmd(token string) *cobra.Command {
	cmd := newGroupCmd("post", "Newsletter posts (list, get) — read-only")
	cmd.AddCommand(
		s.newPostListCmd(token),
		s.newPostGetCmd(token),
	)
	return cmd
}

func (s *Service) newPostListCmd(token string) *cobra.Command {
	var (
		expand                                               []string
		status, audience, platform, orderBy, direction, page string
		limit                                                string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List posts with stats (GET /publications/{pub}/posts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			q := url.Values{}
			addExpand(q, expand)
			setIfNonEmpty(q, "status", status)
			setIfNonEmpty(q, "audience", audience)
			setIfNonEmpty(q, "platform", platform)
			setIfNonEmpty(q, "order_by", orderBy)
			setIfNonEmpty(q, "direction", direction)
			setIfNonEmpty(q, "limit", limit)
			setIfNonEmpty(q, "page", page)
			resp, err := s.call(cmd.Context(), token, http.MethodGet, "/publications/"+pub+"/posts", q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringSliceVar(&expand, "expand", nil, "extra fields: stats, free_web_content, recipients, … (repeatable)")
	cmd.Flags().StringVar(&status, "status", "", "filter by status: draft|confirmed|archived|all")
	cmd.Flags().StringVar(&audience, "audience", "", "filter by audience: free|premium|all")
	cmd.Flags().StringVar(&platform, "platform", "", "filter by platform: web|email|both|all")
	cmd.Flags().StringVar(&orderBy, "order-by", "", "sort field: created|publish_date|displayed_date")
	cmd.Flags().StringVar(&direction, "direction", "", "sort direction: asc|desc")
	cmd.Flags().StringVar(&limit, "limit", "", "page size (1-100)")
	cmd.Flags().StringVar(&page, "page", "", "page number")
	return cmd
}

func (s *Service) newPostGetCmd(token string) *cobra.Command {
	var expand []string
	cmd := &cobra.Command{
		Use:   "get <postId>",
		Short: "Get one post (GET /publications/{pub}/posts/{postId})",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pubID, err := cmd.Flags().GetString("publication-id")
			if err != nil {
				return err
			}
			pub, err := requirePublicationID(pubID)
			if err != nil {
				return err
			}
			q := url.Values{}
			addExpand(q, expand)
			resp, err := s.call(cmd.Context(), token, http.MethodGet,
				"/publications/"+pub+"/posts/"+url.PathEscape(args[0]), q, nil)
			if err != nil {
				return err
			}
			return s.emit(resp)
		},
	}
	addPublicationFlag(cmd)
	cmd.Flags().StringSliceVar(&expand, "expand", nil, "extra fields: stats, free_web_content, recipients, … (repeatable)")
	return cmd
}
