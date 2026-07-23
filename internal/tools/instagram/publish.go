package instagram

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// mediaTypes is the closed set accepted by `publish create --media-type`.
// IMAGE is a feed image; REELS and STORIES are video containers.
var mediaTypes = map[string]bool{"IMAGE": true, "REELS": true, "STORIES": true}

func (s *Service) newPublishCreateCmd(token string) *cobra.Command {
	var (
		imageURL  string
		videoURL  string
		caption   string
		mediaType string
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a media container (POST /me/media) -> container id",
		Annotations: writeAction,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (imageURL == "") == (videoURL == "") {
				return &usageError{msg: "exactly one of --image-url or --video-url is required"}
			}
			if mediaType != "" && !mediaTypes[mediaType] {
				return &usageError{msg: "--media-type must be one of IMAGE, REELS, STORIES"}
			}
			form := url.Values{}
			if imageURL != "" {
				form.Set("image_url", imageURL)
			}
			if videoURL != "" {
				form.Set("video_url", videoURL)
			}
			if caption != "" {
				form.Set("caption", caption)
			}
			if mediaType != "" {
				form.Set("media_type", mediaType)
			}
			body, err := s.postForm(cmd.Context(), token, "/me/media", form)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	cmd.Flags().StringVar(&imageURL, "image-url", "", "publicly reachable image URL")
	cmd.Flags().StringVar(&videoURL, "video-url", "", "publicly reachable video URL (REELS/STORIES)")
	cmd.Flags().StringVar(&caption, "caption", "", "media caption")
	cmd.Flags().StringVar(&mediaType, "media-type", "", "IMAGE|REELS|STORIES (default IMAGE for --image-url)")
	return cmd
}

// containerStatus is the poll response for a media container.
type containerStatus struct {
	StatusCode string `json:"status_code"`
	ID         string `json:"id"`
}

func (s *Service) newPublishStatusCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "status <container_id>",
		Short:       "Poll a container's processing status (GET /{container_id}?fields=status_code)",
		Annotations: readOnly,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("fields", "status_code")
			body, err := s.get(cmd.Context(), token, "/"+url.PathEscape(args[0]), q)
			if err != nil {
				return err
			}
			// A terminal ERROR/EXPIRED container can never be published; surface
			// it as exit 1 so `publish finish` is not fired blindly on it.
			var cs containerStatus
			if json.Unmarshal(body, &cs) == nil {
				switch cs.StatusCode {
				case "ERROR", "EXPIRED":
					return &apiError{msg: fmt.Sprintf(
						"instagram container %s is %s and cannot be published; recreate it",
						args[0], cs.StatusCode)}
				}
			}
			return s.emitJSON(body)
		},
	}
	return cmd
}

func (s *Service) newPublishFinishCmd(token string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "finish <container_id>",
		Short:       "Publish a FINISHED container (POST /me/media_publish)",
		Annotations: writeAction,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			form := url.Values{}
			form.Set("creation_id", args[0])
			body, err := s.postForm(cmd.Context(), token, "/me/media_publish", form)
			if err != nil {
				return err
			}
			return s.emitJSON(body)
		},
	}
	return cmd
}
