package x

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const maxSimpleImageBytes = 5 << 20

// supportedSimpleImageTypes are the content types the one-shot simple upload
// API accepts; anything else (video, GIF, oversized images) goes chunked.
var supportedSimpleImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

func (s *Service) newMediaCmd(token string) *cobra.Command {
	cmd := &cobra.Command{Use: "media", Short: "Media"}
	cmd.AddCommand(
		s.newMediaUploadCmd(token),
		s.newMediaStatusCmd(token),
		s.newMediaMetadataCmd(token),
	)
	return cmd
}

func (s *Service) newMediaUploadCmd(token string) *cobra.Command {
	var file, category string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload one image, GIF, or video and wait until it is ready to attach",
		Long: "Prints the media id to hand to `post create --media-id`, `post quote\n" +
			"--media-id` or `dm send --media-id`. Accepts JPEG, PNG, WebP and GIF\n" +
			"images and .mp4, .webm, .mov or .ts video, with ceilings of 15 MB for a\n" +
			"GIF and 512 MB for video. Small images (JPEG/PNG/WebP up to 5 MB) take the\n" +
			"one-shot path; everything else is uploaded in 4 MiB segments with progress\n" +
			"on stderr and then polled for up to 5 minutes.\n" +
			"\n" +
			"--category defaults to a tweet_* value derived from the file type, so\n" +
			"media destined for a DM must set it explicitly (dm_image, dm_video,\n" +
			"dm_gif) — `dm send` will not accept a tweet_image id.\n" +
			"\n" +
			"If the 5-minute wait expires the media id is still valid: poll\n" +
			"`media status` and attach it once the state is succeeded.",
		Args:        cobra.NoArgs,
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireMediaCategory(category); err != nil {
				return err
			}
			info, err := os.Stat(file)
			if err != nil {
				return fmt.Errorf("read media file: %w", err)
			}
			if info.IsDir() {
				return fmt.Errorf("--file %q is a directory", file)
			}
			if info.Size() == 0 {
				return fmt.Errorf("--file %q is empty", file)
			}
			sniff, err := sniffMediaFile(file)
			if err != nil {
				return err
			}
			mediaType, defaultCategory, err := mediaTypeForUpload(sniff, file)
			if err != nil {
				return err
			}
			if category == "" {
				category = defaultCategory
			}
			var body []byte
			_, simple := supportedSimpleImageTypes[mediaType]
			if simple && info.Size() <= maxSimpleImageBytes && strings.HasSuffix(category, "_image") {
				body, err = s.simpleUpload(cmd.Context(), token, file, mediaType, category)
			} else {
				body, err = s.chunkedUpload(cmd.Context(), token, file, category)
			}
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "media file to upload (image, GIF, or video)")
	cmd.Flags().StringVar(&category, "category", "", "media use: tweet_image, tweet_video, tweet_gif, dm_image, dm_video, dm_gif, or amplify_video (empty = derived from the file type)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func (s *Service) newMediaStatusCmd(token string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <media-id>",
		Short: "Get media upload processing status",
		Long: "Rarely needed on its own — `media upload` already waits for the media to\n" +
			"become attachable. This is the escape hatch after that wait times out:\n" +
			"attach the id once `processing_info.state` is succeeded, or read\n" +
			"`processing_info.error` if it is failed.",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("media id", args[0]); err != nil {
				return err
			}
			query := url.Values{"media_id": {args[0]}, "command": {"STATUS"}}
			body, err := s.call(cmd.Context(), token, http.MethodGet, "/2/media/upload", query, nil)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
}

func (s *Service) newMediaMetadataCmd(token string) *cobra.Command {
	var altText string
	cmd := &cobra.Command{
		Use: "metadata <media-id>",
		Long: "Alt text is capped at 1000 characters. Order matters: set it after\n" +
			"`media upload` and BEFORE the media is attached to a post — X ignores\n" +
			"metadata written against media that is already published.",
		Short:       "Set media alt text",
		Args:        cobra.ExactArgs(1),
		Annotations: sideEffect(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNumericID("media id", args[0]); err != nil {
				return err
			}
			if altText == "" {
				return fmt.Errorf("alt text must not be empty")
			}
			if utf8.RuneCountInString(altText) > 1000 {
				return fmt.Errorf("alt text must not exceed 1000 characters")
			}
			payload := struct {
				ID       string `json:"id"`
				Metadata struct {
					AltText struct {
						Text string `json:"text"`
					} `json:"alt_text"`
				} `json:"metadata"`
			}{ID: args[0]}
			payload.Metadata.AltText.Text = altText
			body, err := s.call(cmd.Context(), token, http.MethodPost, "/2/media/metadata", nil, payload)
			if err != nil {
				return err
			}
			return s.emit(body)
		},
	}
	cmd.Flags().StringVar(&altText, "alt-text", "", "accessible description (maximum 1000 characters)")
	_ = cmd.MarkFlagRequired("alt-text")
	return cmd
}

// simpleUpload posts one small JPEG, PNG, or WebP through the one-shot
// /2/media/upload endpoint and returns the response body.
func (s *Service) simpleUpload(ctx context.Context, token, file, mediaType, category string) ([]byte, error) {
	contents, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	payload := struct {
		Media         string `json:"media"`
		MediaType     string `json:"media_type"`
		MediaCategory string `json:"media_category"`
	}{
		Media:         base64.StdEncoding.EncodeToString(contents),
		MediaType:     mediaType,
		MediaCategory: category,
	}
	return s.call(ctx, token, http.MethodPost, "/2/media/upload", nil, payload)
}
