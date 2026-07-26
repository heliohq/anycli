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
		Long: "Run this before `post video`: it returns `privacy_level_options`, and\n" +
			"`post video --privacy` must be one of exactly those values — the set\n" +
			"differs per account, and a private or under-age account does not offer\n" +
			"`PUBLIC_TO_EVERYONE` at all. It also carries `max_video_post_duration_sec`\n" +
			"and the `comment_disabled` / `duet_disabled` / `stitch_disabled` settings\n" +
			"the creator has chosen, which is the only place those limits are visible.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
		Long: "Exactly one of `--url` or `--file` is required. `--url` has TikTok PULL the\n" +
			"video from a public address, which needs that domain verified in the TikTok\n" +
			"developer portal; `--file` uploads the local bytes as a SINGLE chunk, so a\n" +
			"large file is one long PUT with no resume if it breaks.\n" +
			"\n" +
			"Without `--draft` this is a direct post that publishes to the profile, and\n" +
			"`--privacy` is then required — one of the values `creator info` lists for\n" +
			"this account. `--draft` instead drops the video into the creator's TikTok\n" +
			"inbox to finish by hand, takes no privacy level, and IGNORES `--title`,\n" +
			"which only travels on the direct-post path.\n" +
			"\n" +
			"The branded-content and branded-organic toggles are always sent as false,\n" +
			"so a paid-partnership disclosure cannot be set from here. TikTok also\n" +
			"restricts direct posting to audited apps: on an unaudited one, posts are\n" +
			"forced private and limited to the app's test users. The response is a\n" +
			"`publish_id` and nothing more — the video is not live yet, and `post\n" +
			"status` is what decides that.",
		Args:        cobra.NoArgs,
		Annotations: writeAction,
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
		Long: "`--publish-id` is required and comes from `post video`. Publishing is\n" +
			"asynchronous, so this has to be polled: the id stays reachable while TikTok\n" +
			"downloads and transcodes, and only `PUBLISH_COMPLETE` means the video is\n" +
			"live (`SEND_TO_USER_INBOX` means it landed as a draft instead). A `FAILED`\n" +
			"status carries the reason, and it is the ONLY place a rejected upload\n" +
			"surfaces — `post video` returns successfully long before TikTok has judged\n" +
			"the file.",
		Args:        cobra.NoArgs,
		Annotations: readOnly,
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
