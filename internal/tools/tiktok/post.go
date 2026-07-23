package tiktok

import (
	"net/http"

	"github.com/spf13/cobra"
)

// newCreatorCmd exposes the content-posting prerequisites: `tiktok creator info`
// returns the account's allowed privacy levels, interaction toggles, and video
// duration limit, which a caller must consult before a direct post.
func (s *Service) newCreatorCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "creator", Short: "Content-posting prerequisites"}
	cmd.AddCommand(&cobra.Command{
		Use:   "info",
		Short: "Query posting options and limits for the creator",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/post/publish/creator_info/query/", nil, map[string]any{})
			if err != nil {
				return err
			}
			return s.emit(data)
		},
	})
	return cmd
}

func (s *Service) newPostCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "post", Short: "Publish and track content"}
	cmd.AddCommand(
		s.newPostVideoCmd(token),
		s.newPostStatusCmd(token),
	)
	return cmd
}

func (s *Service) newPostVideoCmd(token string) *cobra.Command {
	var title, file, videoURL, privacy string
	var draft bool
	cmd := &cobra.Command{
		Use:   "video",
		Short: "Post a video (direct post) or upload it as a draft",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireExactlyOne("--file", file, "--url", videoURL); err != nil {
				return err
			}

			source, upload, err := s.buildSource(file, videoURL)
			if err != nil {
				return err
			}

			body := map[string]any{"source_info": source}
			path := "/v2/post/publish/inbox/video/init/"
			if !draft {
				// Direct post also carries post_info; privacy_level is required
				// and must be one of the values from `tiktok creator info`.
				if privacy == "" {
					return errRequired("--privacy (required for direct post; use --draft to upload without publishing)")
				}
				body["post_info"] = map[string]any{
					"privacy_level":        privacy,
					"title":                title,
					"brand_content_toggle": false,
					"brand_organic_toggle": false,
				}
				path = "/v2/post/publish/video/init/"
			}

			data, err := s.call(cmd.Context(), token, http.MethodPost, path, nil, body)
			if err != nil {
				return err
			}
			if upload != nil {
				if err := s.uploadFile(cmd.Context(), data, upload); err != nil {
					return err
				}
			}
			return s.emit(data)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "video caption/title (direct post)")
	cmd.Flags().StringVar(&file, "file", "", "local video file to upload")
	cmd.Flags().StringVar(&videoURL, "url", "", "public URL TikTok pulls the video from")
	cmd.Flags().StringVar(&privacy, "privacy", "", "privacy level for direct post (e.g. PUBLIC_TO_EVERYONE, SELF_ONLY)")
	cmd.Flags().BoolVar(&draft, "draft", false, "upload to the creator's inbox as a draft instead of posting")
	return cmd
}

func (s *Service) newPostStatusCmd(token string) *cobra.Command {
	var publishID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Fetch the processing status of a post",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if publishID == "" {
				return errRequired("--publish-id")
			}
			body := map[string]any{"publish_id": publishID}
			data, err := s.call(cmd.Context(), token, http.MethodPost, "/v2/post/publish/status/fetch/", nil, body)
			if err != nil {
				return err
			}
			return s.emit(data)
		},
	}
	cmd.Flags().StringVar(&publishID, "publish-id", "", "publish id returned by `post video` (required)")
	return cmd
}
